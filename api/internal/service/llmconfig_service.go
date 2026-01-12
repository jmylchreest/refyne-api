package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// LLMConfigService handles LLM configuration operations.
type LLMConfigService struct {
	cfg       *config.Config
	repos     *repository.Repositories
	logger    *slog.Logger
	encryptor *crypto.Encryptor
}

// NewLLMConfigService creates a new LLM config service.
func NewLLMConfigService(cfg *config.Config, repos *repository.Repositories, logger *slog.Logger) (*LLMConfigService, error) {
	encryptor, err := crypto.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}

	return &LLMConfigService{
		cfg:       cfg,
		repos:     repos,
		logger:    logger,
		encryptor: encryptor,
	}, nil
}

// LLMConfigOutput represents LLM config output.
type LLMConfigOutput struct {
	Provider   string `json:"provider"`
	HasAPIKey  bool   `json:"has_api_key"`
	BaseURL    string `json:"base_url,omitempty"`
	Model      string `json:"model,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// UpdateLLMConfigInput represents input for updating LLM config.
type UpdateLLMConfigInput struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
	Model    string `json:"model,omitempty"`
}

// GetConfig retrieves a user's LLM configuration.
func (s *LLMConfigService) GetConfig(ctx context.Context, userID string) (*LLMConfigOutput, error) {
	cfg, err := s.repos.LLMConfig.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM config: %w", err)
	}

	if cfg == nil {
		return &LLMConfigOutput{
			Provider: "credits", // Default to credits mode
		}, nil
	}

	return &LLMConfigOutput{
		Provider:  cfg.Provider,
		HasAPIKey: cfg.APIKeyEncrypted != "",
		BaseURL:   cfg.BaseURL,
		Model:     cfg.Model,
		UpdatedAt: cfg.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// UpdateConfig updates a user's LLM configuration.
func (s *LLMConfigService) UpdateConfig(ctx context.Context, userID string, input UpdateLLMConfigInput) (*LLMConfigOutput, error) {
	// Get existing config to preserve encrypted key if not updating
	existing, _ := s.repos.LLMConfig.GetByUserID(ctx, userID)

	var apiKeyEncrypted string
	if input.APIKey != "" {
		// Encrypt the new API key
		encrypted, err := s.encryptor.Encrypt(input.APIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt API key: %w", err)
		}
		apiKeyEncrypted = encrypted
	} else if existing != nil {
		// Preserve existing encrypted key if no new key provided
		apiKeyEncrypted = existing.APIKeyEncrypted
	}

	now := time.Now()
	cfg := &models.LLMConfig{
		ID:              ulid.Make().String(),
		UserID:          userID,
		Provider:        input.Provider,
		APIKeyEncrypted: apiKeyEncrypted,
		BaseURL:         input.BaseURL,
		Model:           input.Model,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.repos.LLMConfig.Upsert(ctx, cfg); err != nil {
		return nil, fmt.Errorf("failed to update LLM config: %w", err)
	}

	return &LLMConfigOutput{
		Provider:  cfg.Provider,
		HasAPIKey: cfg.APIKeyEncrypted != "",
		BaseURL:   cfg.BaseURL,
		Model:     cfg.Model,
		UpdatedAt: cfg.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// GetDecryptedAPIKey retrieves and decrypts a user's LLM API key.
// This should only be called when actually making LLM requests.
func (s *LLMConfigService) GetDecryptedAPIKey(ctx context.Context, userID string) (string, error) {
	cfg, err := s.repos.LLMConfig.GetByUserID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get LLM config: %w", err)
	}
	if cfg == nil || cfg.APIKeyEncrypted == "" {
		return "", nil
	}

	decrypted, err := s.encryptor.Decrypt(cfg.APIKeyEncrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt API key: %w", err)
	}

	return decrypted, nil
}
