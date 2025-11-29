package web

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	invitationDuration = 7 * 24 * time.Hour // 7 days
)

type MemberService struct {
	memberRepo     OrganizationMemberRepository
	userRepo       UserRepository
	invitationRepo OrganizationInvitationRepository
	auditRepo      AuditLogRepository
}

func NewMemberService(
	memberRepo OrganizationMemberRepository,
	userRepo UserRepository,
	invitationRepo OrganizationInvitationRepository,
	auditRepo AuditLogRepository,
) *MemberService {
	return &MemberService{
		memberRepo:     memberRepo,
		userRepo:       userRepo,
		invitationRepo: invitationRepo,
		auditRepo:      auditRepo,
	}
}

// GetMembers retrieves all members of an organization
func (s *MemberService) GetMembers(ctx context.Context, orgID string, requestUserID string) ([]OrganizationMember, error) {
	// Check if requester is a member
	_, err := s.memberRepo.GetByOrganizationAndUser(ctx, orgID, requestUserID)
	if err != nil {
		return nil, errors.New("access denied")
	}

	members, err := s.memberRepo.Select(ctx, OrganizationMemberSelectParams{
		OrganizationID: orgID,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get members: %w", err)
	}

	return members, nil
}

// InviteMember invites a user to join an organization
func (s *MemberService) InviteMember(ctx context.Context, orgID, email, role string, inviterUserID string) (*OrganizationInvitation, error) {
	// Check if inviter has permission
	inviter, err := s.memberRepo.GetByOrganizationAndUser(ctx, orgID, inviterUserID)
	if err != nil {
		return nil, errors.New("access denied")
	}

	if !CanManageMembers(inviter.Role) {
		return nil, errors.New("access denied: requires admin or owner role")
	}

	// Validate role
	if !IsValidRole(role) {
		return nil, errors.New("invalid role")
	}

	// Only owners can invite other owners
	if role == RoleOwner && inviter.Role != RoleOwner {
		return nil, errors.New("only owners can invite other owners")
	}

	// Check if user already exists and is a member
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err == nil && user.ID != "" {
		// User exists, check if already a member
		_, err := s.memberRepo.GetByOrganizationAndUser(ctx, orgID, user.ID)
		if err == nil {
			return nil, errors.New("user is already a member")
		}
	}

	// Check if there's already a pending invitation
	existingInvitations, err := s.invitationRepo.Select(ctx, InvitationSelectParams{
		OrganizationID: orgID,
		Email:          email,
		Status:         InvitationStatusPending,
	})

	if err == nil && len(existingInvitations) > 0 {
		return nil, errors.New("invitation already sent")
	}

	// Generate invitation token
	token, tokenHash, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Create invitation
	invitation := &OrganizationInvitation{
		ID:             uuid.New().String(),
		OrganizationID: orgID,
		Email:          email,
		Role:           role,
		TokenHash:      tokenHash,
		InvitedBy:      inviterUserID,
		Status:         InvitationStatusPending,
		ExpiresAt:      time.Now().Add(invitationDuration),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := s.invitationRepo.Create(ctx, invitation); err != nil {
		return nil, fmt.Errorf("failed to create invitation: %w", err)
	}

	// Store the plain token temporarily for returning to caller
	invitation.TokenHash = token

	// Audit log
	if s.auditRepo != nil {
		s.auditRepo.Create(ctx, &AuditLog{
			ID:             uuid.New().String(),
			OrganizationID: &orgID,
			UserID:         &inviterUserID,
			Action:         AuditActionMemberInvited,
			ResourceType:   "invitation",
			ResourceID:     invitation.ID,
			Metadata: map[string]interface{}{
				"email": email,
				"role":  role,
			},
			CreatedAt: time.Now(),
		})
	}

	return invitation, nil
}

// AcceptInvitation accepts an invitation and adds the user to the organization
func (s *MemberService) AcceptInvitation(ctx context.Context, token string, userID string) error {
	// Hash token
	tokenHash := hashToken(token)

	// Get invitation
	invitation, err := s.invitationRepo.GetByToken(ctx, tokenHash)
	if err != nil {
		return errors.New("invalid or expired invitation")
	}

	// Check if invitation is valid
	if !invitation.IsValid() {
		return errors.New("invitation is expired or already used")
	}

	// Get user
	user, err := s.userRepo.Get(ctx, userID)
	if err != nil {
		return errors.New("user not found")
	}

	// Check if email matches
	if user.Email != invitation.Email {
		return errors.New("invitation email does not match user email")
	}

	// Check if user is already a member
	_, err = s.memberRepo.GetByOrganizationAndUser(ctx, invitation.OrganizationID, userID)
	if err == nil {
		return errors.New("user is already a member")
	}

	// Add user as member
	member := &OrganizationMember{
		ID:             uuid.New().String(),
		OrganizationID: invitation.OrganizationID,
		UserID:         userID,
		Role:           invitation.Role,
		InvitedBy:      &invitation.InvitedBy,
		JoinedAt:       time.Now(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := s.memberRepo.Create(ctx, member); err != nil {
		return fmt.Errorf("failed to add member: %w", err)
	}

	// Update invitation status
	now := time.Now()
	invitation.Status = InvitationStatusAccepted
	invitation.AcceptedAt = &now
	invitation.UpdatedAt = now

	if err := s.invitationRepo.Update(ctx, &invitation); err != nil {
		// Non-critical error
	}

	// Audit log
	if s.auditRepo != nil {
		s.auditRepo.Create(ctx, &AuditLog{
			ID:             uuid.New().String(),
			OrganizationID: &invitation.OrganizationID,
			UserID:         &userID,
			Action:         AuditActionMemberJoined,
			ResourceType:   "member",
			ResourceID:     member.ID,
			CreatedAt:      time.Now(),
		})
	}

	return nil
}

// RemoveMember removes a member from an organization
func (s *MemberService) RemoveMember(ctx context.Context, orgID, memberUserID string, requestUserID string) error {
	// Check if requester has permission
	requester, err := s.memberRepo.GetByOrganizationAndUser(ctx, orgID, requestUserID)
	if err != nil {
		return errors.New("access denied")
	}

	if !CanManageMembers(requester.Role) {
		return errors.New("access denied: requires admin or owner role")
	}

	// Get member to remove
	member, err := s.memberRepo.GetByOrganizationAndUser(ctx, orgID, memberUserID)
	if err != nil {
		return errors.New("member not found")
	}

	// Can't remove owner (unless it's the last owner removing themselves)
	if member.Role == RoleOwner {
		// Count owners
		owners, err := s.memberRepo.Select(ctx, OrganizationMemberSelectParams{
			OrganizationID: orgID,
			Role:           RoleOwner,
		})

		if err != nil {
			return err
		}

		// If there's only one owner, can't remove
		if len(owners) == 1 && memberUserID != requestUserID {
			return errors.New("cannot remove the only owner")
		}

		// Only owners can remove owners
		if requester.Role != RoleOwner {
			return errors.New("only owners can remove other owners")
		}
	}

	// Remove member
	if err := s.memberRepo.Delete(ctx, member.ID); err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}

	// Audit log
	if s.auditRepo != nil {
		s.auditRepo.Create(ctx, &AuditLog{
			ID:             uuid.New().String(),
			OrganizationID: &orgID,
			UserID:         &requestUserID,
			Action:         AuditActionMemberRemoved,
			ResourceType:   "member",
			ResourceID:     member.ID,
			Metadata: map[string]interface{}{
				"removed_user_id": memberUserID,
			},
			CreatedAt: time.Now(),
		})
	}

	return nil
}

// UpdateMemberRole updates a member's role
func (s *MemberService) UpdateMemberRole(ctx context.Context, orgID, memberUserID, newRole string, requestUserID string) error {
	// Check if requester has permission
	requester, err := s.memberRepo.GetByOrganizationAndUser(ctx, orgID, requestUserID)
	if err != nil {
		return errors.New("access denied")
	}

	if !CanManageMembers(requester.Role) {
		return errors.New("access denied: requires admin or owner role")
	}

	// Validate role
	if !IsValidRole(newRole) {
		return errors.New("invalid role")
	}

	// Only owners can assign owner role
	if newRole == RoleOwner && requester.Role != RoleOwner {
		return errors.New("only owners can assign owner role")
	}

	// Get member
	member, err := s.memberRepo.GetByOrganizationAndUser(ctx, orgID, memberUserID)
	if err != nil {
		return errors.New("member not found")
	}

	// Can't change own role
	if memberUserID == requestUserID {
		return errors.New("cannot change your own role")
	}

	// Only owners can change owner roles
	if member.Role == RoleOwner && requester.Role != RoleOwner {
		return errors.New("only owners can change owner roles")
	}

	// Update role
	oldRole := member.Role
	member.Role = newRole
	member.UpdatedAt = time.Now()

	if err := s.memberRepo.Update(ctx, &member); err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}

	// Audit log
	if s.auditRepo != nil {
		s.auditRepo.Create(ctx, &AuditLog{
			ID:             uuid.New().String(),
			OrganizationID: &orgID,
			UserID:         &requestUserID,
			Action:         AuditActionMemberRoleChanged,
			ResourceType:   "member",
			ResourceID:     member.ID,
			Metadata: map[string]interface{}{
				"member_user_id": memberUserID,
				"old_role":       oldRole,
				"new_role":       newRole,
			},
			CreatedAt: time.Now(),
		})
	}

	return nil
}

// GetPendingInvitations retrieves pending invitations for an organization
func (s *MemberService) GetPendingInvitations(ctx context.Context, orgID string, requestUserID string) ([]OrganizationInvitation, error) {
	// Check if requester has permission
	requester, err := s.memberRepo.GetByOrganizationAndUser(ctx, orgID, requestUserID)
	if err != nil {
		return nil, errors.New("access denied")
	}

	if !CanManageMembers(requester.Role) {
		return nil, errors.New("access denied: requires admin or owner role")
	}

	invitations, err := s.invitationRepo.Select(ctx, InvitationSelectParams{
		OrganizationID: orgID,
		Status:         InvitationStatusPending,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get invitations: %w", err)
	}

	return invitations, nil
}

// RevokeInvitation revokes a pending invitation
func (s *MemberService) RevokeInvitation(ctx context.Context, invitationID string, requestUserID string) error {
	// Get invitation
	invitation, err := s.invitationRepo.Get(ctx, invitationID)
	if err != nil {
		return errors.New("invitation not found")
	}

	// Check if requester has permission
	requester, err := s.memberRepo.GetByOrganizationAndUser(ctx, invitation.OrganizationID, requestUserID)
	if err != nil {
		return errors.New("access denied")
	}

	if !CanManageMembers(requester.Role) {
		return errors.New("access denied: requires admin or owner role")
	}

	// Update status
	invitation.Status = InvitationStatusRevoked
	invitation.UpdatedAt = time.Now()

	if err := s.invitationRepo.Update(ctx, &invitation); err != nil {
		return fmt.Errorf("failed to revoke invitation: %w", err)
	}

	return nil
}
