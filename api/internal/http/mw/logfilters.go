package mw

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/jmylchreest/slog-logfilter"
)

// LogFiltersConfig holds configuration for the log filters loader.
type LogFiltersConfig struct {
	S3Client     *s3.Client
	Bucket       string
	Key          string // Default: "config/logfilters.json"
	CacheTTL     time.Duration // How often to check for updates (default: 5 min)
	ErrorBackoff time.Duration // How long to wait after an error (default: 1 min)
	Logger       *slog.Logger
}

// LogFiltersLoader loads log filters from S3 and applies them to slog-logfilter.
// Features:
// - Lazy loading: doesn't fetch until Start() is called or first refresh
// - Etag caching: only downloads when filters change
// - Error backoff: waits before retrying on S3 errors
// - Fail safe: keeps existing filters if update fails
type LogFiltersLoader struct {
	s3Client *s3.Client
	bucket   string
	key      string

	mu           sync.RWMutex
	etag         string
	lastFetch    time.Time
	lastCheck    time.Time
	lastError    time.Time
	initialized  bool
	filterCount  int
	cacheTTL     time.Duration
	errorBackoff time.Duration
	logger       *slog.Logger

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewLogFiltersLoader creates a new log filters loader.
func NewLogFiltersLoader(cfg LogFiltersConfig) *LogFiltersLoader {
	if cfg.Key == "" {
		cfg.Key = "config/logfilters.json"
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 5 * time.Minute
	}
	if cfg.ErrorBackoff == 0 {
		cfg.ErrorBackoff = 1 * time.Minute
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &LogFiltersLoader{
		s3Client:     cfg.S3Client,
		bucket:       cfg.Bucket,
		key:          cfg.Key,
		cacheTTL:     cfg.CacheTTL,
		errorBackoff: cfg.ErrorBackoff,
		logger:       cfg.Logger,
		stopCh:       make(chan struct{}),
	}
}

// Start begins the periodic refresh of log filters.
// It immediately fetches the filters and then periodically checks for updates.
func (l *LogFiltersLoader) Start(ctx context.Context) {
	if l.s3Client == nil {
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

	l.logger.Info("log filters loader started",
		"bucket", l.bucket,
		"key", l.key,
		"cache_ttl", l.cacheTTL.String(),
	)
}

// Stop stops the periodic refresh.
func (l *LogFiltersLoader) Stop() {
	close(l.stopCh)
	l.wg.Wait()
}

// refresh fetches the log filters from S3.
func (l *LogFiltersLoader) refresh(ctx context.Context) {
	l.mu.Lock()
	// Check if in error backoff
	if !l.lastError.IsZero() && time.Since(l.lastError) < l.errorBackoff {
		l.mu.Unlock()
		return
	}
	currentEtag := l.etag
	l.mu.Unlock()

	// Build request with conditional fetch
	input := &s3.GetObjectInput{
		Bucket: &l.bucket,
		Key:    &l.key,
	}
	if currentEtag != "" {
		quotedEtag := "\"" + currentEtag + "\""
		input.IfNoneMatch = &quotedEtag
	}

	resp, err := l.s3Client.GetObject(ctx, input)
	if err != nil {
		// Check for NoSuchKey (file doesn't exist)
		var apiErr *types.NoSuchKey
		if errors.As(err, &apiErr) {
			l.mu.Lock()
			l.initialized = true
			l.lastCheck = time.Now()
			l.lastError = time.Now()
			l.mu.Unlock()
			l.logger.Info("log filters file not found in S3 (using default filters)",
				"bucket", l.bucket,
				"key", l.key,
			)
			return
		}

		// Check for 304 Not Modified
		var notModified interface{ ErrorCode() string }
		if errors.As(err, &notModified) && notModified.ErrorCode() == "NotModified" {
			l.mu.Lock()
			l.lastCheck = time.Now()
			count := l.filterCount
			etag := l.etag
			l.mu.Unlock()
			l.logger.Debug("log filters unchanged (etag match)",
				"etag", etag,
				"filter_count", count,
			)
			return
		}

		// Other error
		l.mu.Lock()
		l.lastError = time.Now()
		l.initialized = true
		l.mu.Unlock()
		l.logger.Error("failed to fetch log filters from S3",
			"error", err,
			"bucket", l.bucket,
			"key", l.key,
		)
		return
	}
	defer resp.Body.Close()

	// Parse filters
	var filters []logfilter.LogFilter
	if err := json.NewDecoder(resp.Body).Decode(&filters); err != nil {
		l.mu.Lock()
		l.lastError = time.Now()
		l.initialized = true
		l.mu.Unlock()
		l.logger.Error("failed to parse log filters JSON", "error", err)
		return
	}

	// Apply filters to slog-logfilter
	logfilter.SetFilters(filters)

	// Update state
	now := time.Now()
	newEtag := ""
	if resp.ETag != nil {
		newEtag = *resp.ETag
		// Strip quotes from ETag
		if len(newEtag) >= 2 && newEtag[0] == '"' && newEtag[len(newEtag)-1] == '"' {
			newEtag = newEtag[1 : len(newEtag)-1]
		}
	}

	l.mu.Lock()
	previousEtag := l.etag
	l.initialized = true
	l.lastFetch = now
	l.lastCheck = now
	l.lastError = time.Time{}
	l.etag = newEtag
	l.filterCount = len(filters)
	l.mu.Unlock()

	// Count active filters
	activeCount := 0
	for _, f := range filters {
		if f.IsActive() {
			activeCount++
		}
	}

	l.logger.Info("log filters loaded from S3",
		"bucket", l.bucket,
		"key", l.key,
		"etag", newEtag,
		"previous_etag", previousEtag,
		"total_filters", len(filters),
		"active_filters", activeCount,
	)
}

// LogFiltersStats contains statistics about the log filters loader.
type LogFiltersStats struct {
	Initialized bool      `json:"initialized"`
	FilterCount int       `json:"filter_count"`
	Etag        string    `json:"etag"`
	LastFetch   time.Time `json:"last_fetch"`
	LastCheck   time.Time `json:"last_check"`
	CacheTTL    string    `json:"cache_ttl"`
}

// Stats returns current loader statistics.
func (l *LogFiltersLoader) Stats() LogFiltersStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return LogFiltersStats{
		Initialized: l.initialized,
		FilterCount: l.filterCount,
		Etag:        l.etag,
		LastFetch:   l.lastFetch,
		LastCheck:   l.lastCheck,
		CacheTTL:    l.cacheTTL.String(),
	}
}
