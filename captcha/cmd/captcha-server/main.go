// Package main provides the entry point for the captcha server.
package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof" // Register pprof handlers on DefaultServeMux
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

	"github.com/jmylchreest/refyne-api/captcha/internal/api/handlers"
	"github.com/jmylchreest/refyne-api/captcha/internal/auth"
	"github.com/jmylchreest/refyne-api/captcha/internal/browser"
	"github.com/jmylchreest/refyne-api/captcha/internal/challenge"
	"github.com/jmylchreest/refyne-api/captcha/internal/config"
	"github.com/jmylchreest/refyne-api/captcha/internal/http/mw"
	"github.com/jmylchreest/refyne-api/captcha/internal/logging"
	"github.com/jmylchreest/refyne-api/captcha/internal/models"
	"github.com/jmylchreest/refyne-api/captcha/internal/session"
	"github.com/jmylchreest/refyne-api/captcha/internal/shutdown"
	"github.com/jmylchreest/refyne-api/captcha/internal/solver"
	"github.com/jmylchreest/refyne-api/captcha/internal/version"
)

func main() {
	// Load configuration first (logging config comes from env)
	cfg := config.Load()

	// Initialize logger using slog-logfilter (respects LOG_LEVEL, LOG_FORMAT env vars)
	logger := logging.SetDefault()

	logger.Info("starting captcha server",
		"version", version.Get().Version,
		"port", cfg.Port,
		"pool_size", cfg.BrowserPoolSize,
	)

	// Start pprof server on localhost only (access via: fly proxy 6060:6060)
	if os.Getenv("ENABLE_PPROF") == "true" {
		go func() {
			pprofAddr := "localhost:6060"
			logger.Info("starting pprof server", "addr", pprofAddr)
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				logger.Error("pprof server error", "error", err)
			}
		}()
	}

	if cfg.DisableStealth {
		logger.Warn("stealth mode DISABLED - browser will be detected as automated (for testing only)")
	} else {
		logger.Info("stealth mode enabled")
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize browser pool
	pool := browser.NewPool(cfg, logger)
	defer pool.Close()

	// Warmup: ensure Chromium is ready and pre-create one browser
	if err := pool.Warmup(ctx, 1); err != nil {
		logger.Error("failed to warmup browser pool", "error", err)
		os.Exit(1)
	}

	// Initialize session manager
	sessions := session.NewManager(cfg, logger)
	defer sessions.Close()
	go sessions.StartCleanup(ctx)

	// Initialize idle monitor for scale-to-zero
	idleMonitor := shutdown.NewIdleMonitor(shutdown.IdleMonitorConfig{
		Timeout: cfg.IdleTimeout,
		Logger:  logger,
	})
	idleMonitor.Start()
	defer idleMonitor.Stop()

	// Initialize challenge detector
	detector := challenge.NewDetector(logger)

	// Initialize solver chain - wait solver first (handles auto-resolving challenges)
	solvers := make([]solver.Solver, 0)

	// Wait solver handles Cloudflare JS challenges that auto-resolve
	waitSolver := solver.NewWaitSolver(detector, cfg.ChallengeTimeout)
	solvers = append(solvers, waitSolver)
	logger.Info("wait solver enabled (for auto-resolving challenges)")

	if cfg.TwoCaptchaAPIKey != "" {
		logger.Info("2captcha solver enabled")
		solvers = append(solvers, solver.NewTwoCaptcha(cfg.TwoCaptchaAPIKey))
	}

	if cfg.CapSolverAPIKey != "" {
		logger.Info("capsolver enabled (not yet implemented)")
		// TODO: Add CapSolver implementation
	}

	solverChain := solver.NewChain(solvers...)

	// Initialize Clerk verifier for direct JWT validation (optional)
	var clerkVerifier *auth.ClerkVerifier
	if cfg.ClerkIssuer != "" {
		clerkVerifier = auth.NewClerkVerifier(cfg.ClerkIssuer)
		logger.Info("Clerk JWT verification enabled", "issuer", cfg.ClerkIssuer)
	}

	// Build auth config
	authConfig := mw.AuthConfig{
		ClerkVerifier:   clerkVerifier,
		RefyneAPISecret: cfg.RefyneAPISecret,
		RequiredFeature: cfg.RequiredFeature,
		Logger:          logger,
	}

	// Initialize handlers
	healthHandler := handlers.NewHealthHandler(pool)
	solveHandler := handlers.NewSolveHandler(pool, sessions, detector, solverChain, cfg, logger)

	// Create router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(idleMonitor.Middleware) // Track requests for scale-to-zero
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(cfg.ChallengeTimeout + 30*time.Second))

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Rate limiting (optional) - can be enabled via reverse proxy
	_ = httprate.LimitByIP // imported for future use

	// Auth middleware (applied to protected routes)
	// Only enable if at least one auth method is configured AND not in standalone mode
	authEnabled := cfg.RefyneAPISecret != "" || clerkVerifier != nil
	if cfg.AllowUnauthenticated {
		logger.Info("running in standalone mode (FlareSolverr-compatible) - authentication disabled",
			"hint", "set ALLOW_UNAUTHENTICATED=false for production use",
		)
	} else if authEnabled {
		logger.Info("authentication middleware enabled",
			"has_refyne_secret", cfg.RefyneAPISecret != "",
			"has_clerk_verifier", clerkVerifier != nil,
			"required_feature", cfg.RequiredFeature,
		)
		// Apply auth middleware BEFORE routes are registered
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				// Skip auth for health, OpenAPI docs
				if req.URL.Path == "/health" || req.URL.Path == "/openapi.json" ||
					req.URL.Path == "/openapi.yaml" || req.URL.Path == "/docs" {
					next.ServeHTTP(w, req)
					return
				}
				mw.Auth(authConfig)(next).ServeHTTP(w, req)
			})
		})
	} else {
		logger.Warn("no authentication configured - service is unprotected",
			"hint", "set REFYNE_API_SECRET or CLERK_ISSUER for production, or ALLOW_UNAUTHENTICATED=true for local dev",
		)
	}

	// Create Huma API with OpenAPI documentation
	humaConfig := huma.DefaultConfig("Captcha Server", version.Get().Version)
	humaConfig.Info.Description = "FlareSolverr-compatible CAPTCHA solving service with anti-bot bypass capabilities"
	humaConfig.Info.Contact = &huma.Contact{
		Name: "Refyne",
		URL:  "https://github.com/jmylchreest/refyne-api",
	}
	humaConfig.Info.License = &huma.License{
		Name: "MIT",
	}
	humaConfig.Servers = []*huma.Server{
		{URL: fmt.Sprintf("http://localhost:%d", cfg.Port), Description: "Local development"},
	}
	// Huma auto-exposes /openapi.json, /openapi.yaml, and /docs
	api := humachi.New(r, humaConfig)

	// Register health endpoint (no auth required)
	huma.Register(api, huma.Operation{
		OperationID: "getHealth",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Health check",
		Description: "Returns health status, version, and browser pool statistics",
		Tags:        []string{"Health"},
	}, func(ctx context.Context, input *struct{}) (*handlers.HealthOutput, error) {
		resp := healthHandler.Handle(ctx)
		return &handlers.HealthOutput{Body: *resp}, nil
	})

	// Register FlareSolverr-compatible endpoint
	huma.Register(api, huma.Operation{
		OperationID:   "solve",
		Method:        http.MethodPost,
		Path:          "/v1",
		Summary:       "Solve challenge",
		Description:   "FlareSolverr-compatible endpoint for solving anti-bot challenges. Supports session management, CAPTCHA solving, and Cloudflare bypass.",
		Tags:          []string{"Solve"},
		DefaultStatus: http.StatusOK,
	}, func(ctx context.Context, input *SolveInput) (*SolveOutput, error) {
		resp := solveHandler.Handle(ctx, &input.Body)
		return &SolveOutput{Body: *resp}, nil
	})

	// Create HTTP server
	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: cfg.ChallengeTimeout + 60*time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info("server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal (SIGTERM or idle timeout)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	var idleShutdown bool
	select {
	case <-quit:
		logger.Info("received shutdown signal")
	case <-idleMonitor.ShutdownChan():
		idleShutdown = true
		logger.Info("idle shutdown triggered")
	}

	// Cancel context to stop background tasks
	cancel()

	// Determine shutdown timeout:
	// - Idle shutdown: fast exit (we know active requests = 0)
	// - Signal shutdown: allow time for in-flight requests
	shutdownTimeout := 30 * time.Second
	if idleShutdown {
		// Fast shutdown - idle monitor already confirmed 0 active requests
		shutdownTimeout = 2 * time.Second
	}

	logger.Info("shutting down server...", "timeout", shutdownTimeout, "idle_triggered", idleShutdown)

	// Graceful shutdown - immediately stops accepting new connections
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
	}

	logger.Info("server stopped")
}

// SolveInput is the input for solve requests.
type SolveInput struct {
	Body models.SolveRequest
}

// SolveOutput is the output for solve requests.
type SolveOutput struct {
	Body models.SolveResponse
}
