package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type APIKeyService struct {
	repo APIKeyRepository
}

func NewAPIKeyService(repo APIKeyRepository) *APIKeyService {
	return &APIKeyService{
		repo: repo,
	}
}

// GenerateAPIKey generates a new secure API key (32 bytes = 256 bits)
// Returns the key in base64 format (safe for URLs and headers)
func (s *APIKeyService) GenerateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}

	// Prefix with "gms_" for Google Maps Scraper
	return "gms_" + base64.URLEncoding.EncodeToString(b), nil
}

// HashAPIKey creates a SHA-256 hash of the API key for storage
func (s *APIKeyService) HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// Create creates a new API key
// Returns the full API key (which should be shown to user only once) and the APIKey object
func (s *APIKeyService) Create(ctx context.Context, name string, expiresAt *time.Time) (string, *APIKey, error) {
	// Extract user and organization from context
	user := getUserFromContext(ctx)
	orgID := getOrganizationIDFromContext(ctx)

	if orgID == "" {
		return "", nil, fmt.Errorf("organization context required")
	}
	if user == nil {
		return "", nil, fmt.Errorf("user context required")
	}

	// Generate new API key
	key, err := s.GenerateAPIKey()
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Hash the key for storage
	keyHash := s.HashAPIKey(key)

	now := time.Now().UTC()
	apiKey := &APIKey{
		ID:             uuid.New().String(),
		OrganizationID: orgID,
		CreatedBy:      user.ID,
		Name:           name,
		KeyHash:        keyHash,
		Status:         APIKeyStatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
		ExpiresAt:      expiresAt,
	}

	if err := apiKey.Validate(); err != nil {
		return "", nil, fmt.Errorf("invalid API key: %w", err)
	}

	if err := s.repo.Create(ctx, apiKey); err != nil {
		return "", nil, fmt.Errorf("failed to create API key: %w", err)
	}

	// Return the plaintext key (to show user) and the APIKey object
	return key, apiKey, nil
}

// Validate validates an API key and returns the APIKey object and updated context if valid
func (s *APIKeyService) Validate(ctx context.Context, key string) (*APIKey, context.Context, error) {
	if key == "" {
		return nil, ctx, fmt.Errorf("API key is required")
	}

	// Hash the provided key
	keyHash := s.HashAPIKey(key)

	// Look up the API key by hash
	apiKey, err := s.repo.GetByKey(ctx, keyHash)
	if err != nil {
		return nil, ctx, fmt.Errorf("invalid API key")
	}

	// Check if the key is active
	if !apiKey.IsActive() {
		return nil, ctx, fmt.Errorf("API key is inactive or expired")
	}

	// Update last used timestamp
	now := time.Now().UTC()
	apiKey.LastUsedAt = &now
	if err := s.repo.Update(ctx, &apiKey); err != nil {
		// Log error but don't fail validation
		// The key is still valid even if we can't update last_used_at
	}

	// Inject organization ID into context
	if apiKey.OrganizationID != "" {
		ctx = context.WithValue(ctx, contextKeyOrganizationID, apiKey.OrganizationID)
	}

	return &apiKey, ctx, nil
}

// Get retrieves an API key by ID
func (s *APIKeyService) Get(ctx context.Context, id string) (*APIKey, error) {
	apiKey, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	// Verify organization access
	orgID := getOrganizationIDFromContext(ctx)
	if orgID != "" && apiKey.OrganizationID != orgID {
		return nil, fmt.Errorf("API key not found") // Don't leak existence
	}

	return &apiKey, nil
}

// List retrieves all API keys for the current organization
func (s *APIKeyService) List(ctx context.Context) ([]APIKey, error) {
	orgID := getOrganizationIDFromContext(ctx)
	if orgID == "" {
		return nil, fmt.Errorf("organization context required")
	}

	return s.repo.Select(ctx, APIKeySelectParams{OrganizationID: orgID})
}

// ListActive retrieves all active API keys for the current organization
func (s *APIKeyService) ListActive(ctx context.Context) ([]APIKey, error) {
	orgID := getOrganizationIDFromContext(ctx)
	if orgID == "" {
		return nil, fmt.Errorf("organization context required")
	}

	return s.repo.Select(ctx, APIKeySelectParams{
		OrganizationID: orgID,
		Status:         APIKeyStatusActive,
	})
}

// Revoke revokes an API key by ID
func (s *APIKeyService) Revoke(ctx context.Context, id string) error {
	// Get and verify ownership
	apiKey, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	apiKey.Status = APIKeyStatusRevoked
	apiKey.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, apiKey); err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	return nil
}

// Delete permanently deletes an API key
func (s *APIKeyService) Delete(ctx context.Context, id string) error {
	// Verify ownership first
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}

	return nil
}
