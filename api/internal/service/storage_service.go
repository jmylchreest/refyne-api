// Package service contains the business logic layer.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	appconfig "github.com/jmylchreest/refyne-api/internal/config"
)

// StorageService handles object storage operations (Tigris/S3-compatible).
type StorageService struct {
	client  *s3.Client
	bucket  string
	enabled bool
	logger  *slog.Logger
}

// NewStorageService creates a new storage service.
func NewStorageService(cfg *appconfig.Config, logger *slog.Logger) (*StorageService, error) {
	if !cfg.StorageEnabled {
		logger.Info("storage service disabled - no bucket configured")
		return &StorageService{
			enabled: false,
			logger:  logger,
		}, nil
	}

	// Load AWS config with static credentials
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.StorageRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.StorageAccessKey,
			cfg.StorageSecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with custom endpoint for S3-compatible storage (Tigris, MinIO, etc.)
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.StorageEndpoint)
		o.UsePathStyle = true // Required for some S3-compatible services
	})

	logger.Info("storage service initialized",
		"bucket", cfg.StorageBucket,
		"endpoint", cfg.StorageEndpoint,
	)

	return &StorageService{
		client:  client,
		bucket:  cfg.StorageBucket,
		enabled: true,
		logger:  logger,
	}, nil
}

// IsEnabled returns whether storage is configured and available.
func (s *StorageService) IsEnabled() bool {
	return s.enabled
}

// Client returns the underlying S3 client (may be nil if storage is disabled).
func (s *StorageService) Client() *s3.Client {
	return s.client
}

// Bucket returns the configured bucket name.
func (s *StorageService) Bucket() string {
	return s.bucket
}

// JobResultData represents a single extraction result for storage.
type JobResultData struct {
	ID        string          `json:"id"`
	URL       string          `json:"url"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

// LLMRequestCapture captures details of an LLM request for debugging.
type LLMRequestCapture struct {
	ID         string            `json:"id"`
	URL        string            `json:"url"`
	Timestamp  time.Time         `json:"timestamp"`
	JobType    string            `json:"job_type"` // "analyze", "extract", "crawl"
	Request    LLMRequestMeta    `json:"request"`
	Response   LLMResponseMeta   `json:"response"`
	Prompt     string            `json:"prompt,omitempty"`       // Full prompt for analyze jobs
	RawContent string            `json:"raw_content,omitempty"`  // Page content for extract/crawl
	Schema     string            `json:"schema,omitempty"`       // Schema used (for extract/crawl)
	Hints      map[string]string `json:"hints_applied,omitempty"`
}

// LLMRequestMeta contains metadata about the LLM request.
type LLMRequestMeta struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	FetchMode   string `json:"fetch_mode,omitempty"`
	ContentSize int    `json:"content_size"`
	PromptSize  int    `json:"prompt_size"`
}

// LLMResponseMeta contains metadata about the LLM response.
type LLMResponseMeta struct {
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	DurationMs   int64  `json:"duration_ms"`
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
}

// JobDebugCapture holds all debug captures for a job.
type JobDebugCapture struct {
	JobID    string              `json:"job_id"`
	Enabled  bool                `json:"enabled"`
	Captures []LLMRequestCapture `json:"captures"`
}

// JobResults represents all results for a job.
type JobResults struct {
	JobID       string          `json:"job_id"`
	UserID      string          `json:"user_id"`
	Status      string          `json:"status"`
	TotalPages  int             `json:"total_pages"`
	Results     []JobResultData `json:"results"`
	CompletedAt time.Time       `json:"completed_at"`
}

// StoreJobResults stores all job results as a single JSON file.
func (s *StorageService) StoreJobResults(ctx context.Context, results *JobResults) error {
	if !s.enabled {
		return nil // Silently skip if storage is disabled
	}

	// Marshal results to JSON
	data, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("failed to marshal job results: %w", err)
	}

	// Store in bucket: results/{job_id}.json
	key := fmt.Sprintf("results/%s.json", results.JobID)

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("failed to store job results: %w", err)
	}

	s.logger.Info("stored job results",
		"job_id", results.JobID,
		"key", key,
		"size_bytes", len(data),
	)

	return nil
}

// GetJobResults retrieves job results from storage.
func (s *StorageService) GetJobResults(ctx context.Context, jobID string) (*JobResults, error) {
	if !s.enabled {
		return nil, fmt.Errorf("storage is not enabled")
	}

	key := fmt.Sprintf("results/%s.json", jobID)

	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get job results: %w", err)
	}
	defer func() { _ = output.Body.Close() }()

	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read job results: %w", err)
	}

	var results JobResults
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job results: %w", err)
	}

	return &results, nil
}

// GetJobResultsPresignedURL returns a presigned URL for downloading job results.
// The URL is valid for the specified duration (default 1 hour).
func (s *StorageService) GetJobResultsPresignedURL(ctx context.Context, jobID string, expiry time.Duration) (string, error) {
	if !s.enabled {
		return "", fmt.Errorf("storage is not enabled")
	}

	if expiry == 0 {
		expiry = 1 * time.Hour
	}

	key := fmt.Sprintf("results/%s.json", jobID)

	presignClient := s3.NewPresignClient(s.client)
	presignedReq, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedReq.URL, nil
}

// DeleteJobResults deletes job results from storage.
func (s *StorageService) DeleteJobResults(ctx context.Context, jobID string) error {
	if !s.enabled {
		return nil // Silently skip if storage is disabled
	}

	key := fmt.Sprintf("results/%s.json", jobID)

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete job results: %w", err)
	}

	s.logger.Info("deleted job results", "job_id", jobID, "key", key)
	return nil
}

// DeleteOldJobResults deletes job results older than the specified age.
// Returns the number of deleted objects.
func (s *StorageService) DeleteOldJobResults(ctx context.Context, maxAge time.Duration) (int, error) {
	if !s.enabled {
		return 0, nil
	}

	cutoff := time.Now().Add(-maxAge)
	deleted := 0

	// List objects in the results/ prefix
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String("results/"),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return deleted, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			if obj.LastModified != nil && obj.LastModified.Before(cutoff) {
				_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: aws.String(s.bucket),
					Key:    obj.Key,
				})
				if err != nil {
					s.logger.Warn("failed to delete old object",
						"key", *obj.Key,
						"error", err,
					)
					continue
				}
				deleted++
			}
		}
	}

	s.logger.Info("cleanup completed",
		"deleted_count", deleted,
		"max_age", maxAge.String(),
	)

	return deleted, nil
}

// JobResultExists checks if job results exist in storage.
func (s *StorageService) JobResultExists(ctx context.Context, jobID string) (bool, error) {
	if !s.enabled {
		return false, nil
	}

	key := fmt.Sprintf("results/%s.json", jobID)

	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if it's a "not found" error
		return false, nil
	}

	return true, nil
}

// StoreDebugCapture stores debug captures for a job to object storage.
func (s *StorageService) StoreDebugCapture(ctx context.Context, capture *JobDebugCapture) error {
	if !s.enabled {
		return nil // Silently skip if storage is disabled
	}

	if capture == nil || len(capture.Captures) == 0 {
		return nil // Nothing to store
	}

	key := fmt.Sprintf("debug/%s.json", capture.JobID)

	data, err := json.Marshal(capture)
	if err != nil {
		return fmt.Errorf("failed to marshal debug capture: %w", err)
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("failed to upload debug capture: %w", err)
	}

	s.logger.Info("stored debug capture",
		"job_id", capture.JobID,
		"capture_count", len(capture.Captures),
		"key", key,
	)
	return nil
}

// GetDebugCapture retrieves debug captures for a job from object storage.
func (s *StorageService) GetDebugCapture(ctx context.Context, jobID string) (*JobDebugCapture, error) {
	if !s.enabled {
		return nil, fmt.Errorf("storage is not enabled")
	}

	key := fmt.Sprintf("debug/%s.json", jobID)

	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if it's a "not found" error - return empty capture
		return &JobDebugCapture{
			JobID:    jobID,
			Enabled:  false,
			Captures: []LLMRequestCapture{},
		}, nil
	}
	defer func() { _ = output.Body.Close() }()

	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read debug capture: %w", err)
	}

	var capture JobDebugCapture
	if err := json.Unmarshal(data, &capture); err != nil {
		return nil, fmt.Errorf("failed to unmarshal debug capture: %w", err)
	}

	return &capture, nil
}

// DebugCaptureExists checks if debug captures exist for a job.
func (s *StorageService) DebugCaptureExists(ctx context.Context, jobID string) (bool, error) {
	if !s.enabled {
		return false, nil
	}

	key := fmt.Sprintf("debug/%s.json", jobID)

	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return false, nil
	}

	return true, nil
}

// DeleteDebugCapture deletes debug captures for a job.
func (s *StorageService) DeleteDebugCapture(ctx context.Context, jobID string) error {
	if !s.enabled {
		return nil
	}

	key := fmt.Sprintf("debug/%s.json", jobID)

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete debug capture: %w", err)
	}

	s.logger.Info("deleted debug capture", "job_id", jobID, "key", key)
	return nil
}

// DeleteOldDebugCaptures deletes debug captures older than the specified age.
// Returns the number of deleted objects.
func (s *StorageService) DeleteOldDebugCaptures(ctx context.Context, maxAge time.Duration) (int, error) {
	if !s.enabled {
		return 0, nil
	}

	cutoff := time.Now().Add(-maxAge)
	deleted := 0

	// List objects in the debug/ prefix
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String("debug/"),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return deleted, fmt.Errorf("failed to list debug objects: %w", err)
		}

		for _, obj := range page.Contents {
			if obj.LastModified != nil && obj.LastModified.Before(cutoff) {
				_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: aws.String(s.bucket),
					Key:    obj.Key,
				})
				if err != nil {
					s.logger.Warn("failed to delete old debug object",
						"key", *obj.Key,
						"error", err,
					)
					continue
				}
				deleted++
			}
		}
	}

	s.logger.Info("debug capture cleanup completed",
		"deleted_count", deleted,
		"max_age", maxAge.String(),
	)

	return deleted, nil
}
