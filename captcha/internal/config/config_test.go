package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Clean up env vars after test
	origEnv := make(map[string]string)
	envVars := []string{
		"PORT", "LOG_LEVEL", "BROWSER_POOL_SIZE", "BROWSER_IDLE_TIMEOUT",
		"BROWSER_MAX_REQUESTS", "BROWSER_MAX_AGE", "CHROME_PATH",
		"CHALLENGE_TIMEOUT", "CHALLENGE_WAIT_TIME", "TWOCAPTCHA_API_KEY",
		"CAPSOLVER_API_KEY", "REFYNE_API_SECRET", "CLERK_ISSUER",
		"REQUIRED_FEATURE", "ALLOW_UNAUTHENTICATED", "PROXY_ENABLED",
		"PROXY_URL", "SESSION_MAX_IDLE",
	}

	for _, v := range envVars {
		origEnv[v] = os.Getenv(v)
	}
	defer func() {
		for k, v := range origEnv {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	t.Run("defaults", func(t *testing.T) {
		// Clear all env vars
		for _, v := range envVars {
			os.Unsetenv(v)
		}

		cfg := Load()

		if cfg.Port != 8191 {
			t.Errorf("Port = %d, want 8191", cfg.Port)
		}
		if cfg.LogLevel != "info" {
			t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
		}
		if cfg.BrowserPoolSize != 5 {
			t.Errorf("BrowserPoolSize = %d, want 5", cfg.BrowserPoolSize)
		}
		if cfg.BrowserIdleTimeout != 5*time.Minute {
			t.Errorf("BrowserIdleTimeout = %v, want 5m", cfg.BrowserIdleTimeout)
		}
		if cfg.BrowserMaxRequests != 100 {
			t.Errorf("BrowserMaxRequests = %d, want 100", cfg.BrowserMaxRequests)
		}
		if cfg.BrowserMaxAge != 30*time.Minute {
			t.Errorf("BrowserMaxAge = %v, want 30m", cfg.BrowserMaxAge)
		}
		if cfg.ChallengeTimeout != 60*time.Second {
			t.Errorf("ChallengeTimeout = %v, want 60s", cfg.ChallengeTimeout)
		}
		if cfg.ChallengeWaitTime != 30*time.Second {
			t.Errorf("ChallengeWaitTime = %v, want 30s", cfg.ChallengeWaitTime)
		}
		if cfg.SessionMaxIdle != 10*time.Minute {
			t.Errorf("SessionMaxIdle = %v, want 10m", cfg.SessionMaxIdle)
		}
		if cfg.RequiredFeature != "captcha" {
			t.Errorf("RequiredFeature = %q, want %q", cfg.RequiredFeature, "captcha")
		}
		if cfg.AllowUnauthenticated != false {
			t.Errorf("AllowUnauthenticated = %v, want false", cfg.AllowUnauthenticated)
		}
		if cfg.ProxyEnabled != false {
			t.Errorf("ProxyEnabled = %v, want false", cfg.ProxyEnabled)
		}
	})

	t.Run("from env", func(t *testing.T) {
		os.Setenv("PORT", "9000")
		os.Setenv("LOG_LEVEL", "debug")
		os.Setenv("BROWSER_POOL_SIZE", "10")
		os.Setenv("BROWSER_IDLE_TIMEOUT", "10m")
		os.Setenv("BROWSER_MAX_AGE", "1h")
		os.Setenv("CHROME_PATH", "/usr/bin/chromium")
		os.Setenv("CHALLENGE_TIMEOUT", "120s")
		os.Setenv("TWOCAPTCHA_API_KEY", "test-2captcha-key")
		os.Setenv("CAPSOLVER_API_KEY", "test-capsolver-key")
		os.Setenv("REFYNE_API_SECRET", "secret-key")
		os.Setenv("CLERK_ISSUER", "https://test.clerk.accounts.dev")
		os.Setenv("REQUIRED_FEATURE", "premium_captcha")
		os.Setenv("ALLOW_UNAUTHENTICATED", "true")
		os.Setenv("PROXY_ENABLED", "true")
		os.Setenv("PROXY_URL", "http://proxy:8080")

		cfg := Load()

		if cfg.Port != 9000 {
			t.Errorf("Port = %d, want 9000", cfg.Port)
		}
		if cfg.LogLevel != "debug" {
			t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
		}
		if cfg.BrowserPoolSize != 10 {
			t.Errorf("BrowserPoolSize = %d, want 10", cfg.BrowserPoolSize)
		}
		if cfg.BrowserIdleTimeout != 10*time.Minute {
			t.Errorf("BrowserIdleTimeout = %v, want 10m", cfg.BrowserIdleTimeout)
		}
		if cfg.BrowserMaxAge != time.Hour {
			t.Errorf("BrowserMaxAge = %v, want 1h", cfg.BrowserMaxAge)
		}
		if cfg.ChromePath != "/usr/bin/chromium" {
			t.Errorf("ChromePath = %q, want %q", cfg.ChromePath, "/usr/bin/chromium")
		}
		if cfg.ChallengeTimeout != 120*time.Second {
			t.Errorf("ChallengeTimeout = %v, want 120s", cfg.ChallengeTimeout)
		}
		if cfg.TwoCaptchaAPIKey != "test-2captcha-key" {
			t.Errorf("TwoCaptchaAPIKey = %q, want %q", cfg.TwoCaptchaAPIKey, "test-2captcha-key")
		}
		if cfg.CapSolverAPIKey != "test-capsolver-key" {
			t.Errorf("CapSolverAPIKey = %q, want %q", cfg.CapSolverAPIKey, "test-capsolver-key")
		}
		if cfg.RefyneAPISecret != "secret-key" {
			t.Errorf("RefyneAPISecret = %q, want %q", cfg.RefyneAPISecret, "secret-key")
		}
		if cfg.ClerkIssuer != "https://test.clerk.accounts.dev" {
			t.Errorf("ClerkIssuer = %q, want %q", cfg.ClerkIssuer, "https://test.clerk.accounts.dev")
		}
		if cfg.RequiredFeature != "premium_captcha" {
			t.Errorf("RequiredFeature = %q, want %q", cfg.RequiredFeature, "premium_captcha")
		}
		if cfg.AllowUnauthenticated != true {
			t.Errorf("AllowUnauthenticated = %v, want true", cfg.AllowUnauthenticated)
		}
		if cfg.ProxyEnabled != true {
			t.Errorf("ProxyEnabled = %v, want true", cfg.ProxyEnabled)
		}
		if cfg.ProxyURL != "http://proxy:8080" {
			t.Errorf("ProxyURL = %q, want %q", cfg.ProxyURL, "http://proxy:8080")
		}
	})

	t.Run("invalid values use defaults", func(t *testing.T) {
		os.Setenv("PORT", "not-a-number")
		os.Setenv("BROWSER_IDLE_TIMEOUT", "invalid-duration")

		cfg := Load()

		if cfg.Port != 8191 {
			t.Errorf("Port with invalid value = %d, want default 8191", cfg.Port)
		}
		if cfg.BrowserIdleTimeout != 5*time.Minute {
			t.Errorf("BrowserIdleTimeout with invalid value = %v, want default 5m", cfg.BrowserIdleTimeout)
		}
	})
}

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_VAR", "test-value")
	defer os.Unsetenv("TEST_VAR")

	if got := getEnv("TEST_VAR", "default"); got != "test-value" {
		t.Errorf("getEnv() = %q, want %q", got, "test-value")
	}

	if got := getEnv("NONEXISTENT_VAR", "default"); got != "default" {
		t.Errorf("getEnv() for missing var = %q, want %q", got, "default")
	}
}

func TestGetEnvInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	if got := getEnvInt("TEST_INT", 0); got != 42 {
		t.Errorf("getEnvInt() = %d, want %d", got, 42)
	}

	os.Setenv("TEST_INT", "not-a-number")
	if got := getEnvInt("TEST_INT", 10); got != 10 {
		t.Errorf("getEnvInt() with invalid value = %d, want default %d", got, 10)
	}

	if got := getEnvInt("NONEXISTENT_VAR", 99); got != 99 {
		t.Errorf("getEnvInt() for missing var = %d, want %d", got, 99)
	}
}

func TestGetEnvDuration(t *testing.T) {
	os.Setenv("TEST_DUR", "5m")
	defer os.Unsetenv("TEST_DUR")

	if got := getEnvDuration("TEST_DUR", time.Second); got != 5*time.Minute {
		t.Errorf("getEnvDuration() = %v, want %v", got, 5*time.Minute)
	}

	os.Setenv("TEST_DUR", "invalid")
	if got := getEnvDuration("TEST_DUR", time.Hour); got != time.Hour {
		t.Errorf("getEnvDuration() with invalid value = %v, want default %v", got, time.Hour)
	}

	if got := getEnvDuration("NONEXISTENT_VAR", 30*time.Second); got != 30*time.Second {
		t.Errorf("getEnvDuration() for missing var = %v, want %v", got, 30*time.Second)
	}
}
