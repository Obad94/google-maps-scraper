package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gosom/google-maps-scraper/web"
)

type apiKeyRepo struct {
	db *sql.DB
}

func NewAPIKeyRepository(db *sql.DB) web.APIKeyRepository {
	return &apiKeyRepo{db: db}
}

func (r *apiKeyRepo) Get(ctx context.Context, id string) (web.APIKey, error) {
	const q = `SELECT id, name, key_hash, status, created_at, updated_at, last_used_at, expires_at
		FROM api_keys WHERE id = $1`

	row := r.db.QueryRowContext(ctx, q, id)

	return rowToAPIKey(row)
}

func (r *apiKeyRepo) GetByKey(ctx context.Context, keyHash string) (web.APIKey, error) {
	const q = `SELECT id, name, key_hash, status, created_at, updated_at, last_used_at, expires_at
		FROM api_keys WHERE key_hash = $1`

	row := r.db.QueryRowContext(ctx, q, keyHash)

	return rowToAPIKey(row)
}

func (r *apiKeyRepo) Create(ctx context.Context, apiKey *web.APIKey) error {
	const q = `INSERT INTO api_keys
		(id, name, key_hash, status, created_at, updated_at, last_used_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	var lastUsedAt, expiresAt *time.Time
	if apiKey.LastUsedAt != nil {
		lastUsedAt = apiKey.LastUsedAt
	}
	if apiKey.ExpiresAt != nil {
		expiresAt = apiKey.ExpiresAt
	}

	_, err := r.db.ExecContext(ctx, q,
		apiKey.ID,
		apiKey.Name,
		apiKey.KeyHash,
		apiKey.Status,
		apiKey.CreatedAt,
		apiKey.UpdatedAt,
		lastUsedAt,
		expiresAt,
	)

	return err
}

func (r *apiKeyRepo) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM api_keys WHERE id = $1`

	_, err := r.db.ExecContext(ctx, q, id)

	return err
}

func (r *apiKeyRepo) Select(ctx context.Context, params web.APIKeySelectParams) ([]web.APIKey, error) {
	q := `SELECT id, name, key_hash, status, created_at, updated_at, last_used_at, expires_at
		FROM api_keys`

	var args []interface{}
	argCount := 1

	if params.Status != "" {
		q += fmt.Sprintf(` WHERE status = $%d`, argCount)
		args = append(args, params.Status)
		argCount++
	}

	q += " ORDER BY created_at DESC"

	if params.Limit > 0 {
		q += fmt.Sprintf(` LIMIT $%d`, argCount)
		args = append(args, params.Limit)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apiKeys []web.APIKey

	for rows.Next() {
		apiKey, err := rowToAPIKey(rows)
		if err != nil {
			return nil, err
		}

		apiKeys = append(apiKeys, apiKey)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return apiKeys, nil
}

func (r *apiKeyRepo) Update(ctx context.Context, apiKey *web.APIKey) error {
	const q = `UPDATE api_keys
		SET name = $1, key_hash = $2, status = $3, updated_at = $4, last_used_at = $5, expires_at = $6
		WHERE id = $7`

	var lastUsedAt, expiresAt *time.Time
	if apiKey.LastUsedAt != nil {
		lastUsedAt = apiKey.LastUsedAt
	}
	if apiKey.ExpiresAt != nil {
		expiresAt = apiKey.ExpiresAt
	}

	_, err := r.db.ExecContext(ctx, q,
		apiKey.Name,
		apiKey.KeyHash,
		apiKey.Status,
		time.Now().UTC(),
		lastUsedAt,
		expiresAt,
		apiKey.ID,
	)

	return err
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func rowToAPIKey(row scannable) (web.APIKey, error) {
	var apiKey web.APIKey
	var lastUsedAt, expiresAt sql.NullTime

	err := row.Scan(
		&apiKey.ID,
		&apiKey.Name,
		&apiKey.KeyHash,
		&apiKey.Status,
		&apiKey.CreatedAt,
		&apiKey.UpdatedAt,
		&lastUsedAt,
		&expiresAt,
	)
	if err != nil {
		return web.APIKey{}, err
	}

	if lastUsedAt.Valid {
		apiKey.LastUsedAt = &lastUsedAt.Time
	}
	if expiresAt.Valid {
		apiKey.ExpiresAt = &expiresAt.Time
	}

	return apiKey, nil
}
