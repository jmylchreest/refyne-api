// Package routes provides shared route registration for the Refyne API.
// This allows both the main server and the OpenAPI generator to use
// the same route definitions, ensuring the spec is always in sync.
package routes

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/http/handlers"
)

// JobHandlers defines the interface for job-related operations.
type JobHandlers interface {
	ListJobs(ctx context.Context, input *handlers.ListJobsInput) (*handlers.ListJobsOutput, error)
	GetJob(ctx context.Context, input *handlers.GetJobInput) (*handlers.GetJobOutput, error)
	GetCrawlMap(ctx context.Context, input *handlers.GetCrawlMapInput) (*handlers.GetCrawlMapOutput, error)
	GetJobResultsDownload(ctx context.Context, input *handlers.GetJobResultsDownloadInput) (*handlers.GetJobResultsDownloadOutput, error)
	GetJobWebhookDeliveries(ctx context.Context, input *handlers.GetJobWebhookDeliveriesInput) (*handlers.GetJobWebhookDeliveriesOutput, error)
	GetJobDebugCapture(ctx context.Context, input *handlers.GetJobDebugCaptureInput) (*handlers.GetJobDebugCaptureOutput, error)
	// RegisterRawEndpoints registers SSE/multi-format endpoints for OpenAPI documentation.
	RegisterRawEndpoints(api huma.API)
}

// CrawlHandlers defines the interface for crawl job creation.
type CrawlHandlers interface {
	CreateCrawlJob(ctx context.Context, input *handlers.CreateCrawlJobInput) (*handlers.CreateCrawlJobOutput, error)
}

// UsageHandlers defines the interface for usage operations.
type UsageHandlers interface {
	GetUsage(ctx context.Context, input *handlers.GetUsageInput) (*handlers.GetUsageOutput, error)
}

// UserLLMHandlers defines the interface for user LLM provider operations.
type UserLLMHandlers interface {
	ListProviders(ctx context.Context, input *struct{}) (*handlers.ListProvidersOutput, error)
	ListModels(ctx context.Context, input *handlers.UserListModelsInput) (*handlers.UserListModelsOutput, error)
	ListServiceKeys(ctx context.Context, input *struct{}) (*handlers.ListUserServiceKeysOutput, error)
	UpsertServiceKey(ctx context.Context, input *handlers.UpsertUserServiceKeyInput) (*handlers.UpsertUserServiceKeyOutput, error)
	DeleteServiceKey(ctx context.Context, input *handlers.DeleteUserServiceKeyInput) (*handlers.DeleteUserServiceKeyOutput, error)
	GetFallbackChain(ctx context.Context, input *struct{}) (*handlers.GetUserFallbackChainOutput, error)
	SetFallbackChain(ctx context.Context, input *handlers.SetUserFallbackChainInput) (*handlers.SetUserFallbackChainOutput, error)
}

// APIKeyHandlers defines the interface for API key operations.
// This is only used in hosted mode.
type APIKeyHandlers interface {
	ListKeys(ctx context.Context, input *struct{}) (*handlers.ListKeysOutput, error)
	CreateKey(ctx context.Context, input *handlers.CreateKeyInput) (*handlers.CreateKeyOutput, error)
	RevokeKey(ctx context.Context, input *handlers.RevokeKeyInput) (*handlers.RevokeKeyOutput, error)
}

// SchemaCatalogHandlers defines the interface for schema catalog operations.
type SchemaCatalogHandlers interface {
	ListSchemas(ctx context.Context, input *handlers.ListSchemasInput) (*handlers.ListSchemasOutput, error)
	GetSchema(ctx context.Context, input *handlers.GetSchemaInput) (*handlers.GetSchemaOutput, error)
	CreateSchema(ctx context.Context, input *handlers.CreateSchemaInput) (*handlers.CreateSchemaOutput, error)
	UpdateSchema(ctx context.Context, input *handlers.UpdateSchemaInput) (*handlers.UpdateSchemaOutput, error)
	DeleteSchema(ctx context.Context, input *handlers.DeleteSchemaInput) (*handlers.DeleteSchemaOutput, error)
	// Admin only
	ListAllSchemas(ctx context.Context, input *struct{}) (*handlers.ListAllSchemasOutput, error)
	CreatePlatformSchema(ctx context.Context, input *handlers.CreatePlatformSchemaInput) (*handlers.CreateSchemaOutput, error)
}

// SavedSitesHandlers defines the interface for saved sites operations.
type SavedSitesHandlers interface {
	ListSavedSites(ctx context.Context, input *struct{}) (*handlers.ListSavedSitesOutput, error)
	GetSavedSite(ctx context.Context, input *handlers.GetSavedSiteInput) (*handlers.GetSavedSiteOutput, error)
	CreateSavedSite(ctx context.Context, input *handlers.CreateSavedSiteInput) (*handlers.CreateSavedSiteOutput, error)
	UpdateSavedSite(ctx context.Context, input *handlers.UpdateSavedSiteInput) (*handlers.UpdateSavedSiteOutput, error)
	DeleteSavedSite(ctx context.Context, input *handlers.DeleteSavedSiteInput) (*handlers.DeleteSavedSiteOutput, error)
}

// WebhookHandlers defines the interface for webhook operations.
type WebhookHandlers interface {
	ListWebhooks(ctx context.Context, input *struct{}) (*handlers.ListWebhooksOutput, error)
	GetWebhook(ctx context.Context, input *handlers.GetWebhookInput) (*handlers.GetWebhookOutput, error)
	CreateWebhook(ctx context.Context, input *handlers.CreateWebhookInput) (*handlers.CreateWebhookOutput, error)
	UpdateWebhook(ctx context.Context, input *handlers.UpdateWebhookInput) (*handlers.UpdateWebhookOutput, error)
	DeleteWebhook(ctx context.Context, input *handlers.DeleteWebhookInput) (*handlers.DeleteWebhookOutput, error)
	ListWebhookDeliveries(ctx context.Context, input *handlers.ListWebhookDeliveriesInput) (*handlers.ListWebhookDeliveriesOutput, error)
}

// AnalyzeHandlers defines the interface for URL analysis operations.
type AnalyzeHandlers interface {
	Analyze(ctx context.Context, input *handlers.AnalyzeInput) (*handlers.AnalyzeOutput, error)
}

// ExtractionHandlers defines the interface for extraction operations.
type ExtractionHandlers interface {
	Extract(ctx context.Context, input *handlers.ExtractInput) (*handlers.ExtractOutput, error)
}

// AdminHandlers defines the interface for admin operations.
// These endpoints are hidden from public OpenAPI documentation.
type AdminHandlers interface {
	ListServiceKeys(ctx context.Context, input *struct{}) (*handlers.ListServiceKeysOutput, error)
	UpsertServiceKey(ctx context.Context, input *handlers.UpsertServiceKeyInput) (*handlers.UpsertServiceKeyOutput, error)
	DeleteServiceKey(ctx context.Context, input *handlers.DeleteServiceKeyInput) (*handlers.DeleteServiceKeyOutput, error)
	GetFallbackChain(ctx context.Context, input *handlers.GetFallbackChainInput) (*handlers.GetFallbackChainOutput, error)
	SetFallbackChain(ctx context.Context, input *handlers.SetFallbackChainInput) (*handlers.SetFallbackChainOutput, error)
	ListModels(ctx context.Context, input *handlers.ListModelsInput) (*handlers.ListModelsOutput, error)
	ValidateModels(ctx context.Context, input *handlers.ValidateModelsInput) (*handlers.ValidateModelsOutput, error)
	ListTiers(ctx context.Context, input *struct{}) (*handlers.ListTiersOutput, error)
	ValidateTiers(ctx context.Context, input *handlers.ValidateTiersInput) (*handlers.ValidateTiersOutput, error)
	SyncTiers(ctx context.Context, input *struct{}) (*handlers.SyncTiersOutput, error)
}

// AdminAnalyticsHandlers defines the interface for admin analytics operations.
type AdminAnalyticsHandlers interface {
	GetOverview(ctx context.Context, input *handlers.OverviewInput) (*handlers.GetOverviewOutput, error)
	GetJobs(ctx context.Context, input *handlers.AnalyticsJobsInput) (*handlers.GetAnalyticsJobsOutput, error)
	GetErrors(ctx context.Context, input *handlers.ErrorsInput) (*handlers.GetErrorsOutput, error)
	GetTrends(ctx context.Context, input *handlers.TrendsInput) (*handlers.GetTrendsOutput, error)
	GetUsers(ctx context.Context, input *handlers.AnalyticsUsersInput) (*handlers.GetAnalyticsUsersOutput, error)
	GetJobResults(ctx context.Context, input *handlers.AdminJobResultsInput) (*handlers.AdminJobResultsOutput, error)
}

// Handlers aggregates all handler interfaces for route registration.
// For the main server, pass real handler implementations.
// For OpenAPI generation, pass stub implementations.
type Handlers struct {
	// Public endpoints
	HealthCheck    func(ctx context.Context, input *struct{}) (*handlers.HealthCheckOutput, error)
	ListTierLimits func(ctx context.Context, input *struct{}) (*handlers.ListTierLimitsOutput, error)

	// Kubernetes probes (hidden from docs)
	Livez  func(ctx context.Context, input *struct{}) (*handlers.LivezOutput, error)
	Readyz func(ctx context.Context, input *struct{}) (*handlers.ReadyzOutput, error)

	// Protected endpoint handlers
	Job            JobHandlers
	Crawl          CrawlHandlers
	Usage          UsageHandlers
	UserLLM        UserLLMHandlers
	APIKey         APIKeyHandlers // May be nil in self-hosted mode
	SchemaCatalog  SchemaCatalogHandlers
	SavedSites     SavedSitesHandlers
	Webhook        WebhookHandlers
	Analyze        AnalyzeHandlers
	Extraction     ExtractionHandlers
	Admin          AdminHandlers
	AdminAnalytics AdminAnalyticsHandlers
}

// IncludeAPIKeys returns true if API key endpoints should be registered.
// In self-hosted mode, API keys are not used.
func (h *Handlers) IncludeAPIKeys() bool {
	return h.APIKey != nil
}
