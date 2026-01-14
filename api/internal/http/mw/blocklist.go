package mw

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/jmylchreest/refyne-api/internal/config"
)

// IPBlocklist provides IP-based request blocking with S3-backed blocklist.
// Features:
// - Lazy loading: doesn't fetch until first request
// - Etag caching: only downloads when blocklist changes
// - Error backoff: waits before retrying on S3 errors
// - Fail open: allows requests if blocklist unavailable
// - Optimized lookups: O(1) for exact IPs, sorted CIDRs for early exit
type IPBlocklist struct {
	loader *config.S3Loader

	mu           sync.RWMutex
	blocked      map[string]bool // exact IP matches (O(1) lookup)
	blockedCIDRs []*net.IPNet    // CIDR ranges sorted by prefix length (most specific first)
	totalEntries int             // total entries in blocklist
	logger       *slog.Logger
}

// BlocklistConfig holds configuration for the IP blocklist.
type BlocklistConfig = config.S3LoaderConfig

// NewIPBlocklist creates a new IP blocklist middleware.
// The blocklist is lazy-loaded on first request.
func NewIPBlocklist(cfg BlocklistConfig) *IPBlocklist {
	return &IPBlocklist{
		loader:       config.NewS3Loader(cfg),
		blocked:      make(map[string]bool),
		blockedCIDRs: make([]*net.IPNet, 0),
		logger:       cfg.Logger,
	}
}

// Middleware returns the HTTP middleware handler.
func (b *IPBlocklist) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip if S3 not configured (blocklist disabled)
			if !b.loader.IsEnabled() {
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
	if !b.loader.NeedsRefresh() {
		return
	}

	// Refresh in background to not block requests
	// Use a detached context since the request context will be canceled
	// when the HTTP request completes, but the refresh should continue.
	go b.refresh(context.WithoutCancel(ctx))
}

// refresh fetches the blocklist from S3 and parses it.
func (b *IPBlocklist) refresh(ctx context.Context) {
	result, err := b.loader.Fetch(ctx)
	if err != nil {
		// S3Loader already logged the error
		return
	}
	if result == nil || result.NotChanged {
		// Not modified or S3 not configured
		return
	}

	// Parse blocklist entries from JSON
	var entries []string
	if err := json.Unmarshal(result.Data, &entries); err != nil {
		b.logger.Error("failed to parse blocklist JSON", "error", err)
		return
	}

	// Build lookup structures
	blocked := make(map[string]bool)
	var cidrs []*net.IPNet
	var invalidCount int

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
				invalidCount++
				continue
			}
			cidrs = append(cidrs, ipNet)
		} else {
			// Exact IP match
			if ip := net.ParseIP(entry); ip != nil {
				blocked[ip.String()] = true
			} else {
				b.logger.Warn("invalid IP in blocklist", "entry", entry)
				invalidCount++
			}
		}
	}

	// Sort CIDRs by prefix length (most specific/largest prefix first)
	// This allows early exit when checking - more specific ranges checked first
	sort.Slice(cidrs, func(i, j int) bool {
		onesI, _ := cidrs[i].Mask.Size()
		onesJ, _ := cidrs[j].Mask.Size()
		return onesI > onesJ // larger prefix = more specific = first
	})

	// Update cache
	b.mu.Lock()
	b.blocked = blocked
	b.blockedCIDRs = cidrs
	b.totalEntries = len(blocked) + len(cidrs)
	b.mu.Unlock()

	// Log detailed info about the refresh
	stats := b.loader.Stats()
	b.logger.Info("blocklist loaded from S3",
		"bucket", stats.Bucket,
		"key", stats.Key,
		"etag", stats.Etag,
		"exact_ips", len(blocked),
		"cidr_ranges", len(cidrs),
		"total_entries", len(blocked)+len(cidrs),
		"invalid_entries", invalidCount,
	)
}

// Refresh forces an immediate refresh of the blocklist.
// This can be called to manually trigger a reload.
func (b *IPBlocklist) Refresh(ctx context.Context) {
	b.refresh(ctx)
}

// ClearCache clears the cached blocklist and forces a refresh on next request.
func (b *IPBlocklist) ClearCache() {
	b.mu.Lock()
	b.blocked = make(map[string]bool)
	b.blockedCIDRs = make([]*net.IPNet, 0)
	b.totalEntries = 0
	b.mu.Unlock()
	// Note: S3Loader doesn't expose a ClearCache method, but NeedsRefresh
	// will return true after cacheTTL expires naturally
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

// BlocklistStats contains statistics about the blocklist for observability.
type BlocklistStats struct {
	Initialized  bool   `json:"initialized"`
	TotalEntries int    `json:"total_entries"`
	ExactIPs     int    `json:"exact_ips"`
	CIDRRanges   int    `json:"cidr_ranges"`
	Etag         string `json:"etag"`
	LastFetch    string `json:"last_fetch"`
	LastCheck    string `json:"last_check"`
	CacheTTL     string `json:"cache_ttl"`
	Bucket       string `json:"bucket"`
	Key          string `json:"key"`
}

// Stats returns current blocklist statistics.
func (b *IPBlocklist) Stats() BlocklistStats {
	b.mu.RLock()
	exactIPs := len(b.blocked)
	cidrRanges := len(b.blockedCIDRs)
	totalEntries := b.totalEntries
	b.mu.RUnlock()

	loaderStats := b.loader.Stats()

	lastFetch := ""
	if !loaderStats.LastFetch.IsZero() {
		lastFetch = loaderStats.LastFetch.Format("2006-01-02T15:04:05Z")
	}
	lastCheck := ""
	if !loaderStats.LastCheck.IsZero() {
		lastCheck = loaderStats.LastCheck.Format("2006-01-02T15:04:05Z")
	}

	return BlocklistStats{
		Initialized:  loaderStats.Initialized,
		TotalEntries: totalEntries,
		ExactIPs:     exactIPs,
		CIDRRanges:   cidrRanges,
		Etag:         loaderStats.Etag,
		LastFetch:    lastFetch,
		LastCheck:    lastCheck,
		CacheTTL:     loaderStats.CacheTTL,
		Bucket:       loaderStats.Bucket,
		Key:          loaderStats.Key,
	}
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
