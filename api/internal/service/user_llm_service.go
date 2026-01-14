package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// UserLLMService handles user LLM provider key and fallback chain operations.
type UserLLMService struct {
	repos     *repository.Repositories
	encryptor *crypto.Encryptor
	logger    *slog.Logger
}

// NewUserLLMService creates a new user LLM service.
func NewUserLLMService(repos *repository.Repositories, encryptor *crypto.Encryptor, logger *slog.Logger) *UserLLMService {
	return &UserLLMService{
		repos:     repos,
		encryptor: encryptor,
		logger:    logger,
	}
}

// UserServiceKeyInput represents input for creating/updating a user service key.
type UserServiceKeyInput struct {
	Provider  string
	APIKey    string
	BaseURL   string
	IsEnabled bool
}

// ListServiceKeys returns all configured service keys for a user.
func (s *UserLLMService) ListServiceKeys(ctx context.Context, userID string) ([]*models.UserServiceKey, error) {
	return s.repos.UserServiceKey.GetByUserID(ctx, userID)
}

// UpsertServiceKey creates or updates a user service key.
func (s *UserLLMService) UpsertServiceKey(ctx context.Context, userID string, input UserServiceKeyInput) (*models.UserServiceKey, error) {
	// Validate provider
	if !isValidUserProvider(input.Provider) {
		return nil, fmt.Errorf("invalid provider: %s (must be openrouter, anthropic, openai, or ollama)", input.Provider)
	}

	// Get existing key if any
	existing, err := s.repos.UserServiceKey.GetByUserAndProvider(ctx, userID, input.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing key: %w", err)
	}

	// Determine the encrypted key to use
	var encryptedKey string
	if input.APIKey != "" {
		// New key provided - encrypt it
		if s.encryptor != nil {
			encrypted, err := s.encryptor.Encrypt(input.APIKey)
			if err != nil {
				return nil, fmt.Errorf("failed to encrypt API key: %w", err)
			}
			encryptedKey = encrypted
		} else {
			encryptedKey = input.APIKey
		}
	} else if existing != nil {
		// Keep existing encrypted key
		encryptedKey = existing.APIKeyEncrypted
	}

	key := &models.UserServiceKey{
		UserID:          userID,
		Provider:        input.Provider,
		APIKeyEncrypted: encryptedKey,
		BaseURL:         input.BaseURL,
		IsEnabled:       input.IsEnabled,
	}

	if err := s.repos.UserServiceKey.Upsert(ctx, key); err != nil {
		return nil, err
	}

	s.logger.Info("user service key updated",
		"user_id", userID,
		"provider", input.Provider,
		"enabled", input.IsEnabled,
	)

	// Fetch the updated key to get timestamps
	return s.repos.UserServiceKey.GetByUserAndProvider(ctx, userID, input.Provider)
}

// DeleteServiceKey removes a user service key.
func (s *UserLLMService) DeleteServiceKey(ctx context.Context, userID, keyID string) error {
	// Verify the key belongs to the user
	key, err := s.repos.UserServiceKey.GetByID(ctx, keyID)
	if err != nil {
		return fmt.Errorf("failed to get service key: %w", err)
	}
	if key == nil {
		return fmt.Errorf("service key not found")
	}
	if key.UserID != userID {
		return fmt.Errorf("service key not found")
	}

	if err := s.repos.UserServiceKey.Delete(ctx, keyID); err != nil {
		return err
	}

	s.logger.Info("user service key deleted",
		"user_id", userID,
		"key_id", keyID,
		"provider", key.Provider,
	)
	return nil
}

// GetFallbackChain returns the user's fallback chain configuration.
func (s *UserLLMService) GetFallbackChain(ctx context.Context, userID string) ([]*models.UserFallbackChainEntry, error) {
	return s.repos.UserFallbackChain.GetByUserID(ctx, userID)
}

// UserFallbackChainEntryInput represents a single entry in the user's fallback chain input.
type UserFallbackChainEntryInput struct {
	Provider    string   `json:"provider"`
	Model       string   `json:"model"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	IsEnabled   bool     `json:"is_enabled"`
}

// SetFallbackChain replaces the user's fallback chain configuration.
func (s *UserLLMService) SetFallbackChain(ctx context.Context, userID string, entries []UserFallbackChainEntryInput) ([]*models.UserFallbackChainEntry, error) {
	modelEntries := make([]*models.UserFallbackChainEntry, 0, len(entries))

	for i, e := range entries {
		// Validate provider
		if !isValidUserProvider(e.Provider) {
			return nil, fmt.Errorf("invalid provider at position %d: %s (must be openrouter, anthropic, openai, or ollama)", i+1, e.Provider)
		}

		// Validate model is not empty
		if e.Model == "" {
			return nil, fmt.Errorf("model at position %d cannot be empty", i+1)
		}

		modelEntries = append(modelEntries, &models.UserFallbackChainEntry{
			UserID:      userID,
			Position:    i + 1,
			Provider:    e.Provider,
			Model:       e.Model,
			Temperature: e.Temperature,
			MaxTokens:   e.MaxTokens,
			IsEnabled:   e.IsEnabled,
		})
	}

	if err := s.repos.UserFallbackChain.ReplaceAll(ctx, userID, modelEntries); err != nil {
		return nil, fmt.Errorf("failed to update fallback chain: %w", err)
	}

	s.logger.Info("user fallback chain updated",
		"user_id", userID,
		"entries", len(entries),
	)

	return s.repos.UserFallbackChain.GetByUserID(ctx, userID)
}

// GetDecryptedKey returns the decrypted API key for a provider.
// This is used internally by the extraction service.
func (s *UserLLMService) GetDecryptedKey(ctx context.Context, userID, provider string) (string, error) {
	key, err := s.repos.UserServiceKey.GetByUserAndProvider(ctx, userID, provider)
	if err != nil {
		return "", fmt.Errorf("failed to get service key: %w", err)
	}
	if key == nil || key.APIKeyEncrypted == "" {
		return "", nil
	}
	if !key.IsEnabled {
		return "", nil
	}

	if s.encryptor != nil {
		decrypted, err := s.encryptor.Decrypt(key.APIKeyEncrypted)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt API key: %w", err)
		}
		return decrypted, nil
	}

	return key.APIKeyEncrypted, nil
}

// isValidUserProvider checks if a provider name is valid for user keys.
func isValidUserProvider(provider string) bool {
	switch provider {
	case "openrouter", "anthropic", "openai", "ollama":
		return true
	default:
		return false
	}
}
