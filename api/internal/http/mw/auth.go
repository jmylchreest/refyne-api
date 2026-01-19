// Package mw contains HTTP middleware for the refyne-api.
package mw

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jmylchreest/refyne-api/internal/auth"
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

			// Check if it's an API key (starts with rf_)
			if strings.HasPrefix(token, "rf_") {
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

			// Check if it's an API key (starts with rf_)
			if strings.HasPrefix(token, "rf_") {
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
