package service

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// mockServiceKeyRepository implements repository.ServiceKeyRepository for testing
type mockServiceKeyRepository struct {
	mu   sync.RWMutex
	keys []*models.ServiceKey
}

func newMockServiceKeyRepository() *mockServiceKeyRepository {
	return &mockServiceKeyRepository{
		keys: make([]*models.ServiceKey, 0),
	}
}

func (m *mockServiceKeyRepository) Upsert(ctx context.Context, key *models.ServiceKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Replace if exists
	for i, k := range m.keys {
		if k.Provider == key.Provider {
			m.keys[i] = key
			return nil
		}
	}
	m.keys = append(m.keys, key)
	return nil
}

func (m *mockServiceKeyRepository) GetByProvider(ctx context.Context, provider string) (*models.ServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, k := range m.keys {
		if k.Provider == provider {
			return k, nil
		}
	}
	return nil, nil
}

func (m *mockServiceKeyRepository) GetAll(ctx context.Context) ([]*models.ServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.keys, nil
}

func (m *mockServiceKeyRepository) GetEnabled(ctx context.Context) ([]*models.ServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var enabled []*models.ServiceKey
	for _, k := range m.keys {
		if k.IsEnabled {
			enabled = append(enabled, k)
		}
	}
	return enabled, nil
}

func (m *mockServiceKeyRepository) Delete(ctx context.Context, provider string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, k := range m.keys {
		if k.Provider == provider {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			return nil
		}
	}
	return nil
}

// mockFallbackChainRepository implements repository.FallbackChainRepository for testing
type mockFallbackChainRepository struct {
	mu      sync.RWMutex
	entries []*models.FallbackChainEntry
}

func newMockFallbackChainRepository() *mockFallbackChainRepository {
	return &mockFallbackChainRepository{
		entries: make([]*models.FallbackChainEntry, 0),
	}
}

func (m *mockFallbackChainRepository) GetAll(ctx context.Context) ([]*models.FallbackChainEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.entries, nil
}

func (m *mockFallbackChainRepository) GetByTier(ctx context.Context, tier *string) ([]*models.FallbackChainEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.FallbackChainEntry
	for _, e := range m.entries {
		if (tier == nil && e.Tier == nil) || (tier != nil && e.Tier != nil && *tier == *e.Tier) {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockFallbackChainRepository) GetEnabled(ctx context.Context) ([]*models.FallbackChainEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var enabled []*models.FallbackChainEntry
	for _, e := range m.entries {
		if e.IsEnabled && e.Tier == nil {
			enabled = append(enabled, e)
		}
	}
	return enabled, nil
}

func (m *mockFallbackChainRepository) GetEnabledByTier(ctx context.Context, tier string) ([]*models.FallbackChainEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.FallbackChainEntry
	for _, e := range m.entries {
		if e.IsEnabled && e.Tier != nil && *e.Tier == tier {
			result = append(result, e)
		}
	}
	// Fall back to default chain if no tier-specific entries
	if len(result) == 0 {
		return m.GetEnabled(ctx)
	}
	return result, nil
}

func (m *mockFallbackChainRepository) GetAllTiers(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockFallbackChainRepository) ReplaceAll(ctx context.Context, entries []*models.FallbackChainEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = entries
	return nil
}

func (m *mockFallbackChainRepository) ReplaceAllByTier(ctx context.Context, tier *string, entries []*models.FallbackChainEntry) error {
	return nil
}

func (m *mockFallbackChainRepository) DeleteByTier(ctx context.Context, tier string) error {
	return nil
}

func (m *mockFallbackChainRepository) Create(ctx context.Context, entry *models.FallbackChainEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockFallbackChainRepository) Update(ctx context.Context, entry *models.FallbackChainEntry) error {
	return nil
}

func (m *mockFallbackChainRepository) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockFallbackChainRepository) Reorder(ctx context.Context, ids []string) error {
	return nil
}

// mockUserServiceKeyRepository implements repository.UserServiceKeyRepository for testing
type mockUserServiceKeyRepository struct {
	mu   sync.RWMutex
	keys []*models.UserServiceKey
}

func newMockUserServiceKeyRepository() *mockUserServiceKeyRepository {
	return &mockUserServiceKeyRepository{
		keys: make([]*models.UserServiceKey, 0),
	}
}

func (m *mockUserServiceKeyRepository) Upsert(ctx context.Context, key *models.UserServiceKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Replace if exists
	for i, k := range m.keys {
		if k.UserID == key.UserID && k.Provider == key.Provider {
			m.keys[i] = key
			return nil
		}
	}
	m.keys = append(m.keys, key)
	return nil
}

func (m *mockUserServiceKeyRepository) GetByID(ctx context.Context, id string) (*models.UserServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, k := range m.keys {
		if k.ID == id {
			return k, nil
		}
	}
	return nil, nil
}

func (m *mockUserServiceKeyRepository) GetByUserID(ctx context.Context, userID string) ([]*models.UserServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.UserServiceKey
	for _, k := range m.keys {
		if k.UserID == userID {
			result = append(result, k)
		}
	}
	return result, nil
}

func (m *mockUserServiceKeyRepository) GetByUserAndProvider(ctx context.Context, userID, provider string) (*models.UserServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, k := range m.keys {
		if k.UserID == userID && k.Provider == provider {
			return k, nil
		}
	}
	return nil, nil
}

func (m *mockUserServiceKeyRepository) GetEnabledByUserID(ctx context.Context, userID string) ([]*models.UserServiceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.UserServiceKey
	for _, k := range m.keys {
		if k.UserID == userID && k.IsEnabled {
			result = append(result, k)
		}
	}
	return result, nil
}

func (m *mockUserServiceKeyRepository) Delete(ctx context.Context, id string) error {
	return nil
}

// mockUserFallbackChainRepository implements repository.UserFallbackChainRepository for testing
type mockUserFallbackChainRepository struct {
	mu      sync.RWMutex
	entries []*models.UserFallbackChainEntry
}

func newMockUserFallbackChainRepository() *mockUserFallbackChainRepository {
	return &mockUserFallbackChainRepository{
		entries: make([]*models.UserFallbackChainEntry, 0),
	}
}

func (m *mockUserFallbackChainRepository) GetByUserID(ctx context.Context, userID string) ([]*models.UserFallbackChainEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.UserFallbackChainEntry
	for _, e := range m.entries {
		if e.UserID == userID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockUserFallbackChainRepository) GetEnabledByUserID(ctx context.Context, userID string) ([]*models.UserFallbackChainEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.UserFallbackChainEntry
	for _, e := range m.entries {
		if e.UserID == userID && e.IsEnabled {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockUserFallbackChainRepository) ReplaceAll(ctx context.Context, userID string, entries []*models.UserFallbackChainEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Remove existing for user
	var keep []*models.UserFallbackChainEntry
	for _, e := range m.entries {
		if e.UserID != userID {
			keep = append(keep, e)
		}
	}
	m.entries = append(keep, entries...)
	return nil
}

// newTestLLMConfigResolver creates a resolver with mocks for testing
func newTestLLMConfigResolver() (
	*LLMConfigResolver,
	*mockServiceKeyRepository,
	*mockFallbackChainRepository,
	*mockUserServiceKeyRepository,
	*mockUserFallbackChainRepository,
) {
	serviceKeyRepo := newMockServiceKeyRepository()
	fallbackChainRepo := newMockFallbackChainRepository()
	userServiceKeyRepo := newMockUserServiceKeyRepository()
	userFallbackChainRepo := newMockUserFallbackChainRepository()

	repos := &repository.Repositories{
		ServiceKey:        serviceKeyRepo,
		FallbackChain:     fallbackChainRepo,
		UserServiceKey:    userServiceKeyRepo,
		UserFallbackChain: userFallbackChainRepo,
	}

	cfg := &config.Config{
		ServiceOpenRouterKey: "env-openrouter-key",
		ServiceAnthropicKey:  "env-anthropic-key",
		ServiceOpenAIKey:     "env-openai-key",
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	resolver := NewLLMConfigResolver(cfg, repos, nil, logger)
	return resolver, serviceKeyRepo, fallbackChainRepo, userServiceKeyRepo, userFallbackChainRepo
}

func TestGetServiceKeys_EnvVarFallback(t *testing.T) {
	resolver, _, _, _, _ := newTestLLMConfigResolver()
	ctx := context.Background()

	// No DB keys set - should use env vars
	keys := resolver.GetServiceKeys(ctx)

	if keys.OpenRouterKey != "env-openrouter-key" {
		t.Errorf("OpenRouterKey = %q, want %q", keys.OpenRouterKey, "env-openrouter-key")
	}
	if keys.AnthropicKey != "env-anthropic-key" {
		t.Errorf("AnthropicKey = %q, want %q", keys.AnthropicKey, "env-anthropic-key")
	}
	if keys.OpenAIKey != "env-openai-key" {
		t.Errorf("OpenAIKey = %q, want %q", keys.OpenAIKey, "env-openai-key")
	}
}

func TestGetServiceKeys_DBOverridesEnv(t *testing.T) {
	resolver, serviceKeyRepo, _, _, _ := newTestLLMConfigResolver()
	ctx := context.Background()

	// Set DB keys
	serviceKeyRepo.keys = []*models.ServiceKey{
		{Provider: "llm.ProviderOpenRouter", APIKeyEncrypted: "db-openrouter-key", IsEnabled: true},
		{Provider: "llm.ProviderAnthropic", APIKeyEncrypted: "db-anthropic-key", IsEnabled: true},
	}

	keys := resolver.GetServiceKeys(ctx)

	// DB keys should override env vars
	if keys.OpenRouterKey != "db-openrouter-key" {
		t.Errorf("OpenRouterKey = %q, want %q", keys.OpenRouterKey, "db-openrouter-key")
	}
	if keys.AnthropicKey != "db-anthropic-key" {
		t.Errorf("AnthropicKey = %q, want %q", keys.AnthropicKey, "db-anthropic-key")
	}
	// OpenAI should still use env var
	if keys.OpenAIKey != "env-openai-key" {
		t.Errorf("OpenAIKey = %q, want %q", keys.OpenAIKey, "env-openai-key")
	}
}

func TestGetServiceKeys_WithEncryption(t *testing.T) {
	// Create encryptor
	key := crypto.DeriveKeyFromSecret("test-secret")
	enc, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	// Encrypt a test key
	encrypted, err := enc.Encrypt("encrypted-api-key")
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	// Create resolver with encryptor
	serviceKeyRepo := newMockServiceKeyRepository()
	serviceKeyRepo.keys = []*models.ServiceKey{
		{Provider: "llm.ProviderOpenRouter", APIKeyEncrypted: encrypted, IsEnabled: true},
	}

	repos := &repository.Repositories{
		ServiceKey: serviceKeyRepo,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	resolver := NewLLMConfigResolver(cfg, repos, enc, logger)
	ctx := context.Background()

	keys := resolver.GetServiceKeys(ctx)

	// Should decrypt the key
	if keys.OpenRouterKey != "encrypted-api-key" {
		t.Errorf("OpenRouterKey = %q, want %q", keys.OpenRouterKey, "encrypted-api-key")
	}
}

func TestGetStrictMode(t *testing.T) {
	resolver, _, _, _, _ := newTestLLMConfigResolver()
	ctx := context.Background()

	tests := []struct {
		name            string
		provider        string
		model           string
		chainStrictMode *bool
		want            bool
	}{
		{
			name:            "chain override true",
			provider:        "openrouter",
			model:           "any-model",
			chainStrictMode: boolPtr(true),
			want:            true,
		},
		{
			name:            "chain override false",
			provider:        "openrouter",
			model:           "any-model",
			chainStrictMode: boolPtr(false),
			want:            false,
		},
		{
			name:            "no override uses default",
			provider:        "openrouter",
			model:           "google/gemma-3-27b-it:free",
			chainStrictMode: nil,
			// Default behavior depends on llm.GetModelSettings
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolver.GetStrictMode(ctx, tt.provider, tt.model, tt.chainStrictMode)

			// Only check for explicit chain overrides
			if tt.chainStrictMode != nil && got != tt.want {
				t.Errorf("GetStrictMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveConfigs_Override(t *testing.T) {
	resolver, _, _, _, _ := newTestLLMConfigResolver()
	ctx := context.Background()
	userID := "user_123"

	tests := []struct {
		name        string
		override    *LLMConfigInput
		byokAllowed bool
		wantBYOK    bool
		wantLen     int
	}{
		{
			name: "override with BYOK allowed",
			override: &LLMConfigInput{
				Provider: "openrouter",
				Model:    "gpt-4",
				APIKey:   "user-key",
			},
			byokAllowed: true,
			wantBYOK:    true,
			wantLen:     1,
		},
		{
			name: "override without BYOK - falls back to system",
			override: &LLMConfigInput{
				Provider: "openrouter",
				Model:    "gpt-4",
				APIKey:   "user-key",
			},
			byokAllowed: false,
			wantBYOK:    false,
			wantLen:     4, // System chain (3 openrouter + 1 ollama)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, isBYOK := resolver.ResolveConfigs(ctx, userID, tt.override, "free", tt.byokAllowed, false)

			if isBYOK != tt.wantBYOK {
				t.Errorf("isBYOK = %v, want %v", isBYOK, tt.wantBYOK)
			}
			if len(configs) != tt.wantLen {
				t.Errorf("len(configs) = %d, want %d", len(configs), tt.wantLen)
			}

			// For BYOK case, verify the override was used
			if tt.wantBYOK && len(configs) > 0 {
				if configs[0].Provider != tt.override.Provider {
					t.Errorf("provider = %s, want %s", configs[0].Provider, tt.override.Provider)
				}
			}
		})
	}
}

func TestResolveConfigs_BYOKAndModelsCustom(t *testing.T) {
	resolver, _, _, userServiceKeyRepo, userFallbackChainRepo := newTestLLMConfigResolver()
	ctx := context.Background()
	userID := "user_123"

	// Set up user's keys
	userServiceKeyRepo.keys = []*models.UserServiceKey{
		{UserID: userID, Provider: "openrouter", APIKeyEncrypted: "user-or-key", IsEnabled: true},
	}

	// Set up user's fallback chain
	userFallbackChainRepo.entries = []*models.UserFallbackChainEntry{
		{UserID: userID, Provider: "openrouter", Model: "user-model", Position: 1, IsEnabled: true},
	}

	configs, isBYOK := resolver.ResolveConfigs(ctx, userID, nil, "free", true, true)

	if !isBYOK {
		t.Error("expected isBYOK = true for BYOK + models_custom")
	}
	if len(configs) != 1 {
		t.Errorf("len(configs) = %d, want 1", len(configs))
	}
	if len(configs) > 0 {
		if configs[0].Provider != "openrouter" {
			t.Errorf("provider = %s, want openrouter", configs[0].Provider)
		}
		if configs[0].Model != "user-model" {
			t.Errorf("model = %s, want user-model", configs[0].Model)
		}
		if configs[0].APIKey != "user-or-key" {
			t.Errorf("apiKey = %s, want user-or-key", configs[0].APIKey)
		}
	}
}

func TestResolveConfigs_ModelsCustomOnly(t *testing.T) {
	resolver, _, _, _, userFallbackChainRepo := newTestLLMConfigResolver()
	ctx := context.Background()
	userID := "user_123"

	// Set up user's fallback chain (no user keys)
	userFallbackChainRepo.entries = []*models.UserFallbackChainEntry{
		{UserID: userID, Provider: "llm.ProviderOpenRouter", Model: "custom-model", Position: 1, IsEnabled: true},
	}

	configs, isBYOK := resolver.ResolveConfigs(ctx, userID, nil, "free", false, true)

	if isBYOK {
		t.Error("expected isBYOK = false for models_custom only")
	}
	if len(configs) != 1 {
		t.Errorf("len(configs) = %d, want 1", len(configs))
	}
	if len(configs) > 0 {
		if configs[0].Model != "custom-model" {
			t.Errorf("model = %s, want custom-model", configs[0].Model)
		}
		// Should use system key
		if configs[0].APIKey != "env-openrouter-key" {
			t.Errorf("apiKey = %s, want env-openrouter-key", configs[0].APIKey)
		}
	}
}

func TestResolveConfigs_BYOKOnly(t *testing.T) {
	resolver, _, fallbackChainRepo, userServiceKeyRepo, _ := newTestLLMConfigResolver()
	ctx := context.Background()
	userID := "user_123"

	// Set up user's keys (no user chain)
	userServiceKeyRepo.keys = []*models.UserServiceKey{
		{UserID: userID, Provider: "openrouter", APIKeyEncrypted: "user-or-key", IsEnabled: true},
	}

	// Set up system fallback chain
	fallbackChainRepo.entries = []*models.FallbackChainEntry{
		{Provider: "openrouter", Model: "system-model", Position: 1, IsEnabled: true},
	}

	configs, isBYOK := resolver.ResolveConfigs(ctx, userID, nil, "free", true, false)

	if !isBYOK {
		t.Error("expected isBYOK = true for BYOK only")
	}
	if len(configs) < 1 {
		t.Errorf("expected at least 1 config, got %d", len(configs))
	}
	if len(configs) > 0 {
		// Should use system model with user key
		if configs[0].Model != "system-model" {
			t.Errorf("model = %s, want system-model", configs[0].Model)
		}
		if configs[0].APIKey != "user-or-key" {
			t.Errorf("apiKey = %s, want user-or-key", configs[0].APIKey)
		}
	}
}

func TestResolveConfigs_Default(t *testing.T) {
	resolver, _, _, _, _ := newTestLLMConfigResolver()
	ctx := context.Background()
	userID := "user_123"

	configs, isBYOK := resolver.ResolveConfigs(ctx, userID, nil, "free", false, false)

	if isBYOK {
		t.Error("expected isBYOK = false for default case")
	}
	// Should get hardcoded chain (3 openrouter + 1 ollama)
	if len(configs) < 1 {
		t.Error("expected at least one config in default chain")
	}
}

func TestGetDefaultConfigsForTier(t *testing.T) {
	resolver, _, fallbackChainRepo, _, _ := newTestLLMConfigResolver()
	ctx := context.Background()

	// Set up tier-specific chain
	proTier := "pro"
	fallbackChainRepo.entries = []*models.FallbackChainEntry{
		{Tier: &proTier, Provider: "llm.ProviderOpenRouter", Model: "pro-model", Position: 1, IsEnabled: true},
		{Tier: nil, Provider: "llm.ProviderOpenRouter", Model: "default-model", Position: 1, IsEnabled: true},
	}

	// Test tier-specific chain
	configs := resolver.GetDefaultConfigsForTier(ctx, "pro")
	if len(configs) != 1 {
		t.Errorf("len(configs) = %d, want 1", len(configs))
	}
	if len(configs) > 0 && configs[0].Model != "pro-model" {
		t.Errorf("model = %s, want pro-model", configs[0].Model)
	}

	// Test fallback to default chain
	configs = resolver.GetDefaultConfigsForTier(ctx, "nonexistent")
	if len(configs) != 1 {
		t.Errorf("len(configs) = %d, want 1", len(configs))
	}
	if len(configs) > 0 && configs[0].Model != "default-model" {
		t.Errorf("model = %s, want default-model", configs[0].Model)
	}
}

func TestGetDefaultConfigsForTier_HardcodedFallback(t *testing.T) {
	// Create resolver without any DB chain
	repos := &repository.Repositories{
		FallbackChain: nil, // No fallback chain repo
	}

	cfg := &config.Config{
		ServiceOpenRouterKey: "env-key",
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver := NewLLMConfigResolver(cfg, repos, nil, logger)
	ctx := context.Background()

	configs := resolver.GetDefaultConfigsForTier(ctx, "free")

	// Should return hardcoded chain (3 openrouter + 1 ollama)
	if len(configs) != 4 {
		t.Errorf("len(configs) = %d, want 4", len(configs))
	}

	// Last should be ollama
	if len(configs) > 0 && configs[len(configs)-1].Provider != "llm.ProviderOllama" {
		t.Errorf("last provider = %s, want llm.ProviderOllama", configs[len(configs)-1].Provider)
	}
}

func TestGetDefaultConfig(t *testing.T) {
	resolver, _, _, _, _ := newTestLLMConfigResolver()
	ctx := context.Background()

	config := resolver.GetDefaultConfig(ctx, "free")
	if config == nil {
		t.Fatal("expected non-nil config")
	}

	// Should get first config from chain
	if config.Provider == "" {
		t.Error("expected provider to be set")
	}
}

func TestBuildUserFallbackChain_NoKeys(t *testing.T) {
	resolver, _, _, _, userFallbackChainRepo := newTestLLMConfigResolver()
	ctx := context.Background()
	userID := "user_123"

	// Set up user chain but no keys
	userFallbackChainRepo.entries = []*models.UserFallbackChainEntry{
		{UserID: userID, Provider: "openrouter", Model: "model-1", Position: 1, IsEnabled: true},
	}

	configs := resolver.BuildUserFallbackChain(ctx, userID)

	// Should return nil - no keys for openrouter
	if len(configs) != 0 {
		t.Errorf("expected empty configs when no keys, got %d", len(configs))
	}
}

func TestBuildUserFallbackChain_OllamaNoKey(t *testing.T) {
	resolver, _, _, userServiceKeyRepo, userFallbackChainRepo := newTestLLMConfigResolver()
	ctx := context.Background()
	userID := "user_123"

	// Set up ollama entry (doesn't need key)
	userFallbackChainRepo.entries = []*models.UserFallbackChainEntry{
		{UserID: userID, Provider: "llm.ProviderOllama", Model: "llama3.2", Position: 1, IsEnabled: true},
	}

	// No keys needed for ollama
	userServiceKeyRepo.keys = []*models.UserServiceKey{}

	configs := resolver.BuildUserFallbackChain(ctx, userID)

	if len(configs) != 1 {
		t.Errorf("expected 1 config for ollama, got %d", len(configs))
	}
	if len(configs) > 0 && configs[0].Provider != "llm.ProviderOllama" {
		t.Errorf("provider = %s, want llm.ProviderOllama", configs[0].Provider)
	}
}

func TestResolveConfig_SingleConfig(t *testing.T) {
	resolver, _, _, _, _ := newTestLLMConfigResolver()
	ctx := context.Background()
	userID := "user_123"

	config, isBYOK := resolver.ResolveConfig(ctx, userID, nil, "free", false, false)

	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if isBYOK {
		t.Error("expected isBYOK = false")
	}
}

// Helper functions
func boolPtr(b bool) *bool {
	return &b
}
