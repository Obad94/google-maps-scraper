package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gosom/google-maps-scraper/web"
)

var _ web.OrganizationMemberRepository = (*organizationMemberRepository)(nil)

type organizationMemberRepository struct {
	db *sql.DB
}

func NewOrganizationMemberRepository(db *sql.DB) web.OrganizationMemberRepository {
	return &organizationMemberRepository{db: db}
}

func (r *organizationMemberRepository) Get(ctx context.Context, id string) (web.OrganizationMember, error) {
	q := `
		SELECT id, organization_id, user_id, role, invited_by, joined_at, created_at, updated_at
		FROM organization_members
		WHERE id = $1
	`

	var member web.OrganizationMember
	var invitedBy sql.NullString

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&member.ID,
		&member.OrganizationID,
		&member.UserID,
		&member.Role,
		&invitedBy,
		&member.JoinedAt,
		&member.CreatedAt,
		&member.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return web.OrganizationMember{}, fmt.Errorf("member not found: %w", err)
		}
		return web.OrganizationMember{}, fmt.Errorf("failed to get member: %w", err)
	}

	if invitedBy.Valid {
		member.InvitedBy = &invitedBy.String
	}

	return member, nil
}

func (r *organizationMemberRepository) GetByOrganizationAndUser(ctx context.Context, orgID, userID string) (web.OrganizationMember, error) {
	q := `
		SELECT id, organization_id, user_id, role, invited_by, joined_at, created_at, updated_at
		FROM organization_members
		WHERE organization_id = $1 AND user_id = $2
	`

	var member web.OrganizationMember
	var invitedBy sql.NullString

	err := r.db.QueryRowContext(ctx, q, orgID, userID).Scan(
		&member.ID,
		&member.OrganizationID,
		&member.UserID,
		&member.Role,
		&invitedBy,
		&member.JoinedAt,
		&member.CreatedAt,
		&member.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return web.OrganizationMember{}, fmt.Errorf("member not found: %w", err)
		}
		return web.OrganizationMember{}, fmt.Errorf("failed to get member: %w", err)
	}

	if invitedBy.Valid {
		member.InvitedBy = &invitedBy.String
	}

	return member, nil
}

func (r *organizationMemberRepository) Create(ctx context.Context, member *web.OrganizationMember) error {
	if err := member.Validate(); err != nil {
		return fmt.Errorf("invalid member: %w", err)
	}

	q := `
		INSERT INTO organization_members (id, organization_id, user_id, role, invited_by, joined_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.ExecContext(ctx, q,
		member.ID,
		member.OrganizationID,
		member.UserID,
		member.Role,
		member.InvitedBy,
		member.JoinedAt,
		member.CreatedAt,
		member.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create member: %w", err)
	}

	return nil
}

func (r *organizationMemberRepository) Update(ctx context.Context, member *web.OrganizationMember) error {
	if err := member.Validate(); err != nil {
		return fmt.Errorf("invalid member: %w", err)
	}

	q := `
		UPDATE organization_members
		SET role = $2, updated_at = $3
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, q,
		member.ID,
		member.Role,
		member.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update member: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("member not found")
	}

	return nil
}

func (r *organizationMemberRepository) Delete(ctx context.Context, id string) error {
	q := `DELETE FROM organization_members WHERE id = $1`

	result, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("failed to delete member: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("member not found")
	}

	return nil
}

func (r *organizationMemberRepository) Select(ctx context.Context, params web.OrganizationMemberSelectParams) ([]web.OrganizationMember, error) {
	q := `
		SELECT om.id, om.organization_id, om.user_id, om.role, om.invited_by, om.joined_at, om.created_at, om.updated_at,
		       u.id, u.email, u.first_name, u.last_name, u.avatar_url, u.status
		FROM organization_members om
		LEFT JOIN users u ON om.user_id = u.id
		WHERE 1=1
	`

	args := []interface{}{}
	argCount := 1

	if params.OrganizationID != "" {
		q += fmt.Sprintf(" AND om.organization_id = $%d", argCount)
		args = append(args, params.OrganizationID)
		argCount++
	}

	if params.UserID != "" {
		q += fmt.Sprintf(" AND om.user_id = $%d", argCount)
		args = append(args, params.UserID)
		argCount++
	}

	if params.Role != "" {
		q += fmt.Sprintf(" AND om.role = $%d", argCount)
		args = append(args, params.Role)
		argCount++
	}

	q += " ORDER BY om.joined_at DESC"

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
		return nil, fmt.Errorf("failed to select members: %w", err)
	}
	defer rows.Close()

	var members []web.OrganizationMember

	for rows.Next() {
		var member web.OrganizationMember
		var invitedBy sql.NullString
		var user web.User

		err := rows.Scan(
			&member.ID,
			&member.OrganizationID,
			&member.UserID,
			&member.Role,
			&invitedBy,
			&member.JoinedAt,
			&member.CreatedAt,
			&member.UpdatedAt,
			&user.ID,
			&user.Email,
			&user.FirstName,
			&user.LastName,
			&user.AvatarURL,
			&user.Status,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}

		if invitedBy.Valid {
			member.InvitedBy = &invitedBy.String
		}

		member.User = &user

		members = append(members, member)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate members: %w", err)
	}

	return members, nil
}

func (r *organizationMemberRepository) GetUserOrganizations(ctx context.Context, userID string) ([]web.Organization, error) {
	q := `
		SELECT o.id, o.name, o.slug, o.description, o.status, o.settings, o.created_at, o.updated_at, o.deleted_at
		FROM organizations o
		INNER JOIN organization_members om ON o.id = om.organization_id
		WHERE om.user_id = $1 AND o.deleted_at IS NULL
		ORDER BY o.created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user organizations: %w", err)
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

		organizations = append(organizations, org)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate organizations: %w", err)
	}

	return organizations, nil
}

func (r *organizationMemberRepository) CountByOrganization(ctx context.Context, orgID string) (int, error) {
	q := `SELECT COUNT(*) FROM organization_members WHERE organization_id = $1`

	var count int
	err := r.db.QueryRowContext(ctx, q, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count members: %w", err)
	}

	return count, nil
}
