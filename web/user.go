package web

import (
	"context"
	"errors"
	"time"
)

const (
	UserStatusActive   = "active"
	UserStatusInactive = "inactive"
	UserStatusSuspended = "suspended"
)

// User represents an individual user account
type User struct {
	ID            string
	Email         string
	PasswordHash  string
	FirstName     string
	LastName      string
	AvatarURL     string
	EmailVerified bool
	Status        string
	LastLoginAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time
}

func (u *User) Validate() error {
	if u.ID == "" {
		return errors.New("missing id")
	}

	if u.Email == "" {
		return errors.New("missing email")
	}

	if u.PasswordHash == "" {
		return errors.New("missing password hash")
	}

	if u.Status == "" {
		return errors.New("missing status")
	}

	if u.Status != UserStatusActive &&
		u.Status != UserStatusInactive &&
		u.Status != UserStatusSuspended {
		return errors.New("invalid status")
	}

	if u.CreatedAt.IsZero() {
		return errors.New("missing created_at")
	}

	return nil
}

func (u *User) IsActive() bool {
	return u.Status == UserStatusActive && u.DeletedAt == nil
}

func (u *User) FullName() string {
	if u.FirstName == "" && u.LastName == "" {
		return u.Email
	}

	if u.FirstName == "" {
		return u.LastName
	}

	if u.LastName == "" {
		return u.FirstName
	}

	return u.FirstName + " " + u.LastName
}

type UserSelectParams struct {
	Email  string
	Status string
	Limit  int
	Offset int
}

type UserRepository interface {
	Get(context.Context, string) (User, error)
	GetByEmail(context.Context, string) (User, error)
	Create(context.Context, *User) error
	Update(context.Context, *User) error
	Delete(context.Context, string) error
	Select(context.Context, UserSelectParams) ([]User, error)
}
