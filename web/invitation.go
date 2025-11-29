package web

import (
	"context"
	"errors"
	"time"
)

const (
	InvitationStatusPending  = "pending"
	InvitationStatusAccepted = "accepted"
	InvitationStatusExpired  = "expired"
	InvitationStatusRevoked  = "revoked"
)

// OrganizationInvitation represents an invitation to join an organization
type OrganizationInvitation struct {
	ID             string
	OrganizationID string
	Email          string
	Role           string
	TokenHash      string
	InvitedBy      string
	Status         string
	ExpiresAt      time.Time
	AcceptedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time

	// Populated by joins
	Organization *Organization
	Inviter      *User
}

func (i *OrganizationInvitation) Validate() error {
	if i.ID == "" {
		return errors.New("missing id")
	}

	if i.OrganizationID == "" {
		return errors.New("missing organization_id")
	}

	if i.Email == "" {
		return errors.New("missing email")
	}

	if i.Role == "" {
		return errors.New("missing role")
	}

	if !IsValidRole(i.Role) {
		return errors.New("invalid role")
	}

	if i.TokenHash == "" {
		return errors.New("missing token_hash")
	}

	if i.InvitedBy == "" {
		return errors.New("missing invited_by")
	}

	if i.Status == "" {
		return errors.New("missing status")
	}

	if i.ExpiresAt.IsZero() {
		return errors.New("missing expires_at")
	}

	if i.CreatedAt.IsZero() {
		return errors.New("missing created_at")
	}

	return nil
}

func (i *OrganizationInvitation) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

func (i *OrganizationInvitation) IsValid() bool {
	return i.Status == InvitationStatusPending && !i.IsExpired()
}

type InvitationSelectParams struct {
	OrganizationID string
	Email          string
	Status         string
	Limit          int
	Offset         int
}

type OrganizationInvitationRepository interface {
	Get(context.Context, string) (OrganizationInvitation, error)
	GetByToken(context.Context, string) (OrganizationInvitation, error)
	Create(context.Context, *OrganizationInvitation) error
	Update(context.Context, *OrganizationInvitation) error
	Delete(context.Context, string) error
	Select(context.Context, InvitationSelectParams) ([]OrganizationInvitation, error)
	CleanupExpired(context.Context) error
}
