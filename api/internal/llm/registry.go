package llm

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jmylchreest/refyne-api/internal/config"
)

// FeatureChecker is an interface for checking user features.
// This avoids import cycles with the mw package.
type FeatureChecker interface {
	HasFeature(pattern string) bool
	HasAllFeatures(features []string) bool
}

// CapabilitiesCache is an interface for caching model capabilities.
// This allows external services (like PricingService) to populate the cache.
type CapabilitiesCache interface {
	SetModelCapabilities(provider, model string, caps ModelCapabilities)
	SetModelCapabilitiesBulk(provider string, models map[string]ModelCapabilities)
}

// APIFormat defines how to parse provider API responses.
type APIFormat string

const (
	// APIFormatOpenAI is for OpenAI-compatible APIs (choices[].message.content).
	APIFormatOpenAI APIFormat = "openai"
	// APIFormatAnthropic is for Anthropic format (content[].text).
	APIFormatAnthropic APIFormat = "anthropic"
	// APIFormatOllama is for Ollama format (message.content).
	APIFormatOllama APIFormat = "ollama"
)

// AuthType defines authentication method for a provider.
type AuthType string

const (
	// AuthTypeBearer uses Authorization: Bearer <key>.
	AuthTypeBearer AuthType = "bearer"
	// AuthTypeAPIKey uses a custom header (e.g., x-api-key: <key>).
	AuthTypeAPIKey AuthType = "api_key"
	// AuthTypeNone means no authentication required (e.g., local Ollama).
	AuthTypeNone AuthType = "none"
)

// ProviderStatus indicates lifecycle state of a provider.
type ProviderStatus string

const (
	// ProviderStatusActive means the provider is fully supported.
	ProviderStatusActive ProviderStatus = "active"
	// ProviderStatusDecommissioned means the provider is retired but configs still work.
	ProviderStatusDecommissioned ProviderStatus = "decommissioned"
	// ProviderStatusBeta means the provider is in beta testing.
	ProviderStatusBeta ProviderStatus = "beta"
)

// ProviderAPIConfig contains all API interaction configuration for a provider.
type ProviderAPIConfig struct {
	BaseURL              string            // Default base URL (can be overridden by user)
	ChatEndpoint         string            // Chat completions path (e.g., "/v1/chat/completions")
	AuthType             AuthType          // How to authenticate
	AuthHeader           string            // Custom auth header name (for AuthTypeAPIKey)
	ExtraHeaders         map[string]string // Additional static headers
	APIFormat            APIFormat         // Response parsing format
	AllowBaseURLOverride bool              // True for self-hostable providers (Ollama, Helicone)

	// Pricing capabilities (uses refyne's CostEstimator interface)
	SupportsPricing        bool // Provider can estimate costs via refyne's EstimateCost
	SupportsGenerationCost bool // Provider can fetch actual generation costs
	SupportsDynamicPricing bool // Provider fetches pricing from API (vs static)
}

// ProviderInfo contains metadata about an LLM provider for API responses.
type ProviderInfo struct {
	Name             string   `json:"name"`                        // Database key: "ollama", "openai", etc.
	DisplayName      string   `json:"display_name"`                // UI label: "Ollama", "OpenAI", etc.
	Description      string   `json:"description"`                 // Brief description for UI
	RequiresKey      bool     `json:"requires_key"`                // Whether API key is required
	KeyPlaceholder   string   `json:"key_placeholder,omitempty"`   // Placeholder for key input (e.g., "sk-...")
	BaseURLHint      string   `json:"base_url_hint,omitempty"`     // Default/example base URL
	DocsURL          string   `json:"docs_url,omitempty"`          // Link to provider docs
	RequiredFeatures []string `json:"required_features,omitempty"` // Features needed to access this provider

	// Lifecycle fields
	Status            ProviderStatus `json:"status"`                       // Provider lifecycle status
	DecommissionNote  string         `json:"decommission_note,omitempty"`  // Message shown when decommissioned
	SuccessorProvider string         `json:"successor_provider,omitempty"` // Suggested replacement provider
	AllowBaseURLOverride bool        `json:"allow_base_url_override"`      // For self-hosted providers
}

// ModelInfo contains metadata about an LLM model.
type ModelInfo struct {
	ID                  string            `json:"id"`                          // Model identifier (e.g., "gpt-4o")
	Name                string            `json:"name"`                        // Display name
	Provider            string            `json:"provider"`                    // Parent provider
	ContextWindow       int               `json:"context_window,omitempty"`    // Context window size
	MaxCompletionTokens int               `json:"max_completion_tokens,omitempty"` // Max output tokens (from provider API, 0 = unknown)
	Capabilities        ModelCapabilities `json:"capabilities"`                // What the model supports
	DefaultTemp         float64           `json:"default_temperature"`         // Recommended temperature
	DefaultMaxTokens    int               `json:"default_max_tokens"`          // Recommended max tokens
}

// ModelLister is a function that lists available models for a provider.
type ModelLister func(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error)

// CapabilitiesLookup is a function that returns capabilities for a specific model.
// This doesn't require authentication - it uses cached/static data.
type CapabilitiesLookup func(ctx context.Context, model string) ModelCapabilities

// ProviderRegistration contains all information about a registered provider.
type ProviderRegistration struct {
	Info              ProviderInfo
	RequiredFeatures  []string           // Features required to access this provider (empty = always available)
	ListModels        ModelLister        // Function to list available models
	GetCapabilities   CapabilitiesLookup // Function to get capabilities for a model (no auth needed)

	// API configuration for registry-driven LLM calls
	APIConfig ProviderAPIConfig

	// Lifecycle status
	Status            ProviderStatus // Provider lifecycle status (active, decommissioned, beta)
	DecommissionNote  string         // Message shown to users when decommissioned
	SuccessorProvider string         // Suggested replacement provider
}

// Registry manages LLM provider registrations and caches model capabilities.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]*ProviderRegistration
	cfg       *config.Config
	logger    *slog.Logger

	// Model capabilities cache - keyed by "provider/model"
	capsMu sync.RWMutex
	caps   map[string]ModelCapabilities
}

// NewRegistry creates a new provider registry.
func NewRegistry(cfg *config.Config, logger *slog.Logger) *Registry {
	return &Registry{
		providers: make(map[string]*ProviderRegistration),
		cfg:       cfg,
		logger:    logger,
		caps:      make(map[string]ModelCapabilities),
	}
}

// capsCacheKey returns the cache key for a provider/model combination.
func capsCacheKey(provider, model string) string {
	return provider + "/" + model
}

// SetModelCapabilities stores capabilities in the registry's cache.
// This is called by external services (like PricingService) when they fetch model data.
func (r *Registry) SetModelCapabilities(provider, model string, caps ModelCapabilities) {
	r.capsMu.Lock()
	defer r.capsMu.Unlock()
	r.caps[capsCacheKey(provider, model)] = caps
}

// SetModelCapabilitiesBulk stores multiple model capabilities at once.
func (r *Registry) SetModelCapabilitiesBulk(provider string, models map[string]ModelCapabilities) {
	r.capsMu.Lock()
	defer r.capsMu.Unlock()
	for model, caps := range models {
		r.caps[capsCacheKey(provider, model)] = caps
	}
}

// getCachedCapabilities returns cached capabilities if available.
func (r *Registry) getCachedCapabilities(provider, model string) (ModelCapabilities, bool) {
	r.capsMu.RLock()
	defer r.capsMu.RUnlock()
	caps, ok := r.caps[capsCacheKey(provider, model)]
	return caps, ok
}

// Register adds a provider to the registry.
func (r *Registry) Register(name string, reg ProviderRegistration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure info name matches registration key
	reg.Info.Name = name
	reg.Info.RequiredFeatures = reg.RequiredFeatures

	// Copy lifecycle fields to Info for API responses
	reg.Info.Status = reg.Status
	reg.Info.DecommissionNote = reg.DecommissionNote
	reg.Info.SuccessorProvider = reg.SuccessorProvider
	reg.Info.AllowBaseURLOverride = reg.APIConfig.AllowBaseURLOverride

	// Default to active status if not specified
	if reg.Status == "" {
		reg.Status = ProviderStatusActive
		reg.Info.Status = ProviderStatusActive
	}

	r.providers[name] = &reg
}

// ListProviders returns all providers the user has access to.
// By default, decommissioned providers are excluded.
func (r *Registry) ListProviders(checker FeatureChecker) []ProviderInfo {
	return r.ListProvidersWithStatus(checker, false)
}

// ListProvidersWithStatus returns providers filtered by access and optionally including decommissioned.
func (r *Registry) ListProvidersWithStatus(checker FeatureChecker, includeDecommissioned bool) []ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var providers []ProviderInfo
	for _, reg := range r.providers {
		// Skip decommissioned providers unless explicitly requested
		if !includeDecommissioned && reg.Status == ProviderStatusDecommissioned {
			continue
		}
		if r.isProviderAllowedLocked(reg, checker) {
			providers = append(providers, reg.Info)
		}
	}

	return providers
}

// IsProviderDecommissioned checks if a provider has been decommissioned.
func (r *Registry) IsProviderDecommissioned(provider string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, ok := r.providers[provider]
	return ok && reg.Status == ProviderStatusDecommissioned
}

// GetProviderAPIConfig returns the API configuration for a provider.
// Returns nil if provider not found.
func (r *Registry) GetProviderAPIConfig(provider string) *ProviderAPIConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, ok := r.providers[provider]
	if !ok {
		return nil
	}
	return &reg.APIConfig
}

// GetProvider returns a provider registration by name.
func (r *Registry) GetProvider(name string) (*ProviderRegistration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, ok := r.providers[name]
	return reg, ok
}

// IsProviderAllowed checks if a user has access to a specific provider.
func (r *Registry) IsProviderAllowed(provider string, checker FeatureChecker) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, ok := r.providers[provider]
	if !ok {
		return false
	}

	return r.isProviderAllowedLocked(reg, checker)
}

// isProviderAllowedLocked checks provider access (caller must hold lock).
func (r *Registry) isProviderAllowedLocked(reg *ProviderRegistration, checker FeatureChecker) bool {
	// No feature requirements = always allowed
	if len(reg.RequiredFeatures) == 0 {
		return true
	}

	// Self-hosted deployment mode bypasses feature requirements
	if r.cfg.IsSelfHosted() {
		return true
	}

	// Check all required features
	if checker == nil {
		return false
	}

	return checker.HasAllFeatures(reg.RequiredFeatures)
}

// ListModels returns available models for a provider if the user has access.
func (r *Registry) ListModels(ctx context.Context, provider string, checker FeatureChecker, baseURL, apiKey string) ([]ModelInfo, error) {
	r.mu.RLock()
	reg, ok := r.providers[provider]
	r.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	if !r.IsProviderAllowed(provider, checker) {
		return nil, nil
	}

	if reg.ListModels == nil {
		return nil, nil
	}

	return reg.ListModels(ctx, baseURL, apiKey)
}

// GetMissingFeatures returns the features a user is missing for a provider.
// Returns nil if the user has access or provider doesn't exist.
func (r *Registry) GetMissingFeatures(provider string, checker FeatureChecker) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, ok := r.providers[provider]
	if !ok || len(reg.RequiredFeatures) == 0 {
		return nil
	}

	// Self-hosted mode has all features
	if r.cfg.IsSelfHosted() {
		return nil
	}

	if checker == nil {
		return reg.RequiredFeatures
	}

	var missing []string
	for _, f := range reg.RequiredFeatures {
		if !checker.HasFeature(f) {
			missing = append(missing, f)
		}
	}

	return missing
}

// AllProviderNames returns all registered provider names (for validation).
func (r *Registry) AllProviderNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}

	return names
}

// GetModelCapabilities returns capabilities for a specific model.
// Priority: 1) cached capabilities, 2) provider's GetCapabilities function, 3) empty defaults
func (r *Registry) GetModelCapabilities(ctx context.Context, provider, model string) ModelCapabilities {
	// Check cache first
	if caps, ok := r.getCachedCapabilities(provider, model); ok {
		if r.logger != nil {
			r.logger.Debug("model capabilities lookup",
				"provider", provider,
				"model", model,
				"source", "cache",
				"structured_outputs", caps.SupportsStructuredOutputs,
				"tools", caps.SupportsTools,
				"streaming", caps.SupportsStreaming,
				"reasoning", caps.SupportsReasoning,
			)
		}
		return caps
	}

	// Try provider's capability lookup function
	r.mu.RLock()
	reg, ok := r.providers[provider]
	r.mu.RUnlock()

	if ok && reg.GetCapabilities != nil {
		caps := reg.GetCapabilities(ctx, model)
		if r.logger != nil {
			r.logger.Debug("model capabilities lookup",
				"provider", provider,
				"model", model,
				"source", "provider_default",
				"structured_outputs", caps.SupportsStructuredOutputs,
				"tools", caps.SupportsTools,
				"streaming", caps.SupportsStreaming,
				"reasoning", caps.SupportsReasoning,
			)
		}
		return caps
	}

	// Return empty capabilities
	if r.logger != nil {
		r.logger.Debug("model capabilities lookup",
			"provider", provider,
			"model", model,
			"source", "empty_default",
		)
	}
	return ModelCapabilities{}
}

// SupportsStructuredOutputs is a convenience method to check if a model supports structured outputs.
func (r *Registry) SupportsStructuredOutputs(ctx context.Context, provider, model string) bool {
	return r.GetModelCapabilities(ctx, provider, model).SupportsStructuredOutputs
}

// GetMaxCompletionTokens returns the max output tokens for a model.
// Returns 0 if unknown (caller should use a default).
// This is populated by provider APIs (e.g., OpenRouter) or static defaults.
func (r *Registry) GetMaxCompletionTokens(ctx context.Context, provider, model string) int {
	caps := r.GetModelCapabilities(ctx, provider, model)
	return caps.MaxCompletionTokens
}

// GetContextLength returns the context window size for a model.
// Returns 0 if unknown.
func (r *Registry) GetContextLength(ctx context.Context, provider, model string) int {
	caps := r.GetModelCapabilities(ctx, provider, model)
	return caps.ContextLength
}

// SupportsPricing returns true if the provider can estimate costs.
func (r *Registry) SupportsPricing(provider string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, ok := r.providers[provider]
	if !ok {
		return false
	}
	return reg.APIConfig.SupportsPricing
}

// SupportsGenerationCost returns true if the provider can fetch actual generation costs.
func (r *Registry) SupportsGenerationCost(provider string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, ok := r.providers[provider]
	if !ok {
		return false
	}
	return reg.APIConfig.SupportsGenerationCost
}

// SupportsDynamicPricing returns true if the provider fetches pricing from API.
func (r *Registry) SupportsDynamicPricing(provider string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, ok := r.providers[provider]
	if !ok {
		return false
	}
	return reg.APIConfig.SupportsDynamicPricing
}

// ListPricingProviders returns all providers that support pricing estimation.
func (r *Registry) ListPricingProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var providers []string
	for name, reg := range r.providers {
		if reg.APIConfig.SupportsPricing {
			providers = append(providers, name)
		}
	}
	return providers
}
