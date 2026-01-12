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

	"github.com/jmylchreest/refyne-api/internal/auth"
	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/database"
	"github.com/jmylchreest/refyne-api/internal/http/handlers"
	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/logging"
	"github.com/jmylchreest/refyne-api/internal/repository"
	"github.com/jmylchreest/refyne-api/internal/service"
	"github.com/jmylchreest/refyne-api/internal/worker"
)

func main() {
	// Initialize logger with TTY detection, source paths, and format control
	logger := logging.SetDefault()

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
	defer db.Close()

	// Run migrations
	if err := database.Migrate(db); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Initialize repositories
	repos := repository.NewRepositories(db)

	// Initialize services
	services, err := service.NewServices(cfg, repos, logger)
	if err != nil {
		logger.Error("failed to initialize services", "error", err)
		os.Exit(1)
	}

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
		services.Extraction,
		services.Webhook,
		worker.Config{
			PollInterval: 5 * time.Second,
			Concurrency:  3,
		},
		logger,
	)
	ctx, cancel := context.WithCancel(context.Background())
	jobWorker.Start(ctx)

	// Create router
	router := chi.NewRouter()

	// Global middleware
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(60 * time.Second))

	// CORS configuration
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Create Huma API
	humaConfig := huma.DefaultConfig("Refyne API", "1.0.0")
	humaConfig.Info.Description = "LLM-powered web scraping API"
	humaConfig.Servers = []*huma.Server{
		{URL: cfg.BaseURL, Description: "API Server"},
	}

	api := humachi.New(router, humaConfig)

	// Health check (no auth required)
	huma.Get(api, "/api/v1/health", handlers.HealthCheck)

	// Clerk webhook (signature verified by handler, not user auth)
	if cfg.ClerkWebhookSecret != "" {
		clerkWebhook := handlers.NewClerkWebhookHandler(cfg, services.Balance, logger)
		router.Post("/api/v1/webhooks/clerk", clerkWebhook.HandleWebhook)
		logger.Info("clerk webhook endpoint enabled")
	}

	// Protected routes
	router.Group(func(r chi.Router) {
		r.Use(mw.Auth(clerkVerifier, services.Auth))
		r.Use(mw.TierGate(services.Usage))

		// Create a new Huma API for protected routes
		protectedAPI := humachi.New(r, humaConfig)

		// Job list and status routes (no quota check needed)
		huma.Get(protectedAPI, "/api/v1/jobs", handlers.NewJobHandler(services.Job).ListJobs)
		huma.Get(protectedAPI, "/api/v1/jobs/{id}", handlers.NewJobHandler(services.Job).GetJob)

		// SSE streaming for job results
		r.Get("/api/v1/jobs/{id}/stream", handlers.NewJobHandler(services.Job).StreamResults)

		// Usage routes
		huma.Get(protectedAPI, "/api/v1/usage", handlers.NewUsageHandler(services.Usage).GetUsage)

		// LLM config routes
		huma.Get(protectedAPI, "/api/v1/llm-config", handlers.NewLLMConfigHandler(services.LLMConfig).GetConfig)
		huma.Put(protectedAPI, "/api/v1/llm-config", handlers.NewLLMConfigHandler(services.LLMConfig).UpdateConfig)

		// API key routes (for hosted mode - users can manage their own API keys)
		if !cfg.IsSelfHosted() {
			huma.Get(protectedAPI, "/api/v1/keys", handlers.NewAPIKeyHandler(services.APIKey).ListKeys)
			huma.Post(protectedAPI, "/api/v1/keys", handlers.NewAPIKeyHandler(services.APIKey).CreateKey)
			huma.Delete(protectedAPI, "/api/v1/keys/{id}", handlers.NewAPIKeyHandler(services.APIKey).RevokeKey)
		}

		// Superadmin routes (requires global_superadmin in Clerk public_metadata)
		adminHandler := handlers.NewAdminHandler(services.Admin)
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
	})

	// Extraction/crawl routes with quota checking
	router.Group(func(r chi.Router) {
		r.Use(mw.Auth(clerkVerifier, services.Auth))
		r.Use(mw.TierGate(services.Usage))
		r.Use(mw.RequireUsageQuota(services.Usage))

		// Create a new Huma API for quota-gated routes
		quotaAPI := humachi.New(r, humaConfig)

		// Extraction routes (with tier-based rate limiting)
		huma.Post(quotaAPI, "/api/v1/extract", handlers.NewExtractionHandler(services.Extraction).Extract)

		// Job creation routes
		huma.Post(quotaAPI, "/api/v1/crawl", handlers.NewJobHandler(services.Job).CreateCrawlJob)
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
