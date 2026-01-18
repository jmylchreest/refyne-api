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
	providerRegistry := llm.InitRegistry(cfg)
	logger.Info("provider registry initialized", "providers", providerRegistry.AllProviderNames())

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
		ExposedHeaders:   []string{"Link", "X-Request-ID", "X-RateLimit-Limit", "X-RateLimit-Remaining", "Retry-After"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Request size limit (1MB) - prevent large payload attacks
	router.Use(middleware.RequestSize(1 * 1024 * 1024))

	// Global rate limit by IP (fallback for unauthenticated requests)
	// Authenticated users get tier-based limits applied later
	router.Use(httprate.LimitByIP(100, time.Minute))

	// Global concurrency throttle - prevent system overload
	router.Use(middleware.Throttle(100))

	// Create Huma API config for main API with OpenAPI docs
	humaConfig := huma.DefaultConfig("Refyne API", "1.0.0")
	humaConfig.Info.Description = "LLM-powered web extraction API that transforms unstructured websites into clean, typed JSON."
	humaConfig.Servers = []*huma.Server{
		{URL: cfg.BaseURL, Description: "API Server"},
	}
	// Add security scheme for Bearer auth
	humaConfig.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			Description:  "API key authentication. Include your API key in the Authorization header as `Bearer rf_your_key`.",
		},
	}

	// Main API with OpenAPI docs
	api := humachi.New(router, humaConfig)

	// Config for hidden routes (K8s probes - no docs needed)
	hiddenConfig := huma.DefaultConfig("Refyne API", "1.0.0")
	hiddenConfig.DocsPath = ""
	hiddenConfig.OpenAPIPath = ""
	hiddenConfig.SchemasPath = ""
	hiddenAPI := humachi.New(router, hiddenConfig)

	// Config for protected routes (no separate docs - they're served by the main API)
	// Note: Routes registered here don't appear in the main OpenAPI spec.
	// The comprehensive API documentation is maintained in the web project's openapi.json
	protectedConfig := huma.DefaultConfig("Refyne API", "1.0.0")
	protectedConfig.Info.Description = humaConfig.Info.Description
	protectedConfig.Servers = humaConfig.Servers
	protectedConfig.DocsPath = ""
	protectedConfig.OpenAPIPath = ""
	protectedConfig.SchemasPath = ""

	// Health check (public, shown in docs)
	huma.Get(api, "/api/v1/health", handlers.HealthCheck)

	// Public pricing/tier info (for dynamic pricing pages)
	huma.Get(api, "/api/v1/pricing/tiers", handlers.ListTierLimits)

	// Kubernetes probes (hidden from docs - internal use only)
	huma.Get(hiddenAPI, "/healthz", handlers.Livez)
	readyzHandler := handlers.NewReadyzHandler(db)
	huma.Get(hiddenAPI, "/readyz", readyzHandler.Readyz)

	// Clerk webhook (signature verified by handler, not user auth)
	if cfg.ClerkWebhookSecret != "" {
		userCleanupSvc := service.NewUserCleanupService(db, logger)
		clerkWebhook := handlers.NewClerkWebhookHandler(cfg, services.Balance, userCleanupSvc, services.TierSync, logger)
		router.Post("/api/v1/webhooks/clerk", clerkWebhook.HandleWebhook)
		logger.Info("clerk webhook endpoint enabled")
	}

	// Protected routes
	router.Group(func(r chi.Router) {
		r.Use(mw.Auth(clerkVerifier, services.Auth))
		r.Use(mw.TierGate(services.Usage))

		// Create a new Huma API for protected routes
		protectedAPI := humachi.New(r, protectedConfig)

		// Job list and status routes (no quota check needed)
		jobHandler := handlers.NewJobHandlerWithWebhook(services.Job, services.Storage, services.Webhook)
		huma.Get(protectedAPI, "/api/v1/jobs", jobHandler.ListJobs)
		huma.Get(protectedAPI, "/api/v1/jobs/{id}", jobHandler.GetJob)
		huma.Get(protectedAPI, "/api/v1/jobs/{id}/crawl-map", jobHandler.GetCrawlMap)
		huma.Get(protectedAPI, "/api/v1/jobs/{id}/download", jobHandler.GetJobResultsDownload)
		huma.Get(protectedAPI, "/api/v1/jobs/{id}/webhooks", jobHandler.GetJobWebhookDeliveries)

		// Raw HTTP handlers for format-aware responses (non-JSON content types)
		r.Get("/api/v1/jobs/{id}/results", jobHandler.GetJobResultsRaw)
		r.Get("/api/v1/jobs/{id}/stream", jobHandler.StreamResults)

		// Usage routes
		huma.Get(protectedAPI, "/api/v1/usage", handlers.NewUsageHandler(services.Usage).GetUsage)

		// LLM config routes (legacy single-provider config)
		huma.Get(protectedAPI, "/api/v1/llm-config", handlers.NewLLMConfigHandler(services.LLMConfig).GetConfig)
		huma.Put(protectedAPI, "/api/v1/llm-config", handlers.NewLLMConfigHandler(services.LLMConfig).UpdateConfig)

		// User LLM provider keys and fallback chain routes
		userLLMHandler := handlers.NewUserLLMHandler(services.UserLLM, services.Admin, providerRegistry)
		huma.Get(protectedAPI, "/api/v1/llm/providers", userLLMHandler.ListProviders)
		huma.Get(protectedAPI, "/api/v1/llm/keys", userLLMHandler.ListServiceKeys)
		huma.Put(protectedAPI, "/api/v1/llm/keys", userLLMHandler.UpsertServiceKey)
		huma.Delete(protectedAPI, "/api/v1/llm/keys/{id}", userLLMHandler.DeleteServiceKey)
		huma.Get(protectedAPI, "/api/v1/llm/chain", userLLMHandler.GetFallbackChain)
		huma.Put(protectedAPI, "/api/v1/llm/chain", userLLMHandler.SetFallbackChain)
		huma.Get(protectedAPI, "/api/v1/llm/models/{provider}", userLLMHandler.ListModels)

		// API key routes (for hosted mode - users can manage their own API keys)
		if !cfg.IsSelfHosted() {
			huma.Get(protectedAPI, "/api/v1/keys", handlers.NewAPIKeyHandler(services.APIKey).ListKeys)
			huma.Post(protectedAPI, "/api/v1/keys", handlers.NewAPIKeyHandler(services.APIKey).CreateKey)
			huma.Delete(protectedAPI, "/api/v1/keys/{id}", handlers.NewAPIKeyHandler(services.APIKey).RevokeKey)
		}

		// Superadmin routes (requires global_superadmin in Clerk public_metadata)
		adminHandler := handlers.NewAdminHandler(services.Admin, services.TierSync)
		huma.Get(protectedAPI, "/api/v1/admin/service-keys", adminHandler.ListServiceKeys)
		huma.Put(protectedAPI, "/api/v1/admin/service-keys", adminHandler.UpsertServiceKey)
		huma.Delete(protectedAPI, "/api/v1/admin/service-keys/{provider}", adminHandler.DeleteServiceKey)

		// Fallback chain configuration
		huma.Get(protectedAPI, "/api/v1/admin/fallback-chain", adminHandler.GetFallbackChain)
		huma.Put(protectedAPI, "/api/v1/admin/fallback-chain", adminHandler.SetFallbackChain)

		// Provider models listing and validation
		huma.Get(protectedAPI, "/api/v1/admin/models/{provider}", adminHandler.ListModels)
		huma.Post(protectedAPI, "/api/v1/admin/models/validate", adminHandler.ValidateModels)

		// Subscription tiers (from Clerk)
		huma.Get(protectedAPI, "/api/v1/admin/tiers", adminHandler.ListTiers)
		huma.Post(protectedAPI, "/api/v1/admin/tiers/validate", adminHandler.ValidateTiers)
		huma.Post(protectedAPI, "/api/v1/admin/tiers/sync", adminHandler.SyncTiers)

		// Admin schema catalog management
		schemaCatalogHandler := handlers.NewSchemaCatalogHandler(repos.SchemaCatalog)
		huma.Get(protectedAPI, "/api/v1/admin/schemas", schemaCatalogHandler.ListAllSchemas)
		huma.Post(protectedAPI, "/api/v1/admin/schemas", schemaCatalogHandler.CreatePlatformSchema)

		// Schema catalog routes (accessible to all authenticated users)
		huma.Get(protectedAPI, "/api/v1/schemas", schemaCatalogHandler.ListSchemas)
		huma.Get(protectedAPI, "/api/v1/schemas/{id}", schemaCatalogHandler.GetSchema)

		// Saved sites routes
		savedSitesHandler := handlers.NewSavedSitesHandler(repos.SavedSites)
		huma.Get(protectedAPI, "/api/v1/sites", savedSitesHandler.ListSavedSites)
		huma.Get(protectedAPI, "/api/v1/sites/{id}", savedSitesHandler.GetSavedSite)
		huma.Post(protectedAPI, "/api/v1/sites", savedSitesHandler.CreateSavedSite)
		huma.Put(protectedAPI, "/api/v1/sites/{id}", savedSitesHandler.UpdateSavedSite)
		huma.Delete(protectedAPI, "/api/v1/sites/{id}", savedSitesHandler.DeleteSavedSite)

		// Webhook management routes
		var webhookEncryptor *crypto.Encryptor
		if len(cfg.EncryptionKey) > 0 {
			webhookEncryptor, _ = crypto.NewEncryptor(cfg.EncryptionKey)
		}
		webhookHandler := handlers.NewWebhookHandler(repos.Webhook, repos.WebhookDelivery, webhookEncryptor)
		huma.Get(protectedAPI, "/api/v1/webhooks", webhookHandler.ListWebhooks)
		huma.Get(protectedAPI, "/api/v1/webhooks/{id}", webhookHandler.GetWebhook)
		huma.Post(protectedAPI, "/api/v1/webhooks", webhookHandler.CreateWebhook)
		huma.Put(protectedAPI, "/api/v1/webhooks/{id}", webhookHandler.UpdateWebhook)
		huma.Delete(protectedAPI, "/api/v1/webhooks/{id}", webhookHandler.DeleteWebhook)
		huma.Get(protectedAPI, "/api/v1/webhooks/{id}/deliveries", webhookHandler.ListWebhookDeliveries)
	})

	// Schema routes that require schema_custom feature
	router.Group(func(r chi.Router) {
		r.Use(mw.Auth(clerkVerifier, services.Auth))
		r.Use(mw.TierGate(services.Usage))
		r.Use(mw.RequireFeature("schema_custom"))

		schemaAPI := humachi.New(r, protectedConfig)
		schemaCatalogHandler := handlers.NewSchemaCatalogHandler(repos.SchemaCatalog)
		huma.Post(schemaAPI, "/api/v1/schemas", schemaCatalogHandler.CreateSchema)
		huma.Put(schemaAPI, "/api/v1/schemas/{id}", schemaCatalogHandler.UpdateSchema)
		huma.Delete(schemaAPI, "/api/v1/schemas/{id}", schemaCatalogHandler.DeleteSchema)
	})

	// Analyze routes (requires content_analyzer feature)
	router.Group(func(r chi.Router) {
		r.Use(mw.Auth(clerkVerifier, services.Auth))
		r.Use(mw.TierGate(services.Usage))
		r.Use(mw.RequireFeature("content_analyzer"))

		analyzeAPI := humachi.New(r, protectedConfig)
		huma.Post(analyzeAPI, "/api/v1/analyze", handlers.NewAnalyzeHandler(services.Analyzer, repos.Job).Analyze)
	})

	// Extraction/crawl routes with quota and concurrency checking
	router.Group(func(r chi.Router) {
		r.Use(mw.Auth(clerkVerifier, services.Auth))
		r.Use(mw.TierGate(services.Usage))
		r.Use(mw.RequireUsageQuota(services.Usage))
		r.Use(mw.RequireConcurrentJobLimit(services.Job))
		r.Use(mw.RateLimitByUser(mw.DefaultRateLimitConfig()))
		// Extend write deadline for sync crawl requests (wait=true)
		r.Use(mw.ExtendWriteDeadlineForSyncRequests())

		// Create a new Huma API for quota-gated routes
		quotaAPI := humachi.New(r, protectedConfig)

		// Extraction routes
		huma.Post(quotaAPI, "/api/v1/extract", handlers.NewExtractionHandler(services.Extraction, services.Job).Extract)

		// Job creation routes
		huma.Post(quotaAPI, "/api/v1/crawl", handlers.NewJobHandler(services.Job, services.Storage).CreateCrawlJob)
	})

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
