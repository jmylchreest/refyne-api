package service

import (
	"context"
	"log/slog"
	"testing"
)

// ========================================
// TierSyncService Tests
// ========================================

// Note: TierSyncService depends on ClerkBackendClient which makes external HTTP calls.
// These tests cover the cases where Clerk is not configured.
// Full integration testing with Clerk would require HTTP mocking or live API access.

func TestNewTierSyncService(t *testing.T) {
	logger := slog.Default()

	// Test with nil client
	svc := NewTierSyncService(nil, logger)
	if svc == nil {
		t.Fatal("expected service, got nil")
	}
	if svc.clerkClient != nil {
		t.Error("expected clerkClient to be nil")
	}
	if svc.logger == nil {
		t.Error("expected logger to be set")
	}
}

func TestTierSyncService_SyncFromClerk_NilClient(t *testing.T) {
	logger := slog.Default()
	svc := NewTierSyncService(nil, logger)
	ctx := context.Background()

	// Should return nil without error when Clerk client is not configured
	err := svc.SyncFromClerk(ctx)
	if err != nil {
		t.Fatalf("expected no error with nil Clerk client, got: %v", err)
	}
}

func TestTierSyncService_SyncFromClerk_NilClientMultipleCalls(t *testing.T) {
	logger := slog.Default()
	svc := NewTierSyncService(nil, logger)
	ctx := context.Background()

	// Should handle multiple calls gracefully
	for i := 0; i < 3; i++ {
		err := svc.SyncFromClerk(ctx)
		if err != nil {
			t.Fatalf("call %d: expected no error, got: %v", i+1, err)
		}
	}
}

// Note: Testing with a real or mocked ClerkBackendClient would require:
// 1. Setting up an HTTP mock server (e.g., httptest.Server)
// 2. Creating a ClerkBackendClient that uses the mock server's URL
// 3. Configuring the mock to return SubscriptionProduct data
//
// Example test setup (not implemented here due to HTTP transport constraints):
//
// func TestTierSyncService_SyncFromClerk_WithMockedClerk(t *testing.T) {
//     // Setup mock HTTP server
//     server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//         if r.URL.Path == "/v1/billing/plans" {
//             json.NewEncoder(w).Encode(auth.ListPlansResponse{
//                 Data: []auth.SubscriptionProduct{
//                     {ID: "plan_1", Name: "Free", Slug: "free", PubliclyVisible: true, IsDefault: true},
//                     {ID: "plan_2", Name: "Pro", Slug: "pro", PubliclyVisible: true, IsDefault: false},
//                 },
//                 TotalCount: 2,
//             })
//             return
//         }
//         http.NotFound(w, r)
//     }))
//     defer server.Close()
//
//     // Create ClerkBackendClient pointing to mock server
//     // (would require modifying ClerkBackendClient to accept custom base URL)
//
//     // Test sync
//     svc := NewTierSyncService(clerkClient, logger)
//     err := svc.SyncFromClerk(ctx)
//     // Assert no error and verify tier metadata was updated
// }
