// Package constants defines centralized configuration for extraction operations.
package constants

import "time"

// Extraction retry and backoff configuration.
const (
	// MaxRetryAttempts is the maximum number of retry attempts per provider
	// before falling back to the next provider in the chain.
	MaxRetryAttempts = 3

	// InitialBackoff is the initial delay before the first retry.
	InitialBackoff = 2 * time.Second

	// MaxBackoff is the maximum delay between retries (caps exponential growth).
	MaxBackoff = 30 * time.Second

	// BackoffMultiplier is the factor by which backoff increases after each retry.
	// With InitialBackoff=2s and multiplier=2: delays are 2s, 4s, 8s, etc.
	BackoffMultiplier = 2.0

	// RateLimitBackoff is a longer initial delay specifically for 429 rate limit errors.
	// Rate limits often require waiting longer than transient errors.
	RateLimitBackoff = 5 * time.Second

	// ProviderFallbackDelay is the brief delay before trying the next provider.
	// Allows the previous provider's rate limit to start recovering.
	ProviderFallbackDelay = 1 * time.Second
)

// Extraction concurrency limits.
const (
	// DefaultExtractConcurrency is the default number of concurrent extractions
	// within a single crawl job.
	DefaultExtractConcurrency = 3

	// MaxExtractConcurrency is the maximum allowed concurrency per job.
	MaxExtractConcurrency = 10
)

// Crawling behavior configuration.
const (
	// HonourRobotsTxt controls whether the crawler respects robots.txt directives.
	// When true, disallowed paths will not be crawled.
	// Currently disabled - Refyne is designed for intentional extraction where
	// users typically have permission or are extracting from their own sites.
	// Set to true to enable ethical crawling for general-purpose use cases.
	HonourRobotsTxt = false

	// HonourNoIndex controls whether the crawler respects meta robots noindex directives.
	// When true, pages with <meta name="robots" content="noindex"> will be skipped.
	// Currently disabled for same reasons as HonourRobotsTxt.
	HonourNoIndex = false

	// HonourCrawlDelay controls whether the crawler respects Crawl-delay directives
	// in robots.txt. When true, delays specified by the site will be honoured.
	HonourCrawlDelay = false

	// DefaultCrawlDelay is the minimum delay between requests to the same domain
	// when not using robots.txt crawl-delay directive.
	DefaultCrawlDelay = 200 * time.Millisecond

	// MaxSitemapURLs limits how many URLs to process from a sitemap to prevent
	// runaway crawls on very large sitemaps.
	MaxSitemapURLs = 1000

	// SitemapFetchTimeout is the timeout for fetching and parsing sitemap.xml.
	SitemapFetchTimeout = 30 * time.Second
)

// ErrorVisibility determines what error information is visible to different user types.
type ErrorVisibility int

const (
	// ErrorVisibilityUser shows sanitized error messages (safe for all users).
	ErrorVisibilityUser ErrorVisibility = iota

	// ErrorVisibilityAdmin shows full error details including provider/model info.
	ErrorVisibilityAdmin
)

// ErrorCategory classifies errors for visibility and retry decisions.
type ErrorCategory string

const (
	// ErrorCategoryRateLimit indicates a 429 rate limit error - retryable with backoff.
	ErrorCategoryRateLimit ErrorCategory = "rate_limit"

	// ErrorCategoryModelUnsupported indicates the model doesn't support a feature
	// (e.g., response_format) - not retryable with same model, try next provider.
	ErrorCategoryModelUnsupported ErrorCategory = "model_unsupported"

	// ErrorCategoryInvalidKey indicates an authentication error - not retryable.
	ErrorCategoryInvalidKey ErrorCategory = "invalid_key"

	// ErrorCategoryQuotaExceeded indicates quota/credit exhaustion - not retryable.
	ErrorCategoryQuotaExceeded ErrorCategory = "quota_exceeded"

	// ErrorCategoryProviderError indicates a provider-side error - may be retryable.
	ErrorCategoryProviderError ErrorCategory = "provider_error"

	// ErrorCategoryContentTooLong indicates context length exceeded - not retryable.
	ErrorCategoryContentTooLong ErrorCategory = "content_too_long"

	// ErrorCategoryTimeout indicates request timeout - retryable.
	ErrorCategoryTimeout ErrorCategory = "timeout"

	// ErrorCategoryUnknown indicates an unclassified error.
	ErrorCategoryUnknown ErrorCategory = "unknown"
)

// IsRetryableCategory returns true if the error category is potentially retryable
// with the same provider (after backoff).
func IsRetryableCategory(category ErrorCategory) bool {
	switch category {
	case ErrorCategoryRateLimit, ErrorCategoryProviderError, ErrorCategoryTimeout:
		return true
	default:
		return false
	}
}

// ShouldFallbackCategory returns true if the error category suggests
// trying the next provider in the fallback chain.
func ShouldFallbackCategory(category ErrorCategory) bool {
	switch category {
	case ErrorCategoryRateLimit, ErrorCategoryModelUnsupported, ErrorCategoryProviderError, ErrorCategoryTimeout:
		return true
	default:
		return false
	}
}

// SanitizedErrorMessages maps error categories to user-friendly messages
// for non-BYOK users (hides provider/model details).
var SanitizedErrorMessages = map[ErrorCategory]string{
	ErrorCategoryRateLimit:        "The service is experiencing high demand. Please try again in a few minutes.",
	ErrorCategoryModelUnsupported: "The extraction service encountered a compatibility issue. Please try again.",
	ErrorCategoryInvalidKey:       "There was an authentication issue. Please contact support if this persists.",
	ErrorCategoryQuotaExceeded:    "Service quota has been reached. Please try again later or upgrade your plan.",
	ErrorCategoryProviderError:    "The extraction service encountered a temporary issue. Please try again.",
	ErrorCategoryContentTooLong:   "The content is too large to process. Try extracting from a smaller page.",
	ErrorCategoryTimeout:          "The request took too long. Please try again with a simpler schema.",
	ErrorCategoryUnknown:          "An unexpected error occurred. Please try again or contact support.",
}

// GetSanitizedErrorMessage returns a user-friendly error message for a category.
func GetSanitizedErrorMessage(category ErrorCategory) string {
	if msg, ok := SanitizedErrorMessages[category]; ok {
		return msg
	}
	return SanitizedErrorMessages[ErrorCategoryUnknown]
}
