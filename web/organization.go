package web

import (
	"context"
	"errors"
	"time"
)

const (
	OrganizationStatusActive   = "active"
	OrganizationStatusInactive = "inactive"
	OrganizationStatusSuspended = "suspended"
)

// Organization represents a tenant in the multi-tenant system
type Organization struct {
	ID          string
	Name        string
	Slug        string
	Description string
	Status      string
	Settings    map[string]interface{}
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

func (o *Organization) Validate() error {
	if o.ID == "" {
		return errors.New("missing id")
	}

	if o.Name == "" {
		return errors.New("missing name")
	}

	if o.Slug == "" {
		return errors.New("missing slug")
	}

	if o.Status == "" {
		return errors.New("missing status")
	}

	if o.Status != OrganizationStatusActive &&
		o.Status != OrganizationStatusInactive &&
		o.Status != OrganizationStatusSuspended {
		return errors.New("invalid status")
	}

	if o.CreatedAt.IsZero() {
		return errors.New("missing created_at")
	}

	return nil
}

func (o *Organization) IsActive() bool {
	return o.Status == OrganizationStatusActive && o.DeletedAt == nil
}

type OrganizationSelectParams struct {
	Status string
	Limit  int
	Offset int
}

type OrganizationRepository interface {
	Get(context.Context, string) (Organization, error)
	GetBySlug(context.Context, string) (Organization, error)
	Create(context.Context, *Organization) error
	Update(context.Context, *Organization) error
	Delete(context.Context, string) error
	Select(context.Context, OrganizationSelectParams) ([]Organization, error)
}
