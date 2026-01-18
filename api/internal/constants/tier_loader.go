package constants

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/jmylchreest/refyne-api/internal/config"
)

// TierSettingsJSON represents the JSON structure for tier settings from S3.
type TierSettingsJSON struct {
	Tiers map[string]TierLimitsJSON `json:"tiers"`
}

// TierLimitsJSON represents tier limits in JSON format.
type TierLimitsJSON struct {
	MonthlyExtractions     int  `json:"monthly_extractions"`
	MonthlyBYOKExtractions int  `json:"monthly_byok_extractions"`
	MaxPagesPerCrawl       int  `json:"max_pages_per_crawl"`
	MaxConcurrentJobs      int  `json:"max_concurrent_jobs"`
	RequestsPerMinute      int  `json:"requests_per_minute"`
	WebhooksEnabled        bool `json:"webhooks_enabled"`
	BYOKEnabled            bool `json:"byok_enabled"`
	AntiBotEnabled         bool `json:"anti_bot_enabled"`
}

// TierSettingsLoader provides S3-backed tier settings with caching.
type TierSettingsLoader struct {
	loader *config.S3Loader

	mu     sync.RWMutex
	tiers  map[string]TierLimits // overrides from S3
	logger *slog.Logger
}

// TierSettingsConfig holds configuration for the tier settings loader.
type TierSettingsConfig = config.S3LoaderConfig

// Global tier settings loader instance
var (
	tierLoader     *TierSettingsLoader
	tierLoaderOnce sync.Once
)

// InitTierLoader initializes the global tier settings loader.
// Call this at startup if you want S3-backed tier settings.
func InitTierLoader(cfg TierSettingsConfig) {
	tierLoaderOnce.Do(func() {
		tierLoader = &TierSettingsLoader{
			loader: config.NewS3Loader(cfg),
			tiers:  make(map[string]TierLimits),
			logger: cfg.Logger,
		}
		if tierLoader.logger == nil {
			tierLoader.logger = slog.Default()
		}
	})
}

// GetTierLoader returns the global tier settings loader (may be nil if not initialized).
func GetTierLoader() *TierSettingsLoader {
	return tierLoader
}

// IsEnabled returns true if S3 is configured.
func (t *TierSettingsLoader) IsEnabled() bool {
	return t.loader.IsEnabled()
}

// MaybeRefresh checks if we need to refresh tier settings from S3.
func (t *TierSettingsLoader) MaybeRefresh(ctx context.Context) {
	if !t.loader.NeedsRefresh() {
		return
	}

	// Refresh in background to not block requests
	go t.refresh(context.WithoutCancel(ctx))
}

// refresh fetches tier settings from S3 and parses them.
func (t *TierSettingsLoader) refresh(ctx context.Context) {
	result, err := t.loader.Fetch(ctx)
	if err != nil {
		// S3Loader already logged the error
		return
	}
	if result == nil || result.NotChanged {
		return
	}

	// Parse JSON
	var settings TierSettingsJSON
	if err := json.Unmarshal(result.Data, &settings); err != nil {
		t.logger.Error("failed to parse tier settings JSON", "error", err)
		return
	}

	// Convert to TierLimits
	newTiers := make(map[string]TierLimits)
	for name, limits := range settings.Tiers {
		newTiers[name] = TierLimits{
			MonthlyExtractions:     limits.MonthlyExtractions,
			MonthlyBYOKExtractions: limits.MonthlyBYOKExtractions,
			MaxPagesPerCrawl:       limits.MaxPagesPerCrawl,
			MaxConcurrentJobs:      limits.MaxConcurrentJobs,
			RequestsPerMinute:      limits.RequestsPerMinute,
			WebhooksEnabled:        limits.WebhooksEnabled,
			BYOKEnabled:            limits.BYOKEnabled,
			AntiBotEnabled:         limits.AntiBotEnabled,
		}
	}

	t.mu.Lock()
	t.tiers = newTiers
	t.mu.Unlock()

	t.logger.Info("tier settings loaded from S3",
		"tier_count", len(newTiers),
	)
}

// GetLimits returns tier limits, checking S3 overrides first.
// Returns nil if no override exists for the tier.
func (t *TierSettingsLoader) GetLimits(tier string) *TierLimits {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if limits, ok := t.tiers[tier]; ok {
		return &limits
	}
	return nil
}

// GetTierLimitsWithS3 returns tier limits, checking S3 overrides first.
// This is the main function to call - it handles normalization and fallback.
func GetTierLimitsWithS3(ctx context.Context, tier string) TierLimits {
	// Normalize first
	normalized := normalizeTierName(tier)

	// Check S3 loader if initialized
	if tierLoader != nil && tierLoader.IsEnabled() {
		tierLoader.MaybeRefresh(ctx)

		// Try normalized name first, then original
		if limits := tierLoader.GetLimits(normalized); limits != nil {
			return *limits
		}
		if limits := tierLoader.GetLimits(tier); limits != nil {
			return *limits
		}
	}

	// Fall back to hardcoded defaults
	return GetTierLimits(tier)
}
