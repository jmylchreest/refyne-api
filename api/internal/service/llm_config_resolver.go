package service

import (
	"context"
	"log/slog"

	"github.com/jmylchreest/refyne-api/internal/config"
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

// GetServiceKeys retrieves service keys, preferring DB over env vars.
func (r *LLMConfigResolver) GetServiceKeys(ctx context.Context) ServiceKeys {
	keys := ServiceKeys{}

	// Try to load from database first
	if r.repos != nil && r.repos.ServiceKey != nil {
		dbKeys, err := r.repos.ServiceKey.GetAll(ctx)
		if err == nil {
			for _, k := range dbKeys {
				// Decrypt the key if we have an encryptor
				apiKey := k.APIKeyEncrypted
				if r.encryptor != nil && k.APIKeyEncrypted != "" {
					if decrypted, err := r.encryptor.Decrypt(k.APIKeyEncrypted); err == nil {
						apiKey = decrypted
					}
				}

				switch k.Provider {
				case llm.ProviderOpenRouter:
					keys.OpenRouterKey = apiKey
				case llm.ProviderAnthropic:
					keys.AnthropicKey = apiKey
				case llm.ProviderOpenAI:
					keys.OpenAIKey = apiKey
				}
			}
		}
	}

	// Fall back to env vars for any missing keys
	if keys.OpenRouterKey == "" {
		keys.OpenRouterKey = r.cfg.ServiceOpenRouterKey
	}
	if keys.AnthropicKey == "" {
		keys.AnthropicKey = r.cfg.ServiceAnthropicKey
	}
	if keys.OpenAIKey == "" {
		keys.OpenAIKey = r.cfg.ServiceOpenAIKey
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

// ResolveConfig returns the first valid config from ResolveConfigs.
// This is a convenience method for services that only need a single config.
func (r *LLMConfigResolver) ResolveConfig(ctx context.Context, userID string, override *LLMConfigInput, tier string, byokAllowed, modelsCustomAllowed bool) (*LLMConfigInput, bool) {
	configs, isBYOK := r.ResolveConfigs(ctx, userID, override, tier, byokAllowed, modelsCustomAllowed)
	if len(configs) > 0 {
		return configs[0], isBYOK
	}
	return nil, false
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
					StrictMode: r.GetStrictMode(ctx, entry.Provider, entry.Model, entry.StrictMode),
				})
			}
		}
	}

	return configs
}

// GetDefaultConfigsForTier returns the default LLM configs for a tier.
func (r *LLMConfigResolver) GetDefaultConfigsForTier(ctx context.Context, tier string) []*LLMConfigInput {
	// First try to get from admin-configured fallback chain
	if r.repos != nil && r.repos.FallbackChain != nil {
		var chain []*models.FallbackChainEntry
		var err error

		if tier != "" {
			chain, err = r.repos.FallbackChain.GetEnabledByTier(ctx, tier)
		}
		if err != nil || len(chain) == 0 {
			chain, err = r.repos.FallbackChain.GetEnabled(ctx)
		}

		if err == nil && len(chain) > 0 {
			serviceKeys := r.GetServiceKeys(ctx)
			configs := make([]*LLMConfigInput, 0, len(chain))

			for _, entry := range chain {
				config := &LLMConfigInput{
					Provider:   entry.Provider,
					Model:      entry.Model,
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
				}
			}

			if len(configs) > 0 {
				return configs
			}
		}
	}

	// Fallback to hardcoded defaults
	return r.getHardcodedDefaultChain(ctx)
}

// GetDefaultConfig returns the first valid default config for a tier.
func (r *LLMConfigResolver) GetDefaultConfig(ctx context.Context, tier string) *LLMConfigInput {
	configs := r.GetDefaultConfigsForTier(ctx, tier)
	if len(configs) > 0 {
		return configs[0]
	}
	// Ultimate fallback
	return &LLMConfigInput{
		Provider:   llm.ProviderOllama,
		Model:      "llama3.2",
		StrictMode: r.GetStrictMode(ctx, llm.ProviderOllama, "llama3.2", nil),
	}
}

// getHardcodedDefaultChain returns the hardcoded fallback chain.
// This is used when no admin-configured chain is available.
func (r *LLMConfigResolver) getHardcodedDefaultChain(ctx context.Context) []*LLMConfigInput {
	serviceKeys := r.GetServiceKeys(ctx)
	configs := make([]*LLMConfigInput, 0, 4)

	// OpenRouter free models chain (requires OpenRouter API key)
	if serviceKeys.OpenRouterKey != "" {
		// 1. Xiaomi MiMo - fast and capable
		configs = append(configs, &LLMConfigInput{
			Provider:   llm.ProviderOpenRouter,
			APIKey:     serviceKeys.OpenRouterKey,
			Model:      "xiaomi/mimo-v2-flash:free",
			StrictMode: r.GetStrictMode(ctx, llm.ProviderOpenRouter, "xiaomi/mimo-v2-flash:free", nil),
		})

		// 2. GPT-OSS-120B - large open-source model
		configs = append(configs, &LLMConfigInput{
			Provider:   llm.ProviderOpenRouter,
			APIKey:     serviceKeys.OpenRouterKey,
			Model:      "openai/gpt-oss-120b:free",
			StrictMode: r.GetStrictMode(ctx, llm.ProviderOpenRouter, "openai/gpt-oss-120b:free", nil),
		})

		// 3. Gemma 3 27B - Google's instruction-tuned model
		configs = append(configs, &LLMConfigInput{
			Provider:   llm.ProviderOpenRouter,
			APIKey:     serviceKeys.OpenRouterKey,
			Model:      "google/gemma-3-27b-it:free",
			StrictMode: r.GetStrictMode(ctx, llm.ProviderOpenRouter, "google/gemma-3-27b-it:free", nil),
		})
	}

	// Final fallback: Ollama (no API key needed, requires local setup)
	configs = append(configs, &LLMConfigInput{
		Provider:   llm.ProviderOllama,
		Model:      "llama3.2",
		StrictMode: r.GetStrictMode(ctx, llm.ProviderOllama, "llama3.2", nil),
	})

	return configs
}
