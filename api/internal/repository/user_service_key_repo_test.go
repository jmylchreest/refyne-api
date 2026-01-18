package repository

import (
	"context"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// ========================================
// UserServiceKeyRepository Tests
// ========================================

func TestUserServiceKeyRepository_Upsert_Create(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	key := &models.UserServiceKey{
		UserID:          "user-1",
		Provider:        "openrouter",
		APIKeyEncrypted: "encrypted-api-key",
		BaseURL:         "https://openrouter.ai/api/v1",
		IsEnabled:       true,
	}

	err := repos.UserServiceKey.Upsert(ctx, key)
	if err != nil {
		t.Fatalf("failed to upsert user service key: %v", err)
	}

	if key.ID == "" {
		t.Error("expected ID to be generated")
	}

	// Verify by fetching
	fetched, err := repos.UserServiceKey.GetByID(ctx, key.ID)
	if err != nil {
		t.Fatalf("failed to fetch user service key: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected user service key, got nil")
	}
	if fetched.UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", fetched.UserID, "user-1")
	}
	if fetched.Provider != "openrouter" {
		t.Errorf("Provider = %q, want %q", fetched.Provider, "openrouter")
	}
	if fetched.APIKeyEncrypted != "encrypted-api-key" {
		t.Errorf("APIKeyEncrypted = %q, want %q", fetched.APIKeyEncrypted, "encrypted-api-key")
	}
	if fetched.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("BaseURL = %q, want %q", fetched.BaseURL, "https://openrouter.ai/api/v1")
	}
	if !fetched.IsEnabled {
		t.Error("expected IsEnabled to be true")
	}
}

func TestUserServiceKeyRepository_Upsert_Update(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create initial key
	key := &models.UserServiceKey{
		UserID:          "user-update",
		Provider:        "anthropic",
		APIKeyEncrypted: "old-key",
		IsEnabled:       true,
	}
	repos.UserServiceKey.Upsert(ctx, key)

	// Update with same user+provider
	updatedKey := &models.UserServiceKey{
		UserID:          "user-update",
		Provider:        "anthropic",
		APIKeyEncrypted: "new-key",
		BaseURL:         "https://api.anthropic.com",
		IsEnabled:       false,
	}
	err := repos.UserServiceKey.Upsert(ctx, updatedKey)
	if err != nil {
		t.Fatalf("failed to upsert updated key: %v", err)
	}

	// Verify update
	fetched, _ := repos.UserServiceKey.GetByUserAndProvider(ctx, "user-update", "anthropic")
	if fetched.APIKeyEncrypted != "new-key" {
		t.Errorf("APIKeyEncrypted = %q, want %q", fetched.APIKeyEncrypted, "new-key")
	}
	if fetched.BaseURL != "https://api.anthropic.com" {
		t.Errorf("BaseURL = %q, want %q", fetched.BaseURL, "https://api.anthropic.com")
	}
	if fetched.IsEnabled {
		t.Error("expected IsEnabled to be false after update")
	}
}

func TestUserServiceKeyRepository_GetByID_NotFound(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	key, err := repos.UserServiceKey.GetByID(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != nil {
		t.Error("expected nil for nonexistent ID")
	}
}

func TestUserServiceKeyRepository_GetByUserID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create multiple keys for same user
	providers := []string{"anthropic", "openai", "openrouter"}
	for _, p := range providers {
		repos.UserServiceKey.Upsert(ctx, &models.UserServiceKey{
			UserID:          "user-multi",
			Provider:        p,
			APIKeyEncrypted: "key-" + p,
			IsEnabled:       true,
		})
	}

	// Create key for different user
	repos.UserServiceKey.Upsert(ctx, &models.UserServiceKey{
		UserID:          "other-user",
		Provider:        "openai",
		APIKeyEncrypted: "other-key",
		IsEnabled:       true,
	})

	keys, err := repos.UserServiceKey.GetByUserID(ctx, "user-multi")
	if err != nil {
		t.Fatalf("failed to get keys: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}

	// Should be ordered by provider
	if keys[0].Provider != "anthropic" {
		t.Errorf("expected first provider to be 'anthropic', got %q", keys[0].Provider)
	}
}

func TestUserServiceKeyRepository_GetByUserAndProvider(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	repos.UserServiceKey.Upsert(ctx, &models.UserServiceKey{
		UserID:          "user-specific",
		Provider:        "openrouter",
		APIKeyEncrypted: "specific-key",
		IsEnabled:       true,
	})

	key, err := repos.UserServiceKey.GetByUserAndProvider(ctx, "user-specific", "openrouter")
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if key == nil {
		t.Fatal("expected key, got nil")
	}
	if key.APIKeyEncrypted != "specific-key" {
		t.Errorf("APIKeyEncrypted = %q, want %q", key.APIKeyEncrypted, "specific-key")
	}

	// Non-existent combination
	key, err = repos.UserServiceKey.GetByUserAndProvider(ctx, "user-specific", "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != nil {
		t.Error("expected nil for nonexistent provider")
	}
}

func TestUserServiceKeyRepository_GetEnabledByUserID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create mix of enabled and disabled keys
	repos.UserServiceKey.Upsert(ctx, &models.UserServiceKey{
		UserID:          "user-enabled",
		Provider:        "openrouter",
		APIKeyEncrypted: "key-1",
		IsEnabled:       true,
	})
	repos.UserServiceKey.Upsert(ctx, &models.UserServiceKey{
		UserID:          "user-enabled",
		Provider:        "anthropic",
		APIKeyEncrypted: "key-2",
		IsEnabled:       false,
	})
	repos.UserServiceKey.Upsert(ctx, &models.UserServiceKey{
		UserID:          "user-enabled",
		Provider:        "openai",
		APIKeyEncrypted: "key-3",
		IsEnabled:       true,
	})

	keys, err := repos.UserServiceKey.GetEnabledByUserID(ctx, "user-enabled")
	if err != nil {
		t.Fatalf("failed to get enabled keys: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 enabled keys, got %d", len(keys))
	}

	for _, k := range keys {
		if !k.IsEnabled {
			t.Errorf("got disabled key %q in enabled list", k.Provider)
		}
	}
}

func TestUserServiceKeyRepository_Delete(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create a key
	key := &models.UserServiceKey{
		UserID:          "user-delete",
		Provider:        "openrouter",
		APIKeyEncrypted: "to-delete",
		IsEnabled:       true,
	}
	repos.UserServiceKey.Upsert(ctx, key)

	// Delete it
	err := repos.UserServiceKey.Delete(ctx, key.ID)
	if err != nil {
		t.Fatalf("failed to delete key: %v", err)
	}

	// Verify deleted
	fetched, _ := repos.UserServiceKey.GetByID(ctx, key.ID)
	if fetched != nil {
		t.Error("expected key to be deleted")
	}
}

func TestUserServiceKeyRepository_Delete_NonExistent(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Delete should not error even if not found
	err := repos.UserServiceKey.Delete(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error deleting nonexistent key: %v", err)
	}
}

func TestUserServiceKeyRepository_EmptyBaseURL(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create key without BaseURL
	key := &models.UserServiceKey{
		UserID:          "user-no-baseurl",
		Provider:        "openai",
		APIKeyEncrypted: "key-no-baseurl",
		IsEnabled:       true,
	}
	repos.UserServiceKey.Upsert(ctx, key)

	// Fetch and verify BaseURL is empty string
	fetched, _ := repos.UserServiceKey.GetByID(ctx, key.ID)
	if fetched.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty string", fetched.BaseURL)
	}
}
