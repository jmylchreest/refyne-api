// Package mw provides HTTP middleware for the Refyne API.
package mw

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jmylchreest/refyne-api/internal/constants"
)

// CachePolicy defines caching behavior for a route pattern.
type CachePolicy struct {
	// Pattern is the route pattern to match (prefix match by default).
	Pattern string
	// CacheControl is the Cache-Control header value to set.
	CacheControl string
}

// CacheConfig holds the cache middleware configuration.
type CacheConfig struct {
	// Policies are the cache policies to apply, matched in order.
	Policies []CachePolicy
	// DefaultPolicy is applied when no policy matches (empty = no header set).
	DefaultPolicy string
}

// DefaultCacheConfig returns sensible cache defaults for the API.
// Public endpoints get CDN-friendly caching, K8s probes are never cached,
// protected endpoints get private caching with appropriate TTLs.
func DefaultCacheConfig() CacheConfig {
	// Convert duration constants to seconds for Cache-Control header
	shortSecs := int(constants.CacheMaxAgeShort.Seconds())
	mediumSecs := int(constants.CacheMaxAgeMedium.Seconds())
	longSecs := int(constants.CacheMaxAgeLong.Seconds())

	return CacheConfig{
		DefaultPolicy: "private, no-cache",
		Policies: []CachePolicy{
			// Public endpoints - CDN cacheable
			{Pattern: "/api/v1/health", CacheControl: fmt.Sprintf("public, max-age=%d", shortSecs)},
			{Pattern: "/api/v1/pricing/tiers", CacheControl: fmt.Sprintf("public, max-age=%d, stale-while-revalidate=60", mediumSecs)},

			// K8s probes - never cache (must reflect real-time state)
			{Pattern: "/healthz", CacheControl: "no-store"},
			{Pattern: "/readyz", CacheControl: "no-store"},

			// Protected endpoints - stable data (long cache)
			{Pattern: "/api/v1/llm/providers", CacheControl: fmt.Sprintf("private, max-age=%d", longSecs)},
			{Pattern: "/api/v1/llm/models/", CacheControl: fmt.Sprintf("private, max-age=%d", longSecs)},
			{Pattern: "/api/v1/schemas", CacheControl: fmt.Sprintf("private, max-age=%d", mediumSecs)},

			// Protected endpoints - dynamic data (no cache)
			{Pattern: "/api/v1/usage", CacheControl: "private, no-cache"},
			{Pattern: "/api/v1/jobs", CacheControl: "private, no-cache"},

			// SSE streaming - already handled in handler, but belt-and-suspenders
			{Pattern: "/stream", CacheControl: "no-cache"},
		},
	}
}

// Cache returns middleware that sets Cache-Control headers based on route patterns.
// For non-GET/HEAD requests, it sets "no-store" to prevent caching of mutations.
// For GET/HEAD requests, it matches against configured policies in order.
func Cache(cfg CacheConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Non-GET/HEAD requests should never be cached
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Cache-Control", "no-store")
				next.ServeHTTP(w, r)
				return
			}

			// Find matching policy (first match wins)
			path := r.URL.Path
			for _, policy := range cfg.Policies {
				if matchesPattern(path, policy.Pattern) {
					w.Header().Set("Cache-Control", policy.CacheControl)
					next.ServeHTTP(w, r)
					return
				}
			}

			// Apply default policy if no match and default is set
			if cfg.DefaultPolicy != "" {
				w.Header().Set("Cache-Control", cfg.DefaultPolicy)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// matchesPattern checks if the path matches the pattern.
// Supports prefix matching and substring matching for patterns like "/stream".
func matchesPattern(path, pattern string) bool {
	// Exact match or prefix match
	if path == pattern || strings.HasPrefix(path, pattern) {
		return true
	}
	// Substring match for patterns that might appear mid-path (e.g., "/stream")
	if strings.Contains(path, pattern) {
		return true
	}
	return false
}
