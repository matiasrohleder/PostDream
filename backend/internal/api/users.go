package api

import (
	"net/http"
	"strings"

	"github.com/matiasrohleder/PostDream/internal/store"
)

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, users)
}

type userBody struct {
	Email    *string   `json:"email"`
	Name     *string   `json:"name"`
	Password *string   `json:"password"`
	Role     *string   `json:"role"`
	InboxIDs *[]string `json:"inbox_ids"`
}

func validRole(r *string) bool { return r == nil || *r == "admin" || *r == "member" }

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var b userBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if !strings.Contains(str(b.Email), "@") {
		writeErr(w, 400, "valid email required")
		return
	}
	if len(str(b.Password)) < 8 {
		writeErr(w, 400, "password must have at least 8 characters")
		return
	}
	role := "member"
	if b.Role != nil {
		if !validRole(b.Role) {
			writeErr(w, 400, "role must be admin or member")
			return
		}
		role = *b.Role
	}
	u, err := s.store.CreateUser(r.Context(), str(b.Email), str(b.Name), *b.Password, role)
	if err != nil {
		storeErr(w, err)
		return
	}
	if b.InboxIDs != nil {
		if u, err = s.store.UpdateUser(r.Context(), u.ID, store.UserUpdate{InboxIDs: b.InboxIDs}); err != nil {
			storeErr(w, err)
			return
		}
	}
	writeJSON(w, 201, u)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var b userBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if !validRole(b.Role) {
		writeErr(w, 400, "role must be admin or member")
		return
	}
	if b.Password != nil && len(*b.Password) < 8 {
		writeErr(w, 400, "password must have at least 8 characters")
		return
	}
	if b.Role != nil && *b.Role != "admin" {
		if target, err := s.store.GetUser(r.Context(), id); err == nil && target.IsAdmin() {
			if n, _ := s.store.CountAdmins(r.Context()); n <= 1 {
				writeErr(w, 400, "cannot demote the last admin")
				return
			}
		}
	}
	u, err := s.store.UpdateUser(r.Context(), id, store.UserUpdate{Name: b.Name, Role: b.Role, Password: b.Password, InboxIDs: b.InboxIDs})
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if me := from(r).User; me != nil && me.ID == id {
		writeErr(w, 400, "you cannot delete yourself")
		return
	}
	if target, err := s.store.GetUser(r.Context(), id); err == nil && target.IsAdmin() {
		if n, _ := s.store.CountAdmins(r.Context()); n <= 1 {
			writeErr(w, 400, "cannot delete the last admin")
			return
		}
	}
	if err := s.store.DeleteUser(r.Context(), id); err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
