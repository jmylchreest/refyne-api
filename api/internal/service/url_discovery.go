package service

import (
	"context"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gocolly/colly/v2"
)

// URLDiscoveryOptions configures URL discovery behavior.
type URLDiscoveryOptions struct {
	// FollowSelector is a CSS selector to find links to follow.
	FollowSelector string

	// FollowPattern is a regex pattern to filter URLs.
	FollowPattern string

	// MaxPages limits the number of pages to discover.
	MaxPages int

	// MaxDepth limits how deep to crawl from the seed.
	MaxDepth int

	// MaxURLs limits total URLs to queue.
	MaxURLs int

	// SameDomainOnly restricts discovery to the same domain.
	SameDomainOnly bool

	// Delay between requests.
	Delay time.Duration

	// NextSelector is a CSS selector for pagination links.
	NextSelector string
}

// DiscoveredURL represents a URL found during discovery.
type DiscoveredURL struct {
	URL       string
	Depth     int
	ParentURL string
}

// URLDiscoverer handles URL discovery using Colly.
type URLDiscoverer struct {
	logger *slog.Logger
}

// NewURLDiscoverer creates a new URL discoverer.
func NewURLDiscoverer(logger *slog.Logger) *URLDiscoverer {
	return &URLDiscoverer{logger: logger}
}

// Discover finds URLs starting from the given seed URL(s).
// Returns discovered URLs respecting the configured limits.
func (d *URLDiscoverer) Discover(ctx context.Context, seedURLs []string, opts URLDiscoveryOptions) ([]DiscoveredURL, error) {
	if len(seedURLs) == 0 {
		return nil, nil
	}

	d.logger.Debug("URL discovery starting",
		"seed_count", len(seedURLs),
		"follow_selector", opts.FollowSelector,
		"follow_pattern", opts.FollowPattern,
		"next_selector", opts.NextSelector,
		"max_pages", opts.MaxPages,
		"max_depth", opts.MaxDepth,
		"same_domain_only", opts.SameDomainOnly,
	)

	// Parse seed URL for domain restriction
	var allowedDomain string
	if opts.SameDomainOnly && len(seedURLs) > 0 {
		parsedSeed, err := url.Parse(seedURLs[0])
		if err == nil {
			allowedDomain = parsedSeed.Host
		}
	}

	// Compile follow pattern if provided
	var followRegex *regexp.Regexp
	if opts.FollowPattern != "" {
		var err error
		followRegex, err = regexp.Compile(opts.FollowPattern)
		if err != nil {
			d.logger.Warn("invalid follow pattern, ignoring", "pattern", opts.FollowPattern, "error", err)
		}
	}

	// Track discovered URLs
	var mu sync.Mutex
	discovered := []DiscoveredURL{}
	seen := make(map[string]bool)
	depths := make(map[string]int)

	// Mark seed URLs
	for _, seedURL := range seedURLs {
		normalizedSeed := normalizeDiscoveredURL(seedURL)
		seen[normalizedSeed] = true
		depths[seedURL] = 0
		discovered = append(discovered, DiscoveredURL{
			URL:       seedURL,
			Depth:     0,
			ParentURL: "",
		})
	}

	// Set up limits
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = 100 // Default limit
	}
	maxDepth := opts.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 3 // Default depth
	}
	maxURLs := opts.MaxURLs
	if maxURLs <= 0 {
		maxURLs = maxPages * 2 // Default URL limit
	}

	// If only extracting seed URLs (maxDepth=0 or similar), return seeds
	if maxDepth == 0 {
		return discovered, nil
	}

	// Create Colly collector
	c := colly.NewCollector(
		colly.MaxDepth(maxDepth),
		colly.Async(true),
	)

	// Set delay if configured
	if opts.Delay > 0 {
		_ = c.Limit(&colly.LimitRule{
			Delay: opts.Delay,
		})
	}

	// Set allowed domains if same domain only
	if allowedDomain != "" {
		c.AllowedDomains = []string{allowedDomain}
	}

	// Determine which selector to use for links
	// Normalize selector: convert newline-separated selectors to comma-separated
	// CSS requires comma-separation, but users may provide newline-separated lists
	linkSelector := "a[href]"
	if opts.FollowSelector != "" {
		linkSelector = normalizeSelector(opts.FollowSelector)
	}

	d.logger.Debug("URL discovery configured",
		"link_selector", linkSelector,
		"allowed_domain", allowedDomain,
		"max_depth", maxDepth,
		"max_urls", maxURLs,
	)

	// Track links found for debugging
	var linksFound int
	var linksFiltered int

	// Handle link discovery
	c.OnHTML(linkSelector, func(e *colly.HTMLElement) {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return
		default:
		}

		href := e.Attr("href")
		if href == "" {
			return
		}

		mu.Lock()
		linksFound++
		mu.Unlock()

		// Resolve relative URLs
		absoluteURL := e.Request.AbsoluteURL(href)
		if absoluteURL == "" {
			mu.Lock()
			linksFiltered++
			mu.Unlock()
			return
		}

		// Normalize URL
		normalizedURL := normalizeDiscoveredURL(absoluteURL)

		// Apply follow pattern filter
		if followRegex != nil && !followRegex.MatchString(absoluteURL) {
			mu.Lock()
			linksFiltered++
			mu.Unlock()
			return
		}

		// Check domain restriction
		if allowedDomain != "" {
			parsedURL, err := url.Parse(absoluteURL)
			if err != nil || parsedURL.Host != allowedDomain {
				mu.Lock()
				linksFiltered++
				mu.Unlock()
				return
			}
		}

		mu.Lock()
		defer mu.Unlock()

		// Check if already seen
		if seen[normalizedURL] {
			return
		}

		// Check limits
		if len(discovered) >= maxURLs {
			return
		}

		// Calculate depth
		parentURL := e.Request.URL.String()
		depth := depths[parentURL] + 1
		if depth > maxDepth {
			return
		}

		seen[normalizedURL] = true
		depths[absoluteURL] = depth
		discovered = append(discovered, DiscoveredURL{
			URL:       absoluteURL,
			Depth:     depth,
			ParentURL: parentURL,
		})

		d.logger.Debug("URL discovered",
			"url", absoluteURL,
			"depth", depth,
			"parent", parentURL,
			"total_discovered", len(discovered),
		)

		// Continue crawling if we haven't hit page limit
		// Use collector.Visit for proper async queueing
		if len(discovered) < maxURLs && depth < maxDepth {
			go func(url string) {
				_ = c.Visit(url)
			}(absoluteURL)
		}
	})

	// Handle pagination (next page links)
	if opts.NextSelector != "" {
		c.OnHTML(opts.NextSelector, func(e *colly.HTMLElement) {
			select {
			case <-ctx.Done():
				return
			default:
			}

			href := e.Attr("href")
			if href == "" {
				return
			}

			absoluteURL := e.Request.AbsoluteURL(href)
			if absoluteURL == "" {
				return
			}

			normalizedURL := normalizeDiscoveredURL(absoluteURL)

			mu.Lock()
			if !seen[normalizedURL] && len(discovered) < maxURLs {
				seen[normalizedURL] = true
				parentURL := e.Request.URL.String()
				depth := depths[parentURL] + 1
				depths[absoluteURL] = depth
				discovered = append(discovered, DiscoveredURL{
					URL:       absoluteURL,
					Depth:     depth,
					ParentURL: parentURL,
				})
				d.logger.Debug("pagination URL discovered",
					"url", absoluteURL,
					"depth", depth,
				)
				mu.Unlock()
				// Use collector.Visit for proper async queueing
				go func(url string) {
					_ = c.Visit(url)
				}(absoluteURL)
			} else {
				mu.Unlock()
			}
		})
	}

	// Log when pages are visited (for debugging)
	c.OnRequest(func(r *colly.Request) {
		d.logger.Debug("visiting page", "url", r.URL.String())
	})

	// Log responses (for debugging)
	c.OnResponse(func(r *colly.Response) {
		d.logger.Debug("page fetched",
			"url", r.Request.URL.String(),
			"status", r.StatusCode,
			"body_size", len(r.Body),
		)
	})

	// Handle errors
	c.OnError(func(r *colly.Response, err error) {
		d.logger.Warn("URL discovery error", "url", r.Request.URL.String(), "error", err)
	})

	// Start crawling from seed URLs
	for _, seedURL := range seedURLs {
		if err := c.Visit(seedURL); err != nil {
			d.logger.Debug("failed to visit seed URL", "url", seedURL, "error", err)
		}
	}

	// Wait for completion
	c.Wait()

	d.logger.Debug("URL discovery completed",
		"links_found", linksFound,
		"links_filtered", linksFiltered,
		"urls_discovered", len(discovered),
		"max_pages", maxPages,
	)

	// Enforce maxPages limit on final results
	if len(discovered) > maxPages {
		discovered = discovered[:maxPages]
	}

	return discovered, nil
}

// normalizeSelector converts newline-separated CSS selectors to comma-separated.
// Users may provide selectors like:
//
//	a[href*='/jobs/']
//	a[href*='/products/']
//
// But CSS requires comma separation: "a[href*='/jobs/'], a[href*='/products/']"
func normalizeSelector(selector string) string {
	// Split by newlines
	lines := strings.Split(selector, "\n")
	var selectors []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			selectors = append(selectors, trimmed)
		}
	}
	// Join with comma and space (standard CSS format)
	return strings.Join(selectors, ", ")
}

// normalizeDiscoveredURL normalizes a URL for deduplication.
func normalizeDiscoveredURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// Remove fragment
	parsed.Fragment = ""

	// Normalize scheme to lowercase
	parsed.Scheme = strings.ToLower(parsed.Scheme)

	// Normalize host to lowercase
	parsed.Host = strings.ToLower(parsed.Host)

	// Remove trailing slash from path (unless it's just "/")
	if parsed.Path != "/" {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}

	return parsed.String()
}
