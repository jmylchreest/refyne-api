// Package models defines the domain models for the application.
// Note: User management, OAuth, sessions, subscriptions, and credits are handled by Clerk.
// The UserID fields reference Clerk user IDs (e.g., "user_xxx").
package models

import (
	"time"
)

// APIKey represents an API key for programmatic access.
type APIKey struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"` // Clerk user ID
	Name       string     `json:"name"`
	KeyHash    string     `json:"-"`
	KeyPrefix  string     `json:"key_prefix"` // First 8 chars for display
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// LLMConfig represents a user's LLM configuration.
type LLMConfig struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"` // Clerk user ID
	Provider        string    `json:"provider"` // anthropic, openai, openrouter, ollama
	APIKeyEncrypted string    `json:"-"`
	BaseURL         string    `json:"base_url,omitempty"`
	Model           string    `json:"model,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// JobStatus represents the status of a job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// JobType represents the type of job.
type JobType string

const (
	JobTypeExtract JobType = "extract"
	JobTypeCrawl   JobType = "crawl"
)

// Job represents an extraction or crawl job.
type Job struct {
	ID               string     `json:"id"`
	UserID           string     `json:"user_id"` // Clerk user ID
	Type             JobType    `json:"type"`
	Status           JobStatus  `json:"status"`
	URL              string     `json:"url"`
	SchemaJSON       string     `json:"schema_json"`
	CrawlOptionsJSON string     `json:"crawl_options_json,omitempty"`
	ResultJSON       string     `json:"result_json,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	PageCount        int        `json:"page_count"`
	TokenUsageInput  int        `json:"token_usage_input"`
	TokenUsageOutput int        `json:"token_usage_output"`
	CostCredits      int        `json:"cost_credits"`
	WebhookURL       string     `json:"webhook_url,omitempty"`
	WebhookStatus    string     `json:"webhook_status,omitempty"`
	WebhookAttempts  int        `json:"webhook_attempts"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// JobResult represents a single result from a crawl job.
type JobResult struct {
	ID                string    `json:"id"`
	JobID             string    `json:"job_id"`
	URL               string    `json:"url"`
	DataJSON          string    `json:"data_json,omitempty"`
	ErrorMessage      string    `json:"error_message,omitempty"`
	TokenUsageInput   int       `json:"token_usage_input"`
	TokenUsageOutput  int       `json:"token_usage_output"`
	FetchDurationMs   int       `json:"fetch_duration_ms"`
	ExtractDurationMs int       `json:"extract_duration_ms"`
	CreatedAt         time.Time `json:"created_at"`
}

// UsageRecord represents a lean usage tracking record for billing.
// Detailed telemetry is stored in UsageInsight (1:1 relationship).
type UsageRecord struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"` // Clerk user ID
	JobID           string    `json:"job_id,omitempty"`
	Date            string    `json:"date"`    // YYYY-MM-DD, indexed for fast billing queries
	Type            JobType   `json:"type"`    // extract, crawl
	Status          string    `json:"status"`  // success, failed, partial
	TotalChargedUSD float64   `json:"total_charged_usd"` // What we debited from balance
	IsBYOK          bool      `json:"is_byok"` // True if user's own API key was used
	CreatedAt       time.Time `json:"created_at"`
}

// TelemetryEvent represents a telemetry event for analytics.
type TelemetryEvent struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id,omitempty"` // Clerk user ID (optional)
	EventType string    `json:"event_type"`
	EventData string    `json:"event_data,omitempty"` // JSON
	CreatedAt time.Time `json:"created_at"`
}

// License represents a self-hosted license.
type License struct {
	ID               string     `json:"id"`
	LicenseKey       string     `json:"license_key"`
	OrganizationName string     `json:"organization_name"`
	Email            string     `json:"email"`
	Tier             string     `json:"tier"`
	MaxUsers         int        `json:"max_users"`
	Features         []string   `json:"features"`
	IssuedAt         time.Time  `json:"issued_at"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// ServiceKey represents a system-wide LLM provider API key (admin-configured).
// These are the service keys used for users on free tier or without BYOK.
type ServiceKey struct {
	ID              string    `json:"id"`
	Provider        string    `json:"provider"` // openrouter, anthropic, openai
	APIKeyEncrypted string    `json:"-"`        // Encrypted API key
	DefaultModel    string    `json:"default_model"`
	IsEnabled       bool      `json:"is_enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// FallbackChainEntry represents a single entry in the LLM fallback chain.
// The extraction service tries each entry in order until one succeeds.
// Tier can be nil for the default chain, or a specific tier name (free, pro, enterprise).
type FallbackChainEntry struct {
	ID        string    `json:"id"`
	Tier      *string   `json:"tier,omitempty"` // nil = default chain, otherwise tier-specific
	Position  int       `json:"position"`       // Order in the chain (1, 2, 3...)
	Provider  string    `json:"provider"`       // openrouter, anthropic, openai, ollama
	Model     string    `json:"model"`          // Model identifier (e.g., "xiaomi/mimo-v2-flash:free")
	IsEnabled bool      `json:"is_enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
