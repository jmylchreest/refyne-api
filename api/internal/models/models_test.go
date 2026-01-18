package models

import (
	"encoding/json"
	"testing"
	"time"
)

// ========================================
// FlexInt Tests
// ========================================

func TestFlexInt_UnmarshalJSON_Number(t *testing.T) {
	data := []byte(`42`)
	var f FlexInt
	err := json.Unmarshal(data, &f)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != 42 {
		t.Errorf("FlexInt = %d, want 42", f)
	}
}

func TestFlexInt_UnmarshalJSON_String(t *testing.T) {
	data := []byte(`"123"`)
	var f FlexInt
	err := json.Unmarshal(data, &f)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != 123 {
		t.Errorf("FlexInt = %d, want 123", f)
	}
}

func TestFlexInt_UnmarshalJSON_EmptyString(t *testing.T) {
	data := []byte(`""`)
	var f FlexInt
	err := json.Unmarshal(data, &f)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != 0 {
		t.Errorf("FlexInt = %d, want 0 for empty string", f)
	}
}

func TestFlexInt_UnmarshalJSON_InvalidString(t *testing.T) {
	data := []byte(`"not-a-number"`)
	var f FlexInt
	err := json.Unmarshal(data, &f)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != 0 {
		t.Errorf("FlexInt = %d, want 0 for invalid string", f)
	}
}

func TestFlexInt_UnmarshalJSON_Null(t *testing.T) {
	data := []byte(`null`)
	var f FlexInt
	err := json.Unmarshal(data, &f)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != 0 {
		t.Errorf("FlexInt = %d, want 0 for null", f)
	}
}

func TestFlexInt_UnmarshalJSON_Negative(t *testing.T) {
	data := []byte(`-5`)
	var f FlexInt
	err := json.Unmarshal(data, &f)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != -5 {
		t.Errorf("FlexInt = %d, want -5", f)
	}
}

func TestFlexInt_UnmarshalJSON_NegativeString(t *testing.T) {
	data := []byte(`"-10"`)
	var f FlexInt
	err := json.Unmarshal(data, &f)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != -10 {
		t.Errorf("FlexInt = %d, want -10", f)
	}
}

func TestFlexInt_MarshalJSON(t *testing.T) {
	f := FlexInt(99)
	data, err := json.Marshal(f)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "99" {
		t.Errorf("marshaled = %s, want 99", string(data))
	}
}

func TestFlexInt_Int(t *testing.T) {
	f := FlexInt(42)
	if f.Int() != 42 {
		t.Errorf("Int() = %d, want 42", f.Int())
	}
}

func TestFlexInt_InStruct(t *testing.T) {
	type TestStruct struct {
		Count FlexInt `json:"count"`
	}

	tests := []struct {
		name     string
		json     string
		expected int
	}{
		{"number", `{"count": 5}`, 5},
		{"string", `{"count": "10"}`, 10},
		{"empty string", `{"count": ""}`, 0},
		{"missing", `{}`, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s TestStruct
			err := json.Unmarshal([]byte(tt.json), &s)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.Count.Int() != tt.expected {
				t.Errorf("Count = %d, want %d", s.Count.Int(), tt.expected)
			}
		})
	}
}

// ========================================
// JobStatus Constants Tests
// ========================================

func TestJobStatus_Constants(t *testing.T) {
	if JobStatusPending != "pending" {
		t.Errorf("JobStatusPending = %q, want %q", JobStatusPending, "pending")
	}
	if JobStatusRunning != "running" {
		t.Errorf("JobStatusRunning = %q, want %q", JobStatusRunning, "running")
	}
	if JobStatusCompleted != "completed" {
		t.Errorf("JobStatusCompleted = %q, want %q", JobStatusCompleted, "completed")
	}
	if JobStatusFailed != "failed" {
		t.Errorf("JobStatusFailed = %q, want %q", JobStatusFailed, "failed")
	}
	if JobStatusCancelled != "cancelled" {
		t.Errorf("JobStatusCancelled = %q, want %q", JobStatusCancelled, "cancelled")
	}
}

// ========================================
// JobType Constants Tests
// ========================================

func TestJobType_Constants(t *testing.T) {
	if JobTypeExtract != "extract" {
		t.Errorf("JobTypeExtract = %q, want %q", JobTypeExtract, "extract")
	}
	if JobTypeCrawl != "crawl" {
		t.Errorf("JobTypeCrawl = %q, want %q", JobTypeCrawl, "crawl")
	}
	if JobTypeAnalyze != "analyze" {
		t.Errorf("JobTypeAnalyze = %q, want %q", JobTypeAnalyze, "analyze")
	}
}

// ========================================
// CrawlStatus Constants Tests
// ========================================

func TestCrawlStatus_Constants(t *testing.T) {
	if CrawlStatusPending != "pending" {
		t.Errorf("CrawlStatusPending = %q, want %q", CrawlStatusPending, "pending")
	}
	if CrawlStatusCrawling != "crawling" {
		t.Errorf("CrawlStatusCrawling = %q, want %q", CrawlStatusCrawling, "crawling")
	}
	if CrawlStatusCompleted != "completed" {
		t.Errorf("CrawlStatusCompleted = %q, want %q", CrawlStatusCompleted, "completed")
	}
	if CrawlStatusFailed != "failed" {
		t.Errorf("CrawlStatusFailed = %q, want %q", CrawlStatusFailed, "failed")
	}
	if CrawlStatusSkipped != "skipped" {
		t.Errorf("CrawlStatusSkipped = %q, want %q", CrawlStatusSkipped, "skipped")
	}
}

// ========================================
// WebhookEventType Constants Tests
// ========================================

func TestWebhookEventType_Constants(t *testing.T) {
	if WebhookEventAll != "*" {
		t.Errorf("WebhookEventAll = %q, want %q", WebhookEventAll, "*")
	}
	if WebhookEventJobStarted != "job.started" {
		t.Errorf("WebhookEventJobStarted = %q, want %q", WebhookEventJobStarted, "job.started")
	}
	if WebhookEventJobCompleted != "job.completed" {
		t.Errorf("WebhookEventJobCompleted = %q, want %q", WebhookEventJobCompleted, "job.completed")
	}
	if WebhookEventJobFailed != "job.failed" {
		t.Errorf("WebhookEventJobFailed = %q, want %q", WebhookEventJobFailed, "job.failed")
	}
	if WebhookEventJobProgress != "job.progress" {
		t.Errorf("WebhookEventJobProgress = %q, want %q", WebhookEventJobProgress, "job.progress")
	}
	if WebhookEventExtractSuccess != "extract.success" {
		t.Errorf("WebhookEventExtractSuccess = %q, want %q", WebhookEventExtractSuccess, "extract.success")
	}
	if WebhookEventExtractFailed != "extract.failed" {
		t.Errorf("WebhookEventExtractFailed = %q, want %q", WebhookEventExtractFailed, "extract.failed")
	}
}

// ========================================
// WebhookDeliveryStatus Constants Tests
// ========================================

func TestWebhookDeliveryStatus_Constants(t *testing.T) {
	if WebhookDeliveryStatusPending != "pending" {
		t.Errorf("WebhookDeliveryStatusPending = %q, want %q", WebhookDeliveryStatusPending, "pending")
	}
	if WebhookDeliveryStatusSuccess != "success" {
		t.Errorf("WebhookDeliveryStatusSuccess = %q, want %q", WebhookDeliveryStatusSuccess, "success")
	}
	if WebhookDeliveryStatusFailed != "failed" {
		t.Errorf("WebhookDeliveryStatusFailed = %q, want %q", WebhookDeliveryStatusFailed, "failed")
	}
	if WebhookDeliveryStatusRetrying != "retrying" {
		t.Errorf("WebhookDeliveryStatusRetrying = %q, want %q", WebhookDeliveryStatusRetrying, "retrying")
	}
}

// ========================================
// Model Struct JSON Tests
// ========================================

func TestJob_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	job := Job{
		ID:        "job-123",
		UserID:    "user-456",
		Type:      JobTypeExtract,
		Status:    JobStatusCompleted,
		URL:       "https://example.com",
		PageCount: 5,
		CostUSD:   0.05,
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded Job
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != job.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, job.ID)
	}
	if decoded.Type != job.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, job.Type)
	}
	if decoded.Status != job.Status {
		t.Errorf("Status = %q, want %q", decoded.Status, job.Status)
	}
}

func TestAPIKey_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	apiKey := APIKey{
		ID:        "key-123",
		UserID:    "user-456",
		Name:      "Test Key",
		KeyPrefix: "rf_test_",
		Scopes:    []string{"jobs:read", "jobs:write"},
		CreatedAt: now,
	}

	data, err := json.Marshal(apiKey)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded APIKey
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Name != apiKey.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, apiKey.Name)
	}
	if len(decoded.Scopes) != 2 {
		t.Errorf("Scopes length = %d, want 2", len(decoded.Scopes))
	}
}

func TestHeader_JSONSerialization(t *testing.T) {
	header := Header{
		Name:  "Authorization",
		Value: "Bearer token123",
	}

	data, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded Header
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Name != header.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, header.Name)
	}
	if decoded.Value != header.Value {
		t.Errorf("Value = %q, want %q", decoded.Value, header.Value)
	}
}

func TestWebhook_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	webhook := Webhook{
		ID:       "wh-123",
		UserID:   "user-456",
		Name:     "Production Webhook",
		URL:      "https://api.example.com/webhook",
		Events:   []string{"job.completed", "job.failed"},
		Headers:  []Header{{Name: "X-Custom", Value: "value"}},
		IsActive: true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(webhook)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded Webhook
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Name != webhook.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, webhook.Name)
	}
	if len(decoded.Events) != 2 {
		t.Errorf("Events length = %d, want 2", len(decoded.Events))
	}
	if len(decoded.Headers) != 1 {
		t.Errorf("Headers length = %d, want 1", len(decoded.Headers))
	}
}

// ========================================
// Model ZeroValue Tests
// ========================================

func TestJob_ZeroValue(t *testing.T) {
	var job Job

	if job.ID != "" {
		t.Error("ID should be empty by default")
	}
	if job.Status != "" {
		t.Error("Status should be empty by default")
	}
	if job.PageCount != 0 {
		t.Error("PageCount should be 0 by default")
	}
	if job.IsBYOK {
		t.Error("IsBYOK should be false by default")
	}
}

func TestUsageRecord_ZeroValue(t *testing.T) {
	var record UsageRecord

	if record.TotalChargedUSD != 0 {
		t.Error("TotalChargedUSD should be 0 by default")
	}
	if record.IsBYOK {
		t.Error("IsBYOK should be false by default")
	}
}
