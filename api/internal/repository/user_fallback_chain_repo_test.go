package repository

import (
	"context"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// ========================================
// UserFallbackChainRepository Tests
// ========================================

func TestUserFallbackChainRepository_ReplaceAll(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	temp := 0.7
	maxTokens := 4096
	strict := true
	entries := []*models.UserFallbackChainEntry{
		{
			Provider:    "openrouter",
			Model:       "anthropic/claude-3-haiku",
			Temperature: &temp,
			MaxTokens:   &maxTokens,
			StrictMode:  &strict,
			IsEnabled:   true,
		},
		{
			Provider:  "anthropic",
			Model:     "claude-3-sonnet-20240229",
			IsEnabled: true,
		},
		{
			Provider:  "openai",
			Model:     "gpt-4o-mini",
			IsEnabled: false,
		},
	}

	err := repos.UserFallbackChain.ReplaceAll(ctx, "user-1", entries)
	if err != nil {
		t.Fatalf("failed to replace all entries: %v", err)
	}

	// Verify
	result, err := repos.UserFallbackChain.GetByUserID(ctx, "user-1")
	if err != nil {
		t.Fatalf("failed to get entries: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("expected 3 entries, got %d", len(result))
	}

	// Check positions are sequential
	for i, entry := range result {
		if entry.Position != i+1 {
			t.Errorf("result[%d].Position = %d, want %d", i, entry.Position, i+1)
		}
		if entry.UserID != "user-1" {
			t.Errorf("result[%d].UserID = %q, want %q", i, entry.UserID, "user-1")
		}
	}

	// Check optional fields
	if result[0].Temperature == nil || *result[0].Temperature != 0.7 {
		t.Error("expected Temperature to be 0.7")
	}
	if result[0].MaxTokens == nil || *result[0].MaxTokens != 4096 {
		t.Error("expected MaxTokens to be 4096")
	}
	if result[0].StrictMode == nil || *result[0].StrictMode != true {
		t.Error("expected StrictMode to be true")
	}
}

func TestUserFallbackChainRepository_ReplaceAll_ReplacesExisting(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create initial entries
	initialEntries := []*models.UserFallbackChainEntry{
		{Provider: "old-provider", Model: "old-model", IsEnabled: true},
	}
	repos.UserFallbackChain.ReplaceAll(ctx, "user-replace", initialEntries)

	// Replace with new entries
	newEntries := []*models.UserFallbackChainEntry{
		{Provider: "new-1", Model: "model-1", IsEnabled: true},
		{Provider: "new-2", Model: "model-2", IsEnabled: true},
	}
	err := repos.UserFallbackChain.ReplaceAll(ctx, "user-replace", newEntries)
	if err != nil {
		t.Fatalf("failed to replace: %v", err)
	}

	result, _ := repos.UserFallbackChain.GetByUserID(ctx, "user-replace")
	if len(result) != 2 {
		t.Errorf("expected 2 entries after replace, got %d", len(result))
	}

	// Old entries should be gone
	for _, entry := range result {
		if entry.Provider == "old-provider" {
			t.Error("old entry should have been deleted")
		}
	}
}

func TestUserFallbackChainRepository_ReplaceAll_Empty(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create initial entries
	initialEntries := []*models.UserFallbackChainEntry{
		{Provider: "provider", Model: "model", IsEnabled: true},
	}
	repos.UserFallbackChain.ReplaceAll(ctx, "user-clear", initialEntries)

	// Replace with empty list
	err := repos.UserFallbackChain.ReplaceAll(ctx, "user-clear", []*models.UserFallbackChainEntry{})
	if err != nil {
		t.Fatalf("failed to replace with empty: %v", err)
	}

	result, _ := repos.UserFallbackChain.GetByUserID(ctx, "user-clear")
	if len(result) != 0 {
		t.Errorf("expected 0 entries after clearing, got %d", len(result))
	}
}

func TestUserFallbackChainRepository_GetByUserID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create entries for user-1
	repos.UserFallbackChain.ReplaceAll(ctx, "user-1", []*models.UserFallbackChainEntry{
		{Provider: "provider-a", Model: "model-a", IsEnabled: true},
		{Provider: "provider-b", Model: "model-b", IsEnabled: true},
	})

	// Create entries for user-2
	repos.UserFallbackChain.ReplaceAll(ctx, "user-2", []*models.UserFallbackChainEntry{
		{Provider: "provider-c", Model: "model-c", IsEnabled: true},
	})

	// Get user-1 entries
	result, err := repos.UserFallbackChain.GetByUserID(ctx, "user-1")
	if err != nil {
		t.Fatalf("failed to get entries: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 entries for user-1, got %d", len(result))
	}

	// Verify ordered by position
	if result[0].Model != "model-a" {
		t.Errorf("first entry should be model-a, got %s", result[0].Model)
	}
}

func TestUserFallbackChainRepository_GetByUserID_NotFound(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	result, err := repos.UserFallbackChain.GetByUserID(ctx, "nonexistent-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 entries for nonexistent user, got %d", len(result))
	}
}

func TestUserFallbackChainRepository_GetEnabledByUserID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create mix of enabled and disabled entries
	repos.UserFallbackChain.ReplaceAll(ctx, "user-enabled", []*models.UserFallbackChainEntry{
		{Provider: "enabled-1", Model: "model-1", IsEnabled: true},
		{Provider: "disabled", Model: "model-2", IsEnabled: false},
		{Provider: "enabled-2", Model: "model-3", IsEnabled: true},
	})

	result, err := repos.UserFallbackChain.GetEnabledByUserID(ctx, "user-enabled")
	if err != nil {
		t.Fatalf("failed to get enabled entries: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 enabled entries, got %d", len(result))
	}

	for _, entry := range result {
		if !entry.IsEnabled {
			t.Errorf("got disabled entry %q in enabled list", entry.Provider)
		}
	}
}

func TestUserFallbackChainRepository_GetEnabledByUserID_NoEnabled(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create only disabled entries
	repos.UserFallbackChain.ReplaceAll(ctx, "user-all-disabled", []*models.UserFallbackChainEntry{
		{Provider: "disabled-1", Model: "model-1", IsEnabled: false},
		{Provider: "disabled-2", Model: "model-2", IsEnabled: false},
	})

	result, err := repos.UserFallbackChain.GetEnabledByUserID(ctx, "user-all-disabled")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 enabled entries, got %d", len(result))
	}
}

func TestUserFallbackChainRepository_IsolationBetweenUsers(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create entries for user-1
	repos.UserFallbackChain.ReplaceAll(ctx, "user-1", []*models.UserFallbackChainEntry{
		{Provider: "user1-provider", Model: "user1-model", IsEnabled: true},
	})

	// Create entries for user-2
	repos.UserFallbackChain.ReplaceAll(ctx, "user-2", []*models.UserFallbackChainEntry{
		{Provider: "user2-provider", Model: "user2-model", IsEnabled: true},
	})

	// Replacing user-1 should not affect user-2
	repos.UserFallbackChain.ReplaceAll(ctx, "user-1", []*models.UserFallbackChainEntry{
		{Provider: "new-provider", Model: "new-model", IsEnabled: true},
	})

	user2Entries, _ := repos.UserFallbackChain.GetByUserID(ctx, "user-2")
	if len(user2Entries) != 1 {
		t.Errorf("user-2 entries should be unchanged, got %d entries", len(user2Entries))
	}
	if user2Entries[0].Provider != "user2-provider" {
		t.Errorf("user-2 entry should be unchanged, got provider %q", user2Entries[0].Provider)
	}
}

func TestUserFallbackChainRepository_OptionalFieldsNil(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create entry with all optional fields nil
	repos.UserFallbackChain.ReplaceAll(ctx, "user-nil-fields", []*models.UserFallbackChainEntry{
		{
			Provider:    "provider",
			Model:       "model",
			Temperature: nil,
			MaxTokens:   nil,
			StrictMode:  nil,
			IsEnabled:   true,
		},
	})

	result, _ := repos.UserFallbackChain.GetByUserID(ctx, "user-nil-fields")
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}

	if result[0].Temperature != nil {
		t.Errorf("expected Temperature to be nil, got %v", *result[0].Temperature)
	}
	if result[0].MaxTokens != nil {
		t.Errorf("expected MaxTokens to be nil, got %v", *result[0].MaxTokens)
	}
	if result[0].StrictMode != nil {
		t.Errorf("expected StrictMode to be nil, got %v", *result[0].StrictMode)
	}
}
