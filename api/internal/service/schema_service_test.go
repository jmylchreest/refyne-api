package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// ========================================
// SchemaService Tests
// ========================================

// schemaSnapshotRepository is a mock implementation for testing.
type schemaSnapshotRepository struct {
	snapshots        map[string]*models.SchemaSnapshot
	byUserHash       map[string]*models.SchemaSnapshot // key: userID:hash
	createErr        error
	getByIDErr       error
	getByUserHashErr error
	getByUserIDErr   error
	getNextVerErr    error
	incrementErr     error
	nextVersion      int
}

func newSchemaSnapshotRepository() *schemaSnapshotRepository {
	return &schemaSnapshotRepository{
		snapshots:   make(map[string]*models.SchemaSnapshot),
		byUserHash:  make(map[string]*models.SchemaSnapshot),
		nextVersion: 1,
	}
}

func (r *schemaSnapshotRepository) Create(ctx context.Context, snapshot *models.SchemaSnapshot) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.snapshots[snapshot.ID] = snapshot
	key := snapshot.UserID + ":" + snapshot.Hash
	r.byUserHash[key] = snapshot
	return nil
}

func (r *schemaSnapshotRepository) GetByID(ctx context.Context, id string) (*models.SchemaSnapshot, error) {
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return r.snapshots[id], nil
}

func (r *schemaSnapshotRepository) GetByUserAndHash(ctx context.Context, userID, hash string) (*models.SchemaSnapshot, error) {
	if r.getByUserHashErr != nil {
		return nil, r.getByUserHashErr
	}
	key := userID + ":" + hash
	return r.byUserHash[key], nil
}

func (r *schemaSnapshotRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.SchemaSnapshot, error) {
	if r.getByUserIDErr != nil {
		return nil, r.getByUserIDErr
	}
	var results []*models.SchemaSnapshot
	for _, s := range r.snapshots {
		if s.UserID == userID {
			results = append(results, s)
		}
	}
	// Apply simple pagination
	if offset >= len(results) {
		return []*models.SchemaSnapshot{}, nil
	}
	end := offset + limit
	if end > len(results) || limit == 0 {
		end = len(results)
	}
	return results[offset:end], nil
}

func (r *schemaSnapshotRepository) GetNextVersion(ctx context.Context, userID string) (int, error) {
	if r.getNextVerErr != nil {
		return 0, r.getNextVerErr
	}
	v := r.nextVersion
	r.nextVersion++
	return v, nil
}

func (r *schemaSnapshotRepository) IncrementUsageCount(ctx context.Context, id string) error {
	if r.incrementErr != nil {
		return r.incrementErr
	}
	if s, ok := r.snapshots[id]; ok {
		s.UsageCount++
	}
	return nil
}

// setupSchemaService creates a SchemaService with mock repositories for testing.
func setupSchemaService(mockRepo *schemaSnapshotRepository) *SchemaService {
	repos := &repository.Repositories{
		SchemaSnapshot: mockRepo,
	}
	logger := slog.Default()
	return NewSchemaService(repos, logger)
}

// ----------------------------------------
// Constructor Tests
// ----------------------------------------

func TestNewSchemaService(t *testing.T) {
	logger := slog.Default()
	repos := &repository.Repositories{}

	svc := NewSchemaService(repos, logger)
	if svc == nil {
		t.Fatal("expected service, got nil")
	}
	if svc.repos != repos {
		t.Error("repos not set correctly")
	}
	if svc.logger != logger {
		t.Error("logger not set correctly")
	}
}

// ----------------------------------------
// hashSchema Tests
// ----------------------------------------

func TestHashSchema(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "simple JSON",
			input: `{"type":"object"}`,
		},
		{
			name:  "complex schema",
			input: `{"type":"object","properties":{"name":{"type":"string"},"price":{"type":"number"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashSchema(tt.input)

			// Verify it's a valid SHA256 hex string (64 chars)
			if len(result) != 64 {
				t.Errorf("hash length = %d, want 64", len(result))
			}

			// Verify it's deterministic
			result2 := hashSchema(tt.input)
			if result != result2 {
				t.Error("hash should be deterministic")
			}

			// Verify against manual computation
			h := sha256.New()
			h.Write([]byte(tt.input))
			expected := hex.EncodeToString(h.Sum(nil))
			if result != expected {
				t.Errorf("hash = %q, want %q", result, expected)
			}
		})
	}
}

func TestHashSchema_DifferentInputs(t *testing.T) {
	hash1 := hashSchema(`{"type":"string"}`)
	hash2 := hashSchema(`{"type":"number"}`)

	if hash1 == hash2 {
		t.Error("different inputs should produce different hashes")
	}
}

// ----------------------------------------
// GetOrCreateSnapshot Tests
// ----------------------------------------

func TestSchemaService_GetOrCreateSnapshot_NewSchema(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	userID := "user-1"
	schemaJSON := `{"type":"object","properties":{"title":{"type":"string"}}}`
	name := "Test Schema"

	snapshot, err := svc.GetOrCreateSnapshot(ctx, userID, schemaJSON, name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snapshot == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if snapshot.ID == "" {
		t.Error("expected ID to be generated")
	}
	if snapshot.UserID != userID {
		t.Errorf("UserID = %q, want %q", snapshot.UserID, userID)
	}
	if snapshot.SchemaJSON != schemaJSON {
		t.Errorf("SchemaJSON mismatch")
	}
	if snapshot.Name != name {
		t.Errorf("Name = %q, want %q", snapshot.Name, name)
	}
	if snapshot.Version != 1 {
		t.Errorf("Version = %d, want 1", snapshot.Version)
	}
	if snapshot.UsageCount != 1 {
		t.Errorf("UsageCount = %d, want 1", snapshot.UsageCount)
	}

	expectedHash := hashSchema(schemaJSON)
	if snapshot.Hash != expectedHash {
		t.Errorf("Hash = %q, want %q", snapshot.Hash, expectedHash)
	}
}

func TestSchemaService_GetOrCreateSnapshot_ExistingSchema(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	userID := "user-1"
	schemaJSON := `{"type":"object"}`
	name := "Test Schema"

	// Create first snapshot
	snapshot1, err := svc.GetOrCreateSnapshot(ctx, userID, schemaJSON, name)
	if err != nil {
		t.Fatalf("unexpected error creating first snapshot: %v", err)
	}

	// Request same schema again - should return existing
	snapshot2, err := svc.GetOrCreateSnapshot(ctx, userID, schemaJSON, "Different Name")
	if err != nil {
		t.Fatalf("unexpected error creating second snapshot: %v", err)
	}

	if snapshot2.ID != snapshot1.ID {
		t.Errorf("expected same snapshot ID, got %q vs %q", snapshot2.ID, snapshot1.ID)
	}

	// Usage count should have been incremented
	if snapshot2.UsageCount != 2 {
		t.Errorf("UsageCount = %d, want 2", snapshot2.UsageCount)
	}
}

func TestSchemaService_GetOrCreateSnapshot_DifferentUsers(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	schemaJSON := `{"type":"object"}`

	// Same schema, different users
	snapshot1, err := svc.GetOrCreateSnapshot(ctx, "user-1", schemaJSON, "Schema 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snapshot2, err := svc.GetOrCreateSnapshot(ctx, "user-2", schemaJSON, "Schema 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snapshot1.ID == snapshot2.ID {
		t.Error("different users should create different snapshots")
	}
}

func TestSchemaService_GetOrCreateSnapshot_GetNextVersionError(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	mockRepo.getNextVerErr = errors.New("version error")
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	_, err := svc.GetOrCreateSnapshot(ctx, "user-1", `{"type":"object"}`, "Test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, mockRepo.getNextVerErr) && err.Error() != "failed to get next version: version error" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSchemaService_GetOrCreateSnapshot_CreateError(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	mockRepo.createErr = errors.New("create error")
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	_, err := svc.GetOrCreateSnapshot(ctx, "user-1", `{"type":"object"}`, "Test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, mockRepo.createErr) && err.Error() != "failed to create schema snapshot: create error" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSchemaService_GetOrCreateSnapshot_IncrementErrorNonFatal(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	userID := "user-1"
	schemaJSON := `{"type":"object"}`

	// Create first snapshot
	_, err := svc.GetOrCreateSnapshot(ctx, userID, schemaJSON, "Test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Set increment error
	mockRepo.incrementErr = errors.New("increment error")

	// Should still succeed even with increment error
	snapshot, err := svc.GetOrCreateSnapshot(ctx, userID, schemaJSON, "Test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected snapshot despite increment error")
	}
}

func TestSchemaService_GetOrCreateSnapshot_VersionIncrement(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	userID := "user-1"

	// Create multiple different schemas for same user
	snapshot1, _ := svc.GetOrCreateSnapshot(ctx, userID, `{"type":"string"}`, "Schema 1")
	snapshot2, _ := svc.GetOrCreateSnapshot(ctx, userID, `{"type":"number"}`, "Schema 2")
	snapshot3, _ := svc.GetOrCreateSnapshot(ctx, userID, `{"type":"boolean"}`, "Schema 3")

	if snapshot1.Version != 1 {
		t.Errorf("snapshot1 version = %d, want 1", snapshot1.Version)
	}
	if snapshot2.Version != 2 {
		t.Errorf("snapshot2 version = %d, want 2", snapshot2.Version)
	}
	if snapshot3.Version != 3 {
		t.Errorf("snapshot3 version = %d, want 3", snapshot3.Version)
	}
}

// ----------------------------------------
// GetByID Tests
// ----------------------------------------

func TestSchemaService_GetByID(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	// Create a snapshot
	snapshot, _ := svc.GetOrCreateSnapshot(ctx, "user-1", `{"type":"object"}`, "Test")

	// Retrieve by ID
	retrieved, err := svc.GetByID(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if retrieved.ID != snapshot.ID {
		t.Errorf("ID = %q, want %q", retrieved.ID, snapshot.ID)
	}
}

func TestSchemaService_GetByID_NotFound(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	retrieved, err := svc.GetByID(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil for nonexistent ID")
	}
}

func TestSchemaService_GetByID_Error(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	mockRepo.getByIDErr = errors.New("database error")
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	_, err := svc.GetByID(ctx, "any-id")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ----------------------------------------
// GetByUserID Tests
// ----------------------------------------

func TestSchemaService_GetByUserID(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	// Create snapshots for user-1
	svc.GetOrCreateSnapshot(ctx, "user-1", `{"type":"string"}`, "Schema 1")
	svc.GetOrCreateSnapshot(ctx, "user-1", `{"type":"number"}`, "Schema 2")

	// Create snapshot for user-2
	svc.GetOrCreateSnapshot(ctx, "user-2", `{"type":"boolean"}`, "Schema 3")

	// Get user-1's snapshots
	snapshots, err := svc.GetByUserID(ctx, "user-1", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(snapshots))
	}

	for _, s := range snapshots {
		if s.UserID != "user-1" {
			t.Errorf("got snapshot for wrong user: %s", s.UserID)
		}
	}
}

func TestSchemaService_GetByUserID_Empty(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	snapshots, err := svc.GetByUserID(ctx, "nonexistent-user", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshots) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(snapshots))
	}
}

func TestSchemaService_GetByUserID_Error(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	mockRepo.getByUserIDErr = errors.New("database error")
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	_, err := svc.GetByUserID(ctx, "user-1", 10, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSchemaService_GetByUserID_Pagination(t *testing.T) {
	mockRepo := newSchemaSnapshotRepository()
	svc := setupSchemaService(mockRepo)
	ctx := context.Background()

	// Create 5 snapshots
	for i := 0; i < 5; i++ {
		schema := `{"type":"object","index":` + string(rune('0'+i)) + `}`
		svc.GetOrCreateSnapshot(ctx, "user-1", schema, "Schema")
	}

	// Get with limit
	snapshots, err := svc.GetByUserID(ctx, "user-1", 2, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshots) != 2 {
		t.Errorf("expected 2 snapshots with limit, got %d", len(snapshots))
	}

	// Get with offset beyond count
	snapshots, err = svc.GetByUserID(ctx, "user-1", 10, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshots) != 0 {
		t.Errorf("expected 0 snapshots with high offset, got %d", len(snapshots))
	}
}
