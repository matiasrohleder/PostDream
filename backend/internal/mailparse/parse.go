// Package mailparse turns a raw RFC 5322 message into the fields PostDream stores.
package mailparse

import (
	"bytes"
	"strings"

	"github.com/jhillyerd/enmime"
)

type Parsed struct {
	MessageID   string
	From        string
	To          string
	Cc          string
	Subject     string
	Text        string
	HTML        string
	Headers     map[string][]string
	Attachments []Part
}

type Part struct {
	Filename    string
	ContentType string
	ContentID   string
	Inline      bool
	Data        []byte
}

// clean makes a string safe to store in a Postgres TEXT column.
func clean(s string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(s, "�"), "\x00", "")
}

// Parse never fails hard: when the MIME structure is broken it falls back to
// treating the whole message as plain text so the mail is still captured.
func Parse(raw []byte) *Parsed {
	p := &Parsed{Headers: map[string][]string{}}
	env, err := enmime.ReadEnvelope(bytes.NewReader(raw))
	if err != nil || env == nil {
		p.Text = clean(string(raw))
		p.Subject = "(unparseable message)"
		return p
	}
	p.MessageID = clean(strings.Trim(env.GetHeader("Message-Id"), "<> "))
	p.From = clean(env.GetHeader("From"))
	p.To = clean(env.GetHeader("To"))
	p.Cc = clean(env.GetHeader("Cc"))
	p.Subject = clean(env.GetHeader("Subject"))
	p.Text = clean(env.Text)
	p.HTML = clean(env.HTML)
	if env.Root != nil {
		for k, vs := range env.Root.Header {
			for _, v := range vs {
				p.Headers[k] = append(p.Headers[k], clean(v))
			}
		}
	}
	add := func(parts []*enmime.Part, inline bool) {
		for _, part := range parts {
			name := part.FileName
			if name == "" {
				name = "unnamed"
			}
			p.Attachments = append(p.Attachments, Part{
				Filename:    clean(name),
				ContentType: clean(part.ContentType),
				ContentID:   clean(strings.Trim(part.ContentID, "<> ")),
				Inline:      inline,
				Data:        part.Content,
			})
		}
	}
	add(env.Attachments, false)
	add(env.Inlines, true)
	add(env.OtherParts, true)
	return p
}
