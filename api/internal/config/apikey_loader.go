package config

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"
	"sync"
)

// APIKeyConfig represents a configured API key from S3.
type APIKeyConfig struct {
	Key          string              `json:"key"`
	Enabled      bool                `json:"enabled"`
	Name         string              `json:"name"`
	Description  string              `json:"description,omitempty"`
	Identity     APIKeyIdentity      `json:"identity"`
	Restrictions APIKeyRestrictions  `json:"restrictions"`
	RateLimits   APIKeyRateLimits    `json:"rate_limits"`
	LLMConfig    *APIKeyLLMConfigs   `json:"llm_config,omitempty"`
	TierOverrides map[string]any     `json:"tier_overrides,omitempty"`
}

// APIKeyLLMConfig specifies a single LLM model configuration.
type APIKeyLLMConfig struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	MaxTokens *int   `json:"max_tokens,omitempty"` // Optional override
}

// APIKeyLLMConfigs specifies the LLM configuration(s) for an API key.
// This bypasses the normal tier-based fallback chain resolution.
// Supports both single model (backward compatible) and multiple models (fallback chain).
type APIKeyLLMConfigs struct {
	// Single model config (backward compatible)
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	// Multiple models for fallback chain
	Models []APIKeyLLMConfig `json:"models,omitempty"`
}

// GetModels returns all configured models as a slice.
// Handles both single-model (backward compatible) and multi-model configurations.
func (c *APIKeyLLMConfigs) GetModels() []APIKeyLLMConfig {
	if len(c.Models) > 0 {
		return c.Models
	}
	// Backward compatibility: single provider/model
	if c.Provider != "" && c.Model != "" {
		return []APIKeyLLMConfig{{Provider: c.Provider, Model: c.Model}}
	}
	return nil
}

// APIKeyIdentity represents the synthetic identity for an API key.
type APIKeyIdentity struct {
	ClientID string   `json:"client_id"`
	Tier     string   `json:"tier"`
	Features []string `json:"features"`
}

// APIKeyRestrictions defines what an API key can access.
type APIKeyRestrictions struct {
	Endpoints   []string `json:"endpoints"`    // e.g., "POST /api/v1/extract"
	Referrers   []string `json:"referrers"`    // e.g., "demo.refyne.uk", "localhost:*"
	URLPatterns []string `json:"url_patterns"` // e.g., "https://demo.refyne.uk/*"
}

// APIKeyRateLimits defines rate limits for an API key.
type APIKeyRateLimits struct {
	RequestsPerMinute int `json:"requests_per_minute"`
	RequestsPerHour   int `json:"requests_per_hour"`
	RequestsPerDay    int `json:"requests_per_day"`
}

// APIKeysJSON represents the JSON structure from S3.
type APIKeysJSON struct {
	APIKeys []APIKeyConfig `json:"api_keys"`
}

// APIKeyLoader provides S3-backed API key configuration with caching.
type APIKeyLoader struct {
	loader *S3Loader

	mu     sync.RWMutex
	keys   map[string]*APIKeyConfig // key -> config
	logger *slog.Logger
}

// Global API key loader instance
var (
	apiKeyLoader     *APIKeyLoader
	apiKeyLoaderOnce sync.Once
)

// InitAPIKeyLoader initializes the global API key loader.
// Call this at startup if you want S3-backed API key configuration.
func InitAPIKeyLoader(cfg S3LoaderConfig) {
	apiKeyLoaderOnce.Do(func() {
		apiKeyLoader = &APIKeyLoader{
			loader: NewS3Loader(cfg),
			keys:   make(map[string]*APIKeyConfig),
			logger: cfg.Logger,
		}
		if apiKeyLoader.logger == nil {
			apiKeyLoader.logger = slog.Default()
		}
	})
}

// GetAPIKeyLoader returns the global API key loader (may be nil if not initialized).
func GetAPIKeyLoader() *APIKeyLoader {
	return apiKeyLoader
}

// Load performs an initial blocking load of API keys from S3.
// Call this at startup to ensure keys are available for the first request.
func (l *APIKeyLoader) Load(ctx context.Context) {
	if !l.IsEnabled() {
		return
	}
	l.refresh(ctx)
}

// IsEnabled returns true if S3 is configured.
func (l *APIKeyLoader) IsEnabled() bool {
	return l.loader.IsEnabled()
}

// MaybeRefresh checks if we need to refresh API keys from S3.
func (l *APIKeyLoader) MaybeRefresh(ctx context.Context) {
	if !l.loader.NeedsRefresh() {
		return
	}

	// Refresh in background to not block requests
	go l.refresh(context.WithoutCancel(ctx))
}

// refresh fetches API key config from S3 and parses it.
func (l *APIKeyLoader) refresh(ctx context.Context) {
	result, err := l.loader.Fetch(ctx)
	if err != nil {
		// S3Loader already logged the error
		return
	}
	if result == nil || result.NotChanged {
		return
	}

	// Parse JSON
	var config APIKeysJSON
	if err := json.Unmarshal(result.Data, &config); err != nil {
		l.logger.Error("failed to parse API keys JSON", "error", err)
		return
	}

	// Build lookup map
	newKeys := make(map[string]*APIKeyConfig)
	for i := range config.APIKeys {
		key := &config.APIKeys[i]
		newKeys[key.Key] = key
	}

	l.mu.Lock()
	l.keys = newKeys
	l.mu.Unlock()

	// Log each loaded key with relevant details
	for _, key := range newKeys {
		keyPrefix := key.Key
		if len(keyPrefix) > 15 {
			keyPrefix = keyPrefix[:15] + "..."
		}

		var llmModels []string
		if key.LLMConfig != nil {
			for _, m := range key.LLMConfig.GetModels() {
				llmModels = append(llmModels, m.Provider+"/"+m.Model)
			}
		}

		l.logger.Info("S3 API key loaded",
			"name", key.Name,
			"key", keyPrefix,
			"enabled", key.Enabled,
			"tier", key.Identity.Tier,
			"llm_models", llmModels,
			"referrers", key.Restrictions.Referrers,
		)
	}

	l.logger.Info("API keys loaded from S3", "key_count", len(newKeys))
}

// GetConfig returns the API key config if the key matches and is enabled.
// Returns nil if the key is not found or disabled.
func (l *APIKeyLoader) GetConfig(apiKey string) *APIKeyConfig {
	l.mu.RLock()
	defer l.mu.RUnlock()

	config, ok := l.keys[apiKey]
	if !ok || !config.Enabled {
		return nil
	}
	return config
}

// ValidateEndpoint checks if the endpoint is allowed for this API key.
// Supports wildcard patterns (e.g., "GET /api/v1/jobs/*").
func (c *APIKeyConfig) ValidateEndpoint(method, path string) bool {
	if len(c.Restrictions.Endpoints) == 0 {
		return true // No restrictions means all endpoints allowed
	}

	endpoint := method + " " + path
	for _, allowed := range c.Restrictions.Endpoints {
		if matchPattern(endpoint, allowed) {
			return true
		}
	}
	return false
}

// ValidateReferrer checks if the referrer is allowed for this API key.
func (c *APIKeyConfig) ValidateReferrer(referrer string) bool {
	if len(c.Restrictions.Referrers) == 0 {
		return true // No restrictions means any referrer allowed
	}

	if referrer == "" {
		return false // Referrer required but not provided
	}

	// Extract host from referrer URL for matching (e.g., "http://localhost:8080" -> "localhost:8080")
	host := referrer
	if u, err := url.Parse(referrer); err == nil && u.Host != "" {
		host = u.Host
	}

	for _, pattern := range c.Restrictions.Referrers {
		// Match against full referrer URL or just the host
		if matchPattern(referrer, pattern) || matchPattern(host, pattern) {
			return true
		}
	}
	return false
}

// ValidateTargetURL checks if the target URL is allowed for this API key.
func (c *APIKeyConfig) ValidateTargetURL(targetURL string) bool {
	if len(c.Restrictions.URLPatterns) == 0 {
		return true // No restrictions means any URL allowed
	}

	for _, pattern := range c.Restrictions.URLPatterns {
		if matchPattern(targetURL, pattern) {
			return true
		}
	}
	return false
}

// matchPattern performs simple wildcard matching.
// Supports * as wildcard at end or in middle (e.g., "localhost:*", "https://demo.refyne.uk/*").
func matchPattern(value, pattern string) bool {
	// Simple prefix match with wildcard at end
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(value, prefix)
	}

	// Simple suffix match with wildcard at start
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(value, suffix)
	}

	// Wildcard in middle (e.g., "localhost:*")
	if strings.Contains(pattern, "*") {
		parts := strings.SplitN(pattern, "*", 2)
		return strings.HasPrefix(value, parts[0]) && strings.HasSuffix(value, parts[1])
	}

	// Exact match
	return value == pattern
}

// GetAPIKeyConfigWithS3 looks up an API key in the S3 config.
// Returns nil if not found or not enabled.
func GetAPIKeyConfigWithS3(ctx context.Context, apiKey string) *APIKeyConfig {
	if apiKeyLoader == nil || !apiKeyLoader.IsEnabled() {
		return nil
	}

	apiKeyLoader.MaybeRefresh(ctx)
	return apiKeyLoader.GetConfig(apiKey)
}
