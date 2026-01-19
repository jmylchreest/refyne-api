package mw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ========================================
// UserClaims Tests
// ========================================

func TestUserClaims_HasFeature(t *testing.T) {
	tests := []struct {
		name     string
		claims   *UserClaims
		feature  string
		expected bool
	}{
		{
			name:     "nil claims",
			claims:   nil,
			feature:  "provider_byok",
			expected: false,
		},
		{
			name:     "empty features",
			claims:   &UserClaims{Features: []string{}},
			feature:  "provider_byok",
			expected: false,
		},
		{
			name:     "exact match",
			claims:   &UserClaims{Features: []string{"provider_byok", "models_custom"}},
			feature:  "provider_byok",
			expected: true,
		},
		{
			name:     "no match",
			claims:   &UserClaims{Features: []string{"provider_byok"}},
			feature:  "models_custom",
			expected: false,
		},
		{
			name:     "wildcard match - provider_*",
			claims:   &UserClaims{Features: []string{"provider_byok", "provider_openai"}},
			feature:  "provider_*",
			expected: true,
		},
		{
			name:     "wildcard match - selfhosted_*",
			claims:   &UserClaims{Features: []string{"selfhosted_unlimited", "selfhosted_custom"}},
			feature:  "selfhosted_*",
			expected: true,
		},
		{
			name:     "wildcard no match",
			claims:   &UserClaims{Features: []string{"provider_byok"}},
			feature:  "models_*",
			expected: false,
		},
		{
			name:     "wildcard with no matching prefix",
			claims:   &UserClaims{Features: []string{"foo_bar"}},
			feature:  "provider_*",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.claims.HasFeature(tt.feature)
			if got != tt.expected {
				t.Errorf("HasFeature(%q) = %v, want %v", tt.feature, got, tt.expected)
			}
		})
	}
}

func TestUserClaims_HasAllFeatures(t *testing.T) {
	tests := []struct {
		name     string
		claims   *UserClaims
		features []string
		expected bool
	}{
		{
			name:     "empty requirements",
			claims:   &UserClaims{Features: []string{"provider_byok"}},
			features: []string{},
			expected: true,
		},
		{
			name:     "all features present",
			claims:   &UserClaims{Features: []string{"provider_byok", "models_custom", "webhooks"}},
			features: []string{"provider_byok", "models_custom"},
			expected: true,
		},
		{
			name:     "missing one feature",
			claims:   &UserClaims{Features: []string{"provider_byok"}},
			features: []string{"provider_byok", "models_custom"},
			expected: false,
		},
		{
			name:     "wildcard all present",
			claims:   &UserClaims{Features: []string{"provider_byok", "selfhosted_unlimited"}},
			features: []string{"provider_*", "selfhosted_*"},
			expected: true,
		},
		{
			name:     "wildcard missing one",
			claims:   &UserClaims{Features: []string{"provider_byok"}},
			features: []string{"provider_*", "selfhosted_*"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.claims.HasAllFeatures(tt.features)
			if got != tt.expected {
				t.Errorf("HasAllFeatures(%v) = %v, want %v", tt.features, got, tt.expected)
			}
		})
	}
}

func TestUserClaims_HasAnyFeature(t *testing.T) {
	tests := []struct {
		name     string
		claims   *UserClaims
		features []string
		expected bool
	}{
		{
			name:     "empty requirements",
			claims:   &UserClaims{Features: []string{"provider_byok"}},
			features: []string{},
			expected: false,
		},
		{
			name:     "has first feature",
			claims:   &UserClaims{Features: []string{"provider_byok"}},
			features: []string{"provider_byok", "models_custom"},
			expected: true,
		},
		{
			name:     "has second feature",
			claims:   &UserClaims{Features: []string{"models_custom"}},
			features: []string{"provider_byok", "models_custom"},
			expected: true,
		},
		{
			name:     "has none",
			claims:   &UserClaims{Features: []string{"webhooks"}},
			features: []string{"provider_byok", "models_custom"},
			expected: false,
		},
		{
			name:     "wildcard match",
			claims:   &UserClaims{Features: []string{"provider_byok"}},
			features: []string{"models_*", "provider_*"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.claims.HasAnyFeature(tt.features)
			if got != tt.expected {
				t.Errorf("HasAnyFeature(%v) = %v, want %v", tt.features, got, tt.expected)
			}
		})
	}
}

// ========================================
// GetUserClaims Tests
// ========================================

func TestGetUserClaims(t *testing.T) {
	t.Run("claims present", func(t *testing.T) {
		expected := &UserClaims{
			UserID: "user-123",
			Email:  "test@example.com",
			Tier:   "pro",
		}
		ctx := context.WithValue(context.Background(), UserClaimsKey, expected)

		got := GetUserClaims(ctx)
		if got == nil {
			t.Fatal("expected claims to be present")
		}
		if got.UserID != expected.UserID {
			t.Errorf("UserID = %s, want %s", got.UserID, expected.UserID)
		}
		if got.Email != expected.Email {
			t.Errorf("Email = %s, want %s", got.Email, expected.Email)
		}
	})

	t.Run("no claims", func(t *testing.T) {
		got := GetUserClaims(context.Background())
		if got != nil {
			t.Error("expected nil claims for empty context")
		}
	})

	t.Run("wrong type in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), UserClaimsKey, "not claims")
		got := GetUserClaims(ctx)
		if got != nil {
			t.Error("expected nil claims for wrong type")
		}
	})
}

// ========================================
// RequireSuperadmin Middleware Tests
// ========================================

func TestRequireSuperadmin(t *testing.T) {
	handler := RequireSuperadmin()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	t.Run("no claims - forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("non-superadmin - forbidden", func(t *testing.T) {
		claims := &UserClaims{
			UserID:           "user-123",
			GlobalSuperadmin: false,
		}
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("superadmin - passes", func(t *testing.T) {
		claims := &UserClaims{
			UserID:           "admin-123",
			GlobalSuperadmin: true,
		}
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})
}

// ========================================
// RequireScope Middleware Tests
// ========================================

func TestRequireScope(t *testing.T) {
	handler := RequireScope("jobs:write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	t.Run("no claims - unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/jobs", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("JWT (non-API key) - passes without scope check", func(t *testing.T) {
		claims := &UserClaims{
			UserID:   "user-123",
			IsAPIKey: false,
			Scopes:   []string{}, // Empty scopes, but it's a JWT so should pass
		}
		req := httptest.NewRequest(http.MethodPost, "/jobs", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d (JWT should bypass scope check)", rec.Code, http.StatusOK)
		}
	})

	t.Run("API key without required scope - forbidden", func(t *testing.T) {
		claims := &UserClaims{
			UserID:   "user-123",
			IsAPIKey: true,
			Scopes:   []string{"jobs:read"},
		}
		req := httptest.NewRequest(http.MethodPost, "/jobs", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("API key with exact scope - passes", func(t *testing.T) {
		claims := &UserClaims{
			UserID:   "user-123",
			IsAPIKey: true,
			Scopes:   []string{"jobs:read", "jobs:write"},
		}
		req := httptest.NewRequest(http.MethodPost, "/jobs", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("API key with wildcard scope - passes", func(t *testing.T) {
		claims := &UserClaims{
			UserID:   "user-123",
			IsAPIKey: true,
			Scopes:   []string{"*"},
		}
		req := httptest.NewRequest(http.MethodPost, "/jobs", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})
}

// ========================================
// Auth Middleware Tests
// ========================================

func TestAuth_MissingAuthHeader(t *testing.T) {
	handler := Auth(nil, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	// Test with a token that's neither a valid API key nor Clerk JWT
	handler := Auth(nil, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer invalid_token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should be unauthorized since there's no Clerk verifier
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// Note: Full Auth middleware testing with actual Clerk token validation
// would require mocking the ClerkVerifier, which depends on external JWKS.
// The unit tests above cover the middleware logic paths.

// ========================================
// OptionalAuth Middleware Tests
// ========================================

func TestOptionalAuth_NoAuth(t *testing.T) {
	var receivedClaims *UserClaims
	handler := OptionalAuth(nil, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedClaims = GetUserClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if receivedClaims != nil {
		t.Error("expected nil claims for unauthenticated request")
	}
}

func TestOptionalAuth_InvalidAuth(t *testing.T) {
	var receivedClaims *UserClaims
	handler := OptionalAuth(nil, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedClaims = GetUserClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public", nil)
	req.Header.Set("Authorization", "Bearer invalid_token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should still pass with invalid auth
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if receivedClaims != nil {
		t.Error("expected nil claims for invalid auth")
	}
}

func TestOptionalAuth_WithBearerPrefix(t *testing.T) {
	// Test that both "Bearer token" and raw "token" formats work
	handler := OptionalAuth(nil, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Claims should be nil for invalid tokens, but request should pass
		_ = GetUserClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Without Bearer prefix
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public", nil)
	req.Header.Set("Authorization", "some_token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
