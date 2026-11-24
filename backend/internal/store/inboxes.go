package store

import (
	"context"
	"crypto/subtle"
	"regexp"
	"strings"
	"time"
)

type Inbox struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Description    string `json:"description"`
	SMTPUsername   string `json:"smtp_username"`
	SMTPPassword   string `json:"smtp_password,omitempty"`
	APIToken       string `json:"api_token,omitempty"`
	RetentionHours *int   `json:"retention_hours"`
	Settings
	CreatedAt     time.Time  `json:"created_at"`
	MessageCount  int        `json:"message_count"`
	UnreadCount   int        `json:"unread_count"`
	LastMessageAt *time.Time `json:"last_message_at"`
}

const inboxBaseCols = `i.id, i.name, i.slug, i.description, i.smtp_username, i.smtp_password, i.api_token,
	i.retention_hours, i.created_at, i.chaos, i.spam_check, i.allowed_hosts, i.link_domains,
	i.allowed_recipients, i.relay_recipients,`

const inboxCols = inboxBaseCols + `
	(SELECT count(*) FROM messages m WHERE m.inbox_id = i.id),
	(SELECT count(*) FROM messages m WHERE m.inbox_id = i.id AND NOT m.is_read),
	(SELECT max(received_at) FROM messages m WHERE m.inbox_id = i.id)`

// inboxLiteCols skips the per-inbox counters (used on hot paths: SMTP AUTH, API tokens).
const inboxLiteCols = inboxBaseCols + ` 0, 0, NULL::timestamptz`

func scanInbox(row interface{ Scan(...any) error }) (*Inbox, error) {
	in := &Inbox{}
	err := row.Scan(&in.ID, &in.Name, &in.Slug, &in.Description, &in.SMTPUsername, &in.SMTPPassword,
		&in.APIToken, &in.RetentionHours, &in.CreatedAt, &in.Chaos, &in.SpamCheck, &in.AllowedHosts, &in.LinkDomains,
		&in.AllowedRecipients, &in.RelayRecipients, &in.MessageCount, &in.UnreadCount, &in.LastMessageAt)
	if err != nil {
		return nil, notFound(err)
	}
	return in, nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(s string) string {
	s = slugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "inbox-" + RandomToken(3)
	}
	return s
}

// ListInboxes returns all inboxes for admins, or only the member's inboxes otherwise.
func (s *Store) ListInboxes(ctx context.Context, u *User) ([]*Inbox, error) {
	q := `SELECT ` + inboxCols + ` FROM inboxes i`
	args := []any{}
	if u != nil && !u.IsAdmin() {
		q += ` WHERE i.id IN (SELECT inbox_id FROM inbox_members WHERE user_id = $1)`
		args = append(args, u.ID)
	}
	q += ` ORDER BY i.name`
	rows, err := s.DB.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Inbox{}
	for rows.Next() {
		in, err := scanInbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

func (s *Store) GetInbox(ctx context.Context, id string) (*Inbox, error) {
	return scanInbox(s.DB.QueryRow(ctx, `SELECT `+inboxCols+` FROM inboxes i WHERE i.id = $1`, id))
}

func (s *Store) GetInboxBySlug(ctx context.Context, slug string) (*Inbox, error) {
	return scanInbox(s.DB.QueryRow(ctx, `SELECT `+inboxLiteCols+` FROM inboxes i WHERE i.slug = $1`, slug))
}

// GetInboxLite loads an inbox without its message counters.
func (s *Store) GetInboxLite(ctx context.Context, id string) (*Inbox, error) {
	return scanInbox(s.DB.QueryRow(ctx, `SELECT `+inboxLiteCols+` FROM inboxes i WHERE i.id = $1`, id))
}

func (s *Store) InboxByAPIToken(ctx context.Context, token string) (*Inbox, error) {
	return scanInbox(s.DB.QueryRow(ctx, `SELECT `+inboxLiteCols+` FROM inboxes i WHERE i.api_token = $1`, token))
}

// InboxBySMTPCredentials validates SMTP AUTH credentials and returns the matching inbox.
func (s *Store) InboxBySMTPCredentials(ctx context.Context, username, password string) (*Inbox, error) {
	var id, pw string
	err := s.DB.QueryRow(ctx, `SELECT id, smtp_password FROM inboxes WHERE smtp_username = $1`, username).Scan(&id, &pw)
	if err != nil {
		return nil, notFound(err)
	}
	if subtle.ConstantTimeCompare([]byte(pw), []byte(password)) != 1 {
		return nil, ErrNotFound
	}
	return s.GetInboxLite(ctx, id)
}

type InboxInput struct {
	Name           string
	Slug           string
	Description    string
	SMTPUsername   string
	SMTPPassword   string
	RetentionHours *int
}

func (s *Store) CreateInbox(ctx context.Context, in InboxInput) (*Inbox, error) {
	if in.Slug == "" {
		in.Slug = Slugify(in.Name)
	}
	if in.SMTPUsername == "" {
		in.SMTPUsername = in.Slug
	}
	if in.SMTPPassword == "" {
		in.SMTPPassword = RandomToken(12)
	}
	var id string
	err := s.DB.QueryRow(ctx, `INSERT INTO inboxes (name, slug, description, smtp_username, smtp_password, api_token, retention_hours)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		in.Name, in.Slug, in.Description, in.SMTPUsername, in.SMTPPassword, "pd_"+RandomToken(20), in.RetentionHours).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetInbox(ctx, id)
}

type InboxUpdate struct {
	Name           *string
	Description    *string
	SMTPUsername   *string
	SMTPPassword   *string
	RetentionHours **int
}

func (s *Store) UpdateInbox(ctx context.Context, id string, u InboxUpdate) (*Inbox, error) {
	sets := []string{}
	args := []any{id}
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, col+" = $"+itoa(len(args)))
	}
	if u.Name != nil {
		add("name", *u.Name)
	}
	if u.Description != nil {
		add("description", *u.Description)
	}
	if u.SMTPUsername != nil {
		add("smtp_username", *u.SMTPUsername)
	}
	if u.SMTPPassword != nil {
		add("smtp_password", *u.SMTPPassword)
	}
	if u.RetentionHours != nil {
		add("retention_hours", *u.RetentionHours)
	}
	if len(sets) > 0 {
		tag, err := s.DB.Exec(ctx, `UPDATE inboxes SET `+strings.Join(sets, ", ")+` WHERE id = $1`, args...)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			return nil, ErrNotFound
		}
	}
	return s.GetInbox(ctx, id)
}

// RotateInboxSecrets regenerates the SMTP password and the API token.
func (s *Store) RotateInboxSecrets(ctx context.Context, id string) (*Inbox, error) {
	tag, err := s.DB.Exec(ctx, `UPDATE inboxes SET smtp_password=$2, api_token=$3 WHERE id=$1`,
		id, RandomToken(12), "pd_"+RandomToken(20))
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetInbox(ctx, id)
}

func (s *Store) DeleteInbox(ctx context.Context, id string) error {
	tag, err := s.DB.Exec(ctx, `DELETE FROM inboxes WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) InboxMemberIDs(ctx context.Context, inboxID string) ([]string, error) {
	rows, err := s.DB.Query(ctx, `SELECT user_id::text FROM inbox_members WHERE inbox_id=$1`, inboxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) SetInboxMembers(ctx context.Context, inboxID string, userIDs []string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM inbox_members WHERE inbox_id=$1`, inboxID); err != nil {
		return err
	}
	for _, uid := range userIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO inbox_members (inbox_id, user_id) VALUES ($1,$2)`, inboxID, uid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ChaosRule makes an SMTP stage fail with the given reply code, `Probability` percent of the time.
type ChaosRule struct {
	Code        int    `json:"code"`
	Probability int    `json:"probability"`
	Message     string `json:"message,omitempty"`
}

// Chaos holds the fault-injection rules per SMTP stage.
type Chaos struct {
	Auth *ChaosRule `json:"auth,omitempty"`
	Mail *ChaosRule `json:"mail,omitempty"`
	Rcpt *ChaosRule `json:"rcpt,omitempty"`
	Data *ChaosRule `json:"data,omitempty"`
}

// Settings are the per-inbox options beyond identity and credentials.
type Settings struct {
	Chaos             Chaos    `json:"chaos"`
	SpamCheck         bool     `json:"spam_check"`
	AllowedHosts      []string `json:"allowed_hosts"`
	LinkDomains       []string `json:"link_domains"`
	AllowedRecipients string   `json:"allowed_recipients"`
	RelayRecipients   string   `json:"relay_recipients"`
}

// SettingsUpdate changes only the non-nil fields.
type SettingsUpdate struct {
	SpamCheck         *bool
	AllowedHosts      *[]string
	LinkDomains       *[]string
	AllowedRecipients *string
	RelayRecipients   *string
}

func (s *Store) UpdateInboxSettings(ctx context.Context, id string, u SettingsUpdate) (*Inbox, error) {
	sets := []string{}
	args := []any{id}
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, col+" = $"+itoa(len(args)))
	}
	if u.SpamCheck != nil {
		add("spam_check", *u.SpamCheck)
	}
	if u.AllowedHosts != nil {
		add("allowed_hosts", *u.AllowedHosts)
	}
	if u.LinkDomains != nil {
		add("link_domains", *u.LinkDomains)
	}
	if u.AllowedRecipients != nil {
		add("allowed_recipients", *u.AllowedRecipients)
	}
	if u.RelayRecipients != nil {
		add("relay_recipients", *u.RelayRecipients)
	}
	if len(sets) > 0 {
		tag, err := s.DB.Exec(ctx, `UPDATE inboxes SET `+strings.Join(sets, ", ")+` WHERE id = $1`, args...)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			return nil, ErrNotFound
		}
	}
	return s.GetInbox(ctx, id)
}

func (s *Store) SetChaos(ctx context.Context, id string, c Chaos) (*Inbox, error) {
	tag, err := s.DB.Exec(ctx, `UPDATE inboxes SET chaos = $2 WHERE id = $1`, id, c)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetInbox(ctx, id)
}
