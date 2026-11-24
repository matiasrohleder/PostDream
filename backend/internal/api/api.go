// Package api is the JSON/HTTP API consumed by the React UI and by automated tests.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/matiasrohleder/PostDream/internal/config"
	"github.com/matiasrohleder/PostDream/internal/events"
	"github.com/matiasrohleder/PostDream/internal/ingest"
	"github.com/matiasrohleder/PostDream/internal/linkcheck"
	"github.com/matiasrohleder/PostDream/internal/metrics"
	"github.com/matiasrohleder/PostDream/internal/ratelimit"
	"github.com/matiasrohleder/PostDream/internal/relay"
	"github.com/matiasrohleder/PostDream/internal/spam"
	"github.com/matiasrohleder/PostDream/internal/store"
	"github.com/matiasrohleder/PostDream/internal/webhook"
)

const sessionCookie = "pd_session"

var Version = "dev"

// Deps groups the collaborators of the API server.
type Deps struct {
	Config    *config.Config
	Store     *store.Store
	Hub       *events.Hub
	Pipeline  *ingest.Pipeline
	Spam      spam.Checker // nil when no engine is configured
	Links     *linkcheck.Checker
	Webhooks  *webhook.Deliverer
	Relay     *relay.Config // nil when releasing is disabled
	Static    fs.FS
	POP3Port  string
	UserAgent string
}

type Server struct {
	Deps
	cfg          *config.Config
	store        *store.Store
	hub          *events.Hub
	loginLimiter *ratelimit.Limiter
	checkLimiter *ratelimit.Limiter // link checks hit the network: limit per caller
}

func New(d Deps) *Server {
	return &Server{
		Deps: d, cfg: d.Config, store: d.Store, hub: d.Hub,
		loginLimiter: ratelimit.New(10, 15*time.Minute),
		checkLimiter: ratelimit.New(30, 10*time.Minute),
	}
}

// principal is whoever is making the request: a UI user or an inbox API token.
type principal struct {
	User  *store.User  // nil when authenticated by API token
	Inbox *store.Inbox // set when authenticated by API token
}

// allowedInboxes returns nil when unrestricted (admin), else the ids the caller may see.
func (p *principal) allowedInboxes() []string {
	if p.Inbox != nil {
		return []string{p.Inbox.ID}
	}
	if p.User.IsAdmin() {
		return nil
	}
	if p.User.InboxIDs == nil {
		return []string{}
	}
	return p.User.InboxIDs
}

func (p *principal) canAccess(inboxID string) bool {
	allowed := p.allowedInboxes()
	return allowed == nil || slices.Contains(allowed, inboxID)
}

func (p *principal) isAdmin() bool { return p.User != nil && p.User.IsAdmin() }

// key identifies the caller for rate limiting.
func (p *principal) key() string {
	if p.Inbox != nil {
		return "inbox:" + p.Inbox.ID
	}
	return "user:" + p.User.ID + p.User.Email
}

// label is the human-readable identity used in audit logs.
func (p *principal) label() string {
	if p.User != nil {
		return p.User.Email
	}
	return "api-token:" + p.Inbox.Slug
}

type ctxKey struct{}

func from(r *http.Request) *principal { return r.Context().Value(ctxKey{}).(*principal) }

// access levels for routes.
type access int

const (
	public    access = iota
	anyAuth          // session or inbox API token
	userOnly         // session only
	adminOnly        // admin session
)

type route struct {
	Method, Path string
	Access       access
	handler      http.HandlerFunc
}

// Routes is the single source of truth for the API surface (also checked
// against openapi.json by a test).
func (s *Server) Routes() []route {
	return []route{
		{"GET", "/api/health", public, s.health},
		{"GET", "/api/config", public, s.publicConfig},
		{"GET", "/api/openapi.json", public, s.openAPI},
		{"POST", "/api/auth/login", public, s.login},
		{"POST", "/api/auth/logout", public, s.logout},
		{"GET", "/api/auth/me", userOnly, s.me},
		{"PUT", "/api/auth/password", userOnly, s.changePassword},

		{"GET", "/api/stats", anyAuth, s.stats},
		{"GET", "/api/events", anyAuth, s.events},

		{"GET", "/api/inboxes", anyAuth, s.listInboxes},
		{"GET", "/api/inboxes/{id}", anyAuth, s.getInbox},
		{"POST", "/api/inboxes", adminOnly, s.createInbox},
		{"PATCH", "/api/inboxes/{id}", adminOnly, s.updateInbox},
		{"DELETE", "/api/inboxes/{id}", adminOnly, s.deleteInbox},
		{"POST", "/api/inboxes/{id}/rotate", adminOnly, s.rotateInbox},
		{"PATCH", "/api/inboxes/{id}/settings", adminOnly, s.updateSettings},
		{"GET", "/api/inboxes/{id}/chaos", anyAuth, s.getChaos},
		{"PUT", "/api/inboxes/{id}/chaos", anyAuth, s.putChaos},
		{"GET", "/api/inboxes/{id}/webhooks", adminOnly, s.listWebhooks},
		{"POST", "/api/inboxes/{id}/webhooks", adminOnly, s.createWebhook},

		{"PATCH", "/api/webhooks/{id}", adminOnly, s.updateWebhook},
		{"DELETE", "/api/webhooks/{id}", adminOnly, s.deleteWebhook},
		{"POST", "/api/webhooks/{id}/rotate", adminOnly, s.rotateWebhook},
		{"POST", "/api/webhooks/{id}/test", adminOnly, s.testWebhook},
		{"GET", "/api/webhooks/{id}/deliveries", adminOnly, s.webhookDeliveries},

		{"GET", "/api/rules", adminOnly, s.listRules},
		{"POST", "/api/rules", adminOnly, s.createRule},
		{"DELETE", "/api/rules/{id}", adminOnly, s.deleteRule},

		{"GET", "/api/tags", anyAuth, s.listTags},

		{"POST", "/api/send", anyAuth, s.send},

		{"GET", "/api/messages", anyAuth, s.listMessages},
		{"GET", "/api/messages/latest", anyAuth, s.latestMessage},
		{"DELETE", "/api/messages", anyAuth, s.deleteAllMessages},
		{"POST", "/api/messages/read", anyAuth, s.markRead},
		{"POST", "/api/messages/delete", anyAuth, s.bulkDelete},
		{"POST", "/api/messages/tags", anyAuth, s.bulkTags},
		{"GET", "/api/messages/{id}", anyAuth, s.getMessage},
		{"DELETE", "/api/messages/{id}", anyAuth, s.deleteMessage},
		{"PUT", "/api/messages/{id}/tags", anyAuth, s.setTags},
		{"GET", "/api/messages/{id}/html", anyAuth, s.messageHTML},
		{"GET", "/api/messages/{id}/raw", anyAuth, s.messageRaw},
		{"GET", "/api/messages/{id}/attachments/{aid}", anyAuth, s.attachment},
		{"GET", "/api/messages/{id}/html-check", anyAuth, s.htmlCheck},
		{"GET", "/api/messages/{id}/link-check", anyAuth, s.linkCheck},
		{"GET", "/api/messages/{id}/deliverability", anyAuth, s.deliverability},
		{"GET", "/api/messages/{id}/spam", anyAuth, s.spamCheck},
		{"POST", "/api/messages/{id}/release", userOnly, s.release},
		{"GET", "/api/relay/log", adminOnly, s.relayLog},

		{"GET", "/api/users", adminOnly, s.listUsers},
		{"POST", "/api/users", adminOnly, s.createUser},
		{"PATCH", "/api/users/{id}", adminOnly, s.updateUser},
		{"DELETE", "/api/users/{id}", adminOnly, s.deleteUser},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, rt := range s.Routes() {
		pattern := rt.Method + " " + rt.Path
		switch rt.Access {
		case public:
			mux.HandleFunc(pattern, rt.handler)
		case anyAuth:
			mux.Handle(pattern, s.authenticate(rt.handler, false))
		case userOnly:
			mux.Handle(pattern, s.authenticate(rt.handler, true))
		case adminOnly:
			mux.Handle(pattern, s.authenticate(s.requireAdmin(rt.handler), true))
		}
	}
	if s.cfg.MetricsToken != "" {
		mux.HandleFunc("GET /metrics", s.metricsAuth)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, 404, "not found") })
	mux.Handle("/", s.spa())
	return s.withLogging(s.hostGuard(securityHeaders(mux)))
}

// authenticate resolves the caller from the session cookie or a Bearer API token.
func (s *Server) authenticate(next http.HandlerFunc, userOnly bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := &principal{}
		if tok := bearer(r); tok != "" {
			if userOnly {
				writeErr(w, 403, "API tokens cannot use this endpoint")
				return
			}
			inbox, err := s.store.InboxByAPIToken(r.Context(), tok)
			if err != nil {
				writeErr(w, 401, "invalid API token")
				return
			}
			p.Inbox = inbox
		} else if !s.cfg.AuthEnabled {
			p.User = &store.User{ID: "", Email: "anonymous", Name: "Anonymous", Role: "admin"}
		} else {
			c, err := r.Cookie(sessionCookie)
			if err != nil {
				writeErr(w, 401, "login required")
				return
			}
			u, err := s.store.UserBySession(r.Context(), c.Value)
			if err != nil {
				writeErr(w, 401, "session expired")
				return
			}
			p.User = u
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)))
	})
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !from(r).isAdmin() {
			writeErr(w, 403, "admin only")
			return
		}
		next(w, r)
	}
}

func bearer(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return r.Header.Get("X-Api-Token")
}

func (s *Server) clientIP(r *http.Request) string {
	if !s.cfg.TrustProxy {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		return host
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// hostGuard rejects requests whose Host header is not in ALLOWED_HOSTS (DNS rebinding protection).
func (s *Server) hostGuard(next http.Handler) http.Handler {
	if len(s.cfg.AllowedHosts) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(r.Host)
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if host == "localhost" || host == "127.0.0.1" || host == "::1" || slices.Contains(s.cfg.AllowedHosts, host) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "host not allowed", http.StatusMisdirectedRequest)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/events" && r.URL.Path != "/api/health" {
			metrics.Inc(metrics.HTTPRequests, strconv.Itoa(sw.status/100)+"xx")
			slog.Debug("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "dur", time.Since(start))
		}
	})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	return readJSONLimit(r, v, 1<<20)
}

func readJSONLimit(r *http.Request, v any, limit int64) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, limit))
	return dec.Decode(v)
}

// storeErr maps store errors to HTTP responses.
func storeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "not found")
		return
	}
	if strings.Contains(err.Error(), "duplicate key") {
		writeErr(w, 409, "already exists: "+err.Error())
		return
	}
	if strings.Contains(err.Error(), "invalid input syntax for type uuid") {
		writeErr(w, 404, "not found")
		return
	}
	slog.Error("store error", "err", err)
	writeErr(w, 500, "internal error")
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.Ping(r.Context()); err != nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok", "version": Version})
}

func (s *Server) publicConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"version":          Version,
		"auth_enabled":     s.cfg.AuthEnabled,
		"smtp_auth":        s.cfg.SMTPAuth,
		"smtp_port":        portOf(s.cfg.SMTPAddr),
		"smtp_tls":         s.cfg.SMTPTLSCert != "",
		"smtp_require_tls": s.cfg.SMTPRequireTLS,
		"pop3_port":        s.POP3Port,
		"retention_hours":  s.cfg.RetentionHours,
		"default_inbox":    s.cfg.DefaultInboxSlug,
		"max_message_mb":   s.cfg.SMTPMaxMessageBytes / (1024 * 1024),
		"spam_engine":      s.cfg.SpamEngine(),
		"relay_enabled":    s.Relay != nil,
		"metrics_enabled":  s.cfg.MetricsToken != "" || s.cfg.MetricsAddr != "",
		"webhook_events":   webhook.Events,
		"search_fields":    store.SearchFields,
	})
}

func portOf(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return port
}
