// Package auth handles authentication with Clerk.
package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken  = errors.New("invalid token")
	ErrTokenExpired  = errors.New("token expired")
	ErrMissingClaims = errors.New("missing required claims")
	ErrJWKSFetch     = errors.New("failed to fetch JWKS")
)

// ClerkClaims represents the claims in a Clerk JWT.
type ClerkClaims struct {
	jwt.RegisteredClaims
	UserID          string                 `json:"sub"`
	Email           string                 `json:"email,omitempty"`
	EmailVerified   bool                   `json:"email_verified,omitempty"`
	FullName        string                 `json:"name,omitempty"`
	FirstName       string                 `json:"first_name,omitempty"`
	LastName        string                 `json:"last_name,omitempty"`
	ImageURL        string                 `json:"image_url,omitempty"`
	PublicMetadata  map[string]interface{} `json:"public_metadata,omitempty"`
	PrivateMetadata map[string]interface{} `json:"private_metadata,omitempty"`
	SessionID       string                 `json:"sid,omitempty"`
	OrgID           string                 `json:"org_id,omitempty"`
	OrgRole         string                 `json:"org_role,omitempty"`
	OrgSlug         string                 `json:"org_slug,omitempty"`
	// Clerk Commerce claims
	Plan     string `json:"pla,omitempty"` // Plan/tier from Clerk Commerce (e.g., "u:tier_v1_free")
	Features string `json:"fea,omitempty"` // Features from Clerk Commerce
}

// GetTier returns the subscription tier.
// First checks Clerk Commerce's pla claim (e.g., "u:tier_v1_free"),
// then falls back to public_metadata.subscription.tier.
func (c *ClerkClaims) GetTier() string {
	// Check Clerk Commerce plan claim first (format: "u:tier_slug")
	if c.Plan != "" {
		// Strip "u:" prefix if present
		tier := strings.TrimPrefix(c.Plan, "u:")
		if tier != "" {
			return tier
		}
	}

	// Fall back to public_metadata.subscription.tier
	if c.PublicMetadata != nil {
		if sub, ok := c.PublicMetadata["subscription"].(map[string]interface{}); ok {
			if tier, ok := sub["tier"].(string); ok {
				return tier
			}
		}
	}

	return "free"
}

// ClerkVerifier verifies Clerk JWTs using JWKS.
type ClerkVerifier struct {
	issuer     string
	jwksURL    string
	httpClient *http.Client
	keyCache   *jwksCache
}

// jwksCache caches the JWKS keys.
type jwksCache struct {
	mu        sync.RWMutex
	keys      map[string]interface{}
	expiresAt time.Time
}

// NewClerkVerifier creates a new Clerk JWT verifier.
// The issuer is typically "https://<your-clerk-frontend-api>.clerk.accounts.dev"
func NewClerkVerifier(issuer string) *ClerkVerifier {
	// Normalize issuer
	issuer = strings.TrimSuffix(issuer, "/")

	return &ClerkVerifier{
		issuer:  issuer,
		jwksURL: issuer + "/.well-known/jwks.json",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		keyCache: &jwksCache{
			keys: make(map[string]interface{}),
		},
	}
}

// VerifyToken verifies a Clerk JWT and returns the claims.
func (v *ClerkVerifier) VerifyToken(tokenString string) (*ClerkClaims, error) {
	// Parse the token without verification first to get the key ID
	token, err := jwt.ParseWithClaims(tokenString, &ClerkClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify the signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Get the key ID from the token header
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("missing key ID in token header")
		}

		// Get the public key from JWKS
		return v.getPublicKey(kid)
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*ClerkClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	// Verify issuer
	if claims.Issuer != v.issuer {
		return nil, fmt.Errorf("%w: invalid issuer", ErrInvalidToken)
	}

	// Verify required claims
	if claims.UserID == "" {
		return nil, ErrMissingClaims
	}

	return claims, nil
}

// getPublicKey retrieves a public key from the JWKS cache or fetches it.
func (v *ClerkVerifier) getPublicKey(kid string) (interface{}, error) {
	// Check cache first
	v.keyCache.mu.RLock()
	if key, ok := v.keyCache.keys[kid]; ok && time.Now().Before(v.keyCache.expiresAt) {
		v.keyCache.mu.RUnlock()
		return key, nil
	}
	v.keyCache.mu.RUnlock()

	// Fetch JWKS
	if err := v.refreshJWKS(); err != nil {
		return nil, err
	}

	// Get key from cache
	v.keyCache.mu.RLock()
	defer v.keyCache.mu.RUnlock()

	key, ok := v.keyCache.keys[kid]
	if !ok {
		return nil, fmt.Errorf("key %s not found in JWKS", kid)
	}

	return key, nil
}

// refreshJWKS fetches the JWKS from Clerk and caches the keys.
func (v *ClerkVerifier) refreshJWKS() error {
	v.keyCache.mu.Lock()
	defer v.keyCache.mu.Unlock()

	// Double-check if another goroutine already refreshed
	if time.Now().Before(v.keyCache.expiresAt) {
		return nil
	}

	resp, err := v.httpClient.Get(v.jwksURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrJWKSFetch, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrJWKSFetch, resp.StatusCode)
	}

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("%w: %v", ErrJWKSFetch, err)
	}

	// Parse and cache keys
	newKeys := make(map[string]interface{})
	for _, key := range jwks.Keys {
		if key.Kty != "RSA" || key.Use != "sig" {
			continue
		}

		pubKey, err := parseRSAPublicKey(key.N, key.E)
		if err != nil {
			continue
		}

		newKeys[key.Kid] = pubKey
	}

	v.keyCache.keys = newKeys
	v.keyCache.expiresAt = time.Now().Add(1 * time.Hour)

	return nil
}

// parseRSAPublicKey parses an RSA public key from base64url-encoded N and E values.
func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	// Decode N (modulus)
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nBytes)

	// Decode E (exponent)
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, err
	}
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

// ContextKey is a type for context keys.
type ContextKey string

const (
	// ClerkClaimsKey is the context key for Clerk claims.
	ClerkClaimsKey ContextKey = "clerk_claims"
)

// GetClaimsFromContext retrieves Clerk claims from context.
func GetClaimsFromContext(ctx context.Context) *ClerkClaims {
	claims, ok := ctx.Value(ClerkClaimsKey).(*ClerkClaims)
	if !ok {
		return nil
	}
	return claims
}
