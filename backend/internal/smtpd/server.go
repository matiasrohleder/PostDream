// Package smtpd is the SMTP listener that captures every message it receives.
// Nothing is ever relayed: mail is parsed, stored and the client gets "250 OK".
package smtpd

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"regexp"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/matiasrohleder/PostDream/internal/config"
	"github.com/matiasrohleder/PostDream/internal/ingest"
	"github.com/matiasrohleder/PostDream/internal/metrics"
	"github.com/matiasrohleder/PostDream/internal/ratelimit"
	"github.com/matiasrohleder/PostDream/internal/store"
)

type Backend struct {
	cfg       *config.Config
	store     *store.Store
	pipeline  *ingest.Pipeline
	limiter   *ratelimit.Limiter
	allowedRe *regexp.Regexp // global SMTP_ALLOWED_RECIPIENTS
	rand      func(n int) int
}

func New(cfg *config.Config, st *store.Store, p *ingest.Pipeline) (*smtp.Server, error) {
	be := &Backend{cfg: cfg, store: st, pipeline: p, limiter: ratelimit.New(10, 10*time.Minute), rand: rand.IntN}
	if cfg.SMTPAllowedRecipients != "" {
		be.allowedRe = regexp.MustCompile(cfg.SMTPAllowedRecipients) // validated in config.Load
	}
	s := smtp.NewServer(be)
	s.Addr = cfg.SMTPAddr
	s.Domain = cfg.SMTPHostname
	s.ReadTimeout = 60 * time.Second
	s.WriteTimeout = 60 * time.Second
	s.MaxMessageBytes = cfg.SMTPMaxMessageBytes
	s.MaxRecipients = cfg.SMTPMaxRecipients
	// It's a sandbox: allow AUTH over plain connections unless TLS is required.
	// Put a TLS terminator in front or configure SMTP_TLS_CERT/KEY (STARTTLS) when exposing it publicly.
	s.AllowInsecureAuth = !cfg.SMTPRequireTLS
	if cfg.SMTPTLSCert != "" {
		cert, err := tls.LoadX509KeyPair(cfg.SMTPTLSCert, cfg.SMTPTLSKey)
		if err != nil {
			return nil, fmt.Errorf("load SMTP TLS cert: %w", err)
		}
		s.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	}
	return s, nil
}

func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	ip := ""
	if addr, ok := c.Conn().RemoteAddr().(*net.TCPAddr); ok {
		ip = addr.IP.String()
	}
	return &session{b: b, conn: c, ip: ip}, nil
}

type session struct {
	b        *Backend
	conn     *smtp.Conn
	ip       string
	inbox    *store.Inbox // set after AUTH, or the default inbox for anonymous mail
	authed   bool
	smtpUser string
	from     string
	to       []string
}

func reject(reason string, err *smtp.SMTPError) error {
	metrics.Inc(metrics.SMTPRejected, reason)
	return err
}

func (s *session) tlsActive() bool {
	_, ok := s.conn.TLSConnectionState()
	return ok
}

func (s *session) AuthMechanisms() []string {
	if s.b.cfg.SMTPAuth == config.SMTPAuthOff {
		return nil
	}
	return []string{sasl.Plain, sasl.Login}
}

func (s *session) Auth(mech string) (sasl.Server, error) {
	check := func(username, password string) error {
		if s.b.limiter.Blocked(s.ip) {
			return &smtp.SMTPError{Code: 421, EnhancedCode: smtp.EnhancedCode{4, 7, 0}, Message: "Too many failed AUTH attempts, try later"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		inbox, err := s.b.store.InboxBySMTPCredentials(ctx, username, password)
		if err != nil {
			s.b.limiter.Fail(s.ip)
			metrics.Inc(metrics.SMTPAuthFailures)
			slog.Warn("smtp auth failed", "ip", s.ip, "user", username)
			return smtp.ErrAuthFailed
		}
		s.b.limiter.Reset(s.ip)
		if err := s.b.chaos(inbox, "auth"); err != nil {
			return err
		}
		s.inbox, s.authed, s.smtpUser = inbox, true, username
		return nil
	}
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			return check(username, password)
		}), nil
	case sasl.Login:
		return &loginServer{authenticate: check}, nil
	}
	return nil, smtp.ErrAuthUnknownMechanism
}

// resolveInbox picks the inbox for anonymous sessions (the default inbox).
func (s *session) resolveInbox() error {
	if s.inbox != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	inbox, err := s.b.store.GetInboxBySlug(ctx, s.b.cfg.DefaultInboxSlug)
	if err != nil {
		slog.Error("default inbox not found", "slug", s.b.cfg.DefaultInboxSlug, "err", err)
		return reject("no_default_inbox", &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1},
			Message: "No default inbox; authenticate with inbox credentials"})
	}
	s.inbox = inbox
	return nil
}

func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	if s.b.cfg.SMTPRequireTLS && !s.tlsActive() {
		return reject("tls_required", &smtp.SMTPError{Code: 530, EnhancedCode: smtp.EnhancedCode{5, 7, 0}, Message: "Must issue a STARTTLS command first"})
	}
	if s.b.cfg.SMTPAuth == config.SMTPAuthRequired && !s.authed {
		metrics.Inc(metrics.SMTPRejected, "auth_required")
		return smtp.ErrAuthRequired
	}
	if err := s.resolveInbox(); err != nil {
		return err
	}
	if err := s.b.chaos(s.inbox, "mail"); err != nil {
		return err
	}
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	if s.b.allowedRe != nil && !s.b.allowedRe.MatchString(to) {
		return reject("recipient_not_allowed", &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "Recipient not allowed"})
	}
	if s.inbox != nil && s.inbox.AllowedRecipients != "" {
		re, err := regexp.Compile(s.inbox.AllowedRecipients)
		if err == nil && !re.MatchString(to) {
			return reject("recipient_not_allowed", &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "Recipient not allowed for this inbox"})
		}
	}
	if err := s.b.chaos(s.inbox, "rcpt"); err != nil {
		return err
	}
	s.to = append(s.to, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if err := s.resolveInbox(); err != nil {
		return err
	}
	if err := s.b.chaos(s.inbox, "data"); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = s.b.pipeline.Ingest(ctx, ingest.Envelope{
		InboxID: s.inbox.ID, From: s.from, To: s.to, ClientIP: s.ip, SMTPUser: s.smtpUser, Source: "smtp",
	}, raw)
	if err != nil {
		slog.Error("store message failed", "err", err)
		return reject("storage_failure", &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary storage failure"})
	}
	return nil
}

// chaos returns an injected SMTP error when the inbox has a fault rule for this
// stage and the dice say so.
func (b *Backend) chaos(inbox *store.Inbox, stage string) error {
	if inbox == nil {
		return nil
	}
	var rule *store.ChaosRule
	switch stage {
	case "auth":
		rule = inbox.Chaos.Auth
	case "mail":
		rule = inbox.Chaos.Mail
	case "rcpt":
		rule = inbox.Chaos.Rcpt
	case "data":
		rule = inbox.Chaos.Data
	}
	if rule == nil || rule.Probability <= 0 || rule.Code < 400 || rule.Code > 599 {
		return nil
	}
	if rule.Probability < 100 && b.rand(100) >= rule.Probability {
		return nil
	}
	metrics.Inc(metrics.ChaosFaultsApplied, stage)
	msg := rule.Message
	if msg == "" {
		msg = fmt.Sprintf("Chaos: simulated %d at %s stage", rule.Code, stage)
	}
	return &smtp.SMTPError{Code: rule.Code, EnhancedCode: smtp.EnhancedCode{rule.Code / 100, 0, 0}, Message: msg}
}

func (s *session) Reset() {
	s.from = ""
	s.to = nil
}

func (s *session) Logout() error { return nil }

// loginServer implements the (non-standard but ubiquitous) AUTH LOGIN mechanism.
type loginServer struct {
	username     string
	step         int
	authenticate func(username, password string) error
}

func (l *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	switch l.step {
	case 0:
		l.step++
		if len(response) > 0 { // initial response carries the username
			l.username = string(response)
			l.step++
			return []byte("Password:"), false, nil
		}
		return []byte("Username:"), false, nil
	case 1:
		l.username = string(response)
		l.step++
		return []byte("Password:"), false, nil
	case 2:
		l.step++
		return nil, true, l.authenticate(l.username, string(response))
	}
	return nil, true, errors.New("unexpected AUTH LOGIN step")
}
