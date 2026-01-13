package mw

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// TierLimits is an alias to the centralized constants.TierLimits.
// Use constants.GetTierLimits() to retrieve tier limits.
type TierLimits = constants.TierLimits

// GetTierLimits returns the limits for a given tier.
// This delegates to the centralized constants package.
func GetTierLimits(tier string) TierLimits {
	return constants.GetTierLimits(tier)
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
				slog.Error("failed to check usage quota",
					"user_id", claims.UserID,
					"error", err,
				)
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

// RequireFeature returns middleware that checks if a feature is enabled via Clerk Commerce.
// Features are checked against the user's Clerk billing features (from the "fea" JWT claim).
func RequireFeature(feature string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetUserClaims(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// Check if user has the required feature from Clerk Commerce
			hasFeature := claims.HasFeature(feature)

			// Debug log feature check
			slog.Debug("feature check",
				"user_id", claims.UserID,
				"tier", claims.Tier,
				"required_feature", feature,
				"has_feature", hasFeature,
				"user_features", claims.Features,
			)

			if !hasFeature {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				response := map[string]any{
					"error":   "feature_not_available",
					"message": getFeatureNotAvailableMessage(feature),
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
		defaultLimits := constants.Tiers[constants.TierFree]
		return &defaultLimits
	}
	return &limits
}

// getQuotaExceededMessage returns a user-friendly message for quota exceeded errors.
func getQuotaExceededMessage(tier string, _ int) string {
	return constants.QuotaExceededMessage(tier)
}

// getFeatureNotAvailableMessage returns a user-friendly message for feature not available errors.
func getFeatureNotAvailableMessage(feature string) string {
	return constants.FeatureNotAvailableMessage(feature)
}

func intToString(n int) string {
	return fmt.Sprintf("%d", n)
}

// JobService interface for concurrent job checking.
type JobService interface {
	CountActiveJobsByUser(ctx context.Context, userID string) (int, error)
}

// RequireConcurrentJobLimit returns middleware that checks if the user has capacity for more jobs.
// This should be applied to job creation endpoints (extract, crawl).
func RequireConcurrentJobLimit(jobSvc JobService) func(http.Handler) http.Handler {
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
			if limits.MaxConcurrentJobs == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// Check current active jobs
			activeJobs, err := jobSvc.CountActiveJobsByUser(r.Context(), claims.UserID)
			if err != nil {
				slog.Error("failed to count active jobs",
					"user_id", claims.UserID,
					"error", err,
				)
				http.Error(w, `{"error":"failed to check job limit"}`, http.StatusInternalServerError)
				return
			}

			// Check if at limit
			if activeJobs >= limits.MaxConcurrentJobs {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Concurrent-Limit", intToString(limits.MaxConcurrentJobs))
				w.Header().Set("X-Concurrent-Active", intToString(activeJobs))
				w.WriteHeader(http.StatusTooManyRequests)

				response := map[string]any{
					"error":   "concurrent_job_limit_exceeded",
					"message": constants.ConcurrentJobLimitMessage(claims.Tier),
					"limit":   limits.MaxConcurrentJobs,
					"active":  activeJobs,
					"tier":    claims.Tier,
				}
				json.NewEncoder(w).Encode(response)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
