package routes

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
)

// Register registers all API routes with the given Huma API instance.
// Pass real handler implementations for the main server, or stub implementations
// for OpenAPI generation.
func Register(api huma.API, h *Handlers) {
	// =========================================================================
	// Public Routes (no auth required)
	// =========================================================================

	// Health check
	mw.PublicGet(api, "/api/v1/health", h.HealthCheck,
		mw.WithTags("Health"),
		mw.WithSummary("Health check"),
		mw.WithOperationID("healthCheck"))

	// Public pricing/tier info (for dynamic pricing pages)
	mw.PublicGet(api, "/api/v1/pricing/tiers", h.ListTierLimits,
		mw.WithTags("Pricing"),
		mw.WithSummary("List subscription tiers"),
		mw.WithOperationID("listTiers"))


	// Kubernetes probes (hidden from docs - internal use only)
	mw.HiddenGet(api, "/healthz", h.Livez)
	mw.HiddenGet(api, "/readyz", h.Readyz)

	// =========================================================================
	// Protected Routes (require bearer auth)
	// =========================================================================

	// --- Jobs ---
	mw.ProtectedGet(api, "/api/v1/jobs", h.Job.ListJobs,
		mw.WithTags("Jobs"),
		mw.WithSummary("List jobs"),
		mw.WithOperationID("listJobs"))
	mw.ProtectedGet(api, "/api/v1/jobs/{id}", h.Job.GetJob,
		mw.WithTags("Jobs"),
		mw.WithSummary("Get job details"),
		mw.WithOperationID("getJob"))
	mw.ProtectedGet(api, "/api/v1/jobs/{id}/crawl-map", h.Job.GetCrawlMap,
		mw.WithTags("Jobs"),
		mw.WithSummary("Get crawl map"),
		mw.WithOperationID("getCrawlMap"))
	mw.ProtectedGet(api, "/api/v1/jobs/{id}/download", h.Job.GetJobResultsDownload,
		mw.WithTags("Jobs"),
		mw.WithSummary("Download job results"),
		mw.WithOperationID("downloadJobResults"))
	mw.ProtectedGet(api, "/api/v1/jobs/{id}/webhooks", h.Job.GetJobWebhookDeliveries,
		mw.WithTags("Jobs"),
		mw.WithSummary("Get job webhook deliveries"),
		mw.WithOperationID("getJobWebhookDeliveries"))
	mw.ProtectedGet(api, "/api/v1/jobs/{id}/debug-capture", h.Job.GetJobDebugCapture,
		mw.WithTags("Jobs"),
		mw.WithSummary("Get job debug captures"),
		mw.WithDescription("Returns captured LLM prompts and metadata for debugging extraction issues"),
		mw.WithOperationID("getJobDebugCapture"))

	// Raw HTTP handlers for format-aware responses (non-JSON content types)
	// RegisterRawEndpoints adds them to OpenAPI with proper security requirements.
	h.Job.RegisterRawEndpoints(api)

	// --- Usage ---
	mw.ProtectedGet(api, "/api/v1/usage", h.Usage.GetUsage,
		mw.WithTags("Usage"),
		mw.WithSummary("Get usage statistics"),
		mw.WithOperationID("getUsage"))

	// --- Configuration ---
	mw.ProtectedGet(api, "/api/v1/cleaners", h.ListCleaners,
		mw.WithTags("Configuration"),
		mw.WithSummary("List content cleaners"),
		mw.WithDescription("Returns available content cleaners with their options. Cleaners process HTML before extraction to reduce tokens or extract main content."),
		mw.WithOperationID("listCleaners"))

	// --- LLM Providers ---
	mw.ProtectedGet(api, "/api/v1/llm/providers", h.UserLLM.ListProviders,
		mw.WithTags("LLM Providers"),
		mw.WithSummary("List LLM providers"),
		mw.WithOperationID("listProviders"))
	mw.ProtectedGet(api, "/api/v1/llm/models/{provider}", h.UserLLM.ListModels,
		mw.WithTags("LLM Providers"),
		mw.WithSummary("List models for provider"),
		mw.WithOperationID("listModels"))

	// --- LLM Keys ---
	mw.ProtectedGet(api, "/api/v1/llm/keys", h.UserLLM.ListServiceKeys,
		mw.WithTags("LLM Keys"),
		mw.WithSummary("List user LLM keys"),
		mw.WithOperationID("listLlmKeys"))
	mw.ProtectedPut(api, "/api/v1/llm/keys", h.UserLLM.UpsertServiceKey,
		mw.WithTags("LLM Keys"),
		mw.WithSummary("Upsert user LLM key"),
		mw.WithOperationID("upsertLlmKey"))
	mw.ProtectedDelete(api, "/api/v1/llm/keys/{id}", h.UserLLM.DeleteServiceKey,
		mw.WithTags("LLM Keys"),
		mw.WithSummary("Delete user LLM key"),
		mw.WithOperationID("deleteLlmKey"))

	// --- LLM Chain ---
	mw.ProtectedGet(api, "/api/v1/llm/chain", h.UserLLM.GetFallbackChain,
		mw.WithTags("LLM Chain"),
		mw.WithSummary("Get LLM fallback chain"),
		mw.WithOperationID("getLlmChain"))
	mw.ProtectedPut(api, "/api/v1/llm/chain", h.UserLLM.SetFallbackChain,
		mw.WithTags("LLM Chain"),
		mw.WithSummary("Set LLM fallback chain"),
		mw.WithOperationID("setLlmChain"))

	// --- API Keys (hosted mode only) ---
	if h.IncludeAPIKeys() {
		mw.ProtectedGet(api, "/api/v1/keys", h.APIKey.ListKeys,
			mw.WithTags("API Keys"),
			mw.WithSummary("List API keys"),
			mw.WithOperationID("listApiKeys"))
		mw.ProtectedPost(api, "/api/v1/keys", h.APIKey.CreateKey,
			mw.WithTags("API Keys"),
			mw.WithSummary("Create API key"),
			mw.WithOperationID("createApiKey"))
		mw.ProtectedDelete(api, "/api/v1/keys/{id}", h.APIKey.RevokeKey,
			mw.WithTags("API Keys"),
			mw.WithSummary("Revoke API key"),
			mw.WithOperationID("revokeApiKey"))
	}

	// --- Admin Routes (require superadmin, hidden from OpenAPI) ---
	mw.ProtectedGet(api, "/api/v1/admin/service-keys", h.Admin.ListServiceKeys,
		mw.WithTags("Admin"),
		mw.WithSummary("List service keys"),
		mw.WithOperationID("adminListServiceKeys"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedPut(api, "/api/v1/admin/service-keys", h.Admin.UpsertServiceKey,
		mw.WithTags("Admin"),
		mw.WithSummary("Upsert service key"),
		mw.WithOperationID("adminUpsertServiceKey"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedDelete(api, "/api/v1/admin/service-keys/{provider}", h.Admin.DeleteServiceKey,
		mw.WithTags("Admin"),
		mw.WithSummary("Delete service key"),
		mw.WithOperationID("adminDeleteServiceKey"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/fallback-chain", h.Admin.GetFallbackChain,
		mw.WithTags("Admin"),
		mw.WithSummary("Get admin fallback chain"),
		mw.WithOperationID("adminGetFallbackChain"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedPut(api, "/api/v1/admin/fallback-chain", h.Admin.SetFallbackChain,
		mw.WithTags("Admin"),
		mw.WithSummary("Set admin fallback chain"),
		mw.WithOperationID("adminSetFallbackChain"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/models/{provider}", h.Admin.ListModels,
		mw.WithTags("Admin"),
		mw.WithSummary("List models for provider (admin)"),
		mw.WithOperationID("adminListModels"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedPost(api, "/api/v1/admin/models/validate", h.Admin.ValidateModels,
		mw.WithTags("Admin"),
		mw.WithSummary("Validate models"),
		mw.WithOperationID("adminValidateModels"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/tiers", h.Admin.ListTiers,
		mw.WithTags("Admin"),
		mw.WithSummary("List subscription tiers (admin)"),
		mw.WithOperationID("adminListTiers"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedPost(api, "/api/v1/admin/tiers/validate", h.Admin.ValidateTiers,
		mw.WithTags("Admin"),
		mw.WithSummary("Validate tiers"),
		mw.WithOperationID("adminValidateTiers"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedPost(api, "/api/v1/admin/tiers/sync", h.Admin.SyncTiers,
		mw.WithTags("Admin"),
		mw.WithSummary("Sync tiers from Clerk"),
		mw.WithOperationID("adminSyncTiers"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/schemas", h.SchemaCatalog.ListAllSchemas,
		mw.WithTags("Admin"),
		mw.WithSummary("List all schemas (admin)"),
		mw.WithOperationID("adminListSchemas"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedPost(api, "/api/v1/admin/schemas", h.SchemaCatalog.CreatePlatformSchema,
		mw.WithTags("Admin"),
		mw.WithSummary("Create platform schema"),
		mw.WithOperationID("adminCreatePlatformSchema"),
		mw.WithSuperadmin(),
		mw.WithHidden())

	// --- Admin Analytics Routes ---
	mw.ProtectedGet(api, "/api/v1/admin/analytics/overview", h.AdminAnalytics.GetOverview,
		mw.WithTags("Admin"),
		mw.WithSummary("Get analytics overview"),
		mw.WithOperationID("adminGetAnalyticsOverview"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/analytics/jobs", h.AdminAnalytics.GetJobs,
		mw.WithTags("Admin"),
		mw.WithSummary("Get analytics jobs"),
		mw.WithOperationID("adminGetAnalyticsJobs"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/analytics/errors", h.AdminAnalytics.GetErrors,
		mw.WithTags("Admin"),
		mw.WithSummary("Get analytics errors"),
		mw.WithOperationID("adminGetAnalyticsErrors"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/analytics/trends", h.AdminAnalytics.GetTrends,
		mw.WithTags("Admin"),
		mw.WithSummary("Get analytics trends"),
		mw.WithOperationID("adminGetAnalyticsTrends"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/analytics/users", h.AdminAnalytics.GetUsers,
		mw.WithTags("Admin"),
		mw.WithSummary("Get analytics users"),
		mw.WithOperationID("adminGetAnalyticsUsers"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/analytics/jobs/{id}/results", h.AdminAnalytics.GetJobResults,
		mw.WithTags("Admin"),
		mw.WithSummary("Get job results download URL"),
		mw.WithOperationID("adminGetJobResults"),
		mw.WithSuperadmin(),
		mw.WithHidden())

	// --- Schemas (read access for all authenticated users) ---
	mw.ProtectedGet(api, "/api/v1/schemas", h.SchemaCatalog.ListSchemas,
		mw.WithTags("Schemas"),
		mw.WithSummary("List schemas"),
		mw.WithOperationID("listSchemas"))
	mw.ProtectedGet(api, "/api/v1/schemas/{id}", h.SchemaCatalog.GetSchema,
		mw.WithTags("Schemas"),
		mw.WithSummary("Get schema"),
		mw.WithOperationID("getSchema"))

	// Schema write operations (require schema_custom feature)
	mw.ProtectedPost(api, "/api/v1/schemas", h.SchemaCatalog.CreateSchema,
		mw.WithTags("Schemas"),
		mw.WithSummary("Create schema"),
		mw.WithOperationID("createSchema"),
		mw.WithFeature("schema_custom"))
	mw.ProtectedPut(api, "/api/v1/schemas/{id}", h.SchemaCatalog.UpdateSchema,
		mw.WithTags("Schemas"),
		mw.WithSummary("Update schema"),
		mw.WithOperationID("updateSchema"),
		mw.WithFeature("schema_custom"))
	mw.ProtectedDelete(api, "/api/v1/schemas/{id}", h.SchemaCatalog.DeleteSchema,
		mw.WithTags("Schemas"),
		mw.WithSummary("Delete schema"),
		mw.WithOperationID("deleteSchema"),
		mw.WithFeature("schema_custom"))

	// --- Saved Sites ---
	mw.ProtectedGet(api, "/api/v1/sites", h.SavedSites.ListSavedSites,
		mw.WithTags("Sites"),
		mw.WithSummary("List saved sites"),
		mw.WithOperationID("listSites"))
	mw.ProtectedGet(api, "/api/v1/sites/{id}", h.SavedSites.GetSavedSite,
		mw.WithTags("Sites"),
		mw.WithSummary("Get saved site"),
		mw.WithOperationID("getSite"))
	mw.ProtectedPost(api, "/api/v1/sites", h.SavedSites.CreateSavedSite,
		mw.WithTags("Sites"),
		mw.WithSummary("Create saved site"),
		mw.WithOperationID("createSite"))
	mw.ProtectedPut(api, "/api/v1/sites/{id}", h.SavedSites.UpdateSavedSite,
		mw.WithTags("Sites"),
		mw.WithSummary("Update saved site"),
		mw.WithOperationID("updateSite"))
	mw.ProtectedDelete(api, "/api/v1/sites/{id}", h.SavedSites.DeleteSavedSite,
		mw.WithTags("Sites"),
		mw.WithSummary("Delete saved site"),
		mw.WithOperationID("deleteSite"))

	// --- Webhooks ---
	mw.ProtectedGet(api, "/api/v1/webhooks", h.Webhook.ListWebhooks,
		mw.WithTags("Webhooks"),
		mw.WithSummary("List webhooks"),
		mw.WithOperationID("listWebhooks"))
	mw.ProtectedGet(api, "/api/v1/webhooks/{id}", h.Webhook.GetWebhook,
		mw.WithTags("Webhooks"),
		mw.WithSummary("Get webhook"),
		mw.WithOperationID("getWebhook"))
	mw.ProtectedPost(api, "/api/v1/webhooks", h.Webhook.CreateWebhook,
		mw.WithTags("Webhooks"),
		mw.WithSummary("Create webhook"),
		mw.WithOperationID("createWebhook"))
	mw.ProtectedPut(api, "/api/v1/webhooks/{id}", h.Webhook.UpdateWebhook,
		mw.WithTags("Webhooks"),
		mw.WithSummary("Update webhook"),
		mw.WithOperationID("updateWebhook"))
	mw.ProtectedDelete(api, "/api/v1/webhooks/{id}", h.Webhook.DeleteWebhook,
		mw.WithTags("Webhooks"),
		mw.WithSummary("Delete webhook"),
		mw.WithOperationID("deleteWebhook"))
	mw.ProtectedGet(api, "/api/v1/webhooks/{id}/deliveries", h.Webhook.ListWebhookDeliveries,
		mw.WithTags("Webhooks"),
		mw.WithSummary("List webhook deliveries"),
		mw.WithOperationID("listWebhookDeliveries"))

	// --- Analyze (requires content_analyzer feature) ---
	mw.ProtectedPost(api, "/api/v1/analyze", h.Analyze.Analyze,
		mw.WithTags("Extraction"),
		mw.WithSummary("Analyze URL"),
		mw.WithOperationID("analyze"),
		mw.WithFeature("content_analyzer"))

	// --- Extract and Crawl (require quota and concurrency checks) ---
	mw.ProtectedPost(api, "/api/v1/extract", h.Extraction.Extract,
		mw.WithTags("Extraction"),
		mw.WithSummary("Extract data from URL"),
		mw.WithOperationID("extract"),
		mw.WithQuotaCheck(),
		mw.WithConcurrencyCheck())
	mw.ProtectedPost(api, "/api/v1/crawl", h.Crawl.CreateCrawlJob,
		mw.WithTags("Extraction"),
		mw.WithSummary("Start crawl job"),
		mw.WithOperationID("crawl"),
		mw.WithQuotaCheck(),
		mw.WithConcurrencyCheck())
}
