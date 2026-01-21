// Package main is the entry point for the refyne-api server.
// Note: User management, OAuth, sessions, subscriptions, and billing are handled by Clerk.
// Self-hosted mode is API-only with API key authentication.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"

	"github.com/jmylchreest/refyne-api/internal/auth"
	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/database"
	"github.com/jmylchreest/refyne-api/internal/http/handlers"
	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/logging"
	"github.com/jmylchreest/refyne-api/internal/repository"
	"github.com/jmylchreest/refyne-api/internal/service"
	"github.com/jmylchreest/refyne-api/internal/version"
	"github.com/jmylchreest/refyne-api/internal/worker"
)

func main() {
	// Initialize logger with TTY detection, source paths, and format control
	logger := logging.SetDefault()

	// Log version info first thing
	v := version.Get()
	logger.Info("starting refyne-api",
		"version", v.Version,
		"commit", v.Commit,
		"built", v.Date,
		"go_version", v.GoVersion,
	)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize database
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	// Run migrations (with logging for each migration applied)
	if err := database.MigrateWithLogger(db, logger); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Log current schema version
	schemaVersion, err := database.GetLatestSchemaVersion(db)
	if err != nil {
		logger.Warn("failed to get schema version", "error", err)
	} else if schemaVersion != "" {
		migrationCount, _ := database.GetMigrationCount(db)
		logger.Info("database schema ready", "schema_version", schemaVersion, "migrations_applied", migrationCount)
	}

	// Initialize repositories
	repos := repository.NewRepositories(db)

	// Clean up stale running jobs from previous server runs
	// Jobs running for more than 1 hour are considered stale on startup
	staleCount, err := repos.Job.MarkStaleRunningJobsFailed(context.Background(), 1*time.Hour)
	if err != nil {
		logger.Warn("failed to clean up stale jobs", "error", err)
	} else if staleCount > 0 {
		logger.Info("cleaned up stale running jobs", "count", staleCount)
	}

	// Initialize services
	services, err := service.NewServices(cfg, repos, logger)
	if err != nil {
		logger.Error("failed to initialize services", "error", err)
		os.Exit(1)
	}

	// Initialize LLM provider registry
	providerRegistry := llm.InitRegistry(cfg, logger)
	logger.Info("provider registry initialized", "providers", providerRegistry.AllProviderNames())

	// Wire the registry to services that need capability lookups
	// PricingService populates the registry's capability cache when fetching OpenRouter data
	services.Pricing.SetCapabilitiesCache(providerRegistry)
	// LLMConfigResolver uses the registry for StrictMode determination (shared by ExtractionService and AnalyzerService)
	services.LLMConfigResolver.SetRegistry(providerRegistry)

	// Initialize Clerk verifier for JWT validation (hosted mode)
	var clerkVerifier *auth.ClerkVerifier
	if cfg.ClerkIssuerURL != "" {
		clerkVerifier = auth.NewClerkVerifier(cfg.ClerkIssuerURL)
		logger.Info("clerk authentication enabled", "issuer", cfg.ClerkIssuerURL)
	} else if !cfg.IsSelfHosted() {
		logger.Warn("CLERK_ISSUER_URL not set - JWT authentication will fail")
	}

	// Start background worker for job processing
	jobWorker := worker.New(
		repos.Job,
		repos.JobResult,
		services.Extraction,
		services.Webhook,
		services.Storage,
		services.Sitemap,
		worker.Config{
			PollInterval: 5 * time.Second,
			Concurrency:  3,
		},
		logger,
	)
	ctx, cancel := context.WithCancel(context.Background())
	jobWorker.Start(ctx)

	// Start cleanup service if enabled
	if cfg.CleanupEnabled {
		cleanupSvc := service.NewCleanupService(
			repos.Job,
			repos.JobResult,
			services.Storage,
			logger,
		)
		go cleanupSvc.RunScheduledCleanup(ctx, cfg.CleanupMaxAge, cfg.CleanupInterval)
		logger.Info("cleanup service started",
			"max_age", cfg.CleanupMaxAge.String(),
			"interval", cfg.CleanupInterval.String(),
		)
	}

	// Create router
	router := chi.NewRouter()

	// Global middleware
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)

	// S3-backed configuration loaders
	// All use the same bucket with different keys under config/
	var logFiltersLoader *mw.LogFiltersLoader
	if services.Storage.IsEnabled() && cfg.BlocklistBucket != "" {
		bucket := cfg.BlocklistBucket

		// IP blocklist (early in chain to reject bad actors quickly)
		blocklist := mw.NewIPBlocklist(mw.BlocklistConfig{
			S3Client: services.Storage.Client(),
			Bucket:   bucket,
			Key:      "config/blocklist.json",
			Logger:   logger,
		})
		router.Use(blocklist.Middleware())

		// Log filters (dynamic log filtering from S3)
		logFiltersLoader = mw.NewLogFiltersLoader(mw.LogFiltersConfig{
			S3Client: services.Storage.Client(),
			Bucket:   bucket,
			Key:      "config/logfilters.json",
			Logger:   logger,
		})
		logFiltersLoader.Start(ctx)

		// Model defaults (LLM model settings from S3)
		llm.InitGlobalModelDefaults(llm.ModelDefaultsConfig{
			S3Client: services.Storage.Client(),
			Bucket:   bucket,
			Key:      "config/model_defaults.json",
			Logger:   logger,
		})

		// Tier settings (override hardcoded tier limits from S3)
		constants.InitTierLoader(constants.TierSettingsConfig{
			S3Client: services.Storage.Client(),
			Bucket:   bucket,
			Key:      "config/tier_settings.json",
			Logger:   logger,
		})

		logger.Info("S3 config loaders enabled",
			"bucket", bucket,
			"cache_ttl", "5m",
			"configs", []string{"blocklist.json", "logfilters.json", "model_defaults.json", "tier_settings.json"},
		)
	}

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	// Request timeout middleware with different timeouts per endpoint type
	router.Use(mw.Timeout(mw.TimeoutConfig{
		Default:  constants.DefaultRequestTimeout,
		Extended: constants.LLMRequestTimeout,
		// LLM operations get extended timeout (page fetch + inference)
		ExtendedPatterns: []string{"/analyze", "/extract"},
		// SSE streaming has no timeout (managed by client disconnect)
		SkipPatterns: []string{"/stream"},
	}))

	// CORS configuration
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID", "X-RateLimit-Limit", "X-RateLimit-Remaining", "Retry-After", "Cache-Control", "X-API-Version"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// API version header (for SDK compatibility checks)
	router.Use(mw.APIVersion())

	// Cache-Control headers for response caching
	router.Use(mw.Cache(mw.DefaultCacheConfig()))

	// Request size limit (1MB) - prevent large payload attacks
	router.Use(middleware.RequestSize(1 * 1024 * 1024))

	// Global rate limit by IP (fallback for unauthenticated requests)
	// Authenticated users get tier-based limits applied later
	router.Use(httprate.LimitByIP(100, time.Minute))

	// Global concurrency throttle - prevent system overload
	router.Use(middleware.Throttle(100))

	// Create Huma API config for main API with OpenAPI docs
	humaConfig := huma.DefaultConfig("Refyne API", version.Get().Short())
	humaConfig.Info.Description = "LLM-powered web extraction API that transforms unstructured websites into clean, typed JSON."
	// Disable $schema field in responses - it conflicts with "schema" field in SDK code generators
	humaConfig.CreateHooks = nil
	humaConfig.Servers = []*huma.Server{
		{URL: cfg.BaseURL, Description: "API Server"},
	}
	// Add security scheme for Bearer auth
	humaConfig.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		mw.SecurityScheme: {
			Type:        "http",
			Scheme:      "bearer",
			Description: "API key authentication. Include your API key in the Authorization header as `Bearer rf_your_key`.",
		},
	}

	// Define OpenAPI tags with display names for documentation
	humaConfig.Tags = []*huma.Tag{
		{Name: "Extraction", Description: "Data extraction, crawling, and analysis endpoints", Extensions: map[string]any{"x-displayName": "Extraction"}},
		{Name: "Jobs", Description: "Job status and results retrieval", Extensions: map[string]any{"x-displayName": "Jobs"}},
		{Name: "Schemas", Description: "Schema catalog management", Extensions: map[string]any{"x-displayName": "Schemas"}},
		{Name: "Sites", Description: "Saved site configuration", Extensions: map[string]any{"x-displayName": "Sites"}},
		{Name: "Webhooks", Description: "Webhook management for real-time notifications", Extensions: map[string]any{"x-displayName": "Webhooks"}},
		{Name: "API Keys", Description: "API key management", Extensions: map[string]any{"x-displayName": "API Keys"}},
		{Name: "LLM Providers", Description: "Available LLM providers and models", Extensions: map[string]any{"x-displayName": "LLM Providers"}},
		{Name: "LLM Keys", Description: "User LLM API key management", Extensions: map[string]any{"x-displayName": "LLM Keys"}},
		{Name: "LLM Chain", Description: "LLM fallback chain configuration", Extensions: map[string]any{"x-displayName": "LLM Chain"}},
		{Name: "Usage", Description: "Usage statistics and billing", Extensions: map[string]any{"x-displayName": "Usage"}},
		{Name: "Health", Description: "System health and status", Extensions: map[string]any{"x-displayName": "Health"}},
		{Name: "Pricing", Description: "Pricing and tier information", Extensions: map[string]any{"x-displayName": "Pricing"}},
	}

	// Single Huma API instance for all routes - this ensures all routes appear in OpenAPI
	api := humachi.New(router, humaConfig)

	// Add Huma middleware for authentication based on operation security requirements
	api.UseMiddleware(mw.HumaAuth(api, mw.HumaAuthConfig{
		ClerkVerifier:     clerkVerifier,
		AuthService:       services.Auth,
		SubscriptionCache: services.SubscriptionCache,
		UsageService:      services.Usage,
		JobService:        services.Job,
	}))

	// Add user-based rate limiting for operations that need it
	api.UseMiddleware(mw.HumaRateLimit(mw.DefaultRateLimitConfig()))

	// =========================================================================
	// Public Routes (no auth required)
	// =========================================================================

	// Health check
	mw.PublicGet(api, "/api/v1/health", handlers.HealthCheck,
		mw.WithTags("Health"),
		mw.WithSummary("Health check"),
		mw.WithOperationID("healthCheck"))

	// Public pricing/tier info (for dynamic pricing pages)
	mw.PublicGet(api, "/api/v1/pricing/tiers", handlers.ListTierLimits,
		mw.WithTags("Pricing"),
		mw.WithSummary("List subscription tiers"),
		mw.WithOperationID("listTiers"))

	// Kubernetes probes (hidden from docs - internal use only)
	mw.HiddenGet(api, "/healthz", handlers.Livez)
	readyzHandler := handlers.NewReadyzHandler(db)
	mw.HiddenGet(api, "/readyz", readyzHandler.Readyz)

	// Clerk webhook (signature verified by handler, not user auth)
	if cfg.ClerkWebhookSecret != "" {
		userCleanupSvc := service.NewUserCleanupService(db, services.Storage, logger)
		clerkWebhook := handlers.NewClerkWebhookHandler(cfg, services.Balance, userCleanupSvc, services.TierSync, logger)
		router.Post("/api/v1/webhooks/clerk", clerkWebhook.HandleWebhook)
		logger.Info("clerk webhook endpoint enabled")
	}

	// =========================================================================
	// Protected Routes (require bearer auth)
	// =========================================================================

	// Initialize handlers
	jobHandler := handlers.NewJobHandlerWithWebhook(services.Job, services.Storage, services.Webhook)
	usageHandler := handlers.NewUsageHandler(services.Usage)
	userLLMHandler := handlers.NewUserLLMHandler(services.UserLLM, services.Admin, providerRegistry)
	adminHandler := handlers.NewAdminHandler(services.Admin, services.TierSync)
	adminAnalyticsHandler := handlers.NewAdminAnalyticsHandler(repos.Analytics, services.Storage)
	schemaCatalogHandler := handlers.NewSchemaCatalogHandler(repos.SchemaCatalog)
	savedSitesHandler := handlers.NewSavedSitesHandler(repos.SavedSites)
	var webhookEncryptor *crypto.Encryptor
	if len(cfg.EncryptionKey) > 0 {
		webhookEncryptor, _ = crypto.NewEncryptor(cfg.EncryptionKey)
	}
	webhookHandler := handlers.NewWebhookHandler(repos.Webhook, repos.WebhookDelivery, webhookEncryptor)
	extractionHandler := handlers.NewExtractionHandler(services.Extraction, services.Job)
	crawlHandler := handlers.NewJobHandler(services.Job, services.Storage, services.LLMConfigResolver)
	analyzeHandler := handlers.NewAnalyzeHandler(services.Analyzer, repos.Job)

	// --- Jobs ---
	mw.ProtectedGet(api, "/api/v1/jobs", jobHandler.ListJobs,
		mw.WithTags("Jobs"),
		mw.WithSummary("List jobs"),
		mw.WithOperationID("listJobs"))
	mw.ProtectedGet(api, "/api/v1/jobs/{id}", jobHandler.GetJob,
		mw.WithTags("Jobs"),
		mw.WithSummary("Get job details"),
		mw.WithOperationID("getJob"))
	mw.ProtectedGet(api, "/api/v1/jobs/{id}/crawl-map", jobHandler.GetCrawlMap,
		mw.WithTags("Jobs"),
		mw.WithSummary("Get crawl map"),
		mw.WithOperationID("getCrawlMap"))
	mw.ProtectedGet(api, "/api/v1/jobs/{id}/download", jobHandler.GetJobResultsDownload,
		mw.WithTags("Jobs"),
		mw.WithSummary("Download job results"),
		mw.WithOperationID("downloadJobResults"))
	mw.ProtectedGet(api, "/api/v1/jobs/{id}/webhooks", jobHandler.GetJobWebhookDeliveries,
		mw.WithTags("Jobs"),
		mw.WithSummary("Get job webhook deliveries"),
		mw.WithOperationID("getJobWebhookDeliveries"))

	// Raw HTTP handlers for format-aware responses (non-JSON content types)
	// These use Chi middleware for auth since they're not Huma operations.
	// RegisterRawEndpoints adds them to OpenAPI with proper security requirements.
	jobHandler.RegisterRawEndpoints(api)
	chiAuthMiddleware := mw.Auth(clerkVerifier, services.Auth, services.SubscriptionCache)
	router.With(chiAuthMiddleware).Get("/api/v1/jobs/{id}/results", jobHandler.GetJobResultsRaw)
	router.With(chiAuthMiddleware).Get("/api/v1/jobs/{id}/stream", jobHandler.StreamResults)

	// --- Usage ---
	mw.ProtectedGet(api, "/api/v1/usage", usageHandler.GetUsage,
		mw.WithTags("Usage"),
		mw.WithSummary("Get usage statistics"),
		mw.WithOperationID("getUsage"))

	// --- LLM Providers ---
	mw.ProtectedGet(api, "/api/v1/llm/providers", userLLMHandler.ListProviders,
		mw.WithTags("LLM Providers"),
		mw.WithSummary("List LLM providers"),
		mw.WithOperationID("listProviders"))
	mw.ProtectedGet(api, "/api/v1/llm/models/{provider}", userLLMHandler.ListModels,
		mw.WithTags("LLM Providers"),
		mw.WithSummary("List models for provider"),
		mw.WithOperationID("listModels"))

	// --- LLM Keys ---
	mw.ProtectedGet(api, "/api/v1/llm/keys", userLLMHandler.ListServiceKeys,
		mw.WithTags("LLM Keys"),
		mw.WithSummary("List user LLM keys"),
		mw.WithOperationID("listLlmKeys"))
	mw.ProtectedPut(api, "/api/v1/llm/keys", userLLMHandler.UpsertServiceKey,
		mw.WithTags("LLM Keys"),
		mw.WithSummary("Upsert user LLM key"),
		mw.WithOperationID("upsertLlmKey"))
	mw.ProtectedDelete(api, "/api/v1/llm/keys/{id}", userLLMHandler.DeleteServiceKey,
		mw.WithTags("LLM Keys"),
		mw.WithSummary("Delete user LLM key"),
		mw.WithOperationID("deleteLlmKey"))

	// --- LLM Chain ---
	mw.ProtectedGet(api, "/api/v1/llm/chain", userLLMHandler.GetFallbackChain,
		mw.WithTags("LLM Chain"),
		mw.WithSummary("Get LLM fallback chain"),
		mw.WithOperationID("getLlmChain"))
	mw.ProtectedPut(api, "/api/v1/llm/chain", userLLMHandler.SetFallbackChain,
		mw.WithTags("LLM Chain"),
		mw.WithSummary("Set LLM fallback chain"),
		mw.WithOperationID("setLlmChain"))

	// --- API Keys (hosted mode only) ---
	if !cfg.IsSelfHosted() {
		apiKeyHandler := handlers.NewAPIKeyHandler(services.APIKey)
		mw.ProtectedGet(api, "/api/v1/keys", apiKeyHandler.ListKeys,
			mw.WithTags("API Keys"),
			mw.WithSummary("List API keys"),
			mw.WithOperationID("listApiKeys"))
		mw.ProtectedPost(api, "/api/v1/keys", apiKeyHandler.CreateKey,
			mw.WithTags("API Keys"),
			mw.WithSummary("Create API key"),
			mw.WithOperationID("createApiKey"))
		mw.ProtectedDelete(api, "/api/v1/keys/{id}", apiKeyHandler.RevokeKey,
			mw.WithTags("API Keys"),
			mw.WithSummary("Revoke API key"),
			mw.WithOperationID("revokeApiKey"))
	}

	// --- Admin Routes (require superadmin, hidden from OpenAPI) ---
	mw.ProtectedGet(api, "/api/v1/admin/service-keys", adminHandler.ListServiceKeys,
		mw.WithTags("Admin"),
		mw.WithSummary("List service keys"),
		mw.WithOperationID("adminListServiceKeys"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedPut(api, "/api/v1/admin/service-keys", adminHandler.UpsertServiceKey,
		mw.WithTags("Admin"),
		mw.WithSummary("Upsert service key"),
		mw.WithOperationID("adminUpsertServiceKey"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedDelete(api, "/api/v1/admin/service-keys/{provider}", adminHandler.DeleteServiceKey,
		mw.WithTags("Admin"),
		mw.WithSummary("Delete service key"),
		mw.WithOperationID("adminDeleteServiceKey"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/fallback-chain", adminHandler.GetFallbackChain,
		mw.WithTags("Admin"),
		mw.WithSummary("Get admin fallback chain"),
		mw.WithOperationID("adminGetFallbackChain"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedPut(api, "/api/v1/admin/fallback-chain", adminHandler.SetFallbackChain,
		mw.WithTags("Admin"),
		mw.WithSummary("Set admin fallback chain"),
		mw.WithOperationID("adminSetFallbackChain"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/models/{provider}", adminHandler.ListModels,
		mw.WithTags("Admin"),
		mw.WithSummary("List models for provider (admin)"),
		mw.WithOperationID("adminListModels"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedPost(api, "/api/v1/admin/models/validate", adminHandler.ValidateModels,
		mw.WithTags("Admin"),
		mw.WithSummary("Validate models"),
		mw.WithOperationID("adminValidateModels"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/tiers", adminHandler.ListTiers,
		mw.WithTags("Admin"),
		mw.WithSummary("List subscription tiers (admin)"),
		mw.WithOperationID("adminListTiers"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedPost(api, "/api/v1/admin/tiers/validate", adminHandler.ValidateTiers,
		mw.WithTags("Admin"),
		mw.WithSummary("Validate tiers"),
		mw.WithOperationID("adminValidateTiers"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedPost(api, "/api/v1/admin/tiers/sync", adminHandler.SyncTiers,
		mw.WithTags("Admin"),
		mw.WithSummary("Sync tiers from Clerk"),
		mw.WithOperationID("adminSyncTiers"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/schemas", schemaCatalogHandler.ListAllSchemas,
		mw.WithTags("Admin"),
		mw.WithSummary("List all schemas (admin)"),
		mw.WithOperationID("adminListSchemas"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedPost(api, "/api/v1/admin/schemas", schemaCatalogHandler.CreatePlatformSchema,
		mw.WithTags("Admin"),
		mw.WithSummary("Create platform schema"),
		mw.WithOperationID("adminCreatePlatformSchema"),
		mw.WithSuperadmin(),
		mw.WithHidden())

	// --- Admin Analytics Routes ---
	mw.ProtectedGet(api, "/api/v1/admin/analytics/overview", adminAnalyticsHandler.GetOverview,
		mw.WithTags("Admin"),
		mw.WithSummary("Get analytics overview"),
		mw.WithOperationID("adminGetAnalyticsOverview"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/analytics/jobs", adminAnalyticsHandler.GetJobs,
		mw.WithTags("Admin"),
		mw.WithSummary("Get analytics jobs"),
		mw.WithOperationID("adminGetAnalyticsJobs"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/analytics/errors", adminAnalyticsHandler.GetErrors,
		mw.WithTags("Admin"),
		mw.WithSummary("Get analytics errors"),
		mw.WithOperationID("adminGetAnalyticsErrors"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/analytics/trends", adminAnalyticsHandler.GetTrends,
		mw.WithTags("Admin"),
		mw.WithSummary("Get analytics trends"),
		mw.WithOperationID("adminGetAnalyticsTrends"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/analytics/users", adminAnalyticsHandler.GetUsers,
		mw.WithTags("Admin"),
		mw.WithSummary("Get analytics users"),
		mw.WithOperationID("adminGetAnalyticsUsers"),
		mw.WithSuperadmin(),
		mw.WithHidden())
	mw.ProtectedGet(api, "/api/v1/admin/analytics/jobs/{id}/results", adminAnalyticsHandler.GetJobResults,
		mw.WithTags("Admin"),
		mw.WithSummary("Get job results download URL"),
		mw.WithOperationID("adminGetJobResults"),
		mw.WithSuperadmin(),
		mw.WithHidden())

	// --- Schemas (read access for all authenticated users) ---
	mw.ProtectedGet(api, "/api/v1/schemas", schemaCatalogHandler.ListSchemas,
		mw.WithTags("Schemas"),
		mw.WithSummary("List schemas"),
		mw.WithOperationID("listSchemas"))
	mw.ProtectedGet(api, "/api/v1/schemas/{id}", schemaCatalogHandler.GetSchema,
		mw.WithTags("Schemas"),
		mw.WithSummary("Get schema"),
		mw.WithOperationID("getSchema"))

	// Schema write operations (require schema_custom feature)
	mw.ProtectedPost(api, "/api/v1/schemas", schemaCatalogHandler.CreateSchema,
		mw.WithTags("Schemas"),
		mw.WithSummary("Create schema"),
		mw.WithOperationID("createSchema"),
		mw.WithFeature("schema_custom"))
	mw.ProtectedPut(api, "/api/v1/schemas/{id}", schemaCatalogHandler.UpdateSchema,
		mw.WithTags("Schemas"),
		mw.WithSummary("Update schema"),
		mw.WithOperationID("updateSchema"),
		mw.WithFeature("schema_custom"))
	mw.ProtectedDelete(api, "/api/v1/schemas/{id}", schemaCatalogHandler.DeleteSchema,
		mw.WithTags("Schemas"),
		mw.WithSummary("Delete schema"),
		mw.WithOperationID("deleteSchema"),
		mw.WithFeature("schema_custom"))

	// --- Saved Sites ---
	mw.ProtectedGet(api, "/api/v1/sites", savedSitesHandler.ListSavedSites,
		mw.WithTags("Sites"),
		mw.WithSummary("List saved sites"),
		mw.WithOperationID("listSites"))
	mw.ProtectedGet(api, "/api/v1/sites/{id}", savedSitesHandler.GetSavedSite,
		mw.WithTags("Sites"),
		mw.WithSummary("Get saved site"),
		mw.WithOperationID("getSite"))
	mw.ProtectedPost(api, "/api/v1/sites", savedSitesHandler.CreateSavedSite,
		mw.WithTags("Sites"),
		mw.WithSummary("Create saved site"),
		mw.WithOperationID("createSite"))
	mw.ProtectedPut(api, "/api/v1/sites/{id}", savedSitesHandler.UpdateSavedSite,
		mw.WithTags("Sites"),
		mw.WithSummary("Update saved site"),
		mw.WithOperationID("updateSite"))
	mw.ProtectedDelete(api, "/api/v1/sites/{id}", savedSitesHandler.DeleteSavedSite,
		mw.WithTags("Sites"),
		mw.WithSummary("Delete saved site"),
		mw.WithOperationID("deleteSite"))

	// --- Webhooks ---
	mw.ProtectedGet(api, "/api/v1/webhooks", webhookHandler.ListWebhooks,
		mw.WithTags("Webhooks"),
		mw.WithSummary("List webhooks"),
		mw.WithOperationID("listWebhooks"))
	mw.ProtectedGet(api, "/api/v1/webhooks/{id}", webhookHandler.GetWebhook,
		mw.WithTags("Webhooks"),
		mw.WithSummary("Get webhook"),
		mw.WithOperationID("getWebhook"))
	mw.ProtectedPost(api, "/api/v1/webhooks", webhookHandler.CreateWebhook,
		mw.WithTags("Webhooks"),
		mw.WithSummary("Create webhook"),
		mw.WithOperationID("createWebhook"))
	mw.ProtectedPut(api, "/api/v1/webhooks/{id}", webhookHandler.UpdateWebhook,
		mw.WithTags("Webhooks"),
		mw.WithSummary("Update webhook"),
		mw.WithOperationID("updateWebhook"))
	mw.ProtectedDelete(api, "/api/v1/webhooks/{id}", webhookHandler.DeleteWebhook,
		mw.WithTags("Webhooks"),
		mw.WithSummary("Delete webhook"),
		mw.WithOperationID("deleteWebhook"))
	mw.ProtectedGet(api, "/api/v1/webhooks/{id}/deliveries", webhookHandler.ListWebhookDeliveries,
		mw.WithTags("Webhooks"),
		mw.WithSummary("List webhook deliveries"),
		mw.WithOperationID("listWebhookDeliveries"))

	// --- Analyze (requires content_analyzer feature) ---
	mw.ProtectedPost(api, "/api/v1/analyze", analyzeHandler.Analyze,
		mw.WithTags("Extraction"),
		mw.WithSummary("Analyze URL"),
		mw.WithOperationID("analyze"),
		mw.WithFeature("content_analyzer"))

	// --- Extract and Crawl (require quota and concurrency checks) ---
	mw.ProtectedPost(api, "/api/v1/extract", extractionHandler.Extract,
		mw.WithTags("Extraction"),
		mw.WithSummary("Extract data from URL"),
		mw.WithOperationID("extract"),
		mw.WithQuotaCheck(),
		mw.WithConcurrencyCheck())
	mw.ProtectedPost(api, "/api/v1/crawl", crawlHandler.CreateCrawlJob,
		mw.WithTags("Extraction"),
		mw.WithSummary("Start crawl job"),
		mw.WithOperationID("crawl"),
		mw.WithQuotaCheck(),
		mw.WithConcurrencyCheck())

	// Create server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
		<-sigChan

		logger.Info("shutting down server")

		// Stop the worker first
		cancel()
		jobWorker.Stop()

		// Stop log filters loader if running
		if logFiltersLoader != nil {
			logFiltersLoader.Stop()
		}

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown error", "error", err)
		}
	}()

	// Start server
	mode := "hosted"
	if cfg.IsSelfHosted() {
		mode = "self-hosted"
	}
	logger.Info("starting server", "port", cfg.Port, "base_url", cfg.BaseURL, "mode", mode)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
