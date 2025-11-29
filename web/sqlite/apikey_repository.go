package sqlite

import (
	"context"
	"database/sql"
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
		FROM api_keys WHERE id = ?`

	row := r.db.QueryRowContext(ctx, q, id)

	return rowToAPIKey(row)
}

func (r *apiKeyRepo) GetByKey(ctx context.Context, keyHash string) (web.APIKey, error) {
	const q = `SELECT id, name, key_hash, status, created_at, updated_at, last_used_at, expires_at
		FROM api_keys WHERE key_hash = ?`

	row := r.db.QueryRowContext(ctx, q, keyHash)

	return rowToAPIKey(row)
}

func (r *apiKeyRepo) Create(ctx context.Context, apiKey *web.APIKey) error {
	const q = `INSERT INTO api_keys
		(id, name, key_hash, status, created_at, updated_at, last_used_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	var lastUsedAt, expiresAt *int64
	if apiKey.LastUsedAt != nil {
		ts := apiKey.LastUsedAt.Unix()
		lastUsedAt = &ts
	}
	if apiKey.ExpiresAt != nil {
		ts := apiKey.ExpiresAt.Unix()
		expiresAt = &ts
	}

	_, err := r.db.ExecContext(ctx, q,
		apiKey.ID,
		apiKey.Name,
		apiKey.KeyHash,
		apiKey.Status,
		apiKey.CreatedAt.Unix(),
		apiKey.UpdatedAt.Unix(),
		lastUsedAt,
		expiresAt,
	)

	return err
}

func (r *apiKeyRepo) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM api_keys WHERE id = ?`

	_, err := r.db.ExecContext(ctx, q, id)

	return err
}

func (r *apiKeyRepo) Select(ctx context.Context, params web.APIKeySelectParams) ([]web.APIKey, error) {
	q := `SELECT id, name, key_hash, status, created_at, updated_at, last_used_at, expires_at
		FROM api_keys`

	var args []interface{}

	if params.Status != "" {
		q += ` WHERE status = ?`
		args = append(args, params.Status)
	}

	q += " ORDER BY created_at DESC"

	if params.Limit > 0 {
		q += " LIMIT ?"
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
		SET name = ?, key_hash = ?, status = ?, updated_at = ?, last_used_at = ?, expires_at = ?
		WHERE id = ?`

	var lastUsedAt, expiresAt *int64
	if apiKey.LastUsedAt != nil {
		ts := apiKey.LastUsedAt.Unix()
		lastUsedAt = &ts
	}
	if apiKey.ExpiresAt != nil {
		ts := apiKey.ExpiresAt.Unix()
		expiresAt = &ts
	}

	_, err := r.db.ExecContext(ctx, q,
		apiKey.Name,
		apiKey.KeyHash,
		apiKey.Status,
		time.Now().UTC().Unix(),
		lastUsedAt,
		expiresAt,
		apiKey.ID,
	)

	return err
}

func rowToAPIKey(row scannable) (web.APIKey, error) {
	var apiKey web.APIKey
	var createdAt, updatedAt int64
	var lastUsedAt, expiresAt sql.NullInt64

	err := row.Scan(
		&apiKey.ID,
		&apiKey.Name,
		&apiKey.KeyHash,
		&apiKey.Status,
		&createdAt,
		&updatedAt,
		&lastUsedAt,
		&expiresAt,
	)
	if err != nil {
		return web.APIKey{}, err
	}

	// Convert Unix timestamps to time.Time
	apiKey.CreatedAt = time.Unix(createdAt, 0).UTC()
	apiKey.UpdatedAt = time.Unix(updatedAt, 0).UTC()

	if lastUsedAt.Valid {
		t := time.Unix(lastUsedAt.Int64, 0).UTC()
		apiKey.LastUsedAt = &t
	}
	if expiresAt.Valid {
		t := time.Unix(expiresAt.Int64, 0).UTC()
		apiKey.ExpiresAt = &t
	}

	return apiKey, nil
}
