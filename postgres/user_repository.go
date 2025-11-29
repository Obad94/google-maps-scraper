package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gosom/google-maps-scraper/web"
)

var _ web.UserRepository = (*userRepository)(nil)

type userRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) web.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Get(ctx context.Context, id string) (web.User, error) {
	q := `
		SELECT id, email, password_hash, first_name, last_name, avatar_url,
		       email_verified, status, last_login_at, created_at, updated_at, deleted_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`

	var user web.User
	var lastLoginAt, deletedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FirstName,
		&user.LastName,
		&user.AvatarURL,
		&user.EmailVerified,
		&user.Status,
		&lastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&deletedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return web.User{}, fmt.Errorf("user not found: %w", err)
		}
		return web.User{}, fmt.Errorf("failed to get user: %w", err)
	}

	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	if deletedAt.Valid {
		user.DeletedAt = &deletedAt.Time
	}

	return user, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (web.User, error) {
	q := `
		SELECT id, email, password_hash, first_name, last_name, avatar_url,
		       email_verified, status, last_login_at, created_at, updated_at, deleted_at
		FROM users
		WHERE email = $1 AND deleted_at IS NULL
	`

	var user web.User
	var lastLoginAt, deletedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, q, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FirstName,
		&user.LastName,
		&user.AvatarURL,
		&user.EmailVerified,
		&user.Status,
		&lastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&deletedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return web.User{}, fmt.Errorf("user not found: %w", err)
		}
		return web.User{}, fmt.Errorf("failed to get user: %w", err)
	}

	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	if deletedAt.Valid {
		user.DeletedAt = &deletedAt.Time
	}

	return user, nil
}

func (r *userRepository) Create(ctx context.Context, user *web.User) error {
	if err := user.Validate(); err != nil {
		return fmt.Errorf("invalid user: %w", err)
	}

	q := `
		INSERT INTO users (id, email, password_hash, first_name, last_name, avatar_url,
		                   email_verified, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := r.db.ExecContext(ctx, q,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.FirstName,
		user.LastName,
		user.AvatarURL,
		user.EmailVerified,
		user.Status,
		user.CreatedAt,
		user.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (r *userRepository) Update(ctx context.Context, user *web.User) error {
	if err := user.Validate(); err != nil {
		return fmt.Errorf("invalid user: %w", err)
	}

	q := `
		UPDATE users
		SET email = $2, password_hash = $3, first_name = $4, last_name = $5,
		    avatar_url = $6, email_verified = $7, status = $8, last_login_at = $9,
		    updated_at = $10
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, q,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.FirstName,
		user.LastName,
		user.AvatarURL,
		user.EmailVerified,
		user.Status,
		user.LastLoginAt,
		user.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

func (r *userRepository) Delete(ctx context.Context, id string) error {
	// Soft delete
	q := `
		UPDATE users
		SET deleted_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

func (r *userRepository) Select(ctx context.Context, params web.UserSelectParams) ([]web.User, error) {
	q := `
		SELECT id, email, password_hash, first_name, last_name, avatar_url,
		       email_verified, status, last_login_at, created_at, updated_at, deleted_at
		FROM users
		WHERE deleted_at IS NULL
	`

	args := []interface{}{}
	argCount := 1

	if params.Email != "" {
		q += fmt.Sprintf(" AND email ILIKE $%d", argCount)
		args = append(args, "%"+params.Email+"%")
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
		return nil, fmt.Errorf("failed to select users: %w", err)
	}
	defer rows.Close()

	var users []web.User

	for rows.Next() {
		var user web.User
		var lastLoginAt, deletedAt sql.NullTime

		err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.PasswordHash,
			&user.FirstName,
			&user.LastName,
			&user.AvatarURL,
			&user.EmailVerified,
			&user.Status,
			&lastLoginAt,
			&user.CreatedAt,
			&user.UpdatedAt,
			&deletedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		if lastLoginAt.Valid {
			user.LastLoginAt = &lastLoginAt.Time
		}

		if deletedAt.Valid {
			user.DeletedAt = &deletedAt.Time
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate users: %w", err)
	}

	return users, nil
}
