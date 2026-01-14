package mw

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jmylchreest/slog-logfilter"

	"github.com/jmylchreest/refyne-api/internal/config"
)

// LogFiltersConfig holds configuration for the log filters loader.
type LogFiltersConfig = config.S3LoaderConfig

// LogFiltersLoader loads log filters from S3 and applies them to slog-logfilter.
// Features:
// - Lazy loading: doesn't fetch until Start() is called or first refresh
// - Etag caching: only downloads when filters change
// - Error backoff: waits before retrying on S3 errors
// - Fail safe: keeps existing filters if update fails
type LogFiltersLoader struct {
	loader *config.S3Loader

	mu          sync.RWMutex
	filterCount int
	logger      *slog.Logger
	cacheTTL    time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewLogFiltersLoader creates a new log filters loader.
func NewLogFiltersLoader(cfg LogFiltersConfig) *LogFiltersLoader {
	// S3Loader handles defaults for CacheTTL, ErrorBackoff, Logger
	cacheTTL := cfg.CacheTTL
	if cacheTTL == 0 {
		cacheTTL = 5 * time.Minute
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &LogFiltersLoader{
		loader:   config.NewS3Loader(cfg),
		logger:   logger,
		cacheTTL: cacheTTL,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the periodic refresh of log filters.
// It immediately fetches the filters and then periodically checks for updates.
func (l *LogFiltersLoader) Start(ctx context.Context) {
	if !l.loader.IsEnabled() {
		l.logger.Info("log filters loader disabled (no S3 client)")
		return
	}

	// Initial fetch
	l.refresh(ctx)

	// Start periodic refresh
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		ticker := time.NewTicker(l.cacheTTL)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				l.refresh(context.Background())
			case <-l.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop stops the periodic refresh.
func (l *LogFiltersLoader) Stop() {
	close(l.stopCh)
	l.wg.Wait()
}

// refresh fetches the log filters from S3 and applies them.
func (l *LogFiltersLoader) refresh(ctx context.Context) {
	result, err := l.loader.Fetch(ctx)
	if err != nil {
		// S3Loader already logged the error
		return
	}
	if result == nil || result.NotChanged {
		// Not modified or S3 not configured
		return
	}

	// Parse filters
	var filters []logfilter.LogFilter
	if err := json.Unmarshal(result.Data, &filters); err != nil {
		l.logger.Error("failed to parse log filters JSON", "error", err)
		return
	}

	// Apply filters to slog-logfilter
	logfilter.SetFilters(filters)

	// Update state
	l.mu.Lock()
	l.filterCount = len(filters)
	l.mu.Unlock()

	// Count active filters
	activeCount := 0
	for _, f := range filters {
		if f.IsActive() {
			activeCount++
		}
	}

	stats := l.loader.Stats()
	l.logger.Info("log filters loaded from S3",
		"bucket", stats.Bucket,
		"key", stats.Key,
		"etag", stats.Etag,
		"total_filters", len(filters),
		"active_filters", activeCount,
	)
}

// Refresh forces an immediate refresh of the log filters.
func (l *LogFiltersLoader) Refresh(ctx context.Context) {
	l.refresh(ctx)
}

// LogFiltersStats contains statistics about the log filters loader.
type LogFiltersStats struct {
	Initialized bool   `json:"initialized"`
	FilterCount int    `json:"filter_count"`
	Etag        string `json:"etag"`
	LastFetch   string `json:"last_fetch"`
	LastCheck   string `json:"last_check"`
	CacheTTL    string `json:"cache_ttl"`
	Bucket      string `json:"bucket"`
	Key         string `json:"key"`
}

// Stats returns current loader statistics.
func (l *LogFiltersLoader) Stats() LogFiltersStats {
	l.mu.RLock()
	filterCount := l.filterCount
	l.mu.RUnlock()

	loaderStats := l.loader.Stats()

	lastFetch := ""
	if !loaderStats.LastFetch.IsZero() {
		lastFetch = loaderStats.LastFetch.Format("2006-01-02T15:04:05Z")
	}
	lastCheck := ""
	if !loaderStats.LastCheck.IsZero() {
		lastCheck = loaderStats.LastCheck.Format("2006-01-02T15:04:05Z")
	}

	return LogFiltersStats{
		Initialized: loaderStats.Initialized,
		FilterCount: filterCount,
		Etag:        loaderStats.Etag,
		LastFetch:   lastFetch,
		LastCheck:   lastCheck,
		CacheTTL:    loaderStats.CacheTTL,
		Bucket:      loaderStats.Bucket,
		Key:         loaderStats.Key,
	}
}
