package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionDuration = 30 * 24 * time.Hour // 30 days
	tokenLength     = 32                  // 32 bytes = 256 bits
)

type AuthService struct {
	userRepo    UserRepository
	sessionRepo UserSessionRepository
	auditRepo   AuditLogRepository
	orgRepo     OrganizationRepository
	memberRepo  OrganizationMemberRepository
}

func NewAuthService(userRepo UserRepository, sessionRepo UserSessionRepository, auditRepo AuditLogRepository) *AuthService {
	return &AuthService{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		auditRepo:   auditRepo,
	}
}

// NewAuthServiceWithOrg creates an AuthService with organization support for multi-tenancy
func NewAuthServiceWithOrg(userRepo UserRepository, sessionRepo UserSessionRepository, auditRepo AuditLogRepository, orgRepo OrganizationRepository, memberRepo OrganizationMemberRepository) *AuthService {
	return &AuthService{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		auditRepo:   auditRepo,
		orgRepo:     orgRepo,
		memberRepo:  memberRepo,
	}
}

// Register creates a new user account
func (s *AuthService) Register(ctx context.Context, email, password, firstName, lastName string) (*User, error) {
	// Check if user already exists
	existingUser, err := s.userRepo.GetByEmail(ctx, email)
	if err == nil && existingUser.ID != "" {
		return nil, errors.New("user already exists")
	}

	// Hash password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user := &User{
		ID:            uuid.New().String(),
		Email:         email,
		PasswordHash:  string(passwordHash),
		FirstName:     firstName,
		LastName:      lastName,
		EmailVerified: false,
		Status:        UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Create default organization for the user (multi-tenancy support)
	if s.orgRepo != nil && s.memberRepo != nil {
		orgName := fmt.Sprintf("%s's Organization", firstName)
		if firstName == "" {
			orgName = fmt.Sprintf("%s's Organization", email)
		}
		
		org := &Organization{
			ID:          uuid.New().String(),
			Name:        orgName,
			Slug:        fmt.Sprintf("org-%s", user.ID[:8]),
			Description: "Default organization",
			Status:      OrganizationStatusActive,
			Settings:    make(map[string]interface{}),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		
		if err := s.orgRepo.Create(ctx, org); err != nil {
			// Log error but don't fail registration
			fmt.Printf("Warning: failed to create default organization: %v\n", err)
		} else {
			// Add user as owner of the organization
			member := &OrganizationMember{
				ID:             uuid.New().String(),
				OrganizationID: org.ID,
				UserID:         user.ID,
				Role:           RoleOwner,
				JoinedAt:       time.Now(),
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}
			
			if err := s.memberRepo.Create(ctx, member); err != nil {
				fmt.Printf("Warning: failed to add user to organization: %v\n", err)
			}
		}
	}

	// Audit log
	if s.auditRepo != nil {
		s.auditRepo.Create(ctx, &AuditLog{
			ID:           uuid.New().String(),
			UserID:       &user.ID,
			Action:       "user.registered",
			ResourceType: "user",
			ResourceID:   user.ID,
			CreatedAt:    time.Now(),
		})
	}

	return user, nil
}

// Login authenticates a user and creates a session
func (s *AuthService) Login(ctx context.Context, email, password, ipAddress, userAgent string) (*User, *UserSession, string, error) {
	// Get user by email
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, nil, "", errors.New("invalid email or password")
	}

	// Check if user is active
	if !user.IsActive() {
		return nil, nil, "", errors.New("user account is inactive or suspended")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, nil, "", errors.New("invalid email or password")
	}

	// Generate session token
	token, tokenHash, err := generateToken()
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to generate token: %w", err)
	}

	// Create session
	session := &UserSession{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(sessionDuration),
		CreatedAt: time.Now(),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, nil, "", fmt.Errorf("failed to create session: %w", err)
	}

	// Update last login
	now := time.Now()
	user.LastLoginAt = &now
	user.UpdatedAt = now
	if err := s.userRepo.Update(ctx, &user); err != nil {
		// Non-critical error, just log it
	}

	// Audit log
	if s.auditRepo != nil {
		s.auditRepo.Create(ctx, &AuditLog{
			ID:           uuid.New().String(),
			UserID:       &user.ID,
			Action:       AuditActionUserLogin,
			ResourceType: "user",
			ResourceID:   user.ID,
			IPAddress:    ipAddress,
			UserAgent:    userAgent,
			CreatedAt:    time.Now(),
		})
	}

	return &user, session, token, nil
}

// ValidateSession validates a session token and returns the user
func (s *AuthService) ValidateSession(ctx context.Context, token string) (*User, *UserSession, error) {
	// Hash the token
	tokenHash := hashToken(token)

	// Get session
	session, err := s.sessionRepo.GetByToken(ctx, tokenHash)
	if err != nil {
		return nil, nil, errors.New("invalid session")
	}

	// Check if session is valid
	if !session.IsValid() {
		return nil, nil, errors.New("session expired")
	}

	// Get user
	user, err := s.userRepo.Get(ctx, session.UserID)
	if err != nil {
		return nil, nil, errors.New("user not found")
	}

	// Check if user is active
	if !user.IsActive() {
		return nil, nil, errors.New("user account is inactive")
	}

	// Update last used
	now := time.Now()
	session.LastUsedAt = &now
	if err := s.sessionRepo.Update(ctx, &session); err != nil {
		// Non-critical error
	}

	return &user, &session, nil
}

// Logout invalidates a session
func (s *AuthService) Logout(ctx context.Context, sessionID string, userID string) error {
	if err := s.sessionRepo.Delete(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Audit log
	if s.auditRepo != nil {
		s.auditRepo.Create(ctx, &AuditLog{
			ID:           uuid.New().String(),
			UserID:       &userID,
			Action:       AuditActionUserLogout,
			ResourceType: "user",
			ResourceID:   userID,
			CreatedAt:    time.Now(),
		})
	}

	return nil
}

// LogoutAll invalidates all sessions for a user
func (s *AuthService) LogoutAll(ctx context.Context, userID string) error {
	if err := s.sessionRepo.DeleteByUser(ctx, userID); err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}

	return nil
}

// ChangePassword changes a user's password
func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	// Get user
	user, err := s.userRepo.Get(ctx, userID)
	if err != nil {
		return errors.New("user not found")
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return errors.New("invalid password")
	}

	// Hash new password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	user.PasswordHash = string(passwordHash)
	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, &user); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Invalidate all sessions (force re-login)
	s.sessionRepo.DeleteByUser(ctx, userID)

	// Audit log
	if s.auditRepo != nil {
		s.auditRepo.Create(ctx, &AuditLog{
			ID:           uuid.New().String(),
			UserID:       &userID,
			Action:       AuditActionUserPasswordChanged,
			ResourceType: "user",
			ResourceID:   userID,
			CreatedAt:    time.Now(),
		})
	}

	return nil
}

// generateToken generates a random token and its hash
func generateToken() (string, string, error) {
	// Generate random bytes
	b := make([]byte, tokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}

	// Encode to base64
	token := base64.URLEncoding.EncodeToString(b)

	// Hash the token
	hash := hashToken(token)

	return token, hash, nil
}

// hashToken hashes a token using SHA-256
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", hash)
}
