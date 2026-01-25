// Package service contains the business logic layer.
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/jmylchreest/refyne-api/internal/captcha"
	"github.com/jmylchreest/refyne/pkg/fetcher"
)

// DynamicFetcher implements fetcher.Fetcher using the captcha service for browser rendering.
// This fetcher uses a real browser to render JavaScript-heavy pages and handle anti-bot challenges.
type DynamicFetcher struct {
	captchaSvc *CaptchaService
	userID     string
	tier       string
	jobID      string
	logger     *slog.Logger
}

// DynamicFetcherConfig holds configuration for creating a DynamicFetcher.
type DynamicFetcherConfig struct {
	CaptchaSvc *CaptchaService
	UserID     string
	Tier       string
	JobID      string
	Logger     *slog.Logger
}

// NewDynamicFetcher creates a new DynamicFetcher that uses browser rendering.
func NewDynamicFetcher(cfg DynamicFetcherConfig) *DynamicFetcher {
	return &DynamicFetcher{
		captchaSvc: cfg.CaptchaSvc,
		userID:     cfg.UserID,
		tier:       cfg.Tier,
		jobID:      cfg.JobID,
		logger:     cfg.Logger,
	}
}

// Fetch retrieves page content using browser rendering via the captcha service.
func (f *DynamicFetcher) Fetch(ctx context.Context, url string, opts fetcher.Options) (fetcher.Content, error) {
	startTime := time.Now()

	// Convert fetcher cookies to captcha cookies
	var cookies []captcha.Cookie
	for _, c := range opts.Cookies {
		cookies = append(cookies, captcha.Cookie{
			Name:   c.Name,
			Value:  c.Value,
			Domain: c.Domain,
		})
	}

	// Calculate timeout in milliseconds
	timeoutMs := 60000 // Default 60 seconds
	if opts.Timeout > 0 {
		timeoutMs = int(opts.Timeout.Milliseconds())
	}

	f.logger.Info("fetching with browser rendering",
		"url", url,
		"user_id", f.userID,
		"job_id", f.jobID,
		"timeout_ms", timeoutMs,
	)

	// Call captcha service
	result, err := f.captchaSvc.FetchDynamicContent(ctx, f.userID, f.tier, CaptchaSolveInput{
		URL:        url,
		MaxTimeout: timeoutMs,
		Cookies:    cookies,
		JobID:      f.jobID,
	})
	if err != nil {
		f.logger.Error("browser rendering failed",
			"url", url,
			"user_id", f.userID,
			"job_id", f.jobID,
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fetcher.Content{}, err
	}

	// Check if solve was successful
	if result.Status != "ok" || result.Solution == nil {
		f.logger.Warn("browser rendering returned non-ok status",
			"url", url,
			"status", result.Status,
			"message", result.Message,
		)
		return fetcher.Content{}, fetcher.ErrAntiBot
	}

	f.logger.Info("browser rendering completed",
		"url", url,
		"user_id", f.userID,
		"job_id", f.jobID,
		"challenge_type", result.ChallengeType,
		"solved", result.Solved,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	// Extract links from the response if available
	var links []string
	// Note: The captcha service doesn't currently return links, but we can parse them from HTML if needed

	return fetcher.Content{
		URL:         result.Solution.URL,
		HTML:        result.Solution.Response,
		StatusCode:  result.Solution.Status,
		ContentType: "text/html",
		FetchedAt:   time.Now(),
		Links:       links,
	}, nil
}

// Close releases any resources. For DynamicFetcher, this is a no-op as the
// captcha service manages its own lifecycle.
func (f *DynamicFetcher) Close() error {
	return nil
}

// Type returns the fetcher type identifier.
func (f *DynamicFetcher) Type() string {
	return "dynamic"
}
