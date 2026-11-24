package api

import (
	_ "embed"
	"net/http"
)

// openapiSpec documents every route in Routes(); TestOpenAPICoversRoutes keeps them in sync.
//
//go:embed openapi.json
var openapiSpec []byte

func (s *Server) openAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(openapiSpec)
}
