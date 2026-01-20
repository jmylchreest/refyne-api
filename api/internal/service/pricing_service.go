package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jmylchreest/refyne-api/internal/llm"
	refynellm "github.com/jmylchreest/refyne/pkg/llm"
)

// ModelPricing represents pricing and capabilities for a single model.
type ModelPricing struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	PromptPrice     float64               `json:"prompt_price"`     // Price per token (USD)
	CompletionPrice float64               `json:"completion_price"` // Price per token (USD)
	ContextLength   int                   `json:"context_length"`
	IsFree          bool                  `json:"is_free"`
	Capabilities    llm.ModelCapabilities `json:"capabilities"` // What features the model supports
}

// PricingService manages model pricing data from LLM providers.
// Uses refyne's provider interfaces for cost tracking, estimation, and model listing.
type PricingService struct {
	logger *slog.Logger

	// OpenRouter pricing cache
	openRouterAPIKey string
	openRouterPrices map[string]*ModelPricing
	openRouterMu     sync.RWMutex
	lastRefresh      time.Time
	refreshInterval  time.Duration

	// Capabilities cache (typically the llm.Registry)
	capCache llm.CapabilitiesCache
}

// PricingServiceConfig holds configuration for the pricing service.
type PricingServiceConfig struct {
	OpenRouterAPIKey string
	RefreshInterval  time.Duration
}

// SetCapabilitiesCache sets the capabilities cache that will be populated when pricing is fetched.
// This is typically called after the registry is created.
func (s *PricingService) SetCapabilitiesCache(cache llm.CapabilitiesCache) {
	s.capCache = cache
}

// SetOpenRouterAPIKey sets/updates the OpenRouter API key.
// This is called after initialization when keys are resolved from the database.
func (s *PricingService) SetOpenRouterAPIKey(key string) {
	s.openRouterAPIKey = key
}

// NewPricingService creates a new pricing service.
// Uses refyne's provider interfaces for cost tracking, estimation, and model listing.
func NewPricingService(cfg PricingServiceConfig, logger *slog.Logger) *PricingService {
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = 1 * time.Hour // Default: cache for 1 hour
	}
	if logger == nil {
		logger = slog.Default()
	}

	componentLogger := logger.With("component", "pricing")

	return &PricingService{
		logger:           componentLogger,
		openRouterAPIKey: cfg.OpenRouterAPIKey,
		openRouterPrices: make(map[string]*ModelPricing),
		refreshInterval:  cfg.RefreshInterval,
	}
}

// ensureFresh checks if the cache is stale and refreshes if needed.
// Uses a simple TTL approach - refresh on access if cache is older than refreshInterval.
// On failure, retries up to 3 times with backoff, then falls back to cached data.
func (s *PricingService) ensureFresh(ctx context.Context) {
	s.openRouterMu.RLock()
	isStale := time.Since(s.lastRefresh) > s.refreshInterval
	hasCachedData := len(s.openRouterPrices) > 0
	cacheAge := time.Since(s.lastRefresh)
	s.openRouterMu.RUnlock()

	if !isStale {
		s.logger.Debug("pricing cache is fresh",
			"age_seconds", int(cacheAge.Seconds()),
			"ttl_seconds", int(s.refreshInterval.Seconds()),
		)
		return
	}

	s.logger.Debug("pricing cache is stale, refreshing",
		"age_seconds", int(cacheAge.Seconds()),
		"ttl_seconds", int(s.refreshInterval.Seconds()),
		"has_cached_data", hasCachedData,
	)

	retryDelays := []time.Duration{200 * time.Millisecond, 600 * time.Millisecond, 1200 * time.Millisecond}

	var lastErr error
	for attempt := 0; attempt <= len(retryDelays); attempt++ {
		if attempt > 0 {
			delay := retryDelays[attempt-1]
			s.logger.Debug("retrying pricing fetch",
				"attempt", attempt+1,
				"delay_ms", delay.Milliseconds(),
			)
			time.Sleep(delay)
		}

		if err := s.RefreshOpenRouterPricing(ctx); err != nil {
			lastErr = err
			s.logger.Debug("pricing fetch attempt failed",
				"attempt", attempt+1,
				"error", err,
			)
			continue
		}
		// Success
		return
	}

	// All retries failed
	if hasCachedData {
		s.logger.Warn("failed to refresh OpenRouter pricing after retries, using cached data",
			"attempts", len(retryDelays)+1,
			"error", lastErr,
		)
	} else {
		s.logger.Error("failed to fetch OpenRouter pricing, no cached data available",
			"attempts", len(retryDelays)+1,
			"error", lastErr,
		)
	}
}

// RefreshOpenRouterPricing fetches current pricing from OpenRouter using refyne's provider.
func (s *PricingService) RefreshOpenRouterPricing(ctx context.Context) error {
	// Create an OpenRouter provider using refyne
	// API key is optional for models endpoint but may provide more data
	provider, err := refynellm.NewOpenRouterProvider(refynellm.ProviderConfig{
		APIKey: s.openRouterAPIKey,
	})
	if err != nil {
		return fmt.Errorf("failed to create OpenRouter provider: %w", err)
	}

	// Use refyne's ModelLister to fetch models
	models, err := provider.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	// Convert refyne ModelInfo to our ModelPricing format
	prices := make(map[string]*ModelPricing, len(models))
	for _, model := range models {
		prices[model.ID] = &ModelPricing{
			ID:              model.ID,
			Name:            model.Name,
			PromptPrice:     model.PromptPrice,
			CompletionPrice: model.CompletionPrice,
			ContextLength:   model.ContextLength,
			IsFree:          model.IsFree,
			Capabilities:    llm.ConvertCapabilities(model.Capabilities),
		}
	}

	s.openRouterMu.Lock()
	s.openRouterPrices = prices
	s.lastRefresh = time.Now()
	s.openRouterMu.Unlock()

	// Populate the capabilities cache if configured
	if s.capCache != nil {
		capMap := make(map[string]llm.ModelCapabilities, len(prices))
		for id, pricing := range prices {
			capMap[id] = pricing.Capabilities
		}
		s.capCache.SetModelCapabilitiesBulk("openrouter", capMap)
		s.logger.Debug("capabilities cache populated", "provider", "openrouter", "model_count", len(capMap))
	}

	s.logger.Info("OpenRouter pricing refreshed", "model_count", len(prices))
	return nil
}

// GetModelPricing returns pricing for a specific model.
// Returns nil if model not found.
func (s *PricingService) GetModelPricing(provider, model string) *ModelPricing {
	switch provider {
	case "openrouter":
		// Ensure cache is fresh (uses TTL-based refresh on access)
		s.ensureFresh(context.Background())

		s.openRouterMu.RLock()
		defer s.openRouterMu.RUnlock()
		return s.openRouterPrices[model]
	default:
		// For other providers, return nil (use estimation)
		return nil
	}
}

// EstimateCost calculates estimated cost based on cached pricing.
// Falls back to refyne's CostEstimator or hardcoded estimates if pricing not available.
func (s *PricingService) EstimateCost(provider, model string, inputTokens, outputTokens int) float64 {
	// First, try cached pricing (fastest - no API call)
	pricing := s.GetModelPricing(provider, model)
	if pricing != nil {
		inputCost := float64(inputTokens) * pricing.PromptPrice
		outputCost := float64(outputTokens) * pricing.CompletionPrice
		cost := inputCost + outputCost
		s.logger.Debug("cost estimated from cached pricing",
			"provider", provider,
			"model", model,
			"input_tokens", inputTokens,
			"output_tokens", outputTokens,
			"prompt_price", pricing.PromptPrice,
			"completion_price", pricing.CompletionPrice,
			"cost_usd", cost,
		)
		return cost
	}

	// Try refyne's CostEstimator (uses provider's static pricing data)
	cost, err := s.estimateCostViaRefyne(provider, model, inputTokens, outputTokens)
	if err == nil && cost > 0 {
		s.logger.Debug("cost estimated via refyne provider",
			"provider", provider,
			"model", model,
			"input_tokens", inputTokens,
			"output_tokens", outputTokens,
			"cost_usd", cost,
		)
		return cost
	}

	// Final fallback to hardcoded estimates
	cost = s.estimateCostFallback(model, inputTokens, outputTokens)
	s.logger.Debug("cost estimated from fallback pricing",
		"provider", provider,
		"model", model,
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
		"cost_usd", cost,
	)
	return cost
}

// estimateCostViaRefyne uses refyne's CostEstimator interface.
func (s *PricingService) estimateCostViaRefyne(provider, model string, inputTokens, outputTokens int) (float64, error) {
	// Get the API key for the provider (needed to create provider instance)
	var apiKey string
	if provider == "openrouter" {
		apiKey = s.openRouterAPIKey
	}

	// Create a refyne provider
	refyneProvider, err := refynellm.NewProvider(provider, refynellm.ProviderConfig{
		APIKey: apiKey,
		Model:  model,
	})
	if err != nil {
		return 0, err
	}

	// Check if the provider supports cost estimation
	if !refynellm.CanEstimateCost(refyneProvider) {
		return 0, fmt.Errorf("provider %s does not support cost estimation", provider)
	}

	costEstimator, ok := refynellm.AsCostEstimator(refyneProvider)
	if !ok {
		return 0, fmt.Errorf("provider %s does not implement CostEstimator", provider)
	}

	return costEstimator.EstimateCost(context.Background(), model, inputTokens, outputTokens)
}

// estimateCostFallback provides rough cost estimates when pricing data isn't available.
func (s *PricingService) estimateCostFallback(model string, inputTokens, outputTokens int) float64 {
	// Per million tokens
	inputPricePer1M := 0.25
	outputPricePer1M := 1.00

	modelLower := strings.ToLower(model)
	switch {
	case strings.Contains(modelLower, "gpt-4") && !strings.Contains(modelLower, "mini"):
		inputPricePer1M = 15.0
		outputPricePer1M = 60.0
	case strings.Contains(modelLower, "claude-3-opus"):
		inputPricePer1M = 15.0
		outputPricePer1M = 75.0
	case strings.Contains(modelLower, "gpt-3.5"), strings.Contains(modelLower, "claude-3-sonnet"), strings.Contains(modelLower, "claude-3.5"):
		inputPricePer1M = 3.0
		outputPricePer1M = 15.0
	case strings.Contains(modelLower, "gpt-4o-mini"), strings.Contains(modelLower, "claude-3-haiku"):
		inputPricePer1M = 0.15
		outputPricePer1M = 0.60
	case strings.Contains(modelLower, "llama"), strings.Contains(modelLower, "mixtral"), strings.Contains(modelLower, "gemma"):
		inputPricePer1M = 0.10
		outputPricePer1M = 0.40
	case strings.Contains(modelLower, ":free"):
		return 0
	}

	inputCost := float64(inputTokens) * inputPricePer1M / 1_000_000
	outputCost := float64(outputTokens) * outputPricePer1M / 1_000_000
	return inputCost + outputCost
}

// GetActualCost retrieves the actual cost from a provider for a specific generation.
// Uses refyne's CostTracker interface when available.
// This makes an API call, so use sparingly (e.g., for recording final costs).
// If apiKey is provided, it will be used instead of the service key (for BYOK users).
// Retries up to 3 times with delays since generation stats may not be immediately available.
func (s *PricingService) GetActualCost(ctx context.Context, provider, generationID, apiKey string) (float64, error) {
	if generationID == "" {
		return 0, fmt.Errorf("generation ID required for cost lookup")
	}

	// Use provided API key (from resolved LLM config) - this should be populated
	// by the LLMConfigResolver with either user's BYOK key or system service key
	key := apiKey
	keySource := "resolved_config"

	// Fall back to service key only if no key was passed
	if key == "" && provider == "openrouter" {
		key = s.openRouterAPIKey
		keySource = "service_fallback"
		if key != "" {
			s.logger.Warn("using fallback service key for cost lookup - resolved config had no API key",
				"provider", provider,
				"generation_id", generationID,
			)
		}
	}

	if key == "" {
		s.logger.Error("API key required for cost lookup - neither resolved config nor service fallback has key",
			"provider", provider,
			"generation_id", generationID,
			"has_service_fallback", s.openRouterAPIKey != "",
		)
		return 0, fmt.Errorf("%s API key required for cost lookup (check LLM config resolution and service key configuration)", provider)
	}

	s.logger.Debug("performing cost lookup",
		"provider", provider,
		"generation_id", generationID,
		"key_source", keySource,
	)

	// Create a refyne provider to use its CostTracker interface
	refyneProvider, err := refynellm.NewProvider(provider, refynellm.ProviderConfig{
		APIKey: key,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create provider for cost lookup: %w", err)
	}

	// Check if the provider supports cost tracking
	costTracker, ok := refynellm.AsCostTracker(refyneProvider)
	if !ok || !costTracker.SupportsGenerationCost() {
		return 0, fmt.Errorf("actual cost lookup not supported for provider: %s", provider)
	}

	// Retry with delays - generation stats may not be immediately available
	retryDelays := []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, 1000 * time.Millisecond}

	var lastErr error
	for attempt := 0; attempt <= len(retryDelays); attempt++ {
		if attempt > 0 {
			delay := retryDelays[attempt-1]
			s.logger.Debug("retrying generation cost lookup",
				"provider", provider,
				"attempt", attempt+1,
				"delay_ms", delay.Milliseconds(),
				"generation_id", generationID,
			)
			time.Sleep(delay)
		}

		cost, err := costTracker.GetGenerationCost(ctx, generationID)
		if err == nil {
			s.logger.Debug("actual cost fetched via refyne provider",
				"provider", provider,
				"generation_id", generationID,
				"cost_usd", cost,
			)
			return cost, nil
		}
		lastErr = err

		// Only retry on 404 (not found yet) - other errors are not recoverable
		if !strings.Contains(err.Error(), "status 404") {
			return 0, err
		}
	}

	return 0, lastErr
}

// GetCachedModelCount returns the number of cached model prices.
func (s *PricingService) GetCachedModelCount() int {
	s.openRouterMu.RLock()
	defer s.openRouterMu.RUnlock()
	return len(s.openRouterPrices)
}

// LastRefresh returns when pricing was last refreshed.
func (s *PricingService) LastRefresh() time.Time {
	s.openRouterMu.RLock()
	defer s.openRouterMu.RUnlock()
	return s.lastRefresh
}
