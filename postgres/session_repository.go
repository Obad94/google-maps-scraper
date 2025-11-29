package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gosom/google-maps-scraper/web"
)

var _ web.UserSessionRepository = (*sessionRepository)(nil)

type sessionRepository struct {
	db *sql.DB
}

func NewUserSessionRepository(db *sql.DB) web.UserSessionRepository {
	return &sessionRepository{db: db}
}

func (r *sessionRepository) Get(ctx context.Context, id string) (web.UserSession, error) {
	q := `
		SELECT id, user_id, token_hash, expires_at, created_at, last_used_at
		FROM user_sessions
		WHERE id = $1
	`

	var session web.UserSession
	var lastUsedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&session.ExpiresAt,
		&session.CreatedAt,
		&lastUsedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return web.UserSession{}, fmt.Errorf("session not found: %w", err)
		}
		return web.UserSession{}, fmt.Errorf("failed to get session: %w", err)
	}

	if lastUsedAt.Valid {
		session.LastUsedAt = &lastUsedAt.Time
	}

	return session, nil
}

func (r *sessionRepository) GetByToken(ctx context.Context, tokenHash string) (web.UserSession, error) {
	q := `
		SELECT id, user_id, token_hash, expires_at, created_at, last_used_at
		FROM user_sessions
		WHERE token_hash = $1
	`

	var session web.UserSession
	var lastUsedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, q, tokenHash).Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&session.ExpiresAt,
		&session.CreatedAt,
		&lastUsedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return web.UserSession{}, fmt.Errorf("session not found: %w", err)
		}
		return web.UserSession{}, fmt.Errorf("failed to get session: %w", err)
	}

	if lastUsedAt.Valid {
		session.LastUsedAt = &lastUsedAt.Time
	}

	return session, nil
}

func (r *sessionRepository) Create(ctx context.Context, session *web.UserSession) error {
	if err := session.Validate(); err != nil {
		return fmt.Errorf("invalid session: %w", err)
	}

	q := `
		INSERT INTO user_sessions (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := r.db.ExecContext(ctx, q,
		session.ID,
		session.UserID,
		session.TokenHash,
		session.ExpiresAt,
		session.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

func (r *sessionRepository) Update(ctx context.Context, session *web.UserSession) error {
	if err := session.Validate(); err != nil {
		return fmt.Errorf("invalid session: %w", err)
	}

	q := `
		UPDATE user_sessions
		SET last_used_at = $2
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, q, session.ID, session.LastUsedAt)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found")
	}

	return nil
}

func (r *sessionRepository) Delete(ctx context.Context, id string) error {
	q := `DELETE FROM user_sessions WHERE id = $1`

	result, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found")
	}

	return nil
}

func (r *sessionRepository) DeleteByUser(ctx context.Context, userID string) error {
	q := `DELETE FROM user_sessions WHERE user_id = $1`

	_, err := r.db.ExecContext(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}

	return nil
}

func (r *sessionRepository) Select(ctx context.Context, params web.UserSessionSelectParams) ([]web.UserSession, error) {
	q := `
		SELECT id, user_id, token_hash, expires_at, created_at, last_used_at
		FROM user_sessions
		WHERE 1=1
	`

	args := []interface{}{}
	argCount := 1

	if params.UserID != "" {
		q += fmt.Sprintf(" AND user_id = $%d", argCount)
		args = append(args, params.UserID)
		argCount++
	}

	q += " ORDER BY created_at DESC"

	if params.Limit > 0 {
		q += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, params.Limit)
		argCount++
	}

	if params.Offset > 0 {
		q += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, params.Offset)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to select sessions: %w", err)
	}
	defer rows.Close()

	var sessions []web.UserSession

	for rows.Next() {
		var session web.UserSession
		var lastUsedAt sql.NullTime

		err := rows.Scan(
			&session.ID,
			&session.UserID,
			&session.TokenHash,
			&session.ExpiresAt,
			&session.CreatedAt,
			&lastUsedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		if lastUsedAt.Valid {
			session.LastUsedAt = &lastUsedAt.Time
		}

		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate sessions: %w", err)
	}

	return sessions, nil
}

func (r *sessionRepository) CleanupExpired(ctx context.Context) error {
	q := `DELETE FROM user_sessions WHERE expires_at < $1`

	_, err := r.db.ExecContext(ctx, q, time.Now())
	if err != nil {
		return fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}

	return nil
}
