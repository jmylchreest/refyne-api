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
// Feature availability is controlled by Clerk features, not these limits.
type TierLimitsJSON struct {
	DisplayName          string  `json:"display_name,omitempty"`
	Visible              *bool   `json:"visible,omitempty"` // Pointer to detect explicit false vs missing
	Order                int     `json:"order,omitempty"`
	MonthlyExtractions   int     `json:"monthly_extractions"`
	MaxPagesPerCrawl     int     `json:"max_pages_per_crawl"`
	MaxConcurrentJobs    int     `json:"max_concurrent_jobs"`
	JobPriority          int     `json:"job_priority,omitempty"` // Scheduling priority (higher = higher priority)
	RequestsPerMinute    int     `json:"requests_per_minute"`
	CreditAllocationUSD  float64 `json:"credit_allocation_usd,omitempty"`
	CreditRolloverMonths int     `json:"credit_rollover_months,omitempty"`
	MarkupPercentage     float64 `json:"markup_percentage,omitempty"`
	CostPerTransaction   float64 `json:"cost_per_transaction,omitempty"`
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
		// Handle Visible pointer - default to true if not specified
		visible := true
		if limits.Visible != nil {
			visible = *limits.Visible
		}

		// Default JobPriority to MaxConcurrentJobs if not specified
		jobPriority := limits.JobPriority
		if jobPriority == 0 && limits.MaxConcurrentJobs > 0 {
			jobPriority = limits.MaxConcurrentJobs
		}

		newTiers[name] = TierLimits{
			DisplayName:          limits.DisplayName,
			Visible:              visible,
			Order:                limits.Order,
			MonthlyExtractions:   limits.MonthlyExtractions,
			MaxPagesPerCrawl:     limits.MaxPagesPerCrawl,
			MaxConcurrentJobs:    limits.MaxConcurrentJobs,
			JobPriority:          jobPriority,
			RequestsPerMinute:    limits.RequestsPerMinute,
			CreditAllocationUSD:  limits.CreditAllocationUSD,
			CreditRolloverMonths: limits.CreditRolloverMonths,
			MarkupPercentage:     limits.MarkupPercentage,
			CostPerTransaction:   limits.CostPerTransaction,
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
	normalized := NormalizeTierName(tier)

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
