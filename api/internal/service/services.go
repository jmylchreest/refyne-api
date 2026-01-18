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
	Auth       *AuthService
	Extraction *ExtractionService
	Job        *JobService
	APIKey     *APIKeyService
	Usage      *UsageService
	LLMConfig  *LLMConfigService
	Webhook    *WebhookService
	Balance    *BalanceService
	Schema     *SchemaService
	Billing    *BillingService
	Admin      *AdminService
	Analyzer   *AnalyzerService
	Storage    *StorageService
	UserLLM    *UserLLMService
	Sitemap    *SitemapService
	Pricing    *PricingService
	TierSync   *TierSyncService
}

// NewServices creates all service instances.
func NewServices(cfg *config.Config, repos *repository.Repositories, logger *slog.Logger) (*Services, error) {
	authSvc := NewAuthService(cfg, repos, logger)
	jobSvc := NewJobService(cfg, repos, logger)
	apiKeySvc := NewAPIKeyService(repos, logger)
	usageSvc := NewUsageService(repos, logger)
	schemaSvc := NewSchemaService(repos, logger)

	// Create billing config with defaults
	billingCfg := config.DefaultBillingConfig()
	balanceSvc := NewBalanceService(repos, &billingCfg, logger)

	// Create OpenRouter client for cost tracking (if we have a service key)
	var orClient *llm.OpenRouterClient
	if cfg.ServiceOpenRouterKey != "" {
		orClient = llm.NewOpenRouterClient(cfg.ServiceOpenRouterKey)
	}

	// Create pricing service for model cost estimation
	pricingSvc := NewPricingService(PricingServiceConfig{
		OpenRouterAPIKey: cfg.ServiceOpenRouterKey,
	}, logger)

	// Create billing service
	billingSvc := NewBillingService(repos, &billingCfg, orClient, pricingSvc, logger)

	// Create extraction service with billing dependency
	extractionSvc := NewExtractionServiceWithBilling(cfg, repos, billingSvc, logger)

	llmConfigSvc, err := NewLLMConfigService(cfg, repos, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM config service: %w", err)
	}

	// Create encryptor for admin service and user LLM service (to encrypt service keys)
	var encryptor *crypto.Encryptor
	if len(cfg.EncryptionKey) > 0 {
		encryptor, _ = crypto.NewEncryptor(cfg.EncryptionKey)
	}

	// Create webhook service with tracking and encryption support
	webhookSvc := NewWebhookService(logger, repos.Webhook, repos.WebhookDelivery, encryptor)

	adminSvc := NewAdminServiceWithClerk(repos, encryptor, cfg.ClerkSecretKey, logger)
	analyzerSvc := NewAnalyzerServiceWithBilling(cfg, repos, billingSvc, logger)
	userLLMSvc := NewUserLLMService(repos, encryptor, logger)

	// Create storage service (Tigris/S3-compatible)
	storageSvc, err := NewStorageService(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage service: %w", err)
	}

	// Create sitemap service for URL discovery
	sitemapSvc := NewSitemapService(logger)

	// Create tier sync service for syncing visibility/display names from Clerk
	var tierSyncSvc *TierSyncService
	if cfg.ClerkSecretKey != "" {
		clerkClient := auth.NewClerkBackendClient(cfg.ClerkSecretKey)
		tierSyncSvc = NewTierSyncService(clerkClient, logger)

		// Sync tier metadata from Clerk on startup
		if err := tierSyncSvc.SyncFromClerk(context.Background()); err != nil {
			// Log but don't fail startup - we have hardcoded defaults
			logger.Warn("failed to sync tier metadata from Clerk on startup", "error", err)
		}
	}

	return &Services{
		Auth:       authSvc,
		Extraction: extractionSvc,
		Job:        jobSvc,
		APIKey:     apiKeySvc,
		Usage:      usageSvc,
		LLMConfig:  llmConfigSvc,
		Webhook:    webhookSvc,
		Balance:    balanceSvc,
		Schema:     schemaSvc,
		Billing:    billingSvc,
		Admin:      adminSvc,
		Analyzer:   analyzerSvc,
		Storage:    storageSvc,
		UserLLM:    userLLMSvc,
		Sitemap:    sitemapSvc,
		Pricing:    pricingSvc,
		TierSync:   tierSyncSvc,
	}, nil
}

