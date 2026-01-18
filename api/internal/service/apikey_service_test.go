package service

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/repository"
)

// ========================================
// CreateKey Tests
// ========================================

func TestAPIKeyService_CreateKey(t *testing.T) {
	mockRepo := newMockAPIKeyRepository()
	repos := &repository.Repositories{
		APIKey: mockRepo,
	}

	logger := slog.Default()
	svc := NewAPIKeyService(repos, logger)

	t.Run("creates key with default scopes", func(t *testing.T) {
		input := CreateKeyInput{
			Name: "Test Key",
		}

		output, err := svc.CreateKey(context.Background(), "user-123", input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if output == nil {
			t.Fatal("expected output, got nil")
		}

		// Check output fields
		if output.ID == "" {
			t.Error("expected ID to be set")
		}
		if output.Name != "Test Key" {
			t.Errorf("Name = %q, want %q", output.Name, "Test Key")
		}
		if !strings.HasPrefix(output.Key, "rf_") {
			t.Errorf("Key = %q, want prefix rf_", output.Key)
		}
		if !strings.HasPrefix(output.KeyPrefix, "rf_") || !strings.HasSuffix(output.KeyPrefix, "...") {
			t.Errorf("KeyPrefix = %q, want format rf_XXX...", output.KeyPrefix)
		}
		// Default scopes
		expectedScopes := []string{"extract", "crawl", "jobs"}
		if len(output.Scopes) != len(expectedScopes) {
			t.Errorf("Scopes length = %d, want %d", len(output.Scopes), len(expectedScopes))
		}
		if output.ExpiresAt != nil {
			t.Error("expected ExpiresAt to be nil for key without expiry")
		}
		if output.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set")
		}
	})

	t.Run("creates key with custom scopes", func(t *testing.T) {
		input := CreateKeyInput{
			Name:   "Custom Scopes Key",
			Scopes: []string{"jobs:read", "jobs:write"},
		}

		output, err := svc.CreateKey(context.Background(), "user-456", input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if len(output.Scopes) != 2 {
			t.Errorf("Scopes length = %d, want 2", len(output.Scopes))
		}
		if output.Scopes[0] != "jobs:read" || output.Scopes[1] != "jobs:write" {
			t.Errorf("Scopes = %v, want [jobs:read jobs:write]", output.Scopes)
		}
	})

	t.Run("creates key with expiration", func(t *testing.T) {
		expiresAt := time.Now().Add(24 * time.Hour)
		input := CreateKeyInput{
			Name:      "Expiring Key",
			ExpiresAt: &expiresAt,
		}

		output, err := svc.CreateKey(context.Background(), "user-789", input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if output.ExpiresAt == nil {
			t.Fatal("expected ExpiresAt to be set")
		}
		// Allow small time difference
		diff := output.ExpiresAt.Sub(expiresAt)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("ExpiresAt = %v, want %v", output.ExpiresAt, expiresAt)
		}
	})

	t.Run("generates unique keys", func(t *testing.T) {
		input := CreateKeyInput{Name: "Unique Key"}

		output1, err := svc.CreateKey(context.Background(), "user-unique", input)
		if err != nil {
			t.Fatalf("expected no error for first key, got %v", err)
		}

		output2, err := svc.CreateKey(context.Background(), "user-unique", input)
		if err != nil {
			t.Fatalf("expected no error for second key, got %v", err)
		}

		if output1.Key == output2.Key {
			t.Error("expected unique keys, but got duplicates")
		}
		if output1.ID == output2.ID {
			t.Error("expected unique IDs, but got duplicates")
		}
	})

	t.Run("key can be validated after creation", func(t *testing.T) {
		input := CreateKeyInput{
			Name:   "Validatable Key",
			Scopes: []string{"*"},
		}

		output, err := svc.CreateKey(context.Background(), "user-validate", input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// The key should be findable by its hash
		keyHash := hashAPIKey(output.Key)
		foundKey, err := mockRepo.GetByKeyHash(context.Background(), keyHash)
		if err != nil {
			t.Fatalf("expected no error looking up key, got %v", err)
		}
		if foundKey == nil {
			t.Fatal("expected to find key by hash")
		}
		if foundKey.UserID != "user-validate" {
			t.Errorf("UserID = %q, want %q", foundKey.UserID, "user-validate")
		}
	})
}

// ========================================
// ListKeys Tests
// ========================================

func TestAPIKeyService_ListKeys(t *testing.T) {
	mockRepo := newMockAPIKeyRepository()
	repos := &repository.Repositories{
		APIKey: mockRepo,
	}

	logger := slog.Default()
	svc := NewAPIKeyService(repos, logger)

	t.Run("returns empty list for user with no keys", func(t *testing.T) {
		keys, err := svc.ListKeys(context.Background(), "user-no-keys")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(keys) != 0 {
			t.Errorf("expected 0 keys, got %d", len(keys))
		}
	})

	t.Run("returns keys for user", func(t *testing.T) {
		// Create some keys for a user
		userID := "user-with-keys"
		_, _ = svc.CreateKey(context.Background(), userID, CreateKeyInput{Name: "Key 1"})
		_, _ = svc.CreateKey(context.Background(), userID, CreateKeyInput{Name: "Key 2"})
		_, _ = svc.CreateKey(context.Background(), userID, CreateKeyInput{Name: "Key 3"})

		keys, err := svc.ListKeys(context.Background(), userID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(keys) != 3 {
			t.Errorf("expected 3 keys, got %d", len(keys))
		}

		// Verify keys belong to user
		for _, key := range keys {
			if key.UserID != userID {
				t.Errorf("key UserID = %q, want %q", key.UserID, userID)
			}
		}
	})

	t.Run("does not return keys for other users", func(t *testing.T) {
		userA := "user-a-list"
		userB := "user-b-list"

		_, _ = svc.CreateKey(context.Background(), userA, CreateKeyInput{Name: "User A Key"})
		_, _ = svc.CreateKey(context.Background(), userB, CreateKeyInput{Name: "User B Key"})

		keysA, _ := svc.ListKeys(context.Background(), userA)
		keysB, _ := svc.ListKeys(context.Background(), userB)

		// Each user should only see their own keys
		for _, key := range keysA {
			if key.UserID != userA {
				t.Errorf("User A got key from user %q", key.UserID)
			}
		}
		for _, key := range keysB {
			if key.UserID != userB {
				t.Errorf("User B got key from user %q", key.UserID)
			}
		}
	})
}

// ========================================
// RevokeKey Tests
// ========================================

func TestAPIKeyService_RevokeKey(t *testing.T) {
	mockRepo := newMockAPIKeyRepository()
	repos := &repository.Repositories{
		APIKey: mockRepo,
	}

	logger := slog.Default()
	svc := NewAPIKeyService(repos, logger)

	t.Run("revokes own key", func(t *testing.T) {
		userID := "user-revoke-own"
		output, _ := svc.CreateKey(context.Background(), userID, CreateKeyInput{Name: "To Revoke"})

		err := svc.RevokeKey(context.Background(), userID, output.ID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify key is revoked
		key, _ := mockRepo.GetByID(context.Background(), output.ID)
		if key.RevokedAt == nil {
			t.Error("expected key to be revoked")
		}
	})

	t.Run("fails to revoke other user key", func(t *testing.T) {
		userA := "user-a-revoke"
		userB := "user-b-revoke"

		output, _ := svc.CreateKey(context.Background(), userA, CreateKeyInput{Name: "User A Key"})

		// User B tries to revoke User A's key
		err := svc.RevokeKey(context.Background(), userB, output.ID)
		if err == nil {
			t.Fatal("expected error when revoking other user's key")
		}

		// Verify key is NOT revoked
		key, _ := mockRepo.GetByID(context.Background(), output.ID)
		if key.RevokedAt != nil {
			t.Error("expected key to NOT be revoked")
		}
	})

	t.Run("fails for non-existent key", func(t *testing.T) {
		err := svc.RevokeKey(context.Background(), "user-any", "non-existent-key-id")
		if err == nil {
			t.Fatal("expected error for non-existent key")
		}
	})

	t.Run("can revoke already revoked key", func(t *testing.T) {
		userID := "user-double-revoke"
		output, _ := svc.CreateKey(context.Background(), userID, CreateKeyInput{Name: "Double Revoke"})

		// Revoke once
		err := svc.RevokeKey(context.Background(), userID, output.ID)
		if err != nil {
			t.Fatalf("expected no error on first revoke, got %v", err)
		}

		// Revoke again - should still succeed (idempotent)
		err = svc.RevokeKey(context.Background(), userID, output.ID)
		if err != nil {
			t.Fatalf("expected no error on second revoke, got %v", err)
		}
	})
}

// ========================================
// CreateKeyInput/Output Tests
// ========================================

func TestCreateKeyInput_Fields(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	input := CreateKeyInput{
		Name:      "Test Name",
		Scopes:    []string{"read", "write"},
		ExpiresAt: &expiresAt,
	}

	if input.Name != "Test Name" {
		t.Errorf("Name = %q, want %q", input.Name, "Test Name")
	}
	if len(input.Scopes) != 2 {
		t.Errorf("Scopes length = %d, want 2", len(input.Scopes))
	}
	if input.ExpiresAt == nil {
		t.Error("expected ExpiresAt to be set")
	}
}

func TestCreateKeyOutput_Fields(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(time.Hour)
	output := CreateKeyOutput{
		ID:        "key-123",
		Name:      "Test Key",
		Key:       "rf_secret_key",
		KeyPrefix: "rf_secret_...",
		Scopes:    []string{"*"},
		ExpiresAt: &expiresAt,
		CreatedAt: now,
	}

	if output.ID != "key-123" {
		t.Errorf("ID = %q, want %q", output.ID, "key-123")
	}
	if output.Key != "rf_secret_key" {
		t.Errorf("Key = %q, want %q", output.Key, "rf_secret_key")
	}
	if output.KeyPrefix != "rf_secret_..." {
		t.Errorf("KeyPrefix = %q, want %q", output.KeyPrefix, "rf_secret_...")
	}
}
