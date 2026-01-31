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
// Mock Repositories for AdminService Tests
// ========================================

// adminServiceKeyRepository implements repository.ServiceKeyRepository for testing.
type adminServiceKeyRepository struct {
	mu   sync.RWMutex
	keys map[string]*models.ServiceKey // keyed by provider
}

func newAdminServiceKeyRepository() *adminServiceKeyRepository {
	return &adminServiceKeyRepository{
		keys: make(map[string]*models.ServiceKey),
	}
}

func (m *adminServiceKeyRepository) Upsert(ctx context.Context, key *models.ServiceKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.keys[key.Provider]; ok {
		// Update existing
		key.ID = existing.ID
		key.CreatedAt = existing.CreatedAt
	} else {
		// Create new
		key.ID = ulid.Make().String()
		key.CreatedAt = time.Now()
	}
	key.UpdatedAt = time.Now()
	m.keys[key.Provider] = key
	return nil
}

func (m *adminServiceKeyRepository) GetByProvider(ctx context.Context, provider string) (*models.ServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if key, ok := m.keys[provider]; ok {
		return key, nil
	}
	return nil, nil
}

func (m *adminServiceKeyRepository) GetAll(ctx context.Context) ([]*models.ServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*models.ServiceKey, 0, len(m.keys))
	for _, key := range m.keys {
		result = append(result, key)
	}
	// Sort by provider for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].Provider < result[j].Provider
	})
	return result, nil
}

func (m *adminServiceKeyRepository) GetEnabled(ctx context.Context) ([]*models.ServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.ServiceKey
	for _, key := range m.keys {
		if key.IsEnabled {
			result = append(result, key)
		}
	}
	return result, nil
}

func (m *adminServiceKeyRepository) Delete(ctx context.Context, provider string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.keys, provider)
	return nil
}

// adminFallbackChainRepository implements repository.FallbackChainRepository for testing.
type adminFallbackChainRepository struct {
	mu      sync.RWMutex
	entries map[string][]*models.FallbackChainEntry // keyed by tier ("" for default)
}

func newAdminFallbackChainRepository() *adminFallbackChainRepository {
	return &adminFallbackChainRepository{
		entries: make(map[string][]*models.FallbackChainEntry),
	}
}

func tierKey(tier *string) string {
	if tier == nil {
		return ""
	}
	return *tier
}

func (m *adminFallbackChainRepository) GetAll(ctx context.Context) ([]*models.FallbackChainEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.FallbackChainEntry
	for _, entries := range m.entries {
		result = append(result, entries...)
	}
	// Sort by tier then position
	sort.Slice(result, func(i, j int) bool {
		iTier := ""
		if result[i].Tier != nil {
			iTier = *result[i].Tier
		}
		jTier := ""
		if result[j].Tier != nil {
			jTier = *result[j].Tier
		}
		if iTier != jTier {
			return iTier < jTier
		}
		return result[i].Position < result[j].Position
	})
	return result, nil
}

func (m *adminFallbackChainRepository) GetByTier(ctx context.Context, tier *string) ([]*models.FallbackChainEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := tierKey(tier)
	if entries, ok := m.entries[key]; ok {
		result := make([]*models.FallbackChainEntry, len(entries))
		copy(result, entries)
		sort.Slice(result, func(i, j int) bool {
			return result[i].Position < result[j].Position
		})
		return result, nil
	}
	return []*models.FallbackChainEntry{}, nil
}

func (m *adminFallbackChainRepository) GetEnabled(ctx context.Context) ([]*models.FallbackChainEntry, error) {
	return m.GetEnabledByTier(ctx, "")
}

func (m *adminFallbackChainRepository) GetEnabledByTier(ctx context.Context, tier string) ([]*models.FallbackChainEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try tier-specific first
	if tier != "" {
		if entries, ok := m.entries[tier]; ok {
			var result []*models.FallbackChainEntry
			for _, e := range entries {
				if e.IsEnabled {
					result = append(result, e)
				}
			}
			if len(result) > 0 {
				sort.Slice(result, func(i, j int) bool {
					return result[i].Position < result[j].Position
				})
				return result, nil
			}
		}
	}

	// Fall back to default
	if entries, ok := m.entries[""]; ok {
		var result []*models.FallbackChainEntry
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

	return []*models.FallbackChainEntry{}, nil
}

func (m *adminFallbackChainRepository) GetAllTiers(ctx context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var tiers []string
	for tier := range m.entries {
		if tier != "" { // Exclude default
			tiers = append(tiers, tier)
		}
	}
	sort.Strings(tiers)
	return tiers, nil
}

func (m *adminFallbackChainRepository) ReplaceAll(ctx context.Context, entries []*models.FallbackChainEntry) error {
	return m.ReplaceAllByTier(ctx, nil, entries)
}

func (m *adminFallbackChainRepository) ReplaceAllByTier(ctx context.Context, tier *string, entries []*models.FallbackChainEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := tierKey(tier)
	now := time.Now()

	for i, e := range entries {
		e.ID = ulid.Make().String()
		e.Tier = tier
		e.Position = i + 1
		e.CreatedAt = now
		e.UpdatedAt = now
	}

	m.entries[key] = entries
	return nil
}

func (m *adminFallbackChainRepository) DeleteByTier(ctx context.Context, tier string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, tier)
	return nil
}

func (m *adminFallbackChainRepository) Create(ctx context.Context, entry *models.FallbackChainEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tierKey(entry.Tier)
	entry.ID = ulid.Make().String()
	entry.CreatedAt = time.Now()
	entry.UpdatedAt = time.Now()
	m.entries[key] = append(m.entries[key], entry)
	return nil
}

func (m *adminFallbackChainRepository) Update(ctx context.Context, entry *models.FallbackChainEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tierKey(entry.Tier)
	for i, e := range m.entries[key] {
		if e.ID == entry.ID {
			entry.UpdatedAt = time.Now()
			m.entries[key][i] = entry
			return nil
		}
	}
	return nil
}

func (m *adminFallbackChainRepository) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for tier, entries := range m.entries {
		for i, e := range entries {
			if e.ID == id {
				m.entries[tier] = append(entries[:i], entries[i+1:]...)
				return nil
			}
		}
	}
	return nil
}

func (m *adminFallbackChainRepository) Reorder(ctx context.Context, ids []string) error {
	// Not implemented for tests
	return nil
}

// ========================================
// Test Setup Helpers
// ========================================

func setupAdminService(t *testing.T) (*AdminService, *adminServiceKeyRepository, *adminFallbackChainRepository, *crypto.Encryptor) {
	t.Helper()

	keyRepo := newAdminServiceKeyRepository()
	chainRepo := newAdminFallbackChainRepository()

	repos := &repository.Repositories{
		ServiceKey:    keyRepo,
		FallbackChain: chainRepo,
	}

	// Create a test encryptor with a fixed key
	testKey := []byte("12345678901234567890123456789012") // 32 bytes for AES-256
	encryptor, err := crypto.NewEncryptor(testKey)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	logger := slog.Default()
	svc := NewAdminService(repos, encryptor, logger)

	return svc, keyRepo, chainRepo, encryptor
}

func setupAdminServiceNoEncryption(t *testing.T) (*AdminService, *adminServiceKeyRepository, *adminFallbackChainRepository) {
	t.Helper()

	keyRepo := newAdminServiceKeyRepository()
	chainRepo := newAdminFallbackChainRepository()

	repos := &repository.Repositories{
		ServiceKey:    keyRepo,
		FallbackChain: chainRepo,
	}

	logger := slog.Default()
	svc := NewAdminService(repos, nil, logger)

	return svc, keyRepo, chainRepo
}

// ========================================
// ListServiceKeys Tests
// ========================================

func TestAdminService_ListServiceKeys(t *testing.T) {
	svc, keyRepo, _, _ := setupAdminService(t)
	ctx := context.Background()

	// Create some keys
	keyRepo.Upsert(ctx, &models.ServiceKey{
		Provider:        "openrouter",
		APIKeyEncrypted: "encrypted-1",
		IsEnabled:       true,
	})
	keyRepo.Upsert(ctx, &models.ServiceKey{
		Provider:        "anthropic",
		APIKeyEncrypted: "encrypted-2",
		IsEnabled:       true,
	})

	keys, err := svc.ListServiceKeys(ctx)
	if err != nil {
		t.Fatalf("failed to list keys: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestAdminService_ListServiceKeys_Empty(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	keys, err := svc.ListServiceKeys(ctx)
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

func TestAdminService_UpsertServiceKey_Create(t *testing.T) {
	svc, _, _, encryptor := setupAdminService(t)
	ctx := context.Background()

	input := ServiceKeyInput{
		Provider:  "openrouter",
		APIKey:    "test-api-key",
		IsEnabled: true,
	}

	key, err := svc.UpsertServiceKey(ctx, input)
	if err != nil {
		t.Fatalf("failed to upsert key: %v", err)
	}

	if key == nil {
		t.Fatal("expected key, got nil")
	}
	if key.ID == "" {
		t.Error("expected ID to be generated")
	}
	if key.Provider != "openrouter" {
		t.Errorf("Provider = %q, want %q", key.Provider, "openrouter")
	}
	if !key.IsEnabled {
		t.Error("expected IsEnabled to be true")
	}

	// Verify key was encrypted
	decrypted, err := encryptor.Decrypt(key.APIKeyEncrypted)
	if err != nil {
		t.Fatalf("failed to decrypt key: %v", err)
	}
	if decrypted != "test-api-key" {
		t.Errorf("decrypted key = %q, want %q", decrypted, "test-api-key")
	}
}

func TestAdminService_UpsertServiceKey_Update(t *testing.T) {
	svc, _, _, encryptor := setupAdminService(t)
	ctx := context.Background()

	// Create initial key
	svc.UpsertServiceKey(ctx, ServiceKeyInput{
		Provider:  "anthropic",
		APIKey:    "old-key",
		IsEnabled: true,
	})

	// Update
	key, err := svc.UpsertServiceKey(ctx, ServiceKeyInput{
		Provider:  "anthropic",
		APIKey:    "new-key",
		IsEnabled: false,
	})
	if err != nil {
		t.Fatalf("failed to update key: %v", err)
	}

	if key.IsEnabled {
		t.Error("expected IsEnabled to be false")
	}

	decrypted, _ := encryptor.Decrypt(key.APIKeyEncrypted)
	if decrypted != "new-key" {
		t.Errorf("decrypted key = %q, want %q", decrypted, "new-key")
	}
}

// TestAdminService_UpsertServiceKey_InvalidProvider removed - provider validation
// is now done at handler level against the LLM registry, not at service level

func TestAdminService_UpsertServiceKey_NoEncryption(t *testing.T) {
	svc, _, _ := setupAdminServiceNoEncryption(t)
	ctx := context.Background()

	key, err := svc.UpsertServiceKey(ctx, ServiceKeyInput{
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

func TestAdminService_DeleteServiceKey(t *testing.T) {
	svc, keyRepo, _, _ := setupAdminService(t)
	ctx := context.Background()

	// Create a key
	keyRepo.Upsert(ctx, &models.ServiceKey{
		Provider:        "openrouter",
		APIKeyEncrypted: "encrypted-key",
		IsEnabled:       true,
	})

	err := svc.DeleteServiceKey(ctx, "openrouter")
	if err != nil {
		t.Fatalf("failed to delete key: %v", err)
	}

	// Verify deleted
	keys, _ := svc.ListServiceKeys(ctx)
	if len(keys) != 0 {
		t.Errorf("expected 0 keys after delete, got %d", len(keys))
	}
}

func TestAdminService_DeleteServiceKey_NonExistent(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	// Delete should not error even if not found
	err := svc.DeleteServiceKey(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ========================================
// GetFallbackChain Tests
// ========================================

func TestAdminService_GetFallbackChain(t *testing.T) {
	svc, _, chainRepo, _ := setupAdminService(t)
	ctx := context.Background()

	// Create entries for default chain
	chainRepo.ReplaceAllByTier(ctx, nil, []*models.FallbackChainEntry{
		{Provider: "openrouter", Model: "model-1", IsEnabled: true},
		{Provider: "anthropic", Model: "model-2", IsEnabled: true},
	})

	// Create entries for pro tier
	proTier := "pro"
	chainRepo.ReplaceAllByTier(ctx, &proTier, []*models.FallbackChainEntry{
		{Provider: "openai", Model: "gpt-4", IsEnabled: true},
	})

	entries, err := svc.GetFallbackChain(ctx)
	if err != nil {
		t.Fatalf("failed to get fallback chain: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestAdminService_GetFallbackChainByTier(t *testing.T) {
	svc, _, chainRepo, _ := setupAdminService(t)
	ctx := context.Background()

	// Create entries for pro tier
	proTier := "pro"
	chainRepo.ReplaceAllByTier(ctx, &proTier, []*models.FallbackChainEntry{
		{Provider: "openai", Model: "gpt-4", IsEnabled: true},
		{Provider: "anthropic", Model: "claude-3", IsEnabled: true},
	})

	entries, err := svc.GetFallbackChainByTier(ctx, &proTier)
	if err != nil {
		t.Fatalf("failed to get fallback chain by tier: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 entries for pro tier, got %d", len(entries))
	}

	for _, e := range entries {
		if e.Tier == nil || *e.Tier != "pro" {
			t.Error("expected all entries to have tier 'pro'")
		}
	}
}

func TestAdminService_GetFallbackChainByTier_Default(t *testing.T) {
	svc, _, chainRepo, _ := setupAdminService(t)
	ctx := context.Background()

	// Create entries for default chain
	chainRepo.ReplaceAllByTier(ctx, nil, []*models.FallbackChainEntry{
		{Provider: "openrouter", Model: "default-model", IsEnabled: true},
	})

	entries, err := svc.GetFallbackChainByTier(ctx, nil)
	if err != nil {
		t.Fatalf("failed to get default chain: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry for default chain, got %d", len(entries))
	}
	if entries[0].Tier != nil {
		t.Error("expected tier to be nil for default chain")
	}
}

func TestAdminService_GetAllTiers(t *testing.T) {
	svc, _, chainRepo, _ := setupAdminService(t)
	ctx := context.Background()

	// Create chains for multiple tiers
	chainRepo.ReplaceAllByTier(ctx, nil, []*models.FallbackChainEntry{
		{Provider: "openrouter", Model: "default-model", IsEnabled: true},
	})
	proTier := "pro"
	chainRepo.ReplaceAllByTier(ctx, &proTier, []*models.FallbackChainEntry{
		{Provider: "openai", Model: "gpt-4", IsEnabled: true},
	})
	enterpriseTier := "enterprise"
	chainRepo.ReplaceAllByTier(ctx, &enterpriseTier, []*models.FallbackChainEntry{
		{Provider: "anthropic", Model: "claude-opus", IsEnabled: true},
	})

	tiers, err := svc.GetAllTiers(ctx)
	if err != nil {
		t.Fatalf("failed to get all tiers: %v", err)
	}

	// Should not include default (nil tier)
	if len(tiers) != 2 {
		t.Errorf("expected 2 tiers, got %d", len(tiers))
	}

	// Check tiers are present (sorted)
	if tiers[0] != "enterprise" {
		t.Errorf("tiers[0] = %q, want %q", tiers[0], "enterprise")
	}
	if tiers[1] != "pro" {
		t.Errorf("tiers[1] = %q, want %q", tiers[1], "pro")
	}
}

// ========================================
// SetFallbackChain Tests
// ========================================

func TestAdminService_SetFallbackChain_Default(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	temp := 0.7
	maxTokens := 4096

	input := FallbackChainInput{
		Tier: nil, // Default chain
		Entries: []FallbackChainEntryInput{
			{Provider: "openrouter", Model: "claude-3-haiku", Temperature: &temp, MaxTokens: &maxTokens, IsEnabled: true},
			{Provider: "anthropic", Model: "claude-3-sonnet", IsEnabled: true},
			{Provider: "openai", Model: "gpt-4o-mini", IsEnabled: false},
		},
	}

	entries, err := svc.SetFallbackChain(ctx, input)
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
		if e.Tier != nil {
			t.Errorf("entries[%d].Tier should be nil for default chain", i)
		}
	}

	// Verify optional fields
	if entries[0].Temperature == nil || *entries[0].Temperature != 0.7 {
		t.Error("expected Temperature to be 0.7")
	}
	if entries[0].MaxTokens == nil || *entries[0].MaxTokens != 4096 {
		t.Error("expected MaxTokens to be 4096")
	}
}

func TestAdminService_SetFallbackChain_WithTier(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	proTier := "pro"
	input := FallbackChainInput{
		Tier: &proTier,
		Entries: []FallbackChainEntryInput{
			{Provider: "openai", Model: "gpt-4", IsEnabled: true},
		},
	}

	entries, err := svc.SetFallbackChain(ctx, input)
	if err != nil {
		t.Fatalf("failed to set fallback chain: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Tier == nil || *entries[0].Tier != "pro" {
		t.Error("expected tier to be 'pro'")
	}
}

func TestAdminService_SetFallbackChain_ReplacesExisting(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	// Set initial chain
	svc.SetFallbackChain(ctx, FallbackChainInput{
		Entries: []FallbackChainEntryInput{
			{Provider: "openrouter", Model: "old-model", IsEnabled: true},
		},
	})

	// Replace with new chain
	entries, err := svc.SetFallbackChain(ctx, FallbackChainInput{
		Entries: []FallbackChainEntryInput{
			{Provider: "anthropic", Model: "new-model-1", IsEnabled: true},
			{Provider: "openai", Model: "new-model-2", IsEnabled: true},
		},
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

// TestAdminService_SetFallbackChain_InvalidProvider removed - provider validation
// is now done at handler level against the LLM registry, not at service level

func TestAdminService_SetFallbackChain_EmptyModel(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	_, err := svc.SetFallbackChain(ctx, FallbackChainInput{
		Entries: []FallbackChainEntryInput{
			{Provider: "openrouter", Model: "", IsEnabled: true},
		},
	})

	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestAdminService_SetFallbackChain_OllamaValid(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	// Ollama is valid in chains (doesn't require API key)
	entries, err := svc.SetFallbackChain(ctx, FallbackChainInput{
		Entries: []FallbackChainEntryInput{
			{Provider: "ollama", Model: "llama3.2", IsEnabled: true},
		},
	})

	if err != nil {
		t.Fatalf("expected ollama to be valid: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Provider != "ollama" {
		t.Errorf("Provider = %q, want %q", entries[0].Provider, "ollama")
	}
}

// ========================================
// DeleteFallbackChainByTier Tests
// ========================================

func TestAdminService_DeleteFallbackChainByTier(t *testing.T) {
	svc, _, chainRepo, _ := setupAdminService(t)
	ctx := context.Background()

	// Create chain for pro tier
	proTier := "pro"
	chainRepo.ReplaceAllByTier(ctx, &proTier, []*models.FallbackChainEntry{
		{Provider: "openai", Model: "gpt-4", IsEnabled: true},
	})

	err := svc.DeleteFallbackChainByTier(ctx, "pro")
	if err != nil {
		t.Fatalf("failed to delete chain by tier: %v", err)
	}

	// Verify deleted
	entries, _ := svc.GetFallbackChainByTier(ctx, &proTier)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after delete, got %d", len(entries))
	}
}

// ========================================
// ListModels Tests (Static Lists)
// ========================================

func TestAdminService_ListModels_Anthropic(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	models, err := svc.ListModels(ctx, "anthropic")
	if err != nil {
		t.Fatalf("failed to list Anthropic models: %v", err)
	}

	if len(models) == 0 {
		t.Error("expected at least some Anthropic models")
	}

	// Check that known models are present
	foundSonnet := false
	for _, m := range models {
		if m.ID == "claude-sonnet-4-5-20250514" {
			foundSonnet = true
			if m.ContextSize != 200000 {
				t.Errorf("claude-sonnet context size = %d, want 200000", m.ContextSize)
			}
		}
	}
	if !foundSonnet {
		t.Error("expected to find claude-sonnet-4-5-20250514 in Anthropic models")
	}
}

func TestAdminService_ListModels_OpenAI_Static(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	// Without a configured OpenAI key, should return static list
	models, err := svc.ListModels(ctx, "openai")
	if err != nil {
		t.Fatalf("failed to list OpenAI models: %v", err)
	}

	if len(models) == 0 {
		t.Error("expected at least some OpenAI models")
	}

	// Check that known models are present
	foundGPT4o := false
	for _, m := range models {
		if m.ID == "gpt-4o" {
			foundGPT4o = true
		}
	}
	if !foundGPT4o {
		t.Error("expected to find gpt-4o in OpenAI models")
	}
}

func TestAdminService_ListModels_UnknownProvider(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	_, err := svc.ListModels(ctx, "unknown-provider")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

// ========================================
// ValidateModels Tests
// ========================================

func TestAdminService_ValidateModels_Anthropic(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	input := ValidateModelsInput{
		Models: []struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}{
			{Provider: "anthropic", Model: "claude-sonnet-4-5-20250514"},
			{Provider: "anthropic", Model: "nonexistent-model"},
		},
	}

	results, err := svc.ValidateModels(ctx, input)
	if err != nil {
		t.Fatalf("failed to validate models: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First model should be valid
	if results[0].Status != ModelStatusValid {
		t.Errorf("results[0].Status = %q, want %q", results[0].Status, ModelStatusValid)
	}

	// Second model should be unknown (not in static list, but may still work)
	if results[1].Status != ModelStatusUnknown {
		t.Errorf("results[1].Status = %q, want %q", results[1].Status, ModelStatusUnknown)
	}
}

// ========================================
// Subscription Tiers Tests (Without Clerk)
// ========================================

func TestAdminService_ListSubscriptionTiers_NoClerk(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	tiers, err := svc.ListSubscriptionTiers(ctx)
	if err != nil {
		t.Fatalf("failed to list tiers: %v", err)
	}

	// Should return hardcoded tiers when Clerk is not configured
	if len(tiers) != 3 {
		t.Errorf("expected 3 hardcoded tiers, got %d", len(tiers))
	}

	// Check for expected tiers
	tierSlugs := make(map[string]bool)
	for _, tier := range tiers {
		tierSlugs[tier.Slug] = true
	}

	expectedSlugs := []string{"free", "pro", "enterprise"}
	for _, slug := range expectedSlugs {
		if !tierSlugs[slug] {
			t.Errorf("expected tier %q to be present", slug)
		}
	}
}

func TestAdminService_ValidateTierExists_NoClerk(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	tests := []struct {
		tierID        string
		expectExists  bool
		expectedSlug  string
	}{
		{"free", true, "free"},
		{"pro", true, "pro"},
		{"enterprise", true, "enterprise"},
		{"nonexistent", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.tierID, func(t *testing.T) {
			exists, slug, err := svc.ValidateTierExists(ctx, tt.tierID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if exists != tt.expectExists {
				t.Errorf("exists = %v, want %v", exists, tt.expectExists)
			}
			if slug != tt.expectedSlug {
				t.Errorf("slug = %q, want %q", slug, tt.expectedSlug)
			}
		})
	}
}

func TestAdminService_ValidateTiers_NoClerk(t *testing.T) {
	svc, _, _, _ := setupAdminService(t)
	ctx := context.Background()

	results, err := svc.ValidateTiers(ctx, []string{"free", "pro", "invalid"})
	if err != nil {
		t.Fatalf("failed to validate tiers: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// free should be valid
	if results[0].Status != "valid" {
		t.Errorf("results[0].Status = %q, want %q", results[0].Status, "valid")
	}
	if results[0].CurrentSlug != "free" {
		t.Errorf("results[0].CurrentSlug = %q, want %q", results[0].CurrentSlug, "free")
	}

	// pro should be valid
	if results[1].Status != "valid" {
		t.Errorf("results[1].Status = %q, want %q", results[1].Status, "valid")
	}

	// invalid should be not_found
	if results[2].Status != "not_found" {
		t.Errorf("results[2].Status = %q, want %q", results[2].Status, "not_found")
	}
}

// ========================================
// Helper Function Tests
// ========================================

func TestFormatOpenAIModelName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"gpt-4", "Gpt 4"},
		{"gpt-4o-mini", "Gpt 4o Mini"},
		{"o1", "O1"},
		{"o3-mini", "O3 Mini"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := formatOpenAIModelName(tt.input)
			if result != tt.expected {
				t.Errorf("formatOpenAIModelName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestIsValidProvider and TestIsValidChainProvider removed - provider validation
// is now done at handler level against the LLM registry

