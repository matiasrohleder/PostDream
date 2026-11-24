// Package jobs runs durable background work stored in Postgres (webhook
// deliveries, spam analysis). Jobs survive restarts and are shared safely
// between replicas thanks to FOR UPDATE SKIP LOCKED.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/matiasrohleder/PostDream/internal/store"
)

// Handler processes one job payload. A returned error triggers a retry with backoff.
type Handler func(ctx context.Context, payload json.RawMessage, attempt int) error

// Permanent wraps an error that must not be retried.
type Permanent struct{ Err error }

func (p Permanent) Error() string { return p.Err.Error() }

type Runner struct {
	Store    *store.Store
	Workers  int
	handlers map[string]Handler
	wake     chan struct{}
	once     sync.Once
}

func (r *Runner) init() {
	r.once.Do(func() {
		r.handlers = map[string]Handler{}
		r.wake = make(chan struct{}, 1)
	})
}

// Handle registers the handler for a job kind.
func (r *Runner) Handle(kind string, h Handler) {
	r.init()
	r.handlers[kind] = h
}

// Enqueue stores a job and wakes the local workers.
func (r *Runner) Enqueue(ctx context.Context, kind string, payload any, maxAttempts int) error {
	r.init()
	if err := r.Store.EnqueueJob(ctx, kind, payload, 0, maxAttempts); err != nil {
		return err
	}
	select {
	case r.wake <- struct{}{}:
	default:
	}
	return nil
}

// Backoff returns the delay before retry number `attempt` (1-based): 10s, 30s, 2m, 10m, 30m, 1h...
func Backoff(attempt int) time.Duration {
	steps := []time.Duration{10 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute}
	if attempt-1 < len(steps) && attempt >= 1 {
		return steps[attempt-1]
	}
	return time.Hour
}

// Run starts the workers and blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	r.init()
	n := r.Workers
	if n < 1 {
		n = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.worker(ctx)
		}()
	}
	wg.Wait()
}

func (r *Runner) worker(ctx context.Context) {
	for ctx.Err() == nil {
		worked, err := r.runOne(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Error("job queue error", "err", err)
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		case <-time.After(time.Second):
		}
	}
}

func (r *Runner) runOne(ctx context.Context) (bool, error) {
	j, err := r.Store.ClaimJob(ctx, 5*time.Minute)
	if err != nil || j == nil {
		return false, err
	}
	h, ok := r.handlers[j.Kind]
	if !ok {
		return true, r.Store.RetryJob(ctx, &store.Job{ID: j.ID, Attempts: j.MaxAttempts, MaxAttempts: j.MaxAttempts}, "no handler for kind "+j.Kind, 0)
	}
	jctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	err = safeCall(jctx, h, j)
	cancel()
	// Use a fresh context: the job's outcome must be recorded even during shutdown.
	bg, cancelBg := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelBg()
	if err == nil {
		return true, r.Store.CompleteJob(bg, j.ID)
	}
	if _, perm := err.(Permanent); perm {
		j.Attempts = j.MaxAttempts
	}
	slog.Warn("job failed", "kind", j.Kind, "id", j.ID, "attempt", j.Attempts, "err", err)
	return true, r.Store.RetryJob(bg, j, err.Error(), Backoff(j.Attempts))
}

func safeCall(ctx context.Context, h Handler, j *store.Job) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = Permanent{fmt.Errorf("panic: %v", p)}
		}
	}()
	return h(ctx, j.Payload, j.Attempts)
}
