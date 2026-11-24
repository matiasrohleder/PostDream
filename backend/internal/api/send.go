package api

import (
	"bytes"
	"encoding/base64"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/jhillyerd/enmime"

	"github.com/matiasrohleder/PostDream/internal/ingest"
	"github.com/matiasrohleder/PostDream/internal/store"
)

type sendAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`    // base64
	ContentID   string `json:"content_id"` // set for inline (cid:) parts
}

type sendBody struct {
	Inbox       string            `json:"inbox"` // id or slug; optional with an API token
	From        string            `json:"from"`
	To          []string          `json:"to"`
	Cc          []string          `json:"cc"`
	Bcc         []string          `json:"bcc"`
	ReplyTo     string            `json:"reply_to"`
	Subject     string            `json:"subject"`
	Text        string            `json:"text"`
	HTML        string            `json:"html"`
	Headers     map[string]string `json:"headers"`
	Tags        []string          `json:"tags"`
	Attachments []sendAttachment  `json:"attachments"`
	Raw         string            `json:"raw"` // a complete RFC 5322 message; overrides the fields above
}

// send injects a message into an inbox without SMTP. It goes through the same
// pipeline as SMTP mail (tags, rules, webhooks, spam analysis, live updates).
// Nothing is relayed.
func (s *Server) send(w http.ResponseWriter, r *http.Request) {
	p := from(r)
	var b sendBody
	limit := s.cfg.SMTPMaxMessageBytes*4/3 + 64*1024 // base64 overhead
	if err := readJSONLimit(r, &b, limit); err != nil {
		writeErr(w, 400, "invalid body (or larger than the max message size)")
		return
	}
	inbox, err := s.resolveSendInbox(r, p, b.Inbox)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var raw []byte
	var envFrom string
	var envTo []string
	if b.Raw != "" {
		raw = []byte(b.Raw)
		msg, err := mail.ReadMessage(bytes.NewReader(raw))
		if err != nil {
			writeErr(w, 400, "raw is not a valid RFC 5322 message: "+err.Error())
			return
		}
		envFrom = firstAddress(msg.Header.Get("From"))
		envTo = addressesOf(msg.Header.Get("To"), msg.Header.Get("Cc"), msg.Header.Get("Bcc"))
		envTo = append(envTo, b.Bcc...)
		if b.From != "" {
			envFrom = b.From
		}
		if len(b.To) > 0 {
			envTo = b.To
		}
	} else {
		raw, envFrom, envTo, err = buildMessage(&b)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	if int64(len(raw)) > s.cfg.SMTPMaxMessageBytes {
		writeErr(w, 413, "message larger than the max message size")
		return
	}
	user := ""
	if p.Inbox != nil {
		user = "api-token"
	} else if p.User != nil {
		user = p.User.Email
	}
	summary, err := s.Pipeline.Ingest(r.Context(), ingest.Envelope{
		InboxID: inbox.ID, From: envFrom, To: envTo, ClientIP: s.clientIP(r), SMTPUser: user, Source: "api",
	}, raw)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 201, summary)
}

func (s *Server) resolveSendInbox(r *http.Request, p *principal, ref string) (*store.Inbox, error) {
	if ref == "" {
		if p.Inbox != nil {
			return p.Inbox, nil
		}
		ref = s.cfg.DefaultInboxSlug
	}
	in, err := s.store.GetInboxLite(r.Context(), ref)
	if err != nil {
		in, err = s.store.GetInboxBySlug(r.Context(), ref)
	}
	if err != nil || !p.canAccess(in.ID) {
		return nil, errors.New("inbox not found or not accessible")
	}
	return in, nil
}

func parseAddrs(list []string) ([]mail.Address, []string, error) {
	var out []mail.Address
	var plain []string
	for _, v := range list {
		a, err := mail.ParseAddress(v)
		if err != nil {
			return nil, nil, errors.New("invalid address: " + v)
		}
		out = append(out, *a)
		plain = append(plain, a.Address)
	}
	return out, plain, nil
}

func buildMessage(b *sendBody) ([]byte, string, []string, error) {
	if b.From == "" {
		return nil, "", nil, errors.New("from is required")
	}
	fromAddr, err := mail.ParseAddress(b.From)
	if err != nil {
		return nil, "", nil, errors.New("invalid from address")
	}
	to, toPlain, err := parseAddrs(b.To)
	if err != nil {
		return nil, "", nil, err
	}
	cc, ccPlain, err := parseAddrs(b.Cc)
	if err != nil {
		return nil, "", nil, err
	}
	bcc, bccPlain, err := parseAddrs(b.Bcc)
	if err != nil {
		return nil, "", nil, err
	}
	if len(to)+len(cc)+len(bcc) == 0 {
		return nil, "", nil, errors.New("at least one recipient (to, cc or bcc) is required")
	}
	mb := enmime.Builder().From(fromAddr.Name, fromAddr.Address).Subject(b.Subject).
		ToAddrs(to).CCAddrs(cc).BCCAddrs(bcc).
		Header("Message-ID", "<"+store.RandomToken(12)+"@postdream.send>")
	if b.ReplyTo != "" {
		rt, err := mail.ParseAddress(b.ReplyTo)
		if err != nil {
			return nil, "", nil, errors.New("invalid reply_to address")
		}
		mb = mb.ReplyTo(rt.Name, rt.Address)
	}
	if b.Text != "" || b.HTML == "" {
		mb = mb.Text([]byte(b.Text))
	}
	if b.HTML != "" {
		mb = mb.HTML([]byte(b.HTML))
	}
	for k, v := range b.Headers {
		if strings.ContainsAny(k+v, "\r\n") || k == "" {
			return nil, "", nil, errors.New("invalid header " + k)
		}
		mb = mb.Header(k, v)
	}
	if tags := store.NormalizeTags(b.Tags); len(tags) > 0 {
		mb = mb.Header("X-Tags", strings.Join(tags, ", "))
	}
	for _, a := range b.Attachments {
		data, err := base64.StdEncoding.DecodeString(a.Content)
		if err != nil {
			return nil, "", nil, errors.New("attachment " + a.Filename + ": content must be base64")
		}
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		if a.ContentID != "" {
			mb = mb.AddInline(data, ct, a.Filename, a.ContentID)
		} else {
			mb = mb.AddAttachment(data, ct, a.Filename)
		}
	}
	part, err := mb.Build()
	if err != nil {
		return nil, "", nil, err
	}
	var buf bytes.Buffer
	if err := part.Encode(&buf); err != nil {
		return nil, "", nil, err
	}
	rcpts := append(append(toPlain, ccPlain...), bccPlain...)
	return buf.Bytes(), fromAddr.Address, rcpts, nil
}

func firstAddress(h string) string {
	if a, err := mail.ParseAddress(h); err == nil {
		return a.Address
	}
	return strings.TrimSpace(h)
}

func addressesOf(headers ...string) []string {
	var out []string
	for _, h := range headers {
		if h == "" {
			continue
		}
		if list, err := mail.ParseAddressList(h); err == nil {
			for _, a := range list {
				out = append(out, a.Address)
			}
		}
	}
	return out
}
