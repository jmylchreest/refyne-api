package auth

import (
	"context"
	"testing"
)

func TestClerkClaims_GetTier(t *testing.T) {
	tests := []struct {
		name   string
		claims *ClerkClaims
		want   string
	}{
		{
			name: "from Plan field with u: prefix",
			claims: &ClerkClaims{
				Plan: "u:tier_v1_pro",
			},
			want: "tier_v1_pro",
		},
		{
			name: "from Plan field with o: prefix",
			claims: &ClerkClaims{
				Plan: "o:tier_v1_enterprise",
			},
			want: "tier_v1_enterprise",
		},
		{
			name: "from public_metadata",
			claims: &ClerkClaims{
				PublicMetadata: map[string]interface{}{
					"subscription": map[string]interface{}{
						"tier": "premium",
					},
				},
			},
			want: "premium",
		},
		{
			name: "Plan takes precedence over metadata",
			claims: &ClerkClaims{
				Plan: "u:pro",
				PublicMetadata: map[string]interface{}{
					"subscription": map[string]interface{}{
						"tier": "basic",
					},
				},
			},
			want: "pro",
		},
		{
			name:   "default to free",
			claims: &ClerkClaims{},
			want:   "free",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.claims.GetTier(); got != tt.want {
				t.Errorf("GetTier() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClerkClaims_GetFeatures(t *testing.T) {
	tests := []struct {
		name   string
		claims *ClerkClaims
		want   []string
	}{
		{
			name: "comma separated features",
			claims: &ClerkClaims{
				Features: "u:captcha,u:api_access,o:rate_limit",
			},
			want: []string{"captcha", "api_access", "rate_limit"},
		},
		{
			name: "single feature",
			claims: &ClerkClaims{
				Features: "u:captcha",
			},
			want: []string{"captcha"},
		},
		{
			name:   "empty features",
			claims: &ClerkClaims{},
			want:   nil,
		},
		{
			name: "features with whitespace",
			claims: &ClerkClaims{
				Features: " u:captcha , u:api_access ",
			},
			want: []string{"captcha", "api_access"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.claims.GetFeatures()
			if len(got) != len(tt.want) {
				t.Errorf("GetFeatures() = %v, want %v", got, tt.want)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("GetFeatures()[%d] = %q, want %q", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestStripClerkPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"u:feature", "feature"},
		{"o:feature", "feature"},
		{"feature", "feature"},
		{"u:", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := stripClerkPrefix(tt.input); got != tt.want {
				t.Errorf("stripClerkPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetClaimsFromContext(t *testing.T) {
	t.Run("claims present", func(t *testing.T) {
		ctx := context.Background()
		expected := &ClerkClaims{UserID: "user_123"}
		ctx = context.WithValue(ctx, ClerkClaimsKey, expected)

		claims := GetClaimsFromContext(ctx)
		if claims == nil || claims.UserID != expected.UserID {
			t.Errorf("GetClaimsFromContext() = %+v, want %+v", claims, expected)
		}
	})

	t.Run("claims absent", func(t *testing.T) {
		ctx := context.Background()
		claims := GetClaimsFromContext(ctx)
		if claims != nil {
			t.Errorf("GetClaimsFromContext() = %+v, want nil", claims)
		}
	})
}

func TestNewClerkVerifier(t *testing.T) {
	issuer := "https://example.clerk.accounts.dev"
	verifier := NewClerkVerifier(issuer)

	if verifier.issuer != issuer {
		t.Errorf("verifier.issuer = %q, want %q", verifier.issuer, issuer)
	}

	expectedJWKSURL := issuer + "/.well-known/jwks.json"
	if verifier.jwksURL != expectedJWKSURL {
		t.Errorf("verifier.jwksURL = %q, want %q", verifier.jwksURL, expectedJWKSURL)
	}

	// Test trailing slash is removed
	verifier2 := NewClerkVerifier(issuer + "/")
	if verifier2.issuer != issuer {
		t.Errorf("verifier.issuer with trailing slash = %q, want %q", verifier2.issuer, issuer)
	}
}
