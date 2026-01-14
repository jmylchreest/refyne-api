// Package constants defines centralized configuration for tier limits,
// rate limits, and user-facing messages. Change values here to update
// limits across the entire application.
package constants

import (
	"fmt"
	"time"
)

// Tier names
const (
	TierFree       = "free"
	TierStandard   = "standard"
	TierPro        = "pro"
	TierSelfHosted = "selfhosted"
)

// TierLimits defines the limits for a subscription tier.
type TierLimits struct {
	// MonthlyExtractions is the max extractions per month (0 = unlimited)
	MonthlyExtractions int
	// MaxPagesPerCrawl is the max pages per crawl job (0 = unlimited)
	MaxPagesPerCrawl int
	// MaxConcurrentJobs is the max concurrent jobs (0 = unlimited)
	MaxConcurrentJobs int
	// RequestsPerMinute is the rate limit for API requests
	RequestsPerMinute int
	// WebhooksEnabled indicates if webhooks are available
	WebhooksEnabled bool
	// BYOKEnabled indicates if bring-your-own-key is available
	BYOKEnabled bool
	// AntiBotEnabled indicates if anti-bot features are available
	AntiBotEnabled bool
}

// Tiers defines limits for each subscription tier.
// To change tier limits, modify this map.
var Tiers = map[string]TierLimits{
	TierFree: {
		MonthlyExtractions: 100,
		MaxPagesPerCrawl:   10,
		MaxConcurrentJobs:  1,
		RequestsPerMinute:  10,
		WebhooksEnabled:    false,
		BYOKEnabled:        false,
		AntiBotEnabled:     false,
	},
	TierStandard: {
		MonthlyExtractions: 1000,
		MaxPagesPerCrawl:   50,
		MaxConcurrentJobs:  3,
		RequestsPerMinute:  60,
		WebhooksEnabled:    true,
		BYOKEnabled:        true,
		AntiBotEnabled:     false,
	},
	TierPro: {
		MonthlyExtractions: 0, // Unlimited
		MaxPagesPerCrawl:   500,
		MaxConcurrentJobs:  10,
		RequestsPerMinute:  60,
		WebhooksEnabled:    true,
		BYOKEnabled:        true,
		AntiBotEnabled:     true,
	},
	TierSelfHosted: {
		MonthlyExtractions: 0, // Unlimited
		MaxPagesPerCrawl:   0, // Unlimited
		MaxConcurrentJobs:  0, // Unlimited
		RequestsPerMinute:  0, // Unlimited
		WebhooksEnabled:    true,
		BYOKEnabled:        true,
		AntiBotEnabled:     true,
	},
}

// GetTierLimits returns the limits for a tier, defaulting to free tier.
func GetTierLimits(tier string) TierLimits {
	if limits, ok := Tiers[tier]; ok {
		return limits
	}
	return Tiers[TierFree]
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

// QuotaExceededMessage returns a user-friendly message for monthly quota exceeded.
func QuotaExceededMessage(tier string) string {
	limits := GetTierLimits(tier)
	switch tier {
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
	limits := GetTierLimits(tier)
	switch tier {
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
	case "byok":
		return "Bring Your Own Key (BYOK) is not available on your current plan. Upgrade to use your own LLM API keys."
	case "antibot":
		return "Anti-bot features are not available on your current plan. Upgrade for advanced anti-bot capabilities."
	default:
		return "This feature is not available on your current plan. Please upgrade to access this feature."
	}
}
