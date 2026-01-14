package config

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3LoaderConfig holds common configuration for S3-backed config loaders.
type S3LoaderConfig struct {
	S3Client     *s3.Client
	Bucket       string
	Key          string
	CacheTTL     time.Duration // How often to check for updates (default: 5 min)
	ErrorBackoff time.Duration // How long to wait after an error (default: 1 min)
	Logger       *slog.Logger
}

// S3LoadResult contains the result of an S3 config fetch.
type S3LoadResult struct {
	Data       []byte    // Raw JSON data from S3
	Etag       string    // New ETag
	FetchTime  time.Time // When the data was fetched
	NotChanged bool      // True if data hasn't changed (304)
}

// S3Loader provides common S3 config loading functionality with caching.
// Use this as a base for S3-backed configuration loaders.
type S3Loader struct {
	s3Client *s3.Client
	bucket   string
	key      string

	mu           sync.RWMutex
	etag         string
	lastFetch    time.Time
	lastCheck    time.Time
	lastError    time.Time
	initialized  bool
	fetching     bool // true while a fetch is in progress
	cacheTTL     time.Duration
	errorBackoff time.Duration
	logger       *slog.Logger
}

// NewS3Loader creates a new S3 loader with the given config.
func NewS3Loader(cfg S3LoaderConfig) *S3Loader {
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 5 * time.Minute
	}
	if cfg.ErrorBackoff == 0 {
		cfg.ErrorBackoff = 1 * time.Minute
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &S3Loader{
		s3Client:     cfg.S3Client,
		bucket:       cfg.Bucket,
		key:          cfg.Key,
		cacheTTL:     cfg.CacheTTL,
		errorBackoff: cfg.ErrorBackoff,
		logger:       cfg.Logger,
	}
}

// IsEnabled returns true if S3 is configured.
func (l *S3Loader) IsEnabled() bool {
	return l.s3Client != nil
}

// NeedsRefresh returns true if the config should be refreshed.
func (l *S3Loader) NeedsRefresh() bool {
	l.mu.RLock()
	needsRefresh := !l.initialized || time.Since(l.lastCheck) > l.cacheTTL
	inErrorBackoff := !l.lastError.IsZero() && time.Since(l.lastError) < l.errorBackoff
	alreadyFetching := l.fetching
	l.mu.RUnlock()

	return needsRefresh && !inErrorBackoff && !alreadyFetching
}

// Fetch retrieves the config from S3 with ETag caching.
// Returns (result, nil) on success, (nil, nil) if not modified, (nil, err) on error.
func (l *S3Loader) Fetch(ctx context.Context) (*S3LoadResult, error) {
	if l.s3Client == nil {
		return nil, nil // S3 not configured
	}

	l.mu.Lock()
	// Double-check after acquiring lock - also check if another goroutine is already fetching
	if l.fetching || (l.initialized && time.Since(l.lastCheck) < l.cacheTTL) {
		l.mu.Unlock()
		return nil, nil
	}
	l.fetching = true
	currentEtag := l.etag
	l.mu.Unlock()

	// Ensure we clear the fetching flag when done
	defer func() {
		l.mu.Lock()
		l.fetching = false
		l.mu.Unlock()
	}()

	// Build request with conditional fetch
	input := &s3.GetObjectInput{
		Bucket: &l.bucket,
		Key:    &l.key,
	}
	if currentEtag != "" {
		// Add quotes for HTTP If-None-Match header
		quotedEtag := "\"" + currentEtag + "\""
		input.IfNoneMatch = &quotedEtag
	}

	resp, err := l.s3Client.GetObject(ctx, input)
	if err != nil {
		// Check for NoSuchKey (file doesn't exist)
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			l.mu.Lock()
			wasInitialized := l.initialized
			l.initialized = true
			l.lastCheck = time.Now()
			l.lastError = time.Now()
			l.mu.Unlock()
			// Only log on first check, not every poll
			if !wasInitialized {
				l.logger.Debug("S3 config file not found (using defaults)",
					"bucket", l.bucket,
					"key", l.key,
				)
			}
			return nil, nil
		}

		// Check for 304 Not Modified
		var notModified interface{ ErrorCode() string }
		if errors.As(err, &notModified) && notModified.ErrorCode() == "NotModified" {
			l.mu.Lock()
			l.lastCheck = time.Now()
			l.mu.Unlock()
			l.logger.Debug("S3 config unchanged (etag match)",
				"bucket", l.bucket,
				"key", l.key,
				"etag", currentEtag,
			)
			return &S3LoadResult{NotChanged: true}, nil
		}

		// Other error
		l.mu.Lock()
		l.lastError = time.Now()
		l.initialized = true
		l.mu.Unlock()
		l.logger.Error("failed to fetch S3 config",
			"error", err,
			"bucket", l.bucket,
			"key", l.key,
			"next_retry", time.Now().Add(l.errorBackoff).Format(time.RFC3339),
		)
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	var data []byte
	decoder := json.NewDecoder(resp.Body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		l.mu.Lock()
		l.lastError = time.Now()
		l.initialized = true
		l.mu.Unlock()
		l.logger.Error("failed to parse S3 config JSON", "error", err)
		return nil, err
	}
	data = raw

	// Update cache state
	now := time.Now()
	newEtag := ""
	if resp.ETag != nil {
		newEtag = strings.Trim(*resp.ETag, "\"")
	}

	l.mu.Lock()
	previousEtag := l.etag
	l.initialized = true
	l.lastFetch = now
	l.lastCheck = now
	l.lastError = time.Time{}
	l.etag = newEtag
	l.mu.Unlock()

	l.logger.Debug("S3 config fetched",
		"bucket", l.bucket,
		"key", l.key,
		"etag", newEtag,
		"previous_etag", previousEtag,
		"size", len(data),
	)

	return &S3LoadResult{
		Data:      data,
		Etag:      newEtag,
		FetchTime: now,
	}, nil
}

// Stats returns loader statistics.
type S3LoaderStats struct {
	Initialized  bool      `json:"initialized"`
	Etag         string    `json:"etag"`
	LastFetch    time.Time `json:"last_fetch"`
	LastCheck    time.Time `json:"last_check"`
	CacheTTL     string    `json:"cache_ttl"`
	Bucket       string    `json:"bucket"`
	Key          string    `json:"key"`
}

// Stats returns current loader statistics.
func (l *S3Loader) Stats() S3LoaderStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return S3LoaderStats{
		Initialized: l.initialized,
		Etag:        l.etag,
		LastFetch:   l.lastFetch,
		LastCheck:   l.lastCheck,
		CacheTTL:    l.cacheTTL.String(),
		Bucket:      l.bucket,
		Key:         l.key,
	}
}
