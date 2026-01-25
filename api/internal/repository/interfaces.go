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
	// ClaimPendingWithLimits claims a pending job with priority scheduling and per-user limits
	ClaimPendingWithLimits(ctx context.Context, tierLimits TierJobLimits) (*models.Job, error)
	// DeleteOlderThan deletes jobs older than the specified time and returns the deleted job IDs
	DeleteOlderThan(ctx context.Context, before time.Time) ([]string, error)
	// MarkStaleRunningJobsFailed marks jobs that have been running longer than maxAge as failed
	// Returns the number of jobs marked as failed
	MarkStaleRunningJobsFailed(ctx context.Context, maxAge time.Duration) (int64, error)
	// CountActiveByUserID counts jobs that are pending or running for a user
	CountActiveByUserID(ctx context.Context, userID string) (int, error)
}

// JobResultRepository defines methods for job result data access.
type JobResultRepository interface {
	Create(ctx context.Context, result *models.JobResult) error
	GetByJobID(ctx context.Context, jobID string) ([]*models.JobResult, error)
	// GetAfterID returns results with ID greater than afterID (works with ULIDs which are time-ordered).
	// Pass empty string to get all results.
	GetAfterID(ctx context.Context, jobID, afterID string) ([]*models.JobResult, error)
	// GetCrawlMap returns results ordered by depth for crawl map visualization
	GetCrawlMap(ctx context.Context, jobID string) ([]*models.JobResult, error)
	// DeleteByJobIDs deletes all results for the specified job IDs
	DeleteByJobIDs(ctx context.Context, jobIDs []string) error
	// CountByJobID returns the total number of results (all statuses) for a job
	CountByJobID(ctx context.Context, jobID string) (int, error)
}

// UsageRepository defines methods for usage data access (lean billing table).
type UsageRepository interface {
	Create(ctx context.Context, record *models.UsageRecord) error
	GetByUserID(ctx context.Context, userID string, startDate, endDate string) ([]*models.UsageRecord, error)
	GetSummary(ctx context.Context, userID string, period string) (*UsageSummary, error)
	// GetSummaryByDateRange returns usage summary for a specific date range (used for subscription periods)
	GetSummaryByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) (*UsageSummary, error)
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
	// UpdateSubscriptionPeriod updates the subscription billing period dates
	UpdateSubscriptionPeriod(ctx context.Context, userID string, periodStart, periodEnd time.Time) error
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

// SchemaCatalogRepository defines methods for schema catalog data access.
type SchemaCatalogRepository interface {
	Create(ctx context.Context, schema *models.SchemaCatalog) error
	GetByID(ctx context.Context, id string) (*models.SchemaCatalog, error)
	Update(ctx context.Context, schema *models.SchemaCatalog) error
	Delete(ctx context.Context, id string) error
	// ListForUser returns schemas visible to a user (their own + platform + optionally public)
	ListForUser(ctx context.Context, userID string, orgID *string, includePublic bool) ([]*models.SchemaCatalog, error)
	// ListPlatform returns all platform schemas
	ListPlatform(ctx context.Context) ([]*models.SchemaCatalog, error)
	// ListByCategory returns schemas in a category
	ListByCategory(ctx context.Context, category string) ([]*models.SchemaCatalog, error)
	// ListAll returns all schemas (for admin)
	ListAll(ctx context.Context) ([]*models.SchemaCatalog, error)
	// IncrementUsage increments the usage count for a schema
	IncrementUsage(ctx context.Context, id string) error
}

// SavedSitesRepository defines methods for saved sites data access.
type SavedSitesRepository interface {
	Create(ctx context.Context, site *models.SavedSite) error
	GetByID(ctx context.Context, id string) (*models.SavedSite, error)
	Update(ctx context.Context, site *models.SavedSite) error
	Delete(ctx context.Context, id string) error
	// ListByUserID returns saved sites for a user
	ListByUserID(ctx context.Context, userID string) ([]*models.SavedSite, error)
	// ListByOrganizationID returns saved sites for an organization
	ListByOrganizationID(ctx context.Context, orgID string) ([]*models.SavedSite, error)
	// ListByDomain returns saved sites for a domain
	ListByDomain(ctx context.Context, userID, domain string) ([]*models.SavedSite, error)
}

// UserServiceKeyRepository defines methods for user-configured LLM provider keys.
// These allow users to use their own API keys for LLM providers.
type UserServiceKeyRepository interface {
	Upsert(ctx context.Context, key *models.UserServiceKey) error
	GetByID(ctx context.Context, id string) (*models.UserServiceKey, error)
	GetByUserID(ctx context.Context, userID string) ([]*models.UserServiceKey, error)
	GetByUserAndProvider(ctx context.Context, userID, provider string) (*models.UserServiceKey, error)
	GetEnabledByUserID(ctx context.Context, userID string) ([]*models.UserServiceKey, error)
	Delete(ctx context.Context, id string) error
}

// UserFallbackChainRepository defines methods for user-configured LLM fallback chains.
// Allows users to define their own provider/model order for extractions.
type UserFallbackChainRepository interface {
	// GetByUserID returns all entries for a user
	GetByUserID(ctx context.Context, userID string) ([]*models.UserFallbackChainEntry, error)
	// GetEnabledByUserID returns enabled entries for a user in position order
	GetEnabledByUserID(ctx context.Context, userID string) ([]*models.UserFallbackChainEntry, error)
	// ReplaceAll replaces all entries for a user
	ReplaceAll(ctx context.Context, userID string, entries []*models.UserFallbackChainEntry) error
}

// WebhookRepository defines methods for webhook data access.
// Webhooks allow users to receive notifications when job events occur.
type WebhookRepository interface {
	Create(ctx context.Context, webhook *models.Webhook) error
	GetByID(ctx context.Context, id string) (*models.Webhook, error)
	GetByUserID(ctx context.Context, userID string) ([]*models.Webhook, error)
	GetActiveByUserID(ctx context.Context, userID string) ([]*models.Webhook, error)
	GetByUserAndName(ctx context.Context, userID, name string) (*models.Webhook, error)
	Update(ctx context.Context, webhook *models.Webhook) error
	Delete(ctx context.Context, id string) error
}

// WebhookDeliveryRepository defines methods for webhook delivery tracking.
// Tracks all webhook delivery attempts including successes, failures, and retries.
type WebhookDeliveryRepository interface {
	Create(ctx context.Context, delivery *models.WebhookDelivery) error
	Update(ctx context.Context, delivery *models.WebhookDelivery) error
	GetByID(ctx context.Context, id string) (*models.WebhookDelivery, error)
	GetByJobID(ctx context.Context, jobID string) ([]*models.WebhookDelivery, error)
	GetByWebhookID(ctx context.Context, webhookID string, limit, offset int) ([]*models.WebhookDelivery, error)
	GetPendingRetries(ctx context.Context, limit int) ([]*models.WebhookDelivery, error)
	DeleteByJobIDs(ctx context.Context, jobIDs []string) error
}

// Repositories holds all repository instances.
type Repositories struct {
	APIKey            APIKeyRepository
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
	SchemaCatalog     SchemaCatalogRepository
	SavedSites        SavedSitesRepository
	UserServiceKey    UserServiceKeyRepository
	UserFallbackChain UserFallbackChainRepository
	Webhook           WebhookRepository
	WebhookDelivery   WebhookDeliveryRepository
	RateLimit         RateLimitRepository
	Analytics         *SQLiteAnalyticsRepository
}

// NewRepositories creates all repository instances.
func NewRepositories(db *sql.DB) *Repositories {
	return &Repositories{
		APIKey:            NewSQLiteAPIKeyRepository(db),
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
		SchemaCatalog:     NewSQLiteSchemaCatalogRepository(db),
		SavedSites:        NewSQLiteSavedSitesRepository(db),
		UserServiceKey:    NewSQLiteUserServiceKeyRepository(db),
		UserFallbackChain: NewSQLiteUserFallbackChainRepository(db),
		Webhook:           NewSQLiteWebhookRepository(db),
		WebhookDelivery:   NewSQLiteWebhookDeliveryRepository(db),
		RateLimit:         NewSQLiteRateLimitRepository(db),
		Analytics:         NewSQLiteAnalyticsRepository(db),
	}
}
