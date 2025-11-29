package web

import (
	"context"
	"errors"
	"time"
)

// UserSession represents an active user session
type UserSession struct {
	ID         string
	UserID     string
	TokenHash  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastUsedAt *time.Time

	// Populated by joins
	User *User
}

func (s *UserSession) Validate() error {
	if s.ID == "" {
		return errors.New("missing id")
	}

	if s.UserID == "" {
		return errors.New("missing user_id")
	}

	if s.TokenHash == "" {
		return errors.New("missing token_hash")
	}

	if s.ExpiresAt.IsZero() {
		return errors.New("missing expires_at")
	}

	if s.CreatedAt.IsZero() {
		return errors.New("missing created_at")
	}

	return nil
}

func (s *UserSession) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

func (s *UserSession) IsValid() bool {
	return !s.IsExpired()
}

type UserSessionSelectParams struct {
	UserID string
	Limit  int
	Offset int
}

type UserSessionRepository interface {
	Get(context.Context, string) (UserSession, error)
	GetByToken(context.Context, string) (UserSession, error)
	Create(context.Context, *UserSession) error
	Update(context.Context, *UserSession) error
	Delete(context.Context, string) error
	DeleteByUser(context.Context, string) error
	Select(context.Context, UserSessionSelectParams) ([]UserSession, error)
	CleanupExpired(context.Context) error
}
