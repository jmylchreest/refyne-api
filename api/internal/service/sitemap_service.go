// Package service provides sitemap URL discovery functionality.
package service

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/jmylchreest/refyne-api/internal/constants"
)

// SitemapService handles sitemap URL discovery.
type SitemapService struct {
	logger *slog.Logger
	client *http.Client
}

// NewSitemapService creates a new sitemap service.
func NewSitemapService(logger *slog.Logger) *SitemapService {
	return &SitemapService{
		logger: logger,
		client: &http.Client{
			Timeout: constants.SitemapFetchTimeout,
		},
	}
}

// SitemapURL represents a URL entry from a sitemap.
type SitemapURL struct {
	Loc        string  `xml:"loc"`
	LastMod    string  `xml:"lastmod,omitempty"`
	ChangeFreq string  `xml:"changefreq,omitempty"`
	Priority   float64 `xml:"priority,omitempty"`
}

// Sitemap represents a parsed sitemap.xml file.
type Sitemap struct {
	XMLName xml.Name     `xml:"urlset"`
	URLs    []SitemapURL `xml:"url"`
}

// SitemapIndex represents a sitemap index file.
type SitemapIndex struct {
	XMLName  xml.Name       `xml:"sitemapindex"`
	Sitemaps []SitemapEntry `xml:"sitemap"`
}

// SitemapEntry represents an entry in a sitemap index.
type SitemapEntry struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// DiscoverURLsFromSitemap fetches and parses sitemap.xml to discover URLs.
// It handles both regular sitemaps and sitemap indexes.
// The urlPattern (if provided) filters URLs - only matching URLs are returned.
func (s *SitemapService) DiscoverURLsFromSitemap(ctx context.Context, baseURL string, urlPattern string) ([]string, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// Build sitemap URL
	sitemapURL := fmt.Sprintf("%s://%s/sitemap.xml", parsedURL.Scheme, parsedURL.Host)

	s.logger.Debug("fetching sitemap",
		"url", sitemapURL,
		"base_url", baseURL,
	)

	urls, err := s.fetchSitemap(ctx, sitemapURL, urlPattern, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sitemap: %w", err)
	}

	s.logger.Info("discovered URLs from sitemap",
		"sitemap_url", sitemapURL,
		"url_count", len(urls),
	)

	return urls, nil
}

// fetchSitemap fetches and parses a sitemap, handling both regular sitemaps and indexes.
func (s *SitemapService) fetchSitemap(ctx context.Context, sitemapURL, urlPattern string, depth int) ([]string, error) {
	if depth > 2 {
		s.logger.Warn("sitemap recursion depth exceeded", "url", sitemapURL, "depth", depth)
		return nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Refyne/1.0 (+https://refyne.uk)")
	req.Header.Set("Accept", "application/xml, text/xml, */*")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sitemap: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sitemap returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read sitemap body: %w", err)
	}

	// Try parsing as sitemap index first
	var sitemapIndex SitemapIndex
	if err := xml.Unmarshal(body, &sitemapIndex); err == nil && len(sitemapIndex.Sitemaps) > 0 {
		s.logger.Debug("parsed as sitemap index",
			"sitemap_count", len(sitemapIndex.Sitemaps),
		)
		return s.fetchSitemapIndex(ctx, sitemapIndex, urlPattern, depth)
	}

	// Try parsing as regular sitemap
	var sitemap Sitemap
	if err := xml.Unmarshal(body, &sitemap); err != nil {
		return nil, fmt.Errorf("failed to parse sitemap XML: %w", err)
	}

	return s.filterURLs(sitemap.URLs, urlPattern), nil
}

// fetchSitemapIndex recursively fetches sitemaps from an index.
func (s *SitemapService) fetchSitemapIndex(ctx context.Context, index SitemapIndex, urlPattern string, depth int) ([]string, error) {
	var allURLs []string

	for _, entry := range index.Sitemaps {
		if len(allURLs) >= constants.MaxSitemapURLs {
			s.logger.Warn("reached max sitemap URLs limit",
				"limit", constants.MaxSitemapURLs,
				"total_found", len(allURLs),
			)
			break
		}

		urls, err := s.fetchSitemap(ctx, entry.Loc, urlPattern, depth+1)
		if err != nil {
			s.logger.Warn("failed to fetch nested sitemap",
				"url", entry.Loc,
				"error", err,
			)
			continue
		}

		allURLs = append(allURLs, urls...)
	}

	return allURLs, nil
}

// filterURLs filters sitemap URLs by a regex pattern and applies limits.
func (s *SitemapService) filterURLs(urls []SitemapURL, urlPattern string) []string {
	var result []string

	for _, u := range urls {
		if len(result) >= constants.MaxSitemapURLs {
			break
		}

		// Skip empty URLs
		if u.Loc == "" {
			continue
		}

		// Apply pattern filter if provided
		if urlPattern != "" && !matchesPattern(u.Loc, urlPattern) {
			continue
		}

		result = append(result, u.Loc)
	}

	return result
}

// matchesPattern checks if a URL matches any of the newline-separated patterns.
func matchesPattern(urlStr, pattern string) bool {
	patterns := strings.Split(pattern, "\n")
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(urlStr, p) {
			return true
		}
	}
	return false
}

// TrySitemapDiscovery attempts to discover URLs from sitemap.xml.
// Returns discovered URLs and a boolean indicating if discovery was successful.
// This is a convenience method that doesn't return an error if sitemap is unavailable.
func (s *SitemapService) TrySitemapDiscovery(ctx context.Context, baseURL string, urlPattern string) ([]string, bool) {
	ctx, cancel := context.WithTimeout(ctx, constants.SitemapFetchTimeout)
	defer cancel()

	urls, err := s.DiscoverURLsFromSitemap(ctx, baseURL, urlPattern)
	if err != nil {
		s.logger.Debug("sitemap discovery failed",
			"base_url", baseURL,
			"error", err,
		)
		return nil, false
	}

	if len(urls) == 0 {
		return nil, false
	}

	return urls, true
}

// GetSitemapURLs fetches the raw sitemap URLs without filtering.
// Useful for analysis and showing available URLs to users.
func (s *SitemapService) GetSitemapURLs(ctx context.Context, baseURL string, limit int) ([]SitemapURL, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	sitemapURL := fmt.Sprintf("%s://%s/sitemap.xml", parsedURL.Scheme, parsedURL.Host)

	ctx, cancel := context.WithTimeout(ctx, constants.SitemapFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Refyne/1.0 (+https://refyne.uk)")
	req.Header.Set("Accept", "application/xml, text/xml, */*")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sitemap: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sitemap returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read sitemap body: %w", err)
	}

	// Try parsing as sitemap index first
	var sitemapIndex SitemapIndex
	if err := xml.Unmarshal(body, &sitemapIndex); err == nil && len(sitemapIndex.Sitemaps) > 0 {
		// For index, fetch the first sitemap only
		if len(sitemapIndex.Sitemaps) > 0 {
			return s.fetchSitemapURLsDirect(ctx, sitemapIndex.Sitemaps[0].Loc, limit)
		}
	}

	// Parse as regular sitemap
	var sitemap Sitemap
	if err := xml.Unmarshal(body, &sitemap); err != nil {
		return nil, fmt.Errorf("failed to parse sitemap XML: %w", err)
	}

	if limit > 0 && len(sitemap.URLs) > limit {
		return sitemap.URLs[:limit], nil
	}

	return sitemap.URLs, nil
}

func (s *SitemapService) fetchSitemapURLsDirect(ctx context.Context, sitemapURL string, limit int) ([]SitemapURL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Refyne/1.0 (+https://refyne.uk)")
	req.Header.Set("Accept", "application/xml, text/xml, */*")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sitemap: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sitemap returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read sitemap body: %w", err)
	}

	var sitemap Sitemap
	if err := xml.Unmarshal(body, &sitemap); err != nil {
		return nil, fmt.Errorf("failed to parse sitemap XML: %w", err)
	}

	if limit > 0 && len(sitemap.URLs) > limit {
		return sitemap.URLs[:limit], nil
	}

	return sitemap.URLs, nil
}
