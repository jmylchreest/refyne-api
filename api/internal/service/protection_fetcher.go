// Package service contains the business logic layer.
package service

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/jmylchreest/refyne/pkg/fetcher"

	"github.com/jmylchreest/refyne-api/internal/protection"
)

// ErrProtectionDetected is returned when bot protection is detected during fetch.
// The wrapped error provides details about the type of protection.
var ErrProtectionDetected = NewErrBotProtectionDetected("", "bot protection detected")

// ProtectionAwareFetcher wraps a standard Colly-based fetcher and checks responses
// for bot protection signals. When protection is detected, it returns a specific
// error that can trigger fallback to browser rendering.
type ProtectionAwareFetcher struct {
	detector *protection.Detector
	logger   *slog.Logger
}

// ProtectionAwareFetcherConfig holds configuration for creating a ProtectionAwareFetcher.
type ProtectionAwareFetcherConfig struct {
	Logger *slog.Logger
}

// NewProtectionAwareFetcher creates a new fetcher that detects bot protection.
func NewProtectionAwareFetcher(cfg ProtectionAwareFetcherConfig) *ProtectionAwareFetcher {
	return &ProtectionAwareFetcher{
		detector: protection.NewDetector(),
		logger:   cfg.Logger,
	}
}

// Fetch retrieves page content using Colly and checks for bot protection.
// Returns ErrProtectionDetected if the response appears to be a challenge page.
func (f *ProtectionAwareFetcher) Fetch(ctx context.Context, url string, opts fetcher.Options) (fetcher.Content, error) {
	var result fetcher.Content
	var statusCode int
	var rawBody []byte

	c := colly.NewCollector(
		colly.UserAgent(getDefaultUserAgent(opts)),
		colly.AllowURLRevisit(),
	)

	// Set timeout from options or default
	timeout := 30 * time.Second
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}
	c.SetRequestTimeout(timeout)

	// Apply cookies if provided
	if len(opts.Cookies) > 0 {
		var httpCookies []*http.Cookie
		for _, cookie := range opts.Cookies {
			httpCookies = append(httpCookies, &http.Cookie{
				Name:   cookie.Name,
				Value:  cookie.Value,
				Domain: cookie.Domain,
			})
		}
		if err := c.SetCookies(url, httpCookies); err != nil {
			f.logger.Warn("failed to set cookies", "error", err)
		}
	}

	// Capture response
	c.OnResponse(func(r *colly.Response) {
		statusCode = r.StatusCode
		rawBody = r.Body
		result = fetcher.Content{
			URL:         r.Request.URL.String(),
			HTML:        string(r.Body),
			StatusCode:  r.StatusCode,
			ContentType: r.Headers.Get("Content-Type"),
			FetchedAt:   time.Now(),
		}
	})

	// Extract links
	var links []string
	c.OnHTML("a[href]", func(e *colly.HTMLElement) {
		href := e.Attr("href")
		if href != "" && href[0] != '#' {
			absURL := e.Request.AbsoluteURL(href)
			if absURL != "" {
				links = append(links, absURL)
			}
		}
	})

	// Perform the fetch
	if err := c.Visit(url); err != nil {
		return fetcher.Content{}, err
	}

	result.Links = links

	// Check for bot protection signals
	detection := f.detector.DetectFromResponse(statusCode, nil, rawBody)
	if detection.Detected && detection.IsRetryable() {
		f.logger.Info("bot protection detected",
			"url", url,
			"signal", detection.Signal,
			"confidence", detection.Confidence,
		)
		return result, NewErrBotProtectionDetected(
			string(detection.Signal),
			detection.UserMessage(),
		)
	}

	return result, nil
}

// Close releases any resources. For ProtectionAwareFetcher, this is a no-op.
func (f *ProtectionAwareFetcher) Close() error {
	return nil
}

// Type returns the fetcher type identifier.
func (f *ProtectionAwareFetcher) Type() string {
	return "protection-aware"
}

// getDefaultUserAgent returns the user agent from options or a default.
func getDefaultUserAgent(opts fetcher.Options) string {
	if opts.UserAgent != "" {
		return opts.UserAgent
	}
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
}
