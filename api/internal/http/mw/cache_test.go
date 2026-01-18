package mw

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/constants"
)

// ========================================
// CachePolicy Tests
// ========================================

func TestCachePolicy_Fields(t *testing.T) {
	policy := CachePolicy{
		Pattern:      "/api/v1/health",
		CacheControl: "public, max-age=30",
	}

	if policy.Pattern != "/api/v1/health" {
		t.Errorf("Pattern = %q, want %q", policy.Pattern, "/api/v1/health")
	}
	if policy.CacheControl != "public, max-age=30" {
		t.Errorf("CacheControl = %q, want %q", policy.CacheControl, "public, max-age=30")
	}
}

// ========================================
// CacheConfig Tests
// ========================================

func TestCacheConfig_Fields(t *testing.T) {
	cfg := CacheConfig{
		Policies: []CachePolicy{
			{Pattern: "/test", CacheControl: "no-store"},
		},
		DefaultPolicy: "private, no-cache",
	}

	if len(cfg.Policies) != 1 {
		t.Errorf("Policies length = %d, want 1", len(cfg.Policies))
	}
	if cfg.DefaultPolicy != "private, no-cache" {
		t.Errorf("DefaultPolicy = %q, want %q", cfg.DefaultPolicy, "private, no-cache")
	}
}

// ========================================
// DefaultCacheConfig Tests
// ========================================

func TestDefaultCacheConfig(t *testing.T) {
	cfg := DefaultCacheConfig()

	// Check default policy
	if cfg.DefaultPolicy != "private, no-cache" {
		t.Errorf("DefaultPolicy = %q, want %q", cfg.DefaultPolicy, "private, no-cache")
	}

	// Check that policies exist
	if len(cfg.Policies) == 0 {
		t.Error("Policies should not be empty")
	}

	// Check for specific expected policies (using constants)
	shortSecs := int(constants.CacheMaxAgeShort.Seconds())
	mediumSecs := int(constants.CacheMaxAgeMedium.Seconds())
	longSecs := int(constants.CacheMaxAgeLong.Seconds())

	expectedPolicies := map[string]string{
		"/api/v1/health":        fmt.Sprintf("public, max-age=%d", shortSecs),
		"/api/v1/pricing/tiers": fmt.Sprintf("public, max-age=%d, stale-while-revalidate=60", mediumSecs),
		"/healthz":              "no-store",
		"/readyz":               "no-store",
		"/api/v1/llm/providers": fmt.Sprintf("private, max-age=%d", longSecs),
		"/api/v1/usage":         "private, no-cache",
		"/api/v1/jobs":          "private, no-cache",
	}

	for pattern, expectedCC := range expectedPolicies {
		found := false
		for _, policy := range cfg.Policies {
			if policy.Pattern == pattern {
				found = true
				if policy.CacheControl != expectedCC {
					t.Errorf("Policy %q: CacheControl = %q, want %q",
						pattern, policy.CacheControl, expectedCC)
				}
				break
			}
		}
		if !found {
			t.Errorf("Expected policy for pattern %q not found", pattern)
		}
	}
}

// ========================================
// matchesPattern Tests
// ========================================

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pattern  string
		expected bool
	}{
		// Exact match
		{"exact match", "/healthz", "/healthz", true},
		{"exact match with different path", "/readyz", "/healthz", false},

		// Prefix match
		{"prefix match", "/api/v1/jobs/123", "/api/v1/jobs", true},
		{"prefix match with trailing slash", "/api/v1/schemas/abc", "/api/v1/schemas", true},
		{"no prefix match", "/api/v2/jobs", "/api/v1/jobs", false},

		// Substring match (for patterns like "/stream")
		{"substring match", "/api/v1/jobs/123/stream", "/stream", true},
		{"substring match at end", "/some/path/stream", "/stream", true},
		{"no substring match", "/api/v1/dreams", "/stream", false},

		// Models endpoint
		{"models endpoint", "/api/v1/llm/models/openai", "/api/v1/llm/models/", true},
		{"models endpoint base", "/api/v1/llm/models/", "/api/v1/llm/models/", true},

		// Edge cases
		{"empty path", "", "/api", false},
		{"empty pattern", "/api/v1/test", "", true}, // Empty pattern matches everything via HasPrefix
		{"root path", "/", "/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesPattern(tt.path, tt.pattern)
			if got != tt.expected {
				t.Errorf("matchesPattern(%q, %q) = %v, want %v",
					tt.path, tt.pattern, got, tt.expected)
			}
		})
	}
}

// ========================================
// Cache Middleware Tests
// ========================================

func TestCache_NonGetRequest(t *testing.T) {
	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	cfg := DefaultCacheConfig()
	middleware := Cache(cfg)

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(method, "/api/v1/health", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			cc := rr.Header().Get("Cache-Control")
			if cc != "no-store" {
				t.Errorf("%s request: Cache-Control = %q, want %q", method, cc, "no-store")
			}
		})
	}
}

func TestCache_GetRequest_MatchingPolicy(t *testing.T) {
	shortSecs := int(constants.CacheMaxAgeShort.Seconds())
	mediumSecs := int(constants.CacheMaxAgeMedium.Seconds())
	longSecs := int(constants.CacheMaxAgeLong.Seconds())

	tests := []struct {
		name             string
		path             string
		expectedCacheCtl string
	}{
		{"health endpoint", "/api/v1/health", fmt.Sprintf("public, max-age=%d", shortSecs)},
		{"pricing tiers", "/api/v1/pricing/tiers", fmt.Sprintf("public, max-age=%d, stale-while-revalidate=60", mediumSecs)},
		{"healthz probe", "/healthz", "no-store"},
		{"readyz probe", "/readyz", "no-store"},
		{"llm providers", "/api/v1/llm/providers", fmt.Sprintf("private, max-age=%d", longSecs)},
		{"llm models", "/api/v1/llm/models/openai", fmt.Sprintf("private, max-age=%d", longSecs)},
		{"schemas", "/api/v1/schemas", fmt.Sprintf("private, max-age=%d", mediumSecs)},
		{"usage", "/api/v1/usage", "private, no-cache"},
		{"jobs list", "/api/v1/jobs", "private, no-cache"},
		{"job stream", "/api/v1/jobs/123/stream", "private, no-cache"}, // matches /api/v1/jobs first
		{"standalone stream", "/events/stream", "no-cache"},            // matches /stream pattern
	}

	cfg := DefaultCacheConfig()
	middleware := Cache(cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			cc := rr.Header().Get("Cache-Control")
			if cc != tt.expectedCacheCtl {
				t.Errorf("path %q: Cache-Control = %q, want %q",
					tt.path, cc, tt.expectedCacheCtl)
			}
		})
	}
}

func TestCache_GetRequest_DefaultPolicy(t *testing.T) {
	cfg := CacheConfig{
		Policies:      []CachePolicy{},
		DefaultPolicy: "private, max-age=60",
	}
	middleware := Cache(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unmatched/path", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	cc := rr.Header().Get("Cache-Control")
	if cc != "private, max-age=60" {
		t.Errorf("Cache-Control = %q, want %q", cc, "private, max-age=60")
	}
}

func TestCache_GetRequest_NoDefaultPolicy(t *testing.T) {
	cfg := CacheConfig{
		Policies:      []CachePolicy{},
		DefaultPolicy: "",
	}
	middleware := Cache(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unmatched/path", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	cc := rr.Header().Get("Cache-Control")
	if cc != "" {
		t.Errorf("Cache-Control = %q, want empty (no header set)", cc)
	}
}

func TestCache_HeadRequest(t *testing.T) {
	cfg := CacheConfig{
		Policies: []CachePolicy{
			{Pattern: "/api/v1/test", CacheControl: "public, max-age=120"},
		},
		DefaultPolicy: "private, no-cache",
	}
	middleware := Cache(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodHead, "/api/v1/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	cc := rr.Header().Get("Cache-Control")
	if cc != "public, max-age=120" {
		t.Errorf("HEAD request: Cache-Control = %q, want %q", cc, "public, max-age=120")
	}
}

func TestCache_FirstMatchWins(t *testing.T) {
	cfg := CacheConfig{
		Policies: []CachePolicy{
			{Pattern: "/api", CacheControl: "first-policy"},
			{Pattern: "/api/v1", CacheControl: "second-policy"},
			{Pattern: "/api/v1/specific", CacheControl: "third-policy"},
		},
	}
	middleware := Cache(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Should match first policy since it's a prefix of all paths
	req := httptest.NewRequest(http.MethodGet, "/api/v1/specific/endpoint", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	cc := rr.Header().Get("Cache-Control")
	if cc != "first-policy" {
		t.Errorf("Cache-Control = %q, want %q (first match)", cc, "first-policy")
	}
}

func TestCache_HandlerCalledWithRequest(t *testing.T) {
	cfg := DefaultCacheConfig()
	middleware := Cache(cfg)

	handlerCalled := false
	var receivedPath string

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("handler was not called")
	}
	if receivedPath != "/api/v1/test" {
		t.Errorf("handler received path %q, want %q", receivedPath, "/api/v1/test")
	}
}

func TestCache_ResponseStatusPreserved(t *testing.T) {
	cfg := DefaultCacheConfig()
	middleware := Cache(cfg)

	statuses := []int{http.StatusOK, http.StatusNotFound, http.StatusInternalServerError}

	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != status {
				t.Errorf("response status = %d, want %d", rr.Code, status)
			}
		})
	}
}

func TestCache_ResponseBodyPreserved(t *testing.T) {
	cfg := DefaultCacheConfig()
	middleware := Cache(cfg)

	expectedBody := `{"message": "hello"}`

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedBody))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Body.String() != expectedBody {
		t.Errorf("response body = %q, want %q", rr.Body.String(), expectedBody)
	}
}

// ========================================
// Integration-style Tests
// ========================================

func TestCache_DefaultConfig_PublicEndpoints(t *testing.T) {
	cfg := DefaultCacheConfig()
	middleware := Cache(cfg)

	// Public endpoints should have public caching
	publicPaths := []string{"/api/v1/health", "/api/v1/pricing/tiers"}

	for _, path := range publicPaths {
		t.Run(path, func(t *testing.T) {
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			cc := rr.Header().Get("Cache-Control")
			if cc == "" {
				t.Errorf("path %q: Cache-Control should be set", path)
			}
			if len(cc) < 6 || cc[:6] != "public" {
				t.Errorf("path %q: Cache-Control = %q, should start with 'public'", path, cc)
			}
		})
	}
}

func TestCache_DefaultConfig_K8sProbes(t *testing.T) {
	cfg := DefaultCacheConfig()
	middleware := Cache(cfg)

	// K8s probes should never be cached
	probePaths := []string{"/healthz", "/readyz"}

	for _, path := range probePaths {
		t.Run(path, func(t *testing.T) {
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			cc := rr.Header().Get("Cache-Control")
			if cc != "no-store" {
				t.Errorf("path %q: Cache-Control = %q, want %q", path, cc, "no-store")
			}
		})
	}
}

func TestCache_DefaultConfig_PrivateEndpoints(t *testing.T) {
	cfg := DefaultCacheConfig()
	middleware := Cache(cfg)

	// Protected endpoints should have private caching
	privatePaths := []string{
		"/api/v1/llm/providers",
		"/api/v1/llm/models/anthropic",
		"/api/v1/schemas",
		"/api/v1/usage",
		"/api/v1/jobs",
	}

	for _, path := range privatePaths {
		t.Run(path, func(t *testing.T) {
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			cc := rr.Header().Get("Cache-Control")
			if cc == "" {
				t.Errorf("path %q: Cache-Control should be set", path)
			}
			if len(cc) < 7 || cc[:7] != "private" {
				t.Errorf("path %q: Cache-Control = %q, should start with 'private'", path, cc)
			}
		})
	}
}
