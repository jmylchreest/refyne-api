// Package config handles application configuration.
package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/hkdf"
)

// Config holds all application configuration.
type Config struct {
	// Server settings
	Port    int
	BaseURL string

	// Database
	DatabaseURL string

	// Authentication
	JWTSecret       string
	JWTExpiry       time.Duration
	RefreshExpiry   time.Duration
	EncryptionKey   []byte // 32-byte key for AES-256-GCM encryption

	// Clerk Authentication
	ClerkIssuerURL      string // e.g., "https://xxx.clerk.accounts.dev"
	ClerkSecretKey      string // Clerk Backend API secret key (sk_xxx)
	ClerkWebhookSecret  string // Svix signing secret for Clerk webhooks

	// OAuth providers
	OAuthGoogleClientID       string
	OAuthGoogleClientSecret   string
	OAuthGitHubClientID       string
	OAuthGitHubClientSecret   string
	OAuthMicrosoftClientID    string
	OAuthMicrosoftClientSecret string

	// Stripe
	StripeSecretKey     string
	StripeWebhookSecret string

	// Service LLM keys (for credit users)
	ServiceAnthropicKey  string
	ServiceOpenAIKey     string
	ServiceOpenRouterKey string

	// CORS
	CORSOrigins []string

	// Deployment mode
	DeploymentMode string // "hosted" or "selfhosted"
	AdminEnabled   bool

	// Self-hosted
	LicenseKey     string
	DataDir        string // If set, enables persistence
	APIKeyHash     string // Pre-hashed API key for self-hosted auth

	// Telemetry
	TelemetryDisabled bool
	TelemetryEndpoint string

	// Object Storage (Tigris/S3-compatible)
	StorageEnabled   bool
	StorageEndpoint  string // AWS_ENDPOINT_URL_S3 for Tigris
	StorageAccessKey string // AWS_ACCESS_KEY_ID
	StorageSecretKey string // AWS_SECRET_ACCESS_KEY
	StorageBucket    string // Bucket name (one per environment)
	StorageRegion    string // Region (auto for Tigris)
	BlocklistBucket  string // Optional separate bucket for blocklist (defaults to StorageBucket)

	// Cleanup
	CleanupEnabled       bool          // Enable automatic cleanup
	CleanupMaxAgeResults time.Duration // Max age of job results to keep (default 30 days)
	CleanupMaxAgeDebug   time.Duration // Max age of debug captures to keep (default 7 days)
	CleanupInterval      time.Duration // How often to run cleanup (default 24 hours)

	// Worker
	WorkerPollInterval      time.Duration // How often to poll for new jobs (default 5s)
	WorkerConcurrency       int           // Number of concurrent workers (default 3)
	WorkerShutdownGracePeriod time.Duration // Max time to wait for running jobs during shutdown (default 5m)

	// Captcha/Dynamic Content Service (internal)
	CaptchaServiceURL     string // Internal URL of captcha service (use .internal for Fly private networking)
	CaptchaSecret         string // HMAC secret for signing requests to captcha service
	CaptchaExternalAPIKey string // API key for external captcha services (2captcha, etc.) when native solving fails

	// Idle shutdown settings (for scale-to-zero on Fly.io)
	IdleTimeout time.Duration // Time before shutting down when idle (0 = disabled)
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		Port:              getEnvInt("PORT", 8080),
		BaseURL:           getEnv("BASE_URL", "http://localhost:8080"),
		DatabaseURL:       getEnv("DATABASE_URL", "file:refyne.db?_journal=WAL&_timeout=5000"),
		JWTSecret:         getEnv("JWT_SECRET", ""),
		JWTExpiry:         getEnvDuration("JWT_EXPIRY", 15*time.Minute),
		RefreshExpiry:     getEnvDuration("REFRESH_EXPIRY", 720*time.Hour),

		OAuthGoogleClientID:        getEnv("OAUTH_GOOGLE_CLIENT_ID", ""),
		OAuthGoogleClientSecret:    getEnv("OAUTH_GOOGLE_CLIENT_SECRET", ""),
		OAuthGitHubClientID:        getEnv("OAUTH_GITHUB_CLIENT_ID", ""),
		OAuthGitHubClientSecret:    getEnv("OAUTH_GITHUB_CLIENT_SECRET", ""),
		OAuthMicrosoftClientID:     getEnv("OAUTH_MICROSOFT_CLIENT_ID", ""),
		OAuthMicrosoftClientSecret: getEnv("OAUTH_MICROSOFT_CLIENT_SECRET", ""),

		StripeSecretKey:     getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),

		ServiceAnthropicKey:  getEnv("SERVICE_ANTHROPIC_KEY", ""),
		ServiceOpenAIKey:     getEnv("SERVICE_OPENAI_KEY", ""),
		ServiceOpenRouterKey: getEnv("SERVICE_OPENROUTER_KEY", ""),

		ClerkIssuerURL:     getEnv("CLERK_ISSUER_URL", ""),
		ClerkSecretKey:     getEnv("CLERK_SECRET_KEY", ""),
		ClerkWebhookSecret: getEnv("CLERK_WEBHOOK_SECRET", ""),

		CORSOrigins:    getEnvSlice("CORS_ORIGINS", []string{"http://localhost:3000"}),
		DeploymentMode: getEnv("DEPLOYMENT_MODE", "hosted"),
		AdminEnabled:   getEnvBool("ADMIN_ENABLED", true),

		LicenseKey:        getEnv("REFYNE_LICENSE_KEY", ""),
		DataDir:           getEnv("REFYNE_DATA_DIR", ""),
		APIKeyHash:        getEnv("REFYNE_API_KEY_HASH", ""),
		TelemetryDisabled: getEnvBool("REFYNE_TELEMETRY_DISABLED", false),
		TelemetryEndpoint: getEnv("REFYNE_TELEMETRY_ENDPOINT", "https://api.refyne.uk/api/v1/telemetry/ingest"),

		// Object Storage (Tigris/S3-compatible) - uses Fly's standard env vars
		// BUCKET_NAME is set automatically by `fly storage create`
		StorageEndpoint:  getEnv("AWS_ENDPOINT_URL_S3", ""),
		StorageAccessKey: getEnv("AWS_ACCESS_KEY_ID", ""),
		StorageSecretKey: getEnv("AWS_SECRET_ACCESS_KEY", ""),
		StorageBucket:    getEnvWithFallback("BUCKET_NAME", "STORAGE_BUCKET", ""),
		StorageRegion:    getEnv("AWS_REGION", "auto"),
	}

	// Enable storage if bucket is configured
	cfg.StorageEnabled = cfg.StorageBucket != "" && cfg.StorageEndpoint != ""

	// Blocklist bucket defaults to main storage bucket
	cfg.BlocklistBucket = getEnv("BLOCKLIST_BUCKET", cfg.StorageBucket)

	// Cleanup configuration
	cfg.CleanupEnabled = getEnvBool("CLEANUP_ENABLED", true)
	cfg.CleanupMaxAgeResults = getEnvDuration("CLEANUP_MAX_AGE_RESULTS", 30*24*time.Hour) // 30 days default
	cfg.CleanupMaxAgeDebug = getEnvDuration("CLEANUP_MAX_AGE_DEBUG", 7*24*time.Hour)     // 7 days default
	cfg.CleanupInterval = getEnvDuration("CLEANUP_INTERVAL", 24*time.Hour)               // Daily default

	// Worker configuration
	cfg.WorkerPollInterval = getEnvDuration("WORKER_POLL_INTERVAL", 5*time.Second)
	cfg.WorkerConcurrency = getEnvInt("WORKER_CONCURRENCY", 3)
	cfg.WorkerShutdownGracePeriod = getEnvDuration("WORKER_SHUTDOWN_GRACE_PERIOD", 5*time.Minute)

	// Captcha/dynamic content service configuration (internal service)
	cfg.CaptchaServiceURL = getEnv("CAPTCHA_SERVICE_URL", "")
	cfg.CaptchaSecret = getEnv("CAPTCHA_SECRET", "")
	cfg.CaptchaExternalAPIKey = getEnv("CAPTCHA_EXTERNAL_API_KEY", "")

	// Idle shutdown configuration (for Fly.io scale-to-zero)
	cfg.IdleTimeout = getEnvDuration("IDLE_TIMEOUT", 0) // 0 = disabled

	// Validate required fields for hosted mode
	if cfg.DeploymentMode == "hosted" {
		if cfg.ClerkIssuerURL == "" {
			return nil, fmt.Errorf("CLERK_ISSUER_URL is required for hosted mode")
		}
	}

	// Self-hosted mode adjustments
	if cfg.DeploymentMode == "selfhosted" {
		cfg.AdminEnabled = false

		// Generate a random JWT secret for self-hosted if not provided
		if cfg.JWTSecret == "" {
			cfg.JWTSecret = generateRandomSecret(64)
		}
	}

	// Set up encryption key (derive from JWT secret if not explicitly set)
	encKeyStr := getEnv("ENCRYPTION_KEY", "")
	if encKeyStr != "" {
		// Decode base64 key if provided
		decoded, err := base64.StdEncoding.DecodeString(encKeyStr)
		if err != nil || len(decoded) != 32 {
			return nil, fmt.Errorf("ENCRYPTION_KEY must be a base64-encoded 32-byte key")
		}
		cfg.EncryptionKey = decoded
	} else {
		// Derive from JWT secret for backward compatibility
		cfg.EncryptionKey = deriveEncryptionKey(cfg.JWTSecret)
	}

	return cfg, nil
}

// IsSelfHosted returns true if running in self-hosted mode.
func (c *Config) IsSelfHosted() bool {
	return c.DeploymentMode == "selfhosted"
}

// HasPersistence returns true if data persistence is enabled.
func (c *Config) HasPersistence() bool {
	return c.DataDir != ""
}

// CaptchaEnabled returns true if captcha service is configured.
func (c *Config) CaptchaEnabled() bool {
	return c.CaptchaServiceURL != "" && c.CaptchaSecret != ""
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		lower := strings.ToLower(value)
		return lower == "true" || lower == "1" || lower == "yes"
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}

func getEnvWithFallback(primary, fallback, defaultValue string) string {
	if value := os.Getenv(primary); value != "" {
		return value
	}
	if value := os.Getenv(fallback); value != "" {
		return value
	}
	return defaultValue
}

func generateRandomSecret(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback (should never happen)
		return "self-hosted-secret-change-me-" + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return base64.URLEncoding.EncodeToString(bytes)
}

// deriveEncryptionKey creates a 32-byte AES-256 key from a secret string using HKDF.
// HKDF (HMAC-based Key Derivation Function) is appropriate for deriving keys from
// high-entropy secrets like JWT secrets. For low-entropy passwords, use Argon2 instead.
func deriveEncryptionKey(secret string) []byte {
	// Use HKDF with SHA-256
	// - Salt: fixed but unique to this application
	// - Info: context string to bind the key to its purpose
	salt := []byte("refyne-api-encryption-key-v1")
	info := []byte("aes-256-gcm-encryption")

	hkdfReader := hkdf.New(sha256.New, []byte(secret), salt, info)

	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		// This should never happen with valid inputs
		panic("hkdf: failed to derive key: " + err.Error())
	}

	return key
}
