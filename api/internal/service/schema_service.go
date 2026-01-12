package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// SchemaService handles schema snapshot operations.
type SchemaService struct {
	repos  *repository.Repositories
	logger *slog.Logger
}

// NewSchemaService creates a new schema service.
func NewSchemaService(repos *repository.Repositories, logger *slog.Logger) *SchemaService {
	return &SchemaService{
		repos:  repos,
		logger: logger,
	}
}

// GetOrCreateSnapshot retrieves an existing schema snapshot by hash or creates a new one.
// This provides deduplication - if the same schema is used again, we return the existing snapshot.
func (s *SchemaService) GetOrCreateSnapshot(ctx context.Context, userID, schemaJSON string, name string) (*models.SchemaSnapshot, error) {
	// Calculate hash for deduplication
	hash := hashSchema(schemaJSON)

	// Check if schema already exists for this user
	existing, err := s.repos.SchemaSnapshot.GetByUserAndHash(ctx, userID, hash)
	if err == nil && existing != nil {
		// Schema exists - increment usage count and return
		if err := s.repos.SchemaSnapshot.IncrementUsageCount(ctx, existing.ID); err != nil {
			s.logger.Warn("failed to increment schema usage count", "id", existing.ID, "error", err)
		}
		return existing, nil
	}

	// Create new snapshot
	version, err := s.repos.SchemaSnapshot.GetNextVersion(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get next version: %w", err)
	}

	snapshot := &models.SchemaSnapshot{
		ID:         ulid.Make().String(),
		UserID:     userID,
		Hash:       hash,
		SchemaJSON: schemaJSON,
		Name:       name,
		Version:    version,
		UsageCount: 1,
		CreatedAt:  time.Now(),
	}

	if err := s.repos.SchemaSnapshot.Create(ctx, snapshot); err != nil {
		return nil, fmt.Errorf("failed to create schema snapshot: %w", err)
	}

	s.logger.Info("created schema snapshot",
		"id", snapshot.ID,
		"user_id", userID,
		"version", version,
	)

	return snapshot, nil
}

// GetByID retrieves a schema snapshot by ID.
func (s *SchemaService) GetByID(ctx context.Context, id string) (*models.SchemaSnapshot, error) {
	return s.repos.SchemaSnapshot.GetByID(ctx, id)
}

// GetByUserID retrieves all schema snapshots for a user.
func (s *SchemaService) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.SchemaSnapshot, error) {
	return s.repos.SchemaSnapshot.GetByUserID(ctx, userID, limit, offset)
}

// hashSchema computes a SHA256 hash of the schema JSON.
func hashSchema(schemaJSON string) string {
	h := sha256.New()
	h.Write([]byte(schemaJSON))
	return hex.EncodeToString(h.Sum(nil))
}
