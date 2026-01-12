package mw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jmylchreest/refyne-api/internal/service"
)

// TierLimits defines the limits for each subscription tier.
// These limits come from Clerk's subscription tiers.
type TierLimits struct {
	MonthlyExtractions int  // Max extractions per month (0 = unlimited)
	MaxPagesPerCrawl   int  // Max pages per crawl job
	MaxConcurrentCrawls int // Max concurrent crawl jobs
	WebhooksEnabled    bool // Whether webhooks are available
	BYOKEnabled        bool // Whether BYOK (bring your own key) is available
	AntiBotEnabled     bool // Whether anti-bot features are available
}

// TierConfig maps tier names to their limits.
var TierConfig = map[string]TierLimits{
	"free": {
		MonthlyExtractions:  100,
		MaxPagesPerCrawl:    10,
		MaxConcurrentCrawls: 1,
		WebhooksEnabled:     false,
		BYOKEnabled:         false,
		AntiBotEnabled:      false,
	},
	"starter": {
		MonthlyExtractions:  1000,
		MaxPagesPerCrawl:    50,
		MaxConcurrentCrawls: 3,
		WebhooksEnabled:     true,
		BYOKEnabled:         true,
		AntiBotEnabled:      false,
	},
	"pro": {
		MonthlyExtractions:  0, // Unlimited
		MaxPagesPerCrawl:    500,
		MaxConcurrentCrawls: 10,
		WebhooksEnabled:     true,
		BYOKEnabled:         true,
		AntiBotEnabled:      true,
	},
	// Self-hosted mode has no limits
	"selfhosted": {
		MonthlyExtractions:  0,
		MaxPagesPerCrawl:    0,
		MaxConcurrentCrawls: 0,
		WebhooksEnabled:     true,
		BYOKEnabled:         true,
		AntiBotEnabled:      true,
	},
}

// GetTierLimits returns the limits for a given tier.
func GetTierLimits(tier string) TierLimits {
	if limits, ok := TierConfig[tier]; ok {
		return limits
	}
	// Default to free tier limits
	return TierConfig["free"]
}

// TierContextKey is the context key for tier limits.
type TierContextKey string

const (
	// TierLimitsKey is the context key for tier limits.
	TierLimitsKey TierContextKey = "tier_limits"
)

// TierGate returns middleware that adds tier limits to context and can optionally
// check usage limits before allowing requests through.
func TierGate(usageSvc *service.UsageService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetUserClaims(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// Get tier limits
			limits := GetTierLimits(claims.Tier)

			// Add limits to context for use in handlers
			ctx := context.WithValue(r.Context(), TierLimitsKey, limits)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireUsageQuota returns middleware that checks if the user has remaining quota.
// This should be applied to extraction/crawl endpoints.
func RequireUsageQuota(usageSvc *service.UsageService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetUserClaims(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// Get tier limits
			limits := GetTierLimits(claims.Tier)

			// If unlimited (0), allow through
			if limits.MonthlyExtractions == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// Check current monthly usage
			usage, err := usageSvc.GetMonthlyUsage(r.Context(), claims.UserID)
			if err != nil {
				http.Error(w, `{"error":"failed to check usage quota"}`, http.StatusInternalServerError)
				return
			}

			// Check if over quota
			if usage.TotalJobs >= limits.MonthlyExtractions {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-RateLimit-Limit", intToString(limits.MonthlyExtractions))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.WriteHeader(http.StatusTooManyRequests)

				response := map[string]any{
					"error":   "monthly quota exceeded",
					"message": getQuotaExceededMessage(claims.Tier, limits.MonthlyExtractions),
					"limit":   limits.MonthlyExtractions,
					"used":    usage.TotalJobs,
					"tier":    claims.Tier,
				}
				json.NewEncoder(w).Encode(response)
				return
			}

			// Add remaining quota to response headers
			remaining := limits.MonthlyExtractions - usage.TotalJobs
			w.Header().Set("X-RateLimit-Limit", intToString(limits.MonthlyExtractions))
			w.Header().Set("X-RateLimit-Remaining", intToString(remaining))

			next.ServeHTTP(w, r)
		})
	}
}

// RequireFeature returns middleware that checks if a feature is enabled for the user's tier.
func RequireFeature(feature string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetUserClaims(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			limits := GetTierLimits(claims.Tier)

			var enabled bool
			switch feature {
			case "webhooks":
				enabled = limits.WebhooksEnabled
			case "byok":
				enabled = limits.BYOKEnabled
			case "antibot":
				enabled = limits.AntiBotEnabled
			default:
				enabled = true // Unknown features default to enabled
			}

			if !enabled {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				response := map[string]any{
					"error":   "feature not available",
					"message": getFeatureNotAvailableMessage(feature, claims.Tier),
					"feature": feature,
					"tier":    claims.Tier,
				}
				json.NewEncoder(w).Encode(response)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetTierLimitsFromContext retrieves tier limits from context.
func GetTierLimitsFromContext(ctx context.Context) *TierLimits {
	limits, ok := ctx.Value(TierLimitsKey).(TierLimits)
	if !ok {
		defaultLimits := TierConfig["free"]
		return &defaultLimits
	}
	return &limits
}

// getQuotaExceededMessage returns a user-friendly message for quota exceeded errors.
func getQuotaExceededMessage(tier string, limit int) string {
	switch tier {
	case "free":
		return "You've reached your free tier limit of 100 extractions this month. Upgrade to Starter for 1,000 monthly extractions."
	case "starter":
		return "You've reached your Starter plan limit of 1,000 extractions this month. Upgrade to Pro for unlimited extractions."
	default:
		return "You've reached your monthly extraction limit. Please contact support or upgrade your plan."
	}
}

// getFeatureNotAvailableMessage returns a user-friendly message for feature not available errors.
func getFeatureNotAvailableMessage(feature, tier string) string {
	switch feature {
	case "webhooks":
		return "Webhooks are not available on the free tier. Upgrade to Starter or Pro to use webhooks."
	case "byok":
		return "Bring Your Own Key (BYOK) is not available on the free tier. Upgrade to Starter or Pro to use your own API keys."
	case "antibot":
		return "Anti-bot features are only available on the Pro plan. Upgrade to Pro for advanced anti-bot capabilities."
	default:
		return "This feature is not available on your current plan. Please upgrade to access this feature."
	}
}

func intToString(n int) string {
	return fmt.Sprintf("%d", n)
}
