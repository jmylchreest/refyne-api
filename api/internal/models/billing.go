// Package models defines the domain models for the application.
package models

import "time"

// ========================================
// User Balance
// ========================================

// UserBalance tracks a user's USD balance for API usage.
type UserBalance struct {
	UserID        string     `json:"user_id"`
	BalanceUSD    float64    `json:"balance_usd"`
	LifetimeAdded float64    `json:"lifetime_added"`
	LifetimeSpent float64    `json:"lifetime_spent"`
	PeriodStart   *time.Time `json:"period_start,omitempty"` // Current billing period start (from Clerk subscription)
	PeriodEnd     *time.Time `json:"period_end,omitempty"`   // Current billing period end (from Clerk subscription)
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ========================================
// Credit Transactions
// ========================================

// CreditTransactionType defines the type of credit transaction.
type CreditTransactionType string

const (
	TxTypeSubscription CreditTransactionType = "subscription" // Monthly credit allocation
	TxTypeTopUp        CreditTransactionType = "topup"        // Manual top-up purchase
	TxTypeUsage        CreditTransactionType = "usage"        // API usage deduction
	TxTypeExpiry       CreditTransactionType = "expiry"       // Credits expired (rollover)
	TxTypeRefund       CreditTransactionType = "refund"       // Refund (doesn't claw back spent)
	TxTypeAdjustment   CreditTransactionType = "adjustment"   // Manual admin adjustment
)

// CreditTransaction provides full audit trail for all credit movements.
type CreditTransaction struct {
	ID           string                `json:"id"`
	UserID       string                `json:"user_id"`
	Type         CreditTransactionType `json:"type"`
	AmountUSD    float64               `json:"amount_usd"`    // Positive=credit, Negative=debit
	BalanceAfter float64               `json:"balance_after"` // Balance after this transaction

	// Expiry tracking - set at creation time based on payment_date + rollover_months
	ExpiresAt *time.Time `json:"expires_at,omitempty"` // NULL = never expires
	IsExpired bool       `json:"is_expired"`           // Marked true when expired

	// Idempotency and references
	StripePaymentID *string `json:"stripe_payment_id,omitempty"` // UNIQUE - prevents double-credit
	JobID           *string `json:"job_id,omitempty"`            // For usage transactions

	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// ========================================
// Schema Snapshots
// ========================================

// SchemaSnapshot stores versioned schemas for deduplication and audit.
type SchemaSnapshot struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Hash       string    `json:"hash"`        // SHA256 of SchemaJSON for dedup
	SchemaJSON string    `json:"schema_json"` // The actual schema
	Name       string    `json:"name"`        // User-friendly name (optional)
	Version    int       `json:"version"`     // Auto-increment per user
	UsageCount int       `json:"usage_count"` // How many times used
	CreatedAt  time.Time `json:"created_at"`
}

// ========================================
// Usage Insights (Rich analytics table)
// ========================================

// UsageInsight stores detailed telemetry for each usage record.
// This is a 1:1 relationship with UsageRecord, kept separate for performance.
type UsageInsight struct {
	ID      string `json:"id"`
	UsageID string `json:"usage_id"` // FK to usage_records

	// Job context
	TargetURL       string `json:"target_url"`
	SchemaID        string `json:"schema_id"`                   // FK to schema_snapshots
	CrawlConfigJSON string `json:"crawl_config_json,omitempty"` // JSON for crawl jobs

	// Error details
	ErrorMessage string `json:"error_message,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`

	// Token usage
	TokensInput  int `json:"tokens_input"`
	TokensOutput int `json:"tokens_output"`

	// Cost breakdown
	LLMCostUSD float64 `json:"llm_cost_usd"` // Actual cost from OpenRouter
	MarkupRate float64 `json:"markup_rate"`  // The rate applied (e.g., 0.25)
	MarkupUSD  float64 `json:"markup_usd"`   // LLMCostUSD * MarkupRate

	// LLM details
	LLMProvider  string `json:"llm_provider"`
	LLMModel     string `json:"llm_model"`
	GenerationID string `json:"generation_id,omitempty"` // OpenRouter generation ID
	BYOKProvider string `json:"byok_provider,omitempty"` // Provider if BYOK

	// Execution metrics
	PagesAttempted    int `json:"pages_attempted"`
	PagesSuccessful   int `json:"pages_successful"`
	FetchDurationMs   int `json:"fetch_duration_ms"`
	ExtractDurationMs int `json:"extract_duration_ms"`
	TotalDurationMs   int `json:"total_duration_ms"`

	// Request context
	RequestID string `json:"request_id,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	IPCountry string `json:"ip_country,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// CrawlConfig captures crawl-specific settings for insights.
type CrawlConfig struct {
	MaxDepth         int    `json:"max_depth"`
	MaxPages         int    `json:"max_pages"`
	MaxURLs          int    `json:"max_urls"`
	FollowSelector   string `json:"follow_selector,omitempty"`
	FollowPattern    string `json:"follow_pattern,omitempty"`
	NextSelector     string `json:"next_selector,omitempty"`
	Delay            string `json:"delay,omitempty"`
	Concurrency      int    `json:"concurrency"`
	SameDomainOnly   bool   `json:"same_domain_only"`
	ExtractFromSeeds bool   `json:"extract_from_seeds"`
}
