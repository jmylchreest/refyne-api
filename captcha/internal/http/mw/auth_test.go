package mw

import (
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

func TestValidateSignedHeaders(t *testing.T) {
	secret := "test-secret-key"

	tests := []struct {
		name       string
		setupReq   func(r *http.Request, ts int64)
		wantClaims *UserClaims
		wantErr    bool
	}{
		{
			name: "valid signature",
			setupReq: func(r *http.Request, ts int64) {
				timestamp := strconv.FormatInt(ts, 10)
				userID := "user_123"
				tier := "pro"
				features := "captcha,api_access"

				message := timestamp + ":" + userID + ":" + tier + ":" + features
				mac := hmac.New(sha256.New, []byte(secret))
				mac.Write([]byte(message))
				signature := hex.EncodeToString(mac.Sum(nil))

				r.Header.Set("X-Refyne-Signature", signature)
				r.Header.Set("X-Refyne-Timestamp", timestamp)
				r.Header.Set("X-Refyne-User-ID", userID)
				r.Header.Set("X-Refyne-Tier", tier)
				r.Header.Set("X-Refyne-Features", features)
			},
			wantClaims: &UserClaims{
				UserID:   "user_123",
				Tier:     "pro",
				Features: []string{"captcha", "api_access"},
			},
			wantErr: false,
		},
		{
			name: "missing headers returns nil",
			setupReq: func(r *http.Request, ts int64) {
				// No headers set
			},
			wantClaims: nil,
			wantErr:    false,
		},
		{
			name: "expired timestamp",
			setupReq: func(r *http.Request, ts int64) {
				timestamp := strconv.FormatInt(ts-400, 10) // 400 seconds ago (>5 min)
				userID := "user_123"
				tier := "pro"
				features := "captcha"

				message := timestamp + ":" + userID + ":" + tier + ":" + features
				mac := hmac.New(sha256.New, []byte(secret))
				mac.Write([]byte(message))
				signature := hex.EncodeToString(mac.Sum(nil))

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
			setupReq: func(r *http.Request, ts int64) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1", nil)
			tt.setupReq(req, time.Now().Unix())

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
		})
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
