package service

import (
	"context"
)

// PageExtractor defines the interface for extracting data from a single page.
// Implementations handle schema-based or prompt-based extraction with consistent
// dynamic retry behavior for bot protection and insufficient content.
type PageExtractor interface {
	// Extract fetches and extracts data from a single URL.
	// Returns result (may contain partial data on error) and any error.
	Extract(ctx context.Context, pageURL string) (*PageExtractionResult, error)
}

// PageExtractionResult contains the result of extracting a single page.
// Used by both schema and prompt extraction modes.
type PageExtractionResult struct {
	// URL is the final URL after any redirects.
	URL string

	// Data contains the extracted data (type depends on extraction mode).
	Data any

	// RawContent is the raw cleaned content (for debug capture).
	RawContent string

	// Token usage
	TokensInput  int
	TokensOutput int

	// Timing
	FetchDurationMs   int
	ExtractDurationMs int

	// LLM info
	Provider     string
	Model        string
	GenerationID string // For cost tracking via OpenRouter

	// Error info (may be set even with partial Data)
	Error         error
	ErrorCategory string // "fetch_error", "llm_error", "parse_error", "config_error"

	// Retry info
	UsedDynamicMode bool // True if browser rendering was used
	RetryCount      int  // Number of retries attempted
}

// SchemaExtractorOptions configures a SchemaPageExtractor.
type SchemaExtractorOptions struct {
	// LLMConfig is the LLM configuration to use.
	LLMConfig *LLMConfigInput

	// CleanerChain is the content cleaner chain configuration.
	CleanerChain []CleanerConfig

	// ContentDynamicAllowed indicates if browser rendering is allowed.
	ContentDynamicAllowed bool

	// UserID is for logging and tracking.
	UserID string

	// Tier is the user's subscription tier.
	Tier string

	// JobID is for tracking in browser service.
	JobID string
}

// PromptExtractorOptions configures a PromptPageExtractor.
type PromptExtractorOptions struct {
	// PromptText is the freeform extraction prompt.
	PromptText string

	// LLMConfig is the LLM configuration to use.
	LLMConfig *LLMConfigInput

	// CleanerChain is the content cleaner chain configuration.
	CleanerChain []CleanerConfig

	// IsBYOK indicates if using user's own API keys.
	IsBYOK bool

	// ContentDynamicAllowed indicates if browser rendering is allowed.
	ContentDynamicAllowed bool

	// UserID is for logging and tracking.
	UserID string

	// Tier is the user's subscription tier.
	Tier string

	// JobID is for tracking in browser service.
	JobID string
}
