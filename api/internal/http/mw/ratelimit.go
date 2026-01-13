package mw

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"
	"github.com/jmylchreest/refyne-api/internal/constants"
)

// RateLimitConfig holds configuration for rate limiting.
type RateLimitConfig struct {
	// FreeRequestsPerMinute is the rate limit for free tier users
	FreeRequestsPerMinute int
	// PaidRequestsPerMinute is the rate limit for paid tier users (starter, pro)
	PaidRequestsPerMinute int
	// IPRequestsPerMinute is a fallback rate limit by IP for unauthenticated requests
	IPRequestsPerMinute int
}

// DefaultRateLimitConfig returns defaults from the constants package.
func DefaultRateLimitConfig() RateLimitConfig {
	freeLimits := constants.GetTierLimits(constants.TierFree)
	standardLimits := constants.GetTierLimits(constants.TierStandard)
	return RateLimitConfig{
		FreeRequestsPerMinute: freeLimits.RequestsPerMinute,
		PaidRequestsPerMinute: standardLimits.RequestsPerMinute,
		IPRequestsPerMinute:   constants.GlobalIPRateLimitPerMinute,
	}
}

// RateLimitByUser returns a middleware that rate limits by user ID.
// Should be applied AFTER authentication middleware.
// Falls back to IP-based limiting if user is not authenticated.
func RateLimitByUser(cfg RateLimitConfig) func(http.Handler) http.Handler {
	// Create limiters for each tier
	freeLimiter := httprate.NewRateLimiter(
		cfg.FreeRequestsPerMinute,
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

	paidLimiter := httprate.NewRateLimiter(
		cfg.PaidRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
			claims := GetUserClaims(r.Context())
			if claims == nil || claims.UserID == "" {
				return httprate.KeyByIP(r)
			}
			return "user:" + claims.UserID, nil
		}),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetUserClaims(r.Context())

			var limiter *httprate.RateLimiter
			if claims == nil || claims.Tier == "" || claims.Tier == "free" {
				limiter = freeLimiter
			} else {
				limiter = paidLimiter
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
