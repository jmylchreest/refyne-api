package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// OpenRouter API endpoints
	openRouterModelsURL     = "https://openrouter.ai/api/v1/models"
	openRouterGenerationURL = "https://openrouter.ai/api/v1/generation"
)

// ProviderCostFetcher defines the interface for fetching actual costs from providers.
// Providers that support per-generation cost lookup can implement this interface.
type ProviderCostFetcher interface {
	// FetchGenerationCost retrieves the actual cost for a specific generation.
	// Returns the cost in USD or an error if lookup fails/not supported.
	FetchGenerationCost(ctx context.Context, generationID, apiKey string) (float64, error)
	// SupportsGeneration returns true if the provider supports generation cost lookup.
	SupportsGeneration() bool
}

// OpenRouterCostFetcher implements ProviderCostFetcher for OpenRouter.
type OpenRouterCostFetcher struct {
	httpClient *http.Client
	logger     *slog.Logger
}

// NewOpenRouterCostFetcher creates a new OpenRouter cost fetcher.
func NewOpenRouterCostFetcher(httpClient *http.Client, logger *slog.Logger) *OpenRouterCostFetcher {
	return &OpenRouterCostFetcher{
		httpClient: httpClient,
		logger:     logger,
	}
}

// SupportsGeneration returns true - OpenRouter supports generation cost lookup.
func (f *OpenRouterCostFetcher) SupportsGeneration() bool {
	return true
}

// FetchGenerationCost fetches the actual cost from OpenRouter.
func (f *OpenRouterCostFetcher) FetchGenerationCost(ctx context.Context, generationID, apiKey string) (float64, error) {
	url := fmt.Sprintf("%s?id=%s", openRouterGenerationURL, generationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			TotalCost float64 `json:"total_cost"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	f.logger.Debug("actual cost fetched from OpenRouter",
		"generation_id", generationID,
		"cost_usd", result.Data.TotalCost,
	)
	return result.Data.TotalCost, nil
}

// ModelPricing represents pricing for a single model.
type ModelPricing struct {
	ID              string  `json:"id"`
	PromptPrice     float64 `json:"prompt_price"`     // Price per token (USD)
	CompletionPrice float64 `json:"completion_price"` // Price per token (USD)
	ContextLength   int     `json:"context_length"`
	IsFree          bool    `json:"is_free"`
}

// PricingService manages model pricing data from LLM providers.
type PricingService struct {
	httpClient *http.Client
	logger     *slog.Logger

	// OpenRouter pricing cache
	openRouterAPIKey string
	openRouterPrices map[string]*ModelPricing
	openRouterMu     sync.RWMutex
	lastRefresh      time.Time
	refreshInterval  time.Duration

	// Provider cost fetchers for generation cost lookup
	costFetchers map[string]ProviderCostFetcher
}

// PricingServiceConfig holds configuration for the pricing service.
type PricingServiceConfig struct {
	OpenRouterAPIKey string
	RefreshInterval  time.Duration
}

// NewPricingService creates a new pricing service.
func NewPricingService(cfg PricingServiceConfig, logger *slog.Logger) *PricingService {
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = 1 * time.Hour // Default: cache for 1 hour
	}
	if logger == nil {
		logger = slog.Default()
	}

	httpClient := &http.Client{
		Timeout: 5 * time.Second, // Short timeout for quick retries
	}
	componentLogger := logger.With("component", "pricing")

	// Initialize provider cost fetchers
	costFetchers := map[string]ProviderCostFetcher{
		"openrouter": NewOpenRouterCostFetcher(httpClient, componentLogger),
		// Other providers can be added here when they support generation cost lookup
		// "anthropic": NewAnthropicCostFetcher(httpClient, componentLogger),
		// "openai": NewOpenAICostFetcher(httpClient, componentLogger),
	}

	return &PricingService{
		httpClient:       httpClient,
		logger:           componentLogger,
		openRouterAPIKey: cfg.OpenRouterAPIKey,
		openRouterPrices: make(map[string]*ModelPricing),
		refreshInterval:  cfg.RefreshInterval,
		costFetchers:     costFetchers,
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

// openRouterModelsResponse represents the response from OpenRouter's /api/v1/models endpoint.
type openRouterModelsResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID            string              `json:"id"`
	Pricing       openRouterPricing   `json:"pricing"`
	ContextLength int                 `json:"context_length"`
	TopProvider   *openRouterProvider `json:"top_provider,omitempty"`
}

type openRouterPricing struct {
	Prompt     string `json:"prompt"`     // Price per token as string (e.g., "0.000003")
	Completion string `json:"completion"` // Price per token as string
	Image      string `json:"image,omitempty"`
	Request    string `json:"request,omitempty"`
}

type openRouterProvider struct {
	ContextLength       int  `json:"context_length,omitempty"`
	MaxCompletionTokens int  `json:"max_completion_tokens,omitempty"`
	IsModerated         bool `json:"is_moderated,omitempty"`
}

// RefreshOpenRouterPricing fetches current pricing from OpenRouter.
func (s *PricingService) RefreshOpenRouterPricing(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterModelsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// API key is optional for models endpoint but may provide more data
	if s.openRouterAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.openRouterAPIKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result openRouterModelsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Parse and cache pricing
	prices := make(map[string]*ModelPricing, len(result.Data))
	for _, model := range result.Data {
		promptPrice := parsePrice(model.Pricing.Prompt)
		completionPrice := parsePrice(model.Pricing.Completion)

		prices[model.ID] = &ModelPricing{
			ID:              model.ID,
			PromptPrice:     promptPrice,
			CompletionPrice: completionPrice,
			ContextLength:   model.ContextLength,
			IsFree:          promptPrice == 0 && completionPrice == 0,
		}
	}

	s.openRouterMu.Lock()
	s.openRouterPrices = prices
	s.lastRefresh = time.Now()
	s.openRouterMu.Unlock()

	s.logger.Info("OpenRouter pricing refreshed", "model_count", len(prices))
	return nil
}

// parsePrice converts a string price to float64.
func parsePrice(s string) float64 {
	if s == "" || s == "0" {
		return 0
	}
	var price float64
	fmt.Sscanf(s, "%f", &price)
	return price
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
// Falls back to hardcoded estimates if pricing not available.
func (s *PricingService) EstimateCost(provider, model string, inputTokens, outputTokens int) float64 {
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

	// Fallback to hardcoded estimates
	cost := s.estimateCostFallback(model, inputTokens, outputTokens)
	s.logger.Debug("cost estimated from fallback pricing",
		"provider", provider,
		"model", model,
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
		"cost_usd", cost,
	)
	return cost
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
// This makes an API call, so use sparingly (e.g., for recording final costs).
// If apiKey is provided, it will be used instead of the service key (for BYOK users).
// Retries up to 3 times with delays since generation stats may not be immediately available.
func (s *PricingService) GetActualCost(ctx context.Context, provider, generationID, apiKey string) (float64, error) {
	if generationID == "" {
		return 0, fmt.Errorf("generation ID required for cost lookup")
	}

	// Get the provider's cost fetcher
	fetcher, ok := s.costFetchers[provider]
	if !ok || !fetcher.SupportsGeneration() {
		return 0, fmt.Errorf("actual cost lookup not supported for provider: %s", provider)
	}

	// Use provided API key (BYOK) or fall back to service key for OpenRouter
	key := apiKey
	if key == "" && provider == "openrouter" {
		key = s.openRouterAPIKey
	}
	if key == "" {
		return 0, fmt.Errorf("%s API key required for cost lookup", provider)
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

		cost, err := fetcher.FetchGenerationCost(ctx, generationID, key)
		if err == nil {
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
