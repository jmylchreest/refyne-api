// Package service contains the business logic layer.
// Note: User management, OAuth, sessions, subscriptions, and billing are handled by Clerk.
// The UserID in services references Clerk user IDs (e.g., "user_xxx").
package service

import (
	"fmt"
	"log/slog"

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
}

// NewServices creates all service instances.
func NewServices(cfg *config.Config, repos *repository.Repositories, logger *slog.Logger) (*Services, error) {
	authSvc := NewAuthService(cfg, repos, logger)
	jobSvc := NewJobService(cfg, repos, logger)
	apiKeySvc := NewAPIKeyService(repos, logger)
	usageSvc := NewUsageService(repos, logger)
	webhookSvc := NewWebhookService(logger)
	schemaSvc := NewSchemaService(repos, logger)

	// Create billing config with defaults
	billingCfg := config.DefaultBillingConfig()
	balanceSvc := NewBalanceService(repos, &billingCfg, logger)

	// Create OpenRouter client for cost tracking (if we have a service key)
	var orClient *llm.OpenRouterClient
	if cfg.ServiceOpenRouterKey != "" {
		orClient = llm.NewOpenRouterClient(cfg.ServiceOpenRouterKey)
	}

	// Create billing service
	billingSvc := NewBillingService(repos, &billingCfg, orClient, logger)

	// Create extraction service with billing dependency
	extractionSvc := NewExtractionServiceWithBilling(cfg, repos, billingSvc, logger)

	llmConfigSvc, err := NewLLMConfigService(cfg, repos, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM config service: %w", err)
	}

	// Create encryptor for admin service (to encrypt service keys)
	var encryptor *crypto.Encryptor
	if len(cfg.EncryptionKey) > 0 {
		encryptor, _ = crypto.NewEncryptor(cfg.EncryptionKey)
	}
	adminSvc := NewAdminServiceWithClerk(repos, encryptor, cfg.ClerkSecretKey, logger)

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
	}, nil
}
