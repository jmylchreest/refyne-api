package mw

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ========================================
// extractIP Tests
// ========================================

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		expected   string
	}{
		{
			name:       "IP with port",
			remoteAddr: "192.168.1.1:8080",
			expected:   "192.168.1.1",
		},
		{
			name:       "IP without port",
			remoteAddr: "192.168.1.1",
			expected:   "192.168.1.1",
		},
		{
			name:       "IPv6 with port",
			remoteAddr: "[::1]:8080",
			expected:   "::1",
		},
		{
			name:       "IPv6 without port",
			remoteAddr: "::1",
			expected:   "::1",
		},
		{
			name:       "localhost",
			remoteAddr: "127.0.0.1:12345",
			expected:   "127.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr

			got := extractIP(req)
			if got != tt.expected {
				t.Errorf("extractIP() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// ========================================
// IPBlocklist.isBlocked Tests
// ========================================

func TestIPBlocklist_IsBlocked(t *testing.T) {
	b := &IPBlocklist{
		blocked:      make(map[string]bool),
		blockedCIDRs: make([]*net.IPNet, 0),
		logger:       slog.Default(),
	}

	// Add some blocked IPs and CIDRs
	b.blocked["192.168.1.100"] = true
	b.blocked["10.0.0.50"] = true
	b.blocked["::1"] = true

	_, cidr1, _ := net.ParseCIDR("172.16.0.0/16")
	_, cidr2, _ := net.ParseCIDR("192.168.2.0/24")
	b.blockedCIDRs = []*net.IPNet{cidr1, cidr2}

	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Empty/invalid
		{name: "empty string", ip: "", expected: false},
		{name: "invalid IP", ip: "not-an-ip", expected: false},

		// Exact matches
		{name: "exact match - blocked", ip: "192.168.1.100", expected: true},
		{name: "exact match - blocked 2", ip: "10.0.0.50", expected: true},
		{name: "exact match - not blocked", ip: "192.168.1.101", expected: false},
		{name: "exact match - IPv6 blocked", ip: "::1", expected: true},

		// CIDR matches
		{name: "CIDR match - 172.16.x.x", ip: "172.16.0.1", expected: true},
		{name: "CIDR match - 172.16.255.255", ip: "172.16.255.255", expected: true},
		{name: "CIDR no match - 172.17.0.1", ip: "172.17.0.1", expected: false},
		{name: "CIDR match - 192.168.2.x", ip: "192.168.2.50", expected: true},
		{name: "CIDR no match - 192.168.3.x", ip: "192.168.3.50", expected: false},

		// Not blocked
		{name: "not blocked - 8.8.8.8", ip: "8.8.8.8", expected: false},
		{name: "not blocked - localhost variant", ip: "127.0.0.1", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.isBlocked(tt.ip)
			if got != tt.expected {
				t.Errorf("isBlocked(%q) = %v, want %v", tt.ip, got, tt.expected)
			}
		})
	}
}

// ========================================
// IPBlocklist.ClearCache Tests
// ========================================

func TestIPBlocklist_ClearCache(t *testing.T) {
	b := &IPBlocklist{
		blocked:      make(map[string]bool),
		blockedCIDRs: make([]*net.IPNet, 0),
		logger:       slog.Default(),
	}

	// Add some data
	b.blocked["192.168.1.1"] = true
	_, cidr, _ := net.ParseCIDR("10.0.0.0/8")
	b.blockedCIDRs = append(b.blockedCIDRs, cidr)
	b.totalEntries = 2

	// Verify data exists
	if !b.isBlocked("192.168.1.1") {
		t.Fatal("expected IP to be blocked before clear")
	}

	// Clear cache
	b.ClearCache()

	// Verify data is cleared
	if b.isBlocked("192.168.1.1") {
		t.Error("expected IP to not be blocked after clear")
	}
	if len(b.blocked) != 0 {
		t.Errorf("blocked map has %d entries, want 0", len(b.blocked))
	}
	if len(b.blockedCIDRs) != 0 {
		t.Errorf("blockedCIDRs has %d entries, want 0", len(b.blockedCIDRs))
	}
	if b.totalEntries != 0 {
		t.Errorf("totalEntries = %d, want 0", b.totalEntries)
	}
}

// ========================================
// IPBlocklist.Stats Tests
// ========================================

func TestIPBlocklist_Stats_Empty(t *testing.T) {
	// Create blocklist with disabled loader (no S3 config)
	b := &IPBlocklist{
		loader:       nil, // Will need a mock or minimal loader for full test
		blocked:      make(map[string]bool),
		blockedCIDRs: make([]*net.IPNet, 0),
		logger:       slog.Default(),
	}

	// Add some data for stats
	b.blocked["192.168.1.1"] = true
	b.blocked["192.168.1.2"] = true
	_, cidr, _ := net.ParseCIDR("10.0.0.0/8")
	b.blockedCIDRs = append(b.blockedCIDRs, cidr)
	b.totalEntries = 3

	b.mu.RLock()
	exactIPs := len(b.blocked)
	cidrRanges := len(b.blockedCIDRs)
	total := b.totalEntries
	b.mu.RUnlock()

	if exactIPs != 2 {
		t.Errorf("exactIPs = %d, want 2", exactIPs)
	}
	if cidrRanges != 1 {
		t.Errorf("cidrRanges = %d, want 1", cidrRanges)
	}
	if total != 3 {
		t.Errorf("totalEntries = %d, want 3", total)
	}
}

// ========================================
// BlocklistStats Tests
// ========================================

func TestBlocklistStats_Fields(t *testing.T) {
	stats := BlocklistStats{
		Initialized:  true,
		TotalEntries: 100,
		ExactIPs:     75,
		CIDRRanges:   25,
		Etag:         "abc123",
		LastFetch:    "2024-01-15T10:30:00Z",
		LastCheck:    "2024-01-15T10:35:00Z",
		CacheTTL:     "5m0s",
		Bucket:       "test-bucket",
		Key:          "blocklist.json",
	}

	if !stats.Initialized {
		t.Error("Initialized should be true")
	}
	if stats.TotalEntries != 100 {
		t.Errorf("TotalEntries = %d, want 100", stats.TotalEntries)
	}
	if stats.ExactIPs != 75 {
		t.Errorf("ExactIPs = %d, want 75", stats.ExactIPs)
	}
	if stats.CIDRRanges != 25 {
		t.Errorf("CIDRRanges = %d, want 25", stats.CIDRRanges)
	}
	if stats.Etag != "abc123" {
		t.Errorf("Etag = %q, want %q", stats.Etag, "abc123")
	}
	if stats.Bucket != "test-bucket" {
		t.Errorf("Bucket = %q, want %q", stats.Bucket, "test-bucket")
	}
	if stats.Key != "blocklist.json" {
		t.Errorf("Key = %q, want %q", stats.Key, "blocklist.json")
	}
}

// ========================================
// Concurrent Access Tests
// ========================================

func TestIPBlocklist_ConcurrentAccess(t *testing.T) {
	b := &IPBlocklist{
		blocked:      make(map[string]bool),
		blockedCIDRs: make([]*net.IPNet, 0),
		logger:       slog.Default(),
	}

	// Populate initial data
	b.blocked["192.168.1.1"] = true
	_, cidr, _ := net.ParseCIDR("10.0.0.0/8")
	b.blockedCIDRs = append(b.blockedCIDRs, cidr)

	// Run concurrent reads and writes
	done := make(chan bool)

	// Readers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = b.isBlocked("192.168.1.1")
				_ = b.isBlocked("10.0.0.50")
				_ = b.isBlocked("8.8.8.8")
			}
			done <- true
		}()
	}

	// Writer (clearing cache)
	go func() {
		for i := 0; i < 10; i++ {
			b.ClearCache()
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 11; i++ {
		<-done
	}
}
