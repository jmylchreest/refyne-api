// Package config provides configuration management for the captcha service.
package config

import (
	"os"
	"strconv"
	"strings"
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
	RequiredFeature      string // Feature required to use captcha (e.g., "content_dynamic")
	AllowUnauthenticated bool   // Standalone/FlareSolverr mode - disables all auth (for local dev)

	// Proxy settings
	ProxyEnabled bool
	ProxyURL     string

	// Session settings
	SessionMaxIdle time.Duration
	SessionDBPath  string // Path to SQLite database for session persistence

	// Idle shutdown settings
	IdleTimeout time.Duration // Time before shutting down when idle (0 = disabled)

	// Debug settings
	DisableStealth bool // Disable stealth mode for testing CAPTCHA solving
}

// Load creates a Config from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Port:                 getEnvInt("PORT", 8191),
		LogLevel:             getEnv("LOG_LEVEL", "info"),
		BrowserPoolSize:      getEnvInt("BROWSER_POOL_SIZE", 5),
		BrowserIdleTimeout:   getEnvDuration("BROWSER_IDLE_TIMEOUT", 5*time.Minute),
		BrowserMaxRequests:   getEnvInt("BROWSER_MAX_REQUESTS", 100),
		BrowserMaxAge:        getEnvDuration("BROWSER_MAX_AGE", 30*time.Minute),
		ChromePath:           getEnv("CHROME_PATH", ""),
		ChallengeTimeout:     getEnvDuration("CHALLENGE_TIMEOUT", 60*time.Second),
		ChallengeWaitTime:    getEnvDuration("CHALLENGE_WAIT_TIME", 30*time.Second),
		TwoCaptchaAPIKey:     getEnv("TWOCAPTCHA_API_KEY", ""),
		CapSolverAPIKey:      getEnv("CAPSOLVER_API_KEY", ""),
		RefyneAPISecret:      getEnv("REFYNE_API_SECRET", ""),
		ClerkIssuer:          getEnv("CLERK_ISSUER", ""),
		RequiredFeature:      getEnv("REQUIRED_FEATURE", "content_dynamic"),
		AllowUnauthenticated: getEnvBool("ALLOW_UNAUTHENTICATED", false),
		ProxyEnabled:         getEnvBool("PROXY_ENABLED", false),
		ProxyURL:             getEnv("PROXY_URL", ""),
		SessionMaxIdle:       getEnvDuration("SESSION_MAX_IDLE", 10*time.Minute),
		SessionDBPath:        getEnv("SESSION_DB_PATH", ""),
		IdleTimeout:          getEnvDuration("IDLE_TIMEOUT", 0), // 0 = disabled
		DisableStealth:       getEnvBool("DISABLE_STEALTH", false),
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

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		lower := strings.ToLower(val)
		return lower == "true" || lower == "1" || lower == "yes"
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

