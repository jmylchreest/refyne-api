package worker

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// ========================================
// Config Tests
// ========================================

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		PollInterval: 10 * time.Second,
		Concurrency:  5,
	}

	if cfg.PollInterval != 10*time.Second {
		t.Errorf("PollInterval = %v, want 10s", cfg.PollInterval)
	}
	if cfg.Concurrency != 5 {
		t.Errorf("Concurrency = %d, want 5", cfg.Concurrency)
	}
}

func TestConfig_ZeroValues(t *testing.T) {
	var cfg Config

	if cfg.PollInterval != 0 {
		t.Errorf("PollInterval = %v, want 0", cfg.PollInterval)
	}
	if cfg.Concurrency != 0 {
		t.Errorf("Concurrency = %d, want 0", cfg.Concurrency)
	}
}

// ========================================
// New Worker Tests
// ========================================

func TestNew_Defaults(t *testing.T) {
	cfg := Config{} // Zero values

	w := New(nil, nil, nil, nil, nil, nil, cfg, nil)

	if w == nil {
		t.Fatal("expected worker, got nil")
	}
	// Check defaults were applied
	if w.pollInterval != 5*time.Second {
		t.Errorf("pollInterval = %v, want 5s (default)", w.pollInterval)
	}
	if w.concurrency != 3 {
		t.Errorf("concurrency = %d, want 3 (default)", w.concurrency)
	}
	if w.logger == nil {
		t.Error("logger should be set to default")
	}
}

func TestNew_CustomConfig(t *testing.T) {
	cfg := Config{
		PollInterval: 10 * time.Second,
		Concurrency:  8,
	}
	logger := slog.Default()

	w := New(nil, nil, nil, nil, nil, nil, cfg, logger)

	if w == nil {
		t.Fatal("expected worker, got nil")
	}
	if w.pollInterval != 10*time.Second {
		t.Errorf("pollInterval = %v, want 10s", w.pollInterval)
	}
	if w.concurrency != 8 {
		t.Errorf("concurrency = %d, want 8", w.concurrency)
	}
}

func TestNew_PartialDefaults(t *testing.T) {
	// Only set PollInterval, Concurrency should use default
	cfg := Config{
		PollInterval: 15 * time.Second,
	}

	w := New(nil, nil, nil, nil, nil, nil, cfg, nil)

	if w.pollInterval != 15*time.Second {
		t.Errorf("pollInterval = %v, want 15s", w.pollInterval)
	}
	if w.concurrency != 3 {
		t.Errorf("concurrency = %d, want 3 (default)", w.concurrency)
	}
}

// ========================================
// Start/Stop Tests
// ========================================

func TestWorker_StartStop(t *testing.T) {
	cfg := Config{
		PollInterval: 50 * time.Millisecond, // Short for testing
		Concurrency:  2,
	}

	w := New(nil, nil, nil, nil, nil, nil, cfg, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start should not block
	w.Start(ctx)

	// Give workers time to start
	time.Sleep(10 * time.Millisecond)

	// Stop should complete without hanging
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Stop() timed out")
	}
}

func TestWorker_StopViaContext(t *testing.T) {
	cfg := Config{
		PollInterval: 50 * time.Millisecond,
		Concurrency:  1,
	}

	w := New(nil, nil, nil, nil, nil, nil, cfg, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())

	w.Start(ctx)

	// Cancel context should cause workers to stop
	cancel()

	// Give workers time to exit
	time.Sleep(100 * time.Millisecond)

	// Stop should complete quickly since workers already exited
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Error("Stop() timed out after context cancellation")
	}
}

// ========================================
// Worker Struct Tests
// ========================================

func TestWorker_Fields(t *testing.T) {
	cfg := Config{
		PollInterval: 5 * time.Second,
		Concurrency:  4,
	}
	logger := slog.Default()

	w := New(nil, nil, nil, nil, nil, nil, cfg, logger)

	// Verify all fields are set
	if w.stop == nil {
		t.Error("stop channel should be initialized")
	}
}

// Note: Full worker testing with job processing requires:
// - Mock JobRepository with ClaimPending
// - Mock JobResultRepository
// - Mock ExtractionService with Extract/CrawlWithCallback
// - Mock WebhookService
// - Mock StorageService
//
// The tests above verify:
// - Config handling and defaults
// - Worker construction
// - Clean start/stop lifecycle
// - Context-based cancellation
//
// Integration tests with real repositories would provide more comprehensive coverage.
