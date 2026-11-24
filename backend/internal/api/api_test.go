package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matiasrohleder/PostDream/internal/config"
	"github.com/matiasrohleder/PostDream/internal/store"
)

// TestOpenAPICoversRoutes fails when a route is added without documenting it
// (or the spec documents a route that no longer exists).
func TestOpenAPICoversRoutes(t *testing.T) {
	var spec struct {
		OpenAPI string                                `json:"openapi"`
		Paths   map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(openapiSpec, &spec); err != nil {
		t.Fatalf("openapi.json is not valid JSON: %v", err)
	}
	if !strings.HasPrefix(spec.OpenAPI, "3.1") {
		t.Errorf("expected OpenAPI 3.1, got %q", spec.OpenAPI)
	}
	s := New(Deps{Config: &config.Config{}})
	routes := map[string]bool{}
	for _, r := range s.Routes() {
		key := strings.ToLower(r.Method) + " " + r.Path
		routes[key] = true
		if _, ok := spec.Paths[r.Path][strings.ToLower(r.Method)]; !ok {
			t.Errorf("route %s %s is not documented in openapi.json", r.Method, r.Path)
		}
	}
	for path, methods := range spec.Paths {
		for m := range methods {
			if !routes[m+" "+path] {
				t.Errorf("openapi.json documents %s %s but no such route exists", strings.ToUpper(m), path)
			}
		}
	}
}

func TestHostGuard(t *testing.T) {
	s := New(Deps{Config: &config.Config{AllowedHosts: []string{"mail.acme.test"}}})
	h := s.hostGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for host, want := range map[string]int{
		"mail.acme.test:8025": 204, "MAIL.ACME.TEST": 204, "localhost:8025": 204, "[::1]:8025": 204,
		"evil.test": http.StatusMisdirectedRequest, "1.2.3.4:8025": http.StatusMisdirectedRequest,
	} {
		req := httptest.NewRequest("GET", "/api/health", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("host %s: got %d want %d", host, rec.Code, want)
		}
	}
}

func TestBuildMessage(t *testing.T) {
	raw, from, rcpts, err := buildMessage(&sendBody{
		From: "Shop <shop@acme.test>", To: []string{"a@x.test"}, Bcc: []string{"hidden@x.test"},
		Subject: "Hello", HTML: "<p>Hi</p>", Text: "Hi", Tags: []string{"welcome"},
		Headers:     map[string]string{"X-Custom": "1"},
		Attachments: []sendAttachment{{Filename: "a.txt", ContentType: "text/plain", Content: "aGVsbG8="}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{"Subject: Hello", "X-Tags: welcome", "X-Custom: 1", "Message-Id:", "a.txt", "multipart/"} {
		if !strings.Contains(strings.ToLower(s), strings.ToLower(want)) {
			t.Errorf("missing %q in message", want)
		}
	}
	if strings.Contains(s, "hidden@x.test") {
		t.Error("Bcc must not appear in the headers")
	}
	if from != "shop@acme.test" || len(rcpts) != 2 {
		t.Errorf("envelope: %s %v", from, rcpts)
	}
	if _, _, _, err := buildMessage(&sendBody{From: "a@x.test"}); err == nil {
		t.Error("expected error without recipients")
	}
	if _, _, _, err := buildMessage(&sendBody{From: "a@x.test", To: []string{"b@x.test"}, Headers: map[string]string{"X-Bad": "a\r\nBcc: x"}}); err == nil {
		t.Error("expected error for header injection")
	}
}

func TestPrincipalScopes(t *testing.T) {
	member := &principal{User: &store.User{Role: "member", InboxIDs: []string{"a"}}}
	if !member.canAccess("a") || member.canAccess("b") {
		t.Error("member scope wrong")
	}
	token := &principal{Inbox: &store.Inbox{ID: "b"}}
	if token.canAccess("a") || !token.canAccess("b") || token.isAdmin() {
		t.Error("token scope wrong")
	}
}

func TestAccessTokenMode(t *testing.T) {
	cfg := &config.Config{AuthEnabled: false, AccessToken: "s3cret-token"}
	if cfg.AuthMode() != config.AuthModeToken {
		t.Fatalf("auth mode = %q", cfg.AuthMode())
	}
	s := New(Deps{Config: cfg})
	var got *principal
	h := s.authenticate(func(w http.ResponseWriter, r *http.Request) { got = from(r); w.WriteHeader(204) }, false)
	call := func(req *http.Request) int {
		got = nil
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := call(httptest.NewRequest("GET", "/api/messages", nil)); code != 401 {
		t.Errorf("no credentials: got %d want 401", code)
	}

	req := httptest.NewRequest("GET", "/api/messages", nil)
	req.Header.Set("Authorization", "Bearer s3cret-token")
	if code := call(req); code != 204 || got == nil || got.User == nil || got.User.Role != "admin" {
		t.Errorf("bearer access token: got %d principal %+v", code, got)
	}

	req = httptest.NewRequest("GET", "/api/messages", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "forged"})
	if code := call(req); code != 401 {
		t.Errorf("forged cookie: got %d want 401", code)
	}

	// Login with the token sets a cookie that then authenticates.
	rec := httptest.NewRecorder()
	s.login(rec, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"token":"s3cret-token"}`)))
	if rec.Code != 200 {
		t.Fatalf("login: got %d %s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value == "s3cret-token" {
		t.Fatalf("login cookie: %+v", cookies)
	}
	req = httptest.NewRequest("GET", "/api/messages", nil)
	req.AddCookie(cookies[0])
	if code := call(req); code != 204 {
		t.Errorf("cookie after login: got %d want 204", code)
	}

	rec = httptest.NewRecorder()
	s.login(rec, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"token":"wrong"}`)))
	if rec.Code != 401 {
		t.Errorf("wrong token login: got %d want 401", rec.Code)
	}
}

func TestAuthModes(t *testing.T) {
	for _, c := range []struct {
		cfg  config.Config
		want string
	}{
		{config.Config{AuthEnabled: true}, config.AuthModeUsers},
		{config.Config{AuthEnabled: true, AccessToken: "ignored-token"}, config.AuthModeUsers},
		{config.Config{AuthEnabled: false}, config.AuthModeNone},
		{config.Config{AuthEnabled: false, AccessToken: "abcdefgh"}, config.AuthModeToken},
	} {
		if got := c.cfg.AuthMode(); got != c.want {
			t.Errorf("%+v: got %s want %s", c.cfg, got, c.want)
		}
	}
	// In users mode a Bearer equal to ACCESS_TOKEN is not special.
	s := New(Deps{Config: &config.Config{AuthEnabled: true, AccessToken: "ignored-token"}})
	if s.isAccessToken("ignored-token") {
		t.Error("access token must be ignored when user accounts are enabled")
	}
}
