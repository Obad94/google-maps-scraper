package web

import (
	"context"
	"errors"
	"time"
)

const (
	APIKeyStatusActive  = "active"
	APIKeyStatusRevoked = "revoked"
)

type APIKeyRepository interface {
	Get(context.Context, string) (APIKey, error)
	GetByKey(context.Context, string) (APIKey, error)
	Create(context.Context, *APIKey) error
	Delete(context.Context, string) error
	Select(context.Context, APIKeySelectParams) ([]APIKey, error)
	Update(context.Context, *APIKey) error
}

type APIKeySelectParams struct {
	OrganizationID string
	CreatedBy      string
	Status         string
	Limit          int
}

type APIKey struct {
	ID             string
	OrganizationID string
	CreatedBy      string
	Name           string
	Key            string
	KeyHash        string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastUsedAt     *time.Time
	ExpiresAt      *time.Time
}

func (a *APIKey) Validate() error {
	if a.ID == "" {
		return errors.New("missing id")
	}

	if a.Name == "" {
		return errors.New("missing name")
	}

	if a.KeyHash == "" {
		return errors.New("missing key hash")
	}

	if a.Status == "" {
		return errors.New("missing status")
	}

	if a.Status != APIKeyStatusActive && a.Status != APIKeyStatusRevoked {
		return errors.New("invalid status")
	}

	if a.CreatedAt.IsZero() {
		return errors.New("missing created_at")
	}

	return nil
}

func (a *APIKey) IsActive() bool {
	if a.Status != APIKeyStatusActive {
		return false
	}

	if a.ExpiresAt != nil && a.ExpiresAt.Before(time.Now()) {
		return false
	}

	return true
}
