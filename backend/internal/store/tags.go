package store

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// NormalizeTags trims, drops empty/oversized/control-char tags and de-duplicates
// case-insensitively (keeping the first spelling), returning a sorted list.
func NormalizeTags(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.TrimSpace(strings.ToValidUTF8(t, ""))
		t = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, t)
		if t == "" || utf8.RuneCountInString(t) > 100 || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// SetMessageTags replaces the tags of a message.
func (s *Store) SetMessageTags(ctx context.Context, id string, tags []string) ([]string, error) {
	tags = NormalizeTags(tags)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM message_tags WHERE message_id = $1`, id); err != nil {
		return nil, err
	}
	for _, t := range tags {
		if _, err := tx.Exec(ctx, `INSERT INTO message_tags (message_id, tag) VALUES ($1,$2)`, id, t); err != nil {
			return nil, err
		}
	}
	return tags, tx.Commit(ctx)
}

// AddTags adds tags to many messages at once (existing tags are kept).
func (s *Store) AddTags(ctx context.Context, ids []string, tags []string) error {
	tags = NormalizeTags(tags)
	if len(ids) == 0 || len(tags) == 0 {
		return nil
	}
	_, err := s.DB.Exec(ctx, `INSERT INTO message_tags (message_id, tag)
		SELECT m, t FROM unnest($1::uuid[]) m CROSS JOIN unnest($2::text[]) t
		WHERE NOT EXISTS (SELECT 1 FROM message_tags x WHERE x.message_id = m AND lower(x.tag) = lower(t))`, ids, tags)
	return err
}

// RemoveTags removes tags (case-insensitive) from many messages.
func (s *Store) RemoveTags(ctx context.Context, ids []string, tags []string) error {
	lower := make([]string, len(tags))
	for i, t := range tags {
		lower[i] = strings.ToLower(strings.TrimSpace(t))
	}
	_, err := s.DB.Exec(ctx, `DELETE FROM message_tags WHERE message_id = ANY($1::uuid[]) AND lower(tag) = ANY($2::text[])`, ids, lower)
	return err
}

type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// ListTags returns the tags in use within the given inboxes (nil = all) with message counts.
func (s *Store) ListTags(ctx context.Context, inboxIDs []string) ([]TagCount, error) {
	q := `SELECT min(mt.tag), count(*) FROM message_tags mt JOIN messages m ON m.id = mt.message_id`
	args := []any{}
	if inboxIDs != nil {
		q += ` WHERE m.inbox_id = ANY($1::uuid[])`
		args = append(args, inboxIDs)
	}
	q += ` GROUP BY lower(mt.tag) ORDER BY lower(mt.tag) LIMIT 500`
	rows, err := s.DB.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TagCount{}
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.Tag, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- tag rules ---

type TagRule struct {
	ID        string    `json:"id"`
	InboxID   *string   `json:"inbox_id"`
	Query     string    `json:"query"`
	Tag       string    `json:"tag"`
	CreatedAt time.Time `json:"created_at"`
}

// ListTagRules returns rules for an inbox (including global ones), or all rules when inboxID is "".
func (s *Store) ListTagRules(ctx context.Context, inboxID string) ([]*TagRule, error) {
	q := `SELECT id, inbox_id, query, tag, created_at FROM tag_rules`
	args := []any{}
	if inboxID != "" {
		q += ` WHERE inbox_id IS NULL OR inbox_id = $1`
		args = append(args, inboxID)
	}
	q += ` ORDER BY created_at`
	rows, err := s.DB.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TagRule{}
	for rows.Next() {
		r := &TagRule{}
		if err := rows.Scan(&r.ID, &r.InboxID, &r.Query, &r.Tag, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateTagRule(ctx context.Context, inboxID *string, query, tag string) (*TagRule, error) {
	r := &TagRule{}
	err := s.DB.QueryRow(ctx, `INSERT INTO tag_rules (inbox_id, query, tag) VALUES ($1,$2,$3)
		RETURNING id, inbox_id, query, tag, created_at`, inboxID, query, tag).Scan(&r.ID, &r.InboxID, &r.Query, &r.Tag, &r.CreatedAt)
	return r, err
}

func (s *Store) GetTagRule(ctx context.Context, id string) (*TagRule, error) {
	r := &TagRule{}
	err := s.DB.QueryRow(ctx, `SELECT id, inbox_id, query, tag, created_at FROM tag_rules WHERE id=$1`, id).
		Scan(&r.ID, &r.InboxID, &r.Query, &r.Tag, &r.CreatedAt)
	return r, notFound(err)
}

func (s *Store) DeleteTagRule(ctx context.Context, id string) error {
	tag, err := s.DB.Exec(ctx, `DELETE FROM tag_rules WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ApplyTagRules evaluates every rule of the message's inbox against it and adds
// the matching tags. Returns the message's resulting tags.
func (s *Store) ApplyTagRules(ctx context.Context, messageID, inboxID string) ([]string, error) {
	rules, err := s.ListTagRules(ctx, inboxID)
	if err != nil {
		return nil, err
	}
	var add []string
	for _, r := range rules {
		terms, err := ParseSearch(r.Query)
		if err != nil || len(terms) == 0 {
			continue
		}
		f := MessageFilter{IDs: []string{messageID}, Terms: terms}
		where, args := f.where()
		var ok bool
		if err := s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM messages m WHERE `+where+`)`, args...).Scan(&ok); err != nil {
			return nil, err
		}
		if ok {
			add = append(add, r.Tag)
		}
	}
	if len(add) > 0 {
		if err := s.AddTags(ctx, []string{messageID}, add); err != nil {
			return nil, err
		}
	}
	return s.MessageTags(ctx, messageID)
}

func (s *Store) MessageTags(ctx context.Context, id string) ([]string, error) {
	var tags []string
	err := s.DB.QueryRow(ctx, `SELECT `+tagsCol+` FROM messages m WHERE m.id = $1`, id).Scan(&tags)
	return tags, notFound(err)
}
