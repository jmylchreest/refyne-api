package repository

import (
	"context"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// ========================================
// ServiceKeyRepository Tests
// ========================================

func TestServiceKeyRepository_Upsert_Create(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	key := &models.ServiceKey{
		Provider:        "openrouter",
		APIKeyEncrypted: "encrypted-api-key",
		DefaultModel:    "anthropic/claude-3-haiku",
		IsEnabled:       true,
	}

	err := repos.ServiceKey.Upsert(ctx, key)
	if err != nil {
		t.Fatalf("failed to upsert service key: %v", err)
	}

	if key.ID == "" {
		t.Error("expected ID to be generated")
	}

	// Verify by fetching
	fetched, err := repos.ServiceKey.GetByProvider(ctx, "openrouter")
	if err != nil {
		t.Fatalf("failed to fetch service key: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected service key, got nil")
	}
	if fetched.Provider != "openrouter" {
		t.Errorf("Provider = %q, want %q", fetched.Provider, "openrouter")
	}
	if fetched.APIKeyEncrypted != "encrypted-api-key" {
		t.Errorf("APIKeyEncrypted = %q, want %q", fetched.APIKeyEncrypted, "encrypted-api-key")
	}
	if fetched.DefaultModel != "anthropic/claude-3-haiku" {
		t.Errorf("DefaultModel = %q, want %q", fetched.DefaultModel, "anthropic/claude-3-haiku")
	}
	if !fetched.IsEnabled {
		t.Error("expected IsEnabled to be true")
	}
}

func TestServiceKeyRepository_Upsert_Update(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create initial key
	key := &models.ServiceKey{
		Provider:        "anthropic",
		APIKeyEncrypted: "old-key",
		DefaultModel:    "claude-3-opus",
		IsEnabled:       true,
	}
	repos.ServiceKey.Upsert(ctx, key)

	// Update with same provider
	updatedKey := &models.ServiceKey{
		Provider:        "anthropic",
		APIKeyEncrypted: "new-key",
		DefaultModel:    "claude-3-sonnet",
		IsEnabled:       false,
	}
	err := repos.ServiceKey.Upsert(ctx, updatedKey)
	if err != nil {
		t.Fatalf("failed to upsert updated service key: %v", err)
	}

	// Verify update
	fetched, err := repos.ServiceKey.GetByProvider(ctx, "anthropic")
	if err != nil {
		t.Fatalf("failed to fetch service key: %v", err)
	}
	if fetched.APIKeyEncrypted != "new-key" {
		t.Errorf("APIKeyEncrypted = %q, want %q", fetched.APIKeyEncrypted, "new-key")
	}
	if fetched.DefaultModel != "claude-3-sonnet" {
		t.Errorf("DefaultModel = %q, want %q", fetched.DefaultModel, "claude-3-sonnet")
	}
	if fetched.IsEnabled {
		t.Error("expected IsEnabled to be false after update")
	}
}

func TestServiceKeyRepository_GetByProvider_NotFound(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	key, err := repos.ServiceKey.GetByProvider(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != nil {
		t.Error("expected nil for nonexistent provider")
	}
}

func TestServiceKeyRepository_GetAll(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create multiple keys
	providers := []string{"anthropic", "openai", "openrouter"}
	for _, p := range providers {
		repos.ServiceKey.Upsert(ctx, &models.ServiceKey{
			Provider:        p,
			APIKeyEncrypted: "key-" + p,
			IsEnabled:       true,
		})
	}

	keys, err := repos.ServiceKey.GetAll(ctx)
	if err != nil {
		t.Fatalf("failed to get all keys: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}

	// Should be ordered by provider
	if keys[0].Provider != "anthropic" {
		t.Errorf("expected first provider to be 'anthropic', got %q", keys[0].Provider)
	}
}

func TestServiceKeyRepository_GetEnabled(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create mix of enabled and disabled keys
	repos.ServiceKey.Upsert(ctx, &models.ServiceKey{
		Provider:        "openrouter",
		APIKeyEncrypted: "key-1",
		IsEnabled:       true,
	})
	repos.ServiceKey.Upsert(ctx, &models.ServiceKey{
		Provider:        "anthropic",
		APIKeyEncrypted: "key-2",
		IsEnabled:       false,
	})
	repos.ServiceKey.Upsert(ctx, &models.ServiceKey{
		Provider:        "openai",
		APIKeyEncrypted: "key-3",
		IsEnabled:       true,
	})

	keys, err := repos.ServiceKey.GetEnabled(ctx)
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

func TestServiceKeyRepository_Delete(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create a key
	repos.ServiceKey.Upsert(ctx, &models.ServiceKey{
		Provider:        "openrouter",
		APIKeyEncrypted: "to-delete",
		IsEnabled:       true,
	})

	// Delete it
	err := repos.ServiceKey.Delete(ctx, "openrouter")
	if err != nil {
		t.Fatalf("failed to delete service key: %v", err)
	}

	// Verify deleted
	fetched, err := repos.ServiceKey.GetByProvider(ctx, "openrouter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fetched != nil {
		t.Error("expected service key to be deleted")
	}
}

func TestServiceKeyRepository_Delete_NonExistent(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Delete should not error even if not found
	err := repos.ServiceKey.Delete(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error deleting nonexistent key: %v", err)
	}
}
