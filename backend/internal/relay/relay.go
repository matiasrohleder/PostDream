// Package relay releases a captured message to a real SMTP server. It is off
// unless RELAY_SMTP_URL is set, and each inbox must additionally whitelist the
// recipients it may release to (a regular expression). Every release is audited.
package relay

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

type Config struct {
	Addr     string // host:port
	Host     string
	Mode     string // starttls | tls | none
	Username string
	Password string
	From     string // envelope sender override
}

// Parse reads smtp://user:pass@host:port?tls=starttls|tls|none (also smtps:// for implicit TLS).
func Parse(raw, from string) (*Config, error) {
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("RELAY_SMTP_URL: %w", err)
	}
	c := &Config{Host: u.Hostname(), From: from}
	port := u.Port()
	switch u.Scheme {
	case "smtp":
		c.Mode = "starttls"
		if port == "" {
			port = "587"
		}
		if port == "465" {
			c.Mode = "tls"
		}
	case "smtps":
		c.Mode = "tls"
		if port == "" {
			port = "465"
		}
	default:
		return nil, fmt.Errorf("RELAY_SMTP_URL: scheme must be smtp or smtps")
	}
	if m := u.Query().Get("tls"); m != "" {
		if m != "starttls" && m != "tls" && m != "none" {
			return nil, fmt.Errorf("RELAY_SMTP_URL: tls must be starttls, tls or none")
		}
		c.Mode = m
	}
	if c.Host == "" {
		return nil, fmt.Errorf("RELAY_SMTP_URL: missing host")
	}
	c.Addr = net.JoinHostPort(c.Host, port)
	if u.User != nil {
		c.Username = u.User.Username()
		c.Password, _ = u.User.Password()
	}
	return c, nil
}

// Redacted returns the configuration without the password, for display.
func (c *Config) Redacted() string {
	if c == nil {
		return ""
	}
	user := ""
	if c.Username != "" {
		user = c.Username + "@"
	}
	return fmt.Sprintf("%s%s (%s)", user, c.Addr, c.Mode)
}

// CheckRecipients validates every recipient against the inbox's pattern.
func CheckRecipients(pattern string, to []string) error {
	if strings.TrimSpace(pattern) == "" {
		return errors.New("releasing is disabled for this inbox (no allowed recipients configured)")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid allowed-recipients pattern: %w", err)
	}
	if len(to) == 0 {
		return errors.New("at least one recipient is required")
	}
	if len(to) > 50 {
		return errors.New("at most 50 recipients")
	}
	for _, r := range to {
		if !strings.Contains(r, "@") || strings.ContainsAny(r, "\r\n<> ") {
			return fmt.Errorf("invalid address %q", r)
		}
		if !re.MatchString(r) {
			return fmt.Errorf("recipient %q is not allowed for this inbox", r)
		}
	}
	return nil
}

// Send delivers raw to the recipients. A trace header is prepended.
func (c *Config) Send(from string, to []string, raw []byte, releasedBy string) error {
	if c.From != "" {
		from = c.From
	}
	tlsCfg := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	var (
		cl  *smtp.Client
		err error
	)
	switch c.Mode {
	case "tls":
		cl, err = smtp.DialTLS(c.Addr, tlsCfg)
	case "starttls":
		cl, err = smtp.DialStartTLS(c.Addr, tlsCfg)
	default:
		cl, err = smtp.Dial(c.Addr)
	}
	if err != nil {
		return fmt.Errorf("connect relay: %w", err)
	}
	defer cl.Close()
	cl.CommandTimeout = 30 * time.Second
	cl.SubmissionTimeout = 2 * time.Minute
	if c.Username != "" {
		if err := cl.Auth(sasl.NewPlainClient("", c.Username, c.Password)); err != nil {
			return fmt.Errorf("relay auth: %w", err)
		}
	}
	header := fmt.Sprintf("X-PostDream-Released-By: %s\r\nX-PostDream-Released-At: %s\r\n",
		sanitize(releasedBy), time.Now().UTC().Format(time.RFC1123Z))
	body := append([]byte(header), raw...)
	if err := cl.SendMail(from, to, bytes.NewReader(body)); err != nil {
		return fmt.Errorf("relay send: %w", err)
	}
	return cl.Quit()
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return -1
		}
		return r
	}, s)
}
