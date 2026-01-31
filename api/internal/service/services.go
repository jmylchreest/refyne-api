// Package service contains the business logic layer.
// Note: User management, OAuth, sessions, subscriptions, and billing are handled by Clerk.
// The UserID in services references Clerk user IDs (e.g., "user_xxx").
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/refyne-api/internal/auth"
	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// Services holds all service instances.
type Services struct {
	Auth              *AuthService
	Extraction        *ExtractionService
	Job               *JobService
	APIKey            *APIKeyService
	Usage             *UsageService
	Webhook           *WebhookService
	Balance           *BalanceService
	Schema            *SchemaService
	Billing           *BillingService
	Admin             *AdminService
	Analyzer          *AnalyzerService
	Storage           *StorageService
	UserLLM           *UserLLMService
	Sitemap           *SitemapService
	Pricing           *PricingService
	TierSync          *TierSyncService
	LLMConfigResolver *LLMConfigResolver
	Captcha           *CaptchaService // For dynamic content fetching with browser rendering
	SubscriptionCache *auth.SubscriptionCache // For API key tier/feature hydration from Clerk
}

// NewServices creates all service instances.
func NewServices(cfg *config.Config, repos *repository.Repositories, logger *slog.Logger) (*Services, error) {
	// Create encryptor first - needed by multiple services for BYOK key encryption
	var encryptor *crypto.Encryptor
	if len(cfg.EncryptionKey) > 0 {
		var err error
		encryptor, err = crypto.NewEncryptor(cfg.EncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create encryptor: %w", err)
		}
	} else {
		logger.Warn("no encryption key configured - BYOK feature will be unavailable")
	}

	// Create storage service first (needed by JobService for S3 result storage)
	storageSvc, err := NewStorageService(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage service: %w", err)
	}

	authSvc := NewAuthService(cfg, repos, logger)
	jobSvc := NewJobService(cfg, repos, storageSvc, logger)
	apiKeySvc := NewAPIKeyService(repos, logger)
	usageSvc := NewUsageService(repos, logger)
	schemaSvc := NewSchemaService(repos, logger)

	// Create billing config with defaults
	billingCfg := config.DefaultBillingConfig()
	balanceSvc := NewBalanceService(repos, &billingCfg, logger)

	// Create pricing service for model cost estimation (uses refyne's provider interfaces)
	pricingSvc := NewPricingService(PricingServiceConfig{
		OpenRouterAPIKey: cfg.ServiceOpenRouterKey,
	}, logger)

	// Create billing service
	billingSvc := NewBillingService(repos, &billingCfg, pricingSvc, logger)

	// Create shared LLM config resolver (used by extraction and analyzer services)
	llmResolver := NewLLMConfigResolver(cfg, repos, encryptor, logger)

	// Set up dynamic key resolver for pricing service
	// This allows pricing service to get fresh keys from DB on each refresh attempt
	pricingSvc.SetKeyResolver(func(ctx context.Context) string {
		keys := llmResolver.GetServiceKeys(ctx)
		return keys.OpenRouterKey
	})

	// Also try to set static key now as initial value (may be empty at startup)
	serviceKeys := llmResolver.GetServiceKeys(context.Background())
	if serviceKeys.Has(llm.ProviderOpenRouter) {
		openRouterKey := serviceKeys.Get(llm.ProviderOpenRouter)
		pricingSvc.SetOpenRouterAPIKey(openRouterKey)
		logger.Info("pricing service initialized with OpenRouter key",
			"key_source", "resolver (DB or env)",
			"key_length", len(openRouterKey),
		)
	} else {
		logger.Info("pricing service will resolve OpenRouter key dynamically on first use")
	}

	// Create extraction and analyzer services with resolver dependency
	extractionSvc := NewExtractionServiceWithBilling(cfg, repos, billingSvc, llmResolver, encryptor, logger)
	analyzerSvc := NewAnalyzerServiceWithBilling(cfg, repos, billingSvc, llmResolver, logger)

	// Create webhook service with tracking and encryption support
	webhookSvc := NewWebhookService(logger, repos.Webhook, repos.WebhookDelivery, encryptor)

	adminSvc := NewAdminServiceWithClerk(repos, encryptor, cfg.ClerkSecretKey, logger)
	userLLMSvc := NewUserLLMService(repos, encryptor, logger)

	// Create sitemap service for URL discovery
	sitemapSvc := NewSitemapService(logger)

	// Create captcha service for dynamic content fetching (browser rendering)
	// Only initialized when CAPTCHA_SERVICE_URL is configured
	var captchaSvc *CaptchaService
	if cfg.CaptchaEnabled() {
		captchaSvc = NewCaptchaService(CaptchaServiceConfig{
			ServiceURL: cfg.CaptchaServiceURL,
			Secret:     cfg.CaptchaSecret,
			Logger:     logger,
		})
		// Wire captcha service to extraction and analyzer services for dynamic fetch mode
		extractionSvc.SetCaptchaService(captchaSvc)
		analyzerSvc.SetCaptchaService(captchaSvc)
		logger.Info("captcha service enabled for dynamic content fetching",
			"service_url", cfg.CaptchaServiceURL,
		)
	} else {
		logger.Warn("captcha service NOT configured - dynamic fetch mode unavailable",
			"captcha_url_set", cfg.CaptchaServiceURL != "",
			"captcha_secret_set", cfg.CaptchaSecret != "",
		)
	}

	// Create Clerk-dependent services
	var tierSyncSvc *TierSyncService
	var subscriptionCache *auth.SubscriptionCache
	if cfg.ClerkSecretKey != "" {
		clerkClient := auth.NewClerkBackendClient(cfg.ClerkSecretKey)
		tierSyncSvc = NewTierSyncService(clerkClient, logger)

		// Sync tier metadata from Clerk asynchronously (non-blocking startup)
		// We have hardcoded defaults, so the API will work even if this fails
		go func() {
			if err := tierSyncSvc.SyncFromClerk(context.Background()); err != nil {
				logger.Warn("failed to sync tier metadata from Clerk on startup", "error", err)
			} else {
				logger.Info("tier metadata synced from Clerk")
			}
		}()

		// Create subscription cache for API key tier/feature hydration
		// Uses 5-minute TTL to balance freshness with API rate limits
		subscriptionCache = auth.NewSubscriptionCache(clerkClient, auth.DefaultSubscriptionCacheTTL, logger)
		logger.Info("subscription cache enabled for API key auth", "ttl", auth.DefaultSubscriptionCacheTTL)
	}

	return &Services{
		Auth:              authSvc,
		Extraction:        extractionSvc,
		Job:               jobSvc,
		APIKey:            apiKeySvc,
		Usage:             usageSvc,
		Webhook:           webhookSvc,
		Balance:           balanceSvc,
		Schema:            schemaSvc,
		Billing:           billingSvc,
		Admin:             adminSvc,
		Analyzer:          analyzerSvc,
		Storage:           storageSvc,
		UserLLM:           userLLMSvc,
		Sitemap:           sitemapSvc,
		Pricing:           pricingSvc,
		TierSync:          tierSyncSvc,
		LLMConfigResolver: llmResolver,
		Captcha:           captchaSvc,
		SubscriptionCache: subscriptionCache,
	}, nil
}

