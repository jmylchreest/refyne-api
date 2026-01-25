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

	// Update pricing service with OpenRouter key from resolver (DB takes precedence over env)
	serviceKeys := llmResolver.GetServiceKeys(context.Background())
	if serviceKeys.OpenRouterKey != "" {
		pricingSvc.SetOpenRouterAPIKey(serviceKeys.OpenRouterKey)
		logger.Info("pricing service initialized with OpenRouter key from resolver",
			"key_source", "resolver (DB or env)",
			"key_length", len(serviceKeys.OpenRouterKey),
		)
	} else {
		logger.Warn("pricing service has no OpenRouter key - cost lookups will require key from resolved LLM config",
			"env_key_set", cfg.ServiceOpenRouterKey != "",
		)
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
		// Wire captcha service to extraction service for dynamic fetch mode
		extractionSvc.SetCaptchaService(captchaSvc)
		logger.Info("captcha service enabled for dynamic content fetching",
			"service_url", cfg.CaptchaServiceURL,
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

