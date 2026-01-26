package handlers

import (
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// ConvertCleanerChain converts handler cleaner chain input to service cleaner chain.
// This is the single source of truth for cleaner chain conversion.
func ConvertCleanerChain(input []CleanerConfigInput) []service.CleanerConfig {
	if len(input) == 0 {
		return nil
	}

	chain := make([]service.CleanerConfig, len(input))
	for i, c := range input {
		chain[i] = service.CleanerConfig{Name: c.Name}
		if c.Options != nil {
			chain[i].Options = &service.CleanerOptions{
				Output:             c.Options.Output,
				BaseURL:            c.Options.BaseURL,
				Preset:             c.Options.Preset,
				RemoveSelectors:    c.Options.RemoveSelectors,
				KeepSelectors:      c.Options.KeepSelectors,
				IncludeFrontmatter: c.Options.IncludeFrontmatter,
				ExtractImages:      c.Options.ExtractImages,
				ExtractHeadings:    c.Options.ExtractHeadings,
				ResolveURLs:        c.Options.ResolveURLs,
			}
		}
	}
	return chain
}

// ConvertJobCleanerChain converts job handler cleaner chain input to service cleaner chain.
// This handles the JobCleanerConfigInput type used in the jobs handler.
func ConvertJobCleanerChain(input []JobCleanerConfigInput) []service.CleanerConfig {
	if len(input) == 0 {
		return nil
	}

	chain := make([]service.CleanerConfig, len(input))
	for i, c := range input {
		chain[i] = service.CleanerConfig{Name: c.Name}
		if c.Options != nil {
			chain[i].Options = &service.CleanerOptions{
				Output:             c.Options.Output,
				BaseURL:            c.Options.BaseURL,
				Preset:             c.Options.Preset,
				RemoveSelectors:    c.Options.RemoveSelectors,
				KeepSelectors:      c.Options.KeepSelectors,
				IncludeFrontmatter: c.Options.IncludeFrontmatter,
				ExtractImages:      c.Options.ExtractImages,
				ExtractHeadings:    c.Options.ExtractHeadings,
				ResolveURLs:        c.Options.ResolveURLs,
			}
		}
	}
	return chain
}

// BuildExtractContext creates an ExtractContext from UserContext and optional LLM config.
func BuildExtractContext(uc UserContext, llmConfig *LLMConfigInput) *service.ExtractContext {
	ectx := &service.ExtractContext{
		UserID:                 uc.UserID,
		Tier:                   uc.Tier,
		BYOKAllowed:            uc.BYOKAllowed,
		ModelsCustomAllowed:    uc.ModelsCustomAllowed,
		ModelsPremiumAllowed:   uc.ModelsPremiumAllowed,
		ContentDynamicAllowed:  uc.ContentDynamicAllowed,
		SkipCreditCheckAllowed: uc.SkipCreditCheckAllowed,
		LLMProvider:            uc.LLMProvider,
		LLMModel:               uc.LLMModel,
	}

	if llmConfig != nil && llmConfig.APIKey != "" {
		ectx.IsBYOK = true
	}

	return ectx
}

// BuildEphemeralWebhook creates a WebhookConfig from inline webhook input.
// Returns nil if no webhook is configured.
func BuildEphemeralWebhook(inline *InlineWebhookInput, legacyURL string) *service.WebhookConfig {
	if inline != nil && inline.URL != "" {
		return &service.WebhookConfig{
			URL:     inline.URL,
			Secret:  inline.Secret,
			Events:  inline.Events,
			Headers: convertWebhookHeaders(inline.Headers),
		}
	}
	if legacyURL != "" {
		return &service.WebhookConfig{
			URL:    legacyURL,
			Events: []string{"*"},
		}
	}
	return nil
}

// BuildCrawlEphemeralWebhook creates a WebhookConfig from crawl inline webhook input.
// Returns nil if no webhook is configured.
func BuildCrawlEphemeralWebhook(inline *CrawlInlineWebhookInput, legacyURL string) *service.WebhookConfig {
	if inline != nil && inline.URL != "" {
		return &service.WebhookConfig{
			URL:     inline.URL,
			Secret:  inline.Secret,
			Events:  inline.Events,
			Headers: convertCrawlWebhookHeaders(inline.Headers),
		}
	}
	if legacyURL != "" {
		return &service.WebhookConfig{
			URL:    legacyURL,
			Events: []string{"*"},
		}
	}
	return nil
}

// convertWebhookHeaders converts handler webhook headers to model headers.
func convertWebhookHeaders(input []WebhookHeaderInput) []models.Header {
	if len(input) == 0 {
		return nil
	}
	headers := make([]models.Header, len(input))
	for i, h := range input {
		headers[i] = models.Header{Name: h.Name, Value: h.Value}
	}
	return headers
}

// convertCrawlWebhookHeaders converts crawl handler webhook headers to model headers.
func convertCrawlWebhookHeaders(input []CrawlWebhookHeaderInput) []models.Header {
	if len(input) == 0 {
		return nil
	}
	headers := make([]models.Header, len(input))
	for i, h := range input {
		headers[i] = models.Header{Name: h.Name, Value: h.Value}
	}
	return headers
}

// ConvertLLMConfig converts handler LLM config input to service LLM config.
func ConvertLLMConfig(input *LLMConfigInput) *service.LLMConfigInput {
	if input == nil {
		return nil
	}
	return &service.LLMConfigInput{
		Provider: input.Provider,
		APIKey:   input.APIKey,
		BaseURL:  input.BaseURL,
		Model:    input.Model,
	}
}

// IsBYOKFromLLMConfig determines if the LLM config represents BYOK (bring your own key).
func IsBYOKFromLLMConfig(input *LLMConfigInput) bool {
	return input != nil && input.APIKey != ""
}
