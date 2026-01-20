// Package config provides configuration management for the captcha service.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the captcha service.
type Config struct {
	// Server settings
	Port     int
	LogLevel string

	// Browser pool settings
	BrowserPoolSize    int
	BrowserIdleTimeout time.Duration
	BrowserMaxRequests int
	BrowserMaxAge      time.Duration
	ChromePath         string

	// Challenge solving settings
	ChallengeTimeout  time.Duration
	ChallengeWaitTime time.Duration

	// CAPTCHA solver settings
	TwoCaptchaAPIKey string
	CapSolverAPIKey  string

	// Authentication
	RefyneAPISecret      string // HMAC secret for signed headers from refyne-api
	ClerkIssuer          string // Clerk issuer URL for JWT validation
	RequiredFeature      string // Feature required to use captcha (e.g., "captcha")
	AllowUnauthenticated bool   // Allow unauthenticated requests (for testing)

	// Proxy settings
	ProxyEnabled bool
	ProxyURL     string

	// Session settings
	SessionMaxIdle time.Duration
}

// Load creates a Config from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Port:               getEnvInt("PORT", 8191),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		BrowserPoolSize:    getEnvInt("BROWSER_POOL_SIZE", 5),
		BrowserIdleTimeout: getEnvDuration("BROWSER_IDLE_TIMEOUT", 5*time.Minute),
		BrowserMaxRequests: getEnvInt("BROWSER_MAX_REQUESTS", 100),
		BrowserMaxAge:      getEnvDuration("BROWSER_MAX_AGE", 30*time.Minute),
		ChromePath:         getEnv("CHROME_PATH", ""),
		ChallengeTimeout:   getEnvDuration("CHALLENGE_TIMEOUT", 60*time.Second),
		ChallengeWaitTime:  getEnvDuration("CHALLENGE_WAIT_TIME", 30*time.Second),
		TwoCaptchaAPIKey:   getEnv("TWOCAPTCHA_API_KEY", ""),
		CapSolverAPIKey:    getEnv("CAPSOLVER_API_KEY", ""),
		RefyneAPISecret:      getEnv("REFYNE_API_SECRET", ""),
		ClerkIssuer:          getEnv("CLERK_ISSUER", ""),
		RequiredFeature:      getEnv("REQUIRED_FEATURE", "captcha"),
		AllowUnauthenticated: getEnv("ALLOW_UNAUTHENTICATED", "false") == "true",
		ProxyEnabled:         getEnv("PROXY_ENABLED", "false") == "true",
		ProxyURL:           getEnv("PROXY_URL", ""),
		SessionMaxIdle:     getEnvDuration("SESSION_MAX_IDLE", 10*time.Minute),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
