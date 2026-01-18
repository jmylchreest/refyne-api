package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	appconfig "github.com/jmylchreest/refyne-api/internal/config"
)

// ========================================
// StorageService Tests
// ========================================

// ----------------------------------------
// Constructor Tests
// ----------------------------------------

func TestNewStorageService_Disabled(t *testing.T) {
	cfg := &appconfig.Config{
		StorageEnabled: false,
	}
	logger := slog.Default()

	svc, err := NewStorageService(cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected service, got nil")
	}
	if svc.IsEnabled() {
		t.Error("expected storage to be disabled")
	}
	if svc.Client() != nil {
		t.Error("expected client to be nil when disabled")
	}
	if svc.Bucket() != "" {
		t.Error("expected bucket to be empty when disabled")
	}
}

// Note: Testing with storage enabled requires actual S3/Tigris credentials
// or a local mock S3 server (like MinIO). These are integration tests.

// ----------------------------------------
// Disabled Storage Behavior Tests
// ----------------------------------------

func TestStorageService_StoreJobResults_Disabled(t *testing.T) {
	cfg := &appconfig.Config{
		StorageEnabled: false,
	}
	svc, _ := NewStorageService(cfg, slog.Default())
	ctx := context.Background()

	results := &JobResults{
		JobID:  "test-job",
		UserID: "user-1",
		Status: "completed",
	}

	// Should silently succeed when disabled
	err := svc.StoreJobResults(ctx, results)
	if err != nil {
		t.Errorf("expected no error when disabled, got: %v", err)
	}
}

func TestStorageService_GetJobResults_Disabled(t *testing.T) {
	cfg := &appconfig.Config{
		StorageEnabled: false,
	}
	svc, _ := NewStorageService(cfg, slog.Default())
	ctx := context.Background()

	_, err := svc.GetJobResults(ctx, "test-job")
	if err == nil {
		t.Error("expected error when storage is disabled")
	}
}

func TestStorageService_GetJobResultsPresignedURL_Disabled(t *testing.T) {
	cfg := &appconfig.Config{
		StorageEnabled: false,
	}
	svc, _ := NewStorageService(cfg, slog.Default())
	ctx := context.Background()

	_, err := svc.GetJobResultsPresignedURL(ctx, "test-job", time.Hour)
	if err == nil {
		t.Error("expected error when storage is disabled")
	}
}

func TestStorageService_DeleteJobResults_Disabled(t *testing.T) {
	cfg := &appconfig.Config{
		StorageEnabled: false,
	}
	svc, _ := NewStorageService(cfg, slog.Default())
	ctx := context.Background()

	// Should silently succeed when disabled
	err := svc.DeleteJobResults(ctx, "test-job")
	if err != nil {
		t.Errorf("expected no error when disabled, got: %v", err)
	}
}

func TestStorageService_DeleteOldJobResults_Disabled(t *testing.T) {
	cfg := &appconfig.Config{
		StorageEnabled: false,
	}
	svc, _ := NewStorageService(cfg, slog.Default())
	ctx := context.Background()

	deleted, err := svc.DeleteOldJobResults(ctx, 24*time.Hour)
	if err != nil {
		t.Errorf("expected no error when disabled, got: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted when disabled, got %d", deleted)
	}
}

func TestStorageService_JobResultExists_Disabled(t *testing.T) {
	cfg := &appconfig.Config{
		StorageEnabled: false,
	}
	svc, _ := NewStorageService(cfg, slog.Default())
	ctx := context.Background()

	exists, err := svc.JobResultExists(ctx, "test-job")
	if err != nil {
		t.Errorf("expected no error when disabled, got: %v", err)
	}
	if exists {
		t.Error("expected false when disabled")
	}
}

// ----------------------------------------
// Struct Tests
// ----------------------------------------

func TestJobResultData_Fields(t *testing.T) {
	now := time.Now()
	data := json.RawMessage(`{"title":"Test Product","price":1999}`)

	result := JobResultData{
		ID:        "result-1",
		URL:       "https://example.com/product/1",
		Data:      data,
		CreatedAt: now,
	}

	if result.ID != "result-1" {
		t.Errorf("ID = %q, want %q", result.ID, "result-1")
	}
	if result.URL != "https://example.com/product/1" {
		t.Errorf("URL = %q, want %q", result.URL, "https://example.com/product/1")
	}
	if string(result.Data) != `{"title":"Test Product","price":1999}` {
		t.Errorf("Data = %s, want %s", result.Data, `{"title":"Test Product","price":1999}`)
	}
	if result.CreatedAt != now {
		t.Error("CreatedAt mismatch")
	}
}

func TestJobResultData_JSONMarshal(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	result := JobResultData{
		ID:        "result-1",
		URL:       "https://example.com/page",
		Data:      json.RawMessage(`{"key":"value"}`),
		CreatedAt: now,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled JobResultData
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.ID != result.ID {
		t.Errorf("ID = %q, want %q", unmarshaled.ID, result.ID)
	}
	if unmarshaled.URL != result.URL {
		t.Errorf("URL = %q, want %q", unmarshaled.URL, result.URL)
	}
}

func TestJobResults_Fields(t *testing.T) {
	now := time.Now()
	results := JobResults{
		JobID:      "job-1",
		UserID:     "user-1",
		Status:     "completed",
		TotalPages: 10,
		Results: []JobResultData{
			{ID: "r1", URL: "https://example.com/1"},
			{ID: "r2", URL: "https://example.com/2"},
		},
		CompletedAt: now,
	}

	if results.JobID != "job-1" {
		t.Errorf("JobID = %q, want %q", results.JobID, "job-1")
	}
	if results.UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", results.UserID, "user-1")
	}
	if results.Status != "completed" {
		t.Errorf("Status = %q, want %q", results.Status, "completed")
	}
	if results.TotalPages != 10 {
		t.Errorf("TotalPages = %d, want 10", results.TotalPages)
	}
	if len(results.Results) != 2 {
		t.Errorf("Results count = %d, want 2", len(results.Results))
	}
	if results.CompletedAt != now {
		t.Error("CompletedAt mismatch")
	}
}

func TestJobResults_JSONMarshal(t *testing.T) {
	results := JobResults{
		JobID:      "job-1",
		UserID:     "user-1",
		Status:     "completed",
		TotalPages: 5,
		Results: []JobResultData{
			{ID: "r1", URL: "https://example.com/1", Data: json.RawMessage(`{"title":"Test"}`)},
		},
		CompletedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	data, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled JobResults
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.JobID != results.JobID {
		t.Errorf("JobID = %q, want %q", unmarshaled.JobID, results.JobID)
	}
	if unmarshaled.TotalPages != results.TotalPages {
		t.Errorf("TotalPages = %d, want %d", unmarshaled.TotalPages, results.TotalPages)
	}
	if len(unmarshaled.Results) != 1 {
		t.Errorf("Results count = %d, want 1", len(unmarshaled.Results))
	}
}

// ----------------------------------------
// Accessor Tests
// ----------------------------------------

func TestStorageService_IsEnabled(t *testing.T) {
	cfg := &appconfig.Config{StorageEnabled: false}
	svc, _ := NewStorageService(cfg, slog.Default())

	if svc.IsEnabled() != false {
		t.Error("IsEnabled() should return false when disabled")
	}
}

func TestStorageService_Client_Disabled(t *testing.T) {
	cfg := &appconfig.Config{StorageEnabled: false}
	svc, _ := NewStorageService(cfg, slog.Default())

	if svc.Client() != nil {
		t.Error("Client() should return nil when disabled")
	}
}

func TestStorageService_Bucket_Disabled(t *testing.T) {
	cfg := &appconfig.Config{StorageEnabled: false}
	svc, _ := NewStorageService(cfg, slog.Default())

	if svc.Bucket() != "" {
		t.Error("Bucket() should return empty string when disabled")
	}
}
