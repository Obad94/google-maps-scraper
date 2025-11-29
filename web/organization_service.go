package web

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type OrganizationService struct {
	orgRepo    OrganizationRepository
	memberRepo OrganizationMemberRepository
	auditRepo  AuditLogRepository
}

func NewOrganizationService(
	orgRepo OrganizationRepository,
	memberRepo OrganizationMemberRepository,
	auditRepo AuditLogRepository,
) *OrganizationService {
	return &OrganizationService{
		orgRepo:    orgRepo,
		memberRepo: memberRepo,
		auditRepo:  auditRepo,
	}
}

// Create creates a new organization with the creator as owner
func (s *OrganizationService) Create(ctx context.Context, name, description string, creatorUserID string) (*Organization, error) {
	// Generate slug from name
	slug := generateSlug(name)

	// Check if slug already exists
	existing, err := s.orgRepo.GetBySlug(ctx, slug)
	if err == nil && existing.ID != "" {
		// Slug exists, add random suffix
		slug = fmt.Sprintf("%s-%s", slug, uuid.New().String()[:8])
	}

	// Create organization
	org := &Organization{
		ID:          uuid.New().String(),
		Name:        name,
		Slug:        slug,
		Description: description,
		Status:      OrganizationStatusActive,
		Settings:    make(map[string]interface{}),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.orgRepo.Create(ctx, org); err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	// Add creator as owner
	member := &OrganizationMember{
		ID:             uuid.New().String(),
		OrganizationID: org.ID,
		UserID:         creatorUserID,
		Role:           RoleOwner,
		JoinedAt:       time.Now(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := s.memberRepo.Create(ctx, member); err != nil {
		// Rollback organization creation would be ideal here
		return nil, fmt.Errorf("failed to add owner: %w", err)
	}

	// Audit log
	if s.auditRepo != nil {
		s.auditRepo.Create(ctx, &AuditLog{
			ID:             uuid.New().String(),
			OrganizationID: &org.ID,
			UserID:         &creatorUserID,
			Action:         AuditActionOrgCreated,
			ResourceType:   "organization",
			ResourceID:     org.ID,
			CreatedAt:      time.Now(),
		})
	}

	return org, nil
}

// Get retrieves an organization by ID
func (s *OrganizationService) Get(ctx context.Context, id string, requestUserID string) (*Organization, error) {
	// Check if user is a member
	_, err := s.memberRepo.GetByOrganizationAndUser(ctx, id, requestUserID)
	if err != nil {
		return nil, errors.New("access denied: not a member of this organization")
	}

	org, err := s.orgRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return &org, nil
}

// GetBySlug retrieves an organization by slug
func (s *OrganizationService) GetBySlug(ctx context.Context, slug string, requestUserID string) (*Organization, error) {
	org, err := s.orgRepo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}

	// Check if user is a member
	_, err = s.memberRepo.GetByOrganizationAndUser(ctx, org.ID, requestUserID)
	if err != nil {
		return nil, errors.New("access denied: not a member of this organization")
	}

	return &org, nil
}

// Update updates an organization (owner only)
func (s *OrganizationService) Update(ctx context.Context, org *Organization, requestUserID string) error {
	// Check permission
	member, err := s.memberRepo.GetByOrganizationAndUser(ctx, org.ID, requestUserID)
	if err != nil {
		return errors.New("access denied")
	}

	if !CanManageOrganization(member.Role) {
		return errors.New("access denied: requires owner role")
	}

	org.UpdatedAt = time.Now()
	if err := s.orgRepo.Update(ctx, org); err != nil {
		return err
	}

	// Audit log
	if s.auditRepo != nil {
		s.auditRepo.Create(ctx, &AuditLog{
			ID:             uuid.New().String(),
			OrganizationID: &org.ID,
			UserID:         &requestUserID,
			Action:         AuditActionOrgUpdated,
			ResourceType:   "organization",
			ResourceID:     org.ID,
			CreatedAt:      time.Now(),
		})
	}

	return nil
}

// Delete soft-deletes an organization (owner only)
func (s *OrganizationService) Delete(ctx context.Context, orgID string, requestUserID string) error {
	// Check permission
	member, err := s.memberRepo.GetByOrganizationAndUser(ctx, orgID, requestUserID)
	if err != nil {
		return errors.New("access denied")
	}

	if !CanManageOrganization(member.Role) {
		return errors.New("access denied: requires owner role")
	}

	if err := s.orgRepo.Delete(ctx, orgID); err != nil {
		return err
	}

	// Audit log
	if s.auditRepo != nil {
		s.auditRepo.Create(ctx, &AuditLog{
			ID:             uuid.New().String(),
			OrganizationID: &orgID,
			UserID:         &requestUserID,
			Action:         AuditActionOrgDeleted,
			ResourceType:   "organization",
			ResourceID:     orgID,
			CreatedAt:      time.Now(),
		})
	}

	return nil
}

// GetUserOrganizations retrieves all organizations for a user
func (s *OrganizationService) GetUserOrganizations(ctx context.Context, userID string) ([]Organization, error) {
	return s.memberRepo.GetUserOrganizations(ctx, userID)
}

// UpdateSettings updates organization settings (owner only)
func (s *OrganizationService) UpdateSettings(ctx context.Context, orgID string, settings map[string]interface{}, requestUserID string) error {
	// Check permission
	member, err := s.memberRepo.GetByOrganizationAndUser(ctx, orgID, requestUserID)
	if err != nil {
		return errors.New("access denied")
	}

	if !CanManageOrganization(member.Role) {
		return errors.New("access denied: requires owner role")
	}

	org, err := s.orgRepo.Get(ctx, orgID)
	if err != nil {
		return err
	}

	org.Settings = settings
	org.UpdatedAt = time.Now()

	if err := s.orgRepo.Update(ctx, &org); err != nil {
		return err
	}

	// Audit log
	if s.auditRepo != nil {
		s.auditRepo.Create(ctx, &AuditLog{
			ID:             uuid.New().String(),
			OrganizationID: &orgID,
			UserID:         &requestUserID,
			Action:         AuditActionOrgSettingsChanged,
			ResourceType:   "organization",
			ResourceID:     orgID,
			CreatedAt:      time.Now(),
		})
	}

	return nil
}

// generateSlug generates a URL-friendly slug from a name
func generateSlug(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)

	// Replace spaces and special characters with hyphens
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Limit length
	if len(slug) > 50 {
		slug = slug[:50]
	}

	return slug
}
