package store

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

func itoa(n int) string { return strconv.Itoa(n) }

type Attachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ContentID   string `json:"content_id"`
	Inline      bool   `json:"inline"`
	Size        int    `json:"size"`
	Data        []byte `json:"-"`
}

// MessageSummary is the row shown in the grid.
type MessageSummary struct {
	ID              string    `json:"id"`
	InboxID         string    `json:"inbox_id"`
	InboxName       string    `json:"inbox_name"`
	From            string    `json:"from"`
	To              string    `json:"to"`
	Subject         string    `json:"subject"`
	Snippet         string    `json:"snippet"`
	Size            int       `json:"size"`
	AttachmentCount int       `json:"attachment_count"`
	IsRead          bool      `json:"is_read"`
	ReceivedAt      time.Time `json:"received_at"`
	Tags            []string  `json:"tags"`
	SpamScore       *float32  `json:"spam_score"`
}

type Message struct {
	MessageSummary
	MessageID    string              `json:"message_id"`
	EnvelopeFrom string              `json:"envelope_from"`
	EnvelopeTo   []string            `json:"envelope_to"`
	Cc           string              `json:"cc"`
	TextBody     string              `json:"text_body"`
	HasHTML      bool                `json:"has_html"`
	Headers      map[string][]string `json:"headers"`
	ClientIP     string              `json:"client_ip"`
	SMTPUser     string              `json:"smtp_user"`
	Attachments  []*Attachment       `json:"attachments"`
	SpamReport   json.RawMessage     `json:"spam_report"`
}

// NewMessage is what the SMTP server hands to the store.
type NewMessage struct {
	InboxID      string
	MessageID    string
	EnvelopeFrom string
	EnvelopeTo   []string
	From         string
	To           string
	Cc           string
	Subject      string
	TextBody     string
	HTMLBody     string
	Headers      map[string][]string
	Raw          []byte
	ClientIP     string
	SMTPUser     string
	Attachments  []*Attachment
	Tags         []string
}

func (s *Store) InsertMessage(ctx context.Context, m *NewMessage) (*MessageSummary, error) {
	headers, _ := json.Marshal(m.Headers)
	// Inline parts (cid: images) are not "attachments" from the user's point of view.
	attCount := 0
	for _, a := range m.Attachments {
		if !a.Inline {
			attCount++
		}
	}
	search := strings.ToLower(strings.Join([]string{
		m.Subject, m.From, m.To, m.Cc, m.EnvelopeFrom, strings.Join(m.EnvelopeTo, " "), truncate(m.TextBody, 20000),
	}, " \n"))
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var id string
	var received time.Time
	err = tx.QueryRow(ctx, `INSERT INTO messages (inbox_id, message_id, envelope_from, envelope_to, from_header, to_header,
		cc_header, subject, text_body, html_body, headers, raw, size, attachment_count, client_ip, smtp_user, search_text)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) RETURNING id, received_at`,
		m.InboxID, m.MessageID, m.EnvelopeFrom, m.EnvelopeTo, m.From, m.To, m.Cc, m.Subject, m.TextBody, m.HTMLBody,
		headers, m.Raw, len(m.Raw), attCount, m.ClientIP, m.SMTPUser, search).Scan(&id, &received)
	if err != nil {
		return nil, err
	}
	tags := NormalizeTags(m.Tags)
	for _, t := range tags {
		if _, err := tx.Exec(ctx, `INSERT INTO message_tags (message_id, tag) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, t); err != nil {
			return nil, err
		}
	}
	for _, a := range m.Attachments {
		if _, err := tx.Exec(ctx, `INSERT INTO attachments (message_id, filename, content_type, content_id, inline, size, data)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, a.Filename, a.ContentType, a.ContentID, a.Inline, len(a.Data), a.Data); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &MessageSummary{
		ID: id, InboxID: m.InboxID, From: m.From, To: m.To, Subject: m.Subject, Snippet: snippet(m.TextBody),
		Size: len(m.Raw), AttachmentCount: attCount, ReceivedAt: received, Tags: tags,
	}, nil
}

type MessageFilter struct {
	InboxIDs       []string // nil = no restriction (admin)
	IDs            []string // restrict to these message ids (used to test rules against one message)
	Q              string   // search syntax; parsed into Terms when Terms is nil
	Terms          []Term
	From           string
	To             string
	Subject        string
	Since          *time.Time
	Until          *time.Time
	HasAttachments *bool
	Unread         *bool
	Sort           string
	Order          string
	Page           int
	PageSize       int
}

type MessagePage struct {
	Items    []*MessageSummary `json:"items"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}

func (f *MessageFilter) where() (string, []any) {
	conds := []string{"TRUE"}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return "$" + itoa(len(args))
	}
	if f.InboxIDs != nil {
		conds = append(conds, "m.inbox_id = ANY("+arg(f.InboxIDs)+"::uuid[])")
	}
	if f.IDs != nil {
		conds = append(conds, "m.id = ANY("+arg(f.IDs)+"::uuid[])")
	}
	terms := f.Terms
	if terms == nil && f.Q != "" {
		terms, _ = ParseSearch(f.Q)
	}
	now := time.Now()
	for _, t := range terms {
		conds = append(conds, termSQL(t, arg, now))
	}
	if f.From != "" {
		p := arg(likeEscape(f.From))
		conds = append(conds, "(m.from_header ILIKE "+p+" OR m.envelope_from ILIKE "+p+")")
	}
	if f.To != "" {
		p := arg(likeEscape(f.To))
		conds = append(conds, "(m.to_header ILIKE "+p+" OR m.cc_header ILIKE "+p+" OR array_to_string(m.envelope_to, ' ') ILIKE "+p+")")
	}
	if f.Subject != "" {
		conds = append(conds, "m.subject ILIKE "+arg(likeEscape(f.Subject)))
	}
	if f.Since != nil {
		conds = append(conds, "m.received_at >= "+arg(*f.Since))
	}
	if f.Until != nil {
		conds = append(conds, "m.received_at <= "+arg(*f.Until))
	}
	if f.HasAttachments != nil {
		if *f.HasAttachments {
			conds = append(conds, "m.attachment_count > 0")
		} else {
			conds = append(conds, "m.attachment_count = 0")
		}
	}
	if f.Unread != nil {
		conds = append(conds, "m.is_read = "+arg(!*f.Unread))
	}
	return strings.Join(conds, " AND "), args
}

var sortCols = map[string]string{
	"received_at": "m.received_at",
	"subject":     "lower(m.subject)",
	"from":        "lower(m.from_header)",
	"to":          "lower(m.to_header)",
	"size":        "m.size",
	"inbox":       "lower(i.name)",
}

func (s *Store) ListMessages(ctx context.Context, f MessageFilter) (*MessagePage, error) {
	if f.PageSize <= 0 || f.PageSize > 500 {
		f.PageSize = 50
	}
	if f.Page < 1 {
		f.Page = 1
	}
	col, ok := sortCols[f.Sort]
	if !ok {
		col = "m.received_at"
	}
	order := "DESC"
	if strings.EqualFold(f.Order, "asc") {
		order = "ASC"
	}
	where, args := f.where()

	page := &MessagePage{Items: []*MessageSummary{}, Page: f.Page, PageSize: f.PageSize}
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM messages m WHERE `+where, args...).Scan(&page.Total); err != nil {
		return nil, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := s.DB.Query(ctx, `SELECT m.id, m.inbox_id, i.name, m.from_header, m.to_header, m.subject,
		left(m.text_body, 300), m.size, m.attachment_count, m.is_read, m.received_at, `+tagsCol+`, m.spam_score
		FROM messages m JOIN inboxes i ON i.id = m.inbox_id
		WHERE `+where+` ORDER BY `+col+` `+order+`, m.id
		LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		m := &MessageSummary{}
		if err := rows.Scan(&m.ID, &m.InboxID, &m.InboxName, &m.From, &m.To, &m.Subject, &m.Snippet,
			&m.Size, &m.AttachmentCount, &m.IsRead, &m.ReceivedAt, &m.Tags, &m.SpamScore); err != nil {
			return nil, err
		}
		m.Snippet = snippet(m.Snippet)
		page.Items = append(page.Items, m)
	}
	return page, rows.Err()
}

func (s *Store) GetMessage(ctx context.Context, id string) (*Message, error) {
	m := &Message{}
	var headers, spam []byte
	var html string
	err := s.DB.QueryRow(ctx, `SELECT m.id, m.inbox_id, i.name, m.from_header, m.to_header, m.subject, m.size,
		m.attachment_count, m.is_read, m.received_at, m.message_id, m.envelope_from, m.envelope_to, m.cc_header,
		m.text_body, m.html_body, m.headers, m.client_ip, m.smtp_user, `+tagsCol+`, m.spam_score, m.spam_report
		FROM messages m JOIN inboxes i ON i.id = m.inbox_id WHERE m.id = $1`, id).Scan(
		&m.ID, &m.InboxID, &m.InboxName, &m.From, &m.To, &m.Subject, &m.Size, &m.AttachmentCount, &m.IsRead,
		&m.ReceivedAt, &m.MessageID, &m.EnvelopeFrom, &m.EnvelopeTo, &m.Cc, &m.TextBody, &html, &headers,
		&m.ClientIP, &m.SMTPUser, &m.Tags, &m.SpamScore, &spam)
	if err != nil {
		return nil, notFound(err)
	}
	m.HasHTML = html != ""
	m.Snippet = snippet(m.TextBody)
	json.Unmarshal(headers, &m.Headers)
	if len(spam) > 0 {
		m.SpamReport = spam
	}
	m.Attachments, err = s.listAttachments(ctx, id, false)
	return m, err
}

// MessageInbox returns the inbox id of a message (used for authorization).
func (s *Store) MessageInbox(ctx context.Context, id string) (string, error) {
	var inbox string
	err := s.DB.QueryRow(ctx, `SELECT inbox_id FROM messages WHERE id=$1`, id).Scan(&inbox)
	return inbox, notFound(err)
}

func (s *Store) GetMessageHTML(ctx context.Context, id string) (string, []*Attachment, error) {
	var html string
	if err := s.DB.QueryRow(ctx, `SELECT html_body FROM messages WHERE id=$1`, id).Scan(&html); err != nil {
		return "", nil, notFound(err)
	}
	atts, err := s.listAttachments(ctx, id, true)
	return html, atts, err
}

// tagsCol aggregates a message's tags (alias m) as a sorted text[].
const tagsCol = `ARRAY(SELECT mt.tag FROM message_tags mt WHERE mt.message_id = m.id ORDER BY mt.tag)`

func (s *Store) GetMessageRaw(ctx context.Context, id string) ([]byte, string, error) {
	var raw []byte
	var subject string
	err := s.DB.QueryRow(ctx, `SELECT raw, subject FROM messages WHERE id=$1`, id).Scan(&raw, &subject)
	return raw, subject, notFound(err)
}

func (s *Store) listAttachments(ctx context.Context, messageID string, withInlineData bool) ([]*Attachment, error) {
	dataCol := "NULL::bytea"
	if withInlineData {
		dataCol = "CASE WHEN content_id <> '' THEN data END"
	}
	rows, err := s.DB.Query(ctx, `SELECT id, filename, content_type, content_id, inline, size, `+dataCol+`
		FROM attachments WHERE message_id=$1 ORDER BY filename`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Attachment{}
	for rows.Next() {
		a := &Attachment{}
		if err := rows.Scan(&a.ID, &a.Filename, &a.ContentType, &a.ContentID, &a.Inline, &a.Size, &a.Data); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAttachment(ctx context.Context, messageID, attID string) (*Attachment, error) {
	a := &Attachment{}
	err := s.DB.QueryRow(ctx, `SELECT id, filename, content_type, content_id, inline, size, data
		FROM attachments WHERE id=$1 AND message_id=$2`, attID, messageID).Scan(
		&a.ID, &a.Filename, &a.ContentType, &a.ContentID, &a.Inline, &a.Size, &a.Data)
	return a, notFound(err)
}

func (s *Store) SetRead(ctx context.Context, ids []string, read bool) (int64, error) {
	tag, err := s.DB.Exec(ctx, `UPDATE messages SET is_read=$2 WHERE id = ANY($1::uuid[])`, ids, read)
	return tag.RowsAffected(), err
}

func (s *Store) DeleteMessages(ctx context.Context, ids []string) (int64, error) {
	tag, err := s.DB.Exec(ctx, `DELETE FROM messages WHERE id = ANY($1::uuid[])`, ids)
	return tag.RowsAffected(), err
}

// DeleteMatching deletes every message matching the filter (e.g. "clear inbox").
func (s *Store) DeleteMatching(ctx context.Context, f MessageFilter) (int64, error) {
	where, args := f.where()
	tag, err := s.DB.Exec(ctx, `DELETE FROM messages m WHERE `+where, args...)
	return tag.RowsAffected(), err
}

func (s *Store) MarkAllRead(ctx context.Context, f MessageFilter) (int64, error) {
	where, args := f.where()
	tag, err := s.DB.Exec(ctx, `UPDATE messages m SET is_read = true WHERE NOT m.is_read AND `+where, args...)
	return tag.RowsAffected(), err
}

// PurgeOld deletes messages older than each inbox's retention (or the global default).
func (s *Store) PurgeOld(ctx context.Context, defaultHours int) (int64, error) {
	tag, err := s.DB.Exec(ctx, `DELETE FROM messages m USING inboxes i
		WHERE m.inbox_id = i.id AND m.received_at < now() - make_interval(hours => COALESCE(i.retention_hours, $1))`, defaultHours)
	return tag.RowsAffected(), err
}

// TrimInboxes keeps only the newest maxPerInbox messages of every inbox.
func (s *Store) TrimInboxes(ctx context.Context, maxPerInbox int) (int64, error) {
	tag, err := s.DB.Exec(ctx, `DELETE FROM messages WHERE id IN (
		SELECT id FROM (SELECT id, row_number() OVER (PARTITION BY inbox_id ORDER BY received_at DESC) AS rn FROM messages) t
		WHERE t.rn > $1)`, maxPerInbox)
	return tag.RowsAffected(), err
}

type Stats struct {
	Total      int   `json:"total"`
	Unread     int   `json:"unread"`
	Last24h    int   `json:"last_24h"`
	TotalBytes int64 `json:"total_bytes"`
}

func (s *Store) Stats(ctx context.Context, inboxIDs []string) (*Stats, error) {
	st := &Stats{}
	q := `SELECT count(*), count(*) FILTER (WHERE NOT is_read),
		count(*) FILTER (WHERE received_at > now() - interval '24 hours'), COALESCE(sum(size),0) FROM messages`
	args := []any{}
	if inboxIDs != nil {
		q += ` WHERE inbox_id = ANY($1::uuid[])`
		args = append(args, inboxIDs)
	}
	err := s.DB.QueryRow(ctx, q, args...).Scan(&st.Total, &st.Unread, &st.Last24h, &st.TotalBytes)
	return st, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return s
}
