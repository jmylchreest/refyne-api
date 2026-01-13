package mw

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// IPBlocklist provides IP-based request blocking with S3-backed blocklist.
// Features:
// - Lazy loading: doesn't fetch until first request
// - Etag caching: only downloads when blocklist changes
// - Error backoff: waits before retrying on S3 errors
// - Fail open: allows requests if blocklist unavailable
type IPBlocklist struct {
	s3Client *s3.Client
	bucket   string
	key      string

	mu            sync.RWMutex
	blocked       map[string]bool   // exact IP matches
	blockedCIDRs  []*net.IPNet      // CIDR ranges
	etag          string
	lastCheck     time.Time
	lastError     time.Time
	initialized   bool
	cacheTTL      time.Duration
	errorBackoff  time.Duration
	logger        *slog.Logger
}

// BlocklistConfig holds configuration for the IP blocklist.
type BlocklistConfig struct {
	S3Client     *s3.Client
	Bucket       string
	Key          string
	CacheTTL     time.Duration // How often to check for updates (default: 5 min)
	ErrorBackoff time.Duration // How long to wait after an error (default: 1 min)
	Logger       *slog.Logger
}

// NewIPBlocklist creates a new IP blocklist middleware.
// The blocklist is lazy-loaded on first request.
func NewIPBlocklist(cfg BlocklistConfig) *IPBlocklist {
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 5 * time.Minute
	}
	if cfg.ErrorBackoff == 0 {
		cfg.ErrorBackoff = 1 * time.Minute
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &IPBlocklist{
		s3Client:     cfg.S3Client,
		bucket:       cfg.Bucket,
		key:          cfg.Key,
		blocked:      make(map[string]bool),
		blockedCIDRs: make([]*net.IPNet, 0),
		cacheTTL:     cfg.CacheTTL,
		errorBackoff: cfg.ErrorBackoff,
		logger:       cfg.Logger,
	}
}

// Middleware returns the HTTP middleware handler.
func (b *IPBlocklist) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip if no S3 client configured (blocklist disabled)
			if b.s3Client == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Lazy load / refresh blocklist (non-blocking on error)
			b.maybeRefresh(r.Context())

			// Check if IP is blocked
			clientIP := extractIP(r)
			if b.isBlocked(clientIP) {
				b.logger.Warn("blocked request from blocklisted IP",
					"ip", clientIP,
					"path", r.URL.Path,
				)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// maybeRefresh checks if we need to refresh the blocklist from S3.
// It's non-blocking and fails open on errors.
func (b *IPBlocklist) maybeRefresh(ctx context.Context) {
	b.mu.RLock()
	needsRefresh := !b.initialized || time.Since(b.lastCheck) > b.cacheTTL
	inErrorBackoff := !b.lastError.IsZero() && time.Since(b.lastError) < b.errorBackoff
	b.mu.RUnlock()

	if !needsRefresh || inErrorBackoff {
		return
	}

	// Refresh in background to not block requests
	go b.refresh(ctx)
}

// refresh fetches the blocklist from S3.
func (b *IPBlocklist) refresh(ctx context.Context) {
	b.mu.Lock()
	// Double-check after acquiring lock
	if b.initialized && time.Since(b.lastCheck) < b.cacheTTL {
		b.mu.Unlock()
		return
	}
	currentEtag := b.etag
	b.mu.Unlock()

	// Build request with conditional fetch
	input := &s3.GetObjectInput{
		Bucket: &b.bucket,
		Key:    &b.key,
	}
	if currentEtag != "" {
		input.IfNoneMatch = &currentEtag
	}

	resp, err := b.s3Client.GetObject(ctx, input)
	if err != nil {
		// Check for 304 Not Modified
		var apiErr *types.NoSuchKey
		if errors.As(err, &apiErr) {
			// Blocklist file doesn't exist - that's OK, just mark as checked
			b.mu.Lock()
			b.initialized = true
			b.lastCheck = time.Now()
			b.lastError = time.Now() // Backoff before checking again
			b.mu.Unlock()
			b.logger.Debug("blocklist file not found in S3, will retry later",
				"bucket", b.bucket,
				"key", b.key,
			)
			return
		}

		// Check for 304 Not Modified (etag match)
		var notModified interface{ ErrorCode() string }
		if errors.As(err, &notModified) && notModified.ErrorCode() == "NotModified" {
			b.mu.Lock()
			b.lastCheck = time.Now()
			b.mu.Unlock()
			return
		}

		// Other error - log and backoff
		b.mu.Lock()
		b.lastError = time.Now()
		b.initialized = true // Don't keep blocking on init
		b.mu.Unlock()
		b.logger.Error("failed to fetch blocklist from S3",
			"error", err,
			"bucket", b.bucket,
			"key", b.key,
		)
		return
	}
	defer resp.Body.Close()

	// Parse blocklist
	var entries []string
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		b.mu.Lock()
		b.lastError = time.Now()
		b.initialized = true
		b.mu.Unlock()
		b.logger.Error("failed to parse blocklist JSON", "error", err)
		return
	}

	// Build lookup structures
	blocked := make(map[string]bool)
	var cidrs []*net.IPNet

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Check if it's a CIDR range
		if strings.Contains(entry, "/") {
			_, ipNet, err := net.ParseCIDR(entry)
			if err != nil {
				b.logger.Warn("invalid CIDR in blocklist", "entry", entry, "error", err)
				continue
			}
			cidrs = append(cidrs, ipNet)
		} else {
			// Exact IP match
			if ip := net.ParseIP(entry); ip != nil {
				blocked[ip.String()] = true
			} else {
				b.logger.Warn("invalid IP in blocklist", "entry", entry)
			}
		}
	}

	// Update cache
	b.mu.Lock()
	b.blocked = blocked
	b.blockedCIDRs = cidrs
	b.initialized = true
	b.lastCheck = time.Now()
	b.lastError = time.Time{} // Clear error state
	if resp.ETag != nil {
		b.etag = *resp.ETag
	}
	b.mu.Unlock()

	b.logger.Info("blocklist refreshed",
		"exactIPs", len(blocked),
		"cidrRanges", len(cidrs),
	)
}

// isBlocked checks if an IP is in the blocklist.
func (b *IPBlocklist) isBlocked(ipStr string) bool {
	if ipStr == "" {
		return false
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	// Check exact match
	if b.blocked[ip.String()] {
		return true
	}

	// Check CIDR ranges
	for _, cidr := range b.blockedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
}

// extractIP gets the client IP from the request.
// Assumes middleware.RealIP has already been applied.
func extractIP(r *http.Request) string {
	// chi's RealIP middleware sets RemoteAddr to the real IP
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr might not have a port
		return r.RemoteAddr
	}
	return ip
}
