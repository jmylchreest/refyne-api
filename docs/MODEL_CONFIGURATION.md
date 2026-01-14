# Model Configuration

This document describes how to configure LLM models for Refyne, including model defaults, strict mode settings, and BYOK (Bring Your Own Key) configuration.

## Model Defaults

Model defaults are configured at three levels, with higher levels taking precedence:

1. **Hardcoded defaults** (lowest priority) - Built into the application
2. **S3 config file** - `config/model_defaults.json` in your S3 bucket
3. **Chain configuration** (highest priority) - Per-entry overrides in fallback chains

### Configuration File Format

The S3 config file at `config/model_defaults.json` uses this format:

```json
{
  "provider_defaults": {
    "anthropic": {
      "Temperature": 0.2,
      "MaxTokens": 8192,
      "StrictMode": false
    },
    "openai": {
      "Temperature": 0.2,
      "MaxTokens": 8192,
      "StrictMode": true
    },
    "openrouter": {
      "Temperature": 0.2,
      "MaxTokens": 6144,
      "StrictMode": false
    },
    "ollama": {
      "Temperature": 0.1,
      "MaxTokens": 4096,
      "StrictMode": false
    }
  },
  "model_overrides": {
    "openai/gpt-4o": {
      "Temperature": 0.2,
      "MaxTokens": 8192,
      "StrictMode": true
    },
    "google/gemma-3-27b-it:free": {
      "Temperature": 0.2,
      "MaxTokens": 8192,
      "StrictMode": false
    }
  }
}
```

### Settings

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| Temperature | float | 0.2 | Controls randomness. Lower = more deterministic |
| MaxTokens | int | 4096-8192 | Maximum tokens in LLM response |
| StrictMode | bool | false | Enable strict JSON schema validation |

## Strict Mode

### What is Strict Mode?

Strict mode enables OpenAI's structured outputs feature with strict JSON schema validation. When enabled, the model is guaranteed to return valid JSON matching the exact schema structure.

### Which Models Support Strict Mode?

**Supports Strict Mode (StrictMode: true)**
- `openai/gpt-4o` (via OpenRouter or native OpenAI)
- `openai/gpt-4o-mini` (via OpenRouter or native OpenAI)

**Does NOT Support Strict Mode (StrictMode: false)**
- All Anthropic/Claude models (use tool_use natively)
- Google Gemini/Gemma models
- Meta Llama models
- Mistral models
- Qwen models
- DeepSeek models
- All Ollama local models

### Why Some Models Fail with Strict Mode

When strict mode is enabled for models that don't support it, the model returns an empty response (`"no choices in response"` error). This is because:

1. The strict JSON schema format is OpenAI-specific
2. Other models don't understand the strict schema constraints
3. They fail silently by returning no choices

### Recommendations

1. **For BYOK users**: Only enable strict mode if you're using OpenAI gpt-4o models
2. **For service deployments**: Keep strict mode disabled by default for broader compatibility
3. **For high-accuracy requirements**: Use gpt-4o with strict mode enabled

## BYOK (Bring Your Own Key) Configuration

BYOK users can configure their own LLM providers and models through the dashboard settings.

### Setting Up BYOK

1. Navigate to Settings > LLM Configuration
2. Add your API key for your chosen provider
3. Configure your fallback chain with preferred models
4. Optionally override temperature, max tokens, and strict mode per model

### Provider Configuration

| Provider | API Key Environment Variable | Base URL |
|----------|------------------------------|----------|
| OpenAI | `OPENAI_API_KEY` | https://api.openai.com/v1 |
| Anthropic | `ANTHROPIC_API_KEY` | https://api.anthropic.com |
| OpenRouter | `OPENROUTER_API_KEY` | https://openrouter.ai/api/v1 |
| Ollama | N/A (local) | http://localhost:11434 |

### Error Handling for BYOK

When extraction errors occur:
- **User-visible error**: Sanitized, user-friendly message
- **Error details**: Full error message (visible to BYOK users only)
- **Error category**: Classification (rate_limit, quota_exceeded, model_unsupported, etc.)
- **Provider/Model**: Which provider and model encountered the error

BYOK users see full error details to help with debugging their configuration.

## Fallback Chain Configuration

The fallback chain determines which models to try when extraction fails. Models are tried in order until one succeeds.

### Example Chain Configuration

```json
[
  {
    "provider": "openrouter",
    "model": "openai/gpt-4o",
    "enabled": true,
    "temperature": 0.2,
    "max_tokens": 8192,
    "strict_mode": true
  },
  {
    "provider": "openrouter",
    "model": "google/gemma-3-27b-it:free",
    "enabled": true,
    "temperature": 0.2,
    "max_tokens": 8192,
    "strict_mode": false
  }
]
```

### When to Override Settings

Override chain settings when:
- A specific model performs better with different temperature
- You need strict JSON output from a supported model
- Token limits need adjustment for your use case

## Troubleshooting

### "no choices in response" Error

**Cause**: Strict mode enabled for a model that doesn't support it.

**Solution**:
1. Check the model's strict mode support in the table above
2. Set `StrictMode: false` for that model in your chain config or S3 config
3. Or switch to a model that supports strict mode (gpt-4o)

### Rate Limit Errors

**Cause**: Too many requests to the provider.

**Solution**:
1. Add delay between requests
2. Use a fallback chain with multiple providers
3. Upgrade your API tier

### Model Unavailable

**Cause**: Model ID doesn't exist or is deprecated.

**Solution**:
1. Check the provider's model list for current model IDs
2. Update your configuration with valid model IDs
