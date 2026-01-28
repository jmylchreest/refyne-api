package service

import (
	"context"
	"log/slog"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// ServiceKeys holds decrypted service API keys for LLM providers.
type ServiceKeys struct {
	OpenRouterKey string
	AnthropicKey  string
	OpenAIKey     string
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

// SetPricingService sets the pricing service for dynamic max_completion_tokens lookups.
func (r *LLMConfigResolver) SetPricingService(pricing *PricingService) {
	r.pricing = pricing
}

// GetServiceKeys retrieves service keys, preferring DB over env vars.
func (r *LLMConfigResolver) GetServiceKeys(ctx context.Context) ServiceKeys {
	keys := ServiceKeys{}

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

				switch k.Provider {
				case llm.ProviderOpenRouter:
					keys.OpenRouterKey = apiKey
					r.logger.Debug("loaded OpenRouter key from database", "key_length", len(apiKey))
				case llm.ProviderAnthropic:
					keys.AnthropicKey = apiKey
					r.logger.Debug("loaded Anthropic key from database", "key_length", len(apiKey))
				case llm.ProviderOpenAI:
					keys.OpenAIKey = apiKey
					r.logger.Debug("loaded OpenAI key from database", "key_length", len(apiKey))
				}
			}
		}
	}

	// Fall back to env vars for any missing keys
	if keys.OpenRouterKey == "" {
		keys.OpenRouterKey = r.cfg.ServiceOpenRouterKey
		if keys.OpenRouterKey != "" {
			r.logger.Debug("using OpenRouter key from environment variable")
		}
	}
	if keys.AnthropicKey == "" {
		keys.AnthropicKey = r.cfg.ServiceAnthropicKey
		if keys.AnthropicKey != "" {
			r.logger.Debug("using Anthropic key from environment variable")
		}
	}
	if keys.OpenAIKey == "" {
		keys.OpenAIKey = r.cfg.ServiceOpenAIKey
		if keys.OpenAIKey != "" {
			r.logger.Debug("using OpenAI key from environment variable")
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

// MaxOutputTokensCap is the maximum output tokens we'll request from any model.
// This prevents requesting more output tokens than the model can handle when
// max_completion_tokens is very high (e.g., 131072 for some models where it
// matches the context window size, causing input+output > context_length errors).
// 32768 is reasonable for extraction tasks and well within most models' capabilities.
const MaxOutputTokensCap = 32768

// GetMaxTokens returns the recommended max tokens for a model.
// Priority: 1) chain config override (S3), 2) OpenRouter API max_completion_tokens, 3) provider defaults
// If chainMaxTokens is provided (from S3 fallback chain), it takes highest precedence.
// All values are capped to MaxOutputTokensCap to prevent context overflow errors.
func (r *LLMConfigResolver) GetMaxTokens(ctx context.Context, provider, model string, chainMaxTokens *int) int {
	// If chain config explicitly sets MaxTokens, use that (highest priority)
	// Chain config is intentional so we respect it, but still cap to prevent errors
	if chainMaxTokens != nil && *chainMaxTokens > 0 {
		if *chainMaxTokens > MaxOutputTokensCap {
			r.logger.Debug("capping chain max_tokens to prevent context overflow",
				"model", model,
				"requested", *chainMaxTokens,
				"capped_to", MaxOutputTokensCap,
			)
			return MaxOutputTokensCap
		}
		return *chainMaxTokens
	}

	// For OpenRouter, check the cached max_completion_tokens from the API
	// This provides dynamic, auto-updating token limits without S3 config
	if provider == llm.ProviderOpenRouter && r.pricing != nil {
		if apiMaxTokens := r.pricing.GetMaxCompletionTokens(provider, model); apiMaxTokens > 0 {
			// Cap to prevent context overflow (e.g., GLM 4.5 Air reports 131072 which
			// equals its context window, causing input+output > context_length errors)
			if apiMaxTokens > MaxOutputTokensCap {
				r.logger.Debug("capping OpenRouter API max_completion_tokens to prevent context overflow",
					"model", model,
					"api_max_completion_tokens", apiMaxTokens,
					"capped_to", MaxOutputTokensCap,
				)
				return MaxOutputTokensCap
			}
			r.logger.Debug("using OpenRouter API max_completion_tokens",
				"model", model,
				"max_completion_tokens", apiMaxTokens,
			)
			return apiMaxTokens
		}
	}

	// Fall back to S3/static model settings
	_, maxTokens, _ := llm.GetModelSettings(provider, model, nil, nil, nil)
	return maxTokens
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
		r.logger.Info("using override config (BYOK)",
			"user_id", userID,
			"provider", override.Provider,
			"model", override.Model,
		)
		return []*LLMConfigInput{override}, true
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
			Provider:   entry.Provider,
			APIKey:     apiKey,
			BaseURL:    baseURL,
			Model:      entry.Model,
			MaxTokens:  r.GetMaxTokens(ctx, entry.Provider, entry.Model, entry.MaxTokens),
			StrictMode: r.GetStrictMode(ctx, entry.Provider, entry.Model, entry.StrictMode),
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
			Provider:   entry.Provider,
			Model:      entry.Model,
			MaxTokens:  r.GetMaxTokens(ctx, entry.Provider, entry.Model, entry.MaxTokens),
			StrictMode: r.GetStrictMode(ctx, entry.Provider, entry.Model, entry.StrictMode),
		}

		// Use SYSTEM keys for the provider
		switch entry.Provider {
		case llm.ProviderOpenRouter:
			config.APIKey = serviceKeys.OpenRouterKey
		case llm.ProviderAnthropic:
			config.APIKey = serviceKeys.AnthropicKey
		case llm.ProviderOpenAI:
			config.APIKey = serviceKeys.OpenAIKey
		case llm.ProviderOllama:
			// Ollama doesn't require a key
		}

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
					Provider:   entry.Provider,
					APIKey:     apiKey,
					BaseURL:    userKey.BaseURL,
					Model:      entry.Model,
					MaxTokens:  r.GetMaxTokens(ctx, entry.Provider, entry.Model, entry.MaxTokens),
					StrictMode: r.GetStrictMode(ctx, entry.Provider, entry.Model, entry.StrictMode),
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
				"has_openrouter", serviceKeys.OpenRouterKey != "",
				"has_anthropic", serviceKeys.AnthropicKey != "",
				"has_openai", serviceKeys.OpenAIKey != "",
				"chain_entries", len(chain),
			)
			configs := make([]*LLMConfigInput, 0, len(chain))
			skippedNoKey := 0

			for _, entry := range chain {
				config := &LLMConfigInput{
					Provider:   entry.Provider,
					Model:      entry.Model,
					MaxTokens:  r.GetMaxTokens(ctx, entry.Provider, entry.Model, entry.MaxTokens),
					StrictMode: r.GetStrictMode(ctx, entry.Provider, entry.Model, entry.StrictMode),
				}

				switch entry.Provider {
				case llm.ProviderOpenRouter:
					config.APIKey = serviceKeys.OpenRouterKey
				case llm.ProviderAnthropic:
					config.APIKey = serviceKeys.AnthropicKey
				case llm.ProviderOpenAI:
					config.APIKey = serviceKeys.OpenAIKey
				}

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

