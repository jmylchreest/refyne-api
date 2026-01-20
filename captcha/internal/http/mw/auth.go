// Package mw contains HTTP middleware for the captcha service.
package mw

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jmylchreest/refyne-api/captcha/internal/auth"
)

// ContextKey is a type for context keys.
type ContextKey string

const (
	// UserClaimsKey is the context key for user claims.
	UserClaimsKey ContextKey = "user_claims"
)

// UserClaims represents unified user claims from any auth source.
type UserClaims struct {
	UserID   string   // Clerk user ID (sub claim)
	Email    string
	Name     string
	Tier     string   // From Clerk public_metadata.subscription.tier
	Features []string // From Clerk Commerce "fea" claim
}

// HasFeature checks if the user has a specific feature.
// Supports wildcard patterns with trailing asterisk (e.g., "captcha_*").
func (c *UserClaims) HasFeature(pattern string) bool {
	if c == nil || len(c.Features) == 0 {
		return false
	}

	// Wildcard match (e.g., "captcha_*")
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
func (c *UserClaims) HasAnyFeature(features []string) bool {
	for _, f := range features {
		if c.HasFeature(f) {
			return true
		}
	}
	return false
}

// GetUserClaims retrieves user claims from context.
func GetUserClaims(ctx context.Context) *UserClaims {
	claims, ok := ctx.Value(UserClaimsKey).(*UserClaims)
	if !ok {
		return nil
	}
	return claims
}

// AuthConfig holds configuration for the auth middleware.
type AuthConfig struct {
	// ClerkVerifier for direct JWT validation (optional)
	ClerkVerifier *auth.ClerkVerifier

	// RefyneAPISecret for validating signed headers from refyne-api (optional)
	// If set, trusts X-Refyne-* headers signed with HMAC
	RefyneAPISecret string

	// RequiredFeature is the feature that must be present (e.g., "captcha")
	RequiredFeature string

	// Logger for auth events
	Logger *slog.Logger
}

// Auth returns authentication middleware that supports:
// 1. Signed headers from refyne-api (if RefyneAPISecret is set)
// 2. Direct Clerk JWT validation (if ClerkVerifier is set)
func Auth(cfg AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var claims *UserClaims
			var err error

			// First, try refyne-api signed headers (internal service-to-service)
			if cfg.RefyneAPISecret != "" {
				claims, err = validateSignedHeaders(r, cfg.RefyneAPISecret)
				if err == nil && claims != nil {
					// Successfully validated signed headers
					ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
					r = r.WithContext(ctx)

					// Check required feature
					if cfg.RequiredFeature != "" && !claims.HasFeature(cfg.RequiredFeature) {
						writeFeatureError(w, cfg.RequiredFeature, claims.Tier)
						return
					}

					next.ServeHTTP(w, r)
					return
				}
			}

			// Fall back to direct JWT validation
			if cfg.ClerkVerifier != nil {
				authHeader := r.Header.Get("Authorization")
				if authHeader == "" {
					http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
					return
				}

				var token string
				if strings.HasPrefix(authHeader, "Bearer ") {
					token = strings.TrimPrefix(authHeader, "Bearer ")
				} else {
					token = authHeader
				}

				// Only support JWT (not API keys directly - they should come via signed headers)
				claims, err = validateClerkToken(cfg.ClerkVerifier, token)
				if err != nil {
					if cfg.Logger != nil {
						cfg.Logger.Debug("JWT validation failed", "error", err)
					}
					http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
					return
				}

				ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
				r = r.WithContext(ctx)

				// Check required feature
				if cfg.RequiredFeature != "" && !claims.HasFeature(cfg.RequiredFeature) {
					writeFeatureError(w, cfg.RequiredFeature, claims.Tier)
					return
				}

				next.ServeHTTP(w, r)
				return
			}

			// No auth method configured
			http.Error(w, `{"error":"authentication not configured"}`, http.StatusUnauthorized)
		})
	}
}

// validateSignedHeaders validates the X-Refyne-* headers from refyne-api.
// Headers:
//   - X-Refyne-Signature: HMAC-SHA256 signature
//   - X-Refyne-Timestamp: Unix timestamp (for replay protection)
//   - X-Refyne-User-ID: User ID
//   - X-Refyne-Tier: User tier
//   - X-Refyne-Features: Comma-separated features
func validateSignedHeaders(r *http.Request, secret string) (*UserClaims, error) {
	signature := r.Header.Get("X-Refyne-Signature")
	timestamp := r.Header.Get("X-Refyne-Timestamp")
	userID := r.Header.Get("X-Refyne-User-ID")

	if signature == "" || timestamp == "" || userID == "" {
		return nil, nil // Not using signed headers
	}

	// Verify timestamp (within 5 minutes)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return nil, err
	}
	if time.Now().Unix()-ts > 300 {
		return nil, ErrTimestampExpired
	}

	// Build message to verify
	tier := r.Header.Get("X-Refyne-Tier")
	features := r.Header.Get("X-Refyne-Features")
	email := r.Header.Get("X-Refyne-Email")

	message := timestamp + ":" + userID + ":" + tier + ":" + features

	// Verify HMAC signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return nil, ErrInvalidSignature
	}

	// Parse features
	var featureList []string
	if features != "" {
		featureList = strings.Split(features, ",")
		for i, f := range featureList {
			featureList[i] = strings.TrimSpace(f)
		}
	}

	return &UserClaims{
		UserID:   userID,
		Email:    email,
		Tier:     tier,
		Features: featureList,
	}, nil
}

// validateClerkToken validates a Clerk JWT and converts to UserClaims.
func validateClerkToken(verifier *auth.ClerkVerifier, tokenString string) (*UserClaims, error) {
	clerkClaims, err := verifier.VerifyToken(tokenString)
	if err != nil {
		return nil, err
	}

	name := clerkClaims.FullName
	if name == "" && (clerkClaims.FirstName != "" || clerkClaims.LastName != "") {
		name = strings.TrimSpace(clerkClaims.FirstName + " " + clerkClaims.LastName)
	}

	return &UserClaims{
		UserID:   clerkClaims.UserID,
		Email:    clerkClaims.Email,
		Name:     name,
		Tier:     clerkClaims.GetTier(),
		Features: clerkClaims.GetFeatures(),
	}, nil
}

// writeFeatureError writes a feature-not-available error response.
func writeFeatureError(w http.ResponseWriter, feature, tier string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   "feature_not_available",
		"message": "The captcha feature is not available on your current plan",
		"feature": feature,
		"tier":    tier,
	})
}

// RequireFeature returns middleware that requires a specific feature.
func RequireFeature(feature string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetUserClaims(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			if !claims.HasFeature(feature) {
				writeFeatureError(w, feature, claims.Tier)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Errors
var (
	ErrTimestampExpired = &AuthError{Message: "timestamp expired"}
	ErrInvalidSignature = &AuthError{Message: "invalid signature"}
)

// AuthError represents an authentication error.
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}
