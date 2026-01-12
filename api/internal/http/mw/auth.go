// Package mw contains HTTP middleware for the refyne-api.
package mw

import (
	"context"
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
	Scopes           []string // For API keys
	IsAPIKey         bool     // True if authenticated via API key
}

// Auth returns an authentication middleware that supports both Clerk JWTs and API keys.
func Auth(clerkVerifier *auth.ClerkVerifier, authSvc *service.AuthService) func(http.Handler) http.Handler {
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
				claims, err = validateAPIKey(r.Context(), authSvc, token)
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

	return &UserClaims{
		UserID:           clerkClaims.UserID,
		Email:            clerkClaims.Email,
		Name:             name,
		Tier:             clerkClaims.GetTier(),
		GlobalSuperadmin: globalSuperadmin,
		Scopes:           []string{"*"}, // JWT tokens have all scopes
		IsAPIKey:         false,
	}, nil
}

// validateAPIKey validates an API key via the auth service.
func validateAPIKey(ctx context.Context, authSvc *service.AuthService, apiKey string) (*UserClaims, error) {
	tokenClaims, err := authSvc.ValidateAPIKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}

	return &UserClaims{
		UserID:           tokenClaims.UserID,
		Email:            tokenClaims.Email,
		Tier:             tokenClaims.Tier,
		GlobalSuperadmin: tokenClaims.GlobalSuperadmin,
		Scopes:           tokenClaims.Scopes,
		IsAPIKey:         true,
	}, nil
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
func OptionalAuth(clerkVerifier *auth.ClerkVerifier, authSvc *service.AuthService) func(http.Handler) http.Handler {
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
				claims, _ = validateAPIKey(r.Context(), authSvc, token)
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
