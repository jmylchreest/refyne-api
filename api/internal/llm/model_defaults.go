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

// ModelDefaultsLoader provides S3-backed model settings with caching.
// Falls back to hardcoded defaults if S3 file is unavailable.
type ModelDefaultsLoader struct {
	loader         *config.S3Loader
	mu             sync.RWMutex
	providerDefs   map[string]ModelSettings // Provider-level defaults
	modelOverrides map[string]ModelSettings // Model-specific overrides
	logger         *slog.Logger
}

// ModelDefaultsConfig holds configuration for the model defaults loader.
type ModelDefaultsConfig struct {
	S3Client     *s3.Client
	Bucket       string
	Key          string
	CacheTTL     time.Duration // How often to check for updates (default: 5 min)
	ErrorBackoff time.Duration // How long to wait after an error (default: 1 min)
	Logger       *slog.Logger
}

// ModelDefaultsFile represents the JSON structure stored in S3.
type ModelDefaultsFile struct {
	ProviderDefaults map[string]ModelSettings `json:"provider_defaults"`
	ModelOverrides   map[string]ModelSettings `json:"model_overrides"`
}

// NewModelDefaultsLoader creates a new model defaults loader.
// If S3 is not configured, falls back to hardcoded defaults.
func NewModelDefaultsLoader(cfg ModelDefaultsConfig) *ModelDefaultsLoader {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	loader := &ModelDefaultsLoader{
		loader: config.NewS3Loader(config.S3LoaderConfig{
			S3Client:     cfg.S3Client,
			Bucket:       cfg.Bucket,
			Key:          cfg.Key,
			CacheTTL:     cfg.CacheTTL,
			ErrorBackoff: cfg.ErrorBackoff,
			Logger:       cfg.Logger,
		}),
		providerDefs:   make(map[string]ModelSettings),
		modelOverrides: make(map[string]ModelSettings),
		logger:         cfg.Logger,
	}

	// Copy hardcoded defaults as initial values
	for k, v := range ProviderDefaults {
		loader.providerDefs[k] = v
	}
	for k, v := range ModelOverrides {
		loader.modelOverrides[k] = v
	}

	return loader
}

// MaybeRefresh checks if we need to refresh from S3.
// Non-blocking and fails open on errors.
func (m *ModelDefaultsLoader) MaybeRefresh(ctx context.Context) {
	if !m.loader.IsEnabled() {
		return // S3 not configured, use hardcoded defaults
	}

	if !m.loader.NeedsRefresh() {
		return
	}

	// Refresh in background
	go m.refresh(context.WithoutCancel(ctx))
}

// refresh fetches model defaults from S3.
func (m *ModelDefaultsLoader) refresh(ctx context.Context) {
	result, err := m.loader.Fetch(ctx)
	if err != nil || result == nil {
		return // Error already logged by S3Loader
	}
	if result.NotChanged {
		return // Config unchanged
	}

	// Parse defaults file
	var defaults ModelDefaultsFile
	if err := json.Unmarshal(result.Data, &defaults); err != nil {
		m.logger.Error("failed to parse model defaults JSON", "error", err)
		return
	}

	// Merge with hardcoded defaults (S3 overrides hardcoded)
	providerDefs := make(map[string]ModelSettings)
	modelOverrides := make(map[string]ModelSettings)

	// Start with hardcoded defaults
	for k, v := range ProviderDefaults {
		providerDefs[k] = v
	}
	for k, v := range ModelOverrides {
		modelOverrides[k] = v
	}

	// Override with S3 values
	for k, v := range defaults.ProviderDefaults {
		providerDefs[k] = v
	}
	for k, v := range defaults.ModelOverrides {
		modelOverrides[k] = v
	}

	m.mu.Lock()
	m.providerDefs = providerDefs
	m.modelOverrides = modelOverrides
	m.mu.Unlock()

	stats := m.loader.Stats()
	m.logger.Info("model defaults loaded from S3",
		"bucket", stats.Bucket,
		"key", stats.Key,
		"etag", result.Etag,
		"provider_count", len(providerDefs),
		"model_count", len(modelOverrides),
	)
}

// GetModelSettings returns settings for a model.
// Priority: chain config override > model override > provider default > hardcoded fallback
func (m *ModelDefaultsLoader) GetModelSettings(provider, model string, chainTemp *float64, chainMaxTokens *int, chainStrictMode *bool) (temperature float64, maxTokens int, strictMode bool) {
	m.mu.RLock()
	providerDefs := m.providerDefs
	modelOverrides := m.modelOverrides
	m.mu.RUnlock()

	// Start with provider defaults
	defaults, ok := providerDefs[provider]
	if !ok {
		defaults = ModelSettings{Temperature: 0.2, MaxTokens: 4096, StrictMode: false}
	}

	temperature = defaults.Temperature
	maxTokens = defaults.MaxTokens
	strictMode = defaults.StrictMode

	// Check for model-specific override
	if override, ok := modelOverrides[model]; ok {
		temperature = override.Temperature
		maxTokens = override.MaxTokens
		strictMode = override.StrictMode
	}

	// Chain config takes highest priority
	if chainTemp != nil {
		temperature = *chainTemp
	}
	if chainMaxTokens != nil {
		maxTokens = *chainMaxTokens
	}
	if chainStrictMode != nil {
		strictMode = *chainStrictMode
	}

	return temperature, maxTokens, strictMode
}

// GetDefaultSettings returns settings for a model without chain overrides.
func (m *ModelDefaultsLoader) GetDefaultSettings(provider, model string) ModelSettings {
	m.mu.RLock()
	modelOverrides := m.modelOverrides
	providerDefs := m.providerDefs
	m.mu.RUnlock()

	// Check model-specific override first
	if override, ok := modelOverrides[model]; ok {
		return override
	}

	// Fall back to provider defaults
	if defaults, ok := providerDefs[provider]; ok {
		return defaults
	}

	return ModelSettings{Temperature: 0.2, MaxTokens: 4096, StrictMode: false}
}

// ModelDefaultsStats contains statistics for observability.
type ModelDefaultsStats struct {
	Initialized    bool      `json:"initialized"`
	ProviderCount  int       `json:"provider_count"`
	ModelCount     int       `json:"model_count"`
	Etag           string    `json:"etag"`
	LastFetch      time.Time `json:"last_fetch"`
	LastCheck      time.Time `json:"last_check"`
	UsingS3        bool      `json:"using_s3"`
}

// Stats returns current loader statistics.
func (m *ModelDefaultsLoader) Stats() ModelDefaultsStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s3Stats := m.loader.Stats()
	return ModelDefaultsStats{
		Initialized:   s3Stats.Initialized,
		ProviderCount: len(m.providerDefs),
		ModelCount:    len(m.modelOverrides),
		Etag:          s3Stats.Etag,
		LastFetch:     s3Stats.LastFetch,
		LastCheck:     s3Stats.LastCheck,
		UsingS3:       m.loader.IsEnabled(),
	}
}

// Global instance for backward compatibility
var globalModelDefaults *ModelDefaultsLoader
var globalModelDefaultsMu sync.RWMutex

// InitGlobalModelDefaults initializes the global model defaults loader.
func InitGlobalModelDefaults(cfg ModelDefaultsConfig) {
	globalModelDefaultsMu.Lock()
	defer globalModelDefaultsMu.Unlock()
	globalModelDefaults = NewModelDefaultsLoader(cfg)
}

// GlobalModelDefaults returns the global model defaults loader.
// Falls back to a default loader if not initialized.
func GlobalModelDefaults() *ModelDefaultsLoader {
	globalModelDefaultsMu.RLock()
	loader := globalModelDefaults
	globalModelDefaultsMu.RUnlock()

	if loader == nil {
		// Return a loader with just hardcoded defaults
		globalModelDefaultsMu.Lock()
		defer globalModelDefaultsMu.Unlock()
		if globalModelDefaults == nil {
			globalModelDefaults = NewModelDefaultsLoader(ModelDefaultsConfig{})
		}
		return globalModelDefaults
	}
	return loader
}
