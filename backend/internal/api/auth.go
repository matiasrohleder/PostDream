package api

import (
	"net/http"
	"time"

	"github.com/matiasrohleder/PostDream/internal/store"
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	ip := s.clientIP(r)
	if s.loginLimiter.Blocked(ip) {
		writeErr(w, 429, "too many failed attempts, try again later")
		return
	}
	u, err := s.store.Authenticate(r.Context(), body.Email, body.Password)
	if err != nil {
		s.loginLimiter.Fail(ip)
		writeErr(w, 401, "invalid email or password")
		return
	}
	s.loginLimiter.Reset(ip)
	token, err := s.store.CreateSession(r.Context(), u.ID, s.cfg.SessionTTL)
	if err != nil {
		storeErr(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode, Expires: time.Now().Add(s.cfg.SessionTTL),
	})
	writeJSON(w, 200, u)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, from(r).User)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	p := from(r)
	if p.User.ID == "" {
		writeErr(w, 400, "authentication is disabled")
		return
	}
	var body struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if len(body.New) < 8 {
		writeErr(w, 400, "password must have at least 8 characters")
		return
	}
	if !s.store.CheckPassword(r.Context(), p.User.ID, body.Current) {
		writeErr(w, 400, "current password is incorrect")
		return
	}
	if _, err := s.store.UpdateUser(r.Context(), p.User.ID, store.UserUpdate{Password: &body.New}); err != nil {
		storeErr(w, err)
		return
	}
	// Sessions were invalidated: issue a fresh one so the user stays logged in.
	token, err := s.store.CreateSession(r.Context(), p.User.ID, s.cfg.SessionTTL)
	if err == nil {
		http.SetCookie(w, &http.Cookie{
			Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure,
			SameSite: http.SameSiteStrictMode, Expires: time.Now().Add(s.cfg.SessionTTL),
		})
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
