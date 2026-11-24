package api

import (
	"crypto/subtle"
	"net/http"

	"github.com/matiasrohleder/PostDream/internal/metrics"
)

// MetricsHandler serves Prometheus metrics (counters plus per-inbox gauges read
// from the database at scrape time).
func (s *Server) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var gauges []metrics.Gauge
		if usage, err := s.store.InboxUsage(r.Context()); err == nil {
			for _, u := range usage {
				l := map[string]string{"inbox": u.Slug}
				gauges = append(gauges,
					metrics.Gauge{Name: "postdream_messages_stored", Help: "Messages currently stored, by inbox.", Labels: l, Value: float64(u.Messages)},
					metrics.Gauge{Name: "postdream_messages_unread", Help: "Unread messages, by inbox.", Labels: l, Value: float64(u.Unread)},
					metrics.Gauge{Name: "postdream_stored_bytes", Help: "Bytes of stored messages, by inbox.", Labels: l, Value: float64(u.Bytes)},
				)
			}
		}
		if js, err := s.store.JobStats(r.Context()); err == nil {
			gauges = append(gauges,
				metrics.Gauge{Name: "postdream_jobs_pending", Help: "Background jobs waiting or retrying.", Value: float64(js.Pending)},
				metrics.Gauge{Name: "postdream_jobs_failed", Help: "Background jobs that gave up.", Value: float64(js.Failed)},
			)
		}
		up := 1.0
		if s.store.DB.Ping(r.Context()) != nil {
			up = 0
		}
		gauges = append(gauges, metrics.Gauge{Name: "postdream_database_up", Help: "1 when the database answers.", Value: up})
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		metrics.Write(w, gauges)
	})
}

// metricsAuth serves /metrics on the main port behind METRICS_TOKEN.
func (s *Server) metricsAuth(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(bearer(r)), []byte(s.cfg.MetricsToken)) != 1 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s.MetricsHandler().ServeHTTP(w, r)
}
