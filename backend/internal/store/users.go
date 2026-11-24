package store

import (
	"context"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Role        string     `json:"role"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
	InboxIDs    []string   `json:"inbox_ids"`
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("postdream-dummy"), bcrypt.DefaultCost)

func (u *User) IsAdmin() bool { return u.Role == "admin" }

const userCols = `u.id, u.email, u.name, u.role, u.created_at, u.last_login_at,
	COALESCE((SELECT array_agg(m.inbox_id::text) FROM inbox_members m WHERE m.user_id = u.id), '{}')`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt, &u.LastLoginAt, &u.InboxIDs); err != nil {
		return nil, notFound(err)
	}
	return u, nil
}

func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(ctx context.Context, email, name, password, role string) (*User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	var id string
	err = s.DB.QueryRow(ctx, `INSERT INTO users (email, name, password_hash, role) VALUES ($1,$2,$3,$4) RETURNING id`,
		strings.ToLower(strings.TrimSpace(email)), name, hash, role).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetUser(ctx, id)
}

func (s *Store) GetUser(ctx context.Context, id string) (*User, error) {
	return scanUser(s.DB.QueryRow(ctx, `SELECT `+userCols+` FROM users u WHERE u.id = $1`, id))
}

func (s *Store) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+userCols+` FROM users u ORDER BY u.email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Authenticate checks credentials and returns the user on success.
func (s *Store) Authenticate(ctx context.Context, email, password string) (*User, error) {
	var id, hash string
	err := s.DB.QueryRow(ctx, `SELECT id, password_hash FROM users WHERE email = $1`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&id, &hash)
	if err != nil {
		// Burn comparable time to avoid user enumeration through timing.
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, ErrNotFound
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, ErrNotFound
	}
	s.DB.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, id)
	return s.GetUser(ctx, id)
}

func (s *Store) CheckPassword(ctx context.Context, userID, password string) bool {
	var hash string
	if err := s.DB.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, userID).Scan(&hash); err != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

type UserUpdate struct {
	Name     *string
	Role     *string
	Password *string
	InboxIDs *[]string
}

func (s *Store) UpdateUser(ctx context.Context, id string, upd UserUpdate) (*User, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if upd.Name != nil {
		if _, err := tx.Exec(ctx, `UPDATE users SET name=$2 WHERE id=$1`, id, *upd.Name); err != nil {
			return nil, err
		}
	}
	if upd.Role != nil {
		if _, err := tx.Exec(ctx, `UPDATE users SET role=$2 WHERE id=$1`, id, *upd.Role); err != nil {
			return nil, err
		}
	}
	if upd.Password != nil {
		hash, err := HashPassword(*upd.Password)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1`, id, hash); err != nil {
			return nil, err
		}
		// Invalidate other sessions on password change.
		if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, id); err != nil {
			return nil, err
		}
	}
	if upd.InboxIDs != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM inbox_members WHERE user_id=$1`, id); err != nil {
			return nil, err
		}
		for _, inboxID := range *upd.InboxIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO inbox_members (inbox_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, inboxID, id); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetUser(ctx, id)
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	tag, err := s.DB.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='admin'`).Scan(&n)
	return n, err
}

// --- sessions ---

func (s *Store) CreateSession(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	token := RandomToken(32)
	_, err := s.DB.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1,$2,$3)`,
		HashToken(token), userID, time.Now().Add(ttl))
	return token, err
}

func (s *Store) UserBySession(ctx context.Context, token string) (*User, error) {
	return scanUser(s.DB.QueryRow(ctx, `SELECT `+userCols+` FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, HashToken(token)))
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, HashToken(token))
	return err
}

func (s *Store) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := s.DB.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	return tag.RowsAffected(), err
}
