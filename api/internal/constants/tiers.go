// Package constants defines centralized configuration for tier limits,
// rate limits, and user-facing messages. Change values here to update
// limits across the entire application.
package constants

import (
	"fmt"
	"sync"
	"time"
)

// tiersMu protects concurrent access to the Tiers map.
var tiersMu sync.RWMutex

// Tier names
const (
	TierFree       = "free"
	TierStandard   = "standard"
	TierPro        = "pro"
	TierSelfHosted = "selfhosted"
)

// Feature names used in Clerk Commerce (or license file for self-hosted)
const (
	FeatureProviderBYOK   = "provider_byok"   // Controls BYOK (bring your own API keys)
	FeatureProviderOllama = "provider_ollama" // Controls access to Ollama (local LLM provider)
	FeatureModelsCustom   = "models_custom"   // Controls custom model/fallback chain configuration
	FeatureModelsPremium  = "models_premium"  // Access to premium/charged models with budget-based fallback
	FeatureContentDynamic = "content_dynamic" // JavaScript/real browser support for dynamic content
)

// TierLimits defines the numeric limits for a subscription tier.
// Feature availability (webhooks, BYOK, anti-bot) is controlled by Clerk features.
type TierLimits struct {
	// DisplayName is the user-facing name for this tier (synced from Clerk Commerce).
	// This is exposed via the pricing API.
	DisplayName string
	// Visible controls whether this tier appears in the public pricing API.
	// Set to false for tiers not yet available to users (e.g., Pro, SelfHosted).
	Visible bool
	// Order controls the display order in pricing tables (lower = first).
	Order int
	// MonthlyExtractions is the max extractions per month (0 = unlimited)
	MonthlyExtractions int
	// MaxPagesPerCrawl is the max pages per crawl job (0 = unlimited)
	MaxPagesPerCrawl int
	// MaxConcurrentJobs is the max concurrent jobs (0 = unlimited)
	// Also used as JobPriority for scheduling (higher = higher priority)
	MaxConcurrentJobs int
	// JobPriority controls scheduling priority (higher = processed first)
	// Defaults to MaxConcurrentJobs if not set
	JobPriority int
	// RequestsPerMinute is the rate limit for API requests (0 = unlimited)
	RequestsPerMinute int
	// CreditAllocationUSD is the monthly USD credit for premium model calls (0 = none)
	CreditAllocationUSD float64
	// CreditRolloverMonths controls credit expiry:
	//   -1 = never expires
	//    0 = expires at end of current billing period (no rollover)
	//    N = rolls over for N additional periods
	CreditRolloverMonths int
	// MarkupPercentage is the markup applied to LLM costs (0.02 = 2%).
	// This is an internal business metric not exposed to users via the pricing API.
	MarkupPercentage float64
	// CostPerTransaction is the flat cost added to each transaction.
	// This is an internal business metric not exposed to users via the pricing API.
	CostPerTransaction float64
}

// Tiers defines limits for each subscription tier.
// Feature availability (webhooks, BYOK, anti-bot) is controlled by Clerk features.
// To change tier limits, modify this map.
var Tiers = map[string]TierLimits{
	TierFree: {
		DisplayName:          "Free",    // TODO: Sync from Clerk Commerce
		Visible:              true,      // Available to users
		Order:                0,         // First in list
		MonthlyExtractions:   100,
		MaxPagesPerCrawl:     10,
		MaxConcurrentJobs:    2,
		JobPriority:          2,  // Lower priority than paid tiers
		RequestsPerMinute:    10,
		CreditAllocationUSD:  0,
		CreditRolloverMonths: 0,    // No rollover - expires at end of period
		MarkupPercentage:     0.02, // 2% markup
		CostPerTransaction:   0,
	},
	TierStandard: {
		DisplayName:          "Standard", // TODO: Sync from Clerk Commerce
		Visible:              true,       // Available to users
		Order:                1,          // Second in list
		MonthlyExtractions:   1000,
		MaxPagesPerCrawl:     50,
		MaxConcurrentJobs:    10,
		JobPriority:          10, // Medium priority
		RequestsPerMinute:    60,
		CreditAllocationUSD:  0,
		CreditRolloverMonths: 0,    // No rollover - expires at end of period
		MarkupPercentage:     0.02, // 2% markup
		CostPerTransaction:   0,
	},
	TierPro: {
		DisplayName:          "Pro",  // TODO: Sync from Clerk Commerce
		Visible:              false,  // Not yet available to users
		Order:                2,      // Third in list (when visible)
		MonthlyExtractions:   0,      // Unlimited
		MaxPagesPerCrawl:     500,
		MaxConcurrentJobs:    50,
		JobPriority:          50, // Highest priority
		RequestsPerMinute:    60,
		CreditAllocationUSD:  35.00, // $35 USD credit on $45 plan
		CreditRolloverMonths: 0,     // No rollover - expires at end of period
		MarkupPercentage:     0.02,  // 2% markup
		CostPerTransaction:   0,
	},
	TierSelfHosted: {
		DisplayName:          "Self-Hosted", // TODO: Sync from Clerk Commerce
		Visible:              false,         // Not yet available to users
		Order:                3,             // Last in list (when visible)
		MonthlyExtractions:   0,             // Unlimited
		MaxPagesPerCrawl:     0,             // Unlimited
		MaxConcurrentJobs:    0,             // Unlimited (0 = no limit)
		JobPriority:          100,           // Highest priority for self-hosted
		RequestsPerMinute:    0,             // Unlimited
		CreditAllocationUSD:  0,             // Self-hosted uses own keys
		CreditRolloverMonths: 0,
		MarkupPercentage:     0, // No markup for self-hosted
		CostPerTransaction:   0,
	},
}

// GetTierLimits returns the limits for a tier, defaulting to free tier.
// Normalizes tier names from Clerk Commerce format (e.g., "tier_v1_standard" -> "standard").
// Thread-safe for concurrent access.
func GetTierLimits(tier string) TierLimits {
	tiersMu.RLock()
	defer tiersMu.RUnlock()

	// Direct match first
	if limits, ok := Tiers[tier]; ok {
		return limits
	}

	// Normalize Clerk Commerce tier names (tier_v1_standard -> standard)
	normalized := NormalizeTierName(tier)
	if limits, ok := Tiers[normalized]; ok {
		return limits
	}

	return Tiers[TierFree]
}

// NormalizeTierName converts Clerk Commerce tier names to internal tier names.
// Examples:
//   - "tier_v1_standard" -> "standard"
//   - "tier_v1_pro" -> "pro"
//   - "tier_v1_free" -> "free"
//   - "standard" -> "standard" (already normalized)
func NormalizeTierName(tier string) string {
	// Map of Clerk Commerce tier names to internal tier names
	tierMappings := map[string]string{
		"tier_v1_free":       TierFree,
		"tier_v1_standard":   TierStandard,
		"tier_v1_pro":        TierPro,
		"tier_v1_selfhosted": TierSelfHosted,
	}

	if mapped, ok := tierMappings[tier]; ok {
		return mapped
	}

	return tier
}

// Global rate limiting defaults
const (
	// GlobalIPRateLimitPerMinute is the fallback rate limit for unauthenticated requests
	GlobalIPRateLimitPerMinute = 100
	// GlobalConcurrencyLimit is the max concurrent requests the server will handle
	GlobalConcurrencyLimit = 100
	// MaxRequestBodySize is the max request body size in bytes (1MB)
	MaxRequestBodySize = 1 * 1024 * 1024
)

// Job processing defaults
const (
	// StaleJobAge is how long a job can be "running" before it's considered stale
	StaleJobAge = 30 * time.Minute
	// DefaultWorkerConcurrency is the default number of worker goroutines
	DefaultWorkerConcurrency = 3
	// MaxSyncWaitTimeout is the maximum time a sync crawl request (wait=true) will block.
	// This prevents long-held connections from consuming resources.
	// Clients needing longer waits should use async mode with webhooks or polling.
	MaxSyncWaitTimeout = 2 * time.Minute
)

// HTTP request timeouts
const (
	// DefaultRequestTimeout is the timeout for most API endpoints
	DefaultRequestTimeout = 60 * time.Second
	// LLMRequestTimeout is the extended timeout for LLM-based operations
	// (analyze, extract) which involve page fetching + LLM inference
	LLMRequestTimeout = 3 * time.Minute
)

// Cache durations for Cache-Control headers
const (
	// CacheMaxAgeShort is for rapidly changing data (health checks, etc.)
	CacheMaxAgeShort = 30 * time.Second
	// CacheMaxAgeMedium is for semi-stable data (schemas, tier info)
	CacheMaxAgeMedium = 5 * time.Minute
	// CacheMaxAgeLong is for stable data (provider lists, model lists)
	CacheMaxAgeLong = 1 * time.Hour
	// CacheMaxAgeImmutable is for immutable data (completed job results)
	CacheMaxAgeImmutable = 24 * time.Hour
)

// QuotaExceededMessage returns a user-friendly message for monthly quota exceeded.
func QuotaExceededMessage(tier string) string {
	normalized := NormalizeTierName(tier)
	limits := GetTierLimits(normalized)
	switch normalized {
	case TierFree:
		return fmt.Sprintf("You've reached your free tier limit of %d extractions this month. Upgrade to Standard for %d monthly extractions.",
			limits.MonthlyExtractions, Tiers[TierStandard].MonthlyExtractions)
	case TierStandard:
		return fmt.Sprintf("You've reached your Standard plan limit of %d extractions this month. Upgrade to Pro for unlimited extractions.",
			limits.MonthlyExtractions)
	default:
		return "You've reached your monthly extraction limit. Please contact support or upgrade your plan."
	}
}

// ConcurrentJobLimitMessage returns a user-friendly message for concurrent job limit exceeded.
func ConcurrentJobLimitMessage(tier string) string {
	normalized := NormalizeTierName(tier)
	limits := GetTierLimits(normalized)
	switch normalized {
	case TierFree:
		return fmt.Sprintf("You can only run %d job at a time on the free tier. Wait for your current job to complete or upgrade to Standard for %d concurrent jobs.",
			limits.MaxConcurrentJobs, Tiers[TierStandard].MaxConcurrentJobs)
	case TierStandard:
		return fmt.Sprintf("You've reached your Standard plan limit of %d concurrent jobs. Wait for a job to complete or upgrade to Pro for %d concurrent jobs.",
			limits.MaxConcurrentJobs, Tiers[TierPro].MaxConcurrentJobs)
	case TierPro:
		return fmt.Sprintf("You've reached your Pro plan limit of %d concurrent jobs. Wait for a job to complete.",
			limits.MaxConcurrentJobs)
	default:
		return fmt.Sprintf("You've reached your limit of %d concurrent jobs. Wait for a job to complete.",
			limits.MaxConcurrentJobs)
	}
}

// FeatureNotAvailableMessage returns a user-friendly message for feature not available.
func FeatureNotAvailableMessage(feature string) string {
	switch feature {
	case "content_static":
		return "Static content fetching is required for this operation."
	case "content_dynamic":
		return "Dynamic content fetching (JavaScript rendering) is not available on your current plan. Upgrade to access JavaScript-rendered pages."
	case "content_analyzer":
		return "URL analysis is not available on your current plan. Upgrade to access the analyze feature."
	case "extraction_single_page":
		return "Single page extraction is not available on your current plan."
	case "extraction_crawled":
		return "Multi-page crawl extraction is not available on your current plan. Upgrade to crawl multiple pages."
	case "schema_custom":
		return "Custom schemas are not available on your current plan. Upgrade to create and save your own schemas."
	case "webhooks":
		return "Webhooks are not available on your current plan. Upgrade to use webhook notifications."
	case "byok", "provider_byok":
		return "Bring Your Own Key (BYOK) is not available on your current plan. Upgrade to use your own LLM API keys."
	case "provider_ollama":
		return "Ollama (local LLM provider) is not available on your current plan. This feature is available for self-hosted deployments."
	case "models_custom":
		return "Custom model selection is not available on your current plan. Upgrade to configure your own model fallback chain."
	case "antibot":
		return "Anti-bot features are not available on your current plan. Upgrade for advanced anti-bot capabilities."
	default:
		return "This feature is not available on your current plan. Please upgrade to access this feature."
	}
}

// TierMetadata represents visibility and display info synced from Clerk Commerce.
type TierMetadata struct {
	Slug        string // Tier slug (e.g., "free", "standard", "pro")
	DisplayName string // Human-readable name from Clerk
	Visible     bool   // Whether publicly available in Clerk
}

// UpdateTierMetadata updates tier display names and visibility from Clerk Commerce.
// This should be called on startup and when receiving plan update webhooks.
// Thread-safe for concurrent access.
//
// Visibility is controlled by Clerk's "Publicly available" toggle on each plan.
// Tiers not present in Clerk keep their hardcoded defaults.
func UpdateTierMetadata(metadata []TierMetadata) {
	tiersMu.Lock()
	defer tiersMu.Unlock()

	for _, m := range metadata {
		// Normalize the slug to match our internal tier names
		tierName := NormalizeTierName(m.Slug)

		if tier, ok := Tiers[tierName]; ok {
			// Update display name from Clerk
			if m.DisplayName != "" {
				tier.DisplayName = m.DisplayName
			}

			// Update visibility from Clerk's "Publicly available" setting
			tier.Visible = m.Visible

			Tiers[tierName] = tier
		}
	}
}

// GetVisibleTiers returns all tiers that are marked as visible.
// Thread-safe for concurrent access.
func GetVisibleTiers() map[string]TierLimits {
	tiersMu.RLock()
	defer tiersMu.RUnlock()

	visible := make(map[string]TierLimits)
	for name, tier := range Tiers {
		if tier.Visible {
			visible[name] = tier
		}
	}
	return visible
}
