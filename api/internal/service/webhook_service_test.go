package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/models"
)

// ========================================
// Mock Repositories
// ========================================

type mockWebhookRepository struct {
	mu       sync.RWMutex
	webhooks map[string]*models.Webhook
}

func newMockWebhookRepository() *mockWebhookRepository {
	return &mockWebhookRepository{
		webhooks: make(map[string]*models.Webhook),
	}
}

func (m *mockWebhookRepository) Create(ctx context.Context, webhook *models.Webhook) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.webhooks[webhook.ID] = webhook
	return nil
}

func (m *mockWebhookRepository) GetByID(ctx context.Context, id string) (*models.Webhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.webhooks[id], nil
}

func (m *mockWebhookRepository) GetByUserID(ctx context.Context, userID string) ([]*models.Webhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.Webhook
	for _, w := range m.webhooks {
		if w.UserID == userID {
			result = append(result, w)
		}
	}
	return result, nil
}

func (m *mockWebhookRepository) GetActiveByUserID(ctx context.Context, userID string) ([]*models.Webhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.Webhook
	for _, w := range m.webhooks {
		if w.UserID == userID && w.IsActive {
			result = append(result, w)
		}
	}
	return result, nil
}

func (m *mockWebhookRepository) GetByUserAndName(ctx context.Context, userID, name string) (*models.Webhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.webhooks {
		if w.UserID == userID && w.Name == name {
			return w, nil
		}
	}
	return nil, nil
}

func (m *mockWebhookRepository) Update(ctx context.Context, webhook *models.Webhook) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.webhooks[webhook.ID] = webhook
	return nil
}

func (m *mockWebhookRepository) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.webhooks, id)
	return nil
}

type mockWebhookDeliveryRepository struct {
	mu         sync.RWMutex
	deliveries map[string]*models.WebhookDelivery
	counter    int
}

func newMockWebhookDeliveryRepository() *mockWebhookDeliveryRepository {
	return &mockWebhookDeliveryRepository{
		deliveries: make(map[string]*models.WebhookDelivery),
	}
}

func (m *mockWebhookDeliveryRepository) Create(ctx context.Context, delivery *models.WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if delivery.ID == "" {
		m.counter++
		delivery.ID = fmt.Sprintf("delivery-%d", m.counter)
	}
	delivery.CreatedAt = time.Now()
	m.deliveries[delivery.ID] = delivery
	return nil
}

func (m *mockWebhookDeliveryRepository) Update(ctx context.Context, delivery *models.WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deliveries[delivery.ID] = delivery
	return nil
}

func (m *mockWebhookDeliveryRepository) GetByID(ctx context.Context, id string) (*models.WebhookDelivery, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.deliveries[id], nil
}

func (m *mockWebhookDeliveryRepository) GetByJobID(ctx context.Context, jobID string) ([]*models.WebhookDelivery, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.WebhookDelivery
	for _, d := range m.deliveries {
		if d.JobID == jobID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *mockWebhookDeliveryRepository) GetByWebhookID(ctx context.Context, webhookID string, limit, offset int) ([]*models.WebhookDelivery, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.WebhookDelivery
	for _, d := range m.deliveries {
		if d.WebhookID != nil && *d.WebhookID == webhookID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *mockWebhookDeliveryRepository) GetPendingRetries(ctx context.Context, limit int) ([]*models.WebhookDelivery, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.WebhookDelivery
	now := time.Now()
	for _, d := range m.deliveries {
		if d.Status == models.WebhookDeliveryStatusRetrying && d.NextRetryAt != nil && d.NextRetryAt.Before(now) {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *mockWebhookDeliveryRepository) DeleteByJobIDs(ctx context.Context, jobIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, d := range m.deliveries {
		for _, jobID := range jobIDs {
			if d.JobID == jobID {
				delete(m.deliveries, id)
				break
			}
		}
	}
	return nil
}

// ========================================
// Tests
// ========================================

func TestComputeSignature(t *testing.T) {
	logger := slog.Default()
	svc := NewWebhookService(logger, nil, nil, nil)

	payload := []byte(`{"event":"job.completed","job_id":"job-123"}`)
	secret := "test-secret-key"

	signature := svc.computeSignature(payload, secret)

	// Verify manually
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	if signature != expected {
		t.Errorf("signature = %s, want %s", signature, expected)
	}
}

func TestIsEventSubscribed(t *testing.T) {
	logger := slog.Default()
	svc := NewWebhookService(logger, nil, nil, nil)

	tests := []struct {
		name      string
		events    []string
		eventType string
		want      bool
	}{
		{
			name:      "empty events matches all",
			events:    []string{},
			eventType: "job.completed",
			want:      true,
		},
		{
			name:      "wildcard matches all",
			events:    []string{"*"},
			eventType: "job.completed",
			want:      true,
		},
		{
			name:      "exact match",
			events:    []string{"job.completed", "job.failed"},
			eventType: "job.completed",
			want:      true,
		},
		{
			name:      "no match",
			events:    []string{"job.completed"},
			eventType: "job.failed",
			want:      false,
		},
		{
			name:      "multiple events with wildcard",
			events:    []string{"job.completed", "*"},
			eventType: "any.event",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.isEventSubscribed(tt.events, tt.eventType)
			if got != tt.want {
				t.Errorf("isEventSubscribed(%v, %s) = %v, want %v", tt.events, tt.eventType, got, tt.want)
			}
		})
	}
}

func TestDeliverWithTracking_Success(t *testing.T) {
	// Create test server that returns 200
	var receivedPayload []byte
	var receivedSignature string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPayload, _ = io.ReadAll(r.Body)
		receivedSignature = r.Header.Get("X-Refyne-Signature")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	logger := slog.Default()
	deliveryRepo := newMockWebhookDeliveryRepository()
	svc := NewWebhookService(logger, nil, deliveryRepo, nil)

	config := &WebhookConfig{
		URL:    server.URL,
		Secret: "my-secret",
		Events: []string{"*"},
	}

	result, err := svc.DeliverWithTracking(context.Background(), config, "job.completed", "job-123", map[string]string{"foo": "bar"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StatusCode != 200 {
		t.Errorf("status code = %d, want 200", result.StatusCode)
	}
	if result.ResponseBody != `{"status":"ok"}` {
		t.Errorf("response body = %s, want {\"status\":\"ok\"}", result.ResponseBody)
	}

	// Verify payload was received correctly
	var payload WebhookPayload
	if err := json.Unmarshal(receivedPayload, &payload); err != nil {
		t.Fatalf("failed to unmarshal received payload: %v", err)
	}
	if payload.Event != "job.completed" {
		t.Errorf("event = %s, want job.completed", payload.Event)
	}
	if payload.JobID != "job-123" {
		t.Errorf("job_id = %s, want job-123", payload.JobID)
	}

	// Verify signature
	if receivedSignature == "" {
		t.Error("expected signature header to be set")
	}

	// Verify delivery was tracked
	deliveries, _ := deliveryRepo.GetByJobID(context.Background(), "job-123")
	if len(deliveries) != 1 {
		t.Errorf("expected 1 delivery, got %d", len(deliveries))
	}
	if len(deliveries) > 0 && deliveries[0].Status != models.WebhookDeliveryStatusSuccess {
		t.Errorf("delivery status = %s, want success", deliveries[0].Status)
	}
}

func TestDeliverWithTracking_NotSubscribed(t *testing.T) {
	logger := slog.Default()
	svc := NewWebhookService(logger, nil, nil, nil)

	config := &WebhookConfig{
		URL:    "http://example.com/webhook",
		Events: []string{"job.completed"},
	}

	// Try to deliver an event that's not subscribed
	result, err := svc.DeliverWithTracking(context.Background(), config, "job.failed", "job-123", nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for non-subscribed event")
	}
}

func TestDeliverWithTracking_ServerError(t *testing.T) {
	// Create test server that returns 500
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer server.Close()

	logger := slog.Default()
	deliveryRepo := newMockWebhookDeliveryRepository()
	svc := NewWebhookService(logger, nil, deliveryRepo, nil)

	config := &WebhookConfig{
		URL:    server.URL,
		Events: []string{"*"},
	}

	result, err := svc.DeliverWithTracking(context.Background(), config, "job.completed", "job-123", nil)

	// Should have an error after retries
	if err == nil {
		t.Error("expected error for failed delivery")
	}
	if result.StatusCode != 500 {
		t.Errorf("status code = %d, want 500", result.StatusCode)
	}

	// Should have retried 3 times
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}

	// Verify delivery was marked as failed
	deliveries, _ := deliveryRepo.GetByJobID(context.Background(), "job-123")
	if len(deliveries) > 0 && deliveries[0].Status != models.WebhookDeliveryStatusFailed {
		t.Errorf("delivery status = %s, want failed", deliveries[0].Status)
	}
}

func TestDeliverWithTracking_RetryThenSuccess(t *testing.T) {
	// Create test server that fails twice then succeeds
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	logger := slog.Default()
	deliveryRepo := newMockWebhookDeliveryRepository()
	svc := NewWebhookService(logger, nil, deliveryRepo, nil)

	config := &WebhookConfig{
		URL:    server.URL,
		Events: []string{"*"},
	}

	result, _ := svc.DeliverWithTracking(context.Background(), config, "job.completed", "job-123", nil)

	// Note: result.Error may still be set from previous attempts even on success
	// The key check is the final status code
	if result.StatusCode != 200 {
		t.Errorf("final status code = %d, want 200", result.StatusCode)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}

	// Verify delivery was marked as success
	deliveries, _ := deliveryRepo.GetByJobID(context.Background(), "job-123")
	if len(deliveries) > 0 && deliveries[0].Status != models.WebhookDeliveryStatusSuccess {
		t.Errorf("delivery status = %s, want success", deliveries[0].Status)
	}
}

func TestDeliverWithTracking_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.Default()
	svc := NewWebhookService(logger, nil, nil, nil)

	config := &WebhookConfig{
		URL:    server.URL,
		Events: []string{"*"},
		Headers: []models.Header{
			{Name: "X-Custom-Header", Value: "custom-value"},
			{Name: "Authorization", Value: "Bearer token123"},
		},
	}

	_, err := svc.DeliverWithTracking(context.Background(), config, "job.completed", "job-123", nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("X-Custom-Header = %s, want custom-value", receivedHeaders.Get("X-Custom-Header"))
	}
	if receivedHeaders.Get("Authorization") != "Bearer token123" {
		t.Errorf("Authorization = %s, want Bearer token123", receivedHeaders.Get("Authorization"))
	}
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", receivedHeaders.Get("Content-Type"))
	}
}

func TestGetDeliveriesForJob(t *testing.T) {
	logger := slog.Default()
	deliveryRepo := newMockWebhookDeliveryRepository()
	svc := NewWebhookService(logger, nil, deliveryRepo, nil)

	// Create some deliveries
	deliveryRepo.Create(context.Background(), &models.WebhookDelivery{
		JobID:     "job-123",
		EventType: "job.completed",
		Status:    models.WebhookDeliveryStatusSuccess,
	})
	deliveryRepo.Create(context.Background(), &models.WebhookDelivery{
		JobID:     "job-123",
		EventType: "job.progress",
		Status:    models.WebhookDeliveryStatusFailed,
	})
	deliveryRepo.Create(context.Background(), &models.WebhookDelivery{
		JobID:     "job-456",
		EventType: "job.completed",
		Status:    models.WebhookDeliveryStatusSuccess,
	})

	deliveries, err := svc.GetDeliveriesForJob(context.Background(), "job-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deliveries) != 2 {
		t.Errorf("got %d deliveries, want 2", len(deliveries))
	}
}

func TestSendForJob_EphemeralAndPersistent(t *testing.T) {
	ephemeralCalled := make(chan bool, 1)
	persistentCalled := make(chan bool, 1)

	// Ephemeral webhook server
	ephemeralServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ephemeralCalled <- true
		w.WriteHeader(http.StatusOK)
	}))
	defer ephemeralServer.Close()

	// Persistent webhook server
	persistentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		persistentCalled <- true
		w.WriteHeader(http.StatusOK)
	}))
	defer persistentServer.Close()

	logger := slog.Default()
	webhookRepo := newMockWebhookRepository()
	deliveryRepo := newMockWebhookDeliveryRepository()
	svc := NewWebhookService(logger, webhookRepo, deliveryRepo, nil)

	// Create a persistent webhook
	webhookRepo.Create(context.Background(), &models.Webhook{
		ID:       "webhook-1",
		UserID:   "user-1",
		Name:     "Test Webhook",
		URL:      persistentServer.URL,
		Events:   []string{"*"},
		IsActive: true,
	})

	ephemeralConfig := &WebhookConfig{
		URL:    ephemeralServer.URL,
		Events: []string{"*"},
	}

	// Send for job - this should trigger both webhooks
	svc.SendForJob(context.Background(), "user-1", "job.completed", "job-123", nil, ephemeralConfig)

	// Wait for both webhooks to be called
	select {
	case <-ephemeralCalled:
	case <-time.After(5 * time.Second):
		t.Error("ephemeral webhook not called within timeout")
	}

	select {
	case <-persistentCalled:
	case <-time.After(5 * time.Second):
		t.Error("persistent webhook not called within timeout")
	}
}

func TestSendForJob_DecryptsSecret(t *testing.T) {
	signatureChan := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signatureChan <- r.Header.Get("X-Refyne-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.Default()
	webhookRepo := newMockWebhookRepository()
	deliveryRepo := newMockWebhookDeliveryRepository()

	// Create encryptor
	encryptor, err := crypto.NewEncryptor([]byte(strings.Repeat("0123456789abcdef", 2))) // 32-byte key
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	svc := NewWebhookService(logger, webhookRepo, deliveryRepo, encryptor)

	// Encrypt the secret
	encryptedSecret, err := encryptor.Encrypt("my-webhook-secret")
	if err != nil {
		t.Fatalf("failed to encrypt secret: %v", err)
	}

	// Create a persistent webhook with encrypted secret
	webhookRepo.Create(context.Background(), &models.Webhook{
		ID:              "webhook-1",
		UserID:          "user-1",
		Name:            "Test Webhook",
		URL:             server.URL,
		SecretEncrypted: encryptedSecret,
		Events:          []string{"*"},
		IsActive:        true,
	})

	// Send for job
	svc.SendForJob(context.Background(), "user-1", "job.completed", "job-123", nil, nil)

	// Wait for webhook to be called with proper synchronization
	select {
	case receivedSignature := <-signatureChan:
		// Verify that signature was generated (meaning secret was decrypted)
		if receivedSignature == "" {
			t.Error("expected signature to be set (secret should be decrypted)")
		}
	case <-time.After(5 * time.Second):
		t.Error("webhook not called within timeout")
	}
}

func TestProcessPendingRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.Default()
	webhookRepo := newMockWebhookRepository()
	deliveryRepo := newMockWebhookDeliveryRepository()
	svc := NewWebhookService(logger, webhookRepo, deliveryRepo, nil)

	// Create a webhook
	webhookID := "webhook-1"
	webhookRepo.Create(context.Background(), &models.Webhook{
		ID:       webhookID,
		UserID:   "user-1",
		Name:     "Test Webhook",
		URL:      server.URL,
		Events:   []string{"*"},
		IsActive: true,
	})

	// Create a delivery that's ready for retry
	pastTime := time.Now().Add(-1 * time.Minute)
	deliveryRepo.Create(context.Background(), &models.WebhookDelivery{
		ID:            "delivery-1",
		WebhookID:     &webhookID,
		JobID:         "job-123",
		EventType:     "job.completed",
		URL:           server.URL,
		PayloadJSON:   `{"event":"job.completed","timestamp":"2026-01-18T00:00:00Z","job_id":"job-123","data":null}`,
		Status:        models.WebhookDeliveryStatusRetrying,
		AttemptNumber: 1,
		MaxAttempts:   3,
		NextRetryAt:   &pastTime,
	})

	// Process retries
	processed, err := svc.ProcessPendingRetries(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if processed != 1 {
		t.Errorf("processed = %d, want 1", processed)
	}

	// Verify delivery was marked as success
	delivery, _ := deliveryRepo.GetByID(context.Background(), "delivery-1")
	if delivery.Status != models.WebhookDeliveryStatusSuccess {
		t.Errorf("delivery status = %s, want success", delivery.Status)
	}
}

func TestProcessPendingRetries_WebhookDeleted(t *testing.T) {
	logger := slog.Default()
	webhookRepo := newMockWebhookRepository()
	deliveryRepo := newMockWebhookDeliveryRepository()
	svc := NewWebhookService(logger, webhookRepo, deliveryRepo, nil)

	// Create a delivery that references a non-existent webhook
	webhookID := "deleted-webhook"
	pastTime := time.Now().Add(-1 * time.Minute)
	deliveryRepo.Create(context.Background(), &models.WebhookDelivery{
		ID:            "delivery-1",
		WebhookID:     &webhookID,
		JobID:         "job-123",
		EventType:     "job.completed",
		URL:           "http://example.com/webhook",
		PayloadJSON:   `{}`,
		Status:        models.WebhookDeliveryStatusRetrying,
		AttemptNumber: 1,
		MaxAttempts:   3,
		NextRetryAt:   &pastTime,
	})

	// Process retries
	processed, err := svc.ProcessPendingRetries(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if processed != 0 {
		t.Errorf("processed = %d, want 0 (webhook was deleted)", processed)
	}

	// Verify delivery was marked as failed
	delivery, _ := deliveryRepo.GetByID(context.Background(), "delivery-1")
	if delivery.Status != models.WebhookDeliveryStatusFailed {
		t.Errorf("delivery status = %s, want failed", delivery.Status)
	}
	if !strings.Contains(delivery.ErrorMessage, "deleted or deactivated") {
		t.Errorf("error message = %s, want to contain 'deleted or deactivated'", delivery.ErrorMessage)
	}
}

func TestWebhookError(t *testing.T) {
	err := &WebhookError{StatusCode: 503}
	errMsg := err.Error()

	if !strings.Contains(errMsg, "503") && !strings.Contains(errMsg, "Service Unavailable") {
		t.Errorf("error message = %s, want to contain status info", errMsg)
	}
}
