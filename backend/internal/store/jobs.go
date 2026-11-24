package store

import (
	"context"
	"encoding/json"
	"time"
)

type Job struct {
	ID          int64
	Kind        string
	Payload     json.RawMessage
	Attempts    int // including the current one
	MaxAttempts int
}

// EnqueueJob schedules background work. delay may be zero.
func (s *Store) EnqueueJob(ctx context.Context, kind string, payload any, delay time.Duration, maxAttempts int) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO jobs (kind, payload, run_at, max_attempts) VALUES ($1,$2,now() + make_interval(secs => $3),$4)`,
		kind, b, delay.Seconds(), maxAttempts)
	return err
}

// ClaimJob locks the next due job for lease, or returns nil when there is none.
// SKIP LOCKED lets several workers/replicas share the queue without contention.
func (s *Store) ClaimJob(ctx context.Context, lease time.Duration) (*Job, error) {
	j := &Job{}
	err := s.DB.QueryRow(ctx, `UPDATE jobs SET locked_until = now() + make_interval(secs => $1), attempts = attempts + 1
		WHERE id = (SELECT id FROM jobs WHERE NOT failed AND run_at <= now() AND (locked_until IS NULL OR locked_until < now())
		            ORDER BY run_at LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING id, kind, payload, attempts, max_attempts`, lease.Seconds()).Scan(&j.ID, &j.Kind, &j.Payload, &j.Attempts, &j.MaxAttempts)
	if err != nil {
		if notFound(err) == ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return j, nil
}

func (s *Store) CompleteJob(ctx context.Context, id int64) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, id)
	return err
}

// RetryJob reschedules a failed attempt, or marks the job as failed for good.
func (s *Store) RetryJob(ctx context.Context, j *Job, errMsg string, backoff time.Duration) error {
	if j.Attempts >= j.MaxAttempts {
		_, err := s.DB.Exec(ctx, `UPDATE jobs SET failed=true, locked_until=NULL, last_error=$2 WHERE id=$1`, j.ID, errMsg)
		return err
	}
	_, err := s.DB.Exec(ctx, `UPDATE jobs SET run_at = now() + make_interval(secs => $2), locked_until=NULL, last_error=$3 WHERE id=$1`,
		j.ID, backoff.Seconds(), errMsg)
	return err
}

// PurgeFailedJobs removes jobs that gave up more than `age` ago.
func (s *Store) PurgeFailedJobs(ctx context.Context, age time.Duration) (int64, error) {
	tag, err := s.DB.Exec(ctx, `DELETE FROM jobs WHERE failed AND created_at < now() - make_interval(secs => $1)`, age.Seconds())
	return tag.RowsAffected(), err
}

type JobStats struct {
	Pending int `json:"pending"`
	Failed  int `json:"failed"`
}

func (s *Store) JobStats(ctx context.Context) (*JobStats, error) {
	st := &JobStats{}
	err := s.DB.QueryRow(ctx, `SELECT count(*) FILTER (WHERE NOT failed), count(*) FILTER (WHERE failed) FROM jobs`).Scan(&st.Pending, &st.Failed)
	return st, err
}

// --- spam results ---

func (s *Store) SetSpamResult(ctx context.Context, messageID string, score float64, report any) error {
	b, err := json.Marshal(report)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `UPDATE messages SET spam_score=$2, spam_report=$3 WHERE id=$1`, messageID, score, b)
	return err
}

// --- link cache ---

type CachedLink struct {
	Status    int
	Error     string
	CheckedAt time.Time
}

func (s *Store) CachedLinks(ctx context.Context, urls []string, maxAge time.Duration) (map[string]CachedLink, error) {
	rows, err := s.DB.Query(ctx, `SELECT url, status, error, checked_at FROM link_cache
		WHERE url = ANY($1::text[]) AND checked_at > now() - make_interval(secs => $2)`, urls, maxAge.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]CachedLink{}
	for rows.Next() {
		var u string
		var c CachedLink
		if err := rows.Scan(&u, &c.Status, &c.Error, &c.CheckedAt); err != nil {
			return nil, err
		}
		out[u] = c
	}
	return out, rows.Err()
}

func (s *Store) CacheLink(ctx context.Context, url string, status int, errMsg string) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO link_cache (url, status, error) VALUES ($1,$2,$3)
		ON CONFLICT (url) DO UPDATE SET status=EXCLUDED.status, error=EXCLUDED.error, checked_at=now()`, url, status, errMsg)
	return err
}

func (s *Store) PurgeLinkCache(ctx context.Context, age time.Duration) (int64, error) {
	tag, err := s.DB.Exec(ctx, `DELETE FROM link_cache WHERE checked_at < now() - make_interval(secs => $1)`, age.Seconds())
	return tag.RowsAffected(), err
}

// --- relay audit ---

type RelayEntry struct {
	ID         int64     `json:"id"`
	MessageID  string    `json:"message_id"`
	InboxID    *string   `json:"inbox_id"`
	UserEmail  string    `json:"user_email"`
	Recipients []string  `json:"recipients"`
	Subject    string    `json:"subject"`
	OK         bool      `json:"ok"`
	Error      string    `json:"error"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Store) LogRelay(ctx context.Context, e *RelayEntry) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO relay_log (message_id, inbox_id, user_email, recipients, subject, ok, error)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, e.MessageID, e.InboxID, e.UserEmail, e.Recipients, e.Subject, e.OK, e.Error)
	return err
}

func (s *Store) ListRelayLog(ctx context.Context, inboxIDs []string, limit int) ([]*RelayEntry, error) {
	q := `SELECT id, message_id, inbox_id, user_email, recipients, subject, ok, error, created_at FROM relay_log`
	args := []any{}
	if inboxIDs != nil {
		q += ` WHERE inbox_id = ANY($1::uuid[])`
		args = append(args, inboxIDs)
	}
	args = append(args, limit)
	q += ` ORDER BY id DESC LIMIT $` + itoa(len(args))
	rows, err := s.DB.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*RelayEntry{}
	for rows.Next() {
		e := &RelayEntry{}
		if err := rows.Scan(&e.ID, &e.MessageID, &e.InboxID, &e.UserEmail, &e.Recipients, &e.Subject, &e.OK, &e.Error, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- POP3 ---

type MessageRef struct {
	ID   string
	Size int
}

// ListMessageRefs returns ids and sizes of an inbox's newest `limit` messages, oldest first.
func (s *Store) ListMessageRefs(ctx context.Context, inboxID string, limit int) ([]MessageRef, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, size FROM (
		SELECT id, size, received_at FROM messages WHERE inbox_id=$1 ORDER BY received_at DESC LIMIT $2) t
		ORDER BY received_at, id`, inboxID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MessageRef{}
	for rows.Next() {
		var r MessageRef
		if err := rows.Scan(&r.ID, &r.Size); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- metrics ---

type InboxUsage struct {
	Slug     string
	Messages int
	Unread   int
	Bytes    int64
}

func (s *Store) InboxUsage(ctx context.Context) ([]InboxUsage, error) {
	rows, err := s.DB.Query(ctx, `SELECT i.slug, count(m.id), count(m.id) FILTER (WHERE NOT m.is_read), COALESCE(sum(m.size),0)
		FROM inboxes i LEFT JOIN messages m ON m.inbox_id = i.id GROUP BY i.slug ORDER BY i.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InboxUsage{}
	for rows.Next() {
		var u InboxUsage
		if err := rows.Scan(&u.Slug, &u.Messages, &u.Unread, &u.Bytes); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
