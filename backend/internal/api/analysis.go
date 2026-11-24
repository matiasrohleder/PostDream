package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/matiasrohleder/PostDream/internal/deliverability"
	"github.com/matiasrohleder/PostDream/internal/events"
	"github.com/matiasrohleder/PostDream/internal/htmlcheck"
	"github.com/matiasrohleder/PostDream/internal/jobs"
	"github.com/matiasrohleder/PostDream/internal/metrics"
	"github.com/matiasrohleder/PostDream/internal/relay"
	"github.com/matiasrohleder/PostDream/internal/store"
)

func (s *Server) htmlCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorizeMessage(w, r, id) {
		return
	}
	html, _, err := s.store.GetMessageHTML(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	m, err := s.store.GetMessage(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, htmlcheck.Check(html, strings.TrimSpace(m.TextBody) != ""))
}

func (s *Server) linkCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorizeMessage(w, r, id) {
		return
	}
	p := from(r)
	if s.checkLimiter.Blocked(p.key()) {
		writeErr(w, 429, "too many link checks, try again in a few minutes")
		return
	}
	s.checkLimiter.Fail(p.key()) // counts every check, successful or not
	m, err := s.store.GetMessage(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	html, _, err := s.store.GetMessageHTML(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	inbox, err := s.store.GetInboxLite(r.Context(), m.InboxID)
	if err != nil {
		storeErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	writeJSON(w, 200, s.Links.Check(ctx, html, m.TextBody, inbox.AllowedHosts, inbox.LinkDomains))
}

func (s *Server) deliverability(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorizeMessage(w, r, id) {
		return
	}
	m, err := s.store.GetMessage(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	raw, _, err := s.store.GetMessageRaw(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, deliverability.Run(r.Context(), deliverability.Input{
		Raw: raw, EnvelopeFrom: m.EnvelopeFrom, HasText: strings.TrimSpace(m.TextBody) != "", HasHTML: m.HasHTML,
		ReceivedAt: m.ReceivedAt,
	}, deliverability.Options{}))
}

// spamCheck returns the stored analysis, or runs one now (?refresh=1 forces it).
func (s *Server) spamCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorizeMessage(w, r, id) {
		return
	}
	if s.Spam == nil {
		writeErr(w, 501, "no spam engine configured (set RSPAMD_URL or SPAMASSASSIN_ADDR)")
		return
	}
	if r.URL.Query().Get("refresh") == "" {
		m, err := s.store.GetMessage(r.Context(), id)
		if err != nil {
			storeErr(w, err)
			return
		}
		if len(m.SpamReport) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.Write(m.SpamReport)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	rep, err := s.runSpam(ctx, id)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, rep)
}

func (s *Server) runSpam(ctx context.Context, id string) (any, error) {
	raw, _, err := s.store.GetMessageRaw(ctx, id)
	if err != nil {
		return nil, err
	}
	rep, err := s.Spam.Check(ctx, raw)
	if err != nil {
		metrics.Inc(metrics.SpamChecks, s.Spam.Engine(), "error")
		return nil, err
	}
	metrics.Inc(metrics.SpamChecks, s.Spam.Engine(), "ok")
	if err := s.store.SetSpamResult(ctx, id, rep.Score, rep); err != nil {
		return nil, err
	}
	inbox, _ := s.store.MessageInbox(ctx, id)
	s.hub.Publish(events.Event{Type: "messages.changed", InboxID: inbox})
	return rep, nil
}

// SpamJob is the background handler for spam.check jobs queued at ingestion.
func (s *Server) SpamJob(ctx context.Context, payload json.RawMessage, _ int) error {
	var p struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return jobs.Permanent{Err: err}
	}
	if s.Spam == nil {
		return nil
	}
	_, err := s.runSpam(ctx, p.MessageID)
	if errors.Is(err, store.ErrNotFound) {
		return nil // message deleted meanwhile
	}
	return err
}

// release sends a captured message to real recipients through RELAY_SMTP_URL.
func (s *Server) release(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorizeMessage(w, r, id) {
		return
	}
	if s.Relay == nil {
		writeErr(w, 501, "releasing is disabled (set RELAY_SMTP_URL)")
		return
	}
	var b struct {
		To []string `json:"to"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	for i := range b.To {
		b.To[i] = strings.TrimSpace(b.To[i])
	}
	m, err := s.store.GetMessage(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	inbox, err := s.store.GetInboxLite(r.Context(), m.InboxID)
	if err != nil {
		storeErr(w, err)
		return
	}
	if err := relay.CheckRecipients(inbox.RelayRecipients, b.To); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	raw, _, err := s.store.GetMessageRaw(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	who := from(r).label()
	sendErr := s.Relay.Send(m.EnvelopeFrom, b.To, raw, who)
	entry := &store.RelayEntry{MessageID: id, InboxID: &inbox.ID, UserEmail: who, Recipients: b.To, Subject: m.Subject, OK: sendErr == nil}
	if sendErr != nil {
		entry.Error = sendErr.Error()
		metrics.Inc(metrics.MessagesReleased, "failure")
	} else {
		metrics.Inc(metrics.MessagesReleased, "success")
	}
	if err := s.store.LogRelay(r.Context(), entry); err != nil {
		slog.Error("relay audit log failed", "err", err)
	}
	slog.Info("message released", "id", id, "by", who, "to", b.To, "ok", sendErr == nil)
	if sendErr != nil {
		writeErr(w, 502, sendErr.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "recipients": b.To})
}

func (s *Server) relayLog(w http.ResponseWriter, r *http.Request) {
	var inboxes []string
	if id := r.URL.Query().Get("inbox"); id != "" {
		inboxes = []string{id}
	}
	list, err := s.store.ListRelayLog(r.Context(), inboxes, 100)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}
