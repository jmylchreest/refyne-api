package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// mockAPIKeyRepository implements repository.APIKeyRepository for testing.
type mockAPIKeyRepository struct {
	mu   sync.RWMutex
	keys map[string]*models.APIKey // keyed by hash
}

func newMockAPIKeyRepository() *mockAPIKeyRepository {
	return &mockAPIKeyRepository{
		keys: make(map[string]*models.APIKey),
	}
}

func (m *mockAPIKeyRepository) Create(ctx context.Context, key *models.APIKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys[key.KeyHash] = key
	return nil
}

func (m *mockAPIKeyRepository) GetByID(ctx context.Context, id string) (*models.APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, key := range m.keys {
		if key.ID == id {
			return key, nil
		}
	}
	return nil, nil
}

func (m *mockAPIKeyRepository) GetByKeyHash(ctx context.Context, hash string) (*models.APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if key, ok := m.keys[hash]; ok {
		return key, nil
	}
	return nil, nil
}

func (m *mockAPIKeyRepository) GetByUserID(ctx context.Context, userID string) ([]*models.APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.APIKey
	for _, key := range m.keys {
		if key.UserID == userID {
			result = append(result, key)
		}
	}
	return result, nil
}

func (m *mockAPIKeyRepository) UpdateLastUsed(ctx context.Context, id string, lastUsed time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range m.keys {
		if key.ID == id {
			key.LastUsedAt = &lastUsed
			return nil
		}
	}
	return nil
}

func (m *mockAPIKeyRepository) Revoke(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range m.keys {
		if key.ID == id {
			now := time.Now()
			key.RevokedAt = &now
			return nil
		}
	}
	return nil
}

// hashAPIKey computes the SHA256 hash of an API key (same as auth_service).
func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}

// ========================================
// ValidateAPIKey Tests
// ========================================

func TestValidateAPIKey_ValidKey(t *testing.T) {
	mockRepo := newMockAPIKeyRepository()
	cfg := &config.Config{
		DeploymentMode: "hosted",
	}
	repos := &repository.Repositories{
		APIKey: mockRepo,
	}

	logger := slog.Default()
	svc := NewAuthService(cfg, repos, logger)

	// Create a valid API key
	testKey := "rf_test_1234567890abcdef"
	keyHash := hashAPIKey(testKey)
	mockRepo.Create(context.Background(), &models.APIKey{
		ID:        "key-1",
		UserID:    "user-123",
		Name:      "Test Key",
		KeyHash:   keyHash,
		KeyPrefix: "rf_test_",
		Scopes:    []string{"jobs:read", "jobs:write"},
		CreatedAt: time.Now(),
	})

	// Validate the key
	claims, err := svc.ValidateAPIKey(context.Background(), testKey)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if claims == nil {
		t.Fatal("expected claims, got nil")
	}
	if claims.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-123")
	}
	if claims.Tier != "free" {
		t.Errorf("Tier = %q, want %q", claims.Tier, "free")
	}
	if len(claims.Scopes) != 2 {
		t.Errorf("Scopes length = %d, want 2", len(claims.Scopes))
	}
	if claims.GlobalSuperadmin {
		t.Error("expected GlobalSuperadmin to be false for API key")
	}
}

func TestValidateAPIKey_NonExistentKey(t *testing.T) {
	mockRepo := newMockAPIKeyRepository()
	cfg := &config.Config{
		DeploymentMode: "hosted",
	}
	repos := &repository.Repositories{
		APIKey: mockRepo,
	}

	logger := slog.Default()
	svc := NewAuthService(cfg, repos, logger)

	// Try to validate a non-existent key
	claims, err := svc.ValidateAPIKey(context.Background(), "rf_nonexistent_key")
	if err == nil {
		t.Fatal("expected error for non-existent key")
	}
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
	if claims != nil {
		t.Error("expected nil claims for non-existent key")
	}
}

func TestValidateAPIKey_RevokedKey(t *testing.T) {
	mockRepo := newMockAPIKeyRepository()
	cfg := &config.Config{
		DeploymentMode: "hosted",
	}
	repos := &repository.Repositories{
		APIKey: mockRepo,
	}

	logger := slog.Default()
	svc := NewAuthService(cfg, repos, logger)

	// Create a revoked API key
	testKey := "rf_revoked_key_12345"
	keyHash := hashAPIKey(testKey)
	revokedAt := time.Now().Add(-time.Hour)
	mockRepo.Create(context.Background(), &models.APIKey{
		ID:        "key-2",
		UserID:    "user-456",
		Name:      "Revoked Key",
		KeyHash:   keyHash,
		KeyPrefix: "rf_revoke",
		Scopes:    []string{"*"},
		CreatedAt: time.Now().Add(-time.Hour * 24),
		RevokedAt: &revokedAt,
	})

	// Try to validate the revoked key
	claims, err := svc.ValidateAPIKey(context.Background(), testKey)
	if err == nil {
		t.Fatal("expected error for revoked key")
	}
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
	if claims != nil {
		t.Error("expected nil claims for revoked key")
	}
}

func TestValidateAPIKey_ExpiredKey(t *testing.T) {
	mockRepo := newMockAPIKeyRepository()
	cfg := &config.Config{
		DeploymentMode: "hosted",
	}
	repos := &repository.Repositories{
		APIKey: mockRepo,
	}

	logger := slog.Default()
	svc := NewAuthService(cfg, repos, logger)

	// Create an expired API key
	testKey := "rf_expired_key_12345"
	keyHash := hashAPIKey(testKey)
	expiredAt := time.Now().Add(-time.Hour)
	mockRepo.Create(context.Background(), &models.APIKey{
		ID:        "key-3",
		UserID:    "user-789",
		Name:      "Expired Key",
		KeyHash:   keyHash,
		KeyPrefix: "rf_expire",
		Scopes:    []string{"jobs:read"},
		CreatedAt: time.Now().Add(-time.Hour * 24),
		ExpiresAt: &expiredAt,
	})

	// Try to validate the expired key
	claims, err := svc.ValidateAPIKey(context.Background(), testKey)
	if err == nil {
		t.Fatal("expected error for expired key")
	}
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
	if claims != nil {
		t.Error("expected nil claims for expired key")
	}
}

func TestValidateAPIKey_NotYetExpired(t *testing.T) {
	mockRepo := newMockAPIKeyRepository()
	cfg := &config.Config{
		DeploymentMode: "hosted",
	}
	repos := &repository.Repositories{
		APIKey: mockRepo,
	}

	logger := slog.Default()
	svc := NewAuthService(cfg, repos, logger)

	// Create an API key that expires in the future
	testKey := "rf_future_key_12345"
	keyHash := hashAPIKey(testKey)
	expiresAt := time.Now().Add(time.Hour * 24) // Expires tomorrow
	mockRepo.Create(context.Background(), &models.APIKey{
		ID:        "key-4",
		UserID:    "user-future",
		Name:      "Future Key",
		KeyHash:   keyHash,
		KeyPrefix: "rf_futur",
		Scopes:    []string{"*"},
		CreatedAt: time.Now(),
		ExpiresAt: &expiresAt,
	})

	// Validate the key - should succeed
	claims, err := svc.ValidateAPIKey(context.Background(), testKey)
	if err != nil {
		t.Fatalf("expected no error for not-yet-expired key, got %v", err)
	}
	if claims == nil {
		t.Fatal("expected claims, got nil")
	}
	if claims.UserID != "user-future" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-future")
	}
}

func TestValidateAPIKey_SelfHostedMode(t *testing.T) {
	mockRepo := newMockAPIKeyRepository()
	cfg := &config.Config{
		DeploymentMode: "selfhosted",
	}
	repos := &repository.Repositories{
		APIKey: mockRepo,
	}

	logger := slog.Default()
	svc := NewAuthService(cfg, repos, logger)

	// Create a valid API key
	testKey := "rf_selfhost_key_12345"
	keyHash := hashAPIKey(testKey)
	mockRepo.Create(context.Background(), &models.APIKey{
		ID:        "key-5",
		UserID:    "user-selfhosted",
		Name:      "Self-hosted Key",
		KeyHash:   keyHash,
		KeyPrefix: "rf_selfh",
		Scopes:    []string{"*"},
		CreatedAt: time.Now(),
	})

	// Validate the key - should return "selfhosted" tier
	claims, err := svc.ValidateAPIKey(context.Background(), testKey)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if claims == nil {
		t.Fatal("expected claims, got nil")
	}
	if claims.Tier != "selfhosted" {
		t.Errorf("Tier = %q, want %q for self-hosted mode", claims.Tier, "selfhosted")
	}
}

func TestValidateAPIKey_UpdatesLastUsed(t *testing.T) {
	mockRepo := newMockAPIKeyRepository()
	cfg := &config.Config{
		DeploymentMode: "hosted",
	}
	repos := &repository.Repositories{
		APIKey: mockRepo,
	}

	logger := slog.Default()
	svc := NewAuthService(cfg, repos, logger)

	// Create a valid API key without LastUsedAt
	testKey := "rf_lastused_key_12345"
	keyHash := hashAPIKey(testKey)
	mockRepo.Create(context.Background(), &models.APIKey{
		ID:        "key-6",
		UserID:    "user-lastused",
		Name:      "Last Used Key",
		KeyHash:   keyHash,
		KeyPrefix: "rf_lastu",
		Scopes:    []string{"*"},
		CreatedAt: time.Now(),
	})

	// Validate the key
	_, err := svc.ValidateAPIKey(context.Background(), testKey)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Wait briefly for the goroutine to update LastUsed
	time.Sleep(50 * time.Millisecond)

	// Check that LastUsedAt was updated
	key, _ := mockRepo.GetByID(context.Background(), "key-6")
	if key == nil {
		t.Fatal("expected key, got nil")
	}
	if key.LastUsedAt == nil {
		t.Error("expected LastUsedAt to be updated")
	}
}

func TestValidateAPIKey_EmptyKey(t *testing.T) {
	mockRepo := newMockAPIKeyRepository()
	cfg := &config.Config{
		DeploymentMode: "hosted",
	}
	repos := &repository.Repositories{
		APIKey: mockRepo,
	}

	logger := slog.Default()
	svc := NewAuthService(cfg, repos, logger)

	// Try to validate an empty key
	claims, err := svc.ValidateAPIKey(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
	if claims != nil {
		t.Error("expected nil claims for empty key")
	}
}

// ========================================
// TokenClaims Tests
// ========================================

func TestTokenClaims_Fields(t *testing.T) {
	claims := &TokenClaims{
		UserID:           "user-123",
		Email:            "test@example.com",
		Tier:             "pro",
		GlobalSuperadmin: true,
		Scopes:           []string{"jobs:read", "jobs:write"},
	}

	if claims.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-123")
	}
	if claims.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", claims.Email, "test@example.com")
	}
	if claims.Tier != "pro" {
		t.Errorf("Tier = %q, want %q", claims.Tier, "pro")
	}
	if !claims.GlobalSuperadmin {
		t.Error("expected GlobalSuperadmin to be true")
	}
	if len(claims.Scopes) != 2 {
		t.Errorf("Scopes length = %d, want 2", len(claims.Scopes))
	}
}
