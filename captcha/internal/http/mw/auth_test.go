package mw

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestUserClaims_HasFeature(t *testing.T) {
	tests := []struct {
		name     string
		features []string
		pattern  string
		want     bool
	}{
		{
			name:     "exact match",
			features: []string{"captcha", "api_access"},
			pattern:  "captcha",
			want:     true,
		},
		{
			name:     "no match",
			features: []string{"captcha", "api_access"},
			pattern:  "unknown",
			want:     false,
		},
		{
			name:     "wildcard match",
			features: []string{"captcha_basic", "api_access"},
			pattern:  "captcha_*",
			want:     true,
		},
		{
			name:     "wildcard no match",
			features: []string{"other_feature"},
			pattern:  "captcha_*",
			want:     false,
		},
		{
			name:     "empty features",
			features: []string{},
			pattern:  "captcha",
			want:     false,
		},
		{
			name:     "nil claims",
			features: nil,
			pattern:  "captcha",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var claims *UserClaims
			if tt.features != nil {
				claims = &UserClaims{Features: tt.features}
			} else if tt.name != "nil claims" {
				claims = &UserClaims{Features: tt.features}
			}

			got := claims.HasFeature(tt.pattern)
			if got != tt.want {
				t.Errorf("HasFeature(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestUserClaims_HasAllFeatures(t *testing.T) {
	claims := &UserClaims{Features: []string{"captcha", "api_access", "rate_limit"}}

	tests := []struct {
		name     string
		features []string
		want     bool
	}{
		{
			name:     "all present",
			features: []string{"captcha", "api_access"},
			want:     true,
		},
		{
			name:     "one missing",
			features: []string{"captcha", "unknown"},
			want:     false,
		},
		{
			name:     "empty list",
			features: []string{},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claims.HasAllFeatures(tt.features); got != tt.want {
				t.Errorf("HasAllFeatures(%v) = %v, want %v", tt.features, got, tt.want)
			}
		})
	}
}

func TestUserClaims_HasAnyFeature(t *testing.T) {
	claims := &UserClaims{Features: []string{"captcha", "api_access"}}

	tests := []struct {
		name     string
		features []string
		want     bool
	}{
		{
			name:     "one match",
			features: []string{"unknown", "captcha"},
			want:     true,
		},
		{
			name:     "no match",
			features: []string{"unknown", "other"},
			want:     false,
		},
		{
			name:     "empty list",
			features: []string{},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claims.HasAnyFeature(tt.features); got != tt.want {
				t.Errorf("HasAnyFeature(%v) = %v, want %v", tt.features, got, tt.want)
			}
		})
	}
}

// signRequest creates a valid HMAC signature for testing.
// Format: HMAC-SHA256(timestamp|userID|tier|features|jobID|bodyHash)
func signRequest(secret, timestamp, userID, tier, features, jobID string, body []byte) string {
	bodyHash := sha256.Sum256(body)
	message := timestamp + "|" + userID + "|" + tier + "|" + features + "|" + jobID + "|" + hex.EncodeToString(bodyHash[:])
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestValidateSignedHeaders(t *testing.T) {
	secret := "test-secret-key"
	testBody := []byte(`{"cmd":"request.get","url":"https://example.com"}`)

	tests := []struct {
		name       string
		body       []byte
		setupReq   func(r *http.Request, ts int64, body []byte)
		wantClaims *UserClaims
		wantErr    bool
	}{
		{
			name: "valid signature with body",
			body: testBody,
			setupReq: func(r *http.Request, ts int64, body []byte) {
				timestamp := strconv.FormatInt(ts, 10)
				userID := "user_123"
				tier := "pro"
				features := "captcha,api_access"
				jobID := "job_456"

				signature := signRequest(secret, timestamp, userID, tier, features, jobID, body)

				r.Header.Set("X-Refyne-Signature", signature)
				r.Header.Set("X-Refyne-Timestamp", timestamp)
				r.Header.Set("X-Refyne-User-ID", userID)
				r.Header.Set("X-Refyne-Tier", tier)
				r.Header.Set("X-Refyne-Features", features)
				r.Header.Set("X-Refyne-Job-ID", jobID)
			},
			wantClaims: &UserClaims{
				UserID:   "user_123",
				Tier:     "pro",
				Features: []string{"captcha", "api_access"},
				JobID:    "job_456",
			},
			wantErr: false,
		},
		{
			name: "valid signature without job ID",
			body: testBody,
			setupReq: func(r *http.Request, ts int64, body []byte) {
				timestamp := strconv.FormatInt(ts, 10)
				userID := "user_123"
				tier := "pro"
				features := "captcha"
				jobID := "" // Empty job ID

				signature := signRequest(secret, timestamp, userID, tier, features, jobID, body)

				r.Header.Set("X-Refyne-Signature", signature)
				r.Header.Set("X-Refyne-Timestamp", timestamp)
				r.Header.Set("X-Refyne-User-ID", userID)
				r.Header.Set("X-Refyne-Tier", tier)
				r.Header.Set("X-Refyne-Features", features)
			},
			wantClaims: &UserClaims{
				UserID:   "user_123",
				Tier:     "pro",
				Features: []string{"captcha"},
			},
			wantErr: false,
		},
		{
			name: "missing headers returns nil",
			body: nil,
			setupReq: func(r *http.Request, ts int64, body []byte) {
				// No headers set
			},
			wantClaims: nil,
			wantErr:    false,
		},
		{
			name: "expired timestamp",
			body: testBody,
			setupReq: func(r *http.Request, ts int64, body []byte) {
				timestamp := strconv.FormatInt(ts-400, 10) // 400 seconds ago (>5 min)
				userID := "user_123"
				tier := "pro"
				features := "captcha"
				jobID := ""

				signature := signRequest(secret, timestamp, userID, tier, features, jobID, body)

				r.Header.Set("X-Refyne-Signature", signature)
				r.Header.Set("X-Refyne-Timestamp", timestamp)
				r.Header.Set("X-Refyne-User-ID", userID)
				r.Header.Set("X-Refyne-Tier", tier)
				r.Header.Set("X-Refyne-Features", features)
			},
			wantClaims: nil,
			wantErr:    true,
		},
		{
			name: "invalid signature",
			body: testBody,
			setupReq: func(r *http.Request, ts int64, body []byte) {
				timestamp := strconv.FormatInt(ts, 10)
				r.Header.Set("X-Refyne-Signature", "invalid-signature")
				r.Header.Set("X-Refyne-Timestamp", timestamp)
				r.Header.Set("X-Refyne-User-ID", "user_123")
				r.Header.Set("X-Refyne-Tier", "pro")
				r.Header.Set("X-Refyne-Features", "captcha")
			},
			wantClaims: nil,
			wantErr:    true,
		},
		{
			name: "tampered body fails",
			body: testBody,
			setupReq: func(r *http.Request, ts int64, body []byte) {
				timestamp := strconv.FormatInt(ts, 10)
				userID := "user_123"
				tier := "pro"
				features := "captcha"
				jobID := ""

				// Sign with original body
				signature := signRequest(secret, timestamp, userID, tier, features, jobID, body)

				r.Header.Set("X-Refyne-Signature", signature)
				r.Header.Set("X-Refyne-Timestamp", timestamp)
				r.Header.Set("X-Refyne-User-ID", userID)
				r.Header.Set("X-Refyne-Tier", tier)
				r.Header.Set("X-Refyne-Features", features)

				// But the actual request body is different (simulated by modifying request)
				// We'll handle this in the test by providing different body
			},
			wantClaims: nil,
			wantErr:    true,
		},
		{
			name: "tampered job ID fails",
			body: testBody,
			setupReq: func(r *http.Request, ts int64, body []byte) {
				timestamp := strconv.FormatInt(ts, 10)
				userID := "user_123"
				tier := "pro"
				features := "captcha"
				originalJobID := "job_123"

				// Sign with original job ID
				signature := signRequest(secret, timestamp, userID, tier, features, originalJobID, body)

				r.Header.Set("X-Refyne-Signature", signature)
				r.Header.Set("X-Refyne-Timestamp", timestamp)
				r.Header.Set("X-Refyne-User-ID", userID)
				r.Header.Set("X-Refyne-Tier", tier)
				r.Header.Set("X-Refyne-Features", features)
				r.Header.Set("X-Refyne-Job-ID", "tampered_job_id") // Different job ID
			},
			wantClaims: nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.name == "tampered body fails" {
				// For tampered body test, use different body in request
				body = []byte(`{"cmd":"request.get","url":"https://evil.com"}`)
			} else {
				body = tt.body
			}

			var req *http.Request
			if body != nil {
				req = httptest.NewRequest(http.MethodPost, "/v1", bytes.NewReader(body))
			} else {
				req = httptest.NewRequest(http.MethodPost, "/v1", nil)
			}

			tt.setupReq(req, time.Now().Unix(), tt.body)

			claims, err := validateSignedHeaders(req, secret)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateSignedHeaders() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantClaims == nil {
				if claims != nil {
					t.Errorf("validateSignedHeaders() claims = %+v, want nil", claims)
				}
				return
			}

			if claims == nil {
				t.Error("validateSignedHeaders() claims = nil, want non-nil")
				return
			}

			if claims.UserID != tt.wantClaims.UserID {
				t.Errorf("claims.UserID = %q, want %q", claims.UserID, tt.wantClaims.UserID)
			}
			if claims.Tier != tt.wantClaims.Tier {
				t.Errorf("claims.Tier = %q, want %q", claims.Tier, tt.wantClaims.Tier)
			}
			if len(claims.Features) != len(tt.wantClaims.Features) {
				t.Errorf("claims.Features = %v, want %v", claims.Features, tt.wantClaims.Features)
			}
			if claims.JobID != tt.wantClaims.JobID {
				t.Errorf("claims.JobID = %q, want %q", claims.JobID, tt.wantClaims.JobID)
			}
		})
	}
}

func TestValidateSignedHeaders_BodyRestored(t *testing.T) {
	secret := "test-secret-key"
	testBody := []byte(`{"cmd":"request.get","url":"https://example.com"}`)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	userID := "user_123"
	tier := "pro"
	features := "captcha"
	jobID := ""

	signature := signRequest(secret, timestamp, userID, tier, features, jobID, testBody)

	req := httptest.NewRequest(http.MethodPost, "/v1", bytes.NewReader(testBody))
	req.Header.Set("X-Refyne-Signature", signature)
	req.Header.Set("X-Refyne-Timestamp", timestamp)
	req.Header.Set("X-Refyne-User-ID", userID)
	req.Header.Set("X-Refyne-Tier", tier)
	req.Header.Set("X-Refyne-Features", features)

	// First call consumes body for signature verification
	_, err := validateSignedHeaders(req, secret)
	if err != nil {
		t.Fatalf("validateSignedHeaders() unexpected error: %v", err)
	}

	// Body should still be readable after validation
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(req.Body)
	if err != nil {
		t.Fatalf("failed to read body after validation: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), testBody) {
		t.Errorf("body not restored correctly: got %q, want %q", buf.String(), string(testBody))
	}
}

func TestGetUserClaims(t *testing.T) {
	t.Run("claims present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		expected := &UserClaims{UserID: "user_123"}

		ctx := req.Context()
		ctx = setUserClaims(ctx, expected)
		req = req.WithContext(ctx)

		claims := GetUserClaims(req.Context())
		if claims == nil || claims.UserID != expected.UserID {
			t.Errorf("GetUserClaims() = %+v, want %+v", claims, expected)
		}
	})

	t.Run("claims absent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		claims := GetUserClaims(req.Context())
		if claims != nil {
			t.Errorf("GetUserClaims() = %+v, want nil", claims)
		}
	})
}

// Helper to set claims in context for testing
func setUserClaims(ctx context.Context, claims *UserClaims) context.Context {
	return context.WithValue(ctx, UserClaimsKey, claims)
}

func TestRequireFeature(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("feature present", func(t *testing.T) {
		middleware := RequireFeature("captcha")
		wrappedHandler := middleware(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := setUserClaims(req.Context(), &UserClaims{
			UserID:   "user_123",
			Features: []string{"captcha", "api_access"},
		})
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
		}
	})

	t.Run("feature missing", func(t *testing.T) {
		middleware := RequireFeature("captcha")
		wrappedHandler := middleware(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := setUserClaims(req.Context(), &UserClaims{
			UserID:   "user_123",
			Features: []string{"other_feature"},
			Tier:     "free",
		})
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusForbidden)
		}
	})

	t.Run("no claims", func(t *testing.T) {
		middleware := RequireFeature("captcha")
		wrappedHandler := middleware(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusUnauthorized)
		}
	})
}
