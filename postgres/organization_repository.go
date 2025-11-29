package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gosom/google-maps-scraper/web"
)

var _ web.OrganizationRepository = (*organizationRepository)(nil)

type organizationRepository struct {
	db *sql.DB
}

func NewOrganizationRepository(db *sql.DB) web.OrganizationRepository {
	return &organizationRepository{db: db}
}

func (r *organizationRepository) Get(ctx context.Context, id string) (web.Organization, error) {
	q := `
		SELECT id, name, slug, description, status, settings, created_at, updated_at, deleted_at
		FROM organizations
		WHERE id = $1 AND deleted_at IS NULL
	`

	var org web.Organization
	var settingsJSON []byte
	var deletedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.Description,
		&org.Status,
		&settingsJSON,
		&org.CreatedAt,
		&org.UpdatedAt,
		&deletedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return web.Organization{}, fmt.Errorf("organization not found: %w", err)
		}
		return web.Organization{}, fmt.Errorf("failed to get organization: %w", err)
	}

	if deletedAt.Valid {
		org.DeletedAt = &deletedAt.Time
	}

	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &org.Settings); err != nil {
			return web.Organization{}, fmt.Errorf("failed to unmarshal settings: %w", err)
		}
	}

	return org, nil
}

func (r *organizationRepository) GetBySlug(ctx context.Context, slug string) (web.Organization, error) {
	q := `
		SELECT id, name, slug, description, status, settings, created_at, updated_at, deleted_at
		FROM organizations
		WHERE slug = $1 AND deleted_at IS NULL
	`

	var org web.Organization
	var settingsJSON []byte
	var deletedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, q, slug).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.Description,
		&org.Status,
		&settingsJSON,
		&org.CreatedAt,
		&org.UpdatedAt,
		&deletedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return web.Organization{}, fmt.Errorf("organization not found: %w", err)
		}
		return web.Organization{}, fmt.Errorf("failed to get organization: %w", err)
	}

	if deletedAt.Valid {
		org.DeletedAt = &deletedAt.Time
	}

	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &org.Settings); err != nil {
			return web.Organization{}, fmt.Errorf("failed to unmarshal settings: %w", err)
		}
	}

	return org, nil
}

func (r *organizationRepository) Create(ctx context.Context, org *web.Organization) error {
	if err := org.Validate(); err != nil {
		return fmt.Errorf("invalid organization: %w", err)
	}

	settingsJSON, err := json.Marshal(org.Settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	q := `
		INSERT INTO organizations (id, name, slug, description, status, settings, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err = r.db.ExecContext(ctx, q,
		org.ID,
		org.Name,
		org.Slug,
		org.Description,
		org.Status,
		settingsJSON,
		org.CreatedAt,
		org.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}

	return nil
}

func (r *organizationRepository) Update(ctx context.Context, org *web.Organization) error {
	if err := org.Validate(); err != nil {
		return fmt.Errorf("invalid organization: %w", err)
	}

	settingsJSON, err := json.Marshal(org.Settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	q := `
		UPDATE organizations
		SET name = $2, slug = $3, description = $4, status = $5, settings = $6, updated_at = $7
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, q,
		org.ID,
		org.Name,
		org.Slug,
		org.Description,
		org.Status,
		settingsJSON,
		org.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update organization: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("organization not found")
	}

	return nil
}

func (r *organizationRepository) Delete(ctx context.Context, id string) error {
	// Soft delete
	q := `
		UPDATE organizations
		SET deleted_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("organization not found")
	}

	return nil
}

func (r *organizationRepository) Select(ctx context.Context, params web.OrganizationSelectParams) ([]web.Organization, error) {
	q := `
		SELECT id, name, slug, description, status, settings, created_at, updated_at, deleted_at
		FROM organizations
		WHERE deleted_at IS NULL
	`

	args := []interface{}{}
	argCount := 1

	if params.Status != "" {
		q += fmt.Sprintf(" AND status = $%d", argCount)
		args = append(args, params.Status)
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
		return nil, fmt.Errorf("failed to select organizations: %w", err)
	}
	defer rows.Close()

	var organizations []web.Organization

	for rows.Next() {
		var org web.Organization
		var settingsJSON []byte
		var deletedAt sql.NullTime

		err := rows.Scan(
			&org.ID,
			&org.Name,
			&org.Slug,
			&org.Description,
			&org.Status,
			&settingsJSON,
			&org.CreatedAt,
			&org.UpdatedAt,
			&deletedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan organization: %w", err)
		}

		if deletedAt.Valid {
			org.DeletedAt = &deletedAt.Time
		}

		if len(settingsJSON) > 0 {
			if err := json.Unmarshal(settingsJSON, &org.Settings); err != nil {
				return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
			}
		}

		organizations = append(organizations, org)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate organizations: %w", err)
	}

	return organizations, nil
}
