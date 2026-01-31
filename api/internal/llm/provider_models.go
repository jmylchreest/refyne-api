package llm

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/jmylchreest/refyne-api/internal/config"
)

// ProviderModelsLoader provides S3-backed provider model lists.
// Models are loaded dynamically from configuration, not embedded in code.
type ProviderModelsLoader struct {
	loader    *config.S3Loader
	mu        sync.RWMutex
	providers map[string][]ModelInfo
	logger    *slog.Logger
}

// ProviderModelsConfig holds configuration for the provider models loader.
type ProviderModelsConfig struct {
	S3Client     *s3.Client
	Bucket       string
	Key          string
	CacheTTL     time.Duration // How often to check for updates (default: 5 min)
	ErrorBackoff time.Duration // How long to wait after an error (default: 1 min)
	Logger       *slog.Logger
}

// ProviderModelsFile represents the JSON structure stored in S3.
// This is the same format output by generate_fallbacks.
type ProviderModelsFile struct {
	GeneratedAt string                       `json:"generated_at"`
	Version     string                       `json:"version"`
	Providers   map[string]ProviderModelList `json:"providers"`
}

// ProviderModelList contains models for a single provider.
type ProviderModelList struct {
	DisplayName string           `json:"display_name"`
	Models      []ProviderModel  `json:"models"`
}

// ProviderModel represents a model entry from the S3 config.
type ProviderModel struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	ContextWindow int                     `json:"context_window"`
	MaxOutput     int                     `json:"max_output,omitempty"`
	Pricing       *ProviderModelPricing   `json:"pricing,omitempty"`
	Capabilities  ProviderModelCapabilities `json:"capabilities,omitempty"`
}

// ProviderModelPricing contains pricing info for a model.
type ProviderModelPricing struct {
	PromptPricePer1M     float64 `json:"prompt_price_per_1m"`
	CompletionPricePer1M float64 `json:"completion_price_per_1m"`
	IsFree               bool    `json:"is_free,omitempty"`
}

// ProviderModelCapabilities contains capability flags from config.
type ProviderModelCapabilities struct {
	SupportsStructuredOutputs bool `json:"supports_structured_outputs,omitempty"`
	SupportsTools             bool `json:"supports_tools,omitempty"`
	SupportsStreaming         bool `json:"supports_streaming,omitempty"`
	SupportsVision            bool `json:"supports_vision,omitempty"`
	SupportsReasoning         bool `json:"supports_reasoning,omitempty"`
}

// NewProviderModelsLoader creates a new provider models loader.
// Loads embedded config first, then S3 can override if configured.
func NewProviderModelsLoader(cfg ProviderModelsConfig) *ProviderModelsLoader {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	// Start with embedded provider models (always available in binary)
	providers := loadEmbeddedProviderModels(cfg.Logger)
	if providers == nil {
		providers = make(map[string][]ModelInfo)
	}

	return &ProviderModelsLoader{
		loader: config.NewS3Loader(config.S3LoaderConfig{
			S3Client:     cfg.S3Client,
			Bucket:       cfg.Bucket,
			Key:          cfg.Key,
			CacheTTL:     cfg.CacheTTL,
			ErrorBackoff: cfg.ErrorBackoff,
			Logger:       cfg.Logger,
		}),
		providers: providers,
		logger:    cfg.Logger,
	}
}

// MaybeRefresh checks if we need to refresh from S3.
// Non-blocking and fails open on errors.
func (p *ProviderModelsLoader) MaybeRefresh(ctx context.Context) {
	if !p.loader.IsEnabled() {
		return // S3 not configured
	}

	if !p.loader.NeedsRefresh() {
		return
	}

	// Refresh in background
	go p.refresh(context.WithoutCancel(ctx))
}

// refresh fetches provider models from S3.
func (p *ProviderModelsLoader) refresh(ctx context.Context) {
	result, err := p.loader.Fetch(ctx)
	if err != nil || result == nil {
		return // Error already logged by S3Loader
	}
	if result.NotChanged {
		return // Config unchanged
	}

	// Parse models file
	var modelsFile ProviderModelsFile
	if err := json.Unmarshal(result.Data, &modelsFile); err != nil {
		p.logger.Error("failed to parse provider models JSON", "error", err)
		return
	}

	// Convert to ModelInfo format
	providers := make(map[string][]ModelInfo)
	for providerName, providerData := range modelsFile.Providers {
		models := make([]ModelInfo, 0, len(providerData.Models))
		for _, m := range providerData.Models {
			models = append(models, ModelInfo{
				ID:                  m.ID,
				Name:                m.Name,
				Provider:            providerName,
				ContextWindow:       m.ContextWindow,
				MaxCompletionTokens: m.MaxOutput,
				Capabilities: ModelCapabilities{
					SupportsStructuredOutputs: m.Capabilities.SupportsStructuredOutputs,
					SupportsTools:             m.Capabilities.SupportsTools,
					SupportsStreaming:         m.Capabilities.SupportsStreaming,
					SupportsReasoning:         m.Capabilities.SupportsReasoning,
				},
				DefaultTemp:      GetDefaultSettings(providerName, m.ID).Temperature,
				DefaultMaxTokens: GetDefaultSettings(providerName, m.ID).MaxTokens,
			})
		}
		providers[providerName] = models
	}

	p.mu.Lock()
	p.providers = providers
	p.mu.Unlock()

	stats := p.loader.Stats()
	totalModels := 0
	for _, models := range providers {
		totalModels += len(models)
	}
	p.logger.Info("provider models loaded from S3",
		"bucket", stats.Bucket,
		"key", stats.Key,
		"etag", result.Etag,
		"provider_count", len(providers),
		"total_models", totalModels,
	)
}

// GetModels returns models for a provider from S3-backed config.
// Returns empty slice if provider not found or S3 not configured.
func (p *ProviderModelsLoader) GetModels(provider string) []ModelInfo {
	// Try to refresh from S3 if needed (non-blocking)
	p.MaybeRefresh(context.Background())

	p.mu.RLock()
	defer p.mu.RUnlock()

	models, ok := p.providers[provider]
	if !ok {
		return nil
	}

	// Return a copy to avoid mutation
	result := make([]ModelInfo, len(models))
	copy(result, models)
	return result
}

// GetAllProviders returns all provider names that have models loaded.
func (p *ProviderModelsLoader) GetAllProviders() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	providers := make([]string, 0, len(p.providers))
	for name := range p.providers {
		providers = append(providers, name)
	}
	return providers
}

// HasProvider returns true if the provider has models loaded.
func (p *ProviderModelsLoader) HasProvider(provider string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.providers[provider]
	return ok
}

// Stats returns current loader statistics.
func (p *ProviderModelsLoader) Stats() ProviderModelsStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	s3Stats := p.loader.Stats()

	totalModels := 0
	for _, models := range p.providers {
		totalModels += len(models)
	}

	return ProviderModelsStats{
		Initialized:   s3Stats.Initialized,
		ProviderCount: len(p.providers),
		TotalModels:   totalModels,
		Etag:          s3Stats.Etag,
		LastFetch:     s3Stats.LastFetch,
		LastCheck:     s3Stats.LastCheck,
		UsingS3:       p.loader.IsEnabled(),
	}
}

// ProviderModelsStats contains statistics for observability.
type ProviderModelsStats struct {
	Initialized   bool      `json:"initialized"`
	ProviderCount int       `json:"provider_count"`
	TotalModels   int       `json:"total_models"`
	Etag          string    `json:"etag"`
	LastFetch     time.Time `json:"last_fetch"`
	LastCheck     time.Time `json:"last_check"`
	UsingS3       bool      `json:"using_s3"`
}

// Global instance for provider models
var globalProviderModels *ProviderModelsLoader
var globalProviderModelsMu sync.RWMutex

// InitGlobalProviderModels initializes the global provider models loader.
func InitGlobalProviderModels(cfg ProviderModelsConfig) {
	globalProviderModelsMu.Lock()
	defer globalProviderModelsMu.Unlock()
	globalProviderModels = NewProviderModelsLoader(cfg)
}

// GlobalProviderModels returns the global provider models loader.
// Falls back to an empty loader if not initialized.
func GlobalProviderModels() *ProviderModelsLoader {
	globalProviderModelsMu.RLock()
	loader := globalProviderModels
	globalProviderModelsMu.RUnlock()

	if loader == nil {
		// Return an empty loader
		globalProviderModelsMu.Lock()
		defer globalProviderModelsMu.Unlock()
		if globalProviderModels == nil {
			globalProviderModels = NewProviderModelsLoader(ProviderModelsConfig{})
		}
		return globalProviderModels
	}
	return loader
}
