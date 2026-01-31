package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// ServiceKeys holds decrypted service API keys for all LLM providers.
// Uses a map for provider-agnostic key storage.
type ServiceKeys struct {
	keys map[string]string // provider name -> decrypted key

	// Legacy fields for backward compatibility (deprecated, use Get/Set methods)
	OpenRouterKey string
	AnthropicKey  string
	OpenAIKey     string
}

// NewServiceKeys creates a new ServiceKeys instance.
func NewServiceKeys() *ServiceKeys {
	return &ServiceKeys{keys: make(map[string]string)}
}

// Get returns the API key for a provider.
func (sk *ServiceKeys) Get(provider string) string {
	if sk.keys != nil {
		if key, ok := sk.keys[provider]; ok {
			return key
		}
	}
	// Fallback to legacy fields for backward compatibility
	switch provider {
	case llm.ProviderOpenRouter:
		return sk.OpenRouterKey
	case llm.ProviderAnthropic:
		return sk.AnthropicKey
	case llm.ProviderOpenAI:
		return sk.OpenAIKey
	}
	return ""
}

// Set stores an API key for a provider.
func (sk *ServiceKeys) Set(provider, key string) {
	if sk.keys == nil {
		sk.keys = make(map[string]string)
	}
	sk.keys[provider] = key
	// Also set legacy fields for backward compatibility
	switch provider {
	case llm.ProviderOpenRouter:
		sk.OpenRouterKey = key
	case llm.ProviderAnthropic:
		sk.AnthropicKey = key
	case llm.ProviderOpenAI:
		sk.OpenAIKey = key
	}
}

// Has returns true if a key exists and is non-empty for the provider.
func (sk *ServiceKeys) Has(provider string) bool {
	key := sk.Get(provider)
	return key != ""
}

// LLMConfigResolver handles LLM configuration resolution for all services.
// It consolidates the logic for resolving API keys, building fallback chains,
// and determining model capabilities across ExtractionService and AnalyzerService.
type LLMConfigResolver struct {
	cfg       *config.Config
	repos     *repository.Repositories
	registry  *llm.Registry
	pricing   *PricingService // For dynamic max_completion_tokens
	encryptor *crypto.Encryptor
	logger    *slog.Logger
}

// NewLLMConfigResolver creates a new LLM config resolver.
func NewLLMConfigResolver(cfg *config.Config, repos *repository.Repositories, encryptor *crypto.Encryptor, logger *slog.Logger) *LLMConfigResolver {
	return &LLMConfigResolver{
		cfg:       cfg,
		repos:     repos,
		encryptor: encryptor,
		logger:    logger,
	}
}

// SetRegistry sets the LLM provider registry for capability lookups.
func (r *LLMConfigResolver) SetRegistry(registry *llm.Registry) {
	r.registry = registry
}

// GetRegistry returns the LLM provider registry.
func (r *LLMConfigResolver) GetRegistry() *llm.Registry {
	return r.registry
}

// SetPricingService sets the pricing service for dynamic max_completion_tokens lookups.
func (r *LLMConfigResolver) SetPricingService(pricing *PricingService) {
	r.pricing = pricing
}

// GetServiceKeys retrieves service keys, preferring DB over env vars.
func (r *LLMConfigResolver) GetServiceKeys(ctx context.Context) *ServiceKeys {
	keys := NewServiceKeys()

	// Try to load from database first
	if r.repos != nil && r.repos.ServiceKey != nil {
		dbKeys, err := r.repos.ServiceKey.GetAll(ctx)
		if err != nil {
			r.logger.Warn("failed to retrieve service keys from database, will fall back to env vars",
				"error", err,
			)
		} else {
			for _, k := range dbKeys {
				// Decrypt the key if we have an encryptor
				apiKey := k.APIKeyEncrypted
				if r.encryptor != nil && k.APIKeyEncrypted != "" {
					decrypted, decryptErr := r.encryptor.Decrypt(k.APIKeyEncrypted)
					if decryptErr != nil {
						r.logger.Warn("failed to decrypt service key, skipping",
							"provider", k.Provider,
							"error", decryptErr,
						)
						continue
					}
					apiKey = decrypted
				}

				// Use Set method for all providers (provider-agnostic)
				keys.Set(k.Provider, apiKey)
				r.logger.Debug("loaded service key from database",
					"provider", k.Provider,
					"key_length", len(apiKey),
				)
			}
		}
	}

	// Fall back to env vars for any missing keys
	// Map of provider -> env var value
	envMappings := map[string]string{
		llm.ProviderOpenRouter: r.cfg.ServiceOpenRouterKey,
		llm.ProviderAnthropic:  r.cfg.ServiceAnthropicKey,
		llm.ProviderOpenAI:     r.cfg.ServiceOpenAIKey,
		llm.ProviderHelicone:   r.cfg.ServiceHeliconeKey,
	}

	for provider, envKey := range envMappings {
		if !keys.Has(provider) && envKey != "" {
			keys.Set(provider, envKey)
			r.logger.Debug("using service key from environment variable",
				"provider", provider,
			)
		}
	}

	return keys
}

// GetStrictMode determines if a model supports strict JSON schema mode.
// Uses the registry's cached capabilities when available.
// If chainStrictMode is provided, it takes precedence.
func (r *LLMConfigResolver) GetStrictMode(ctx context.Context, provider, model string, chainStrictMode *bool) bool {
	// If chain config explicitly sets StrictMode, use that (highest priority)
	if chainStrictMode != nil {
		return *chainStrictMode
	}

	// Use registry for capability detection if available
	if r.registry != nil {
		return r.registry.SupportsStructuredOutputs(ctx, provider, model)
	}

	// Fall back to static defaults
	_, _, strictMode := llm.GetModelSettings(provider, model, nil, nil, nil)
	return strictMode
}

// SupportsResponseFormat checks if a model supports the response_format parameter.
// This is weaker than strict structured outputs - it just hints JSON output.
// Uses cached capabilities from OpenRouter when available.
func (r *LLMConfigResolver) SupportsResponseFormat(ctx context.Context, provider, model string) bool {
	if r.registry != nil {
		caps := r.registry.GetModelCapabilities(ctx, provider, model)
		return caps.SupportsResponseFormat
	}
	// Assume support for OpenAI-compatible providers
	return provider == "openai" || provider == "openrouter"
}

// DefaultMaxOutputTokens is the fallback when model's max_completion_tokens is unknown.
// This is only used when we can't determine the actual limit from the provider.
// 16k is about the minimum that modern LLM models support.
const DefaultMaxOutputTokens = 16384

// GetMaxTokens returns the model's max output tokens capability.
// Priority: 1) chain config override (S3), 2) registry/provider API, 3) static defaults
// NOTE: This returns the model's capability, not necessarily what we'll use.
// Use CalculateDynamicMaxTokens at extraction time to get the effective limit
// based on actual input size and context window.
// GUARANTEE: This function always returns a non-zero value (DefaultMaxOutputTokens as fallback).
func (r *LLMConfigResolver) GetMaxTokens(ctx context.Context, provider, model string, chainMaxTokens *int) int {
	var source string
	var result int

	defer func() {
		// Log the lookup path for debugging max_tokens:0 issues
		if r.logger != nil {
			r.logger.Debug("GetMaxTokens resolved",
				"provider", provider,
				"model", model,
				"max_tokens", result,
				"source", source,
			)
		}
	}()

	// If chain config explicitly sets MaxTokens, use that (highest priority)
	if chainMaxTokens != nil && *chainMaxTokens > 0 {
		source = "chain_config"
		result = *chainMaxTokens
		return result
	}

	// Check the registry for cached max_completion_tokens (populated by provider APIs)
	// This works for ALL providers, not just OpenRouter
	if r.registry != nil {
		if apiMaxTokens := r.registry.GetMaxCompletionTokens(ctx, provider, model); apiMaxTokens > 0 {
			source = "registry"
			result = apiMaxTokens
			return result
		}
	}

	// Legacy fallback: check pricing service for OpenRouter (will be deprecated)
	if provider == llm.ProviderOpenRouter && r.pricing != nil {
		if apiMaxTokens := r.pricing.GetMaxCompletionTokens(provider, model); apiMaxTokens > 0 {
			source = "pricing_service"
			result = apiMaxTokens
			return result
		}
	}

	// Fall back to S3/static model settings
	_, maxTokens, _ := llm.GetModelSettings(provider, model, nil, nil, nil)
	if maxTokens > 0 {
		source = "static_defaults"
		result = maxTokens
		return result
	}

	// Ultimate fallback - ensure we NEVER return 0
	source = "default_fallback"
	result = DefaultMaxOutputTokens
	return result
}

// CalculateDynamicMaxTokens determines the effective max output tokens based on:
// - The model's context window
// - The estimated input tokens
// - The model's max_completion_tokens capability
// Returns the smaller of: (context - input - buffer) or configMaxTokens
// Also returns an error if input already exceeds the safe threshold.
func (r *LLMConfigResolver) CalculateDynamicMaxTokens(contextLength, estimatedInputTokens, configMaxTokens int) (int, error) {
	// If context length unknown, use config as-is (graceful fallback)
	if contextLength == 0 {
		return configMaxTokens, nil
	}

	// Check if input exceeds safe threshold (80% of context)
	maxInputTokens := int(float64(contextLength) * ContextCapacityThreshold)
	if estimatedInputTokens > maxInputTokens {
		return 0, fmt.Errorf("input tokens (%d) exceed safe threshold (%d, %.0f%% of %d context)",
			estimatedInputTokens, maxInputTokens, ContextCapacityThreshold*100, contextLength)
	}

	// Calculate available tokens for output (leave 5% buffer for safety)
	safetyBuffer := int(float64(contextLength) * 0.05)
	availableForOutput := contextLength - estimatedInputTokens - safetyBuffer

	// Use the smaller of available space or model's max capability
	effectiveMax := availableForOutput
	if configMaxTokens > 0 && configMaxTokens < effectiveMax {
		effectiveMax = configMaxTokens
	}

	// Ensure we have at least some room for output
	if effectiveMax < 1000 {
		return 0, fmt.Errorf("insufficient context for output: only %d tokens available after %d input (context: %d)",
			effectiveMax, estimatedInputTokens, contextLength)
	}

	r.logger.Debug("calculated dynamic max tokens",
		"context_length", contextLength,
		"input_tokens", estimatedInputTokens,
		"config_max_tokens", configMaxTokens,
		"effective_max_tokens", effectiveMax,
	)

	return effectiveMax, nil
}

// ContextCapacityThreshold is the maximum percentage of context window that input can use.
// If input tokens exceed this threshold, we fail fast rather than wasting an API call.
// 80% leaves room for output tokens and avoids context overflow errors.
const ContextCapacityThreshold = 0.80

// GetContextLength returns the context window size for a model.
// Returns 0 if unknown (no pre-validation will be performed).
func (r *LLMConfigResolver) GetContextLength(ctx context.Context, provider, model string) int {
	// Check the registry for cached context_length (populated by provider APIs)
	// This works for ALL providers, not just OpenRouter
	if r.registry != nil {
		if contextLen := r.registry.GetContextLength(ctx, provider, model); contextLen > 0 {
			return contextLen
		}
	}

	// Legacy fallback: check pricing service for OpenRouter (will be deprecated)
	if provider == llm.ProviderOpenRouter && r.pricing != nil {
		if contextLen := r.pricing.GetContextLength(provider, model); contextLen > 0 {
			return contextLen
		}
	}
	// Return 0 for unknown - no pre-validation possible
	return 0
}

// ValidateContextCapacity checks if the estimated input tokens fit within the model's context window.
// Returns an error if input tokens exceed ContextCapacityThreshold (80%) of the context window.
// Returns nil if validation passes or if context length is unknown (graceful fallback).
func (r *LLMConfigResolver) ValidateContextCapacity(ctx context.Context, provider, model string, estimatedInputTokens int) error {
	contextLength := r.GetContextLength(ctx, provider, model)
	if contextLength == 0 {
		// Unknown context length - skip validation, let the API handle it
		return nil
	}

	maxInputTokens := int(float64(contextLength) * ContextCapacityThreshold)
	if estimatedInputTokens > maxInputTokens {
		return &ErrContextCapacityExceeded{
			Model:                model,
			ContextLength:        contextLength,
			EstimatedInputTokens: estimatedInputTokens,
			MaxInputTokens:       maxInputTokens,
			Threshold:            ContextCapacityThreshold,
		}
	}
	return nil
}

// ErrContextCapacityExceeded is returned when input tokens exceed the safe threshold of a model's context window.
// This allows fast-failing before making an expensive API call that would fail anyway.
type ErrContextCapacityExceeded struct {
	Model                string
	ContextLength        int
	EstimatedInputTokens int
	MaxInputTokens       int
	Threshold            float64
}

func (e *ErrContextCapacityExceeded) Error() string {
	return fmt.Sprintf(
		"input too large for model %s: estimated %d tokens exceeds %.0f%% capacity (%d/%d context window)",
		e.Model, e.EstimatedInputTokens, e.Threshold*100, e.MaxInputTokens, e.ContextLength,
	)
}

// ResolveConfigs determines which LLM configurations to use based on the feature matrix.
// Returns (configs, isBYOK) where isBYOK is true if using user's own keys.
//
// Feature matrix:
// - BYOK + models_custom: user keys + user chain
// - BYOK only: user keys + system chain
// - models_custom only: system keys + user chain
// - Neither: system keys + system chain
func (r *LLMConfigResolver) ResolveConfigs(ctx context.Context, userID string, override *LLMConfigInput, tier string, byokAllowed, modelsCustomAllowed bool) ([]*LLMConfigInput, bool) {
	r.logger.Debug("resolving LLM configs with feature matrix",
		"user_id", userID,
		"tier", tier,
		"byok_allowed", byokAllowed,
		"models_custom_allowed", modelsCustomAllowed,
	)

	// Case 1: Override provided - requires BYOK
	if override != nil && override.Provider != "" {
		if !byokAllowed {
			r.logger.Debug("override provided but BYOK not allowed, using system chain",
				"user_id", userID,
			)
			return r.GetDefaultConfigsForTier(ctx, tier), false
		}

		// Populate defaults for any missing fields
		resolvedOverride := &LLMConfigInput{
			Provider:       override.Provider,
			APIKey:         override.APIKey,
			BaseURL:        override.BaseURL,
			Model:          override.Model,
			TargetProvider: override.TargetProvider,
			TargetAPIKey:   override.TargetAPIKey,
		}

		// Use GetMaxTokens to get proper default if not specified
		if override.MaxTokens > 0 {
			resolvedOverride.MaxTokens = override.MaxTokens
		} else {
			resolvedOverride.MaxTokens = r.GetMaxTokens(ctx, override.Provider, override.Model, nil)
		}

		// Use GetContextLength to get proper default if not specified
		if override.ContextLength > 0 {
			resolvedOverride.ContextLength = override.ContextLength
		} else {
			resolvedOverride.ContextLength = r.GetContextLength(ctx, override.Provider, override.Model)
		}

		// Use GetStrictMode to get proper default
		resolvedOverride.StrictMode = r.GetStrictMode(ctx, override.Provider, override.Model, &override.StrictMode)

		r.logger.Info("using override config (BYOK)",
			"user_id", userID,
			"provider", resolvedOverride.Provider,
			"model", resolvedOverride.Model,
			"max_tokens", resolvedOverride.MaxTokens,
			"context_length", resolvedOverride.ContextLength,
		)
		return []*LLMConfigInput{resolvedOverride}, true
	}

	// Case 2: BYOK + models_custom - user chain with user keys
	if byokAllowed && modelsCustomAllowed {
		configs := r.BuildUserFallbackChain(ctx, userID)
		if len(configs) > 0 {
			r.logger.Info("using user fallback chain with user keys (BYOK + models_custom)",
				"user_id", userID,
				"provider", configs[0].Provider,
				"model", configs[0].Model,
			)
			return configs, true
		}
	}

	// Case 3: models_custom only - user chain with SYSTEM keys
	if modelsCustomAllowed && !byokAllowed {
		configs := r.BuildUserChainWithSystemKeys(ctx, userID, tier)
		if len(configs) > 0 {
			r.logger.Info("using user fallback chain with system keys (models_custom only)",
				"user_id", userID,
				"provider", configs[0].Provider,
				"model", configs[0].Model,
			)
			return configs, false
		}
	}

	// Case 4: BYOK only - check for user keys to use with system chain
	if byokAllowed && !modelsCustomAllowed {
		configs := r.buildBYOKOnlyConfigs(ctx, userID, tier)
		if len(configs) > 0 {
			r.logger.Info("using system chain with user keys (BYOK only)",
				"user_id", userID,
				"provider", configs[0].Provider,
				"model", configs[0].Model,
			)
			return configs, true
		}
	}

	// Case 5: Default - system chain with system keys
	configs := r.GetDefaultConfigsForTier(ctx, tier)
	if len(configs) > 0 {
		r.logger.Debug("using system default chain",
			"user_id", userID,
			"tier", tier,
			"provider", configs[0].Provider,
			"model", configs[0].Model,
		)
	}
	return configs, false
}

// LLMConfigChain provides iteration over fallback LLM configurations.
// It allows services to try each config in sequence until one succeeds.
type LLMConfigChain struct {
	configs []*LLMConfigInput
	isBYOK  bool
	index   int // 0 = not started, 1 = first config, etc.
}

// NewLLMConfigChain creates a new config chain from a slice of configs.
// Use this when you need to create a chain from custom logic (e.g., injected configs).
func NewLLMConfigChain(configs []*LLMConfigInput, isBYOK bool) *LLMConfigChain {
	return &LLMConfigChain{
		configs: configs,
		isBYOK:  isBYOK,
		index:   0,
	}
}

// ResolveConfigChain returns an iterator over LLM configurations for the fallback chain.
// Use Next() to iterate through configs, typically in a retry loop.
//
// Example usage:
//
//	chain := resolver.ResolveConfigChain(ctx, userID, nil, tier, byokAllowed, modelsCustomAllowed)
//	if chain.IsEmpty() {
//	    return nil, ErrNoModelsConfigured
//	}
//	for cfg := chain.Next(); cfg != nil; cfg = chain.Next() {
//	    result, err := doSomething(cfg)
//	    if err == nil {
//	        return result, nil
//	    }
//	    // Log error and continue to next config
//	}
func (r *LLMConfigResolver) ResolveConfigChain(ctx context.Context, userID string, override *LLMConfigInput, tier string, byokAllowed, modelsCustomAllowed bool) *LLMConfigChain {
	configs, isBYOK := r.ResolveConfigs(ctx, userID, override, tier, byokAllowed, modelsCustomAllowed)
	return &LLMConfigChain{
		configs: configs,
		isBYOK:  isBYOK,
		index:   0,
	}
}

// Next returns the next LLM config in the chain, or nil if exhausted.
// Call this in a loop to iterate through all fallback configs.
func (c *LLMConfigChain) Next() *LLMConfigInput {
	if c.index >= len(c.configs) {
		return nil
	}
	cfg := c.configs[c.index]
	c.index++
	return cfg
}

// Current returns the current config without advancing the iterator.
// Returns nil if Next() hasn't been called yet or the chain is exhausted.
func (c *LLMConfigChain) Current() *LLMConfigInput {
	if c.index == 0 || c.index > len(c.configs) {
		return nil
	}
	return c.configs[c.index-1]
}

// First returns the first config in the chain without advancing the iterator.
// Useful for pre-flight checks (e.g., cost estimation) before iterating.
// Returns nil if the chain is empty.
func (c *LLMConfigChain) First() *LLMConfigInput {
	if len(c.configs) == 0 {
		return nil
	}
	return c.configs[0]
}

// IsBYOK returns whether the chain uses the user's own API keys.
func (c *LLMConfigChain) IsBYOK() bool {
	return c.isBYOK
}

// Position returns the current position (1-indexed) and total count.
// Useful for logging "attempt 2 of 5" style messages.
// Returns (0, 0) if Next() hasn't been called yet.
func (c *LLMConfigChain) Position() (current, total int) {
	return c.index, len(c.configs)
}

// IsEmpty returns true if the chain has no configs.
func (c *LLMConfigChain) IsEmpty() bool {
	return len(c.configs) == 0
}

// Len returns the total number of configs in the chain.
func (c *LLMConfigChain) Len() int {
	return len(c.configs)
}

// All returns all configs in the chain as a slice.
// Use this when you need to serialize or store the configs for later use.
func (c *LLMConfigChain) All() []*LLMConfigInput {
	return c.configs
}

// BuildUserFallbackChain builds LLM configs from user's fallback chain using their own keys.
func (r *LLMConfigResolver) BuildUserFallbackChain(ctx context.Context, userID string) []*LLMConfigInput {
	if r.repos.UserFallbackChain == nil || r.repos.UserServiceKey == nil {
		return nil
	}

	// Get user's enabled fallback chain entries
	chain, err := r.repos.UserFallbackChain.GetEnabledByUserID(ctx, userID)
	if err != nil {
		r.logger.Warn("failed to get user fallback chain", "user_id", userID, "error", err)
		return nil
	}
	if len(chain) == 0 {
		return nil
	}

	// Get user's service keys
	keys, err := r.repos.UserServiceKey.GetEnabledByUserID(ctx, userID)
	if err != nil {
		r.logger.Warn("failed to get user service keys", "user_id", userID, "error", err)
		return nil
	}

	// Build a map of provider -> key
	keyMap := make(map[string]*models.UserServiceKey)
	for _, k := range keys {
		keyMap[k.Provider] = k
	}

	configs := make([]*LLMConfigInput, 0, len(chain))

	for _, entry := range chain {
		key, ok := keyMap[entry.Provider]
		if !ok && entry.Provider != llm.ProviderOllama {
			continue // No key for this provider (Ollama doesn't need one)
		}

		var apiKey, baseURL string
		if entry.Provider != llm.ProviderOllama {
			if key == nil || key.APIKeyEncrypted == "" {
				continue // No API key configured
			}
			// Decrypt the API key
			if r.encryptor != nil {
				decrypted, err := r.encryptor.Decrypt(key.APIKeyEncrypted)
				if err != nil {
					r.logger.Warn("failed to decrypt user API key",
						"user_id", userID,
						"provider", entry.Provider,
						"error", err,
					)
					continue
				}
				apiKey = decrypted
			} else {
				apiKey = key.APIKeyEncrypted
			}
			baseURL = key.BaseURL
		}

		configs = append(configs, &LLMConfigInput{
			Provider:      entry.Provider,
			APIKey:        apiKey,
			BaseURL:       baseURL,
			Model:         entry.Model,
			MaxTokens:     r.GetMaxTokens(ctx, entry.Provider, entry.Model, entry.MaxTokens),
			ContextLength: r.GetContextLength(ctx, entry.Provider, entry.Model),
			StrictMode:    r.GetStrictMode(ctx, entry.Provider, entry.Model, entry.StrictMode),
		})
	}

	return configs
}

// BuildUserChainWithSystemKeys builds LLM configs from user's chain using SYSTEM API keys.
// Used when user has models_custom feature but not provider_byok.
func (r *LLMConfigResolver) BuildUserChainWithSystemKeys(ctx context.Context, userID string, tier string) []*LLMConfigInput {
	if r.repos.UserFallbackChain == nil {
		return nil
	}

	// Get user's enabled fallback chain entries
	chain, err := r.repos.UserFallbackChain.GetEnabledByUserID(ctx, userID)
	if err != nil {
		r.logger.Warn("failed to get user fallback chain", "user_id", userID, "error", err)
		return nil
	}
	if len(chain) == 0 {
		return nil
	}

	serviceKeys := r.GetServiceKeys(ctx)
	configs := make([]*LLMConfigInput, 0, len(chain))

	for _, entry := range chain {
		config := &LLMConfigInput{
			Provider:      entry.Provider,
			Model:         entry.Model,
			MaxTokens:     r.GetMaxTokens(ctx, entry.Provider, entry.Model, entry.MaxTokens),
			ContextLength: r.GetContextLength(ctx, entry.Provider, entry.Model),
			StrictMode:    r.GetStrictMode(ctx, entry.Provider, entry.Model, entry.StrictMode),
		}

		// Use SYSTEM keys for the provider (provider-agnostic)
		config.APIKey = serviceKeys.Get(entry.Provider)

		// Skip if no system key available for non-Ollama providers
		if config.APIKey == "" && entry.Provider != llm.ProviderOllama {
			continue
		}

		configs = append(configs, config)
	}

	return configs
}

// buildBYOKOnlyConfigs handles the BYOK-only case where user has their own keys
// but should use the system chain (not custom models).
func (r *LLMConfigResolver) buildBYOKOnlyConfigs(ctx context.Context, userID string, tier string) []*LLMConfigInput {
	if r.repos.UserServiceKey == nil {
		return nil
	}

	// Get user's enabled service keys
	userKeys, err := r.repos.UserServiceKey.GetEnabledByUserID(ctx, userID)
	if err != nil || len(userKeys) == 0 {
		return nil
	}

	// Build map of user's providers
	userKeyMap := make(map[string]*models.UserServiceKey)
	for _, k := range userKeys {
		userKeyMap[k.Provider] = k
	}

	// Get system fallback chain for this tier
	var chain []*models.FallbackChainEntry
	if r.repos.FallbackChain != nil {
		if tier != "" {
			chain, _ = r.repos.FallbackChain.GetEnabledByTier(ctx, tier)
		}
		if len(chain) == 0 {
			chain, _ = r.repos.FallbackChain.GetEnabled(ctx)
		}
	}

	configs := make([]*LLMConfigInput, 0)

	// For each entry in system chain, try to use user's key for that provider
	for _, entry := range chain {
		userKey, hasUserKey := userKeyMap[entry.Provider]
		if hasUserKey && userKey.APIKeyEncrypted != "" {
			var apiKey string
			if r.encryptor != nil {
				decrypted, err := r.encryptor.Decrypt(userKey.APIKeyEncrypted)
				if err == nil {
					apiKey = decrypted
				}
			} else {
				apiKey = userKey.APIKeyEncrypted
			}

			if apiKey != "" {
				configs = append(configs, &LLMConfigInput{
					Provider:      entry.Provider,
					APIKey:        apiKey,
					BaseURL:       userKey.BaseURL,
					Model:         entry.Model,
					MaxTokens:     r.GetMaxTokens(ctx, entry.Provider, entry.Model, entry.MaxTokens),
					ContextLength: r.GetContextLength(ctx, entry.Provider, entry.Model),
					StrictMode:    r.GetStrictMode(ctx, entry.Provider, entry.Model, entry.StrictMode),
				})
			}
		}
	}

	return configs
}

// GetDefaultConfigsForTier returns the default LLM configs for a tier.
// Returns nil if no chain is configured for the tier.
// Fallback order: tier-specific -> default (NULL) -> free tier
func (r *LLMConfigResolver) GetDefaultConfigsForTier(ctx context.Context, tier string) []*LLMConfigInput {
	// Normalize tier name (e.g., "tier_v1_free" -> "free")
	normalizedTier := constants.NormalizeTierName(tier)

	r.logger.Debug("getting default configs for tier",
		"original_tier", tier,
		"normalized_tier", normalizedTier,
	)

	// Try to get from admin-configured fallback chain
	if r.repos != nil && r.repos.FallbackChain != nil {
		var chain []*models.FallbackChainEntry
		var err error

		// 1. Try tier-specific chain
		if normalizedTier != "" {
			chain, err = r.repos.FallbackChain.GetEnabledByTier(ctx, normalizedTier)
		}

		// 2. Fall back to default chain (tier IS NULL)
		if err != nil || len(chain) == 0 {
			chain, err = r.repos.FallbackChain.GetEnabled(ctx)
		}

		// 3. Fall back to "free" tier chain if nothing else found
		if (err != nil || len(chain) == 0) && normalizedTier != constants.TierFree {
			r.logger.Debug("falling back to free tier chain",
				"original_tier", normalizedTier,
			)
			chain, err = r.repos.FallbackChain.GetEnabledByTier(ctx, constants.TierFree)
		}

		if err == nil && len(chain) > 0 {
			serviceKeys := r.GetServiceKeys(ctx)
			r.logger.Debug("service keys status for fallback chain",
				"has_openrouter", serviceKeys.Has(llm.ProviderOpenRouter),
				"has_anthropic", serviceKeys.Has(llm.ProviderAnthropic),
				"has_openai", serviceKeys.Has(llm.ProviderOpenAI),
				"has_helicone", serviceKeys.Has(llm.ProviderHelicone),
				"chain_entries", len(chain),
			)
			configs := make([]*LLMConfigInput, 0, len(chain))
			skippedNoKey := 0

			for _, entry := range chain {
				config := &LLMConfigInput{
					Provider:      entry.Provider,
					Model:         entry.Model,
					MaxTokens:     r.GetMaxTokens(ctx, entry.Provider, entry.Model, entry.MaxTokens),
					ContextLength: r.GetContextLength(ctx, entry.Provider, entry.Model),
					StrictMode:    r.GetStrictMode(ctx, entry.Provider, entry.Model, entry.StrictMode),
				}

				// Use provider-agnostic key lookup
				config.APIKey = serviceKeys.Get(entry.Provider)

				if config.APIKey != "" || entry.Provider == llm.ProviderOllama {
					configs = append(configs, config)
				} else {
					skippedNoKey++
					r.logger.Debug("skipping fallback chain entry - no service key configured",
						"provider", entry.Provider,
						"model", entry.Model,
					)
				}
			}

			if len(configs) > 0 {
				return configs
			}

			// Chain exists but no usable configs (service keys not configured)
			if skippedNoKey > 0 {
				r.logger.Warn("fallback chain exists but no service keys configured",
					"tier", normalizedTier,
					"chain_entries", len(chain),
					"skipped_no_key", skippedNoKey,
				)
				return nil
			}
		}
	}

	// No fallback chain configured - return nil (not hardcoded defaults)
	r.logger.Warn("no fallback chain configured for tier",
		"tier", normalizedTier,
	)
	return nil
}

// GetDefaultConfig returns the first valid default config for a tier.
// Returns nil if no chain is configured for the tier.
func (r *LLMConfigResolver) GetDefaultConfig(ctx context.Context, tier string) *LLMConfigInput {
	configs := r.GetDefaultConfigsForTier(ctx, tier)
	if len(configs) > 0 {
		return configs[0]
	}
	// No fallback chain configured - return nil
	return nil
}

