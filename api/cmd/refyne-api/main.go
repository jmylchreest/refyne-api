// Package main is the entry point for the refyne-api server.
// Note: User management, OAuth, sessions, subscriptions, and billing are handled by Clerk.
// Self-hosted mode is API-only with API key authentication.
package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof" // Register pprof handlers on DefaultServeMux
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/jmylchreest/refyne-api/internal/auth"
	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/database"
	"github.com/jmylchreest/refyne-api/internal/http/handlers"
	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/http/routes"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/logging"
	"github.com/jmylchreest/refyne-api/internal/repository"
	"github.com/jmylchreest/refyne-api/internal/service"
	"github.com/jmylchreest/refyne-api/internal/shutdown"
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

	// Start pprof server (access via: fly proxy 6060:6060)
	// Binds to 0.0.0.0 so it's accessible via Fly.io internal networking
	// Track active connections to suppress idle timeout while profiling
	var pprofActiveConns int64
	if os.Getenv("ENABLE_PPROF") == "true" {
		go func() {
			pprofAddr := "0.0.0.0:6060"
			logger.Info("starting pprof server", "addr", pprofAddr)
			// Wrap DefaultServeMux with connection tracking
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&pprofActiveConns, 1)
				defer atomic.AddInt64(&pprofActiveConns, -1)
				logger.Debug("pprof request", "path", r.URL.Path, "active_conns", atomic.LoadInt64(&pprofActiveConns))
				http.DefaultServeMux.ServeHTTP(w, r)
			})
			if err := http.ListenAndServe(pprofAddr, handler); err != nil {
				logger.Error("pprof server error", "error", err)
			}
		}()
	}

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

	// Clean up stale running jobs from previous server runs (async - not critical for startup)
	// Jobs running for more than 1 hour are considered stale on startup
	go func() {
		staleCount, err := repos.Job.MarkStaleRunningJobsFailed(context.Background(), 1*time.Hour)
		if err != nil {
			logger.Warn("failed to clean up stale jobs", "error", err)
		} else if staleCount > 0 {
			logger.Info("cleaned up stale running jobs", "count", staleCount)
		}
	}()

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
	// LLMConfigResolver uses PricingService for dynamic max_completion_tokens from OpenRouter API
	services.LLMConfigResolver.SetPricingService(services.Pricing)

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
			PollInterval:        cfg.WorkerPollInterval,
			Concurrency:         cfg.WorkerConcurrency,
			ShutdownGracePeriod: cfg.WorkerShutdownGracePeriod,
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
		go cleanupSvc.RunScheduledCleanup(ctx, cfg.CleanupMaxAgeResults, cfg.CleanupMaxAgeDebug, cfg.CleanupInterval)
		logger.Info("cleanup service started",
			"max_age_results", cfg.CleanupMaxAgeResults.String(),
			"max_age_debug", cfg.CleanupMaxAgeDebug.String(),
			"interval", cfg.CleanupInterval.String(),
		)
	}

	// Initialize idle monitor for scale-to-zero (must be created before router middleware)
	// The background work checker prevents idle shutdown while:
	// - The job worker is processing jobs
	// - Someone is connected to pprof (profiling in progress)
	idleMonitor := shutdown.NewIdleMonitor(shutdown.IdleMonitorConfig{
		Timeout: cfg.IdleTimeout,
		Logger:  logger,
		ExcludePaths: []string{
			"/healthz",
			"/livez",
			"/readyz",
			"/api/v1/health",
		},
		BackgroundWorkCheck: func() bool {
			activeJobs := jobWorker.ActiveJobs() > 0
			pprofConns := atomic.LoadInt64(&pprofActiveConns) > 0
			if pprofConns {
				logger.Debug("idle suppressed: pprof connection active", "pprof_conns", atomic.LoadInt64(&pprofActiveConns))
			}
			return activeJobs || pprofConns
		},
	})
	idleMonitor.Start()
	defer idleMonitor.Stop()

	// Create router
	router := chi.NewRouter()

	// Global middleware
	router.Use(middleware.CleanPath) // Normalize double slashes (//api -> /api)
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(idleMonitor.Middleware) // Track requests for scale-to-zero

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

		// API key configuration (for demo/partner/internal keys)
		config.InitAPIKeyLoader(config.S3LoaderConfig{
			S3Client: services.Storage.Client(),
			Bucket:   bucket,
			Key:      "config/api-keys.json",
			Logger:   logger,
		})
		// Pre-load API keys asynchronously (lazy-loads on first use if not ready)
		go func() {
			if loader := config.GetAPIKeyLoader(); loader != nil {
				loader.Load(context.Background())
			}
		}()

		logger.Info("S3 config loaders enabled",
			"bucket", bucket,
			"cache_ttl", "5m",
			"configs", []string{"blocklist.json", "logfilters.json", "model_defaults.json", "tier_settings.json", "api-keys.json"},
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

	// Create Huma API with shared config (ensures OpenAPI spec matches between server and generator)
	humaConfig := routes.NewHumaConfig(cfg.BaseURL)
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

	// Initialize all handlers
	readyzHandler := handlers.NewReadyzHandler(db)
	jobHandler := handlers.NewJobHandlerWithWebhook(services.Job, services.Storage, services.Webhook)
	usageHandler := handlers.NewUsageHandler(services.Usage)
	userLLMHandler := handlers.NewUserLLMHandler(services.UserLLM, services.Admin, providerRegistry)
	adminHandler := handlers.NewAdminHandler(services.Admin, services.TierSync)
	adminAnalyticsHandler := handlers.NewAdminAnalyticsHandler(repos.Analytics, services.Storage)
	metricsHandler := handlers.NewMetricsHandler(repos)
	schemaCatalogHandler := handlers.NewSchemaCatalogHandler(repos.SchemaCatalog)
	savedSitesHandler := handlers.NewSavedSitesHandler(repos.SavedSites)
	var webhookEncryptor *crypto.Encryptor
	if len(cfg.EncryptionKey) > 0 {
		webhookEncryptor, _ = crypto.NewEncryptor(cfg.EncryptionKey)
	}
	webhookHandler := handlers.NewWebhookHandler(repos.Webhook, repos.WebhookDelivery, webhookEncryptor)
	extractionHandler := handlers.NewExtractionHandler(services.Extraction, services.Job)
	crawlHandler := handlers.NewJobHandler(services.Job, services.Storage, services.LLMConfigResolver)
	analyzeHandler := handlers.NewAnalyzeHandler(services.Analyzer, services.Job)

	// Build handlers struct for shared route registration
	routeHandlers := &routes.Handlers{
		HealthCheck:    handlers.HealthCheck,
		ListTierLimits: handlers.ListTierLimits,
		ListCleaners:   handlers.ListCleaners,
		Livez:          handlers.Livez,
		Readyz:         readyzHandler.Readyz,
		Job:            jobHandler,
		Crawl:          crawlHandler,
		Usage:          usageHandler,
		UserLLM:        userLLMHandler,
		SchemaCatalog:  schemaCatalogHandler,
		SavedSites:     savedSitesHandler,
		Webhook:        webhookHandler,
		Analyze:        analyzeHandler,
		Extraction:     extractionHandler,
		Admin:          adminHandler,
		AdminAnalytics: adminAnalyticsHandler,
		Metrics:        metricsHandler,
	}

	// Add API key handler in hosted mode
	if !cfg.IsSelfHosted() {
		routeHandlers.APIKey = handlers.NewAPIKeyHandler(services.APIKey)
	}

	// Register all routes using shared definitions
	routes.Register(api, routeHandlers)

	// Clerk webhook (signature verified by handler, not user auth)
	// This is registered separately as it's not part of the user-authenticated API
	if cfg.ClerkWebhookSecret != "" {
		userCleanupSvc := service.NewUserCleanupService(db, services.Storage, logger)
		clerkWebhook := handlers.NewClerkWebhookHandler(cfg, services.Balance, userCleanupSvc, services.TierSync, logger)
		router.Post("/api/v1/webhooks/clerk", clerkWebhook.HandleWebhook)
		logger.Info("clerk webhook endpoint enabled")
	}

	// Raw HTTP handlers for format-aware responses (non-JSON content types)
	// These use Chi middleware for auth since they're not Huma operations.
	// RegisterRawEndpoints (called by routes.Register) adds them to OpenAPI with proper security.
	chiAuthMiddleware := mw.Auth(clerkVerifier, services.Auth, services.SubscriptionCache)
	router.With(chiAuthMiddleware).Get("/api/v1/jobs/{id}/results", jobHandler.GetJobResultsRaw)
	router.With(chiAuthMiddleware).Get("/api/v1/jobs/{id}/stream", jobHandler.StreamResults)

	// Create server with h2c (HTTP/2 cleartext) support for Fly.io proxy
	// WriteTimeout must be long enough for LLM requests (can take 60-120s for complex pages)
	h2s := &http2.Server{}
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      h2c.NewHandler(router, h2s),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  300 * time.Second,
	}

	// Graceful shutdown - triggered by signal OR idle timeout
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

		var idleShutdown bool
		select {
		case <-sigChan:
			logger.Info("received shutdown signal")
		case <-idleMonitor.ShutdownChan():
			idleShutdown = true
			logger.Info("idle shutdown triggered")
		}

		// Determine shutdown timeout:
		// - Idle shutdown: fast exit (we know active requests = 0)
		// - Signal shutdown: allow time for in-flight requests
		shutdownTimeout := 30 * time.Second
		if idleShutdown {
			shutdownTimeout = 2 * time.Second
		}

		logger.Info("shutting down server", "timeout", shutdownTimeout, "idle_triggered", idleShutdown)

		// Stop the worker first
		cancel()
		jobWorker.Stop()

		// Stop log filters loader if running
		if logFiltersLoader != nil {
			logFiltersLoader.Stop()
		}

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
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
