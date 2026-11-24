package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spa serves the embedded React build, falling back to index.html for client routes.
func (s *Server) spa() http.Handler {
	fileServer := http.FileServer(http.FS(s.Static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.Static, p); err != nil {
			p = "index.html"
		}
		index, err := fs.ReadFile(s.Static, "index.html")
		if err != nil {
			http.Error(w, "UI not built. Run `npm run build` in frontend/ (the Docker image does this automatically).", 503)
			return
		}
		if p == "index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(index)
			return
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}
