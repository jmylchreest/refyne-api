# Model Configuration Hints

This document provides guidance on configuring different LLM models for optimal schema generation and data extraction in Refyne.

## Overview

Different models have varying capabilities for structured output generation. The fallback chain allows configuring `temperature` and `max_tokens` per-entry to optimize for each model's characteristics.

## Recommended Settings by Provider

### OpenRouter Models

| Model | Temperature | Max Tokens | Notes |
|-------|-------------|------------|-------|
| `google/gemini-2.0-flash-001` | 0.2 | 8192 | Excellent structured output, fast |
| `google/gemini-2.0-flash-exp:free` | 0.2 | 8192 | Free tier, good for testing |
| `anthropic/claude-3.5-sonnet` | 0.2 | 8192 | Excellent schema quality |
| `anthropic/claude-3-haiku` | 0.2 | 4096 | Fast, good for simple schemas |
| `openai/gpt-4o` | 0.2 | 8192 | Strong structured output |
| `openai/gpt-4o-mini` | 0.2 | 4096 | Cost-effective, good quality |
| `meta-llama/llama-3.1-70b-instruct` | 0.1 | 4096 | Lower temp recommended for consistency |
| `meta-llama/llama-3.1-8b-instruct` | 0.1 | 4096 | Lower temp, simpler schemas |
| `mistralai/mistral-large` | 0.1 | 4096 | Lower temp for structured output |
| `mistralai/mistral-nemo` | 0.1 | 4096 | Good balance of speed/quality |
| `qwen/qwen-2.5-72b-instruct` | 0.15 | 6144 | Strong multilingual support |
| `deepseek/deepseek-chat` | 0.1 | 6144 | Good value, needs lower temp |

### Anthropic Direct API

| Model | Temperature | Max Tokens | Notes |
|-------|-------------|------------|-------|
| `claude-sonnet-4-20250514` | 0.2 | 8192 | Latest, excellent quality |
| `claude-3-5-sonnet-20241022` | 0.2 | 8192 | Excellent schema generation |
| `claude-3-5-haiku-20241022` | 0.2 | 4096 | Fast, cost-effective |

### OpenAI Direct API

| Model | Temperature | Max Tokens | Notes |
|-------|-------------|------------|-------|
| `gpt-4o` | 0.2 | 8192 | Best quality |
| `gpt-4o-mini` | 0.2 | 4096 | Good balance |
| `gpt-4-turbo` | 0.2 | 4096 | Reliable structured output |

### Ollama (Local Models)

| Model | Temperature | Max Tokens | Notes |
|-------|-------------|------------|-------|
| `llama3.2` | 0.1 | 4096 | Lower temp essential |
| `llama3.1:70b` | 0.1 | 4096 | Better schema quality |
| `mistral` | 0.1 | 4096 | Fast, decent quality |
| `mixtral` | 0.1 | 4096 | Good for complex schemas |
| `qwen2.5:14b` | 0.1 | 4096 | Good multilingual |
| `deepseek-coder-v2` | 0.1 | 4096 | Strong at structured output |

## Temperature Guidelines

Temperature controls randomness in model output. For schema generation:

| Temperature | Use Case |
|-------------|----------|
| 0.0-0.1 | Most consistent output. Use for models that tend to be creative (Llama, Mistral, DeepSeek) |
| 0.1-0.2 | Recommended default. Slight variation while maintaining structure |
| 0.2-0.3 | More creative field descriptions. May produce inconsistent formats |
| > 0.3 | Not recommended for schema generation |

**Rule of thumb**: Start with 0.2 for Claude/GPT models, 0.1 for open-source models.

## Max Tokens Guidelines

Max tokens limits the response length. Schema complexity determines needs:

| Schema Type | Recommended Max Tokens |
|-------------|----------------------|
| Simple (5-10 fields) | 2048-3000 |
| Standard (10-20 fields) | 4096 |
| Complex (20+ fields, nested) | 6144-8192 |
| Multi-entity schemas | 8192 |

**Note**: Setting too low will truncate schemas. Setting too high wastes context on some models. The analyze prompt with a complex site can generate 3000-5000 token schemas.

## Model-Specific Considerations

### Claude Models (Anthropic)
- Excellent at following complex instructions
- Naturally produces detailed field descriptions
- Temperature 0.2 works well
- Supports system messages (used for reinforcement)

### GPT-4 Models (OpenAI)
- Strong structured output capabilities
- Good at inferring formats from examples
- Temperature 0.2 works well
- Benefits from explicit format specifications

### Gemini Models (Google via OpenRouter)
- Very fast inference
- Good schema quality
- Handles multilingual content well
- Temperature 0.2 works well

### Llama Models (Meta)
- Quality varies significantly by size (8B vs 70B)
- Needs lower temperature (0.1) for consistent JSON
- May occasionally produce malformed YAML
- 70B+ recommended for production

### Mistral Models
- Good balance of speed and quality
- Needs lower temperature (0.1)
- Mistral Large preferred for complex schemas

### DeepSeek Models
- Excellent value for cost
- Strong at code/structured output
- Needs lower temperature (0.1)
- May need explicit JSON format reminders

### Qwen Models
- Excellent multilingual capabilities
- Good structured output
- Temperature 0.1-0.15 recommended

## Fallback Chain Best Practices

1. **Lead with quality**: Put your best model first for initial schema generation
2. **Fast fallbacks**: Use faster/cheaper models as fallbacks for when primary fails
3. **Match capabilities**: Don't fall back to a much weaker model for complex tasks
4. **Consider costs**: Balance quality vs cost for your use case

Example chain for production:
```
1. anthropic/claude-3.5-sonnet (temp: 0.2, max: 8192) - Primary
2. google/gemini-2.0-flash-001 (temp: 0.2, max: 8192) - Fast fallback
3. openai/gpt-4o-mini (temp: 0.2, max: 4096) - Cost fallback
```

Example chain for budget-conscious:
```
1. google/gemini-2.0-flash-exp:free (temp: 0.2, max: 8192) - Free tier
2. openai/gpt-4o-mini (temp: 0.2, max: 4096) - Cheap fallback
3. meta-llama/llama-3.1-8b-instruct (temp: 0.1, max: 4096) - Local/free
```

## Troubleshooting

### Schemas are truncated
- Increase `max_tokens` in chain config
- Check model's actual output limit

### Inconsistent JSON/YAML format
- Lower temperature to 0.1
- Consider a different model

### Field descriptions too brief
- This is a prompt quality issue, not model config
- The analyze prompt should guide this

### Model times out
- Some models are slower; adjust client timeout
- Consider faster fallback models

### Poor multilingual handling
- Use Qwen or Gemini for non-English sites
- Claude also handles multilingual well
