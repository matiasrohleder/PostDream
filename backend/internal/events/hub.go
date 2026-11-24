// Package events is the pub/sub used for live UI updates (SSE).
//
// Without a database it is purely in-process. With Attach, events are broadcast
// through Postgres LISTEN/NOTIFY so every replica behind a load balancer sees
// messages received by any other replica.
package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const channel = "postdream_events"

// Postgres rejects NOTIFY payloads of 8000 bytes or more.
const maxPayload = 7900

type Event struct {
	Type    string `json:"type"` // message.new | messages.changed | inboxes.changed
	InboxID string `json:"inbox_id,omitempty"`
	Data    any    `json:"data,omitempty"`
}

type Hub struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
	pool *pgxpool.Pool // nil = in-process only
}

func NewHub() *Hub { return &Hub{subs: map[chan Event]struct{}{}} }

func (h *Hub) Subscribe() chan Event {
	ch := make(chan Event, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

// Publish never blocks on subscribers: slow subscribers simply miss events.
func (h *Hub) Publish(e Event) {
	if h.pool == nil {
		h.broadcast(e)
		return
	}
	b, err := json.Marshal(e)
	if err == nil && len(b) > maxPayload {
		e.Data = nil
		b, err = json.Marshal(e)
	}
	if err != nil {
		h.broadcast(e)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := h.pool.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, string(b)); err != nil {
		slog.Warn("event notify failed, delivering locally", "err", err)
		h.broadcast(e)
	}
}

func (h *Hub) broadcast(e Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Attach switches the hub to Postgres LISTEN/NOTIFY and keeps a listener running
// until ctx is cancelled, reconnecting on errors.
func (h *Hub) Attach(ctx context.Context, pool *pgxpool.Pool) {
	h.pool = pool
	go func() {
		for ctx.Err() == nil {
			if err := h.listen(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("event listener stopped, reconnecting", "err", err)
				select {
				case <-ctx.Done():
				case <-time.After(2 * time.Second):
				}
			}
		}
	}()
}

func (h *Hub) listen(ctx context.Context) error {
	conn, err := h.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		return err
	}
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			// The connection may be mid-LISTEN; never hand it back to the pool.
			conn.Conn().Close(context.Background())
			return err
		}
		var e Event
		if json.Unmarshal([]byte(n.Payload), &e) == nil {
			h.broadcast(e)
		}
	}
}
