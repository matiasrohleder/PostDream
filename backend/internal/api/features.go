package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/matiasrohleder/PostDream/internal/events"
	"github.com/matiasrohleder/PostDream/internal/netguard"
	"github.com/matiasrohleder/PostDream/internal/store"
	"github.com/matiasrohleder/PostDream/internal/webhook"
)

// --- per-inbox settings ---

type settingsBody struct {
	SpamCheck         *bool     `json:"spam_check"`
	AllowedHosts      *[]string `json:"allowed_hosts"`
	LinkDomains       *[]string `json:"link_domains"`
	AllowedRecipients *string   `json:"allowed_recipients"`
	RelayRecipients   *string   `json:"relay_recipients"`
}

func cleanList(in *[]string) *[]string {
	if in == nil {
		return nil
	}
	out := []string{}
	for _, v := range *in {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return &out
}

func validRegex(p *string) bool {
	if p == nil || *p == "" {
		return true
	}
	_, err := regexp.Compile(*p)
	return err == nil && len(*p) <= 500
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var b settingsBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if !validRegex(b.AllowedRecipients) {
		writeErr(w, 400, "allowed_recipients is not a valid regular expression")
		return
	}
	if !validRegex(b.RelayRecipients) {
		writeErr(w, 400, "relay_recipients is not a valid regular expression")
		return
	}
	for _, list := range []*[]string{b.AllowedHosts, b.LinkDomains} {
		if list != nil && len(*list) > 100 {
			writeErr(w, 400, "too many entries (max 100)")
			return
		}
	}
	in, err := s.store.UpdateInboxSettings(r.Context(), r.PathValue("id"), store.SettingsUpdate{
		SpamCheck: b.SpamCheck, AllowedHosts: cleanList(b.AllowedHosts), LinkDomains: cleanList(b.LinkDomains),
		AllowedRecipients: trimPtr(b.AllowedRecipients), RelayRecipients: trimPtr(b.RelayRecipients),
	})
	if err != nil {
		storeErr(w, err)
		return
	}
	s.hub.Publish(events.Event{Type: "inboxes.changed"})
	writeJSON(w, 200, in)
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	return &v
}

// --- chaos (admins, or the inbox's own API token so tests can toggle faults) ---

func (s *Server) canManageInbox(r *http.Request, id string) bool {
	p := from(r)
	return p.isAdmin() || (p.Inbox != nil && p.Inbox.ID == id)
}

func (s *Server) getChaos(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !from(r).canAccess(id) {
		writeErr(w, 404, "not found")
		return
	}
	in, err := s.store.GetInboxLite(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, in.Chaos)
}

func validChaos(rule *store.ChaosRule) bool {
	return rule == nil || (rule.Code >= 400 && rule.Code <= 599 && rule.Probability >= 0 && rule.Probability <= 100 && len(rule.Message) <= 200)
}

func (s *Server) putChaos(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.canManageInbox(r, id) {
		writeErr(w, 403, "only admins or this inbox's API token can change chaos rules")
		return
	}
	var c store.Chaos
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	for _, rule := range []*store.ChaosRule{c.Auth, c.Mail, c.Rcpt, c.Data} {
		if !validChaos(rule) {
			writeErr(w, 400, "each rule needs code 400-599, probability 0-100 and a message of at most 200 chars")
			return
		}
		if rule != nil {
			rule.Message = strings.Map(func(r rune) rune {
				if r < 0x20 {
					return -1
				}
				return r
			}, rule.Message)
		}
	}
	in, err := s.store.SetChaos(r.Context(), id, c)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, in.Chaos)
}

// --- webhooks ---

type webhookBody struct {
	URL    *string   `json:"url"`
	Events *[]string `json:"events"`
	Active *bool     `json:"active"`
}

func validEvents(ev *[]string) bool {
	if ev == nil {
		return true
	}
	if len(*ev) == 0 {
		return false
	}
	for _, e := range *ev {
		if !slices.Contains(webhook.Events, e) {
			return false
		}
	}
	return true
}

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListWebhooks(r.Context(), r.PathValue("id"))
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	var b webhookBody
	if err := readJSON(r, &b); err != nil || b.URL == nil {
		writeErr(w, 400, "url is required")
		return
	}
	if err := netguard.ValidateURL(strings.TrimSpace(*b.URL)); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	events := []string{webhook.EventReceived}
	if b.Events != nil {
		if !validEvents(b.Events) {
			writeErr(w, 400, "unknown event")
			return
		}
		events = *b.Events
	}
	if _, err := s.store.GetInboxLite(r.Context(), r.PathValue("id")); err != nil {
		storeErr(w, err)
		return
	}
	hook, err := s.store.CreateWebhook(r.Context(), r.PathValue("id"), strings.TrimSpace(*b.URL), events)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 201, hook)
}

func (s *Server) updateWebhook(w http.ResponseWriter, r *http.Request) {
	var b webhookBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if b.URL != nil {
		v := strings.TrimSpace(*b.URL)
		if err := netguard.ValidateURL(v); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		b.URL = &v
	}
	if !validEvents(b.Events) {
		writeErr(w, 400, "unknown event")
		return
	}
	hook, err := s.store.UpdateWebhook(r.Context(), r.PathValue("id"), store.WebhookUpdate{URL: b.URL, Events: b.Events, Active: b.Active})
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, hook)
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteWebhook(r.Context(), r.PathValue("id")); err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) rotateWebhook(w http.ResponseWriter, r *http.Request) {
	hook, err := s.store.RotateWebhookSecret(r.Context(), r.PathValue("id"))
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, hook)
}

// testWebhook sends a signed "ping" synchronously and returns the outcome.
func (s *Server) testWebhook(w http.ResponseWriter, r *http.Request) {
	hook, err := s.store.GetWebhook(r.Context(), r.PathValue("id"))
	if err != nil {
		storeErr(w, err)
		return
	}
	in, err := s.store.GetInboxLite(r.Context(), hook.InboxID)
	if err != nil {
		storeErr(w, err)
		return
	}
	id := "dlv_" + store.RandomToken(12)
	body, _ := json.Marshal(map[string]any{
		"event": webhook.EventPing, "delivery_id": id, "webhook_id": hook.ID,
		"inbox": map[string]string{"id": in.ID, "slug": in.Slug, "name": in.Name},
	})
	res := s.Webhooks.Send(r.Context(), hook, webhook.EventPing, id, body, 1)
	writeJSON(w, 200, res)
}

func (s *Server) webhookDeliveries(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListDeliveries(r.Context(), r.PathValue("id"), 50)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

// --- tag rules ---

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.ListTagRules(r.Context(), r.URL.Query().Get("inbox"))
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, rules)
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	var b struct {
		InboxID *string `json:"inbox_id"`
		Query   string  `json:"query"`
		Tag     string  `json:"tag"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	b.Query = strings.TrimSpace(b.Query)
	tags := store.NormalizeTags([]string{b.Tag})
	if b.Query == "" || len(tags) != 1 || len(b.Query) > 1000 {
		writeErr(w, 400, "query and tag are required")
		return
	}
	terms, err := store.ParseSearch(b.Query)
	if err != nil {
		writeErr(w, 400, "invalid query: "+err.Error())
		return
	}
	if len(terms) == 0 {
		writeErr(w, 400, "query has no conditions")
		return
	}
	if b.InboxID != nil && *b.InboxID == "" {
		b.InboxID = nil
	}
	rule, err := s.store.CreateTagRule(r.Context(), b.InboxID, b.Query, tags[0])
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 201, rule)
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteTagRule(r.Context(), r.PathValue("id")); err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- tags ---

func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	p := from(r)
	inboxes := p.allowedInboxes()
	if id := r.URL.Query().Get("inbox"); id != "" {
		if !p.canAccess(id) {
			writeErr(w, 404, "not found")
			return
		}
		inboxes = []string{id}
	}
	tags, err := s.store.ListTags(r.Context(), inboxes)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, tags)
}

func (s *Server) setTags(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorizeMessage(w, r, id) {
		return
	}
	var b struct {
		Tags []string `json:"tags"`
	}
	if err := readJSON(r, &b); err != nil || len(b.Tags) > 50 {
		writeErr(w, 400, "tags (max 50) required")
		return
	}
	tags, err := s.store.SetMessageTags(r.Context(), id, b.Tags)
	if err != nil {
		storeErr(w, err)
		return
	}
	s.hub.Publish(events.Event{Type: "messages.changed"})
	writeJSON(w, 200, map[string][]string{"tags": tags})
}

// bulkTags adds and/or removes tags on several messages.
func (s *Server) bulkTags(w http.ResponseWriter, r *http.Request) {
	var b struct {
		IDs    []string `json:"ids"`
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}
	if err := readJSON(r, &b); err != nil || len(b.IDs) == 0 || len(b.IDs) > 1000 {
		writeErr(w, 400, "ids (1..1000) required")
		return
	}
	ids := s.filterAccessible(r, b.IDs)
	if err := s.store.AddTags(r.Context(), ids, b.Add); err != nil {
		storeErr(w, err)
		return
	}
	if len(b.Remove) > 0 {
		if err := s.store.RemoveTags(r.Context(), ids, b.Remove); err != nil {
			storeErr(w, err)
			return
		}
	}
	s.hub.Publish(events.Event{Type: "messages.changed"})
	writeJSON(w, 200, map[string]int{"updated": len(ids)})
}
