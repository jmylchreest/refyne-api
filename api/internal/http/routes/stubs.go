package routes

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/http/handlers"
)

// StubHandlers returns a Handlers instance with stub implementations.
// All handlers return nil responses - these are only used for OpenAPI generation
// where Huma extracts type information from function signatures.
func StubHandlers() *Handlers {
	return &Handlers{
		// Public endpoints
		HealthCheck:    stubHealthCheck,
		ListTierLimits: stubListTierLimits,
		ListCleaners:   stubListCleaners,

		// Kubernetes probes
		Livez:  stubLivez,
		Readyz: stubReadyz,

		// Protected endpoint handlers
		Job:            &stubJobHandlers{},
		Crawl:          &stubCrawlHandlers{},
		Usage:          &stubUsageHandlers{},
		UserLLM:        &stubUserLLMHandlers{},
		APIKey:         &stubAPIKeyHandlers{},
		SchemaCatalog:  &stubSchemaCatalogHandlers{},
		SavedSites:     &stubSavedSitesHandlers{},
		Webhook:        &stubWebhookHandlers{},
		Analyze:        &stubAnalyzeHandlers{},
		Extraction:     &stubExtractionHandlers{},
		Admin:          &stubAdminHandlers{},
		AdminAnalytics: &stubAdminAnalyticsHandlers{},
		Metrics:        &stubMetricsHandlers{},
	}
}

// --- Public endpoint stubs ---

func stubHealthCheck(_ context.Context, _ *struct{}) (*handlers.HealthCheckOutput, error) {
	return nil, nil
}

func stubListTierLimits(_ context.Context, _ *struct{}) (*handlers.ListTierLimitsOutput, error) {
	return nil, nil
}

func stubListCleaners(_ context.Context, _ *struct{}) (*handlers.ListCleanersOutput, error) {
	return nil, nil
}

func stubLivez(_ context.Context, _ *struct{}) (*handlers.LivezOutput, error) {
	return nil, nil
}

func stubReadyz(_ context.Context, _ *struct{}) (*handlers.ReadyzOutput, error) {
	return nil, nil
}

// --- Job handlers stub ---

type stubJobHandlers struct{}

func (s *stubJobHandlers) ListJobs(_ context.Context, _ *handlers.ListJobsInput) (*handlers.ListJobsOutput, error) {
	return nil, nil
}

func (s *stubJobHandlers) GetJob(_ context.Context, _ *handlers.GetJobInput) (*handlers.GetJobOutput, error) {
	return nil, nil
}

func (s *stubJobHandlers) GetCrawlMap(_ context.Context, _ *handlers.GetCrawlMapInput) (*handlers.GetCrawlMapOutput, error) {
	return nil, nil
}

func (s *stubJobHandlers) GetJobResultsDownload(_ context.Context, _ *handlers.GetJobResultsDownloadInput) (*handlers.GetJobResultsDownloadOutput, error) {
	return nil, nil
}

func (s *stubJobHandlers) GetJobWebhookDeliveries(_ context.Context, _ *handlers.GetJobWebhookDeliveriesInput) (*handlers.GetJobWebhookDeliveriesOutput, error) {
	return nil, nil
}

func (s *stubJobHandlers) GetJobDebugCapture(_ context.Context, _ *handlers.GetJobDebugCaptureInput) (*handlers.GetJobDebugCaptureOutput, error) {
	return nil, nil
}

func (s *stubJobHandlers) DownloadJobDebugCapture(_ context.Context, _ *handlers.DownloadJobDebugCaptureInput) (*handlers.DownloadJobDebugCaptureOutput, error) {
	return nil, nil
}

// RegisterRawEndpoints calls the real handler's RegisterRawEndpoints method.
// The real method already registers placeholder handlers, so it works for OpenAPI generation.
// This avoids duplicating the Operation definitions.
func (s *stubJobHandlers) RegisterRawEndpoints(api huma.API) {
	// Create a minimal JobHandler just for registering raw endpoints.
	// The real RegisterRawEndpoints only defines Operations - it doesn't need services.
	realHandler := &handlers.JobHandler{}
	realHandler.RegisterRawEndpoints(api)
}

// --- Crawl handlers stub ---

type stubCrawlHandlers struct{}

func (s *stubCrawlHandlers) CreateCrawlJob(_ context.Context, _ *handlers.CreateCrawlJobInput) (*handlers.CreateCrawlJobOutput, error) {
	return nil, nil
}

// --- Usage handlers stub ---

type stubUsageHandlers struct{}

func (s *stubUsageHandlers) GetUsage(_ context.Context, _ *handlers.GetUsageInput) (*handlers.GetUsageOutput, error) {
	return nil, nil
}

// --- User LLM handlers stub ---

type stubUserLLMHandlers struct{}

func (s *stubUserLLMHandlers) ListProviders(_ context.Context, _ *handlers.ListProvidersInput) (*handlers.ListProvidersOutput, error) {
	return nil, nil
}

func (s *stubUserLLMHandlers) ListModels(_ context.Context, _ *handlers.UserListModelsInput) (*handlers.UserListModelsOutput, error) {
	return nil, nil
}

func (s *stubUserLLMHandlers) ListServiceKeys(_ context.Context, _ *struct{}) (*handlers.ListUserServiceKeysOutput, error) {
	return nil, nil
}

func (s *stubUserLLMHandlers) UpsertServiceKey(_ context.Context, _ *handlers.UpsertUserServiceKeyInput) (*handlers.UpsertUserServiceKeyOutput, error) {
	return nil, nil
}

func (s *stubUserLLMHandlers) DeleteServiceKey(_ context.Context, _ *handlers.DeleteUserServiceKeyInput) (*handlers.DeleteUserServiceKeyOutput, error) {
	return nil, nil
}

func (s *stubUserLLMHandlers) GetFallbackChain(_ context.Context, _ *struct{}) (*handlers.GetUserFallbackChainOutput, error) {
	return nil, nil
}

func (s *stubUserLLMHandlers) SetFallbackChain(_ context.Context, _ *handlers.SetUserFallbackChainInput) (*handlers.SetUserFallbackChainOutput, error) {
	return nil, nil
}

// --- API Key handlers stub ---

type stubAPIKeyHandlers struct{}

func (s *stubAPIKeyHandlers) ListKeys(_ context.Context, _ *struct{}) (*handlers.ListKeysOutput, error) {
	return nil, nil
}

func (s *stubAPIKeyHandlers) CreateKey(_ context.Context, _ *handlers.CreateKeyInput) (*handlers.CreateKeyOutput, error) {
	return nil, nil
}

func (s *stubAPIKeyHandlers) RevokeKey(_ context.Context, _ *handlers.RevokeKeyInput) (*handlers.RevokeKeyOutput, error) {
	return nil, nil
}

// --- Schema Catalog handlers stub ---

type stubSchemaCatalogHandlers struct{}

func (s *stubSchemaCatalogHandlers) ListSchemas(_ context.Context, _ *handlers.ListSchemasInput) (*handlers.ListSchemasOutput, error) {
	return nil, nil
}

func (s *stubSchemaCatalogHandlers) GetSchema(_ context.Context, _ *handlers.GetSchemaInput) (*handlers.GetSchemaOutput, error) {
	return nil, nil
}

func (s *stubSchemaCatalogHandlers) CreateSchema(_ context.Context, _ *handlers.CreateSchemaInput) (*handlers.CreateSchemaOutput, error) {
	return nil, nil
}

func (s *stubSchemaCatalogHandlers) UpdateSchema(_ context.Context, _ *handlers.UpdateSchemaInput) (*handlers.UpdateSchemaOutput, error) {
	return nil, nil
}

func (s *stubSchemaCatalogHandlers) DeleteSchema(_ context.Context, _ *handlers.DeleteSchemaInput) (*handlers.DeleteSchemaOutput, error) {
	return nil, nil
}

func (s *stubSchemaCatalogHandlers) ListAllSchemas(_ context.Context, _ *struct{}) (*handlers.ListAllSchemasOutput, error) {
	return nil, nil
}

func (s *stubSchemaCatalogHandlers) CreatePlatformSchema(_ context.Context, _ *handlers.CreatePlatformSchemaInput) (*handlers.CreateSchemaOutput, error) {
	return nil, nil
}

// --- Saved Sites handlers stub ---

type stubSavedSitesHandlers struct{}

func (s *stubSavedSitesHandlers) ListSavedSites(_ context.Context, _ *struct{}) (*handlers.ListSavedSitesOutput, error) {
	return nil, nil
}

func (s *stubSavedSitesHandlers) GetSavedSite(_ context.Context, _ *handlers.GetSavedSiteInput) (*handlers.GetSavedSiteOutput, error) {
	return nil, nil
}

func (s *stubSavedSitesHandlers) CreateSavedSite(_ context.Context, _ *handlers.CreateSavedSiteInput) (*handlers.CreateSavedSiteOutput, error) {
	return nil, nil
}

func (s *stubSavedSitesHandlers) UpdateSavedSite(_ context.Context, _ *handlers.UpdateSavedSiteInput) (*handlers.UpdateSavedSiteOutput, error) {
	return nil, nil
}

func (s *stubSavedSitesHandlers) DeleteSavedSite(_ context.Context, _ *handlers.DeleteSavedSiteInput) (*handlers.DeleteSavedSiteOutput, error) {
	return nil, nil
}

// --- Webhook handlers stub ---

type stubWebhookHandlers struct{}

func (s *stubWebhookHandlers) ListWebhooks(_ context.Context, _ *struct{}) (*handlers.ListWebhooksOutput, error) {
	return nil, nil
}

func (s *stubWebhookHandlers) GetWebhook(_ context.Context, _ *handlers.GetWebhookInput) (*handlers.GetWebhookOutput, error) {
	return nil, nil
}

func (s *stubWebhookHandlers) CreateWebhook(_ context.Context, _ *handlers.CreateWebhookInput) (*handlers.CreateWebhookOutput, error) {
	return nil, nil
}

func (s *stubWebhookHandlers) UpdateWebhook(_ context.Context, _ *handlers.UpdateWebhookInput) (*handlers.UpdateWebhookOutput, error) {
	return nil, nil
}

func (s *stubWebhookHandlers) DeleteWebhook(_ context.Context, _ *handlers.DeleteWebhookInput) (*handlers.DeleteWebhookOutput, error) {
	return nil, nil
}

func (s *stubWebhookHandlers) ListWebhookDeliveries(_ context.Context, _ *handlers.ListWebhookDeliveriesInput) (*handlers.ListWebhookDeliveriesOutput, error) {
	return nil, nil
}

// --- Analyze handlers stub ---

type stubAnalyzeHandlers struct{}

func (s *stubAnalyzeHandlers) Analyze(_ context.Context, _ *handlers.AnalyzeInput) (*handlers.AnalyzeOutput, error) {
	return nil, nil
}

// --- Extraction handlers stub ---

type stubExtractionHandlers struct{}

func (s *stubExtractionHandlers) Extract(_ context.Context, _ *handlers.ExtractInput) (*handlers.ExtractOutput, error) {
	return nil, nil
}

// --- Admin handlers stub ---

type stubAdminHandlers struct{}

func (s *stubAdminHandlers) ListServiceKeys(_ context.Context, _ *struct{}) (*handlers.ListServiceKeysOutput, error) {
	return nil, nil
}

func (s *stubAdminHandlers) UpsertServiceKey(_ context.Context, _ *handlers.UpsertServiceKeyInput) (*handlers.UpsertServiceKeyOutput, error) {
	return nil, nil
}

func (s *stubAdminHandlers) DeleteServiceKey(_ context.Context, _ *handlers.DeleteServiceKeyInput) (*handlers.DeleteServiceKeyOutput, error) {
	return nil, nil
}

func (s *stubAdminHandlers) GetFallbackChain(_ context.Context, _ *handlers.GetFallbackChainInput) (*handlers.GetFallbackChainOutput, error) {
	return nil, nil
}

func (s *stubAdminHandlers) SetFallbackChain(_ context.Context, _ *handlers.SetFallbackChainInput) (*handlers.SetFallbackChainOutput, error) {
	return nil, nil
}

func (s *stubAdminHandlers) ListModels(_ context.Context, _ *handlers.ListModelsInput) (*handlers.ListModelsOutput, error) {
	return nil, nil
}

func (s *stubAdminHandlers) ValidateModels(_ context.Context, _ *handlers.ValidateModelsInput) (*handlers.ValidateModelsOutput, error) {
	return nil, nil
}

func (s *stubAdminHandlers) ListTiers(_ context.Context, _ *struct{}) (*handlers.ListTiersOutput, error) {
	return nil, nil
}

func (s *stubAdminHandlers) ValidateTiers(_ context.Context, _ *handlers.ValidateTiersInput) (*handlers.ValidateTiersOutput, error) {
	return nil, nil
}

func (s *stubAdminHandlers) SyncTiers(_ context.Context, _ *struct{}) (*handlers.SyncTiersOutput, error) {
	return nil, nil
}

// --- Admin Analytics handlers stub ---

type stubAdminAnalyticsHandlers struct{}

func (s *stubAdminAnalyticsHandlers) GetOverview(_ context.Context, _ *handlers.OverviewInput) (*handlers.GetOverviewOutput, error) {
	return nil, nil
}

func (s *stubAdminAnalyticsHandlers) GetJobs(_ context.Context, _ *handlers.AnalyticsJobsInput) (*handlers.GetAnalyticsJobsOutput, error) {
	return nil, nil
}

func (s *stubAdminAnalyticsHandlers) GetErrors(_ context.Context, _ *handlers.ErrorsInput) (*handlers.GetErrorsOutput, error) {
	return nil, nil
}

func (s *stubAdminAnalyticsHandlers) GetTrends(_ context.Context, _ *handlers.TrendsInput) (*handlers.GetTrendsOutput, error) {
	return nil, nil
}

func (s *stubAdminAnalyticsHandlers) GetUsers(_ context.Context, _ *handlers.AnalyticsUsersInput) (*handlers.GetAnalyticsUsersOutput, error) {
	return nil, nil
}

func (s *stubAdminAnalyticsHandlers) GetJobResults(_ context.Context, _ *handlers.AdminJobResultsInput) (*handlers.AdminJobResultsOutput, error) {
	return nil, nil
}

// --- Metrics handlers stub ---

type stubMetricsHandlers struct{}

func (s *stubMetricsHandlers) GetMetrics(_ context.Context, _ *struct{}) (*handlers.GetMetricsOutput, error) {
	return nil, nil
}
