package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gosom/google-maps-scraper/web"
)

var _ web.OrganizationInvitationRepository = (*invitationRepository)(nil)

type invitationRepository struct {
	db *sql.DB
}

func NewOrganizationInvitationRepository(db *sql.DB) web.OrganizationInvitationRepository {
	return &invitationRepository{db: db}
}

func (r *invitationRepository) Get(ctx context.Context, id string) (web.OrganizationInvitation, error) {
	q := `
		SELECT id, organization_id, email, role, token_hash, invited_by, status, expires_at, accepted_at, created_at, updated_at
		FROM organization_invitations
		WHERE id = $1
	`

	var invitation web.OrganizationInvitation
	var acceptedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&invitation.ID,
		&invitation.OrganizationID,
		&invitation.Email,
		&invitation.Role,
		&invitation.TokenHash,
		&invitation.InvitedBy,
		&invitation.Status,
		&invitation.ExpiresAt,
		&acceptedAt,
		&invitation.CreatedAt,
		&invitation.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return web.OrganizationInvitation{}, fmt.Errorf("invitation not found: %w", err)
		}
		return web.OrganizationInvitation{}, fmt.Errorf("failed to get invitation: %w", err)
	}

	if acceptedAt.Valid {
		invitation.AcceptedAt = &acceptedAt.Time
	}

	return invitation, nil
}

func (r *invitationRepository) GetByToken(ctx context.Context, tokenHash string) (web.OrganizationInvitation, error) {
	q := `
		SELECT id, organization_id, email, role, token_hash, invited_by, status, expires_at, accepted_at, created_at, updated_at
		FROM organization_invitations
		WHERE token_hash = $1
	`

	var invitation web.OrganizationInvitation
	var acceptedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, q, tokenHash).Scan(
		&invitation.ID,
		&invitation.OrganizationID,
		&invitation.Email,
		&invitation.Role,
		&invitation.TokenHash,
		&invitation.InvitedBy,
		&invitation.Status,
		&invitation.ExpiresAt,
		&acceptedAt,
		&invitation.CreatedAt,
		&invitation.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return web.OrganizationInvitation{}, fmt.Errorf("invitation not found: %w", err)
		}
		return web.OrganizationInvitation{}, fmt.Errorf("failed to get invitation: %w", err)
	}

	if acceptedAt.Valid {
		invitation.AcceptedAt = &acceptedAt.Time
	}

	return invitation, nil
}

func (r *invitationRepository) Create(ctx context.Context, invitation *web.OrganizationInvitation) error {
	if err := invitation.Validate(); err != nil {
		return fmt.Errorf("invalid invitation: %w", err)
	}

	q := `
		INSERT INTO organization_invitations (id, organization_id, email, role, token_hash, invited_by, status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := r.db.ExecContext(ctx, q,
		invitation.ID,
		invitation.OrganizationID,
		invitation.Email,
		invitation.Role,
		invitation.TokenHash,
		invitation.InvitedBy,
		invitation.Status,
		invitation.ExpiresAt,
		invitation.CreatedAt,
		invitation.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create invitation: %w", err)
	}

	return nil
}

func (r *invitationRepository) Update(ctx context.Context, invitation *web.OrganizationInvitation) error {
	if err := invitation.Validate(); err != nil {
		return fmt.Errorf("invalid invitation: %w", err)
	}

	q := `
		UPDATE organization_invitations
		SET status = $2, accepted_at = $3, updated_at = $4
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, q,
		invitation.ID,
		invitation.Status,
		invitation.AcceptedAt,
		invitation.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update invitation: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("invitation not found")
	}

	return nil
}

func (r *invitationRepository) Delete(ctx context.Context, id string) error {
	q := `DELETE FROM organization_invitations WHERE id = $1`

	result, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("failed to delete invitation: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("invitation not found")
	}

	return nil
}

func (r *invitationRepository) Select(ctx context.Context, params web.InvitationSelectParams) ([]web.OrganizationInvitation, error) {
	q := `
		SELECT id, organization_id, email, role, token_hash, invited_by, status, expires_at, accepted_at, created_at, updated_at
		FROM organization_invitations
		WHERE 1=1
	`

	args := []interface{}{}
	argCount := 1

	if params.OrganizationID != "" {
		q += fmt.Sprintf(" AND organization_id = $%d", argCount)
		args = append(args, params.OrganizationID)
		argCount++
	}

	if params.Email != "" {
		q += fmt.Sprintf(" AND email = $%d", argCount)
		args = append(args, params.Email)
		argCount++
	}

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
		return nil, fmt.Errorf("failed to select invitations: %w", err)
	}
	defer rows.Close()

	var invitations []web.OrganizationInvitation

	for rows.Next() {
		var invitation web.OrganizationInvitation
		var acceptedAt sql.NullTime

		err := rows.Scan(
			&invitation.ID,
			&invitation.OrganizationID,
			&invitation.Email,
			&invitation.Role,
			&invitation.TokenHash,
			&invitation.InvitedBy,
			&invitation.Status,
			&invitation.ExpiresAt,
			&acceptedAt,
			&invitation.CreatedAt,
			&invitation.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan invitation: %w", err)
		}

		if acceptedAt.Valid {
			invitation.AcceptedAt = &acceptedAt.Time
		}

		invitations = append(invitations, invitation)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate invitations: %w", err)
	}

	return invitations, nil
}

func (r *invitationRepository) CleanupExpired(ctx context.Context) error {
	q := `
		UPDATE organization_invitations
		SET status = $1
		WHERE expires_at < $2 AND status = $3
	`

	_, err := r.db.ExecContext(ctx, q, web.InvitationStatusExpired, time.Now(), web.InvitationStatusPending)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired invitations: %w", err)
	}

	return nil
}
