// Package protection provides detection of bot protection and anti-scraping measures.
package protection

import (
	"net/http"
	"regexp"
	"strings"
)

// SignalType identifies the type of protection detected.
type SignalType string

const (
	SignalNone              SignalType = ""
	SignalCloudflare        SignalType = "cloudflare"
	SignalCaptcha           SignalType = "captcha"
	SignalAccessDenied      SignalType = "access_denied"
	SignalRateLimited       SignalType = "rate_limited"
	SignalEmptyContent      SignalType = "empty_content"
	SignalJavaScriptRequired SignalType = "javascript_required"
)

// DetectionResult contains the result of protection detection.
type DetectionResult struct {
	// Detected is true if any protection signal was found.
	Detected bool

	// Signal identifies the type of protection detected.
	Signal SignalType

	// Confidence is a score from 0-100 indicating detection confidence.
	Confidence int

	// Description provides a human-readable explanation.
	Description string

	// SuggestDynamic is true if browser rendering would likely help.
	SuggestDynamic bool
}

// Detector analyzes HTTP responses for bot protection signals.
type Detector struct {
	// MinContentLength is the minimum expected content length for a real page.
	// Responses shorter than this may indicate a challenge page.
	MinContentLength int
}

// NewDetector creates a new protection detector with default settings.
func NewDetector() *Detector {
	return &Detector{
		MinContentLength: 500,
	}
}

// DetectFromResponse analyzes an HTTP response for protection signals.
func (d *Detector) DetectFromResponse(statusCode int, headers http.Header, body []byte) DetectionResult {
	// Check status code first
	if result := d.checkStatusCode(statusCode); result.Detected {
		return result
	}

	// Check headers for protection signals
	if result := d.checkHeaders(headers); result.Detected {
		return result
	}

	// Check body content for protection patterns
	if result := d.checkBodyContent(body); result.Detected {
		return result
	}

	return DetectionResult{Detected: false}
}

// DetectFromContent analyzes page content directly (when headers aren't available).
func (d *Detector) DetectFromContent(statusCode int, content string) DetectionResult {
	return d.DetectFromResponse(statusCode, nil, []byte(content))
}

// checkStatusCode checks for protection based on HTTP status code.
func (d *Detector) checkStatusCode(statusCode int) DetectionResult {
	switch statusCode {
	case http.StatusForbidden: // 403
		return DetectionResult{
			Detected:       true,
			Signal:         SignalAccessDenied,
			Confidence:     90,
			Description:    "Access denied (HTTP 403) - site may be blocking automated requests",
			SuggestDynamic: true,
		}
	case http.StatusServiceUnavailable: // 503
		return DetectionResult{
			Detected:       true,
			Signal:         SignalCloudflare,
			Confidence:     70,
			Description:    "Service unavailable (HTTP 503) - may indicate Cloudflare or similar challenge",
			SuggestDynamic: true,
		}
	case http.StatusTooManyRequests: // 429
		return DetectionResult{
			Detected:       true,
			Signal:         SignalRateLimited,
			Confidence:     95,
			Description:    "Rate limited (HTTP 429) - too many requests",
			SuggestDynamic: false, // Rate limiting won't be solved by browser rendering
		}
	}
	return DetectionResult{Detected: false}
}

// checkHeaders checks response headers for protection signals.
func (d *Detector) checkHeaders(headers http.Header) DetectionResult {
	if headers == nil {
		return DetectionResult{Detected: false}
	}

	// Cloudflare headers
	if cf := headers.Get("cf-ray"); cf != "" {
		if headers.Get("cf-mitigated") == "challenge" {
			return DetectionResult{
				Detected:       true,
				Signal:         SignalCloudflare,
				Confidence:     95,
				Description:    "Cloudflare challenge detected",
				SuggestDynamic: true,
			}
		}
	}

	// Note: Server header containing "cloudflare" alone doesn't indicate blocking.
	// We rely on content detection for actual challenges (handled by DetectFromContent).

	return DetectionResult{Detected: false}
}

// Compiled regex patterns for efficiency
var (
	// Cloudflare patterns
	cloudflarePatterns = []string{
		"cf-browser-verification",
		"challenge-platform",
		"cf_chl_opt",
		"_cf_chl",
		"Checking your browser",
		"Please Wait... | Cloudflare",
		"Just a moment...",
		"Attention Required! | Cloudflare",
		"ray ID:",
	}

	// Captcha patterns
	captchaPatterns = []string{
		"g-recaptcha",
		"grecaptcha",
		"h-captcha",
		"hcaptcha",
		"data-sitekey",
		"captcha-container",
		"turnstile",
		"cf-turnstile",
	}

	// Access denied patterns
	accessDeniedPatterns = []string{
		"Access Denied",
		"Access to this page has been denied",
		"You don't have permission",
		"Request blocked",
		"Forbidden",
		"Bot detected",
		"automated access",
		"Please verify you are human",
		"are you a robot",
		"prove you're not a robot",
	}

	// JavaScript required patterns (SPA without content)
	jsRequiredPatterns = []string{
		"enable JavaScript",
		"JavaScript is required",
		"requires JavaScript",
		"Please enable JavaScript",
		"This site requires JavaScript",
		"<noscript>",
	}

	// Pattern to check if page has meaningful content
	contentIndicatorRegex = regexp.MustCompile(`<(article|main|section|div[^>]*class[^>]*content)[^>]*>`)
)

// checkBodyContent analyzes response body for protection signals.
func (d *Detector) checkBodyContent(body []byte) DetectionResult {
	if len(body) == 0 {
		return DetectionResult{
			Detected:       true,
			Signal:         SignalEmptyContent,
			Confidence:     80,
			Description:    "Empty response body - may indicate blocked request",
			SuggestDynamic: true,
		}
	}

	content := string(body)
	contentLower := strings.ToLower(content)

	// Check for Cloudflare challenge
	for _, pattern := range cloudflarePatterns {
		if strings.Contains(contentLower, strings.ToLower(pattern)) {
			return DetectionResult{
				Detected:       true,
				Signal:         SignalCloudflare,
				Confidence:     90,
				Description:    "Cloudflare challenge page detected",
				SuggestDynamic: true,
			}
		}
	}

	// Check for captcha
	for _, pattern := range captchaPatterns {
		if strings.Contains(contentLower, strings.ToLower(pattern)) {
			return DetectionResult{
				Detected:       true,
				Signal:         SignalCaptcha,
				Confidence:     95,
				Description:    "Captcha challenge detected",
				SuggestDynamic: true,
			}
		}
	}

	// Check for access denied messages
	for _, pattern := range accessDeniedPatterns {
		if strings.Contains(contentLower, strings.ToLower(pattern)) {
			return DetectionResult{
				Detected:       true,
				Signal:         SignalAccessDenied,
				Confidence:     85,
				Description:    "Access denied message detected",
				SuggestDynamic: true,
			}
		}
	}

	// Check for JavaScript required messages
	for _, pattern := range jsRequiredPatterns {
		if strings.Contains(contentLower, strings.ToLower(pattern)) {
			return DetectionResult{
				Detected:       true,
				Signal:         SignalJavaScriptRequired,
				Confidence:     80,
				Description:    "Page requires JavaScript to render content",
				SuggestDynamic: true,
			}
		}
	}

	// Check for suspiciously small content
	if len(body) < d.MinContentLength {
		// Only flag as suspicious if it doesn't look like a real page
		if !contentIndicatorRegex.MatchString(content) {
			return DetectionResult{
				Detected:       true,
				Signal:         SignalEmptyContent,
				Confidence:     60,
				Description:    "Response too small - may be a challenge or error page",
				SuggestDynamic: true,
			}
		}
	}

	return DetectionResult{Detected: false}
}

// IsRetryable returns true if the protection might be bypassed with browser rendering.
func (r DetectionResult) IsRetryable() bool {
	return r.SuggestDynamic
}

// UserMessage returns a user-friendly message describing the detection.
func (r DetectionResult) UserMessage() string {
	if !r.Detected {
		return ""
	}

	switch r.Signal {
	case SignalCloudflare:
		return "This site uses Cloudflare protection. Browser Rendering can help bypass this."
	case SignalCaptcha:
		return "This site has a captcha challenge. Browser Rendering may be able to solve this."
	case SignalAccessDenied:
		return "This site is blocking automated requests. Browser Rendering can help."
	case SignalRateLimited:
		return "Request was rate limited. Please try again later."
	case SignalEmptyContent:
		return "The site returned minimal content. It may require Browser Rendering."
	case SignalJavaScriptRequired:
		return "This site requires JavaScript to render content. Browser Rendering is recommended."
	default:
		return "Bot protection detected. Browser Rendering is recommended."
	}
}
