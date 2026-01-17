package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// OpenRouterAPIBase is the base URL for OpenRouter API.
	OpenRouterAPIBase = "https://openrouter.ai/api/v1"

	// DefaultTimeout for HTTP requests (cost tracking API).
	DefaultTimeout = 30 * time.Second

	// LLMTimeout for LLM completion requests (longer for free models under load).
	LLMTimeout = 120 * time.Second
)

// OpenRouterClient provides access to OpenRouter API for cost tracking.
type OpenRouterClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewOpenRouterClient creates a new OpenRouter client.
func NewOpenRouterClient(apiKey string) *OpenRouterClient {
	return &OpenRouterClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// GenerationStats represents the response from OpenRouter's generation endpoint.
type GenerationStats struct {
	// ID is the generation ID.
	ID string `json:"id"`

	// Model used for the generation.
	Model string `json:"model"`

	// TotalCost is the total cost in USD with native tokenizer accuracy.
	TotalCost float64 `json:"total_cost"`

	// NativeTokensPrompt is the number of prompt tokens (provider's count).
	NativeTokensPrompt int `json:"native_tokens_prompt"`

	// NativeTokensCompletion is the number of completion tokens (provider's count).
	NativeTokensCompletion int `json:"native_tokens_completion"`

	// ProviderName is the name of the provider that served the request.
	ProviderName string `json:"provider_name"`

	// Generation time in seconds.
	GenerationTime float64 `json:"generation_time"`

	// CreatedAt is when the generation was created.
	CreatedAt string `json:"created_at"`

	// Status of the generation (completed, failed, etc.)
	Status string `json:"status"`

	// Error message if generation failed.
	Error string `json:"error,omitempty"`
}

// GenerationResponse is the wrapper for generation stats response.
type GenerationResponse struct {
	Data GenerationStats `json:"data"`
}

// GetGenerationStats retrieves stats for a specific generation by ID.
// This endpoint returns the actual USD cost with native tokenizer accuracy.
func (c *OpenRouterClient) GetGenerationStats(ctx context.Context, generationID string) (*GenerationStats, error) {
	url := fmt.Sprintf("%s/generation?id=%s", OpenRouterAPIBase, generationID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result GenerationResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result.Data, nil
}

// GetGenerationStatsWithRetry retrieves generation stats with retry logic.
// OpenRouter may take a moment to compute stats after a generation completes.
func (c *OpenRouterClient) GetGenerationStatsWithRetry(ctx context.Context, generationID string, maxRetries int, delay time.Duration) (*GenerationStats, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		stats, err := c.GetGenerationStats(ctx, generationID)
		if err == nil {
			return stats, nil
		}

		lastErr = err

		// Wait before retrying
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			// Continue to next retry
		}
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// EstimateCost provides a rough cost estimate based on token counts.
// This is used as a fallback when generation stats aren't available.
// Prices are approximate and may vary by model.
func EstimateCost(tokensInput, tokensOutput int, model string) float64 {
	// Approximate pricing (USD per 1M tokens) - conservative estimates
	// These should be updated periodically based on OpenRouter pricing
	inputPricePer1M := 0.25   // Default input price
	outputPricePer1M := 1.00  // Default output price

	// Adjust for known model pricing tiers
	switch {
	case containsAny(model, "gpt-4", "claude-3-opus"):
		inputPricePer1M = 15.0
		outputPricePer1M = 60.0
	case containsAny(model, "gpt-3.5", "claude-3-sonnet", "claude-3.5"):
		inputPricePer1M = 3.0
		outputPricePer1M = 15.0
	case containsAny(model, "gpt-4o-mini", "claude-3-haiku"):
		inputPricePer1M = 0.15
		outputPricePer1M = 0.60
	case containsAny(model, "llama", "mixtral", "gemma"):
		inputPricePer1M = 0.10
		outputPricePer1M = 0.40
	case containsAny(model, ":free"):
		// Free models have no cost
		return 0
	}

	inputCost := float64(tokensInput) * inputPricePer1M / 1_000_000
	outputCost := float64(tokensOutput) * outputPricePer1M / 1_000_000

	return inputCost + outputCost
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if containsStr(s, sub) {
			return true
		}
	}
	return false
}

// containsStr checks if s contains substr.
func containsStr(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
