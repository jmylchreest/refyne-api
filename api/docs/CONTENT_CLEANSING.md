# Content Cleansing Approaches

This document outlines HTML content cleansing strategies for the Refyne API, particularly for the analyzer and extraction services.

## Cleaner Chain Pattern

The refinery library uses a **chain builder pattern** where multiple cleaners can be composed:

```go
// Chain example: Trafilatura -> Markdown
cleanerChain := cleaner.NewChain(
    cleaner.NewTrafilatura(&cleaner.TrafilaturaConfig{
        Output: cleaner.OutputHTML,
        Tables: cleaner.Include,
        Links:  cleaner.Include,
    }),
    cleaner.NewMarkdown(),
)
```

Available cleaners:
- `cleaner.NewNoop()` - Pass-through, no cleaning
- `cleaner.NewTrafilatura(config)` - Main content extraction
- `cleaner.NewMarkdown()` - HTML to Markdown conversion
- `cleaner.NewChain(cleaners...)` - Compose multiple cleaners

## Current Implementation

### Extraction Service
Uses: `Trafilatura (text output) -> Markdown` chain
- Extracts main content and converts to clean markdown
- Good for LLM consumption but loses some structural detail

### Analyzer Service
- **Primary:** `Noop` cleaner (raw HTML) - preserves all detail
- **Fallback:** `Trafilatura (HTML output, tables+links)` - used on context length errors
- Auto-retries with fallback cleaner when context window exceeded

## Problem Statement

Different use cases have conflicting requirements:

| Use Case | Need | Risk |
|----------|------|------|
| **Analysis** | Maximum detail to understand page structure | Context length errors |
| **Extraction** | Clean, focused content for accurate extraction | May lose relevant data |
| **E-commerce/Listings** | Preserve tables, product grids, prices | Article extractors remove these |

## Available In-Process Go Libraries

### 1. go-trafilatura (Currently Used)
**Repo:** https://github.com/markusmobius/go-trafilatura

**Pros:**
- Native Go port of Python trafilatura
- Multiple output formats: HTML, text, JSON
- Configurable table/link/image preservation
- Fallback to readability/dom-distiller

**Cons:**
- Designed for article extraction
- May discard product listings, non-article content
- Text output loses all structure

**Configuration Options:**
```go
// HTML output with tables preserved
trafilatura.Extract(reader, trafilatura.Options{
    Output:        trafilatura.OutputHTML,
    IncludeTables: true,
    IncludeLinks:  true,
    IncludeImages: true,
})
```

### 2. go-readability (go-shiori)
**Repo:** https://github.com/go-shiori/go-readability
**Status:** DEPRECATED - use codeberg.org/readeck/go-readability/v2

**Pros:**
- Mozilla Readability port
- Produces HTML and text output
- Extracts metadata (title, author, etc.)

**Cons:**
- Article-focused, not data extraction
- Limited structure preservation
- Deprecated

### 3. GoOse
**Repo:** https://github.com/advancedlogic/GoOse

**Pros:**
- Article/content extraction
- Metadata extraction

**Cons:**
- Article-focused
- Less configurable than trafilatura

## Out-of-Process Options (Future Consideration)

### MinerU-HTML
**Repo:** https://github.com/opendatalab/MinerU-HTML
**Type:** Python with REST API

**Why it's interesting:**
- Uses 0.6B language model for semantic content understanding
- 32% better accuracy than Trafilatura (81.8% vs 63.6% ROUGE-N F1)
- Excellent structure preservation:
  - 90.9% for code blocks
  - 94.0% for tables/formulas
- REST API via FastAPI

**Integration approach:**
- Deploy as sidecar container
- Call `/extract` endpoint with HTML
- Returns clean, LLM-ready content

**Why not now:**
- Adds deployment complexity
- Python dependency
- Latency overhead of HTTP call

## Implemented Strategy

### For Analyzer Service (IMPLEMENTED)

**Retry-with-fallback is now implemented:**

1. **Primary attempt:** Use Noop cleaner (raw HTML)
   - Preserves maximum detail
   - Best for comprehensive analysis

2. **On context length error:** Auto-retry with Trafilatura (HTML output)
   - Reduces token count significantly
   - Preserves tables and links via explicit config
   - Excludes images to save tokens
   - Logs reduction percentage for monitoring

```go
// Implemented in analyzer_service.go
fallbackCleaner := cleaner.NewTrafilatura(&cleaner.TrafilaturaConfig{
    Output: cleaner.OutputHTML, // HTML not text - preserves structure
    Tables: cleaner.Include,    // Explicitly preserve tables
    Links:  cleaner.Include,    // Explicitly preserve links
    Images: cleaner.Exclude,    // Exclude images to save tokens
})
```

The `isContextLengthError()` function detects errors containing:
- `context_length`, `max_tokens`, `token limit`
- `too long`, `maximum context`, `exceeds limit`
- `input too large`, `content_too_large`, `request too large`

### For Extraction Service

**Current approach is good:**
- Trafilatura text -> Markdown works well
- Consider switching to HTML output for e-commerce sites

**Future enhancement:**
- Detect page type (article vs listing vs product)
- Use appropriate cleaner chain per type

## Error Detection

Context length errors typically manifest as:
- `context_length_exceeded` in error message
- `max_tokens` or `token limit` in error
- HTTP 400 with specific error codes from LLM providers

```go
func isContextLengthError(err error) bool {
    errStr := strings.ToLower(err.Error())
    return strings.Contains(errStr, "context_length") ||
           strings.Contains(errStr, "max_tokens") ||
           strings.Contains(errStr, "token limit") ||
           strings.Contains(errStr, "too long") ||
           strings.Contains(errStr, "maximum context")
}
```

## Implementation Priority

1. **DONE:** Add retry logic to analyzer with Trafilatura HTML fallback
2. **Medium-term:** Evaluate MinerU-HTML for complex sites (out-of-process)
3. **Long-term:** Page-type-aware cleaner selection

## References

- [go-trafilatura](https://github.com/markusmobius/go-trafilatura)
- [MinerU-HTML](https://github.com/opendatalab/MinerU-HTML)
- [go-readability v2](https://codeberg.org/readeck/go-readability/v2)
- [Trafilatura evaluation](https://trafilatura.readthedocs.io/en/latest/evaluation.html)
