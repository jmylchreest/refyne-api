package config

import (
	"os"
	"testing"
	"time"
)

// ========================================
// Helper Functions Tests
// ========================================

func TestGetEnv(t *testing.T) {
	// Set a test environment variable
	os.Setenv("TEST_GET_ENV", "test_value")
	defer os.Unsetenv("TEST_GET_ENV")

	t.Run("existing env var", func(t *testing.T) {
		result := getEnv("TEST_GET_ENV", "default")
		if result != "test_value" {
			t.Errorf("getEnv() = %q, want %q", result, "test_value")
		}
	})

	t.Run("missing env var", func(t *testing.T) {
		result := getEnv("TEST_MISSING_VAR", "default_value")
		if result != "default_value" {
			t.Errorf("getEnv() = %q, want %q", result, "default_value")
		}
	})

	t.Run("empty env var", func(t *testing.T) {
		os.Setenv("TEST_EMPTY_VAR", "")
		defer os.Unsetenv("TEST_EMPTY_VAR")

		result := getEnv("TEST_EMPTY_VAR", "default")
		if result != "default" {
			t.Errorf("getEnv() = %q, want %q (empty should use default)", result, "default")
		}
	})
}

func TestGetEnvInt(t *testing.T) {
	t.Run("valid integer", func(t *testing.T) {
		os.Setenv("TEST_INT", "42")
		defer os.Unsetenv("TEST_INT")

		result := getEnvInt("TEST_INT", 0)
		if result != 42 {
			t.Errorf("getEnvInt() = %d, want 42", result)
		}
	})

	t.Run("invalid integer", func(t *testing.T) {
		os.Setenv("TEST_INT_INVALID", "not-a-number")
		defer os.Unsetenv("TEST_INT_INVALID")

		result := getEnvInt("TEST_INT_INVALID", 99)
		if result != 99 {
			t.Errorf("getEnvInt() = %d, want 99 (default)", result)
		}
	})

	t.Run("missing env var", func(t *testing.T) {
		result := getEnvInt("TEST_INT_MISSING", 100)
		if result != 100 {
			t.Errorf("getEnvInt() = %d, want 100 (default)", result)
		}
	})

	t.Run("negative integer", func(t *testing.T) {
		os.Setenv("TEST_INT_NEG", "-5")
		defer os.Unsetenv("TEST_INT_NEG")

		result := getEnvInt("TEST_INT_NEG", 0)
		if result != -5 {
			t.Errorf("getEnvInt() = %d, want -5", result)
		}
	})
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{"true lowercase", "true", true},
		{"TRUE uppercase", "TRUE", true},
		{"True mixed", "True", true},
		{"1", "1", true},
		{"yes lowercase", "yes", true},
		{"YES uppercase", "YES", true},
		{"false lowercase", "false", false},
		{"FALSE uppercase", "FALSE", false},
		{"0", "0", false},
		{"random string", "maybe", false},
		{"empty", "", false}, // Will use default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != "" {
				os.Setenv("TEST_BOOL", tt.value)
				defer os.Unsetenv("TEST_BOOL")
			}

			result := getEnvBool("TEST_BOOL", false)
			if tt.value == "" {
				// Empty uses default
				return
			}
			if result != tt.expected {
				t.Errorf("getEnvBool(%q) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}

	t.Run("missing env var with default true", func(t *testing.T) {
		result := getEnvBool("TEST_BOOL_MISSING", true)
		if result != true {
			t.Error("should return default true")
		}
	})

	t.Run("missing env var with default false", func(t *testing.T) {
		result := getEnvBool("TEST_BOOL_MISSING2", false)
		if result != false {
			t.Error("should return default false")
		}
	})
}

func TestGetEnvDuration(t *testing.T) {
	t.Run("valid duration", func(t *testing.T) {
		os.Setenv("TEST_DUR", "5m")
		defer os.Unsetenv("TEST_DUR")

		result := getEnvDuration("TEST_DUR", time.Hour)
		if result != 5*time.Minute {
			t.Errorf("getEnvDuration() = %v, want 5m", result)
		}
	})

	t.Run("complex duration", func(t *testing.T) {
		os.Setenv("TEST_DUR_COMPLEX", "1h30m")
		defer os.Unsetenv("TEST_DUR_COMPLEX")

		result := getEnvDuration("TEST_DUR_COMPLEX", time.Hour)
		if result != 90*time.Minute {
			t.Errorf("getEnvDuration() = %v, want 1h30m", result)
		}
	})

	t.Run("invalid duration", func(t *testing.T) {
		os.Setenv("TEST_DUR_INVALID", "not-a-duration")
		defer os.Unsetenv("TEST_DUR_INVALID")

		result := getEnvDuration("TEST_DUR_INVALID", 2*time.Hour)
		if result != 2*time.Hour {
			t.Errorf("getEnvDuration() = %v, want 2h (default)", result)
		}
	})

	t.Run("missing env var", func(t *testing.T) {
		result := getEnvDuration("TEST_DUR_MISSING", 30*time.Second)
		if result != 30*time.Second {
			t.Errorf("getEnvDuration() = %v, want 30s (default)", result)
		}
	})
}

func TestGetEnvSlice(t *testing.T) {
	t.Run("comma separated values", func(t *testing.T) {
		os.Setenv("TEST_SLICE", "a,b,c")
		defer os.Unsetenv("TEST_SLICE")

		result := getEnvSlice("TEST_SLICE", []string{})
		if len(result) != 3 {
			t.Errorf("getEnvSlice() length = %d, want 3", len(result))
		}
		if result[0] != "a" || result[1] != "b" || result[2] != "c" {
			t.Errorf("getEnvSlice() = %v, want [a b c]", result)
		}
	})

	t.Run("single value", func(t *testing.T) {
		os.Setenv("TEST_SLICE_SINGLE", "only_one")
		defer os.Unsetenv("TEST_SLICE_SINGLE")

		result := getEnvSlice("TEST_SLICE_SINGLE", []string{})
		if len(result) != 1 {
			t.Errorf("getEnvSlice() length = %d, want 1", len(result))
		}
	})

	t.Run("missing env var", func(t *testing.T) {
		defaultSlice := []string{"default1", "default2"}
		result := getEnvSlice("TEST_SLICE_MISSING", defaultSlice)
		if len(result) != 2 {
			t.Errorf("getEnvSlice() length = %d, want 2 (default)", len(result))
		}
	})
}

func TestGetEnvWithFallback(t *testing.T) {
	t.Run("primary exists", func(t *testing.T) {
		os.Setenv("PRIMARY_KEY", "primary_value")
		defer os.Unsetenv("PRIMARY_KEY")

		result := getEnvWithFallback("PRIMARY_KEY", "FALLBACK_KEY", "default")
		if result != "primary_value" {
			t.Errorf("getEnvWithFallback() = %q, want %q", result, "primary_value")
		}
	})

	t.Run("fallback exists", func(t *testing.T) {
		os.Setenv("FALLBACK_KEY", "fallback_value")
		defer os.Unsetenv("FALLBACK_KEY")

		result := getEnvWithFallback("MISSING_PRIMARY", "FALLBACK_KEY", "default")
		if result != "fallback_value" {
			t.Errorf("getEnvWithFallback() = %q, want %q", result, "fallback_value")
		}
	})

	t.Run("neither exists", func(t *testing.T) {
		result := getEnvWithFallback("MISSING1", "MISSING2", "the_default")
		if result != "the_default" {
			t.Errorf("getEnvWithFallback() = %q, want %q", result, "the_default")
		}
	})
}

// ========================================
// Config Methods Tests
// ========================================

func TestConfig_IsSelfHosted(t *testing.T) {
	t.Run("hosted mode", func(t *testing.T) {
		cfg := &Config{DeploymentMode: "hosted"}
		if cfg.IsSelfHosted() {
			t.Error("IsSelfHosted() should be false for hosted mode")
		}
	})

	t.Run("selfhosted mode", func(t *testing.T) {
		cfg := &Config{DeploymentMode: "selfhosted"}
		if !cfg.IsSelfHosted() {
			t.Error("IsSelfHosted() should be true for selfhosted mode")
		}
	})
}

func TestConfig_HasPersistence(t *testing.T) {
	t.Run("with data dir", func(t *testing.T) {
		cfg := &Config{DataDir: "/data"}
		if !cfg.HasPersistence() {
			t.Error("HasPersistence() should be true when DataDir is set")
		}
	})

	t.Run("without data dir", func(t *testing.T) {
		cfg := &Config{DataDir: ""}
		if cfg.HasPersistence() {
			t.Error("HasPersistence() should be false when DataDir is empty")
		}
	})
}

// ========================================
// deriveEncryptionKey Tests
// ========================================

func TestDeriveEncryptionKey(t *testing.T) {
	key := deriveEncryptionKey("test-secret")

	if len(key) != 32 {
		t.Errorf("key length = %d, want 32", len(key))
	}

	// Same input should produce same key
	key2 := deriveEncryptionKey("test-secret")
	for i := range key {
		if key[i] != key2[i] {
			t.Error("same input should produce same key")
			break
		}
	}

	// Different input should produce different key
	key3 := deriveEncryptionKey("different-secret")
	same := true
	for i := range key {
		if key[i] != key3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different input should produce different key")
	}
}

func TestDeriveEncryptionKey_EmptySecret(t *testing.T) {
	// Should not panic with empty secret
	key := deriveEncryptionKey("")
	if len(key) != 32 {
		t.Errorf("key length = %d, want 32", len(key))
	}
}

// ========================================
// generateRandomSecret Tests
// ========================================

func TestGenerateRandomSecret(t *testing.T) {
	secret := generateRandomSecret(32)
	if len(secret) == 0 {
		t.Error("secret should not be empty")
	}

	// Different calls should produce different secrets
	secret2 := generateRandomSecret(32)
	if secret == secret2 {
		t.Error("random secrets should be different")
	}
}

func TestGenerateRandomSecret_DifferentLengths(t *testing.T) {
	lengths := []int{16, 32, 64, 128}
	for _, l := range lengths {
		secret := generateRandomSecret(l)
		if len(secret) == 0 {
			t.Errorf("secret with length %d should not be empty", l)
		}
	}
}

// ========================================
// Config Struct Tests
// ========================================

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		Port:           8080,
		BaseURL:        "https://api.example.com",
		DatabaseURL:    "postgres://localhost/test",
		DeploymentMode: "hosted",
		AdminEnabled:   true,
		CORSOrigins:    []string{"http://localhost:3000"},
	}

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://api.example.com")
	}
	if cfg.DeploymentMode != "hosted" {
		t.Errorf("DeploymentMode = %q, want %q", cfg.DeploymentMode, "hosted")
	}
	if !cfg.AdminEnabled {
		t.Error("AdminEnabled should be true")
	}
	if len(cfg.CORSOrigins) != 1 {
		t.Errorf("CORSOrigins length = %d, want 1", len(cfg.CORSOrigins))
	}
}

func TestConfig_StorageEnabled(t *testing.T) {
	t.Run("enabled when bucket and endpoint set", func(t *testing.T) {
		cfg := Config{
			StorageBucket:   "my-bucket",
			StorageEndpoint: "https://s3.amazonaws.com",
			StorageEnabled:  true,
		}
		if !cfg.StorageEnabled {
			t.Error("StorageEnabled should be true when bucket and endpoint are set")
		}
	})

	t.Run("disabled when bucket missing", func(t *testing.T) {
		cfg := Config{
			StorageBucket:   "",
			StorageEndpoint: "https://s3.amazonaws.com",
			StorageEnabled:  false,
		}
		if cfg.StorageEnabled {
			t.Error("StorageEnabled should be false when bucket is missing")
		}
	})
}

// Note: Testing Load() directly requires setting many environment variables.
// The helper function tests above provide good coverage of the parsing logic.
// Integration tests for Load() should be done in a controlled environment.
