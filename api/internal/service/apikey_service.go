package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// APIKeyService handles API key operations.
type APIKeyService struct {
	repos  *repository.Repositories
	logger *slog.Logger
}

// NewAPIKeyService creates a new API key service.
func NewAPIKeyService(repos *repository.Repositories, logger *slog.Logger) *APIKeyService {
	return &APIKeyService{
		repos:  repos,
		logger: logger,
	}
}

// CreateKeyInput represents input for creating an API key.
type CreateKeyInput struct {
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreateKeyOutput represents output from creating an API key.
type CreateKeyOutput struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"` // Only returned on creation
	KeyPrefix string     `json:"key_prefix"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateKey creates a new API key.
func (s *APIKeyService) CreateKey(ctx context.Context, userID string, input CreateKeyInput) (*CreateKeyOutput, error) {
	// Generate random key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	key := "rf_" + base64.RawURLEncoding.EncodeToString(keyBytes)
	keyPrefix := key[:11] + "..."

	// Hash the key
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	// Default scopes
	scopes := input.Scopes
	if len(scopes) == 0 {
		scopes = []string{"extract", "crawl", "jobs"}
	}

	now := time.Now()
	apiKey := &models.APIKey{
		ID:        ulid.Make().String(),
		UserID:    userID,
		Name:      input.Name,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		Scopes:    scopes,
		ExpiresAt: input.ExpiresAt,
		CreatedAt: now,
	}

	if err := s.repos.APIKey.Create(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	return &CreateKeyOutput{
		ID:        apiKey.ID,
		Name:      apiKey.Name,
		Key:       key, // Only returned here
		KeyPrefix: keyPrefix,
		Scopes:    scopes,
		ExpiresAt: input.ExpiresAt,
		CreatedAt: now,
	}, nil
}

// ListKeys lists API keys for a user (without the actual key).
func (s *APIKeyService) ListKeys(ctx context.Context, userID string) ([]*models.APIKey, error) {
	return s.repos.APIKey.GetByUserID(ctx, userID)
}

// RevokeKey revokes an API key.
func (s *APIKeyService) RevokeKey(ctx context.Context, userID, keyID string) error {
	// Verify the key belongs to the user
	key, err := s.repos.APIKey.GetByID(ctx, keyID)
	if err != nil {
		return fmt.Errorf("failed to get key: %w", err)
	}
	if key == nil || key.UserID != userID {
		return fmt.Errorf("key not found")
	}

	return s.repos.APIKey.Revoke(ctx, keyID)
}
