package mw

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIVersion(t *testing.T) {
	// Create a simple handler that the middleware wraps
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with APIVersion middleware
	wrapped := APIVersion()(handler)

	// Make a request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Check that X-API-Version header is set
	version := rec.Header().Get("X-API-Version")
	if version == "" {
		t.Error("expected X-API-Version header to be set")
	}

	// Version should not be empty and should be a valid semver-ish format
	// In dev mode it will be "0.0.0-dev", in production it will be like "1.0.0"
	if version == "" {
		t.Error("X-API-Version header should not be empty")
	}

	t.Logf("X-API-Version: %s", version)
}

func TestAPIVersionOnAllResponses(t *testing.T) {
	// Test that the header is added regardless of response status
	testCases := []struct {
		name   string
		status int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"400 Bad Request", http.StatusBadRequest},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			})

			wrapped := APIVersion()(handler)
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)

			if rec.Header().Get("X-API-Version") == "" {
				t.Errorf("expected X-API-Version header for status %d", tc.status)
			}
		})
	}
}

func TestAPIVersionOnAllMethods(t *testing.T) {
	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodOptions,
		http.MethodHead,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := APIVersion()(handler)

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/test", nil)
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)

			if rec.Header().Get("X-API-Version") == "" {
				t.Errorf("expected X-API-Version header for method %s", method)
			}
		})
	}
}
