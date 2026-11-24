package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/matiasrohleder/PostDream/internal/events"
	"github.com/matiasrohleder/PostDream/internal/store"
)

// parseFilter builds a MessageFilter from query params, scoped to what the caller may see.
func parseFilter(r *http.Request, p *principal) (store.MessageFilter, error) {
	q := r.URL.Query()
	f := store.MessageFilter{
		Q: q.Get("q"), From: q.Get("from"), To: q.Get("to"), Subject: q.Get("subject"),
		Sort: q.Get("sort"), Order: q.Get("order"),
	}
	terms, err := store.ParseSearch(f.Q)
	if err != nil {
		return f, fmt.Errorf("invalid search: %v", err)
	}
	if tag := strings.TrimSpace(q.Get("tag")); tag != "" {
		terms = append(terms, store.Term{Field: "tag", Value: tag})
	}
	f.Terms = terms
	f.Page, _ = strconv.Atoi(q.Get("page"))
	f.PageSize, _ = strconv.Atoi(q.Get("page_size"))
	if inbox := q.Get("inbox"); inbox != "" {
		if !p.canAccess(inbox) {
			return f, errors.New("inbox not accessible")
		}
		f.InboxIDs = []string{inbox}
	} else {
		f.InboxIDs = p.allowedInboxes()
	}
	for name, dst := range map[string]**time.Time{"since": &f.Since, "until": &f.Until} {
		if v := q.Get(name); v != "" {
			t, err := parseTime(v, name == "until")
			if err != nil {
				return f, fmt.Errorf("invalid %s: %v", name, err)
			}
			*dst = &t
		}
	}
	if v := q.Get("has_attachments"); v != "" {
		b, _ := strconv.ParseBool(v)
		f.HasAttachments = &b
	}
	if v := q.Get("unread"); v != "" {
		b, _ := strconv.ParseBool(v)
		f.Unread = &b
	}
	return f, nil
}

// parseTime accepts RFC3339, "YYYY-MM-DDTHH:MM" (datetime-local) or "YYYY-MM-DD".
func parseTime(v string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", v, time.UTC); err == nil {
		return t, nil
	}
	t, err := time.ParseInLocation("2006-01-02", v, time.UTC)
	if err == nil && endOfDay {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	return t, err
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r, from(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	page, err := s.store.ListMessages(r.Context(), f)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, page)
}

// latestMessage returns the newest message matching the filters, optionally
// long-polling (?wait=30s) until one arrives. Handy for end-to-end tests.
func (s *Server) latestMessage(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r, from(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	f.Page, f.PageSize, f.Sort, f.Order = 1, 1, "received_at", "desc"
	wait, _ := time.ParseDuration(r.URL.Query().Get("wait"))
	if wait > 2*time.Minute {
		wait = 2 * time.Minute
	}
	deadline := time.Now().Add(wait)
	for {
		page, err := s.store.ListMessages(r.Context(), f)
		if err != nil {
			storeErr(w, err)
			return
		}
		if len(page.Items) > 0 {
			m, err := s.store.GetMessage(r.Context(), page.Items[0].ID)
			if err != nil {
				storeErr(w, err)
				return
			}
			writeJSON(w, 200, m)
			return
		}
		if time.Now().After(deadline) {
			writeErr(w, 404, "no matching message")
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// authorizeMessage loads the message's inbox and checks access. It writes the error response itself.
func (s *Server) authorizeMessage(w http.ResponseWriter, r *http.Request, id string) bool {
	inbox, err := s.store.MessageInbox(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return false
	}
	if !from(r).canAccess(inbox) {
		writeErr(w, 404, "not found")
		return false
	}
	return true
}

func (s *Server) getMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorizeMessage(w, r, id) {
		return
	}
	m, err := s.store.GetMessage(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	if !m.IsRead && r.URL.Query().Get("peek") == "" {
		s.store.SetRead(r.Context(), []string{id}, true)
		m.IsRead = true
		s.hub.Publish(events.Event{Type: "messages.changed", InboxID: m.InboxID})
	}
	writeJSON(w, 200, m)
}

func (s *Server) deleteMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorizeMessage(w, r, id) {
		return
	}
	if _, err := s.store.DeleteMessages(r.Context(), []string{id}); err != nil {
		storeErr(w, err)
		return
	}
	s.hub.Publish(events.Event{Type: "messages.changed"})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type idsBody struct {
	IDs  []string `json:"ids"`
	Read *bool    `json:"read"`
}

// filterAccessible drops ids the caller is not allowed to touch.
func (s *Server) filterAccessible(r *http.Request, ids []string) []string {
	p := from(r)
	if p.allowedInboxes() == nil {
		return ids
	}
	out := []string{}
	for _, id := range ids {
		if inbox, err := s.store.MessageInbox(r.Context(), id); err == nil && p.canAccess(inbox) {
			out = append(out, id)
		}
	}
	return out
}

func (s *Server) bulkDelete(w http.ResponseWriter, r *http.Request) {
	var b idsBody
	if err := readJSON(r, &b); err != nil || len(b.IDs) == 0 || len(b.IDs) > 1000 {
		writeErr(w, 400, "ids (1..1000) required")
		return
	}
	n, err := s.store.DeleteMessages(r.Context(), s.filterAccessible(r, b.IDs))
	if err != nil {
		storeErr(w, err)
		return
	}
	s.hub.Publish(events.Event{Type: "messages.changed"})
	writeJSON(w, 200, map[string]int64{"deleted": n})
}

// markRead marks the given ids (or, with no ids, everything matching the query filters).
func (s *Server) markRead(w http.ResponseWriter, r *http.Request) {
	var b idsBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	read := b.Read == nil || *b.Read
	var n int64
	var err error
	if len(b.IDs) > 0 {
		n, err = s.store.SetRead(r.Context(), s.filterAccessible(r, b.IDs), read)
	} else {
		f, ferr := parseFilter(r, from(r))
		if ferr != nil {
			writeErr(w, 400, ferr.Error())
			return
		}
		n, err = s.store.MarkAllRead(r.Context(), f)
	}
	if err != nil {
		storeErr(w, err)
		return
	}
	s.hub.Publish(events.Event{Type: "messages.changed"})
	writeJSON(w, 200, map[string]int64{"updated": n})
}

// deleteAllMessages deletes every message matching the query filters (e.g. ?inbox=<id>).
func (s *Server) deleteAllMessages(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r, from(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	n, err := s.store.DeleteMatching(r.Context(), f)
	if err != nil {
		storeErr(w, err)
		return
	}
	s.hub.Publish(events.Event{Type: "messages.changed"})
	writeJSON(w, 200, map[string]int64{"deleted": n})
}

var cidRe = regexp.MustCompile(`(?i)cid:([^"'\s)>]+)`)

// messageHTML returns the HTML body with cid: references inlined as data URIs,
// so the UI can render it inside a fully sandboxed iframe (srcdoc, no scripts).
func (s *Server) messageHTML(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorizeMessage(w, r, id) {
		return
	}
	html, atts, err := s.store.GetMessageHTML(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	byCID := map[string]*store.Attachment{}
	for _, a := range atts {
		if a.ContentID != "" && a.Data != nil {
			byCID[strings.ToLower(a.ContentID)] = a
		}
	}
	html = cidRe.ReplaceAllStringFunc(html, func(m string) string {
		if a, ok := byCID[strings.ToLower(m[4:])]; ok {
			return "data:" + a.ContentType + ";base64," + base64.StdEncoding.EncodeToString(a.Data)
		}
		return m
	})
	writeJSON(w, 200, map[string]string{"html": html})
}

func (s *Server) messageRaw(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorizeMessage(w, r, id) {
		return
	}
	raw, subject, err := s.store.GetMessageRaw(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	if r.URL.Query().Get("download") != "" {
		name := safeFilename(subject, "message") + ".eml"
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		w.Header().Set("Content-Type", "message/rfc822")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Write(raw)
}

func (s *Server) attachment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorizeMessage(w, r, id) {
		return
	}
	a, err := s.store.GetAttachment(r.Context(), id, r.PathValue("aid"))
	if err != nil {
		storeErr(w, err)
		return
	}
	// Always force download: never render user-supplied content on our origin.
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": safeFilename(a.Filename, "attachment")}))
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Write(a.Data)
}

var unsafeChars = regexp.MustCompile(`[^\p{L}\p{N}._ -]+`)

func safeFilename(s, def string) string {
	s = strings.TrimSpace(unsafeChars.ReplaceAllString(s, "_"))
	if len(s) > 100 {
		s = s[:100]
	}
	if s == "" {
		return def
	}
	return strings.ToValidUTF8(s, "")
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats(r.Context(), from(r).allowedInboxes())
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, st)
}

// events streams live updates with Server-Sent Events.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "streaming unsupported")
		return
	}
	p := from(r)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ch := s.hub.Subscribe()
	defer s.hub.Unsubscribe(ch)
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case e := <-ch:
			if e.InboxID != "" && !p.canAccess(e.InboxID) {
				continue
			}
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}
