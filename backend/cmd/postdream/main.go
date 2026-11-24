// Command postdream runs the SMTP capture server, the HTTP API/UI, the optional
// POP3 server, the background job workers and the retention janitor.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/netutil"

	"github.com/matiasrohleder/PostDream/internal/api"
	"github.com/matiasrohleder/PostDream/internal/config"
	"github.com/matiasrohleder/PostDream/internal/db"
	"github.com/matiasrohleder/PostDream/internal/events"
	"github.com/matiasrohleder/PostDream/internal/ingest"
	"github.com/matiasrohleder/PostDream/internal/jobs"
	"github.com/matiasrohleder/PostDream/internal/linkcheck"
	"github.com/matiasrohleder/PostDream/internal/netguard"
	"github.com/matiasrohleder/PostDream/internal/pop3"
	"github.com/matiasrohleder/PostDream/internal/relay"
	"github.com/matiasrohleder/PostDream/internal/retention"
	"github.com/matiasrohleder/PostDream/internal/smtpd"
	"github.com/matiasrohleder/PostDream/internal/spam"
	"github.com/matiasrohleder/PostDream/internal/store"
	"github.com/matiasrohleder/PostDream/internal/webhook"
	"github.com/matiasrohleder/PostDream/web"
)

var version = "0.1.0-dev"

func main() {
	level := slog.LevelInfo
	if strings.EqualFold(os.Getenv("LOG_LEVEL"), "debug") {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	relayCfg, err := relay.Parse(cfg.RelaySMTPURL, cfg.RelayFrom)
	if err != nil {
		return err
	}
	api.Version = version
	userAgent := "PostDream/" + version
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	st := store.New(pool)
	if err := bootstrap(ctx, cfg, st); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	// Live events go through Postgres LISTEN/NOTIFY so several replicas can share the load.
	hub := events.NewHub()
	hub.Attach(ctx, pool)

	policy := netguard.Policy{AllowPrivate: cfg.OutboundAllowPrivate, Allowed: cfg.OutboundAllowedHosts}
	runner := &jobs.Runner{Store: st, Workers: cfg.JobWorkers}
	checker := spam.New(cfg)
	pipeline := &ingest.Pipeline{Store: st, Hub: hub, Jobs: runner, SpamEnabled: checker != nil, PublicURL: cfg.PublicURL}
	deliverer := &webhook.Deliverer{Store: st, Policy: policy, UserAgent: userAgent}

	pop3Port := ""
	if cfg.POP3Addr != "" {
		_, pop3Port, _ = net.SplitHostPort(cfg.POP3Addr)
	}
	apiServer := api.New(api.Deps{
		Config: cfg, Store: st, Hub: hub, Pipeline: pipeline, Spam: checker,
		Links:    &linkcheck.Checker{Store: st, Policy: policy, UserAgent: userAgent},
		Webhooks: deliverer, Relay: relayCfg, Static: web.FS(), POP3Port: pop3Port, UserAgent: userAgent,
	})
	runner.Handle(webhook.JobKind, deliverer.Handle)
	runner.Handle(spam.JobKind, apiServer.SpamJob)
	go runner.Run(ctx)

	go (&retention.Janitor{
		Store: st, Hub: hub, DefaultHours: cfg.RetentionHours, MaxPerInbox: cfg.MaxMessagesPerInbox, Interval: cfg.RetentionInterval,
	}).Run(ctx)

	smtpServer, err := smtpd.New(cfg, st, pipeline)
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	smtpLn, err := limitedListener(cfg.SMTPAddr, cfg.SMTPMaxConnections)
	if err != nil {
		return fmt.Errorf("SMTP listen: %w", err)
	}
	httpLn, err := limitedListener(cfg.HTTPAddr, cfg.HTTPMaxConnections)
	if err != nil {
		return fmt.Errorf("HTTP listen: %w", err)
	}

	errc := make(chan error, 4)
	go func() {
		slog.Info("SMTP server listening", "addr", cfg.SMTPAddr, "auth", cfg.SMTPAuth, "tls", cfg.SMTPTLSCert != "",
			"require_tls", cfg.SMTPRequireTLS)
		errc <- smtpServer.Serve(smtpLn)
	}()
	go func() {
		slog.Info("HTTP server listening", "addr", cfg.HTTPAddr, "ui_auth", cfg.AuthEnabled)
		errc <- httpServer.Serve(httpLn)
	}()

	var pop3Server *pop3.Server
	if cfg.POP3Addr != "" {
		pop3Server = &pop3.Server{Store: st, Hub: hub, Hostname: cfg.SMTPHostname}
		if smtpServer.TLSConfig != nil {
			pop3Server.TLSConfig = smtpServer.TLSConfig.Clone()
		}
		ln, err := limitedListener(cfg.POP3Addr, cfg.SMTPMaxConnections)
		if err != nil {
			return fmt.Errorf("POP3 listen: %w", err)
		}
		go func() {
			slog.Info("POP3 server listening", "addr", cfg.POP3Addr, "stls", pop3Server.TLSConfig != nil)
			errc <- pop3Server.Serve(ln)
		}()
	}

	var metricsServer *http.Server
	if cfg.MetricsAddr != "" {
		mux := http.NewServeMux()
		mux.Handle("GET /metrics", apiServer.MetricsHandler())
		metricsServer = &http.Server{Addr: cfg.MetricsAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
		go func() {
			slog.Info("metrics listening", "addr", cfg.MetricsAddr)
			errc <- metricsServer.ListenAndServe()
		}()
	}
	slog.Info("PostDream started", "version", version, "retention_hours", cfg.RetentionHours,
		"spam_engine", cfg.SpamEngine(), "relay", relayCfg.Redacted(), "job_workers", cfg.JobWorkers)

	select {
	case <-ctx.Done():
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	smtpServer.Close()
	if pop3Server != nil {
		pop3Server.Close()
	}
	if metricsServer != nil {
		metricsServer.Shutdown(shutdownCtx)
	}
	return httpServer.Shutdown(shutdownCtx)
}

// limitedListener caps concurrent connections (0 or less = unlimited).
func limitedListener(addr string, max int) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if max > 0 {
		return netutil.LimitListener(ln, max), nil
	}
	return ln, nil
}

// bootstrap creates the first admin user and the default inbox on an empty database.
func bootstrap(ctx context.Context, cfg *config.Config, st *store.Store) error {
	n, err := st.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		pw := cfg.AdminPassword
		generated := pw == ""
		if generated {
			pw = store.RandomToken(10)
		}
		if _, err := st.CreateUser(ctx, cfg.AdminEmail, "Administrator", pw, "admin"); err != nil {
			return err
		}
		if generated {
			slog.Warn("created initial admin with a GENERATED password (set ADMIN_PASSWORD to choose one)", "email", cfg.AdminEmail, "password", pw)
		} else {
			slog.Info("created initial admin", "email", cfg.AdminEmail)
		}
	}
	if _, err := st.GetInboxBySlug(ctx, cfg.DefaultInboxSlug); errors.Is(err, store.ErrNotFound) {
		in, err := st.CreateInbox(ctx, store.InboxInput{
			Name: "Default", Slug: cfg.DefaultInboxSlug, Description: "Receives unauthenticated email",
			SMTPUsername: cfg.DefaultInboxSlug, SMTPPassword: os.Getenv("DEFAULT_INBOX_SMTP_PASSWORD"),
		})
		if err != nil {
			return err
		}
		slog.Info("created default inbox", "slug", in.Slug, "smtp_username", in.SMTPUsername)
	} else if err != nil {
		return err
	}
	return nil
}
