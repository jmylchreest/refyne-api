package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// mockJobRepository implements repository.JobRepository for testing.
type mockJobRepository struct {
	mu   sync.RWMutex
	jobs map[string]*models.Job
}

func newMockJobRepository() *mockJobRepository {
	return &mockJobRepository{
		jobs: make(map[string]*models.Job),
	}
}

func (m *mockJobRepository) Create(ctx context.Context, job *models.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[job.ID] = job
	return nil
}

func (m *mockJobRepository) GetByID(ctx context.Context, id string) (*models.Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if job, ok := m.jobs[id]; ok {
		return job, nil
	}
	return nil, nil
}

func (m *mockJobRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.Job
	for _, job := range m.jobs {
		if job.UserID == userID {
			result = append(result, job)
		}
	}
	// Apply offset and limit
	if offset >= len(result) {
		return []*models.Job{}, nil
	}
	result = result[offset:]
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockJobRepository) Update(ctx context.Context, job *models.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.jobs[job.ID]; ok {
		m.jobs[job.ID] = job
		return nil
	}
	return nil
}

func (m *mockJobRepository) GetPending(ctx context.Context, limit int) ([]*models.Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.Job
	for _, job := range m.jobs {
		if job.Status == models.JobStatusPending {
			result = append(result, job)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockJobRepository) ClaimJob(ctx context.Context, id string) (*models.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, ok := m.jobs[id]; ok && job.Status == models.JobStatusPending {
		job.Status = models.JobStatusRunning
		now := time.Now()
		job.StartedAt = &now
		return job, nil
	}
	return nil, nil
}

func (m *mockJobRepository) ClaimPending(ctx context.Context) (*models.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, job := range m.jobs {
		if job.Status == models.JobStatusPending {
			job.Status = models.JobStatusRunning
			now := time.Now()
			job.StartedAt = &now
			return job, nil
		}
	}
	return nil, nil
}

func (m *mockJobRepository) DeleteOlderThan(ctx context.Context, before time.Time) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var deleted []string
	for id, job := range m.jobs {
		if job.CreatedAt.Before(before) {
			deleted = append(deleted, id)
			delete(m.jobs, id)
		}
	}
	return deleted, nil
}

func (m *mockJobRepository) MarkStaleRunningJobsFailed(ctx context.Context, maxAge time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	cutoff := time.Now().Add(-maxAge)
	for _, job := range m.jobs {
		if job.Status == models.JobStatusRunning && job.StartedAt != nil && job.StartedAt.Before(cutoff) {
			job.Status = models.JobStatusFailed
			job.ErrorMessage = "Job timed out"
			count++
		}
	}
	return count, nil
}

func (m *mockJobRepository) CountActiveByUserID(ctx context.Context, userID string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, job := range m.jobs {
		if job.UserID == userID && (job.Status == models.JobStatusPending || job.Status == models.JobStatusRunning) {
			count++
		}
	}
	return count, nil
}

// mockJobResultRepository implements repository.JobResultRepository for testing.
type mockJobResultRepository struct {
	mu      sync.RWMutex
	results map[string][]*models.JobResult // keyed by jobID
}

func newMockJobResultRepository() *mockJobResultRepository {
	return &mockJobResultRepository{
		results: make(map[string][]*models.JobResult),
	}
}

func (m *mockJobResultRepository) Create(ctx context.Context, result *models.JobResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[result.JobID] = append(m.results[result.JobID], result)
	return nil
}

func (m *mockJobResultRepository) GetByJobID(ctx context.Context, jobID string) ([]*models.JobResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.results[jobID], nil
}

func (m *mockJobResultRepository) GetAfterID(ctx context.Context, jobID, afterID string) ([]*models.JobResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	results := m.results[jobID]
	if afterID == "" {
		return results, nil
	}
	// Find results after the specified ID
	var filtered []*models.JobResult
	found := false
	for _, r := range results {
		if found {
			filtered = append(filtered, r)
		}
		if r.ID == afterID {
			found = true
		}
	}
	return filtered, nil
}

func (m *mockJobResultRepository) GetCrawlMap(ctx context.Context, jobID string) ([]*models.JobResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Return results sorted by depth (simplified - just return all)
	return m.results[jobID], nil
}

func (m *mockJobResultRepository) DeleteByJobIDs(ctx context.Context, jobIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, jobID := range jobIDs {
		delete(m.results, jobID)
	}
	return nil
}

func (m *mockJobResultRepository) CountByJobID(ctx context.Context, jobID string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.results[jobID]), nil
}

// ========================================
// CreateCrawlJob Tests
// ========================================

func TestJobService_CreateCrawlJob(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	cfg := &config.Config{
		BaseURL: "https://api.example.com",
	}
	repos := &repository.Repositories{
		Job: mockJobRepo,
	}

	logger := slog.Default()
	svc := NewJobService(cfg, repos, logger)

	t.Run("creates crawl job with basic input", func(t *testing.T) {
		schema := json.RawMessage(`{"type":"object"}`)
		input := CreateCrawlJobInput{
			URL:    "https://example.com",
			Schema: schema,
		}

		output, err := svc.CreateCrawlJob(context.Background(), "user-123", input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if output == nil {
			t.Fatal("expected output, got nil")
		}
		if output.JobID == "" {
			t.Error("expected JobID to be set")
		}
		if output.Status != "pending" {
			t.Errorf("Status = %q, want %q", output.Status, "pending")
		}
		if output.StatusURL == "" {
			t.Error("expected StatusURL to be set")
		}

		// Verify job in repo
		job, _ := mockJobRepo.GetByID(context.Background(), output.JobID)
		if job == nil {
			t.Fatal("expected job in repo")
		}
		if job.Type != models.JobTypeCrawl {
			t.Errorf("Type = %q, want %q", job.Type, models.JobTypeCrawl)
		}
		if job.UserID != "user-123" {
			t.Errorf("UserID = %q, want %q", job.UserID, "user-123")
		}
	})

	t.Run("creates crawl job with all options", func(t *testing.T) {
		schema := json.RawMessage(`{"type":"array"}`)
		input := CreateCrawlJobInput{
			URL:    "https://blog.example.com",
			Schema: schema,
			Options: CrawlOptions{
				FollowSelector:   "a.article-link",
				MaxDepth:         3,
				MaxPages:         100,
				SameDomainOnly:   true,
				ExtractFromSeeds: true,
			},
			WebhookURL:          "https://webhook.example.com/notify",
			Tier:                "pro",
			BYOKAllowed:         true,
			ModelsCustomAllowed: true,
		}

		output, err := svc.CreateCrawlJob(context.Background(), "user-456", input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify job in repo
		job, _ := mockJobRepo.GetByID(context.Background(), output.JobID)
		if job == nil {
			t.Fatal("expected job in repo")
		}
		if job.WebhookURL != "https://webhook.example.com/notify" {
			t.Errorf("WebhookURL = %q, want %q", job.WebhookURL, "https://webhook.example.com/notify")
		}
		if job.Tier != "pro" {
			t.Errorf("Tier = %q, want %q", job.Tier, "pro")
		}
		if !job.BYOKAllowed {
			t.Error("expected BYOKAllowed to be true")
		}
		if !job.ModelsCustomAllowed {
			t.Error("expected ModelsCustomAllowed to be true")
		}
	})
}

// ========================================
// CreateExtractJob Tests
// ========================================

func TestJobService_CreateExtractJob(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	cfg := &config.Config{
		BaseURL: "https://api.example.com",
	}
	repos := &repository.Repositories{
		Job: mockJobRepo,
	}

	logger := slog.Default()
	svc := NewJobService(cfg, repos, logger)

	t.Run("creates extract job", func(t *testing.T) {
		schema := json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}}}`)
		input := CreateExtractJobInput{
			URL:       "https://example.com/article",
			Schema:    schema,
			FetchMode: "browser",
			IsBYOK:    true,
		}

		output, err := svc.CreateExtractJob(context.Background(), "user-789", input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if output == nil {
			t.Fatal("expected output, got nil")
		}
		if output.JobID == "" {
			t.Error("expected JobID to be set")
		}
		if output.Status != "running" {
			t.Errorf("Status = %q, want %q (extract jobs start in running state)", output.Status, "running")
		}

		// Verify job in repo
		job, _ := mockJobRepo.GetByID(context.Background(), output.JobID)
		if job == nil {
			t.Fatal("expected job in repo")
		}
		if job.Type != models.JobTypeExtract {
			t.Errorf("Type = %q, want %q", job.Type, models.JobTypeExtract)
		}
		if job.PageCount != 1 {
			t.Errorf("PageCount = %d, want 1 for single-page extract", job.PageCount)
		}
		if !job.IsBYOK {
			t.Error("expected IsBYOK to be true")
		}
		if job.StartedAt == nil {
			t.Error("expected StartedAt to be set")
		}
	})
}

// ========================================
// GetJob Tests
// ========================================

func TestJobService_GetJob(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	cfg := &config.Config{}
	repos := &repository.Repositories{
		Job: mockJobRepo,
	}

	logger := slog.Default()
	svc := NewJobService(cfg, repos, logger)

	// Create a job
	job := &models.Job{
		ID:        "job-123",
		UserID:    "user-owner",
		Type:      models.JobTypeExtract,
		Status:    models.JobStatusCompleted,
		CreatedAt: time.Now(),
	}
	mockJobRepo.Create(context.Background(), job)

	t.Run("returns job for owner", func(t *testing.T) {
		result, err := svc.GetJob(context.Background(), "user-owner", "job-123")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result == nil {
			t.Fatal("expected job, got nil")
		}
		if result.ID != "job-123" {
			t.Errorf("ID = %q, want %q", result.ID, "job-123")
		}
	})

	t.Run("returns nil for non-owner", func(t *testing.T) {
		result, err := svc.GetJob(context.Background(), "user-other", "job-123")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result != nil {
			t.Error("expected nil job for non-owner")
		}
	})

	t.Run("returns nil for non-existent job", func(t *testing.T) {
		result, err := svc.GetJob(context.Background(), "user-owner", "job-nonexistent")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result != nil {
			t.Error("expected nil job for non-existent ID")
		}
	})
}

// ========================================
// ListJobs Tests
// ========================================

func TestJobService_ListJobs(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	cfg := &config.Config{}
	repos := &repository.Repositories{
		Job: mockJobRepo,
	}

	logger := slog.Default()
	svc := NewJobService(cfg, repos, logger)

	// Create jobs for different users
	for i := 0; i < 5; i++ {
		mockJobRepo.Create(context.Background(), &models.Job{
			ID:        "job-user-a-" + string(rune('0'+i)),
			UserID:    "user-a",
			Type:      models.JobTypeExtract,
			Status:    models.JobStatusCompleted,
			CreatedAt: time.Now(),
		})
	}
	mockJobRepo.Create(context.Background(), &models.Job{
		ID:        "job-user-b-0",
		UserID:    "user-b",
		Type:      models.JobTypeExtract,
		Status:    models.JobStatusCompleted,
		CreatedAt: time.Now(),
	})

	t.Run("lists jobs for user", func(t *testing.T) {
		jobs, err := svc.ListJobs(context.Background(), "user-a", 10, 0)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(jobs) != 5 {
			t.Errorf("expected 5 jobs, got %d", len(jobs))
		}
	})

	t.Run("applies default limit", func(t *testing.T) {
		jobs, err := svc.ListJobs(context.Background(), "user-a", 0, 0)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		// Should use default limit of 20
		if len(jobs) > 20 {
			t.Errorf("expected at most 20 jobs with default limit, got %d", len(jobs))
		}
	})

	t.Run("caps limit at 100", func(t *testing.T) {
		jobs, err := svc.ListJobs(context.Background(), "user-a", 200, 0)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		// Limit should be capped at 100
		if len(jobs) > 100 {
			t.Errorf("expected at most 100 jobs with capped limit, got %d", len(jobs))
		}
	})

	t.Run("returns empty list for user with no jobs", func(t *testing.T) {
		jobs, err := svc.ListJobs(context.Background(), "user-no-jobs", 10, 0)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(jobs) != 0 {
			t.Errorf("expected 0 jobs, got %d", len(jobs))
		}
	})
}

// ========================================
// GetJobResults Tests
// ========================================

func TestJobService_GetJobResults(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	mockResultRepo := newMockJobResultRepository()
	cfg := &config.Config{}
	repos := &repository.Repositories{
		Job:       mockJobRepo,
		JobResult: mockResultRepo,
	}

	logger := slog.Default()
	svc := NewJobService(cfg, repos, logger)

	// Create a job
	mockJobRepo.Create(context.Background(), &models.Job{
		ID:        "job-with-results",
		UserID:    "user-results",
		Type:      models.JobTypeCrawl,
		Status:    models.JobStatusCompleted,
		CreatedAt: time.Now(),
	})

	// Add results
	for i := 0; i < 3; i++ {
		mockResultRepo.Create(context.Background(), &models.JobResult{
			ID:          "result-" + string(rune('0'+i)),
			JobID:       "job-with-results",
			URL:         "https://example.com/page" + string(rune('0'+i)),
			CrawlStatus: models.CrawlStatusCompleted,
			DataJSON:    `{"title":"Page"}`,
		})
	}

	t.Run("returns results for owner", func(t *testing.T) {
		results, err := svc.GetJobResults(context.Background(), "user-results", "job-with-results")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("returns nil for non-owner", func(t *testing.T) {
		results, err := svc.GetJobResults(context.Background(), "user-other", "job-with-results")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if results != nil {
			t.Error("expected nil results for non-owner")
		}
	})
}

// ========================================
// CompleteExtractJob Tests
// ========================================

func TestJobService_CompleteExtractJob(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	cfg := &config.Config{}
	repos := &repository.Repositories{
		Job: mockJobRepo,
	}

	logger := slog.Default()
	svc := NewJobService(cfg, repos, logger)

	t.Run("completes running job", func(t *testing.T) {
		// Create a running job
		now := time.Now()
		mockJobRepo.Create(context.Background(), &models.Job{
			ID:        "job-to-complete",
			UserID:    "user-complete",
			Type:      models.JobTypeExtract,
			Status:    models.JobStatusRunning,
			StartedAt: &now,
			CreatedAt: now,
		})

		input := CompleteExtractJobInput{
			ResultJSON:       `{"title":"Test Title"}`,
			PageCount:        1,
			TokenUsageInput:  500,
			TokenUsageOutput: 100,
			CostUSD:          0.001,
			LLMProvider:      "openai",
			LLMModel:         "gpt-4o-mini",
		}

		err := svc.CompleteExtractJob(context.Background(), "job-to-complete", input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify job is updated
		job, _ := mockJobRepo.GetByID(context.Background(), "job-to-complete")
		if job == nil {
			t.Fatal("expected job")
		}
		if job.Status != models.JobStatusCompleted {
			t.Errorf("Status = %q, want %q", job.Status, models.JobStatusCompleted)
		}
		if job.ResultJSON != `{"title":"Test Title"}` {
			t.Errorf("ResultJSON = %q, want %q", job.ResultJSON, `{"title":"Test Title"}`)
		}
		if job.TokenUsageInput != 500 {
			t.Errorf("TokenUsageInput = %d, want 500", job.TokenUsageInput)
		}
		if job.LLMProvider != "openai" {
			t.Errorf("LLMProvider = %q, want %q", job.LLMProvider, "openai")
		}
		if job.CompletedAt == nil {
			t.Error("expected CompletedAt to be set")
		}
	})

	t.Run("fails for non-existent job", func(t *testing.T) {
		input := CompleteExtractJobInput{
			ResultJSON: `{}`,
		}
		err := svc.CompleteExtractJob(context.Background(), "job-nonexistent", input)
		if err == nil {
			t.Fatal("expected error for non-existent job")
		}
	})
}

// ========================================
// FailExtractJob Tests
// ========================================

func TestJobService_FailExtractJob(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	cfg := &config.Config{}
	repos := &repository.Repositories{
		Job: mockJobRepo,
	}

	logger := slog.Default()
	svc := NewJobService(cfg, repos, logger)

	t.Run("fails running job", func(t *testing.T) {
		// Create a running job
		now := time.Now()
		mockJobRepo.Create(context.Background(), &models.Job{
			ID:        "job-to-fail",
			UserID:    "user-fail",
			Type:      models.JobTypeExtract,
			Status:    models.JobStatusRunning,
			StartedAt: &now,
			CreatedAt: now,
		})

		input := FailExtractJobInput{
			ErrorMessage:  "Rate limit exceeded",
			ErrorDetails:  "OpenAI API returned 429: rate_limit_exceeded",
			ErrorCategory: "rate_limit",
			LLMProvider:   "openai",
			LLMModel:      "gpt-4o",
		}

		err := svc.FailExtractJob(context.Background(), "job-to-fail", input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify job is updated
		job, _ := mockJobRepo.GetByID(context.Background(), "job-to-fail")
		if job == nil {
			t.Fatal("expected job")
		}
		if job.Status != models.JobStatusFailed {
			t.Errorf("Status = %q, want %q", job.Status, models.JobStatusFailed)
		}
		if job.ErrorMessage != "Rate limit exceeded" {
			t.Errorf("ErrorMessage = %q, want %q", job.ErrorMessage, "Rate limit exceeded")
		}
		if job.ErrorCategory != "rate_limit" {
			t.Errorf("ErrorCategory = %q, want %q", job.ErrorCategory, "rate_limit")
		}
		if job.CompletedAt == nil {
			t.Error("expected CompletedAt to be set")
		}
	})

	t.Run("fails for non-existent job", func(t *testing.T) {
		input := FailExtractJobInput{
			ErrorMessage: "Error",
		}
		err := svc.FailExtractJob(context.Background(), "job-nonexistent", input)
		if err == nil {
			t.Fatal("expected error for non-existent job")
		}
	})
}

// ========================================
// CountActiveJobsByUser Tests
// ========================================

func TestJobService_CountActiveJobsByUser(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	cfg := &config.Config{}
	repos := &repository.Repositories{
		Job: mockJobRepo,
	}

	logger := slog.Default()
	svc := NewJobService(cfg, repos, logger)

	// Create jobs with various statuses
	now := time.Now()
	mockJobRepo.Create(context.Background(), &models.Job{
		ID:        "job-pending",
		UserID:    "user-count",
		Status:    models.JobStatusPending,
		CreatedAt: now,
	})
	mockJobRepo.Create(context.Background(), &models.Job{
		ID:        "job-running",
		UserID:    "user-count",
		Status:    models.JobStatusRunning,
		StartedAt: &now,
		CreatedAt: now,
	})
	mockJobRepo.Create(context.Background(), &models.Job{
		ID:        "job-completed",
		UserID:    "user-count",
		Status:    models.JobStatusCompleted,
		CreatedAt: now,
	})
	mockJobRepo.Create(context.Background(), &models.Job{
		ID:        "job-failed",
		UserID:    "user-count",
		Status:    models.JobStatusFailed,
		CreatedAt: now,
	})

	t.Run("counts only pending and running jobs", func(t *testing.T) {
		count, err := svc.CountActiveJobsByUser(context.Background(), "user-count")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if count != 2 {
			t.Errorf("expected 2 active jobs (pending + running), got %d", count)
		}
	})

	t.Run("returns 0 for user with no active jobs", func(t *testing.T) {
		count, err := svc.CountActiveJobsByUser(context.Background(), "user-no-active")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 active jobs, got %d", count)
		}
	})
}

// ========================================
// GetCrawlMap Tests
// ========================================

func TestJobService_GetCrawlMap(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	mockResultRepo := newMockJobResultRepository()
	cfg := &config.Config{}
	repos := &repository.Repositories{
		Job:       mockJobRepo,
		JobResult: mockResultRepo,
	}

	logger := slog.Default()
	svc := NewJobService(cfg, repos, logger)

	// Create a crawl job
	mockJobRepo.Create(context.Background(), &models.Job{
		ID:        "crawl-job",
		UserID:    "user-crawl",
		Type:      models.JobTypeCrawl,
		Status:    models.JobStatusCompleted,
		CreatedAt: time.Now(),
	})

	// Create an extract job
	mockJobRepo.Create(context.Background(), &models.Job{
		ID:        "extract-job",
		UserID:    "user-crawl",
		Type:      models.JobTypeExtract,
		Status:    models.JobStatusCompleted,
		CreatedAt: time.Now(),
	})

	t.Run("returns crawl map for crawl job", func(t *testing.T) {
		results, err := svc.GetCrawlMap(context.Background(), "user-crawl", "crawl-job")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		// Results may be empty but should not error
		_ = results
	})

	t.Run("fails for extract job", func(t *testing.T) {
		_, err := svc.GetCrawlMap(context.Background(), "user-crawl", "extract-job")
		if err == nil {
			t.Fatal("expected error for extract job")
		}
	})

	t.Run("fails for non-owner", func(t *testing.T) {
		results, err := svc.GetCrawlMap(context.Background(), "user-other", "crawl-job")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if results != nil {
			t.Error("expected nil results for non-owner")
		}
	})
}

// ========================================
// CountJobResults Tests
// ========================================

func TestJobService_CountJobResults(t *testing.T) {
	mockJobRepo := newMockJobRepository()
	mockResultRepo := newMockJobResultRepository()
	cfg := &config.Config{}
	repos := &repository.Repositories{
		Job:       mockJobRepo,
		JobResult: mockResultRepo,
	}

	logger := slog.Default()
	svc := NewJobService(cfg, repos, logger)

	// Create a job with results
	mockJobRepo.Create(context.Background(), &models.Job{
		ID:        "job-count-results",
		UserID:    "user-count-results",
		Type:      models.JobTypeCrawl,
		Status:    models.JobStatusCompleted,
		CreatedAt: time.Now(),
	})

	// Add some results
	for i := 0; i < 5; i++ {
		mockResultRepo.Create(context.Background(), &models.JobResult{
			ID:    "result-count-" + string(rune('0'+i)),
			JobID: "job-count-results",
		})
	}

	t.Run("returns count for owner", func(t *testing.T) {
		count, err := svc.CountJobResults(context.Background(), "user-count-results", "job-count-results")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if count != 5 {
			t.Errorf("expected 5 results, got %d", count)
		}
	})

	t.Run("returns 0 for non-owner", func(t *testing.T) {
		count, err := svc.CountJobResults(context.Background(), "user-other", "job-count-results")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 for non-owner, got %d", count)
		}
	})
}
