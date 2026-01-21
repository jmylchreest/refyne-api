// Package service contains the business logic layer.
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/jmylchreest/refyne-api/internal/repository"
)

// CleanupService handles cleanup of old job data.
type CleanupService struct {
	jobRepo       repository.JobRepository
	jobResultRepo repository.JobResultRepository
	storageSvc    *StorageService
	logger        *slog.Logger
}

// NewCleanupService creates a new cleanup service.
func NewCleanupService(
	jobRepo repository.JobRepository,
	jobResultRepo repository.JobResultRepository,
	storageSvc *StorageService,
	logger *slog.Logger,
) *CleanupService {
	return &CleanupService{
		jobRepo:       jobRepo,
		jobResultRepo: jobResultRepo,
		storageSvc:    storageSvc,
		logger:        logger.With("component", "cleanup"),
	}
}

// CleanupResult contains the results of a cleanup operation.
type CleanupResult struct {
	JobsDeleted           int
	JobResultsDeleted     int
	StorageResultsDeleted int
	StorageDebugDeleted   int
	Errors                []error
}

// CleanupOldJobs removes job data older than the specified duration.
// This cleans up:
// - Job records from the database (completed/failed only)
// - Job result records from the database
// - Result files from object storage (using maxAgeResults)
// - Debug capture files from object storage (using maxAgeDebug)
//
// Note: Usage records are NOT deleted as they're needed for billing history.
func (s *CleanupService) CleanupOldJobs(ctx context.Context, maxAgeResults, maxAgeDebug time.Duration) (*CleanupResult, error) {
	result := &CleanupResult{}
	cutoff := time.Now().Add(-maxAgeResults)

	s.logger.Info("starting job cleanup",
		"max_age_results", maxAgeResults.String(),
		"max_age_debug", maxAgeDebug.String(),
		"cutoff", cutoff.Format(time.RFC3339),
	)

	// Step 1: Get and delete old jobs from database, returning their IDs
	deletedJobIDs, err := s.jobRepo.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		s.logger.Error("failed to delete old jobs", "error", err)
		result.Errors = append(result.Errors, err)
	} else {
		result.JobsDeleted = len(deletedJobIDs)
		s.logger.Info("deleted old jobs", "count", len(deletedJobIDs))
	}

	// Step 2: Delete job results for deleted jobs
	if len(deletedJobIDs) > 0 {
		if err := s.jobResultRepo.DeleteByJobIDs(ctx, deletedJobIDs); err != nil {
			s.logger.Error("failed to delete job results", "error", err)
			result.Errors = append(result.Errors, err)
		} else {
			// Note: We don't know exact count without an additional query
			result.JobResultsDeleted = -1 // Unknown, but attempted
			s.logger.Info("deleted job results for old jobs")
		}
	}

	// Step 3: Clean up old result objects from storage
	if s.storageSvc != nil && s.storageSvc.IsEnabled() {
		count, err := s.storageSvc.DeleteOldJobResults(ctx, maxAgeResults)
		if err != nil {
			s.logger.Error("failed to delete old result storage objects", "error", err)
			result.Errors = append(result.Errors, err)
		} else {
			result.StorageResultsDeleted = count
			s.logger.Info("deleted old result storage objects", "count", count)
		}

		// Step 4: Clean up old debug capture objects from storage
		debugCount, err := s.storageSvc.DeleteOldDebugCaptures(ctx, maxAgeDebug)
		if err != nil {
			s.logger.Error("failed to delete old debug storage objects", "error", err)
			result.Errors = append(result.Errors, err)
		} else {
			result.StorageDebugDeleted = debugCount
			s.logger.Info("deleted old debug storage objects", "count", debugCount)
		}
	}

	s.logger.Info("cleanup completed",
		"jobs_deleted", result.JobsDeleted,
		"storage_results_deleted", result.StorageResultsDeleted,
		"storage_debug_deleted", result.StorageDebugDeleted,
		"errors", len(result.Errors),
	)

	return result, nil
}

// RunScheduledCleanup runs the cleanup task as a background goroutine.
// It runs immediately on start and then at the specified interval.
func (s *CleanupService) RunScheduledCleanup(ctx context.Context, maxAgeResults, maxAgeDebug, interval time.Duration) {
	s.logger.Info("starting scheduled cleanup",
		"max_age_results", maxAgeResults.String(),
		"max_age_debug", maxAgeDebug.String(),
		"interval", interval.String(),
	)

	// Run immediately on start
	if _, err := s.CleanupOldJobs(ctx, maxAgeResults, maxAgeDebug); err != nil {
		s.logger.Error("initial cleanup failed", "error", err)
	}

	// Then run at interval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduled cleanup stopped")
			return
		case <-ticker.C:
			if _, err := s.CleanupOldJobs(ctx, maxAgeResults, maxAgeDebug); err != nil {
				s.logger.Error("scheduled cleanup failed", "error", err)
			}
		}
	}
}
