package web

import (
	"context"
	"errors"
	"time"
)

const (
	// Organization roles with hierarchical permissions
	RoleOwner  = "owner"  // Full control: manage org, billing, delete org, all admin permissions
	RoleAdmin  = "admin"  // Manage members, jobs, API keys, settings
	RoleMember = "member" // Create and manage own jobs, view org jobs
	RoleViewer = "viewer" // Read-only access to jobs and data
)

// OrganizationMember represents a user's membership in an organization
type OrganizationMember struct {
	ID             string
	OrganizationID string
	UserID         string
	Role           string
	InvitedBy      *string
	JoinedAt       time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time

	// Populated by joins
	User         *User
	Organization *Organization
}

func (m *OrganizationMember) Validate() error {
	if m.ID == "" {
		return errors.New("missing id")
	}

	if m.OrganizationID == "" {
		return errors.New("missing organization_id")
	}

	if m.UserID == "" {
		return errors.New("missing user_id")
	}

	if m.Role == "" {
		return errors.New("missing role")
	}

	if !IsValidRole(m.Role) {
		return errors.New("invalid role")
	}

	if m.JoinedAt.IsZero() {
		return errors.New("missing joined_at")
	}

	if m.CreatedAt.IsZero() {
		return errors.New("missing created_at")
	}

	return nil
}

// IsValidRole checks if the role is valid
func IsValidRole(role string) bool {
	return role == RoleOwner || role == RoleAdmin || role == RoleMember || role == RoleViewer
}

// Permission levels for role hierarchy
type Permission int

const (
	PermissionNone Permission = iota
	PermissionRead
	PermissionWrite
	PermissionAdmin
	PermissionOwner
)

// GetPermissionLevel returns the permission level for a role
func GetPermissionLevel(role string) Permission {
	switch role {
	case RoleOwner:
		return PermissionOwner
	case RoleAdmin:
		return PermissionAdmin
	case RoleMember:
		return PermissionWrite
	case RoleViewer:
		return PermissionRead
	default:
		return PermissionNone
	}
}

// HasPermission checks if a role has at least the required permission level
func HasPermission(role string, required Permission) bool {
	return GetPermissionLevel(role) >= required
}

// CanManageMembers checks if role can manage organization members
func CanManageMembers(role string) bool {
	return role == RoleOwner || role == RoleAdmin
}

// CanManageJobs checks if role can manage all jobs in organization
func CanManageJobs(role string) bool {
	return role == RoleOwner || role == RoleAdmin
}

// CanCreateJobs checks if role can create jobs
func CanCreateJobs(role string) bool {
	return role == RoleOwner || role == RoleAdmin || role == RoleMember
}

// CanManageAPIKeys checks if role can manage API keys
func CanManageAPIKeys(role string) bool {
	return role == RoleOwner || role == RoleAdmin
}

// CanManageOrganization checks if role can manage organization settings
func CanManageOrganization(role string) bool {
	return role == RoleOwner
}

type OrganizationMemberSelectParams struct {
	OrganizationID string
	UserID         string
	Role           string
	Limit          int
	Offset         int
}

type OrganizationMemberRepository interface {
	Get(context.Context, string) (OrganizationMember, error)
	GetByOrganizationAndUser(context.Context, string, string) (OrganizationMember, error)
	Create(context.Context, *OrganizationMember) error
	Update(context.Context, *OrganizationMember) error
	Delete(context.Context, string) error
	Select(context.Context, OrganizationMemberSelectParams) ([]OrganizationMember, error)
	GetUserOrganizations(context.Context, string) ([]Organization, error)
	CountByOrganization(context.Context, string) (int, error)
}
