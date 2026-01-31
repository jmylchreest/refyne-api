package service

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
	"github.com/oklog/ulid/v2"
)

// ========================================
// Mock Repositories for UserLLMService Tests
// ========================================

// userLLMServiceKeyRepository implements repository.UserServiceKeyRepository for testing.
type userLLMServiceKeyRepository struct {
	mu   sync.RWMutex
	keys map[string]*models.UserServiceKey // keyed by ID
}

func newUserLLMServiceKeyRepository() *userLLMServiceKeyRepository {
	return &userLLMServiceKeyRepository{
		keys: make(map[string]*models.UserServiceKey),
	}
}

func (m *userLLMServiceKeyRepository) Upsert(ctx context.Context, key *models.UserServiceKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for existing key by user+provider
	for id, existing := range m.keys {
		if existing.UserID == key.UserID && existing.Provider == key.Provider {
			// Update existing
			key.ID = existing.ID
			key.CreatedAt = existing.CreatedAt
			key.UpdatedAt = time.Now()
			m.keys[id] = key
			return nil
		}
	}

	// Create new
	key.ID = ulid.Make().String()
	key.CreatedAt = time.Now()
	key.UpdatedAt = time.Now()
	m.keys[key.ID] = key
	return nil
}

func (m *userLLMServiceKeyRepository) GetByID(ctx context.Context, id string) (*models.UserServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if key, ok := m.keys[id]; ok {
		return key, nil
	}
	return nil, nil
}

func (m *userLLMServiceKeyRepository) GetByUserID(ctx context.Context, userID string) ([]*models.UserServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.UserServiceKey
	for _, key := range m.keys {
		if key.UserID == userID {
			result = append(result, key)
		}
	}
	// Sort by provider for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].Provider < result[j].Provider
	})
	return result, nil
}

func (m *userLLMServiceKeyRepository) GetByUserAndProvider(ctx context.Context, userID, provider string) (*models.UserServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, key := range m.keys {
		if key.UserID == userID && key.Provider == provider {
			return key, nil
		}
	}
	return nil, nil
}

func (m *userLLMServiceKeyRepository) GetEnabledByUserID(ctx context.Context, userID string) ([]*models.UserServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.UserServiceKey
	for _, key := range m.keys {
		if key.UserID == userID && key.IsEnabled {
			result = append(result, key)
		}
	}
	return result, nil
}

func (m *userLLMServiceKeyRepository) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.keys, id)
	return nil
}

// userLLMFallbackChainRepository implements repository.UserFallbackChainRepository for testing.
type userLLMFallbackChainRepository struct {
	mu      sync.RWMutex
	entries map[string][]*models.UserFallbackChainEntry // keyed by userID
}

func newUserLLMFallbackChainRepository() *userLLMFallbackChainRepository {
	return &userLLMFallbackChainRepository{
		entries: make(map[string][]*models.UserFallbackChainEntry),
	}
}

func (m *userLLMFallbackChainRepository) GetByUserID(ctx context.Context, userID string) ([]*models.UserFallbackChainEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if entries, ok := m.entries[userID]; ok {
		// Return copy sorted by position
		result := make([]*models.UserFallbackChainEntry, len(entries))
		copy(result, entries)
		sort.Slice(result, func(i, j int) bool {
			return result[i].Position < result[j].Position
		})
		return result, nil
	}
	return []*models.UserFallbackChainEntry{}, nil
}

func (m *userLLMFallbackChainRepository) GetEnabledByUserID(ctx context.Context, userID string) ([]*models.UserFallbackChainEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if entries, ok := m.entries[userID]; ok {
		var result []*models.UserFallbackChainEntry
		for _, e := range entries {
			if e.IsEnabled {
				result = append(result, e)
			}
		}
		sort.Slice(result, func(i, j int) bool {
			return result[i].Position < result[j].Position
		})
		return result, nil
	}
	return []*models.UserFallbackChainEntry{}, nil
}

func (m *userLLMFallbackChainRepository) ReplaceAll(ctx context.Context, userID string, entries []*models.UserFallbackChainEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Assign IDs and timestamps
	now := time.Now()
	for i, e := range entries {
		e.ID = ulid.Make().String()
		e.UserID = userID
		e.Position = i + 1
		e.CreatedAt = now
		e.UpdatedAt = now
	}

	m.entries[userID] = entries
	return nil
}

// ========================================
// Test Setup Helpers
// ========================================

func setupUserLLMService(t *testing.T) (*UserLLMService, *userLLMServiceKeyRepository, *userLLMFallbackChainRepository, *crypto.Encryptor) {
	t.Helper()

	keyRepo := newUserLLMServiceKeyRepository()
	chainRepo := newUserLLMFallbackChainRepository()

	repos := &repository.Repositories{
		UserServiceKey:    keyRepo,
		UserFallbackChain: chainRepo,
	}

	// Create a test encryptor with a fixed key
	testKey := []byte("12345678901234567890123456789012") // 32 bytes for AES-256
	encryptor, err := crypto.NewEncryptor(testKey)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	logger := slog.Default()
	svc := NewUserLLMService(repos, encryptor, logger)

	return svc, keyRepo, chainRepo, encryptor
}

func setupUserLLMServiceNoEncryption(t *testing.T) (*UserLLMService, *userLLMServiceKeyRepository, *userLLMFallbackChainRepository) {
	t.Helper()

	keyRepo := newUserLLMServiceKeyRepository()
	chainRepo := newUserLLMFallbackChainRepository()

	repos := &repository.Repositories{
		UserServiceKey:    keyRepo,
		UserFallbackChain: chainRepo,
	}

	logger := slog.Default()
	svc := NewUserLLMService(repos, nil, logger)

	return svc, keyRepo, chainRepo
}

// ========================================
// ListServiceKeys Tests
// ========================================

func TestUserLLMService_ListServiceKeys(t *testing.T) {
	svc, keyRepo, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	// Create keys for user-1
	keyRepo.Upsert(ctx, &models.UserServiceKey{
		UserID:          "user-1",
		Provider:        "openrouter",
		APIKeyEncrypted: "encrypted-key-1",
		IsEnabled:       true,
	})
	keyRepo.Upsert(ctx, &models.UserServiceKey{
		UserID:          "user-1",
		Provider:        "anthropic",
		APIKeyEncrypted: "encrypted-key-2",
		IsEnabled:       true,
	})

	// Create key for different user
	keyRepo.Upsert(ctx, &models.UserServiceKey{
		UserID:          "user-2",
		Provider:        "openai",
		APIKeyEncrypted: "encrypted-key-3",
		IsEnabled:       true,
	})

	keys, err := svc.ListServiceKeys(ctx, "user-1")
	if err != nil {
		t.Fatalf("failed to list keys: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}

	// Verify all keys belong to user-1
	for _, k := range keys {
		if k.UserID != "user-1" {
			t.Errorf("got key for wrong user: %s", k.UserID)
		}
	}
}

func TestUserLLMService_ListServiceKeys_Empty(t *testing.T) {
	svc, _, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	keys, err := svc.ListServiceKeys(ctx, "nonexistent-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

// ========================================
// UpsertServiceKey Tests
// ========================================

func TestUserLLMService_UpsertServiceKey_Create(t *testing.T) {
	svc, _, _, encryptor := setupUserLLMService(t)
	ctx := context.Background()

	input := UserServiceKeyInput{
		Provider:  "openrouter",
		APIKey:    "test-api-key-123",
		BaseURL:   "https://openrouter.ai/api/v1",
		IsEnabled: true,
	}

	key, err := svc.UpsertServiceKey(ctx, "user-create", input)
	if err != nil {
		t.Fatalf("failed to upsert key: %v", err)
	}

	if key == nil {
		t.Fatal("expected key, got nil")
	}
	if key.ID == "" {
		t.Error("expected ID to be generated")
	}
	if key.UserID != "user-create" {
		t.Errorf("UserID = %q, want %q", key.UserID, "user-create")
	}
	if key.Provider != "openrouter" {
		t.Errorf("Provider = %q, want %q", key.Provider, "openrouter")
	}
	if key.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("BaseURL = %q, want %q", key.BaseURL, "https://openrouter.ai/api/v1")
	}
	if !key.IsEnabled {
		t.Error("expected IsEnabled to be true")
	}

	// Verify key was encrypted
	decrypted, err := encryptor.Decrypt(key.APIKeyEncrypted)
	if err != nil {
		t.Fatalf("failed to decrypt key: %v", err)
	}
	if decrypted != "test-api-key-123" {
		t.Errorf("decrypted key = %q, want %q", decrypted, "test-api-key-123")
	}
}

func TestUserLLMService_UpsertServiceKey_Update(t *testing.T) {
	svc, _, _, encryptor := setupUserLLMService(t)
	ctx := context.Background()

	// Create initial key
	_, err := svc.UpsertServiceKey(ctx, "user-update", UserServiceKeyInput{
		Provider:  "anthropic",
		APIKey:    "old-key",
		IsEnabled: true,
	})
	if err != nil {
		t.Fatalf("failed to create initial key: %v", err)
	}

	// Update with new key
	key, err := svc.UpsertServiceKey(ctx, "user-update", UserServiceKeyInput{
		Provider:  "anthropic",
		APIKey:    "new-key",
		BaseURL:   "https://custom.anthropic.com",
		IsEnabled: false,
	})
	if err != nil {
		t.Fatalf("failed to update key: %v", err)
	}

	// Verify update
	if key.BaseURL != "https://custom.anthropic.com" {
		t.Errorf("BaseURL = %q, want %q", key.BaseURL, "https://custom.anthropic.com")
	}
	if key.IsEnabled {
		t.Error("expected IsEnabled to be false")
	}

	decrypted, _ := encryptor.Decrypt(key.APIKeyEncrypted)
	if decrypted != "new-key" {
		t.Errorf("decrypted key = %q, want %q", decrypted, "new-key")
	}
}

func TestUserLLMService_UpsertServiceKey_UpdateKeepsExistingKey(t *testing.T) {
	svc, _, _, encryptor := setupUserLLMService(t)
	ctx := context.Background()

	// Create initial key
	initial, _ := svc.UpsertServiceKey(ctx, "user-keep", UserServiceKeyInput{
		Provider:  "openai",
		APIKey:    "original-key",
		IsEnabled: true,
	})

	// Update without providing new API key
	key, err := svc.UpsertServiceKey(ctx, "user-keep", UserServiceKeyInput{
		Provider:  "openai",
		APIKey:    "", // Empty - should keep existing
		BaseURL:   "https://custom.openai.com",
		IsEnabled: false,
	})
	if err != nil {
		t.Fatalf("failed to update key: %v", err)
	}

	// Verify existing key was kept
	decrypted, _ := encryptor.Decrypt(key.APIKeyEncrypted)
	if decrypted != "original-key" {
		t.Errorf("expected original key to be preserved, got %q", decrypted)
	}
	if key.APIKeyEncrypted != initial.APIKeyEncrypted {
		t.Error("expected encrypted key to be unchanged")
	}
	if key.BaseURL != "https://custom.openai.com" {
		t.Errorf("BaseURL = %q, want %q", key.BaseURL, "https://custom.openai.com")
	}
}

// TestUserLLMService_UpsertServiceKey_InvalidProvider removed - provider validation
// is now done at handler level against the LLM registry, not at service level

func TestUserLLMService_UpsertServiceKey_ValidProviders(t *testing.T) {
	svc, _, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	validProviders := []string{"openrouter", "anthropic", "openai", "ollama"}

	for _, provider := range validProviders {
		t.Run(provider, func(t *testing.T) {
			key, err := svc.UpsertServiceKey(ctx, "user-providers", UserServiceKeyInput{
				Provider:  provider,
				APIKey:    "test-key-" + provider,
				IsEnabled: true,
			})
			if err != nil {
				t.Fatalf("failed to upsert key for provider %s: %v", provider, err)
			}
			if key.Provider != provider {
				t.Errorf("Provider = %q, want %q", key.Provider, provider)
			}
		})
	}
}

func TestUserLLMService_UpsertServiceKey_NoEncryption(t *testing.T) {
	svc, _, _ := setupUserLLMServiceNoEncryption(t)
	ctx := context.Background()

	key, err := svc.UpsertServiceKey(ctx, "user-noenc", UserServiceKeyInput{
		Provider:  "openrouter",
		APIKey:    "plaintext-key",
		IsEnabled: true,
	})
	if err != nil {
		t.Fatalf("failed to upsert key: %v", err)
	}

	// Without encryption, key should be stored as-is
	if key.APIKeyEncrypted != "plaintext-key" {
		t.Errorf("APIKeyEncrypted = %q, want %q", key.APIKeyEncrypted, "plaintext-key")
	}
}

// ========================================
// DeleteServiceKey Tests
// ========================================

func TestUserLLMService_DeleteServiceKey(t *testing.T) {
	svc, keyRepo, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	// Create a key
	keyRepo.Upsert(ctx, &models.UserServiceKey{
		UserID:          "user-delete",
		Provider:        "openrouter",
		APIKeyEncrypted: "encrypted-key",
		IsEnabled:       true,
	})

	keys, _ := svc.ListServiceKeys(ctx, "user-delete")
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	keyID := keys[0].ID

	// Delete the key
	err := svc.DeleteServiceKey(ctx, "user-delete", keyID)
	if err != nil {
		t.Fatalf("failed to delete key: %v", err)
	}

	// Verify deleted
	keys, _ = svc.ListServiceKeys(ctx, "user-delete")
	if len(keys) != 0 {
		t.Errorf("expected 0 keys after delete, got %d", len(keys))
	}
}

func TestUserLLMService_DeleteServiceKey_NotFound(t *testing.T) {
	svc, _, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	err := svc.DeleteServiceKey(ctx, "user-1", "nonexistent-key-id")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}

func TestUserLLMService_DeleteServiceKey_WrongUser(t *testing.T) {
	svc, keyRepo, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	// Create a key for user-1
	keyRepo.Upsert(ctx, &models.UserServiceKey{
		UserID:          "user-1",
		Provider:        "openrouter",
		APIKeyEncrypted: "encrypted-key",
		IsEnabled:       true,
	})

	keys, _ := svc.ListServiceKeys(ctx, "user-1")
	keyID := keys[0].ID

	// Try to delete as user-2
	err := svc.DeleteServiceKey(ctx, "user-2", keyID)
	if err == nil {
		t.Fatal("expected error when deleting another user's key")
	}

	// Verify key still exists
	keys, _ = svc.ListServiceKeys(ctx, "user-1")
	if len(keys) != 1 {
		t.Error("key should not have been deleted")
	}
}

// ========================================
// GetFallbackChain Tests
// ========================================

func TestUserLLMService_GetFallbackChain(t *testing.T) {
	svc, _, chainRepo, _ := setupUserLLMService(t)
	ctx := context.Background()

	// Create entries for user
	chainRepo.ReplaceAll(ctx, "user-chain", []*models.UserFallbackChainEntry{
		{Provider: "openrouter", Model: "claude-3-haiku", IsEnabled: true},
		{Provider: "anthropic", Model: "claude-3-sonnet", IsEnabled: true},
	})

	entries, err := svc.GetFallbackChain(ctx, "user-chain")
	if err != nil {
		t.Fatalf("failed to get fallback chain: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Verify order
	if entries[0].Model != "claude-3-haiku" {
		t.Errorf("first entry model = %q, want %q", entries[0].Model, "claude-3-haiku")
	}
	if entries[1].Model != "claude-3-sonnet" {
		t.Errorf("second entry model = %q, want %q", entries[1].Model, "claude-3-sonnet")
	}
}

func TestUserLLMService_GetFallbackChain_Empty(t *testing.T) {
	svc, _, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	entries, err := svc.GetFallbackChain(ctx, "nonexistent-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// ========================================
// SetFallbackChain Tests
// ========================================

func TestUserLLMService_SetFallbackChain(t *testing.T) {
	svc, _, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	temp := 0.7
	maxTokens := 4096

	input := []UserFallbackChainEntryInput{
		{
			Provider:    "openrouter",
			Model:       "anthropic/claude-3-haiku",
			Temperature: &temp,
			MaxTokens:   &maxTokens,
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

	entries, err := svc.SetFallbackChain(ctx, "user-set", input)
	if err != nil {
		t.Fatalf("failed to set fallback chain: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Verify positions
	for i, e := range entries {
		if e.Position != i+1 {
			t.Errorf("entries[%d].Position = %d, want %d", i, e.Position, i+1)
		}
		if e.UserID != "user-set" {
			t.Errorf("entries[%d].UserID = %q, want %q", i, e.UserID, "user-set")
		}
	}

	// Verify optional fields
	if entries[0].Temperature == nil || *entries[0].Temperature != 0.7 {
		t.Error("expected Temperature to be 0.7")
	}
	if entries[0].MaxTokens == nil || *entries[0].MaxTokens != 4096 {
		t.Error("expected MaxTokens to be 4096")
	}
	if entries[1].Temperature != nil {
		t.Error("expected Temperature to be nil for second entry")
	}
}

func TestUserLLMService_SetFallbackChain_ReplacesExisting(t *testing.T) {
	svc, _, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	// Set initial chain
	svc.SetFallbackChain(ctx, "user-replace", []UserFallbackChainEntryInput{
		{Provider: "openrouter", Model: "old-model", IsEnabled: true},
	})

	// Replace with new chain
	entries, err := svc.SetFallbackChain(ctx, "user-replace", []UserFallbackChainEntryInput{
		{Provider: "anthropic", Model: "new-model-1", IsEnabled: true},
		{Provider: "openai", Model: "new-model-2", IsEnabled: true},
	})
	if err != nil {
		t.Fatalf("failed to replace chain: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Verify old entry is gone
	for _, e := range entries {
		if e.Model == "old-model" {
			t.Error("old entry should have been replaced")
		}
	}
}

func TestUserLLMService_SetFallbackChain_Empty(t *testing.T) {
	svc, _, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	// Set initial chain
	svc.SetFallbackChain(ctx, "user-clear", []UserFallbackChainEntryInput{
		{Provider: "openrouter", Model: "model", IsEnabled: true},
	})

	// Clear with empty list
	entries, err := svc.SetFallbackChain(ctx, "user-clear", []UserFallbackChainEntryInput{})
	if err != nil {
		t.Fatalf("failed to clear chain: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entries after clearing, got %d", len(entries))
	}
}

func TestUserLLMService_SetFallbackChain_EmptyModel(t *testing.T) {
	svc, _, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	_, err := svc.SetFallbackChain(ctx, "user-empty-model", []UserFallbackChainEntryInput{
		{Provider: "openrouter", Model: "", IsEnabled: true},
	})

	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

// ========================================
// GetDecryptedKey Tests
// ========================================

func TestUserLLMService_GetDecryptedKey(t *testing.T) {
	svc, _, _, encryptor := setupUserLLMService(t)
	ctx := context.Background()

	// Create key with encryption
	encrypted, _ := encryptor.Encrypt("my-secret-api-key")
	svc.UpsertServiceKey(ctx, "user-decrypt", UserServiceKeyInput{
		Provider:  "openrouter",
		APIKey:    "my-secret-api-key",
		IsEnabled: true,
	})

	_ = encrypted // Not needed, just verifying encryption works

	decrypted, err := svc.GetDecryptedKey(ctx, "user-decrypt", "openrouter")
	if err != nil {
		t.Fatalf("failed to get decrypted key: %v", err)
	}

	if decrypted != "my-secret-api-key" {
		t.Errorf("decrypted = %q, want %q", decrypted, "my-secret-api-key")
	}
}

func TestUserLLMService_GetDecryptedKey_NotFound(t *testing.T) {
	svc, _, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	decrypted, err := svc.GetDecryptedKey(ctx, "user-notfound", "openrouter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decrypted != "" {
		t.Errorf("expected empty string for nonexistent key, got %q", decrypted)
	}
}

func TestUserLLMService_GetDecryptedKey_Disabled(t *testing.T) {
	svc, _, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	// Create disabled key
	svc.UpsertServiceKey(ctx, "user-disabled", UserServiceKeyInput{
		Provider:  "anthropic",
		APIKey:    "disabled-key",
		IsEnabled: false,
	})

	decrypted, err := svc.GetDecryptedKey(ctx, "user-disabled", "anthropic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decrypted != "" {
		t.Errorf("expected empty string for disabled key, got %q", decrypted)
	}
}

func TestUserLLMService_GetDecryptedKey_NoEncryption(t *testing.T) {
	svc, _, _ := setupUserLLMServiceNoEncryption(t)
	ctx := context.Background()

	// Create key without encryption
	svc.UpsertServiceKey(ctx, "user-plain", UserServiceKeyInput{
		Provider:  "openai",
		APIKey:    "plaintext-key",
		IsEnabled: true,
	})

	decrypted, err := svc.GetDecryptedKey(ctx, "user-plain", "openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decrypted != "plaintext-key" {
		t.Errorf("decrypted = %q, want %q", decrypted, "plaintext-key")
	}
}

func TestUserLLMService_GetDecryptedKey_WrongProvider(t *testing.T) {
	svc, _, _, _ := setupUserLLMService(t)
	ctx := context.Background()

	// Create key for openrouter
	svc.UpsertServiceKey(ctx, "user-wrong", UserServiceKeyInput{
		Provider:  "openrouter",
		APIKey:    "openrouter-key",
		IsEnabled: true,
	})

	// Try to get anthropic key
	decrypted, err := svc.GetDecryptedKey(ctx, "user-wrong", "anthropic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decrypted != "" {
		t.Errorf("expected empty string for wrong provider, got %q", decrypted)
	}
}
