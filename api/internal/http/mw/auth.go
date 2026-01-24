// Package mw contains HTTP middleware for the refyne-api.
package mw

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jmylchreest/refyne-api/internal/auth"
	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// ContextKey is a type for context keys.
type ContextKey string

const (
	// UserClaimsKey is the context key for user claims.
	UserClaimsKey ContextKey = "user_claims"
)

// UserClaims represents unified user claims from any auth source.
type UserClaims struct {
	UserID           string   // Clerk user ID (sub claim)
	Email            string
	Name             string
	Tier             string   // From Clerk public_metadata.subscription.tier
	GlobalSuperadmin bool     // From Clerk public_metadata.global_superadmin
	Features         []string // From Clerk Commerce "fea" claim
	Scopes           []string // For API keys
	IsAPIKey         bool     // True if authenticated via API key
	LLMProvider      string   // For S3 API keys: forced LLM provider
	LLMModel         string   // For S3 API keys: forced LLM model
}

// HasFeature checks if the user has a specific feature.
// Supports wildcard patterns with trailing asterisk (e.g., "provider_*", "selfhosted_*").
func (c *UserClaims) HasFeature(pattern string) bool {
	if c == nil || len(c.Features) == 0 {
		return false
	}

	// Wildcard match (e.g., "provider_*", "selfhosted_*")
	if strings.HasSuffix(pattern, "_*") {
		prefix := strings.TrimSuffix(pattern, "*")
		for _, f := range c.Features {
			if strings.HasPrefix(f, prefix) {
				return true
			}
		}
		return false
	}

	// Exact match
	for _, f := range c.Features {
		if f == pattern {
			return true
		}
	}
	return false
}

// HasAllFeatures checks if the user has ALL specified features.
// Each feature can be an exact match or wildcard pattern.
func (c *UserClaims) HasAllFeatures(features []string) bool {
	if len(features) == 0 {
		return true
	}
	for _, f := range features {
		if !c.HasFeature(f) {
			return false
		}
	}
	return true
}

// HasAnyFeature checks if the user has ANY of the specified features.
// Each feature can be an exact match or wildcard pattern.
func (c *UserClaims) HasAnyFeature(features []string) bool {
	for _, f := range features {
		if c.HasFeature(f) {
			return true
		}
	}
	return false
}

// Auth returns an authentication middleware that supports both Clerk JWTs and API keys.
// If subCache is provided, API key auth will fetch tier/features from Clerk.
func Auth(clerkVerifier *auth.ClerkVerifier, authSvc *service.AuthService, subCache *auth.SubscriptionCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			// Extract token
			var token string
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			} else {
				token = authHeader
			}

			var claims *UserClaims
			var err error

			// Check S3-configured API keys first (demo/partner/internal keys)
			// These keys use the rfs_ prefix (refyne static)
			if strings.HasPrefix(token, "rfs_") {
				if s3Claims := validateS3APIKey(r, token); s3Claims != nil {
					ctx := context.WithValue(r.Context(), UserClaimsKey, s3Claims)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Check if it's a user API key (starts with rf_ but not rfs_)
			if strings.HasPrefix(token, "rf_") && !strings.HasPrefix(token, "rfs_") {
				claims, err = validateAPIKey(r.Context(), authSvc, subCache, token)
			} else {
				claims, err = validateClerkToken(clerkVerifier, token)
			}

			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			// Add claims to context
			ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// validateClerkToken validates a Clerk JWT and converts to UserClaims.
func validateClerkToken(verifier *auth.ClerkVerifier, tokenString string) (*UserClaims, error) {
	if verifier == nil {
		return nil, auth.ErrInvalidToken
	}
	clerkClaims, err := verifier.VerifyToken(tokenString)
	if err != nil {
		return nil, err
	}

	// Extract global superadmin status from public metadata
	// NOTE: Clerk JWTs don't include public_metadata by default!
	// You must configure a JWT template in Clerk Dashboard to include it:
	// { "public_metadata": "{{user.public_metadata}}" }
	globalSuperadmin := false
	if clerkClaims.PublicMetadata != nil {
		if superadmin, ok := clerkClaims.PublicMetadata["global_superadmin"].(bool); ok {
			globalSuperadmin = superadmin
		}
	}

	// Build full name
	name := clerkClaims.FullName
	if name == "" && (clerkClaims.FirstName != "" || clerkClaims.LastName != "") {
		name = strings.TrimSpace(clerkClaims.FirstName + " " + clerkClaims.LastName)
	}

	features := clerkClaims.GetFeatures()
	tier := clerkClaims.GetTier()

	// Debug: log what features and tier we're receiving from Clerk
	slog.Debug("clerk token validated",
		"user_id", clerkClaims.UserID,
		"tier", tier,
		"features", features,
		"raw_fea_claim", clerkClaims.Features,
		"raw_pla_claim", clerkClaims.Plan,
	)

	return &UserClaims{
		UserID:           clerkClaims.UserID,
		Email:            clerkClaims.Email,
		Name:             name,
		Tier:             tier,
		GlobalSuperadmin: globalSuperadmin,
		Features:         features,
		Scopes:           []string{"*"}, // JWT tokens have all scopes
		IsAPIKey:         false,
	}, nil
}

// validateAPIKey validates an API key via the auth service.
// If subCache is provided, it will fetch tier/features from Clerk.
func validateAPIKey(ctx context.Context, authSvc *service.AuthService, subCache *auth.SubscriptionCache, apiKey string) (*UserClaims, error) {
	tokenClaims, err := authSvc.ValidateAPIKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}

	claims := &UserClaims{
		UserID:           tokenClaims.UserID,
		Email:            tokenClaims.Email,
		Tier:             tokenClaims.Tier,
		GlobalSuperadmin: tokenClaims.GlobalSuperadmin,
		Scopes:           tokenClaims.Scopes,
		IsAPIKey:         true,
	}

	// If we have a subscription cache, hydrate tier/features from Clerk
	if subCache != nil {
		sub, err := subCache.GetSubscription(ctx, tokenClaims.UserID)
		if err != nil {
			// Log but don't fail - use defaults from tokenClaims
			slog.Warn("failed to fetch subscription for API key",
				"user_id", tokenClaims.UserID,
				"error", err,
			)
		} else if sub != nil && sub.Status == "active" {
			// Override tier from subscription
			claims.Tier = sub.PlanSlug

			// Extract feature slugs
			features := make([]string, 0, len(sub.Features))
			for _, f := range sub.Features {
				features = append(features, f.Slug)
			}
			claims.Features = features

			slog.Debug("hydrated API key claims from Clerk subscription",
				"user_id", tokenClaims.UserID,
				"tier", claims.Tier,
				"features", claims.Features,
			)
		}
	}

	return claims, nil
}

// validateS3APIKey validates an S3-configured API key and its restrictions.
// Returns nil if the key is not found in S3 config or restrictions fail.
func validateS3APIKey(r *http.Request, apiKey string) *UserClaims {
	keyConfig := config.GetAPIKeyConfigWithS3(r.Context(), apiKey)
	if keyConfig == nil {
		return nil // Not an S3-configured key
	}

	// Validate endpoint restriction
	if !keyConfig.ValidateEndpoint(r.Method, r.URL.Path) {
		slog.Warn("S3 API key endpoint restriction failed",
			"key_name", keyConfig.Name,
			"method", r.Method,
			"path", r.URL.Path,
		)
		return nil
	}

	// Validate referrer restriction
	// Check multiple headers in priority order:
	// 1. Referer - standard browser header
	// 2. X-Referer - custom header for server-to-server calls (Cloudflare Workers may strip Referer)
	// 3. Origin - fallback for CORS requests
	referrer := r.Header.Get("Referer")
	if referrer == "" {
		referrer = r.Header.Get("X-Referer")
	}
	if referrer == "" {
		referrer = r.Header.Get("Origin")
	}
	if !keyConfig.ValidateReferrer(referrer) {
		slog.Warn("S3 API key referrer restriction failed",
			"key_name", keyConfig.Name,
			"referrer", referrer,
			"referer_header", r.Header.Get("Referer"),
			"x_referer_header", r.Header.Get("X-Referer"),
			"origin_header", r.Header.Get("Origin"),
		)
		return nil
	}

	slog.Info("S3 API key validated",
		"key_name", keyConfig.Name,
		"client_id", keyConfig.Identity.ClientID,
		"tier", keyConfig.Identity.Tier,
	)

	claims := &UserClaims{
		UserID:           keyConfig.Identity.ClientID,
		Tier:             keyConfig.Identity.Tier,
		Features:         keyConfig.Identity.Features,
		Scopes:           keyConfig.Identity.Features, // Use features as scopes
		IsAPIKey:         true,
		GlobalSuperadmin: false,
	}

	// Inject LLM config if specified (bypasses fallback chain)
	if keyConfig.LLMConfig != nil {
		claims.LLMProvider = keyConfig.LLMConfig.Provider
		claims.LLMModel = keyConfig.LLMConfig.Model
	}

	return claims
}

// GetUserClaims retrieves user claims from context.
func GetUserClaims(ctx context.Context) *UserClaims {
	claims, ok := ctx.Value(UserClaimsKey).(*UserClaims)
	if !ok {
		return nil
	}
	return claims
}

// RequireSuperadmin returns middleware that requires global superadmin access.
func RequireSuperadmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetUserClaims(r.Context())
			if claims == nil || !claims.GlobalSuperadmin {
				http.Error(w, `{"error":"superadmin access required"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireScope returns middleware that requires specific scopes (for API keys).
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetUserClaims(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// JWT tokens have all scopes
			if !claims.IsAPIKey {
				next.ServeHTTP(w, r)
				return
			}

			// Check API key scopes
			for _, s := range claims.Scopes {
				if s == scope || s == "*" {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(w, `{"error":"insufficient scope"}`, http.StatusForbidden)
		})
	}
}

// OptionalAuth returns middleware that validates auth if present but allows unauthenticated requests.
// If subCache is provided, API key auth will fetch tier/features from Clerk.
func OptionalAuth(clerkVerifier *auth.ClerkVerifier, authSvc *service.AuthService, subCache *auth.SubscriptionCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				// No auth provided, continue without claims
				next.ServeHTTP(w, r)
				return
			}

			// Extract token
			var token string
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			} else {
				token = authHeader
			}

			var claims *UserClaims

			// Check S3-configured API keys first (demo/partner/internal keys)
			// These keys use the rfs_ prefix (refyne static)
			if strings.HasPrefix(token, "rfs_") {
				if s3Claims := validateS3APIKey(r, token); s3Claims != nil {
					ctx := context.WithValue(r.Context(), UserClaimsKey, s3Claims)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Check if it's a user API key (starts with rf_ but not rfs_)
			if strings.HasPrefix(token, "rf_") && !strings.HasPrefix(token, "rfs_") {
				claims, _ = validateAPIKey(r.Context(), authSvc, subCache, token)
			} else {
				claims, _ = validateClerkToken(clerkVerifier, token)
			}

			// Add claims to context if valid (nil if invalid)
			if claims != nil {
				ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
