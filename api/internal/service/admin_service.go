package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jmylchreest/refyne-api/internal/auth"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// AdminService handles admin operations.
type AdminService struct {
	repos       *repository.Repositories
	encryptor   *crypto.Encryptor
	clerkClient *auth.ClerkBackendClient
	logger      *slog.Logger
}

// NewAdminService creates a new admin service.
func NewAdminService(repos *repository.Repositories, encryptor *crypto.Encryptor, logger *slog.Logger) *AdminService {
	return &AdminService{
		repos:     repos,
		encryptor: encryptor,
		logger:    logger,
	}
}

// NewAdminServiceWithClerk creates a new admin service with Clerk integration.
func NewAdminServiceWithClerk(repos *repository.Repositories, encryptor *crypto.Encryptor, clerkSecretKey string, logger *slog.Logger) *AdminService {
	var clerkClient *auth.ClerkBackendClient
	if clerkSecretKey != "" {
		clerkClient = auth.NewClerkBackendClient(clerkSecretKey)
	}

	return &AdminService{
		repos:       repos,
		encryptor:   encryptor,
		clerkClient: clerkClient,
		logger:      logger,
	}
}

// ServiceKeyInput represents input for creating/updating a service key.
type ServiceKeyInput struct {
	Provider     string
	APIKey       string
	DefaultModel string
	IsEnabled    bool
}

// ListServiceKeys returns all configured service keys.
func (s *AdminService) ListServiceKeys(ctx context.Context) ([]*models.ServiceKey, error) {
	return s.repos.ServiceKey.GetAll(ctx)
}

// UpsertServiceKey creates or updates a service key.
// If APIKey is empty and a key already exists, the existing key is preserved.
// If APIKey is empty and no key exists, an error is returned.
func (s *AdminService) UpsertServiceKey(ctx context.Context, input ServiceKeyInput) (*models.ServiceKey, error) {
	// Validate provider
	if !isValidProvider(input.Provider) {
		return nil, fmt.Errorf("invalid provider: %s (must be openrouter, anthropic, or openai)", input.Provider)
	}

	// Set default model if not provided
	defaultModel := input.DefaultModel
	if defaultModel == "" {
		defaultModel = getDefaultModelForProvider(input.Provider)
	}

	// Check if key already exists
	existingKey, err := s.repos.ServiceKey.GetByProvider(ctx, input.Provider)
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
	} else if existingKey != nil {
		// No new key provided but existing key exists - preserve it
		encryptedKey = existingKey.APIKeyEncrypted
	} else {
		// No new key and no existing key - error
		return nil, fmt.Errorf("API key is required when creating a new service key")
	}

	key := &models.ServiceKey{
		Provider:        input.Provider,
		APIKeyEncrypted: encryptedKey,
		DefaultModel:    defaultModel,
		IsEnabled:       input.IsEnabled,
	}

	if err := s.repos.ServiceKey.Upsert(ctx, key); err != nil {
		return nil, err
	}

	s.logger.Info("service key updated",
		"provider", input.Provider,
		"model", defaultModel,
		"enabled", input.IsEnabled,
		"key_changed", input.APIKey != "",
	)

	// Fetch the updated key to get timestamps
	return s.repos.ServiceKey.GetByProvider(ctx, input.Provider)
}

// DeleteServiceKey removes a service key.
func (s *AdminService) DeleteServiceKey(ctx context.Context, provider string) error {
	if err := s.repos.ServiceKey.Delete(ctx, provider); err != nil {
		return err
	}

	s.logger.Info("service key deleted", "provider", provider)
	return nil
}

// FallbackChainInput represents input for replacing the fallback chain.
type FallbackChainInput struct {
	Tier    *string                   `json:"tier,omitempty"` // nil for default chain
	Entries []FallbackChainEntryInput `json:"entries"`
}

// FallbackChainEntryInput represents a single entry in the fallback chain input.
type FallbackChainEntryInput struct {
	Provider    string   `json:"provider"`
	Model       string   `json:"model"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	IsEnabled   bool     `json:"is_enabled"`
}

// TierChainSummary represents a summary of chains per tier.
type TierChainSummary struct {
	Tier         *string `json:"tier"` // nil for default
	EntryCount   int     `json:"entry_count"`
	EnabledCount int     `json:"enabled_count"`
}

// GetFallbackChain returns all fallback chain entries grouped by tier.
func (s *AdminService) GetFallbackChain(ctx context.Context) ([]*models.FallbackChainEntry, error) {
	return s.repos.FallbackChain.GetAll(ctx)
}

// GetFallbackChainByTier returns fallback chain entries for a specific tier.
// Pass nil for the default chain.
func (s *AdminService) GetFallbackChainByTier(ctx context.Context, tier *string) ([]*models.FallbackChainEntry, error) {
	return s.repos.FallbackChain.GetByTier(ctx, tier)
}

// GetAllTiers returns a list of all tiers with custom chains configured.
func (s *AdminService) GetAllTiers(ctx context.Context) ([]string, error) {
	return s.repos.FallbackChain.GetAllTiers(ctx)
}

// SetFallbackChain replaces the fallback chain for a specific tier.
// If tier is nil, updates the default chain.
func (s *AdminService) SetFallbackChain(ctx context.Context, input FallbackChainInput) ([]*models.FallbackChainEntry, error) {
	entries := make([]*models.FallbackChainEntry, 0, len(input.Entries))

	for i, e := range input.Entries {
		// Validate provider
		if !isValidChainProvider(e.Provider) {
			return nil, fmt.Errorf("invalid provider at position %d: %s (must be openrouter, anthropic, openai, or ollama)", i+1, e.Provider)
		}

		// Validate model is not empty
		if e.Model == "" {
			return nil, fmt.Errorf("model at position %d cannot be empty", i+1)
		}

		entries = append(entries, &models.FallbackChainEntry{
			Tier:        input.Tier,
			Position:    i + 1,
			Provider:    e.Provider,
			Model:       e.Model,
			Temperature: e.Temperature,
			MaxTokens:   e.MaxTokens,
			IsEnabled:   e.IsEnabled,
		})
	}

	if err := s.repos.FallbackChain.ReplaceAllByTier(ctx, input.Tier, entries); err != nil {
		return nil, fmt.Errorf("failed to update fallback chain: %w", err)
	}

	tierName := "default"
	if input.Tier != nil {
		tierName = *input.Tier
	}
	s.logger.Info("fallback chain updated", "tier", tierName, "entries", len(entries))

	return s.repos.FallbackChain.GetByTier(ctx, input.Tier)
}

// DeleteFallbackChainByTier removes all entries for a specific tier.
// This allows the tier to fall back to the default chain.
func (s *AdminService) DeleteFallbackChainByTier(ctx context.Context, tier string) error {
	if err := s.repos.FallbackChain.DeleteByTier(ctx, tier); err != nil {
		return fmt.Errorf("failed to delete fallback chain: %w", err)
	}

	s.logger.Info("fallback chain deleted", "tier", tier)
	return nil
}

// isValidProvider checks if a provider name is valid for service keys.
func isValidProvider(provider string) bool {
	switch provider {
	case "openrouter", "anthropic", "openai":
		return true
	default:
		return false
	}
}

// isValidChainProvider checks if a provider name is valid for the fallback chain.
// Includes ollama which doesn't require an API key.
func isValidChainProvider(provider string) bool {
	switch provider {
	case "openrouter", "anthropic", "openai", "ollama":
		return true
	default:
		return false
	}
}

// getDefaultModelForProvider returns the default model for a provider.
func getDefaultModelForProvider(provider string) string {
	switch provider {
	case "openrouter":
		return "xiaomi/mimo-v2-flash:free"
	case "anthropic":
		return "claude-sonnet-4-5-20250514"
	case "openai":
		return "gpt-4o-mini"
	default:
		return ""
	}
}

// ProviderModel represents a model available from a provider.
type ProviderModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsFree      bool   `json:"is_free"`
	ContextSize int    `json:"context_size,omitempty"`
}

// ListModels returns available models for a given provider.
func (s *AdminService) ListModels(ctx context.Context, provider string) ([]ProviderModel, error) {
	switch provider {
	case "openrouter":
		return s.listOpenRouterModels(ctx)
	case "anthropic":
		return s.listAnthropicModels(ctx)
	case "openai":
		return s.listOpenAIModels(ctx)
	case "ollama":
		return s.listOllamaModels(ctx)
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

// listOpenRouterModels fetches models from the OpenRouter API.
func (s *AdminService) listOpenRouterModels(ctx context.Context) ([]ProviderModel, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenRouter API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			Description   string `json:"description"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	models := make([]ProviderModel, 0, len(result.Data))
	for _, m := range result.Data {
		// Check if it's a free model (both prompt and completion pricing are "0")
		isFree := m.Pricing.Prompt == "0" && m.Pricing.Completion == "0"

		models = append(models, ProviderModel{
			ID:          m.ID,
			Name:        m.Name,
			Description: m.Description,
			IsFree:      isFree,
			ContextSize: m.ContextLength,
		})
	}

	// Sort by name
	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	return models, nil
}

// listAnthropicModels returns the known Anthropic models.
// Anthropic doesn't have a public models API, so we return a static list.
func (s *AdminService) listAnthropicModels(ctx context.Context) ([]ProviderModel, error) {
	models := []ProviderModel{
		{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", Description: "Most capable model for highly complex tasks", ContextSize: 200000},
		{ID: "claude-sonnet-4-5-20250514", Name: "Claude Sonnet 4.5", Description: "High intelligence and speed", ContextSize: 200000},
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Description: "Excellent code and reasoning", ContextSize: 200000},
		{ID: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet", Description: "Extended thinking capabilities", ContextSize: 200000},
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet v2", Description: "Best for most tasks", ContextSize: 200000},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", Description: "Fast and efficient", ContextSize: 200000},
		{ID: "claude-3-opus-20240229", Name: "Claude 3 Opus", Description: "Original Opus model", ContextSize: 200000},
		{ID: "claude-3-sonnet-20240229", Name: "Claude 3 Sonnet", Description: "Original Sonnet model", ContextSize: 200000},
		{ID: "claude-3-haiku-20240307", Name: "Claude 3 Haiku", Description: "Original Haiku model", ContextSize: 200000},
	}
	return models, nil
}

// listOpenAIModels fetches models from the OpenAI API if we have a key.
// Falls back to a static list if no key is available.
func (s *AdminService) listOpenAIModels(ctx context.Context) ([]ProviderModel, error) {
	// Try to get the service key for OpenAI
	serviceKey, err := s.repos.ServiceKey.GetByProvider(ctx, "openai")
	if err != nil || serviceKey == nil || serviceKey.APIKeyEncrypted == "" {
		// Return static list if no key available
		return s.getStaticOpenAIModels(), nil
	}

	// Decrypt the API key
	apiKey := serviceKey.APIKeyEncrypted
	if s.encryptor != nil {
		decrypted, err := s.encryptor.Decrypt(apiKey)
		if err != nil {
			s.logger.Warn("failed to decrypt OpenAI key, using static list", "error", err)
			return s.getStaticOpenAIModels(), nil
		}
		apiKey = decrypted
	}

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		s.logger.Warn("OpenAI API returned non-200, using static list", "status", resp.StatusCode)
		return s.getStaticOpenAIModels(), nil
	}

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Filter to only GPT models and format them nicely
	models := make([]ProviderModel, 0)
	for _, m := range result.Data {
		// Only include GPT/chat models
		if strings.HasPrefix(m.ID, "gpt-") || strings.HasPrefix(m.ID, "o1") || strings.HasPrefix(m.ID, "o3") {
			models = append(models, ProviderModel{
				ID:   m.ID,
				Name: formatOpenAIModelName(m.ID),
			})
		}
	}

	// Sort by name
	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	return models, nil
}

// getStaticOpenAIModels returns a static list of common OpenAI models.
func (s *AdminService) getStaticOpenAIModels() []ProviderModel {
	return []ProviderModel{
		{ID: "gpt-4o", Name: "GPT-4o", Description: "Most capable GPT-4 model", ContextSize: 128000},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", Description: "Affordable and intelligent small model", ContextSize: 128000},
		{ID: "gpt-4-turbo", Name: "GPT-4 Turbo", Description: "Fast GPT-4 with vision", ContextSize: 128000},
		{ID: "gpt-4", Name: "GPT-4", Description: "Original GPT-4", ContextSize: 8192},
		{ID: "gpt-3.5-turbo", Name: "GPT-3.5 Turbo", Description: "Fast and efficient", ContextSize: 16385},
		{ID: "o1", Name: "o1", Description: "Reasoning model", ContextSize: 200000},
		{ID: "o1-mini", Name: "o1 Mini", Description: "Fast reasoning model", ContextSize: 128000},
		{ID: "o3-mini", Name: "o3 Mini", Description: "Latest reasoning model", ContextSize: 200000},
	}
}

// formatOpenAIModelName formats an OpenAI model ID into a readable name.
func formatOpenAIModelName(id string) string {
	// Handle some common patterns
	name := strings.ReplaceAll(id, "-", " ")
	// Title case each word
	words := strings.Fields(name)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

// listOllamaModels fetches models from a local Ollama instance.
func (s *AdminService) listOllamaModels(ctx context.Context) ([]ProviderModel, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Try default Ollama endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:11434/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		// Ollama not running - return empty list with a common fallback
		s.logger.Debug("Ollama not available, returning default models", "error", err)
		return []ProviderModel{
			{ID: "llama3.2", Name: "Llama 3.2", Description: "Meta's Llama 3.2 model (not installed)"},
			{ID: "llama3.1", Name: "Llama 3.1", Description: "Meta's Llama 3.1 model (not installed)"},
			{ID: "mistral", Name: "Mistral", Description: "Mistral AI model (not installed)"},
			{ID: "gemma2", Name: "Gemma 2", Description: "Google's Gemma 2 model (not installed)"},
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API returned %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name       string `json:"name"`
			ModifiedAt string `json:"modified_at"`
			Size       int64  `json:"size"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	models := make([]ProviderModel, 0, len(result.Models))
	for _, m := range result.Models {
		// Strip the :latest tag if present for display
		name := strings.TrimSuffix(m.Name, ":latest")
		models = append(models, ProviderModel{
			ID:   m.Name,
			Name: name,
		})
	}

	// Sort by name
	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	return models, nil
}

// ModelStatus represents the validation status of a model.
type ModelStatus string

const (
	ModelStatusValid      ModelStatus = "valid"
	ModelStatusNotFound   ModelStatus = "not_found"
	ModelStatusDeprecated ModelStatus = "deprecated"
	ModelStatusUnknown    ModelStatus = "unknown"
)

// ModelValidation represents the validation result for a model.
type ModelValidation struct {
	Provider string      `json:"provider"`
	Model    string      `json:"model"`
	Status   ModelStatus `json:"status"`
	Message  string      `json:"message,omitempty"`
}

// ValidateModelsInput represents input for validating models.
type ValidateModelsInput struct {
	Models []struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	} `json:"models"`
}

// ValidateModels checks if the given provider:model pairs exist and are valid.
func (s *AdminService) ValidateModels(ctx context.Context, input ValidateModelsInput) ([]ModelValidation, error) {
	results := make([]ModelValidation, 0, len(input.Models))

	// Cache models per provider to avoid repeated API calls
	modelCache := make(map[string][]ProviderModel)

	for _, m := range input.Models {
		validation := ModelValidation{
			Provider: m.Provider,
			Model:    m.Model,
			Status:   ModelStatusUnknown,
		}

		// Get models for this provider (from cache or fetch)
		providerModels, ok := modelCache[m.Provider]
		if !ok {
			var err error
			providerModels, err = s.ListModels(ctx, m.Provider)
			if err != nil {
				validation.Status = ModelStatusUnknown
				validation.Message = "Could not fetch models for provider"
				results = append(results, validation)
				continue
			}
			modelCache[m.Provider] = providerModels
		}

		// Check if model exists
		found := false
		for _, pm := range providerModels {
			if pm.ID == m.Model {
				found = true
				validation.Status = ModelStatusValid
				break
			}
		}

		if !found {
			// For Anthropic and OpenAI, we have static lists so unfound models might still work
			// For OpenRouter, we fetch the full list so unfound means it doesn't exist
			switch m.Provider {
			case "openrouter":
				validation.Status = ModelStatusNotFound
				validation.Message = "Model not found in OpenRouter catalog"
			case "ollama":
				validation.Status = ModelStatusUnknown
				validation.Message = "Model may need to be pulled locally"
			default:
				// Anthropic/OpenAI - could be a valid model we don't have in our static list
				validation.Status = ModelStatusUnknown
				validation.Message = "Model not in known list, but may still be valid"
			}
		}

		results = append(results, validation)
	}

	return results, nil
}

// SubscriptionTier represents a subscription tier from Clerk.
type SubscriptionTier struct {
	ID          string `json:"id"`   // Clerk product ID (cprod_xxx)
	Name        string `json:"name"` // Display name
	Slug        string `json:"slug"` // Slug used as tier identifier
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

// ListSubscriptionTiers returns all subscription tiers from Clerk.
func (s *AdminService) ListSubscriptionTiers(ctx context.Context) ([]SubscriptionTier, error) {
	if s.clerkClient == nil {
		s.logger.Warn("Clerk client not configured, returning hardcoded tiers")
		return s.getHardcodedTiers(), nil
	}

	products, err := s.clerkClient.ListSubscriptionProducts(ctx)
	if err != nil {
		s.logger.Warn("failed to fetch subscription products from Clerk", "error", err)
		return s.getHardcodedTiers(), nil
	}

	tiers := make([]SubscriptionTier, 0, len(products))
	for _, p := range products {
		tiers = append(tiers, SubscriptionTier{
			ID:          p.ID,
			Name:        p.Name,
			Slug:        p.Slug,
			Description: p.Description,
			IsDefault:   p.IsDefault,
		})
	}

	return tiers, nil
}

// ValidateTierExists checks if a tier exists in Clerk.
// Returns (exists, currentSlug, error).
func (s *AdminService) ValidateTierExists(ctx context.Context, tierID string) (bool, string, error) {
	if s.clerkClient == nil {
		// Without Clerk, we just check against hardcoded tiers
		for _, t := range s.getHardcodedTiers() {
			if t.ID == tierID || t.Slug == tierID {
				return true, t.Slug, nil
			}
		}
		return false, "", nil
	}

	return s.clerkClient.ValidateProductExists(ctx, tierID)
}

// getHardcodedTiers returns a fallback list of subscription tiers.
// Used when Clerk is not configured or API calls fail.
func (s *AdminService) getHardcodedTiers() []SubscriptionTier {
	return []SubscriptionTier{
		{ID: "free", Name: "Free", Slug: "free", Description: "Free tier with limited features", IsDefault: true},
		{ID: "pro", Name: "Pro", Slug: "pro", Description: "Professional tier with advanced features"},
		{ID: "enterprise", Name: "Enterprise", Slug: "enterprise", Description: "Enterprise tier with all features"},
	}
}

// TierValidation represents the validation result for a tier.
type TierValidation struct {
	TierID      string `json:"tier_id"`
	CurrentSlug string `json:"current_slug,omitempty"`
	Status      string `json:"status"` // valid, not_found, unknown
	Message     string `json:"message,omitempty"`
}

// ValidateTiers checks if the given tier IDs exist and returns their current slugs.
func (s *AdminService) ValidateTiers(ctx context.Context, tierIDs []string) ([]TierValidation, error) {
	results := make([]TierValidation, 0, len(tierIDs))

	for _, tierID := range tierIDs {
		validation := TierValidation{
			TierID: tierID,
			Status: "unknown",
		}

		exists, slug, err := s.ValidateTierExists(ctx, tierID)
		if err != nil {
			validation.Message = "Failed to validate tier: " + err.Error()
		} else if exists {
			validation.Status = "valid"
			validation.CurrentSlug = slug
		} else {
			validation.Status = "not_found"
			validation.Message = "Tier not found in Clerk"
		}

		results = append(results, validation)
	}

	return results, nil
}
