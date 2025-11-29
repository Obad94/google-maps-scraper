package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/gosom/google-maps-scraper/web"
)

// Ensure sessionRepository implements web.UserSessionRepository
var _ web.UserSessionRepository = (*sessionRepository)(nil)

type sessionRepository struct {
	db *sql.DB
}

// NewSessionRepository creates a new SQLite session repository
func NewSessionRepository(db *sql.DB) web.UserSessionRepository {
	return &sessionRepository{db: db}
}

func (r *sessionRepository) Get(ctx context.Context, id string) (web.UserSession, error) {
	const q = `SELECT id, user_id, token_hash, expires_at, created_at, last_used_at FROM user_sessions WHERE id = ?`

	row := r.db.QueryRowContext(ctx, q, id)
	return scanSession(row)
}

func (r *sessionRepository) GetByToken(ctx context.Context, tokenHash string) (web.UserSession, error) {
	const q = `SELECT id, user_id, token_hash, expires_at, created_at, last_used_at FROM user_sessions WHERE token_hash = ?`

	row := r.db.QueryRowContext(ctx, q, tokenHash)
	return scanSession(row)
}

func (r *sessionRepository) Create(ctx context.Context, session *web.UserSession) error {
	const q = `INSERT INTO user_sessions (id, user_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, q,
		session.ID,
		session.UserID,
		session.TokenHash,
		session.ExpiresAt.Unix(),
		session.CreatedAt.Unix(),
	)
	return err
}

func (r *sessionRepository) Update(ctx context.Context, session *web.UserSession) error {
	const q = `UPDATE user_sessions SET expires_at = ?, last_used_at = ? WHERE id = ?`

	var lastUsedAt *int64
	if session.LastUsedAt != nil {
		ts := session.LastUsedAt.Unix()
		lastUsedAt = &ts
	}

	_, err := r.db.ExecContext(ctx, q,
		session.ExpiresAt.Unix(),
		lastUsedAt,
		session.ID,
	)
	return err
}

func (r *sessionRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM user_sessions WHERE id = ?`
	_, err := r.db.ExecContext(ctx, q, id)
	return err
}

func (r *sessionRepository) DeleteByUser(ctx context.Context, userID string) error {
	const q = `DELETE FROM user_sessions WHERE user_id = ?`
	_, err := r.db.ExecContext(ctx, q, userID)
	return err
}

func (r *sessionRepository) DeleteExpired(ctx context.Context) error {
	const q = `DELETE FROM user_sessions WHERE expires_at < ?`
	_, err := r.db.ExecContext(ctx, q, time.Now().Unix())
	return err
}

func (r *sessionRepository) CleanupExpired(ctx context.Context) error {
	return r.DeleteExpired(ctx)
}

func (r *sessionRepository) Select(ctx context.Context, params web.UserSessionSelectParams) ([]web.UserSession, error) {
	q := `SELECT id, user_id, token_hash, expires_at, created_at, last_used_at FROM user_sessions WHERE 1=1`
	var args []any

	if params.UserID != "" {
		q += ` AND user_id = ?`
		args = append(args, params.UserID)
	}

	q += " ORDER BY created_at DESC"

	if params.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, params.Limit)
	}

	if params.Offset > 0 {
		q += " OFFSET ?"
		args = append(args, params.Offset)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []web.UserSession
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	return sessions, rows.Err()
}

func scanSession(row scannable) (web.UserSession, error) {
	var session web.UserSession
	var expiresAt, createdAt int64
	var lastUsedAt sql.NullInt64

	err := row.Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&expiresAt,
		&createdAt,
		&lastUsedAt,
	)
	if err != nil {
		return web.UserSession{}, err
	}

	session.ExpiresAt = time.Unix(expiresAt, 0).UTC()
	session.CreatedAt = time.Unix(createdAt, 0).UTC()
	if lastUsedAt.Valid {
		t := time.Unix(lastUsedAt.Int64, 0).UTC()
		session.LastUsedAt = &t
	}

	return session, nil
}
