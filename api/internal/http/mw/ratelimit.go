package mw

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"
	"github.com/jmylchreest/refyne-api/internal/constants"
)

// RateLimitConfig holds configuration for rate limiting.
type RateLimitConfig struct {
	// TierLimits maps tier names to their requests per minute limit
	// A value of 0 means unlimited (no rate limiting applied)
	TierLimits map[string]int
	// IPRequestsPerMinute is a fallback rate limit by IP for unauthenticated requests
	IPRequestsPerMinute int
}

// DefaultRateLimitConfig returns defaults from the constants package.
func DefaultRateLimitConfig() RateLimitConfig {
	tierLimits := make(map[string]int)
	for _, tier := range []string{constants.TierFree, constants.TierStandard, constants.TierPro, constants.TierSelfHosted} {
		limits := constants.GetTierLimits(tier)
		tierLimits[tier] = limits.RequestsPerMinute
	}
	return RateLimitConfig{
		TierLimits:          tierLimits,
		IPRequestsPerMinute: constants.GlobalIPRateLimitPerMinute,
	}
}

// RateLimitByUser returns a middleware that rate limits by user ID.
// Should be applied AFTER authentication middleware.
// Falls back to IP-based limiting if user is not authenticated.
// Honors RequestsPerMinute=0 as unlimited (no rate limiting).
func RateLimitByUser(cfg RateLimitConfig) func(http.Handler) http.Handler {
	// Create a limiter for each tier that has a non-zero limit
	tierLimiters := make(map[string]*httprate.RateLimiter)
	for tier, limit := range cfg.TierLimits {
		if limit > 0 {
			tierLimiters[tier] = httprate.NewRateLimiter(
				limit,
				time.Minute,
				httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
					claims := GetUserClaims(r.Context())
					if claims == nil || claims.UserID == "" {
						// Fall back to IP if no user
						return httprate.KeyByIP(r)
					}
					return "user:" + claims.UserID, nil
				}),
			)
		}
		// If limit == 0, no limiter is created (unlimited)
	}

	// Create a fallback limiter for unauthenticated/unknown tiers
	fallbackLimiter := httprate.NewRateLimiter(
		cfg.IPRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(httprate.KeyByIP),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetUserClaims(r.Context())

			// Determine the user's tier
			tier := constants.TierFree
			if claims != nil && claims.Tier != "" {
				tier = claims.Tier
			}

			// Normalize tier name (e.g., "tier_v1_standard" -> "standard")
			normalizedTier := constants.NormalizeTierName(tier)

			// Check if this tier has unlimited rate (0)
			if limit, ok := cfg.TierLimits[normalizedTier]; ok && limit == 0 {
				// Unlimited - bypass rate limiting
				next.ServeHTTP(w, r)
				return
			}

			// Get the limiter for this tier
			limiter, ok := tierLimiters[normalizedTier]
			if !ok {
				// Unknown tier or unauthenticated - use fallback
				limiter = fallbackLimiter
			}

			limiter.Handler(next).ServeHTTP(w, r)
		})
	}
}

// RateLimitByIP returns a middleware that rate limits by IP address.
// Useful for public endpoints or as a global fallback.
func RateLimitByIP(requestsPerMinute int) func(http.Handler) http.Handler {
	return httprate.LimitByIP(requestsPerMinute, time.Minute)
}

// RateLimitGlobal returns a middleware that applies a global rate limit
// to prevent overall system overload. Uses a sliding window.
func RateLimitGlobal(requestsPerMinute int) func(http.Handler) http.Handler {
	return httprate.Limit(
		requestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
			return "global", nil
		}),
	)
}
