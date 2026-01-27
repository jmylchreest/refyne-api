package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// LLMCallOptions configures an LLM API call.
type LLMCallOptions struct {
	Temperature float64 // Default: 0.2
	MaxTokens   int     // Default: 4096
	Timeout     time.Duration // Default: 120s
	JSONMode    bool    // Request JSON response format (OpenAI/OpenRouter only)
}

// DefaultLLMCallOptions returns sensible defaults for LLM calls.
func DefaultLLMCallOptions() LLMCallOptions {
	return LLMCallOptions{
		Temperature: 0.2,
		MaxTokens:   4096,
		Timeout:     120 * time.Second,
		JSONMode:    false,
	}
}

// LLMCallResult holds the result of an LLM API call including token usage.
type LLMCallResult struct {
	Content      string
	InputTokens  int
	OutputTokens int
}

// LLMClient handles direct LLM API calls.
type LLMClient struct {
	logger *slog.Logger
}

// NewLLMClient creates a new LLM client.
func NewLLMClient(logger *slog.Logger) *LLMClient {
	return &LLMClient{logger: logger}
}

// Call makes a direct call to an LLM API and returns the response with token usage.
func (c *LLMClient) Call(ctx context.Context, config *LLMConfigInput, prompt string, opts LLMCallOptions) (*LLMCallResult, error) {
	// Validate config
	if config.APIKey == "" && config.Provider != "ollama" {
		return nil, fmt.Errorf("no API key available for provider %s", config.Provider)
	}

	// Apply defaults for zero values
	if opts.Temperature == 0 {
		opts.Temperature = 0.2
	}
	if opts.MaxTokens == 0 {
		opts.MaxTokens = 4096
	}
	if opts.Timeout == 0 {
		opts.Timeout = 120 * time.Second
	}

	// Build request body
	reqBody := map[string]any{
		"model": config.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": opts.Temperature,
		"max_tokens":  opts.MaxTokens,
	}

	// Add JSON mode if requested
	// The caller is responsible for checking model capabilities via SupportsResponseFormat()
	// This parameter is supported by OpenAI-compatible APIs (OpenAI, OpenRouter)
	// Anthropic uses tool_use for structured outputs, Ollama varies by model
	if opts.JSONMode {
		switch config.Provider {
		case "openai", "openrouter":
			reqBody["response_format"] = map[string]string{"type": "json_object"}
		// Anthropic and Ollama don't use response_format - they have different mechanisms
		// If JSONMode is requested for these, we rely on the prompt instructions
		}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Determine API endpoint
	apiURL := c.getAPIURL(config)

	if c.logger != nil {
		c.logger.Debug("making LLM API request",
			"provider", config.Provider,
			"model", config.Model,
			"api_url", apiURL,
			"prompt_length", len(prompt),
			"temperature", opts.Temperature,
			"max_tokens", opts.MaxTokens,
		)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.setAuthHeaders(req, config)

	client := &http.Client{Timeout: opts.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("LLM API request failed", "provider", config.Provider, "error", err)
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if c.logger != nil {
		c.logger.Debug("LLM API response received",
			"provider", config.Provider,
			"status_code", resp.StatusCode,
			"response_length", len(body),
		)
	}

	if resp.StatusCode != http.StatusOK {
		if c.logger != nil {
			c.logger.Error("LLM API error",
				"provider", config.Provider,
				"status_code", resp.StatusCode,
				"response", string(body),
			)
		}
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response based on provider
	return c.ParseResponse(config.Provider, body)
}

// getAPIURL returns the API endpoint for a provider.
func (c *LLMClient) getAPIURL(config *LLMConfigInput) string {
	switch config.Provider {
	case "openrouter":
		return "https://openrouter.ai/api/v1/chat/completions"
	case "anthropic":
		return "https://api.anthropic.com/v1/messages"
	case "openai":
		return "https://api.openai.com/v1/chat/completions"
	case "ollama":
		baseURL := config.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		return baseURL + "/api/chat"
	default:
		return "https://openrouter.ai/api/v1/chat/completions"
	}
}

// setAuthHeaders sets appropriate authentication headers for the provider.
func (c *LLMClient) setAuthHeaders(req *http.Request, config *LLMConfigInput) {
	switch config.Provider {
	case "openrouter":
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
		req.Header.Set("HTTP-Referer", "https://refyne.io")
		req.Header.Set("X-Title", "Refyne")
	case "anthropic":
		req.Header.Set("x-api-key", config.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
}

// ParseResponse extracts the text response and token usage from different LLM provider formats.
// Exported for testing.
func (c *LLMClient) ParseResponse(provider string, body []byte) (*LLMCallResult, error) {
	result := &LLMCallResult{}

	switch provider {
	case "anthropic":
		var resp struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse Anthropic response: %w", err)
		}
		if len(resp.Content) == 0 {
			return nil, fmt.Errorf("empty response from LLM")
		}
		result.Content = resp.Content[0].Text
		result.InputTokens = resp.Usage.InputTokens
		result.OutputTokens = resp.Usage.OutputTokens

	case "ollama":
		var resp struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			PromptEvalCount int `json:"prompt_eval_count"`
			EvalCount       int `json:"eval_count"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
		}
		result.Content = resp.Message.Content
		result.InputTokens = resp.PromptEvalCount
		result.OutputTokens = resp.EvalCount

	default: // OpenAI-compatible (OpenRouter, OpenAI)
		var resp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
		}
		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("empty response from LLM")
		}
		result.Content = resp.Choices[0].Message.Content
		result.InputTokens = resp.Usage.PromptTokens
		result.OutputTokens = resp.Usage.CompletionTokens
	}

	return result, nil
}
