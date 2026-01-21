package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// ========================================
// CleanupOldJobs Tests
// ========================================

func TestCleanupService_CleanupOldJobs(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	mockJobResultRepo := newMockJobResultRepository()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Use nil for storage service (disabled)
	svc := NewCleanupService(mockJobRepo, mockJobResultRepo, nil, logger)

	t.Run("deletes old jobs", func(t *testing.T) {
		// Create jobs with different ages
		now := time.Now()
		oldTime := now.Add(-30 * 24 * time.Hour) // 30 days ago
		recentTime := now.Add(-1 * time.Hour)    // 1 hour ago

		mockJobRepo.Create(context.Background(), &models.Job{
			ID:        "old-job-1",
			UserID:    "user-1",
			Type:      models.JobTypeExtract,
			Status:    models.JobStatusCompleted,
			CreatedAt: oldTime,
		})
		mockJobRepo.Create(context.Background(), &models.Job{
			ID:        "old-job-2",
			UserID:    "user-1",
			Type:      models.JobTypeCrawl,
			Status:    models.JobStatusFailed,
			CreatedAt: oldTime,
		})
		mockJobRepo.Create(context.Background(), &models.Job{
			ID:        "recent-job",
			UserID:    "user-1",
			Type:      models.JobTypeExtract,
			Status:    models.JobStatusCompleted,
			CreatedAt: recentTime,
		})

		// Add results for old jobs
		mockJobResultRepo.Create(context.Background(), &models.JobResult{
			ID:    "result-1",
			JobID: "old-job-1",
		})
		mockJobResultRepo.Create(context.Background(), &models.JobResult{
			ID:    "result-2",
			JobID: "old-job-2",
		})

		// Cleanup jobs older than 7 days (results and debug)
		maxAge := 7 * 24 * time.Hour
		result, err := svc.CleanupOldJobs(context.Background(), maxAge, maxAge)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if result.JobsDeleted != 2 {
			t.Errorf("JobsDeleted = %d, want 2", result.JobsDeleted)
		}
		if len(result.Errors) != 0 {
			t.Errorf("expected no errors, got %d", len(result.Errors))
		}

		// Verify recent job still exists
		job, _ := mockJobRepo.GetByID(context.Background(), "recent-job")
		if job == nil {
			t.Error("recent job should not have been deleted")
		}
	})

	t.Run("handles no old jobs", func(t *testing.T) {
		// Clear the repository
		mockJobRepo2 := newMockJobRepository()
		svc2 := NewCleanupService(mockJobRepo2, mockJobResultRepo, nil, logger)

		result, err := svc2.CleanupOldJobs(context.Background(), 7*24*time.Hour, 7*24*time.Hour)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result.JobsDeleted != 0 {
			t.Errorf("JobsDeleted = %d, want 0", result.JobsDeleted)
		}
	})

	t.Run("with disabled storage", func(t *testing.T) {
		mockJobRepo3 := newMockJobRepository()
		mockJobResultRepo3 := newMockJobResultRepository()

		// Create a disabled storage service
		disabledStorage := &StorageService{
			enabled: false,
			logger:  logger,
		}

		svc3 := NewCleanupService(mockJobRepo3, mockJobResultRepo3, disabledStorage, logger)

		oldTime := time.Now().Add(-30 * 24 * time.Hour)
		mockJobRepo3.Create(context.Background(), &models.Job{
			ID:        "job-with-disabled-storage",
			UserID:    "user-1",
			Status:    models.JobStatusCompleted,
			CreatedAt: oldTime,
		})

		result, err := svc3.CleanupOldJobs(context.Background(), 7*24*time.Hour, 7*24*time.Hour)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		// Storage should be skipped when disabled
		if result.StorageResultsDeleted != 0 {
			t.Errorf("StorageResultsDeleted = %d, want 0 (storage disabled)", result.StorageResultsDeleted)
		}
	})
}

// ========================================
// CleanupResult Tests
// ========================================

func TestCleanupResult_Fields(t *testing.T) {
	result := &CleanupResult{
		JobsDeleted:           10,
		JobResultsDeleted:     50,
		StorageResultsDeleted: 5,
		StorageDebugDeleted:   3,
		Errors:                []error{},
	}

	if result.JobsDeleted != 10 {
		t.Errorf("JobsDeleted = %d, want 10", result.JobsDeleted)
	}
	if result.JobResultsDeleted != 50 {
		t.Errorf("JobResultsDeleted = %d, want 50", result.JobResultsDeleted)
	}
	if result.StorageResultsDeleted != 5 {
		t.Errorf("StorageResultsDeleted = %d, want 5", result.StorageResultsDeleted)
	}
	if result.StorageDebugDeleted != 3 {
		t.Errorf("StorageDebugDeleted = %d, want 3", result.StorageDebugDeleted)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors length = %d, want 0", len(result.Errors))
	}
}

// ========================================
// RunScheduledCleanup Tests
// ========================================

func TestCleanupService_RunScheduledCleanup(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	mockJobResultRepo := newMockJobResultRepository()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := NewCleanupService(mockJobRepo, mockJobResultRepo, nil, logger)

	t.Run("runs cleanup immediately then at interval", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		// Create an old job
		oldTime := time.Now().Add(-30 * 24 * time.Hour)
		mockJobRepo.Create(context.Background(), &models.Job{
			ID:        "scheduled-cleanup-job",
			UserID:    "user-1",
			Status:    models.JobStatusCompleted,
			CreatedAt: oldTime,
		})

		// Start scheduled cleanup in a goroutine with very short interval
		done := make(chan struct{})
		go func() {
			svc.RunScheduledCleanup(ctx, 7*24*time.Hour, 7*24*time.Hour, 50*time.Millisecond)
			close(done)
		}()

		// Wait a bit for initial cleanup to run
		time.Sleep(100 * time.Millisecond)

		// Verify job was cleaned up
		job, _ := mockJobRepo.GetByID(context.Background(), "scheduled-cleanup-job")
		if job != nil {
			t.Error("expected job to be cleaned up")
		}

		// Stop the scheduler
		cancel()

		// Wait for goroutine to finish
		select {
		case <-done:
			// Good
		case <-time.After(500 * time.Millisecond):
			t.Error("scheduler did not stop in time")
		}
	})

	t.Run("stops on context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan struct{})
		go func() {
			svc.RunScheduledCleanup(ctx, time.Hour, time.Hour, time.Second)
			close(done)
		}()

		// Cancel immediately
		cancel()

		// Should stop quickly
		select {
		case <-done:
			// Good
		case <-time.After(200 * time.Millisecond):
			t.Error("scheduler did not stop on context cancellation")
		}
	})
}

// ========================================
// NewCleanupService Tests
// ========================================

func TestNewCleanupService(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	mockJobResultRepo := newMockJobResultRepository()
	logger := slog.Default()

	svc := NewCleanupService(mockJobRepo, mockJobResultRepo, nil, logger)

	if svc == nil {
		t.Fatal("expected service, got nil")
	}
	if svc.jobRepo != mockJobRepo {
		t.Error("expected jobRepo to be set")
	}
	if svc.jobResultRepo != mockJobResultRepo {
		t.Error("expected jobResultRepo to be set")
	}
	if svc.storageSvc != nil {
		t.Error("expected storageSvc to be nil")
	}
}
