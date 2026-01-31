package llm

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/jmylchreest/refyne-api/internal/config"
)

// ModelPricing represents pricing per token for a model.
type ModelPricing struct {
	PromptPricePer1M     float64 `json:"prompt_price_per_1m"`     // Price per 1M input tokens (USD)
	CompletionPricePer1M float64 `json:"completion_price_per_1m"` // Price per 1M output tokens (USD)
	IsFree               bool    `json:"is_free,omitempty"`       // If true, model is free
}

// ModelPricingLoader provides S3-backed model pricing with caching.
// Falls back to hardcoded defaults if S3 file is unavailable.
type ModelPricingLoader struct {
	loader          *config.S3Loader
	mu              sync.RWMutex
	providerPricing map[string]ModelPricing // Provider-level defaults
	modelPricing    map[string]ModelPricing // Model-specific pricing
	logger          *slog.Logger
}

// ModelPricingConfig holds configuration for the model pricing loader.
type ModelPricingConfig struct {
	S3Client     *s3.Client
	Bucket       string
	Key          string
	CacheTTL     time.Duration // How often to check for updates (default: 5 min)
	ErrorBackoff time.Duration // How long to wait after an error (default: 1 min)
	Logger       *slog.Logger
}

// ModelPricingFile represents the JSON structure stored in S3.
type ModelPricingFile struct {
	ProviderDefaults map[string]ModelPricing `json:"provider_defaults"`
	ModelOverrides   map[string]ModelPricing `json:"model_overrides"`
}

// Hardcoded fallback pricing (per million tokens, USD)
var defaultProviderPricing = map[string]ModelPricing{
	"openrouter": {PromptPricePer1M: 0.50, CompletionPricePer1M: 1.50},
	"openai":     {PromptPricePer1M: 2.50, CompletionPricePer1M: 10.0},
	"anthropic":  {PromptPricePer1M: 3.00, CompletionPricePer1M: 15.0},
	"ollama":     {PromptPricePer1M: 0.0, CompletionPricePer1M: 0.0, IsFree: true},
	"helicone":   {PromptPricePer1M: 0.50, CompletionPricePer1M: 1.50},
}

var defaultModelPricing = map[string]ModelPricing{
	// OpenAI models - these are examples, actual pricing should come from S3 config
	"gpt-4o":             {PromptPricePer1M: 2.50, CompletionPricePer1M: 10.0},
	"gpt-4o-mini":        {PromptPricePer1M: 0.15, CompletionPricePer1M: 0.60},
	"gpt-3.5-turbo":      {PromptPricePer1M: 0.50, CompletionPricePer1M: 1.50},
	"openai/gpt-4o":      {PromptPricePer1M: 2.50, CompletionPricePer1M: 10.0},
	"openai/gpt-4o-mini": {PromptPricePer1M: 0.15, CompletionPricePer1M: 0.60},

	// Anthropic models
	"claude-3-opus-20240229":      {PromptPricePer1M: 15.0, CompletionPricePer1M: 75.0},
	"claude-3-sonnet-20240229":    {PromptPricePer1M: 3.0, CompletionPricePer1M: 15.0},
	"claude-3-5-sonnet-20241022":  {PromptPricePer1M: 3.0, CompletionPricePer1M: 15.0},
	"claude-3-haiku-20240307":     {PromptPricePer1M: 0.25, CompletionPricePer1M: 1.25},
	"anthropic/claude-3.5-sonnet": {PromptPricePer1M: 3.0, CompletionPricePer1M: 15.0},
	"anthropic/claude-3-haiku":    {PromptPricePer1M: 0.25, CompletionPricePer1M: 1.25},
	"anthropic/claude-3-opus":     {PromptPricePer1M: 15.0, CompletionPricePer1M: 75.0},

	// Open source models
	"meta-llama/llama-3.1-70b-instruct": {PromptPricePer1M: 0.35, CompletionPricePer1M: 0.40},
	"meta-llama/llama-3.1-8b-instruct":  {PromptPricePer1M: 0.05, CompletionPricePer1M: 0.08},
	"meta-llama/llama-3.2-90b-instruct": {PromptPricePer1M: 0.40, CompletionPricePer1M: 0.50},
	"mistralai/mistral-large":           {PromptPricePer1M: 2.0, CompletionPricePer1M: 6.0},
	"mistralai/mistral-nemo":            {PromptPricePer1M: 0.13, CompletionPricePer1M: 0.13},
	"deepseek/deepseek-chat":            {PromptPricePer1M: 0.14, CompletionPricePer1M: 0.28},
	"qwen/qwen-2.5-72b-instruct":        {PromptPricePer1M: 0.35, CompletionPricePer1M: 0.40},

	// Google models
	"google/gemini-2.0-flash-001":      {PromptPricePer1M: 0.10, CompletionPricePer1M: 0.40},
	"google/gemini-pro-1.5":            {PromptPricePer1M: 1.25, CompletionPricePer1M: 5.0},
	"google/gemini-2.0-flash-exp:free": {PromptPricePer1M: 0.0, CompletionPricePer1M: 0.0, IsFree: true},
	"google/gemma-3-27b-it:free":       {PromptPricePer1M: 0.0, CompletionPricePer1M: 0.0, IsFree: true},
}

// NewModelPricingLoader creates a new model pricing loader.
// If S3 is not configured, falls back to hardcoded defaults.
func NewModelPricingLoader(cfg ModelPricingConfig) *ModelPricingLoader {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	loader := &ModelPricingLoader{
		loader: config.NewS3Loader(config.S3LoaderConfig{
			S3Client:     cfg.S3Client,
			Bucket:       cfg.Bucket,
			Key:          cfg.Key,
			CacheTTL:     cfg.CacheTTL,
			ErrorBackoff: cfg.ErrorBackoff,
			Logger:       cfg.Logger,
		}),
		providerPricing: make(map[string]ModelPricing),
		modelPricing:    make(map[string]ModelPricing),
		logger:          cfg.Logger,
	}

	// Copy hardcoded defaults as initial values
	for k, v := range defaultProviderPricing {
		loader.providerPricing[k] = v
	}
	for k, v := range defaultModelPricing {
		loader.modelPricing[k] = v
	}

	return loader
}

// MaybeRefresh checks if we need to refresh from S3.
// Non-blocking and fails open on errors.
func (m *ModelPricingLoader) MaybeRefresh(ctx context.Context) {
	if !m.loader.IsEnabled() {
		return // S3 not configured, use hardcoded defaults
	}

	if !m.loader.NeedsRefresh() {
		return
	}

	// Refresh in background
	go m.refresh(context.WithoutCancel(ctx))
}

// refresh fetches model pricing from S3.
func (m *ModelPricingLoader) refresh(ctx context.Context) {
	result, err := m.loader.Fetch(ctx)
	if err != nil || result == nil {
		return // Error already logged by S3Loader
	}
	if result.NotChanged {
		return // Config unchanged
	}

	// Parse pricing file
	var pricing ModelPricingFile
	if err := json.Unmarshal(result.Data, &pricing); err != nil {
		m.logger.Error("failed to parse model pricing JSON", "error", err)
		return
	}

	// Merge with hardcoded defaults (S3 overrides hardcoded)
	providerPricing := make(map[string]ModelPricing)
	modelPricing := make(map[string]ModelPricing)

	// Start with hardcoded defaults
	for k, v := range defaultProviderPricing {
		providerPricing[k] = v
	}
	for k, v := range defaultModelPricing {
		modelPricing[k] = v
	}

	// Override with S3 values
	for k, v := range pricing.ProviderDefaults {
		providerPricing[k] = v
	}
	for k, v := range pricing.ModelOverrides {
		modelPricing[k] = v
	}

	m.mu.Lock()
	m.providerPricing = providerPricing
	m.modelPricing = modelPricing
	m.mu.Unlock()

	stats := m.loader.Stats()
	m.logger.Info("model pricing loaded from S3",
		"bucket", stats.Bucket,
		"key", stats.Key,
		"etag", result.Etag,
		"provider_count", len(providerPricing),
		"model_count", len(modelPricing),
	)
}

// GetModelPricing returns pricing for a specific model.
// Priority: model-specific override > provider default
// Returns nil if no specific pricing found (caller should use pattern-based fallback).
func (m *ModelPricingLoader) GetModelPricing(provider, model string) *ModelPricing {
	// Try to refresh from S3 if needed (non-blocking)
	m.MaybeRefresh(context.Background())

	m.mu.RLock()
	providerPricing := m.providerPricing
	modelPricing := m.modelPricing
	m.mu.RUnlock()

	// Check for model-specific pricing first
	if pricing, ok := modelPricing[model]; ok {
		return &pricing
	}

	// Check for model with provider prefix (e.g., "openai/gpt-4o")
	if provider != "" {
		prefixedModel := provider + "/" + model
		if pricing, ok := modelPricing[prefixedModel]; ok {
			return &pricing
		}
	}

	// Fall back to provider defaults
	if provider != "" {
		if pricing, ok := providerPricing[provider]; ok {
			return &pricing
		}
	}

	// Return nil to signal caller should use pattern-based fallback
	return nil
}

// EstimateCost calculates estimated cost based on fallback pricing.
// Returns cost in USD.
func (m *ModelPricingLoader) EstimateCost(provider, model string, inputTokens, outputTokens int) float64 {
	pricing := m.GetModelPricing(provider, model)
	if pricing == nil {
		// No specific pricing found, use pattern-based fallback
		return m.GetFallbackPricing(model, inputTokens, outputTokens)
	}
	if pricing.IsFree {
		return 0
	}

	inputCost := float64(inputTokens) * pricing.PromptPricePer1M / 1_000_000
	outputCost := float64(outputTokens) * pricing.CompletionPricePer1M / 1_000_000
	return inputCost + outputCost
}

// HasModel returns true if we have pricing for this model.
func (m *ModelPricingLoader) HasModel(model string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.modelPricing[model]
	return ok
}

// ModelPricingStats contains statistics for observability.
type ModelPricingStats struct {
	Initialized   bool      `json:"initialized"`
	ProviderCount int       `json:"provider_count"`
	ModelCount    int       `json:"model_count"`
	Etag          string    `json:"etag"`
	LastFetch     time.Time `json:"last_fetch"`
	LastCheck     time.Time `json:"last_check"`
	UsingS3       bool      `json:"using_s3"`
}

// Stats returns current loader statistics.
func (m *ModelPricingLoader) Stats() ModelPricingStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s3Stats := m.loader.Stats()
	return ModelPricingStats{
		Initialized:   s3Stats.Initialized,
		ProviderCount: len(m.providerPricing),
		ModelCount:    len(m.modelPricing),
		Etag:          s3Stats.Etag,
		LastFetch:     s3Stats.LastFetch,
		LastCheck:     s3Stats.LastCheck,
		UsingS3:       m.loader.IsEnabled(),
	}
}

// GetFallbackPricing returns pricing using pattern-based fallback.
// This is used when dynamic pricing from providers is unavailable and
// no exact model match exists in the S3 config.
func (m *ModelPricingLoader) GetFallbackPricing(model string, inputTokens, outputTokens int) float64 {
	modelLower := strings.ToLower(model)

	// Check for free models first
	if strings.Contains(modelLower, ":free") {
		return 0
	}

	// Pattern-based fallback for unknown models
	inputPricePer1M := 0.25
	outputPricePer1M := 1.00

	switch {
	case strings.Contains(modelLower, "gpt-4o-mini"):
		// Must check gpt-4o-mini before gpt-4
		inputPricePer1M = 0.15
		outputPricePer1M = 0.60
	case strings.Contains(modelLower, "gpt-4") && !strings.Contains(modelLower, "mini"):
		inputPricePer1M = 15.0
		outputPricePer1M = 60.0
	case strings.Contains(modelLower, "claude-3-opus"), strings.Contains(modelLower, "claude-opus"):
		inputPricePer1M = 15.0
		outputPricePer1M = 75.0
	case strings.Contains(modelLower, "claude-3-haiku"):
		// Must check haiku before sonnet
		inputPricePer1M = 0.15
		outputPricePer1M = 0.60
	case strings.Contains(modelLower, "gpt-3.5"), strings.Contains(modelLower, "claude-3-sonnet"), strings.Contains(modelLower, "claude-3.5"):
		inputPricePer1M = 3.0
		outputPricePer1M = 15.0
	case strings.Contains(modelLower, "llama"), strings.Contains(modelLower, "mixtral"), strings.Contains(modelLower, "gemma"):
		inputPricePer1M = 0.10
		outputPricePer1M = 0.40
	}

	inputCost := float64(inputTokens) * inputPricePer1M / 1_000_000
	outputCost := float64(outputTokens) * outputPricePer1M / 1_000_000
	return inputCost + outputCost
}

// Global instance for backward compatibility
var globalModelPricing *ModelPricingLoader
var globalModelPricingMu sync.RWMutex

// InitGlobalModelPricing initializes the global model pricing loader.
func InitGlobalModelPricing(cfg ModelPricingConfig) {
	globalModelPricingMu.Lock()
	defer globalModelPricingMu.Unlock()
	globalModelPricing = NewModelPricingLoader(cfg)
}

// GlobalModelPricing returns the global model pricing loader.
// Falls back to a default loader if not initialized.
func GlobalModelPricing() *ModelPricingLoader {
	globalModelPricingMu.RLock()
	loader := globalModelPricing
	globalModelPricingMu.RUnlock()

	if loader == nil {
		// Return a loader with just hardcoded defaults
		globalModelPricingMu.Lock()
		defer globalModelPricingMu.Unlock()
		if globalModelPricing == nil {
			globalModelPricing = NewModelPricingLoader(ModelPricingConfig{})
		}
		return globalModelPricing
	}
	return loader
}
