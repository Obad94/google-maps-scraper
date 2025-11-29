package web

import (
	"context"
	"errors"
	"time"
)

// Audit action constants
const (
	// Organization actions
	AuditActionOrgCreated        = "organization.created"
	AuditActionOrgUpdated        = "organization.updated"
	AuditActionOrgDeleted        = "organization.deleted"
	AuditActionOrgSettingsChanged = "organization.settings_changed"

	// Member actions
	AuditActionMemberInvited = "member.invited"
	AuditActionMemberJoined  = "member.joined"
	AuditActionMemberRemoved = "member.removed"
	AuditActionMemberRoleChanged = "member.role_changed"

	// Job actions
	AuditActionJobCreated = "job.created"
	AuditActionJobUpdated = "job.updated"
	AuditActionJobDeleted = "job.deleted"
	AuditActionJobRetried = "job.retried"

	// API Key actions
	AuditActionAPIKeyCreated = "apikey.created"
	AuditActionAPIKeyRevoked = "apikey.revoked"
	AuditActionAPIKeyDeleted = "apikey.deleted"

	// User actions
	AuditActionUserLogin  = "user.login"
	AuditActionUserLogout = "user.logout"
	AuditActionUserPasswordChanged = "user.password_changed"
	AuditActionUserEmailChanged = "user.email_changed"
)

// AuditLog represents a record of an action performed in the system
type AuditLog struct {
	ID             string
	OrganizationID *string
	UserID         *string
	Action         string
	ResourceType   string
	ResourceID     string
	Metadata       map[string]interface{}
	IPAddress      string
	UserAgent      string
	CreatedAt      time.Time

	// Populated by joins
	User         *User
	Organization *Organization
}

func (a *AuditLog) Validate() error {
	if a.ID == "" {
		return errors.New("missing id")
	}

	if a.Action == "" {
		return errors.New("missing action")
	}

	if a.ResourceType == "" {
		return errors.New("missing resource_type")
	}

	if a.CreatedAt.IsZero() {
		return errors.New("missing created_at")
	}

	return nil
}

type AuditLogSelectParams struct {
	OrganizationID *string
	UserID         *string
	Action         string
	ResourceType   string
	ResourceID     string
	StartDate      *time.Time
	EndDate        *time.Time
	Limit          int
	Offset         int
}

type AuditLogRepository interface {
	Get(context.Context, string) (AuditLog, error)
	Create(context.Context, *AuditLog) error
	Select(context.Context, AuditLogSelectParams) ([]AuditLog, error)
	DeleteOldLogs(context.Context, time.Time) error
}

// AuditLogService provides methods for creating audit logs
type AuditLogService struct {
	repo AuditLogRepository
}

func NewAuditLogService(repo AuditLogRepository) *AuditLogService {
	return &AuditLogService{
		repo: repo,
	}
}

func (s *AuditLogService) Log(ctx context.Context, log *AuditLog) error {
	if log.ID == "" {
		log.ID = generateID()
	}

	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}

	if err := log.Validate(); err != nil {
		return err
	}

	return s.repo.Create(ctx, log)
}

// Helper function to generate UUIDs
func generateID() string {
	// This will be implemented using google/uuid
	return ""
}
