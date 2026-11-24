package api

import (
	"net/http"
	"strings"

	"github.com/matiasrohleder/PostDream/internal/events"
	"github.com/matiasrohleder/PostDream/internal/store"
)

// Members see the SMTP credentials of the inboxes they belong to (they need them
// to configure their apps). API-token callers never see secrets.
func (s *Server) listInboxes(w http.ResponseWriter, r *http.Request) {
	p := from(r)
	var list []*store.Inbox
	var err error
	if p.Inbox != nil {
		in, err := s.store.GetInbox(r.Context(), p.Inbox.ID) // with counters
		if err != nil {
			storeErr(w, err)
			return
		}
		list = []*store.Inbox{in}
	} else {
		list, err = s.store.ListInboxes(r.Context(), p.User)
		if err != nil {
			storeErr(w, err)
			return
		}
	}
	if p.Inbox != nil {
		for _, in := range list {
			in.SMTPPassword, in.APIToken = "", ""
		}
	}
	writeJSON(w, 200, list)
}

func (s *Server) getInbox(w http.ResponseWriter, r *http.Request) {
	p := from(r)
	id := r.PathValue("id")
	if !p.canAccess(id) {
		writeErr(w, 404, "not found")
		return
	}
	in, err := s.store.GetInbox(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	if p.Inbox != nil {
		in.SMTPPassword, in.APIToken = "", ""
	}
	writeJSON(w, 200, in)
}

type inboxBody struct {
	Name           *string `json:"name"`
	Slug           *string `json:"slug"`
	Description    *string `json:"description"`
	SMTPUsername   *string `json:"smtp_username"`
	SMTPPassword   *string `json:"smtp_password"`
	RetentionHours *int    `json:"retention_hours"`
	ClearRetention bool    `json:"clear_retention"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func (s *Server) createInbox(w http.ResponseWriter, r *http.Request) {
	var b inboxBody
	if err := readJSON(r, &b); err != nil || str(b.Name) == "" {
		writeErr(w, 400, "name is required")
		return
	}
	slug := str(b.Slug)
	if slug != "" {
		slug = store.Slugify(slug)
	}
	in, err := s.store.CreateInbox(r.Context(), store.InboxInput{
		Name: str(b.Name), Slug: slug, Description: str(b.Description),
		SMTPUsername: str(b.SMTPUsername), SMTPPassword: str(b.SMTPPassword), RetentionHours: b.RetentionHours,
	})
	if err != nil {
		storeErr(w, err)
		return
	}
	s.hub.Publish(events.Event{Type: "inboxes.changed"})
	writeJSON(w, 201, in)
}

func (s *Server) updateInbox(w http.ResponseWriter, r *http.Request) {
	var b inboxBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	upd := store.InboxUpdate{Name: b.Name, Description: b.Description, SMTPUsername: b.SMTPUsername, SMTPPassword: b.SMTPPassword}
	if b.RetentionHours != nil {
		upd.RetentionHours = &b.RetentionHours
	} else if b.ClearRetention {
		var nilInt *int
		upd.RetentionHours = &nilInt
	}
	if b.SMTPPassword != nil && len(*b.SMTPPassword) < 6 {
		writeErr(w, 400, "SMTP password must have at least 6 characters")
		return
	}
	in, err := s.store.UpdateInbox(r.Context(), r.PathValue("id"), upd)
	if err != nil {
		storeErr(w, err)
		return
	}
	s.hub.Publish(events.Event{Type: "inboxes.changed"})
	writeJSON(w, 200, in)
}

func (s *Server) deleteInbox(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteInbox(r.Context(), r.PathValue("id")); err != nil {
		storeErr(w, err)
		return
	}
	s.hub.Publish(events.Event{Type: "inboxes.changed"})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) rotateInbox(w http.ResponseWriter, r *http.Request) {
	in, err := s.store.RotateInboxSecrets(r.Context(), r.PathValue("id"))
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, in)
}
