// Package main provides the entry point for the captcha server.
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

	"github.com/jmylchreest/refyne-api/captcha/internal/api/handlers"
	"github.com/jmylchreest/refyne-api/captcha/internal/auth"
	"github.com/jmylchreest/refyne-api/captcha/internal/browser"
	"github.com/jmylchreest/refyne-api/captcha/internal/challenge"
	"github.com/jmylchreest/refyne-api/captcha/internal/config"
	"github.com/jmylchreest/refyne-api/captcha/internal/http/mw"
	"github.com/jmylchreest/refyne-api/captcha/internal/logging"
	"github.com/jmylchreest/refyne-api/captcha/internal/models"
	"github.com/jmylchreest/refyne-api/captcha/internal/session"
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

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize browser pool (browsers are created on-demand)
	pool := browser.NewPool(cfg, logger)
	defer pool.Close()

	// Initialize session manager
	sessions := session.NewManager(cfg, logger)
	defer sessions.Close()
	go sessions.StartCleanup(ctx)

	// Initialize challenge detector
	detector := challenge.NewDetector()

	// Initialize solver chain
	var solverChain solver.Solver
	solvers := make([]solver.Solver, 0)

	if cfg.TwoCaptchaAPIKey != "" {
		logger.Info("2Captcha solver enabled")
		solvers = append(solvers, solver.NewTwoCaptcha(cfg.TwoCaptchaAPIKey))
	}

	if cfg.CapSolverAPIKey != "" {
		logger.Info("CapSolver enabled (not yet implemented)")
		// TODO: Add CapSolver implementation
	}

	if len(solvers) > 0 {
		solverChain = solver.NewChain(solvers...)
	}

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
	// Only enable if at least one auth method is configured
	authEnabled := cfg.RefyneAPISecret != "" || clerkVerifier != nil
	if authEnabled && !cfg.AllowUnauthenticated {
		logger.Info("authentication middleware enabled",
			"has_refyne_secret", cfg.RefyneAPISecret != "",
			"has_clerk_verifier", clerkVerifier != nil,
			"required_feature", cfg.RequiredFeature,
		)
	} else if cfg.AllowUnauthenticated {
		logger.Warn("authentication disabled - ALLOW_UNAUTHENTICATED is set")
	} else {
		logger.Warn("no authentication configured - service is unprotected")
	}

	// Create Huma API
	humaConfig := huma.DefaultConfig("Captcha Server", version.Get().Version)
	humaConfig.Info.Description = "FlareSolverr-compatible CAPTCHA solving service"
	api := humachi.New(r, humaConfig)

	// Register health endpoint (no auth required)
	huma.Register(api, huma.Operation{
		OperationID: "health",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Health check",
		Description: "Returns health status and pool statistics",
		Tags:        []string{"Health"},
	}, func(ctx context.Context, input *struct{}) (*handlers.HealthOutput, error) {
		resp := healthHandler.Handle(ctx)
		return &handlers.HealthOutput{Body: *resp}, nil
	})

	// Protected routes - apply auth middleware if configured
	protectedRouter := chi.NewRouter()
	if authEnabled && !cfg.AllowUnauthenticated {
		protectedRouter.Use(mw.Auth(authConfig))
	}

	// Create Huma API for protected routes
	protectedAPI := humachi.New(protectedRouter, humaConfig)

	// Register FlareSolverr-compatible endpoint
	huma.Register(protectedAPI, huma.Operation{
		OperationID: "solve",
		Method:      http.MethodPost,
		Path:        "/v1",
		Summary:     "Solve request",
		Description: "FlareSolverr-compatible endpoint for solving challenges",
		Tags:        []string{"Solve"},
	}, func(ctx context.Context, input *SolveInput) (*SolveOutput, error) {
		resp := solveHandler.Handle(ctx, &input.Body)
		return &SolveOutput{Body: *resp}, nil
	})

	// Also register at root for FlareSolverr compatibility
	huma.Register(protectedAPI, huma.Operation{
		OperationID: "solveRoot",
		Method:      http.MethodPost,
		Path:        "/",
		Summary:     "Solve request (root)",
		Description: "FlareSolverr-compatible endpoint at root path",
		Tags:        []string{"Solve"},
	}, func(ctx context.Context, input *SolveInput) (*SolveOutput, error) {
		resp := solveHandler.Handle(ctx, &input.Body)
		return &SolveOutput{Body: *resp}, nil
	})

	// Mount protected routes on main router
	r.Mount("/", protectedRouter)

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

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	// Cancel context to stop background tasks
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
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
