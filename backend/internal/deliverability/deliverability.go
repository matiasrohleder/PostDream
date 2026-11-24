// Package deliverability runs offline sanity checks that affect inbox placement:
// DKIM signatures, required headers, one-click unsubscribe (RFC 8058), sender
// alignment and MIME structure. It needs no external spam engine.
package deliverability

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/mail"
	"strings"
	"time"

	"github.com/emersion/go-msgauth/dkim"
)

type Status string

const (
	Pass Status = "pass"
	Warn Status = "warn"
	Fail Status = "fail"
	Info Status = "info"
)

type Check struct {
	ID     string `json:"id"`
	Status Status `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

type Report struct {
	Checks []Check `json:"checks"`
	Pass   int     `json:"pass"`
	Warn   int     `json:"warn"`
	Fail   int     `json:"fail"`
}

// Input is what the checks need about a stored message.
type Input struct {
	Raw          []byte
	EnvelopeFrom string
	HasText      bool
	HasHTML      bool
	ReceivedAt   time.Time
}

// Options tune external lookups (DKIM keys are fetched over DNS).
type Options struct {
	LookupTXT func(ctx context.Context, domain string) ([]string, error)
	Timeout   time.Duration
}

func Run(ctx context.Context, in Input, opt Options) *Report {
	rep := &Report{Checks: []Check{}}
	add := func(c Check) {
		rep.Checks = append(rep.Checks, c)
		switch c.Status {
		case Pass:
			rep.Pass++
		case Warn:
			rep.Warn++
		case Fail:
			rep.Fail++
		}
	}
	msg, err := mail.ReadMessage(bytes.NewReader(in.Raw))
	if err != nil {
		add(Check{ID: "parse", Status: Fail, Title: "Message headers could not be parsed", Detail: err.Error()})
		return rep
	}
	h := msg.Header

	add(checkDKIM(ctx, in.Raw, opt))

	// From
	fromDomain := ""
	if v := h.Get("From"); v == "" {
		add(Check{ID: "from", Status: Fail, Title: "Missing From header"})
	} else if list, err := mail.ParseAddressList(v); err != nil || len(list) == 0 {
		add(Check{ID: "from", Status: Fail, Title: "From header is not a valid address", Detail: v})
	} else {
		fromDomain = domainOf(list[0].Address)
		if len(list) > 1 && h.Get("Sender") == "" {
			add(Check{ID: "from", Status: Warn, Title: "Several From addresses without a Sender header"})
		} else {
			add(Check{ID: "from", Status: Pass, Title: "From header is valid", Detail: list[0].Address})
		}
	}

	// Envelope / From alignment (what SPF-based DMARC alignment looks at).
	if env := domainOf(in.EnvelopeFrom); env != "" && fromDomain != "" {
		if aligned(env, fromDomain) {
			add(Check{ID: "alignment", Status: Pass, Title: "Envelope sender is aligned with the From domain", Detail: env})
		} else {
			add(Check{ID: "alignment", Status: Warn, Title: "Envelope sender domain differs from the From domain",
				Detail: fmt.Sprintf("MAIL FROM %s vs From %s: SPF cannot align for DMARC unless DKIM does.", env, fromDomain)})
		}
	}

	// Message-ID
	if v := strings.TrimSpace(h.Get("Message-Id")); v == "" {
		add(Check{ID: "message-id", Status: Warn, Title: "Missing Message-ID header", Detail: "Some providers treat messages without it as suspicious."})
	} else if !strings.HasPrefix(v, "<") || !strings.HasSuffix(v, ">") || !strings.Contains(v, "@") {
		add(Check{ID: "message-id", Status: Warn, Title: "Message-ID is not in the <id@domain> form", Detail: v})
	} else {
		add(Check{ID: "message-id", Status: Pass, Title: "Message-ID is present", Detail: v})
	}

	// Date
	if v := h.Get("Date"); v == "" {
		add(Check{ID: "date", Status: Warn, Title: "Missing Date header"})
	} else if d, err := mail.ParseDate(v); err != nil {
		add(Check{ID: "date", Status: Warn, Title: "Date header cannot be parsed", Detail: v})
	} else if ref := in.ReceivedAt; !ref.IsZero() && d.After(ref.Add(15*time.Minute)) {
		add(Check{ID: "date", Status: Warn, Title: "Date is in the future", Detail: v})
	} else {
		add(Check{ID: "date", Status: Pass, Title: "Date header is valid", Detail: v})
	}

	// Subject
	if v := strings.TrimSpace(h.Get("Subject")); v == "" {
		add(Check{ID: "subject", Status: Warn, Title: "Empty or missing Subject"})
	} else {
		add(Check{ID: "subject", Status: Pass, Title: "Subject is present"})
	}

	// To
	if h.Get("To") == "" && h.Get("Cc") == "" {
		add(Check{ID: "to", Status: Info, Title: "No To/Cc header", Detail: "Recipients only appear in the envelope (undisclosed recipients)."})
	}

	// Reply-To
	if v := h.Get("Reply-To"); v != "" {
		if _, err := mail.ParseAddressList(v); err != nil {
			add(Check{ID: "reply-to", Status: Warn, Title: "Reply-To is not a valid address list", Detail: v})
		}
	}

	// MIME
	ct := strings.ToLower(h.Get("Content-Type"))
	if (strings.HasPrefix(ct, "multipart/") || strings.Contains(ct, "html")) && h.Get("Mime-Version") == "" {
		add(Check{ID: "mime-version", Status: Warn, Title: "MIME content without a MIME-Version header"})
	}
	if in.HasHTML && !in.HasText {
		add(Check{ID: "text-part", Status: Warn, Title: "HTML-only message", Detail: "Add a text/plain alternative: HTML-only mail scores worse with spam filters."})
	} else if in.HasHTML {
		add(Check{ID: "text-part", Status: Pass, Title: "Has both HTML and plain-text parts"})
	}

	// List-Unsubscribe (required by Gmail and Yahoo for bulk senders since 2024).
	lu := h.Get("List-Unsubscribe")
	switch {
	case lu == "":
		add(Check{ID: "list-unsubscribe", Status: Info, Title: "No List-Unsubscribe header",
			Detail: "Required by Gmail and Yahoo for bulk/marketing senders; not needed for transactional mail."})
	case strings.Contains(strings.ToLower(lu), "https://"):
		if strings.EqualFold(strings.TrimSpace(h.Get("List-Unsubscribe-Post")), "List-Unsubscribe=One-Click") {
			add(Check{ID: "list-unsubscribe", Status: Pass, Title: "One-click unsubscribe (RFC 8058) is configured", Detail: lu})
		} else {
			add(Check{ID: "list-unsubscribe", Status: Warn, Title: "List-Unsubscribe without List-Unsubscribe-Post: List-Unsubscribe=One-Click",
				Detail: "Gmail and Yahoo require RFC 8058 one-click unsubscribe for bulk senders."})
		}
	default:
		add(Check{ID: "list-unsubscribe", Status: Pass, Title: "List-Unsubscribe is present (mailto only)", Detail: lu})
	}

	// Raw 8-bit bytes in headers without SMTPUTF8 usually break in some clients.
	end := bytes.Index(in.Raw, []byte("\r\n\r\n"))
	if end < 0 {
		end = bytes.Index(in.Raw, []byte("\n\n"))
	}
	if end > 0 && bytes.ContainsFunc(in.Raw[:end], func(r rune) bool { return r >= 0x80 }) {
		add(Check{ID: "header-encoding", Status: Warn, Title: "Headers contain raw non-ASCII bytes",
			Detail: "Encode non-ASCII header text with RFC 2047 (=?UTF-8?...?=) unless SMTPUTF8 is used end to end."})
	}
	return rep
}

func checkDKIM(ctx context.Context, raw []byte, opt Options) Check {
	if !bytes.Contains(bytes.ToLower(raw[:min(len(raw), 64*1024)]), []byte("dkim-signature:")) {
		return Check{ID: "dkim", Status: Info, Title: "Not DKIM-signed", Detail: "Sandbox mail is often unsigned; production mail should be signed by your provider."}
	}
	timeout := opt.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	lookup := opt.LookupTXT
	if lookup == nil {
		lookup = net.DefaultResolver.LookupTXT
	}
	vopts := &dkim.VerifyOptions{
		MaxVerifications: 5,
		LookupTXT: func(domain string) ([]string, error) {
			c, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return lookup(c, domain)
		},
	}
	verifs, err := dkim.VerifyWithOptions(bytes.NewReader(raw), vopts)
	if err != nil {
		return Check{ID: "dkim", Status: Fail, Title: "DKIM verification error", Detail: err.Error()}
	}
	var ok, bad []string
	for _, v := range verifs {
		if v.Err == nil {
			ok = append(ok, v.Domain)
		} else {
			bad = append(bad, v.Domain+": "+v.Err.Error())
		}
	}
	switch {
	case len(ok) > 0 && len(bad) == 0:
		return Check{ID: "dkim", Status: Pass, Title: "Valid DKIM signature", Detail: strings.Join(ok, ", ")}
	case len(ok) > 0:
		return Check{ID: "dkim", Status: Warn, Title: "Some DKIM signatures failed", Detail: strings.Join(append(ok, bad...), "; ")}
	default:
		return Check{ID: "dkim", Status: Fail, Title: "DKIM signature does not verify", Detail: strings.Join(bad, "; ")}
	}
}

func domainOf(addr string) string {
	addr = strings.Trim(strings.TrimSpace(addr), "<>")
	if i := strings.LastIndexByte(addr, '@'); i >= 0 {
		return strings.ToLower(addr[i+1:])
	}
	return ""
}

// aligned implements relaxed DMARC alignment approximately: same domain or one
// is a subdomain of the other.
func aligned(a, b string) bool {
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}
