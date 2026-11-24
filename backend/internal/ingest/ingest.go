// Package ingest is the single path every captured message goes through, whether
// it arrived over SMTP or via the HTTP send API: parse, auto-tag, store, apply
// tag rules, queue webhooks/spam analysis and notify live clients.
package ingest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/matiasrohleder/PostDream/internal/events"
	"github.com/matiasrohleder/PostDream/internal/jobs"
	"github.com/matiasrohleder/PostDream/internal/mailparse"
	"github.com/matiasrohleder/PostDream/internal/metrics"
	"github.com/matiasrohleder/PostDream/internal/spam"
	"github.com/matiasrohleder/PostDream/internal/store"
	"github.com/matiasrohleder/PostDream/internal/webhook"
)

type Envelope struct {
	InboxID  string
	From     string
	To       []string
	ClientIP string
	SMTPUser string
	Source   string // "smtp" | "api"
}

type Pipeline struct {
	Store       *store.Store
	Hub         *events.Hub
	Jobs        *jobs.Runner
	SpamEnabled bool   // a spam engine is configured
	PublicURL   string // for links in webhook payloads
}

// Ingest stores raw as a new message of env.InboxID and triggers follow-up work.
func (p *Pipeline) Ingest(ctx context.Context, env Envelope, raw []byte) (*store.MessageSummary, error) {
	parsed := mailparse.Parse(raw)
	msg := &store.NewMessage{
		InboxID: env.InboxID, MessageID: parsed.MessageID, EnvelopeFrom: env.From, EnvelopeTo: env.To,
		From: parsed.From, To: parsed.To, Cc: parsed.Cc, Subject: parsed.Subject, TextBody: parsed.Text, HTMLBody: parsed.HTML,
		Headers: parsed.Headers, Raw: raw, ClientIP: env.ClientIP, SMTPUser: env.SMTPUser,
		Tags: AutoTags(parsed, env.To),
	}
	if msg.EnvelopeTo == nil {
		msg.EnvelopeTo = []string{}
	}
	if msg.From == "" {
		msg.From = env.From
	}
	if msg.To == "" && len(env.To) > 0 {
		msg.To = strings.Join(env.To, ", ")
	}
	for _, a := range parsed.Attachments {
		msg.Attachments = append(msg.Attachments, &store.Attachment{
			Filename: a.Filename, ContentType: a.ContentType, ContentID: a.ContentID, Inline: a.Inline, Data: a.Data,
		})
	}
	summary, err := p.Store.InsertMessage(ctx, msg)
	if err != nil {
		return nil, err
	}

	// Rules run after insert so they can use the full search syntax against the stored row.
	if tags, err := p.Store.ApplyTagRules(ctx, summary.ID, env.InboxID); err != nil {
		slog.Warn("tag rules failed", "id", summary.ID, "err", err)
	} else {
		summary.Tags = tags
	}

	inbox, err := p.Store.GetInboxLite(ctx, env.InboxID)
	if err == nil {
		summary.InboxName = inbox.Name
		metrics.Inc(metrics.MessagesReceived, inbox.Slug)
		metrics.Add(metrics.BytesReceived, float64(len(raw)), inbox.Slug)
		p.queueWebhooks(ctx, inbox, summary)
		if p.SpamEnabled && inbox.SpamCheck {
			if err := p.Jobs.Enqueue(ctx, spam.JobKind, map[string]string{"message_id": summary.ID}, 3); err != nil {
				slog.Warn("enqueue spam check failed", "err", err)
			}
		}
	}
	slog.Info("captured message", "id", summary.ID, "inbox", env.InboxID, "source", env.Source, "from", env.From,
		"to", env.To, "bytes", len(raw))
	p.Hub.Publish(events.Event{Type: "message.new", InboxID: env.InboxID, Data: summary})
	return summary, nil
}

// MessagePayload is the JSON body of a message.received webhook.
type MessagePayload struct {
	Event      string                `json:"event"`
	DeliveryID string                `json:"delivery_id"`
	CreatedAt  time.Time             `json:"created_at"`
	Inbox      InboxRef              `json:"inbox"`
	Message    *store.MessageSummary `json:"message"`
	Links      map[string]string     `json:"links,omitempty"`
}

type InboxRef struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func (p *Pipeline) queueWebhooks(ctx context.Context, inbox *store.Inbox, summary *store.MessageSummary) {
	hooks, err := p.Store.ActiveWebhooks(ctx, inbox.ID, webhook.EventReceived)
	if err != nil {
		slog.Warn("list webhooks failed", "err", err)
		return
	}
	for _, h := range hooks {
		payload := MessagePayload{
			Event: webhook.EventReceived, DeliveryID: "dlv_" + store.RandomToken(12), CreatedAt: time.Now().UTC(),
			Inbox: InboxRef{ID: inbox.ID, Slug: inbox.Slug, Name: inbox.Name}, Message: summary,
		}
		if p.PublicURL != "" {
			payload.Links = map[string]string{
				"api": p.PublicURL + "/api/messages/" + summary.ID,
				"raw": p.PublicURL + "/api/messages/" + summary.ID + "/raw",
				"ui":  p.PublicURL + "/inbox/" + inbox.ID + "?m=" + summary.ID,
			}
		}
		body, _ := json.Marshal(payload)
		job := webhook.JobPayload{WebhookID: h.ID, Event: webhook.EventReceived, DeliveryID: payload.DeliveryID, Body: body}
		if err := p.Jobs.Enqueue(ctx, webhook.JobKind, job, webhook.MaxAttempts); err != nil {
			slog.Warn("enqueue webhook failed", "webhook", h.ID, "err", err)
		}
	}
}

// AutoTags derives tags from plus-addressing (qa+signup@x -> "signup") in the
// envelope and To/Cc headers, and from an X-Tags header (comma separated).
func AutoTags(p *mailparse.Parsed, envelopeTo []string) []string {
	var tags []string
	addrs := append([]string{}, envelopeTo...)
	for _, h := range []string{p.To, p.Cc} {
		if h == "" {
			continue
		}
		if list, err := mail.ParseAddressList(h); err == nil {
			for _, a := range list {
				addrs = append(addrs, a.Address)
			}
		}
	}
	for _, a := range addrs {
		local, _, ok := strings.Cut(strings.Trim(a, "<> "), "@")
		if !ok {
			continue
		}
		if _, tag, ok := strings.Cut(local, "+"); ok && tag != "" {
			tags = append(tags, tag)
		}
	}
	for _, k := range []string{"X-Tags", "X-PostDream-Tags"} {
		for _, v := range p.Headers[k] {
			tags = append(tags, strings.Split(v, ",")...)
		}
	}
	return store.NormalizeTags(tags)
}
