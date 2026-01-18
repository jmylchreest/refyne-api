package mw

import (
	"log/slog"
	"testing"
	"time"
)

// ========================================
// LogFiltersStats Tests
// ========================================

func TestLogFiltersStats_Fields(t *testing.T) {
	stats := LogFiltersStats{
		Initialized: true,
		FilterCount: 5,
		Etag:        "abc123",
		LastFetch:   "2024-01-15T10:30:00Z",
		LastCheck:   "2024-01-15T10:35:00Z",
		CacheTTL:    "5m0s",
		Bucket:      "test-bucket",
		Key:         "filters.json",
	}

	if !stats.Initialized {
		t.Error("Initialized should be true")
	}
	if stats.FilterCount != 5 {
		t.Errorf("FilterCount = %d, want 5", stats.FilterCount)
	}
	if stats.Etag != "abc123" {
		t.Errorf("Etag = %q, want %q", stats.Etag, "abc123")
	}
	if stats.LastFetch != "2024-01-15T10:30:00Z" {
		t.Errorf("LastFetch = %q, unexpected value", stats.LastFetch)
	}
	if stats.LastCheck != "2024-01-15T10:35:00Z" {
		t.Errorf("LastCheck = %q, unexpected value", stats.LastCheck)
	}
	if stats.CacheTTL != "5m0s" {
		t.Errorf("CacheTTL = %q, want %q", stats.CacheTTL, "5m0s")
	}
	if stats.Bucket != "test-bucket" {
		t.Errorf("Bucket = %q, want %q", stats.Bucket, "test-bucket")
	}
	if stats.Key != "filters.json" {
		t.Errorf("Key = %q, want %q", stats.Key, "filters.json")
	}
}

func TestLogFiltersStats_ZeroValue(t *testing.T) {
	var stats LogFiltersStats

	if stats.Initialized {
		t.Error("Initialized should be false by default")
	}
	if stats.FilterCount != 0 {
		t.Errorf("FilterCount = %d, want 0", stats.FilterCount)
	}
	if stats.Etag != "" {
		t.Errorf("Etag = %q, want empty", stats.Etag)
	}
}

// ========================================
// LogFiltersLoader Tests
// ========================================

func TestNewLogFiltersLoader_DefaultLogger(t *testing.T) {
	cfg := LogFiltersConfig{
		Logger: nil, // Should use slog.Default()
	}

	loader := NewLogFiltersLoader(cfg)
	if loader == nil {
		t.Fatal("expected loader, got nil")
	}
	if loader.logger == nil {
		t.Error("expected logger to be set")
	}
}

func TestNewLogFiltersLoader_WithLogger(t *testing.T) {
	logger := slog.Default()
	cfg := LogFiltersConfig{
		Logger: logger,
	}

	loader := NewLogFiltersLoader(cfg)
	if loader == nil {
		t.Fatal("expected loader, got nil")
	}
	if loader.logger != logger {
		t.Error("logger not set correctly")
	}
}

func TestNewLogFiltersLoader_DefaultCacheTTL(t *testing.T) {
	cfg := LogFiltersConfig{
		CacheTTL: 0, // Should default to 5 minutes
	}

	loader := NewLogFiltersLoader(cfg)
	if loader == nil {
		t.Fatal("expected loader, got nil")
	}
	if loader.cacheTTL != 5*time.Minute {
		t.Errorf("cacheTTL = %v, want %v", loader.cacheTTL, 5*time.Minute)
	}
}

func TestNewLogFiltersLoader_CustomCacheTTL(t *testing.T) {
	cfg := LogFiltersConfig{
		CacheTTL: 10 * time.Minute,
	}

	loader := NewLogFiltersLoader(cfg)
	if loader == nil {
		t.Fatal("expected loader, got nil")
	}
	if loader.cacheTTL != 10*time.Minute {
		t.Errorf("cacheTTL = %v, want %v", loader.cacheTTL, 10*time.Minute)
	}
}

func TestLogFiltersLoader_Stop(t *testing.T) {
	cfg := LogFiltersConfig{
		Logger: slog.Default(),
	}

	loader := NewLogFiltersLoader(cfg)

	// Stop without starting should not panic
	loader.Stop()
}

// Note: Full LogFiltersLoader tests require mocking S3 client.
// These tests verify construction and basic behavior without S3.
// The Start method checks loader.IsEnabled() and exits early if no S3 client.
