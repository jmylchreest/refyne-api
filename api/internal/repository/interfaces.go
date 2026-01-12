// Package repository defines repository interfaces for data access.
// Note: User management, OAuth, sessions, subscriptions, and credits are handled by Clerk.
package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// APIKeyRepository defines methods for API key data access.
type APIKeyRepository interface {
	Create(ctx context.Context, key *models.APIKey) error
	GetByID(ctx context.Context, id string) (*models.APIKey, error)
	GetByKeyHash(ctx context.Context, hash string) (*models.APIKey, error)
	GetByUserID(ctx context.Context, userID string) ([]*models.APIKey, error)
	UpdateLastUsed(ctx context.Context, id string, lastUsed time.Time) error
	Revoke(ctx context.Context, id string) error
}

// LLMConfigRepository defines methods for LLM config data access.
type LLMConfigRepository interface {
	GetByUserID(ctx context.Context, userID string) (*models.LLMConfig, error)
	Upsert(ctx context.Context, config *models.LLMConfig) error
}

// JobRepository defines methods for job data access.
type JobRepository interface {
	Create(ctx context.Context, job *models.Job) error
	GetByID(ctx context.Context, id string) (*models.Job, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.Job, error)
	Update(ctx context.Context, job *models.Job) error
	GetPending(ctx context.Context, limit int) ([]*models.Job, error)
	ClaimJob(ctx context.Context, id string) (*models.Job, error)
	// ClaimPending atomically claims the next pending job and returns it
	ClaimPending(ctx context.Context) (*models.Job, error)
}

// JobResultRepository defines methods for job result data access.
type JobResultRepository interface {
	Create(ctx context.Context, result *models.JobResult) error
	GetByJobID(ctx context.Context, jobID string) ([]*models.JobResult, error)
	GetAfter(ctx context.Context, jobID, afterID string) ([]*models.JobResult, error)
}

// UsageRepository defines methods for usage data access (lean billing table).
type UsageRepository interface {
	Create(ctx context.Context, record *models.UsageRecord) error
	GetByUserID(ctx context.Context, userID string, startDate, endDate string) ([]*models.UsageRecord, error)
	GetSummary(ctx context.Context, userID string, period string) (*UsageSummary, error)
	GetMonthlySpend(ctx context.Context, userID string, month time.Time) (float64, error)
	CountByUserAndDateRange(ctx context.Context, userID string, startDate, endDate string) (int, error)
}

// UsageSummary represents aggregated usage data.
type UsageSummary struct {
	TotalJobs       int     `json:"total_jobs"`
	TotalChargedUSD float64 `json:"total_charged_usd"`
	BYOKJobs        int     `json:"byok_jobs"`
}

// UsageInsightRepository defines methods for usage insight data access (rich analytics table).
type UsageInsightRepository interface {
	Create(ctx context.Context, insight *models.UsageInsight) error
	GetByUsageID(ctx context.Context, usageID string) (*models.UsageInsight, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.UsageInsight, error)
}

// BalanceRepository defines methods for user balance data access.
type BalanceRepository interface {
	Get(ctx context.Context, userID string) (*models.UserBalance, error)
	Upsert(ctx context.Context, balance *models.UserBalance) error
	// GetAvailableBalance returns balance considering expired credits
	GetAvailableBalance(ctx context.Context, userID string, now time.Time) (float64, error)
}

// CreditTransactionRepository defines methods for credit transaction data access.
type CreditTransactionRepository interface {
	Create(ctx context.Context, tx *models.CreditTransaction) error
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.CreditTransaction, error)
	GetByStripePaymentID(ctx context.Context, stripePaymentID string) (*models.CreditTransaction, error)
	// GetNonExpiredSubscriptionCredits returns credits that haven't expired for expiry recalculation
	GetNonExpiredSubscriptionCredits(ctx context.Context) ([]*models.CreditTransaction, error)
	// UpdateExpiry updates the expiry date for a credit transaction
	UpdateExpiry(ctx context.Context, id string, expiresAt *time.Time) error
	// MarkExpired marks all expired credits as expired
	MarkExpired(ctx context.Context, now time.Time) (int64, error)
	// GetAvailableBalance calculates available balance from non-expired credits
	GetAvailableBalance(ctx context.Context, userID string, now time.Time) (float64, error)
}

// SchemaSnapshotRepository defines methods for schema snapshot data access.
type SchemaSnapshotRepository interface {
	Create(ctx context.Context, snapshot *models.SchemaSnapshot) error
	GetByID(ctx context.Context, id string) (*models.SchemaSnapshot, error)
	GetByUserAndHash(ctx context.Context, userID, hash string) (*models.SchemaSnapshot, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.SchemaSnapshot, error)
	GetNextVersion(ctx context.Context, userID string) (int, error)
	IncrementUsageCount(ctx context.Context, id string) error
}

// TelemetryRepository defines methods for telemetry data access.
type TelemetryRepository interface {
	Create(ctx context.Context, event *models.TelemetryEvent) error
	GetByType(ctx context.Context, eventType string, startTime, endTime time.Time, limit, offset int) ([]*models.TelemetryEvent, error)
	CountByTypeAndPeriod(ctx context.Context, eventType, period string) (int, error)
	CountUniqueUsersByPeriod(ctx context.Context, period string) (int, error)
}

// LicenseRepository defines methods for license data access.
type LicenseRepository interface {
	Create(ctx context.Context, license *models.License) error
	GetByKey(ctx context.Context, key string) (*models.License, error)
	Update(ctx context.Context, license *models.License) error
	List(ctx context.Context, limit, offset int) ([]*models.License, error)
}

// ServiceKeyRepository defines methods for service key data access.
// Service keys are admin-configured LLM API keys used for free tier users.
type ServiceKeyRepository interface {
	Upsert(ctx context.Context, key *models.ServiceKey) error
	GetByProvider(ctx context.Context, provider string) (*models.ServiceKey, error)
	GetAll(ctx context.Context) ([]*models.ServiceKey, error)
	GetEnabled(ctx context.Context) ([]*models.ServiceKey, error)
	Delete(ctx context.Context, provider string) error
}

// FallbackChainRepository defines methods for LLM fallback chain configuration.
// The chain defines the order in which providers/models are tried during extraction.
// Supports tier-specific chains (e.g., free, pro, enterprise) with fallback to default.
type FallbackChainRepository interface {
	// GetAll returns all entries across all tiers
	GetAll(ctx context.Context) ([]*models.FallbackChainEntry, error)
	// GetByTier returns entries for a specific tier (nil for default chain)
	GetByTier(ctx context.Context, tier *string) ([]*models.FallbackChainEntry, error)
	// GetEnabled returns enabled entries from the default chain
	GetEnabled(ctx context.Context) ([]*models.FallbackChainEntry, error)
	// GetEnabledByTier returns enabled entries for a tier, falling back to default if none exist
	GetEnabledByTier(ctx context.Context, tier string) ([]*models.FallbackChainEntry, error)
	// GetAllTiers returns list of tiers with custom chains
	GetAllTiers(ctx context.Context) ([]string, error)
	// ReplaceAll replaces all entries in the default chain
	ReplaceAll(ctx context.Context, entries []*models.FallbackChainEntry) error
	// ReplaceAllByTier replaces all entries for a specific tier
	ReplaceAllByTier(ctx context.Context, tier *string, entries []*models.FallbackChainEntry) error
	// DeleteByTier removes all entries for a specific tier
	DeleteByTier(ctx context.Context, tier string) error
	Create(ctx context.Context, entry *models.FallbackChainEntry) error
	Update(ctx context.Context, entry *models.FallbackChainEntry) error
	Delete(ctx context.Context, id string) error
	Reorder(ctx context.Context, ids []string) error
}

// Repositories holds all repository instances.
type Repositories struct {
	APIKey            APIKeyRepository
	LLMConfig         LLMConfigRepository
	Job               JobRepository
	JobResult         JobResultRepository
	Usage             UsageRepository
	UsageInsight      UsageInsightRepository
	Balance           BalanceRepository
	CreditTransaction CreditTransactionRepository
	SchemaSnapshot    SchemaSnapshotRepository
	Telemetry         TelemetryRepository
	License           LicenseRepository
	ServiceKey        ServiceKeyRepository
	FallbackChain     FallbackChainRepository
}

// NewRepositories creates all repository instances.
func NewRepositories(db *sql.DB) *Repositories {
	return &Repositories{
		APIKey:            NewSQLiteAPIKeyRepository(db),
		LLMConfig:         NewSQLiteLLMConfigRepository(db),
		Job:               NewSQLiteJobRepository(db),
		JobResult:         NewSQLiteJobResultRepository(db),
		Usage:             NewSQLiteUsageRepository(db),
		UsageInsight:      NewSQLiteUsageInsightRepository(db),
		Balance:           NewSQLiteBalanceRepository(db),
		CreditTransaction: NewSQLiteCreditTransactionRepository(db),
		SchemaSnapshot:    NewSQLiteSchemaSnapshotRepository(db),
		Telemetry:         NewSQLiteTelemetryRepository(db),
		License:           NewSQLiteLicenseRepository(db),
		ServiceKey:        NewSQLiteServiceKeyRepository(db),
		FallbackChain:     NewSQLiteFallbackChainRepository(db),
	}
}
