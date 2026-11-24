// Package config loads PostDream configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SMTP authentication modes.
const (
	SMTPAuthOff      = "off"      // AUTH is not advertised; every mail goes to the default inbox.
	SMTPAuthOptional = "optional" // AUTH routes mail to the matching inbox; anonymous mail goes to the default inbox.
	SMTPAuthRequired = "required" // Anonymous mail is rejected.
)

type Config struct {
	DatabaseURL string

	HTTPAddr      string
	CookieSecure  bool
	TrustProxy    bool
	SessionTTL    time.Duration
	AuthEnabled   bool
	AdminEmail    string
	AdminPassword string

	SMTPAddr            string
	SMTPHostname        string
	SMTPAuth            string
	SMTPTLSCert         string
	SMTPTLSKey          string
	SMTPMaxMessageBytes int64
	SMTPMaxRecipients   int
	DefaultInboxSlug    string

	SMTPRequireTLS        bool
	SMTPAllowedRecipients string // global regex every RCPT TO must match ('' = all)
	SMTPMaxConnections    int

	HTTPMaxConnections int
	AllowedHosts       []string // accepted HTTP Host headers ('' = any)
	PublicURL          string   // base URL used in webhook payload links

	RetentionHours      int
	RetentionInterval   time.Duration
	MaxMessagesPerInbox int

	JobWorkers int

	// Outbound requests (webhooks, link check) never reach private networks unless allowed.
	OutboundAllowPrivate bool
	OutboundAllowedHosts []string

	SpamAssassinAddr string // host:port of spamd
	RspamdURL        string // e.g. http://rspamd:11333
	RspamdPassword   string

	RelaySMTPURL string // smtp://user:pass@host:587 (empty = releasing disabled)
	RelayFrom    string // optional envelope sender override for released mail

	POP3Addr string // e.g. :1110 (empty = disabled)

	MetricsAddr  string // separate unauthenticated listener, e.g. :9090
	MetricsToken string // enables /metrics on the HTTP port behind "Authorization: Bearer <token>"
}

func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:         env("DATABASE_URL", "postgres://postdream:postdream@localhost:5432/postdream?sslmode=disable"),
		HTTPAddr:            env("HTTP_ADDR", ":8025"),
		CookieSecure:        envBool("COOKIE_SECURE", false),
		TrustProxy:          envBool("TRUST_PROXY", false),
		SessionTTL:          envDuration("SESSION_TTL", 7*24*time.Hour),
		AuthEnabled:         envBool("AUTH_ENABLED", true),
		AdminEmail:          strings.ToLower(env("ADMIN_EMAIL", "admin@postdream.local")),
		AdminPassword:       env("ADMIN_PASSWORD", ""),
		SMTPAddr:            env("SMTP_ADDR", ":1025"),
		SMTPHostname:        env("SMTP_HOSTNAME", "postdream"),
		SMTPAuth:            strings.ToLower(env("SMTP_AUTH", SMTPAuthOptional)),
		SMTPTLSCert:         env("SMTP_TLS_CERT", ""),
		SMTPTLSKey:          env("SMTP_TLS_KEY", ""),
		SMTPMaxMessageBytes: int64(envInt("SMTP_MAX_MESSAGE_BYTES", 25*1024*1024)),
		SMTPMaxRecipients:   envInt("SMTP_MAX_RECIPIENTS", 100),
		DefaultInboxSlug:    env("DEFAULT_INBOX", "default"),
		RetentionHours:      envInt("RETENTION_HOURS", 72),
		RetentionInterval:   envDuration("RETENTION_INTERVAL", 10*time.Minute),
		MaxMessagesPerInbox: envInt("MAX_MESSAGES_PER_INBOX", 0),

		SMTPRequireTLS:        envBool("SMTP_REQUIRE_TLS", false),
		SMTPAllowedRecipients: env("SMTP_ALLOWED_RECIPIENTS", ""),
		SMTPMaxConnections:    envInt("SMTP_MAX_CONNECTIONS", 1000),
		HTTPMaxConnections:    envInt("HTTP_MAX_CONNECTIONS", 1000),
		AllowedHosts:          envList("ALLOWED_HOSTS"),
		PublicURL:             strings.TrimRight(env("PUBLIC_URL", ""), "/"),
		JobWorkers:            envInt("JOB_WORKERS", 4),
		OutboundAllowPrivate:  envBool("OUTBOUND_ALLOW_PRIVATE", false),
		OutboundAllowedHosts:  envList("OUTBOUND_ALLOWED_HOSTS"),
		SpamAssassinAddr:      env("SPAMASSASSIN_ADDR", ""),
		RspamdURL:             strings.TrimRight(env("RSPAMD_URL", ""), "/"),
		RspamdPassword:        env("RSPAMD_PASSWORD", ""),
		RelaySMTPURL:          env("RELAY_SMTP_URL", ""),
		RelayFrom:             env("RELAY_FROM", ""),
		POP3Addr:              env("POP3_ADDR", ""),
		MetricsAddr:           env("METRICS_ADDR", ""),
		MetricsToken:          env("METRICS_TOKEN", ""),
	}
	switch c.SMTPAuth {
	case SMTPAuthOff, SMTPAuthOptional, SMTPAuthRequired:
	default:
		return nil, fmt.Errorf("SMTP_AUTH must be one of off|optional|required, got %q", c.SMTPAuth)
	}
	if (c.SMTPTLSCert == "") != (c.SMTPTLSKey == "") {
		return nil, fmt.Errorf("SMTP_TLS_CERT and SMTP_TLS_KEY must be set together")
	}
	if c.SMTPAllowedRecipients != "" {
		if _, err := regexp.Compile(c.SMTPAllowedRecipients); err != nil {
			return nil, fmt.Errorf("SMTP_ALLOWED_RECIPIENTS: %w", err)
		}
	}
	if c.SMTPRequireTLS && c.SMTPTLSCert == "" {
		return nil, fmt.Errorf("SMTP_REQUIRE_TLS needs SMTP_TLS_CERT and SMTP_TLS_KEY")
	}
	if c.JobWorkers < 1 {
		c.JobWorkers = 1
	}
	return c, nil
}

// SpamEngine returns the configured spam engine name ("" when none).
func (c *Config) SpamEngine() string {
	switch {
	case c.RspamdURL != "":
		return "rspamd"
	case c.SpamAssassinAddr != "":
		return "spamassassin"
	}
	return ""
}

func envList(key string) []string {
	var out []string
	for _, v := range strings.Split(os.Getenv(key), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, strings.ToLower(v))
		}
	}
	return out
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
