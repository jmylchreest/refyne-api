// Model default settings for temperature, max_tokens, and strict mode
// These match the backend defaults in internal/llm/defaults.go
// Full model defaults are loaded from S3: config/model_defaults.json

export interface ModelSettings {
  temperature: number;
  maxTokens: number;
  strictMode: boolean; // Whether model supports strict JSON schema validation
}

// Provider defaults - used when a model is not in the overrides map
// strictMode: true only for OpenAI native (not via OpenRouter by default)
export const PROVIDER_DEFAULTS: Record<string, ModelSettings> = {
  anthropic: { temperature: 0.2, maxTokens: 8192, strictMode: false },
  openai: { temperature: 0.2, maxTokens: 8192, strictMode: true },
  openrouter: { temperature: 0.2, maxTokens: 6144, strictMode: false },
  ollama: { temperature: 0.1, maxTokens: 4096, strictMode: false },
  helicone: { temperature: 0.2, maxTokens: 8192, strictMode: true }, // Helicone proxies OpenAI-compatible APIs
};

// Model-specific overrides for models that need different defaults
// strictMode: true = supports OpenAI structured outputs with strict validation
// Most models via OpenRouter don't support strict mode - only OpenAI gpt-4o models do
export const MODEL_OVERRIDES: Record<string, ModelSettings> = {
  // OpenRouter models - most don't support strict mode
  'meta-llama/llama-3.1-70b-instruct': { temperature: 0.1, maxTokens: 4096, strictMode: false },
  'meta-llama/llama-3.1-8b-instruct': { temperature: 0.1, maxTokens: 4096, strictMode: false },
  'meta-llama/llama-3.2-90b-instruct': { temperature: 0.1, maxTokens: 4096, strictMode: false },
  'mistralai/mistral-large': { temperature: 0.1, maxTokens: 4096, strictMode: false },
  'mistralai/mistral-nemo': { temperature: 0.1, maxTokens: 4096, strictMode: false },
  'deepseek/deepseek-chat': { temperature: 0.1, maxTokens: 6144, strictMode: false },
  'qwen/qwen-2.5-72b-instruct': { temperature: 0.15, maxTokens: 6144, strictMode: false },
  'qwen/qwen-2.5-coder-32b-instruct': { temperature: 0.1, maxTokens: 6144, strictMode: false },

  // Google/Gemini models - don't support OpenAI strict mode
  'google/gemini-2.0-flash-001': { temperature: 0.2, maxTokens: 8192, strictMode: false },
  'google/gemini-2.0-flash-exp:free': { temperature: 0.2, maxTokens: 8192, strictMode: false },
  'google/gemini-pro-1.5': { temperature: 0.2, maxTokens: 8192, strictMode: false },
  'google/gemma-3-27b-it:free': { temperature: 0.2, maxTokens: 8192, strictMode: false },

  // Claude via OpenRouter - uses tool_use natively, not strict
  'anthropic/claude-3.5-sonnet': { temperature: 0.2, maxTokens: 8192, strictMode: false },
  'anthropic/claude-3-haiku': { temperature: 0.2, maxTokens: 4096, strictMode: false },

  // OpenAI via OpenRouter - these DO support strict mode
  'openai/gpt-4o': { temperature: 0.2, maxTokens: 8192, strictMode: true },
  'openai/gpt-4o-mini': { temperature: 0.2, maxTokens: 4096, strictMode: true },

  // Ollama models - don't support strict mode
  'llama3.2': { temperature: 0.1, maxTokens: 4096, strictMode: false },
  'llama3.1:70b': { temperature: 0.1, maxTokens: 4096, strictMode: false },
  'mistral': { temperature: 0.1, maxTokens: 4096, strictMode: false },
  'mixtral': { temperature: 0.1, maxTokens: 4096, strictMode: false },
  'qwen2.5:14b': { temperature: 0.1, maxTokens: 4096, strictMode: false },
  'deepseek-coder-v2': { temperature: 0.1, maxTokens: 4096, strictMode: false },

  // Free tier OpenRouter models
  'xiaomi/mimo-v2-flash:free': { temperature: 0.2, maxTokens: 4096, strictMode: false },
  'openai/gpt-oss-120b:free': { temperature: 0.2, maxTokens: 4096, strictMode: false },
};

/**
 * Get the default settings for a model.
 * Priority: model override > provider default > fallback
 */
export function getModelDefaults(provider: string, model: string): ModelSettings {
  // Check model-specific override first
  if (MODEL_OVERRIDES[model]) {
    return MODEL_OVERRIDES[model];
  }

  // Fall back to provider defaults
  if (PROVIDER_DEFAULTS[provider]) {
    return PROVIDER_DEFAULTS[provider];
  }

  // Ultimate fallback - strictMode false for safety
  return { temperature: 0.2, maxTokens: 4096, strictMode: false };
}
