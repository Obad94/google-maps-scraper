package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/gosom/google-maps-scraper/web"
)

// Ensure userRepository implements web.UserRepository
var _ web.UserRepository = (*userRepository)(nil)

type userRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new SQLite user repository
func NewUserRepository(db *sql.DB) web.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Get(ctx context.Context, id string) (web.User, error) {
	const q = `SELECT id, email, password_hash, first_name, last_name, avatar_url, email_verified, status, created_at, updated_at, last_login_at, deleted_at FROM users WHERE id = ? AND deleted_at IS NULL`

	row := r.db.QueryRowContext(ctx, q, id)
	return scanUser(row)
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (web.User, error) {
	const q = `SELECT id, email, password_hash, first_name, last_name, avatar_url, email_verified, status, created_at, updated_at, last_login_at, deleted_at FROM users WHERE email = ? AND deleted_at IS NULL`

	row := r.db.QueryRowContext(ctx, q, email)
	return scanUser(row)
}

func (r *userRepository) Create(ctx context.Context, user *web.User) error {
	const q = `INSERT INTO users (id, email, password_hash, first_name, last_name, avatar_url, email_verified, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, q,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.FirstName,
		user.LastName,
		user.AvatarURL,
		user.EmailVerified,
		user.Status,
		user.CreatedAt.Unix(),
		user.UpdatedAt.Unix(),
	)
	return err
}

func (r *userRepository) Update(ctx context.Context, user *web.User) error {
	const q = `UPDATE users SET email = ?, password_hash = ?, first_name = ?, last_name = ?, avatar_url = ?, email_verified = ?, status = ?, updated_at = ?, last_login_at = ? WHERE id = ? AND deleted_at IS NULL`

	var lastLoginAt *int64
	if user.LastLoginAt != nil {
		ts := user.LastLoginAt.Unix()
		lastLoginAt = &ts
	}

	_, err := r.db.ExecContext(ctx, q,
		user.Email,
		user.PasswordHash,
		user.FirstName,
		user.LastName,
		user.AvatarURL,
		user.EmailVerified,
		user.Status,
		user.UpdatedAt.Unix(),
		lastLoginAt,
		user.ID,
	)
	return err
}

func (r *userRepository) Delete(ctx context.Context, id string) error {
	// Soft delete
	const q = `UPDATE users SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`
	_, err := r.db.ExecContext(ctx, q, time.Now().Unix(), id)
	return err
}

func (r *userRepository) Select(ctx context.Context, params web.UserSelectParams) ([]web.User, error) {
	q := `SELECT id, email, password_hash, first_name, last_name, avatar_url, email_verified, status, created_at, updated_at, last_login_at, deleted_at FROM users WHERE deleted_at IS NULL`
	var args []any

	if params.Status != "" {
		q += ` AND status = ?`
		args = append(args, params.Status)
	}

	if params.Email != "" {
		q += ` AND email LIKE ?`
		args = append(args, "%"+params.Email+"%")
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

	var users []web.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

func scanUser(row scannable) (web.User, error) {
	var user web.User
	var createdAt, updatedAt int64
	var lastLoginAt, deletedAt sql.NullInt64
	var avatarURL sql.NullString

	err := row.Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FirstName,
		&user.LastName,
		&avatarURL,
		&user.EmailVerified,
		&user.Status,
		&createdAt,
		&updatedAt,
		&lastLoginAt,
		&deletedAt,
	)
	if err != nil {
		return web.User{}, err
	}

	user.CreatedAt = time.Unix(createdAt, 0).UTC()
	user.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	
	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	}
	
	if lastLoginAt.Valid {
		t := time.Unix(lastLoginAt.Int64, 0).UTC()
		user.LastLoginAt = &t
	}
	
	if deletedAt.Valid {
		t := time.Unix(deletedAt.Int64, 0).UTC()
		user.DeletedAt = &t
	}

	return user, nil
}
