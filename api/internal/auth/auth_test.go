package auth

import (
	"context"
	"testing"
)

// ========================================
// ClerkClaims Tier Methods Tests
// ========================================

func TestClerkClaims_GetUserTier(t *testing.T) {
	tests := []struct {
		name     string
		plan     string
		expected string
	}{
		{"user tier", "u:tier_v1_pro", "tier_v1_pro"},
		{"org tier returns empty", "o:tier_v1_pro", ""},
		{"no prefix returns empty", "tier_v1_pro", ""},
		{"empty plan", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ClerkClaims{Plan: tt.plan}
			got := claims.GetUserTier()
			if got != tt.expected {
				t.Errorf("GetUserTier() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestClerkClaims_GetOrgTier(t *testing.T) {
	tests := []struct {
		name     string
		plan     string
		expected string
	}{
		{"org tier", "o:tier_v1_enterprise", "tier_v1_enterprise"},
		{"user tier returns empty", "u:tier_v1_pro", ""},
		{"no prefix returns empty", "tier_v1_pro", ""},
		{"empty plan", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ClerkClaims{Plan: tt.plan}
			got := claims.GetOrgTier()
			if got != tt.expected {
				t.Errorf("GetOrgTier() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestClerkClaims_GetTier(t *testing.T) {
	tests := []struct {
		name           string
		plan           string
		publicMetadata map[string]interface{}
		expected       string
	}{
		{"user plan", "u:tier_v1_pro", nil, "tier_v1_pro"},
		{"org plan", "o:tier_v1_enterprise", nil, "tier_v1_enterprise"},
		{
			"fallback to public_metadata",
			"",
			map[string]interface{}{
				"subscription": map[string]interface{}{
					"tier": "legacy_tier",
				},
			},
			"legacy_tier",
		},
		{"default to free", "", nil, "free"},
		{
			"plan takes precedence over metadata",
			"u:tier_v1_pro",
			map[string]interface{}{
				"subscription": map[string]interface{}{
					"tier": "should_not_use",
				},
			},
			"tier_v1_pro",
		},
		{
			"tier_override takes precedence over plan",
			"u:tier_v1_free",
			map[string]interface{}{
				"tier_override": "pro",
			},
			"pro",
		},
		{
			"tier_override takes precedence over subscription",
			"",
			map[string]interface{}{
				"tier_override": "enterprise",
				"subscription": map[string]interface{}{
					"tier": "should_not_use",
				},
			},
			"enterprise",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ClerkClaims{
				Plan:           tt.plan,
				PublicMetadata: tt.publicMetadata,
			}
			got := claims.GetTier()
			if got != tt.expected {
				t.Errorf("GetTier() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestClerkClaims_IsUserTier(t *testing.T) {
	tests := []struct {
		name     string
		plan     string
		expected bool
	}{
		{"user tier", "u:tier_v1_pro", true},
		{"org tier", "o:tier_v1_pro", false},
		{"no prefix", "tier_v1_pro", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ClerkClaims{Plan: tt.plan}
			got := claims.IsUserTier()
			if got != tt.expected {
				t.Errorf("IsUserTier() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestClerkClaims_IsOrgTier(t *testing.T) {
	tests := []struct {
		name     string
		plan     string
		expected bool
	}{
		{"org tier", "o:tier_v1_enterprise", true},
		{"user tier", "u:tier_v1_pro", false},
		{"no prefix", "tier_v1_pro", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ClerkClaims{Plan: tt.plan}
			got := claims.IsOrgTier()
			if got != tt.expected {
				t.Errorf("IsOrgTier() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// ========================================
// ClerkClaims Feature Methods Tests
// ========================================

func TestClerkClaims_HasUserFeature(t *testing.T) {
	tests := []struct {
		name     string
		features string
		feature  string
		expected bool
	}{
		{"has user feature", "u:provider_byok,u:models_custom", "provider_byok", true},
		{"missing user feature", "u:provider_byok", "models_custom", false},
		{"has org feature not user", "o:provider_byok", "provider_byok", false},
		{"empty features", "", "provider_byok", false},
		{"with spaces", "u:provider_byok, u:models_custom", "models_custom", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ClerkClaims{Features: tt.features}
			got := claims.HasUserFeature(tt.feature)
			if got != tt.expected {
				t.Errorf("HasUserFeature(%q) = %v, want %v", tt.feature, got, tt.expected)
			}
		})
	}
}

func TestClerkClaims_HasOrgFeature(t *testing.T) {
	tests := []struct {
		name     string
		features string
		feature  string
		expected bool
	}{
		{"has org feature", "o:provider_byok,o:models_custom", "provider_byok", true},
		{"missing org feature", "o:provider_byok", "models_custom", false},
		{"has user feature not org", "u:provider_byok", "provider_byok", false},
		{"empty features", "", "provider_byok", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ClerkClaims{Features: tt.features}
			got := claims.HasOrgFeature(tt.feature)
			if got != tt.expected {
				t.Errorf("HasOrgFeature(%q) = %v, want %v", tt.feature, got, tt.expected)
			}
		})
	}
}

func TestClerkClaims_HasFeature(t *testing.T) {
	tests := []struct {
		name           string
		features       string
		publicMetadata map[string]interface{}
		feature        string
		expected       bool
	}{
		{"has user feature", "u:provider_byok", nil, "provider_byok", true},
		{"has org feature", "o:provider_byok", nil, "provider_byok", true},
		{"has both", "u:provider_byok,o:provider_byok", nil, "provider_byok", true},
		{"missing feature", "u:other_feature", nil, "provider_byok", false},
		{"empty features", "", nil, "provider_byok", false},
		{
			"feature_override grants feature",
			"",
			map[string]interface{}{
				"feature_overrides": []interface{}{"content_dynamic", "provider_byok"},
			},
			"content_dynamic",
			true,
		},
		{
			"feature_override not present",
			"",
			map[string]interface{}{
				"feature_overrides": []interface{}{"other_feature"},
			},
			"content_dynamic",
			false,
		},
		{
			"feature_override combined with clerk features",
			"u:provider_byok",
			map[string]interface{}{
				"feature_overrides": []interface{}{"content_dynamic"},
			},
			"content_dynamic",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ClerkClaims{
				Features:       tt.features,
				PublicMetadata: tt.publicMetadata,
			}
			got := claims.HasFeature(tt.feature)
			if got != tt.expected {
				t.Errorf("HasFeature(%q) = %v, want %v", tt.feature, got, tt.expected)
			}
		})
	}
}

func TestClerkClaims_GetFeatures(t *testing.T) {
	tests := []struct {
		name     string
		features string
		expected []string
	}{
		{
			"mixed features",
			"u:provider_byok,o:models_custom",
			[]string{"provider_byok", "models_custom"},
		},
		{
			"user features only",
			"u:feature1,u:feature2",
			[]string{"feature1", "feature2"},
		},
		{"empty", "", nil},
		{
			"with spaces",
			"u:feature1, o:feature2",
			[]string{"feature1", "feature2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ClerkClaims{Features: tt.features}
			got := claims.GetFeatures()
			if tt.expected == nil {
				if got != nil {
					t.Errorf("GetFeatures() = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.expected) {
				t.Errorf("GetFeatures() length = %d, want %d", len(got), len(tt.expected))
				return
			}
			for i, f := range got {
				if f != tt.expected[i] {
					t.Errorf("GetFeatures()[%d] = %q, want %q", i, f, tt.expected[i])
				}
			}
		})
	}
}

func TestClerkClaims_GetUserFeatures(t *testing.T) {
	tests := []struct {
		name     string
		features string
		expected []string
	}{
		{
			"only user features returned",
			"u:provider_byok,o:models_custom,u:analytics",
			[]string{"provider_byok", "analytics"},
		},
		{"no user features", "o:feature1,o:feature2", []string{}},
		{"empty", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ClerkClaims{Features: tt.features}
			got := claims.GetUserFeatures()
			if tt.expected == nil {
				if got != nil {
					t.Errorf("GetUserFeatures() = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.expected) {
				t.Errorf("GetUserFeatures() length = %d, want %d", len(got), len(tt.expected))
				return
			}
			for i, f := range got {
				if f != tt.expected[i] {
					t.Errorf("GetUserFeatures()[%d] = %q, want %q", i, f, tt.expected[i])
				}
			}
		})
	}
}

func TestClerkClaims_GetOrgFeatures(t *testing.T) {
	tests := []struct {
		name     string
		features string
		expected []string
	}{
		{
			"only org features returned",
			"u:provider_byok,o:models_custom,o:analytics",
			[]string{"models_custom", "analytics"},
		},
		{"no org features", "u:feature1,u:feature2", []string{}},
		{"empty", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ClerkClaims{Features: tt.features}
			got := claims.GetOrgFeatures()
			if tt.expected == nil {
				if got != nil {
					t.Errorf("GetOrgFeatures() = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.expected) {
				t.Errorf("GetOrgFeatures() length = %d, want %d", len(got), len(tt.expected))
				return
			}
			for i, f := range got {
				if f != tt.expected[i] {
					t.Errorf("GetOrgFeatures()[%d] = %q, want %q", i, f, tt.expected[i])
				}
			}
		})
	}
}

func TestClerkClaims_IsOrganizationContext(t *testing.T) {
	tests := []struct {
		name     string
		features string
		expected bool
	}{
		{"org context", "o:feature1,o:feature2", true},
		{"user context", "u:feature1,u:feature2", false},
		{"mixed starts with org", "o:feature1,u:feature2", true},
		{"mixed starts with user", "u:feature1,o:feature2", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ClerkClaims{Features: tt.features}
			got := claims.IsOrganizationContext()
			if got != tt.expected {
				t.Errorf("IsOrganizationContext() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// ========================================
// Helper Function Tests
// ========================================

func TestStripClerkPrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"u:tier_v1_pro", "tier_v1_pro"},
		{"o:tier_v1_enterprise", "tier_v1_enterprise"},
		{"tier_v1_free", "tier_v1_free"},
		{"", ""},
		{"u:", ""},
		{"o:", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripClerkPrefix(tt.input)
			if got != tt.expected {
				t.Errorf("stripClerkPrefix(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ========================================
// ClerkVerifier Tests
// ========================================

func TestNewClerkVerifier(t *testing.T) {
	tests := []struct {
		name           string
		issuer         string
		expectedIssuer string
		expectedJWKS   string
	}{
		{
			"normal issuer",
			"https://clerk.example.com",
			"https://clerk.example.com",
			"https://clerk.example.com/.well-known/jwks.json",
		},
		{
			"issuer with trailing slash",
			"https://clerk.example.com/",
			"https://clerk.example.com",
			"https://clerk.example.com/.well-known/jwks.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewClerkVerifier(tt.issuer)
			if v == nil {
				t.Fatal("expected verifier, got nil")
			}
			if v.issuer != tt.expectedIssuer {
				t.Errorf("issuer = %q, want %q", v.issuer, tt.expectedIssuer)
			}
			if v.jwksURL != tt.expectedJWKS {
				t.Errorf("jwksURL = %q, want %q", v.jwksURL, tt.expectedJWKS)
			}
			if v.httpClient == nil {
				t.Error("httpClient should not be nil")
			}
			if v.keyCache == nil {
				t.Error("keyCache should not be nil")
			}
		})
	}
}

// ========================================
// Context Functions Tests
// ========================================

func TestGetClaimsFromContext(t *testing.T) {
	t.Run("with claims", func(t *testing.T) {
		expected := &ClerkClaims{
			UserID: "user_123",
			Email:  "test@example.com",
		}
		ctx := context.WithValue(context.Background(), ClerkClaimsKey, expected)

		got := GetClaimsFromContext(ctx)
		if got == nil {
			t.Fatal("expected claims, got nil")
		}
		if got.UserID != expected.UserID {
			t.Errorf("UserID = %q, want %q", got.UserID, expected.UserID)
		}
		if got.Email != expected.Email {
			t.Errorf("Email = %q, want %q", got.Email, expected.Email)
		}
	})

	t.Run("without claims", func(t *testing.T) {
		got := GetClaimsFromContext(context.Background())
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("wrong type in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ClerkClaimsKey, "not a claims struct")
		got := GetClaimsFromContext(ctx)
		if got != nil {
			t.Errorf("expected nil for wrong type, got %+v", got)
		}
	})
}

// ========================================
// Error Variable Tests
// ========================================

func TestErrorVariables(t *testing.T) {
	if ErrInvalidToken.Error() != "invalid token" {
		t.Errorf("ErrInvalidToken = %q, want %q", ErrInvalidToken.Error(), "invalid token")
	}
	if ErrTokenExpired.Error() != "token expired" {
		t.Errorf("ErrTokenExpired = %q, want %q", ErrTokenExpired.Error(), "token expired")
	}
	if ErrMissingClaims.Error() != "missing required claims" {
		t.Errorf("ErrMissingClaims = %q, want %q", ErrMissingClaims.Error(), "missing required claims")
	}
	if ErrJWKSFetch.Error() != "failed to fetch JWKS" {
		t.Errorf("ErrJWKSFetch = %q, want %q", ErrJWKSFetch.Error(), "failed to fetch JWKS")
	}
}

// ========================================
// ClerkClaims Struct Tests
// ========================================

func TestClerkClaims_Fields(t *testing.T) {
	claims := ClerkClaims{
		UserID:        "user_abc123",
		Email:         "test@example.com",
		EmailVerified: true,
		FullName:      "Test User",
		FirstName:     "Test",
		LastName:      "User",
		ImageURL:      "https://example.com/avatar.jpg",
		SessionID:     "sess_xyz",
		OrgID:         "org_456",
		OrgRole:       "admin",
		OrgSlug:       "my-org",
		Plan:          "u:tier_v1_pro",
		Features:      "u:provider_byok,u:models_custom",
	}

	if claims.UserID != "user_abc123" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user_abc123")
	}
	if claims.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", claims.Email, "test@example.com")
	}
	if !claims.EmailVerified {
		t.Error("EmailVerified should be true")
	}
	if claims.OrgID != "org_456" {
		t.Errorf("OrgID = %q, want %q", claims.OrgID, "org_456")
	}
	if claims.OrgRole != "admin" {
		t.Errorf("OrgRole = %q, want %q", claims.OrgRole, "admin")
	}
}
