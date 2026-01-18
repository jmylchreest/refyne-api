package repository

import (
	"context"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// ========================================
// WebhookRepository Tests
// ========================================

func TestWebhookRepository_Create(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	webhook := &models.Webhook{
		UserID:          "user-1",
		Name:            "Test Webhook",
		URL:             "https://example.com/webhook",
		SecretEncrypted: "encrypted-secret",
		Events:          []string{"job.completed", "job.failed"},
		IsActive:        true,
	}

	err := repos.Webhook.Create(ctx, webhook)
	if err != nil {
		t.Fatalf("failed to create webhook: %v", err)
	}

	if webhook.ID == "" {
		t.Error("expected ID to be generated")
	}

	// Verify by fetching
	fetched, err := repos.Webhook.GetByID(ctx, webhook.ID)
	if err != nil {
		t.Fatalf("failed to fetch webhook: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected webhook, got nil")
	}
	if fetched.Name != "Test Webhook" {
		t.Errorf("Name = %q, want %q", fetched.Name, "Test Webhook")
	}
	if fetched.URL != "https://example.com/webhook" {
		t.Errorf("URL = %q, want %q", fetched.URL, "https://example.com/webhook")
	}
	if len(fetched.Events) != 2 {
		t.Errorf("Events length = %d, want 2", len(fetched.Events))
	}
	if !fetched.IsActive {
		t.Error("expected IsActive to be true")
	}
}

func TestWebhookRepository_CreateWithHeaders(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	webhook := &models.Webhook{
		UserID: "user-2",
		Name:   "Webhook With Headers",
		URL:    "https://example.com/webhook",
		Events: []string{"*"},
		Headers: []models.Header{
			{Name: "X-Custom-Header", Value: "custom-value"},
			{Name: "Authorization", Value: "Bearer token123"},
		},
		IsActive: true,
	}

	err := repos.Webhook.Create(ctx, webhook)
	if err != nil {
		t.Fatalf("failed to create webhook: %v", err)
	}

	fetched, err := repos.Webhook.GetByID(ctx, webhook.ID)
	if err != nil {
		t.Fatalf("failed to fetch webhook: %v", err)
	}
	if len(fetched.Headers) != 2 {
		t.Errorf("Headers length = %d, want 2", len(fetched.Headers))
	}
	if fetched.Headers[0].Name != "X-Custom-Header" {
		t.Errorf("Headers[0].Name = %q, want %q", fetched.Headers[0].Name, "X-Custom-Header")
	}
}

func TestWebhookRepository_GetByUserID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create webhooks for different users
	for i := 0; i < 3; i++ {
		repos.Webhook.Create(ctx, &models.Webhook{
			UserID:   "user-list",
			Name:     "Webhook " + string(rune('A'+i)),
			URL:      "https://example.com/webhook",
			Events:   []string{"*"},
			IsActive: true,
		})
	}
	repos.Webhook.Create(ctx, &models.Webhook{
		UserID:   "user-other",
		Name:     "Other Webhook",
		URL:      "https://other.com/webhook",
		Events:   []string{"*"},
		IsActive: true,
	})

	webhooks, err := repos.Webhook.GetByUserID(ctx, "user-list")
	if err != nil {
		t.Fatalf("failed to get webhooks: %v", err)
	}
	if len(webhooks) != 3 {
		t.Errorf("expected 3 webhooks, got %d", len(webhooks))
	}

	// Should be ordered by name
	if webhooks[0].Name != "Webhook A" {
		t.Errorf("expected first webhook to be 'Webhook A', got %q", webhooks[0].Name)
	}
}

func TestWebhookRepository_GetActiveByUserID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create mix of active and inactive webhooks
	repos.Webhook.Create(ctx, &models.Webhook{
		UserID:   "user-active",
		Name:     "Active Webhook 1",
		URL:      "https://example.com/webhook1",
		Events:   []string{"*"},
		IsActive: true,
	})
	repos.Webhook.Create(ctx, &models.Webhook{
		UserID:   "user-active",
		Name:     "Inactive Webhook",
		URL:      "https://example.com/webhook2",
		Events:   []string{"*"},
		IsActive: false,
	})
	repos.Webhook.Create(ctx, &models.Webhook{
		UserID:   "user-active",
		Name:     "Active Webhook 2",
		URL:      "https://example.com/webhook3",
		Events:   []string{"*"},
		IsActive: true,
	})

	webhooks, err := repos.Webhook.GetActiveByUserID(ctx, "user-active")
	if err != nil {
		t.Fatalf("failed to get active webhooks: %v", err)
	}
	if len(webhooks) != 2 {
		t.Errorf("expected 2 active webhooks, got %d", len(webhooks))
	}

	for _, w := range webhooks {
		if !w.IsActive {
			t.Errorf("got inactive webhook %q in active list", w.Name)
		}
	}
}

func TestWebhookRepository_GetByUserAndName(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	repos.Webhook.Create(ctx, &models.Webhook{
		UserID:   "user-unique",
		Name:     "Unique Name",
		URL:      "https://example.com/webhook",
		Events:   []string{"*"},
		IsActive: true,
	})

	webhook, err := repos.Webhook.GetByUserAndName(ctx, "user-unique", "Unique Name")
	if err != nil {
		t.Fatalf("failed to get webhook: %v", err)
	}
	if webhook == nil {
		t.Fatal("expected webhook, got nil")
	}
	if webhook.Name != "Unique Name" {
		t.Errorf("Name = %q, want %q", webhook.Name, "Unique Name")
	}

	// Non-existent
	webhook, err = repos.Webhook.GetByUserAndName(ctx, "user-unique", "Non-existent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if webhook != nil {
		t.Error("expected nil for non-existent webhook")
	}
}

func TestWebhookRepository_Update(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	webhook := &models.Webhook{
		UserID:   "user-update",
		Name:     "Original Name",
		URL:      "https://original.com/webhook",
		Events:   []string{"job.completed"},
		IsActive: true,
	}
	repos.Webhook.Create(ctx, webhook)

	// Update
	webhook.Name = "Updated Name"
	webhook.URL = "https://updated.com/webhook"
	webhook.Events = []string{"job.completed", "job.failed"}
	webhook.IsActive = false

	err := repos.Webhook.Update(ctx, webhook)
	if err != nil {
		t.Fatalf("failed to update webhook: %v", err)
	}

	// Verify
	fetched, _ := repos.Webhook.GetByID(ctx, webhook.ID)
	if fetched.Name != "Updated Name" {
		t.Errorf("Name = %q, want %q", fetched.Name, "Updated Name")
	}
	if fetched.URL != "https://updated.com/webhook" {
		t.Errorf("URL = %q, want %q", fetched.URL, "https://updated.com/webhook")
	}
	if len(fetched.Events) != 2 {
		t.Errorf("Events length = %d, want 2", len(fetched.Events))
	}
	if fetched.IsActive {
		t.Error("expected IsActive to be false")
	}
}

func TestWebhookRepository_Delete(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	webhook := &models.Webhook{
		UserID:   "user-delete",
		Name:     "To Delete",
		URL:      "https://example.com/webhook",
		Events:   []string{"*"},
		IsActive: true,
	}
	repos.Webhook.Create(ctx, webhook)

	err := repos.Webhook.Delete(ctx, webhook.ID)
	if err != nil {
		t.Fatalf("failed to delete webhook: %v", err)
	}

	// Verify deleted
	fetched, _ := repos.Webhook.GetByID(ctx, webhook.ID)
	if fetched != nil {
		t.Error("expected webhook to be deleted")
	}
}

func TestWebhookRepository_GetByID_NotFound(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	webhook, err := repos.Webhook.GetByID(ctx, "non-existent-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if webhook != nil {
		t.Error("expected nil for non-existent ID")
	}
}

// ========================================
// WebhookDeliveryRepository Tests
// ========================================

// createTestJob creates a job for testing webhook deliveries (foreign key constraint)
func createTestJob(t *testing.T, repos *Repositories, ctx context.Context, jobID string) {
	t.Helper()
	job := &models.Job{
		ID:     jobID,
		UserID: "test-user",
		Type:   models.JobTypeExtract,
		Status: models.JobStatusCompleted,
	}
	if err := repos.Job.Create(ctx, job); err != nil {
		t.Fatalf("failed to create test job: %v", err)
	}
}

func TestWebhookDeliveryRepository_Create(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create a webhook first
	webhook := &models.Webhook{
		UserID:   "user-delivery",
		Name:     "Delivery Test",
		URL:      "https://example.com/webhook",
		Events:   []string{"*"},
		IsActive: true,
	}
	repos.Webhook.Create(ctx, webhook)

	// Create a job (foreign key constraint)
	createTestJob(t, repos, ctx, "job-123")

	delivery := &models.WebhookDelivery{
		WebhookID:     &webhook.ID,
		JobID:         "job-123",
		EventType:     "job.completed",
		URL:           "https://example.com/webhook",
		PayloadJSON:   `{"event":"job.completed","job_id":"job-123"}`,
		Status:        models.WebhookDeliveryStatusPending,
		AttemptNumber: 1,
		MaxAttempts:   3,
	}

	err := repos.WebhookDelivery.Create(ctx, delivery)
	if err != nil {
		t.Fatalf("failed to create delivery: %v", err)
	}

	if delivery.ID == "" {
		t.Error("expected ID to be generated")
	}

	// Verify
	fetched, err := repos.WebhookDelivery.GetByID(ctx, delivery.ID)
	if err != nil {
		t.Fatalf("failed to fetch delivery: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected delivery, got nil")
	}
	if fetched.JobID != "job-123" {
		t.Errorf("JobID = %q, want %q", fetched.JobID, "job-123")
	}
	if fetched.EventType != "job.completed" {
		t.Errorf("EventType = %q, want %q", fetched.EventType, "job.completed")
	}
	if fetched.Status != models.WebhookDeliveryStatusPending {
		t.Errorf("Status = %q, want %q", fetched.Status, models.WebhookDeliveryStatusPending)
	}
}

func TestWebhookDeliveryRepository_CreateWithHeaders(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create a job (foreign key constraint)
	createTestJob(t, repos, ctx, "job-headers")

	delivery := &models.WebhookDelivery{
		JobID:       "job-headers",
		EventType:   "job.completed",
		URL:         "https://example.com/webhook",
		PayloadJSON: `{}`,
		RequestHeaders: []models.Header{
			{Name: "X-Custom", Value: "value"},
		},
		Status:        models.WebhookDeliveryStatusPending,
		AttemptNumber: 1,
		MaxAttempts:   3,
	}

	err := repos.WebhookDelivery.Create(ctx, delivery)
	if err != nil {
		t.Fatalf("failed to create delivery: %v", err)
	}

	fetched, _ := repos.WebhookDelivery.GetByID(ctx, delivery.ID)
	if len(fetched.RequestHeaders) != 1 {
		t.Errorf("RequestHeaders length = %d, want 1", len(fetched.RequestHeaders))
	}
}

func TestWebhookDeliveryRepository_Update(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create a job (foreign key constraint)
	createTestJob(t, repos, ctx, "job-update")

	delivery := &models.WebhookDelivery{
		JobID:         "job-update",
		EventType:     "job.completed",
		URL:           "https://example.com/webhook",
		PayloadJSON:   `{}`,
		Status:        models.WebhookDeliveryStatusPending,
		AttemptNumber: 1,
		MaxAttempts:   3,
	}
	repos.WebhookDelivery.Create(ctx, delivery)

	// Update with response
	statusCode := 200
	responseTimeMs := 150
	now := time.Now()
	delivery.StatusCode = &statusCode
	delivery.ResponseBody = `{"success":true}`
	delivery.ResponseTimeMs = &responseTimeMs
	delivery.Status = models.WebhookDeliveryStatusSuccess
	delivery.DeliveredAt = &now

	err := repos.WebhookDelivery.Update(ctx, delivery)
	if err != nil {
		t.Fatalf("failed to update delivery: %v", err)
	}

	// Verify
	fetched, _ := repos.WebhookDelivery.GetByID(ctx, delivery.ID)
	if fetched.Status != models.WebhookDeliveryStatusSuccess {
		t.Errorf("Status = %q, want %q", fetched.Status, models.WebhookDeliveryStatusSuccess)
	}
	if fetched.StatusCode == nil || *fetched.StatusCode != 200 {
		t.Error("expected StatusCode to be 200")
	}
	if fetched.DeliveredAt == nil {
		t.Error("expected DeliveredAt to be set")
	}
}

func TestWebhookDeliveryRepository_GetByJobID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create jobs (foreign key constraint)
	createTestJob(t, repos, ctx, "job-multi")
	createTestJob(t, repos, ctx, "job-other")

	// Create deliveries for different jobs
	for i := 0; i < 3; i++ {
		repos.WebhookDelivery.Create(ctx, &models.WebhookDelivery{
			JobID:         "job-multi",
			EventType:     "job.completed",
			URL:           "https://example.com/webhook",
			PayloadJSON:   `{}`,
			Status:        models.WebhookDeliveryStatusSuccess,
			AttemptNumber: 1,
			MaxAttempts:   3,
		})
	}
	repos.WebhookDelivery.Create(ctx, &models.WebhookDelivery{
		JobID:         "job-other",
		EventType:     "job.completed",
		URL:           "https://example.com/webhook",
		PayloadJSON:   `{}`,
		Status:        models.WebhookDeliveryStatusSuccess,
		AttemptNumber: 1,
		MaxAttempts:   3,
	})

	deliveries, err := repos.WebhookDelivery.GetByJobID(ctx, "job-multi")
	if err != nil {
		t.Fatalf("failed to get deliveries: %v", err)
	}
	if len(deliveries) != 3 {
		t.Errorf("expected 3 deliveries, got %d", len(deliveries))
	}
}

func TestWebhookDeliveryRepository_GetByWebhookID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create a webhook
	webhook := &models.Webhook{
		UserID:   "user-wh-del",
		Name:     "Delivery List Test",
		URL:      "https://example.com/webhook",
		Events:   []string{"*"},
		IsActive: true,
	}
	repos.Webhook.Create(ctx, webhook)

	// Create jobs (foreign key constraint)
	for i := 0; i < 5; i++ {
		createTestJob(t, repos, ctx, "job-wh-"+string(rune('0'+i)))
	}

	// Create deliveries
	for i := 0; i < 5; i++ {
		repos.WebhookDelivery.Create(ctx, &models.WebhookDelivery{
			WebhookID:     &webhook.ID,
			JobID:         "job-wh-" + string(rune('0'+i)),
			EventType:     "job.completed",
			URL:           "https://example.com/webhook",
			PayloadJSON:   `{}`,
			Status:        models.WebhookDeliveryStatusSuccess,
			AttemptNumber: 1,
			MaxAttempts:   3,
		})
	}

	// Get with pagination
	deliveries, err := repos.WebhookDelivery.GetByWebhookID(ctx, webhook.ID, 3, 0)
	if err != nil {
		t.Fatalf("failed to get deliveries: %v", err)
	}
	if len(deliveries) != 3 {
		t.Errorf("expected 3 deliveries (limit), got %d", len(deliveries))
	}

	// Get second page
	deliveries, err = repos.WebhookDelivery.GetByWebhookID(ctx, webhook.ID, 3, 3)
	if err != nil {
		t.Fatalf("failed to get deliveries: %v", err)
	}
	if len(deliveries) != 2 {
		t.Errorf("expected 2 deliveries (second page), got %d", len(deliveries))
	}
}

func TestWebhookDeliveryRepository_GetPendingRetries(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create jobs (foreign key constraint)
	createTestJob(t, repos, ctx, "job-retry-ready")
	createTestJob(t, repos, ctx, "job-retry-future")
	createTestJob(t, repos, ctx, "job-success")

	// Create retrying delivery with past next_retry_at
	pastTime := time.Now().Add(-time.Minute)
	futureTime := time.Now().Add(time.Hour)

	repos.WebhookDelivery.Create(ctx, &models.WebhookDelivery{
		JobID:         "job-retry-ready",
		EventType:     "job.completed",
		URL:           "https://example.com/webhook",
		PayloadJSON:   `{}`,
		Status:        models.WebhookDeliveryStatusRetrying,
		AttemptNumber: 1,
		MaxAttempts:   3,
		NextRetryAt:   &pastTime,
	})

	// Create retrying delivery with future next_retry_at
	repos.WebhookDelivery.Create(ctx, &models.WebhookDelivery{
		JobID:         "job-retry-future",
		EventType:     "job.completed",
		URL:           "https://example.com/webhook",
		PayloadJSON:   `{}`,
		Status:        models.WebhookDeliveryStatusRetrying,
		AttemptNumber: 1,
		MaxAttempts:   3,
		NextRetryAt:   &futureTime,
	})

	// Create non-retrying delivery
	repos.WebhookDelivery.Create(ctx, &models.WebhookDelivery{
		JobID:         "job-success",
		EventType:     "job.completed",
		URL:           "https://example.com/webhook",
		PayloadJSON:   `{}`,
		Status:        models.WebhookDeliveryStatusSuccess,
		AttemptNumber: 1,
		MaxAttempts:   3,
	})

	deliveries, err := repos.WebhookDelivery.GetPendingRetries(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get pending retries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Errorf("expected 1 pending retry, got %d", len(deliveries))
	}
	if len(deliveries) > 0 && deliveries[0].JobID != "job-retry-ready" {
		t.Errorf("expected job-retry-ready, got %q", deliveries[0].JobID)
	}
}

func TestWebhookDeliveryRepository_DeleteByJobIDs(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create jobs (foreign key constraint)
	createTestJob(t, repos, ctx, "job-del-1")
	createTestJob(t, repos, ctx, "job-del-2")
	createTestJob(t, repos, ctx, "job-keep")

	// Create deliveries
	repos.WebhookDelivery.Create(ctx, &models.WebhookDelivery{
		JobID:         "job-del-1",
		EventType:     "job.completed",
		URL:           "https://example.com/webhook",
		PayloadJSON:   `{}`,
		Status:        models.WebhookDeliveryStatusSuccess,
		AttemptNumber: 1,
		MaxAttempts:   3,
	})
	repos.WebhookDelivery.Create(ctx, &models.WebhookDelivery{
		JobID:         "job-del-2",
		EventType:     "job.completed",
		URL:           "https://example.com/webhook",
		PayloadJSON:   `{}`,
		Status:        models.WebhookDeliveryStatusSuccess,
		AttemptNumber: 1,
		MaxAttempts:   3,
	})
	repos.WebhookDelivery.Create(ctx, &models.WebhookDelivery{
		JobID:         "job-keep",
		EventType:     "job.completed",
		URL:           "https://example.com/webhook",
		PayloadJSON:   `{}`,
		Status:        models.WebhookDeliveryStatusSuccess,
		AttemptNumber: 1,
		MaxAttempts:   3,
	})

	err := repos.WebhookDelivery.DeleteByJobIDs(ctx, []string{"job-del-1", "job-del-2"})
	if err != nil {
		t.Fatalf("failed to delete deliveries: %v", err)
	}

	// Verify deleted
	deliveries1, _ := repos.WebhookDelivery.GetByJobID(ctx, "job-del-1")
	if len(deliveries1) != 0 {
		t.Error("expected job-del-1 deliveries to be deleted")
	}

	deliveries2, _ := repos.WebhookDelivery.GetByJobID(ctx, "job-del-2")
	if len(deliveries2) != 0 {
		t.Error("expected job-del-2 deliveries to be deleted")
	}

	// Verify kept
	deliveriesKeep, _ := repos.WebhookDelivery.GetByJobID(ctx, "job-keep")
	if len(deliveriesKeep) != 1 {
		t.Error("expected job-keep delivery to be kept")
	}
}

func TestWebhookDeliveryRepository_DeleteByJobIDs_Empty(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Should handle empty slice without error
	err := repos.WebhookDelivery.DeleteByJobIDs(ctx, []string{})
	if err != nil {
		t.Fatalf("unexpected error for empty slice: %v", err)
	}
}
