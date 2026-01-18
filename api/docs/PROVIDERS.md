# Potential LLM Providers

This document lists LLM providers that could be integrated into the extraction service for user-configurable fallback chains.

## Currently Supported

| Provider | Type | Notes |
|----------|------|-------|
| **OpenRouter** | Aggregator | Multi-model gateway, unified API, pay-per-use |
| **Anthropic** | Direct | Claude models, direct API access |
| **OpenAI** | Direct | GPT models, direct API access |
| **Ollama** | Self-hosted | Local model hosting, no API key required |

## Candidates for Integration

### Aggregators / Gateways

| Provider | URL | Notes |
|----------|-----|-------|
| **LiteLLM** | https://litellm.ai | Open-source proxy supporting 100+ LLMs with unified OpenAI-compatible API. Can be self-hosted or used as a service. |
| **ShareAI** | https://shareai.com | AI infrastructure platform with model routing and fallback capabilities. |
| **TensorZero** | https://tensorzero.com | Inference gateway with structured outputs, observability, and optimization. Supports function calling and JSON schemas. |

### Inference Platforms

| Provider | URL | Notes |
|----------|-----|-------|
| **Together AI** | https://together.ai | Fast inference for open models (Llama, Mixtral, etc.). Competitive pricing. |
| **Groq** | https://groq.com | Ultra-fast inference using custom LPU hardware. Very low latency. |
| **Fireworks AI** | https://fireworks.ai | Fast inference, function calling support, fine-tuning. |
| **Replicate** | https://replicate.com | Run models via API, wide model selection, pay-per-second billing. |
| **Anyscale** | https://anyscale.com | Endpoints for open models, enterprise features. |
| **Perplexity** | https://perplexity.ai | Online models with real-time web access (pplx-* models). |
| **Mistral AI** | https://mistral.ai | Direct API for Mistral models. European provider. |
| **Cohere** | https://cohere.com | Enterprise NLP, Command models, strong embeddings. |
| **AI21 Labs** | https://ai21.com | Jamba models, context window optimization. |

### Self-Hosted / Local Options

| Provider | URL | Notes |
|----------|-----|-------|
| **vLLM** | https://vllm.ai | High-throughput serving engine, OpenAI-compatible API. |
| **LocalAI** | https://localai.io | Drop-in OpenAI replacement for local hosting. |
| **llama.cpp** | https://github.com/ggerganov/llama.cpp | Lightweight inference, server mode available. |
| **text-generation-inference** | https://github.com/huggingface/text-generation-inference | HuggingFace's production inference server. |

## Integration Considerations

### API Compatibility

Most providers offer OpenAI-compatible APIs, simplifying integration:
- LiteLLM, Together AI, Groq, Fireworks: Full OpenAI compatibility
- TensorZero: OpenAI-compatible with enhanced features
- Anthropic: Requires dedicated client (different message format)

### Key Features to Evaluate

| Feature | Why It Matters |
|---------|----------------|
| **Structured Output** | JSON mode / function calling for extraction |
| **Rate Limits** | Affects fallback chain behavior |
| **Latency** | User experience for real-time extraction |
| **Cost** | BYOK users pay directly |
| **Context Window** | Large pages need big context |
| **Reliability** | Uptime for system-provided keys |

### Recommended Priority

1. **Together AI** - Fast, cheap, good model selection
2. **Groq** - Extremely fast, good for latency-sensitive workloads
3. **LiteLLM** - Flexibility for users who want to proxy their own setup
4. **Fireworks AI** - Solid function calling, competitive pricing

## Implementation Notes

For BYOK integration, providers should support:
- API key authentication
- Chat completions endpoint
- JSON mode or function calling (preferred)
- Reasonable rate limits for individual users

Consider adding provider-specific configuration:
```go
type ProviderConfig struct {
    Provider    string  // anthropic, openai, together, groq, etc.
    APIKey      string
    BaseURL     string  // For self-hosted or custom endpoints
    Model       string
    MaxTokens   int     // Provider-specific defaults
    Temperature float64
}
```
