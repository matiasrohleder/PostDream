// Package retention periodically deletes old messages and expired sessions.
package retention

import (
	"context"
	"log/slog"
	"time"

	"github.com/matiasrohleder/PostDream/internal/events"
	"github.com/matiasrohleder/PostDream/internal/metrics"
	"github.com/matiasrohleder/PostDream/internal/store"
)

type Janitor struct {
	Store        *store.Store
	Hub          *events.Hub
	DefaultHours int
	MaxPerInbox  int
	Interval     time.Duration
}

func (j *Janitor) Run(ctx context.Context) {
	j.sweep(ctx)
	t := time.NewTicker(j.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			j.sweep(ctx)
		}
	}
}

func (j *Janitor) sweep(ctx context.Context) {
	var total int64
	if n, err := j.Store.PurgeOld(ctx, j.DefaultHours); err != nil {
		slog.Error("retention purge failed", "err", err)
	} else {
		total += n
	}
	if j.MaxPerInbox > 0 {
		if n, err := j.Store.TrimInboxes(ctx, j.MaxPerInbox); err != nil {
			slog.Error("retention trim failed", "err", err)
		} else {
			total += n
		}
	}
	if _, err := j.Store.PurgeExpiredSessions(ctx); err != nil {
		slog.Error("session purge failed", "err", err)
	}
	if _, err := j.Store.PurgeFailedJobs(ctx, 7*24*time.Hour); err != nil {
		slog.Error("failed-jobs purge failed", "err", err)
	}
	if _, err := j.Store.PurgeLinkCache(ctx, time.Hour); err != nil {
		slog.Error("link cache purge failed", "err", err)
	}
	if total > 0 {
		metrics.Add(metrics.RetentionDeleted, float64(total))
		slog.Info("retention: deleted old messages", "count", total)
		j.Hub.Publish(events.Event{Type: "messages.changed"})
	}
}
