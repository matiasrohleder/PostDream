package store

import (
	"context"
	"time"
)

type Webhook struct {
	ID             string     `json:"id"`
	InboxID        string     `json:"inbox_id"`
	URL            string     `json:"url"`
	Secret         string     `json:"secret,omitempty"`
	Events         []string   `json:"events"`
	Active         bool       `json:"active"`
	CreatedAt      time.Time  `json:"created_at"`
	LastStatus     *int       `json:"last_status"`
	LastDeliveryAt *time.Time `json:"last_delivery_at"`
}

const webhookCols = `id, inbox_id, url, secret, events, active, created_at, last_status, last_delivery_at`

func scanWebhook(row interface{ Scan(...any) error }) (*Webhook, error) {
	w := &Webhook{}
	err := row.Scan(&w.ID, &w.InboxID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt, &w.LastStatus, &w.LastDeliveryAt)
	return w, notFound(err)
}

func (s *Store) ListWebhooks(ctx context.Context, inboxID string) ([]*Webhook, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+webhookCols+` FROM webhooks WHERE inbox_id=$1 ORDER BY created_at`, inboxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Webhook{}
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ActiveWebhooks returns the active webhooks of an inbox subscribed to event.
func (s *Store) ActiveWebhooks(ctx context.Context, inboxID, event string) ([]*Webhook, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+webhookCols+` FROM webhooks WHERE inbox_id=$1 AND active AND $2 = ANY(events)`, inboxID, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Webhook{}
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) GetWebhook(ctx context.Context, id string) (*Webhook, error) {
	return scanWebhook(s.DB.QueryRow(ctx, `SELECT `+webhookCols+` FROM webhooks WHERE id=$1`, id))
}

func (s *Store) CreateWebhook(ctx context.Context, inboxID, url string, events []string) (*Webhook, error) {
	return scanWebhook(s.DB.QueryRow(ctx, `INSERT INTO webhooks (inbox_id, url, secret, events) VALUES ($1,$2,$3,$4)
		RETURNING `+webhookCols, inboxID, url, "whsec_"+RandomToken(24), events))
}

type WebhookUpdate struct {
	URL    *string
	Events *[]string
	Active *bool
}

func (s *Store) UpdateWebhook(ctx context.Context, id string, u WebhookUpdate) (*Webhook, error) {
	_, err := s.DB.Exec(ctx, `UPDATE webhooks SET url = COALESCE($2, url), events = COALESCE($3, events),
		active = COALESCE($4, active) WHERE id = $1`, id, u.URL, u.Events, u.Active)
	if err != nil {
		return nil, err
	}
	return s.GetWebhook(ctx, id)
}

func (s *Store) RotateWebhookSecret(ctx context.Context, id string) (*Webhook, error) {
	return scanWebhook(s.DB.QueryRow(ctx, `UPDATE webhooks SET secret=$2 WHERE id=$1 RETURNING `+webhookCols, id, "whsec_"+RandomToken(24)))
}

func (s *Store) DeleteWebhook(ctx context.Context, id string) error {
	tag, err := s.DB.Exec(ctx, `DELETE FROM webhooks WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

type WebhookDelivery struct {
	ID         int64     `json:"id"`
	WebhookID  string    `json:"webhook_id"`
	Event      string    `json:"event"`
	Attempt    int       `json:"attempt"`
	StatusCode int       `json:"status_code"`
	Error      string    `json:"error"`
	DurationMS int       `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// RecordDelivery stores one attempt, updates the webhook's last status and keeps
// only the newest 100 deliveries per webhook.
func (s *Store) RecordDelivery(ctx context.Context, d *WebhookDelivery) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO webhook_deliveries (webhook_id, event, attempt, status_code, error, duration_ms)
		VALUES ($1,$2,$3,$4,$5,$6)`, d.WebhookID, d.Event, d.Attempt, d.StatusCode, d.Error, d.DurationMS); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE webhooks SET last_status=$2, last_delivery_at=now() WHERE id=$1`, d.WebhookID, d.StatusCode); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM webhook_deliveries WHERE webhook_id=$1 AND id NOT IN (
		SELECT id FROM webhook_deliveries WHERE webhook_id=$1 ORDER BY id DESC LIMIT 100)`, d.WebhookID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ListDeliveries(ctx context.Context, webhookID string, limit int) ([]*WebhookDelivery, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, webhook_id, event, attempt, status_code, error, duration_ms, created_at
		FROM webhook_deliveries WHERE webhook_id=$1 ORDER BY id DESC LIMIT $2`, webhookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*WebhookDelivery{}
	for rows.Next() {
		d := &WebhookDelivery{}
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.Event, &d.Attempt, &d.StatusCode, &d.Error, &d.DurationMS, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
