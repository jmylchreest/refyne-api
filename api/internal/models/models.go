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
	JobTypeAnalyze JobType = "analyze"
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
	ErrorMessage     string     `json:"error_message,omitempty"`  // User-visible error (sanitized for non-BYOK)
	ErrorDetails     string     `json:"error_details,omitempty"`  // Full error details (admin/BYOK only)
	ErrorCategory    string     `json:"error_category,omitempty"` // Error classification
	LLMConfigsJSON   string     `json:"llm_configs_json"`         // Resolved LLM config chain (JSON array of LLMConfigInput)
	Tier             string     `json:"tier"`                     // User's subscription tier at job creation time
	IsBYOK           bool       `json:"is_byok"`                  // True if user's own API key was used
	LLMProvider      string     `json:"llm_provider,omitempty"`   // Last provider attempted
	LLMModel         string     `json:"llm_model,omitempty"`      // Last model attempted
	DiscoveryMethod  string     `json:"discovery_method,omitempty"` // How URLs were discovered: "sitemap", "links", or "" for single-page
	URLsQueued       int        `json:"urls_queued"`              // Total URLs queued for processing (for progress tracking)
	PageCount        int        `json:"page_count"`
	TokenUsageInput  int        `json:"token_usage_input"`
	TokenUsageOutput int        `json:"token_usage_output"`
	CostUSD          float64    `json:"cost_usd"`     // USD cost charged to user (0 for BYOK)
	LLMCostUSD       float64    `json:"llm_cost_usd"`     // Actual LLM provider cost (always recorded)
	CaptureDebug     bool       `json:"capture_debug"`    // Whether to capture LLM requests for debugging
	WebhookURL       string     `json:"webhook_url,omitempty"`
	WebhookStatus    string     `json:"webhook_status,omitempty"`
	WebhookAttempts  int        `json:"webhook_attempts"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// CrawlStatus represents the crawl state of a URL in a job.
type CrawlStatus string

const (
	CrawlStatusPending   CrawlStatus = "pending"   // Discovered but not yet crawled
	CrawlStatusCrawling  CrawlStatus = "crawling"  // Currently being fetched/extracted
	CrawlStatusCompleted CrawlStatus = "completed" // Successfully processed
	CrawlStatusFailed    CrawlStatus = "failed"    // Failed to process
	CrawlStatusSkipped   CrawlStatus = "skipped"   // Skipped (e.g., max depth/pages reached)
)

// JobResult represents a single result from a crawl job.
// For crawl jobs, this also serves as the crawl map, tracking the
// relationship between pages (parent_url) and their discovery depth.
type JobResult struct {
	ID                string       `json:"id"`
	JobID             string       `json:"job_id"`
	URL               string       `json:"url"`
	ParentURL         *string      `json:"parent_url,omitempty"`   // URL that discovered this one (nil for seed)
	Depth             int          `json:"depth"`                  // 0 for seed URL, increments for each level
	CrawlStatus       CrawlStatus  `json:"crawl_status"`           // pending, crawling, completed, failed, skipped
	DataJSON          string       `json:"data_json,omitempty"`
	ErrorMessage      string       `json:"error_message,omitempty"`  // User-visible error (sanitized for non-BYOK)
	ErrorDetails      string       `json:"error_details,omitempty"`  // Full error details (admin/BYOK only)
	ErrorCategory     string       `json:"error_category,omitempty"` // Error classification for retry logic
	LLMProvider       string       `json:"llm_provider,omitempty"`   // Provider used (admin/BYOK only)
	LLMModel          string       `json:"llm_model,omitempty"`      // Model used (admin/BYOK only)
	IsBYOK            bool         `json:"is_byok"`                  // True if user's own API key was used
	RetryCount        int          `json:"retry_count"`              // Number of retry attempts made
	TokenUsageInput   int          `json:"token_usage_input"`
	TokenUsageOutput  int          `json:"token_usage_output"`
	FetchDurationMs   int          `json:"fetch_duration_ms"`
	ExtractDurationMs int          `json:"extract_duration_ms"`
	DiscoveredAt      *time.Time   `json:"discovered_at,omitempty"` // When URL was discovered
	CompletedAt       *time.Time   `json:"completed_at,omitempty"`  // When processing finished
	CreatedAt         time.Time    `json:"created_at"`
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
	ID          string    `json:"id"`
	Tier        *string   `json:"tier,omitempty"` // nil = default chain, otherwise tier-specific
	Position    int       `json:"position"`       // Order in the chain (1, 2, 3...)
	Provider    string    `json:"provider"`       // openrouter, anthropic, openai, ollama
	Model       string    `json:"model"`          // Model identifier (e.g., "xiaomi/mimo-v2-flash:free")
	Temperature *float64  `json:"temperature,omitempty"` // nil = use default for model/provider
	MaxTokens   *int      `json:"max_tokens,omitempty"`  // nil = use default for model/provider
	StrictMode  *bool     `json:"strict_mode,omitempty"` // nil = use default for model (most models: false)
	IsEnabled   bool      `json:"is_enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserServiceKey represents a user-configured LLM provider API key.
// Similar to ServiceKey but per-user. Models are specified in UserFallbackChainEntry.
type UserServiceKey struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"` // Clerk user ID
	Provider        string    `json:"provider"` // anthropic, openai, openrouter, ollama
	APIKeyEncrypted string    `json:"-"`        // Encrypted API key
	BaseURL         string    `json:"base_url,omitempty"` // For ollama or custom endpoints
	IsEnabled       bool      `json:"is_enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// UserFallbackChainEntry represents an entry in a user's personal fallback chain.
// The extraction service tries each entry in order, using the user's configured
// provider keys from UserServiceKey.
type UserFallbackChainEntry struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"` // Clerk user ID
	Position    int       `json:"position"` // Order in the chain (1, 2, 3...)
	Provider    string    `json:"provider"` // anthropic, openai, openrouter, ollama
	Model       string    `json:"model"`    // Model identifier
	Temperature *float64  `json:"temperature,omitempty"` // nil = use default for model/provider
	MaxTokens   *int      `json:"max_tokens,omitempty"`  // nil = use default for model/provider
	StrictMode  *bool     `json:"strict_mode,omitempty"` // nil = use default for model (most models: false)
	IsEnabled   bool      `json:"is_enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WebhookEventType represents the type of webhook event.
type WebhookEventType string

const (
	WebhookEventAll            WebhookEventType = "*"
	WebhookEventJobStarted     WebhookEventType = "job.started"
	WebhookEventJobCompleted   WebhookEventType = "job.completed"
	WebhookEventJobFailed      WebhookEventType = "job.failed"
	WebhookEventJobProgress    WebhookEventType = "job.progress"
	WebhookEventExtractSuccess WebhookEventType = "extract.success"
	WebhookEventExtractFailed  WebhookEventType = "extract.failed"
)

// WebhookDeliveryStatus represents the status of a webhook delivery.
type WebhookDeliveryStatus string

const (
	WebhookDeliveryStatusPending  WebhookDeliveryStatus = "pending"
	WebhookDeliveryStatusSuccess  WebhookDeliveryStatus = "success"
	WebhookDeliveryStatusFailed   WebhookDeliveryStatus = "failed"
	WebhookDeliveryStatusRetrying WebhookDeliveryStatus = "retrying"
)

// Webhook represents a user-defined webhook endpoint.
type Webhook struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	Name            string    `json:"name"`
	URL             string    `json:"url"`
	SecretEncrypted string    `json:"-"`        // Encrypted webhook secret for HMAC signing
	Events          []string  `json:"events"`   // Event types to subscribe to (["*"] for all)
	Headers         []Header  `json:"headers"`  // Custom headers to include
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Header represents a custom HTTP header for webhook requests.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// WebhookDelivery represents a single webhook delivery attempt.
type WebhookDelivery struct {
	ID              string                `json:"id"`
	WebhookID       *string               `json:"webhook_id,omitempty"` // nil for ephemeral webhooks
	JobID           string                `json:"job_id"`
	EventType       string                `json:"event_type"`
	URL             string                `json:"url"`
	PayloadJSON     string                `json:"payload_json"`
	RequestHeaders  []Header              `json:"request_headers,omitempty"`
	StatusCode      *int                  `json:"status_code,omitempty"`
	ResponseBody    string                `json:"response_body,omitempty"`
	ResponseTimeMs  *int                  `json:"response_time_ms,omitempty"`
	Status          WebhookDeliveryStatus `json:"status"`
	ErrorMessage    string                `json:"error_message,omitempty"`
	AttemptNumber   int                   `json:"attempt_number"`
	MaxAttempts     int                   `json:"max_attempts"`
	NextRetryAt     *time.Time            `json:"next_retry_at,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	DeliveredAt     *time.Time            `json:"delivered_at,omitempty"`
}
