package repository

import (
	"context"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/oklog/ulid/v2"
)

func TestJobRepository_Create(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	job := &models.Job{
		ID:         ulid.Make().String(),
		UserID:     "user_123",
		Type:       models.JobTypeExtract,
		Status:     models.JobStatusPending,
		URL:        "https://example.com",
		SchemaJSON: `{"type": "object"}`,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := repos.Job.Create(ctx, job)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify it was created
	got, err := repos.Job.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetByID() returned nil")
	}
	if got.ID != job.ID {
		t.Errorf("ID = %s, want %s", got.ID, job.ID)
	}
	if got.UserID != job.UserID {
		t.Errorf("UserID = %s, want %s", got.UserID, job.UserID)
	}
	if got.Status != job.Status {
		t.Errorf("Status = %s, want %s", got.Status, job.Status)
	}
}

func TestJobRepository_GetByID_NotFound(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	got, err := repos.Job.GetByID(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent job")
	}
}

func TestJobRepository_GetByUserID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	userID := "user_123"
	otherUserID := "user_456"

	// Create jobs for different users
	for i := 0; i < 3; i++ {
		job := &models.Job{
			ID:         ulid.Make().String(),
			UserID:     userID,
			Type:       models.JobTypeExtract,
			Status:     models.JobStatusPending,
			URL:        "https://example.com",
			SchemaJSON: "{}",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := repos.Job.Create(ctx, job); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Create job for other user
	otherJob := &models.Job{
		ID:         ulid.Make().String(),
		UserID:     otherUserID,
		Type:       models.JobTypeExtract,
		Status:     models.JobStatusPending,
		URL:        "https://example.com",
		SchemaJSON: "{}",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := repos.Job.Create(ctx, otherJob); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Query by user
	jobs, err := repos.Job.GetByUserID(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("len(jobs) = %d, want 3", len(jobs))
	}

	// Verify all belong to correct user
	for _, job := range jobs {
		if job.UserID != userID {
			t.Errorf("job.UserID = %s, want %s", job.UserID, userID)
		}
	}
}

func TestJobRepository_Update(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	job := &models.Job{
		ID:         ulid.Make().String(),
		UserID:     "user_123",
		Type:       models.JobTypeExtract,
		Status:     models.JobStatusPending,
		URL:        "https://example.com",
		SchemaJSON: "{}",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := repos.Job.Create(ctx, job); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update the job
	job.Status = models.JobStatusCompleted
	job.ResultJSON = `{"data": "extracted"}`
	job.PageCount = 5
	job.TokenUsageInput = 1000
	job.TokenUsageOutput = 500
	completedAt := time.Now()
	job.CompletedAt = &completedAt

	if err := repos.Job.Update(ctx, job); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify updates
	got, err := repos.Job.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != models.JobStatusCompleted {
		t.Errorf("Status = %s, want %s", got.Status, models.JobStatusCompleted)
	}
	if got.ResultJSON != job.ResultJSON {
		t.Errorf("ResultJSON = %s, want %s", got.ResultJSON, job.ResultJSON)
	}
	if got.PageCount != 5 {
		t.Errorf("PageCount = %d, want 5", got.PageCount)
	}
}

func TestJobRepository_GetPending(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create jobs with different statuses
	statuses := []models.JobStatus{
		models.JobStatusPending,
		models.JobStatusPending,
		models.JobStatusRunning,
		models.JobStatusCompleted,
		models.JobStatusFailed,
	}

	for _, status := range statuses {
		job := &models.Job{
			ID:         ulid.Make().String(),
			UserID:     "user_123",
			Type:       models.JobTypeCrawl, // GetPending only returns crawl jobs (used by worker)
			Status:     status,
			URL:        "https://example.com",
			SchemaJSON: "{}",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := repos.Job.Create(ctx, job); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		// Small delay to ensure different creation times
		time.Sleep(time.Millisecond)
	}

	pending, err := repos.Job.GetPending(ctx, 10)
	if err != nil {
		t.Fatalf("GetPending() error = %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("len(pending) = %d, want 2", len(pending))
	}

	for _, job := range pending {
		if job.Status != models.JobStatusPending {
			t.Errorf("job.Status = %s, want pending", job.Status)
		}
	}
}

func TestJobRepository_ClaimJob(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	job := &models.Job{
		ID:         ulid.Make().String(),
		UserID:     "user_123",
		Type:       models.JobTypeExtract,
		Status:     models.JobStatusPending,
		URL:        "https://example.com",
		SchemaJSON: "{}",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := repos.Job.Create(ctx, job); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Claim the job
	claimed, err := repos.Job.ClaimJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("ClaimJob() error = %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimJob() returned nil")
	}
	if claimed.Status != models.JobStatusRunning {
		t.Errorf("Status = %s, want running", claimed.Status)
	}
	if claimed.StartedAt == nil {
		t.Error("StartedAt should be set")
	}

	// Try to claim again - should return nil (already claimed)
	claimed2, err := repos.Job.ClaimJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("ClaimJob() second call error = %v", err)
	}
	if claimed2 != nil {
		t.Error("expected nil when job already claimed")
	}
}

func TestJobRepository_ClaimPending(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create multiple pending crawl jobs (ClaimPending only returns crawl jobs - used by worker)
	jobs := make([]*models.Job, 3)
	for i := 0; i < 3; i++ {
		jobs[i] = &models.Job{
			ID:         ulid.Make().String(),
			UserID:     "user_123",
			Type:       models.JobTypeCrawl,
			Status:     models.JobStatusPending,
			URL:        "https://example.com",
			SchemaJSON: "{}",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := repos.Job.Create(ctx, jobs[i]); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		time.Sleep(time.Millisecond) // Ensure ordering
	}

	// Claim first pending
	claimed, err := repos.Job.ClaimPending(ctx)
	if err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimPending() returned nil")
	}
	if claimed.ID != jobs[0].ID {
		t.Errorf("claimed job ID = %s, want %s (first created)", claimed.ID, jobs[0].ID)
	}
	if claimed.Status != models.JobStatusRunning {
		t.Errorf("Status = %s, want running", claimed.Status)
	}

	// Claim second pending
	claimed2, err := repos.Job.ClaimPending(ctx)
	if err != nil {
		t.Fatalf("ClaimPending() second call error = %v", err)
	}
	if claimed2 == nil {
		t.Fatal("ClaimPending() second call returned nil")
	}
	if claimed2.ID != jobs[1].ID {
		t.Errorf("claimed job ID = %s, want %s (second created)", claimed2.ID, jobs[1].ID)
	}
}

func TestJobRepository_ClaimPending_NoPending(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// No jobs exist
	claimed, err := repos.Job.ClaimPending(ctx)
	if err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}
	if claimed != nil {
		t.Error("expected nil when no pending jobs")
	}
}

func TestJobRepository_DeleteOlderThan(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Insert jobs with specific timestamps
	now := time.Now()
	oldTime := now.Add(-48 * time.Hour).Format(time.RFC3339)
	newTime := now.Add(-1 * time.Hour).Format(time.RFC3339)

	// Old completed job - should be deleted
	_, err := db.Exec(`
		INSERT INTO jobs (id, user_id, type, status, url, schema_json, created_at, updated_at)
		VALUES ('old_completed', 'user_123', 'extract', 'completed', 'https://example.com', '{}', ?, ?)
	`, oldTime, oldTime)
	if err != nil {
		t.Fatalf("failed to insert old job: %v", err)
	}

	// Old pending job - should NOT be deleted (not completed/failed)
	_, err = db.Exec(`
		INSERT INTO jobs (id, user_id, type, status, url, schema_json, created_at, updated_at)
		VALUES ('old_pending', 'user_123', 'extract', 'pending', 'https://example.com', '{}', ?, ?)
	`, oldTime, oldTime)
	if err != nil {
		t.Fatalf("failed to insert old pending job: %v", err)
	}

	// New completed job - should NOT be deleted (too recent)
	_, err = db.Exec(`
		INSERT INTO jobs (id, user_id, type, status, url, schema_json, created_at, updated_at)
		VALUES ('new_completed', 'user_123', 'extract', 'completed', 'https://example.com', '{}', ?, ?)
	`, newTime, newTime)
	if err != nil {
		t.Fatalf("failed to insert new job: %v", err)
	}

	// Create repo with the same db
	jobRepo := NewSQLiteJobRepository(db)

	// Delete jobs older than 24 hours
	deletedIDs, err := jobRepo.DeleteOlderThan(ctx, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("DeleteOlderThan() error = %v", err)
	}

	if len(deletedIDs) != 1 {
		t.Errorf("len(deletedIDs) = %d, want 1", len(deletedIDs))
	}
	if len(deletedIDs) > 0 && deletedIDs[0] != "old_completed" {
		t.Errorf("deleted ID = %s, want old_completed", deletedIDs[0])
	}

	// Verify remaining jobs
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM jobs").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count jobs: %v", err)
	}
	if count != 2 {
		t.Errorf("remaining jobs = %d, want 2", count)
	}
}

func TestJobRepository_MarkStaleRunningJobsFailed(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	now := time.Now()
	staleTime := now.Add(-2 * time.Hour).Format(time.RFC3339)
	recentTime := now.Add(-10 * time.Minute).Format(time.RFC3339)

	// Stale running job - should be marked failed
	_, err := db.Exec(`
		INSERT INTO jobs (id, user_id, type, status, url, schema_json, started_at, created_at, updated_at)
		VALUES ('stale_running', 'user_123', 'extract', 'running', 'https://example.com', '{}', ?, ?, ?)
	`, staleTime, staleTime, staleTime)
	if err != nil {
		t.Fatalf("failed to insert stale job: %v", err)
	}

	// Recent running job - should NOT be marked failed
	_, err = db.Exec(`
		INSERT INTO jobs (id, user_id, type, status, url, schema_json, started_at, created_at, updated_at)
		VALUES ('recent_running', 'user_123', 'extract', 'running', 'https://example.com', '{}', ?, ?, ?)
	`, recentTime, recentTime, recentTime)
	if err != nil {
		t.Fatalf("failed to insert recent job: %v", err)
	}

	jobRepo := NewSQLiteJobRepository(db)

	// Mark jobs running longer than 1 hour as failed
	count, err := jobRepo.MarkStaleRunningJobsFailed(ctx, time.Hour)
	if err != nil {
		t.Fatalf("MarkStaleRunningJobsFailed() error = %v", err)
	}

	if count != 1 {
		t.Errorf("marked count = %d, want 1", count)
	}

	// Verify stale job is now failed
	var status string
	err = db.QueryRow("SELECT status FROM jobs WHERE id = 'stale_running'").Scan(&status)
	if err != nil {
		t.Fatalf("failed to query status: %v", err)
	}
	if status != "failed" {
		t.Errorf("stale job status = %s, want failed", status)
	}

	// Verify recent job is still running
	err = db.QueryRow("SELECT status FROM jobs WHERE id = 'recent_running'").Scan(&status)
	if err != nil {
		t.Fatalf("failed to query status: %v", err)
	}
	if status != "running" {
		t.Errorf("recent job status = %s, want running", status)
	}
}

func TestJobRepository_CountActiveByUserID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	userID := "user_123"

	// Create jobs with different statuses
	testCases := []struct {
		status models.JobStatus
	}{
		{models.JobStatusPending},
		{models.JobStatusPending},
		{models.JobStatusRunning},
		{models.JobStatusCompleted},
		{models.JobStatusFailed},
	}

	for _, tc := range testCases {
		job := &models.Job{
			ID:         ulid.Make().String(),
			UserID:     userID,
			Type:       models.JobTypeExtract,
			Status:     tc.status,
			URL:        "https://example.com",
			SchemaJSON: "{}",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if tc.status == models.JobStatusRunning {
			started := time.Now()
			job.StartedAt = &started
		}
		if err := repos.Job.Create(ctx, job); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Count active jobs (pending + recent running)
	count, err := repos.Job.CountActiveByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("CountActiveByUserID() error = %v", err)
	}

	// Should be 3: 2 pending + 1 running (not stale)
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestJobResultRepository_Create(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create parent job first
	job := &models.Job{
		ID:         ulid.Make().String(),
		UserID:     "user_123",
		Type:       models.JobTypeExtract,
		Status:     models.JobStatusPending,
		URL:        "https://example.com",
		SchemaJSON: "{}",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := repos.Job.Create(ctx, job); err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	// Create result
	result := &models.JobResult{
		ID:               ulid.Make().String(),
		JobID:            job.ID,
		URL:              "https://example.com",
		Depth:            0,
		CrawlStatus:      models.CrawlStatusCompleted,
		DataJSON:         `{"extracted": "data"}`,
		TokenUsageInput:  1000,
		TokenUsageOutput: 500,
		LLMProvider:      "openrouter",
		LLMModel:         "gpt-4",
		CreatedAt:        time.Now(),
	}

	err := repos.JobResult.Create(ctx, result)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify by getting results for job
	results, err := repos.JobResult.GetByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByJobID() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
	if len(results) > 0 {
		if results[0].DataJSON != result.DataJSON {
			t.Errorf("DataJSON = %s, want %s", results[0].DataJSON, result.DataJSON)
		}
	}
}

func TestJobResultRepository_GetAfterID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create parent job
	job := &models.Job{
		ID:         ulid.Make().String(),
		UserID:     "user_123",
		Type:       models.JobTypeCrawl,
		Status:     models.JobStatusPending,
		URL:        "https://example.com",
		SchemaJSON: "{}",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := repos.Job.Create(ctx, job); err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	// Create multiple results
	resultIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		resultIDs[i] = ulid.Make().String()
		result := &models.JobResult{
			ID:          resultIDs[i],
			JobID:       job.ID,
			URL:         "https://example.com/page" + string(rune('0'+i)),
			Depth:       i,
			CrawlStatus: models.CrawlStatusCompleted,
			CreatedAt:   time.Now(),
		}
		if err := repos.JobResult.Create(ctx, result); err != nil {
			t.Fatalf("failed to create result: %v", err)
		}
		time.Sleep(time.Millisecond) // Ensure different ULID timestamps
	}

	// Get results after first ID
	results, err := repos.JobResult.GetAfterID(ctx, job.ID, resultIDs[0])
	if err != nil {
		t.Fatalf("GetAfterID() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}

	// Get all results (empty afterID)
	allResults, err := repos.JobResult.GetAfterID(ctx, job.ID, "")
	if err != nil {
		t.Fatalf("GetAfterID('') error = %v", err)
	}
	if len(allResults) != 3 {
		t.Errorf("len(allResults) = %d, want 3", len(allResults))
	}
}

func TestJobResultRepository_CountByJobID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create parent job
	job := &models.Job{
		ID:         ulid.Make().String(),
		UserID:     "user_123",
		Type:       models.JobTypeCrawl,
		Status:     models.JobStatusPending,
		URL:        "https://example.com",
		SchemaJSON: "{}",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := repos.Job.Create(ctx, job); err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	// Create results
	for i := 0; i < 5; i++ {
		result := &models.JobResult{
			ID:          ulid.Make().String(),
			JobID:       job.ID,
			URL:         "https://example.com",
			Depth:       0,
			CrawlStatus: models.CrawlStatusCompleted,
			CreatedAt:   time.Now(),
		}
		if err := repos.JobResult.Create(ctx, result); err != nil {
			t.Fatalf("failed to create result: %v", err)
		}
	}

	count, err := repos.JobResult.CountByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("CountByJobID() error = %v", err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}
