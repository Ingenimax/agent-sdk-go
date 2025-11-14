# Token Usage Tracking

This document explains the token usage tracking feature in the agent-sdk-go library, which addresses [GitHub Issue #166](https://github.com/Ingenimax/agent-sdk-go/issues/166).

## Overview

The agent-sdk-go library provides comprehensive token usage tracking capabilities for cost monitoring, usage analytics, and optimization. This feature allows developers to track input/output token counts across all supported LLM providers.

## Key Benefits

- **Cost Monitoring**: Track token usage for accurate billing and cost optimization
- **Performance Analytics**: Monitor token efficiency across different prompts and models
- **Resource Planning**: Plan capacity based on actual usage patterns
- **Debugging**: Understand token consumption for optimization
- **Rate Limiting**: Implement token-based rate limiting
- **Backward Compatibility**: Existing code continues to work unchanged

## API Design

### Response Types

The library introduces rich response objects that include token usage information:

```go
// LLMResponse represents the detailed response from an LLM generation request
type LLMResponse struct {
    // Content is the generated text response
    Content string

    // Usage contains token usage information (nil if not available)
    Usage *TokenUsage

    // Model indicates which model was used for generation
    Model string

    // StopReason indicates why the generation stopped (optional)
    StopReason string

    // Metadata contains provider-specific additional information
    Metadata map[string]interface{}
}

// TokenUsage represents token usage information for an LLM request
type TokenUsage struct {
    // InputTokens is the number of tokens in the input/prompt
    InputTokens int

    // OutputTokens is the number of tokens in the generated response
    OutputTokens int

    // TotalTokens is the total number of tokens used (input + output)
    TotalTokens int

    // ReasoningTokens is the number of tokens used for reasoning (optional)
    ReasoningTokens int
}
```

### Available Methods

The library provides two sets of methods for each operation:

**Traditional Methods** (backward compatible):
- `Generate(ctx, prompt, options) string` - Returns only content
- `GenerateWithTools(ctx, prompt, tools, options) string` - Returns only content

**Detailed Methods** (with token usage):
- `GenerateDetailed(ctx, prompt, options) *LLMResponse` - Returns rich response
- `GenerateWithToolsDetailed(ctx, prompt, tools, options) *LLMResponse` - Returns rich response

## Usage Examples

### Basic Token Tracking

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
)

func main() {
    client := anthropic.NewClient("your-api-key",
        anthropic.WithModel("claude-3-haiku-20240307"),
    )

    ctx := context.Background()
    prompt := "Explain quantum computing in one paragraph."

    // Get detailed response with token usage
    response, err := client.GenerateDetailed(ctx, prompt)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Response: %s\n", response.Content)
    fmt.Printf("Model: %s\n", response.Model)

    if response.Usage != nil {
        fmt.Printf("Token Usage:\n")
        fmt.Printf("  Input: %d tokens\n", response.Usage.InputTokens)
        fmt.Printf("  Output: %d tokens\n", response.Usage.OutputTokens)
        fmt.Printf("  Total: %d tokens\n", response.Usage.TotalTokens)

        if response.Usage.ReasoningTokens > 0 {
            fmt.Printf("  Reasoning: %d tokens\n", response.Usage.ReasoningTokens)
        }
    }
}
```

### Cost Calculation

```go
func calculateCost(usage *interfaces.TokenUsage, provider string) float64 {
    if usage == nil {
        return 0
    }

    var inputRate, outputRate float64
    switch provider {
    case "anthropic":
        inputRate = 0.25 / 1000000   // $0.25 per 1M input tokens
        outputRate = 1.25 / 1000000  // $1.25 per 1M output tokens
    case "openai":
        inputRate = 0.15 / 1000000   // $0.15 per 1M input tokens
        outputRate = 0.60 / 1000000  // $0.60 per 1M output tokens
    }

    inputCost := float64(usage.InputTokens) * inputRate
    outputCost := float64(usage.OutputTokens) * outputRate
    return inputCost + outputCost
}
```

### Usage Analytics

```go
type UsageStats struct {
    TotalRequests   int
    TotalTokens     int
    TotalCost       float64
    AverageTokens   float64
    ProviderBreakdown map[string]int
}

func trackUsage(stats *UsageStats, response *interfaces.LLMResponse, provider string) {
    if response.Usage != nil {
        stats.TotalRequests++
        stats.TotalTokens += response.Usage.TotalTokens
        stats.TotalCost += calculateCost(response.Usage, provider)
        stats.AverageTokens = float64(stats.TotalTokens) / float64(stats.TotalRequests)

        if stats.ProviderBreakdown == nil {
            stats.ProviderBreakdown = make(map[string]int)
        }
        stats.ProviderBreakdown[provider] += response.Usage.TotalTokens
    }
}
```

## Implementation Architecture

### Design Principles

1. **Backward Compatibility**: Existing methods remain unchanged
2. **Graceful Degradation**: Returns `nil` usage when unavailable
3. **Code Reuse**: Internal functions shared between traditional and detailed methods
4. **Provider Abstraction**: Unified interface across all providers
5. **Future Extensibility**: Designed for additional features like cost calculation

### Internal Architecture

Each provider implements the detailed methods by:

1. **Shared Logic**: Using internal `generateInternal()` function for actual API calls
2. **Response Mapping**: Converting provider-specific usage data to standard `TokenUsage` format
3. **Graceful Handling**: Returning `nil` usage when provider doesn't support it
4. **Metadata Enrichment**: Adding provider-specific information to response metadata

Example implementation pattern:

```go
// Traditional method (unchanged)
func (c *ProviderClient) Generate(ctx context.Context, prompt string, options ...GenerateOption) (string, error) {
    response, err := c.generateInternal(ctx, prompt, options...)
    if err != nil {
        return "", err
    }
    return response.Content, nil
}

// Detailed method (new)
func (c *ProviderClient) GenerateDetailed(ctx context.Context, prompt string, options ...GenerateOption) (*LLMResponse, error) {
    return c.generateInternal(ctx, prompt, options...)
}

// Shared internal logic
func (c *ProviderClient) generateInternal(ctx context.Context, prompt string, options ...GenerateOption) (*LLMResponse, error) {
    // Perform actual API call and usage extraction
    // Return unified LLMResponse object
}
```

## Provider Capability Matrix

| Provider | Usage Available | Reasoning Tokens | Streaming Usage | Notes |
|----------|----------------|------------------|-----------------|-------|
| Anthropic | ✅ Yes | ❌ No | ✅ Yes | Complete usage data in responses |
| OpenAI | ✅ Yes | ✅ Yes (o1) | ✅ Yes | Full support with reasoning |
| Azure OpenAI | ✅ Yes | ✅ Yes (o1) | ✅ Yes | Same as OpenAI implementation |
| Gemini | ✅ Yes | ✅ Partial (2.5) | ❓ TBD | Has thinking tokens, count may not be available |
| Ollama | ❌ No | ❌ No | ❌ No | Local model, no usage reporting |
| vLLM | ❌ No | ❌ No | ❓ Limited | Local model, check API docs |

## Risks and Considerations

### Technical Risks

1. **Provider API Changes**: Usage format may vary between providers
2. **Accuracy**: Token counting may differ between providers
3. **Performance**: Additional API overhead for usage tracking

### Mitigation Strategies

1. **Graceful Degradation**: Return nil usage when unavailable
2. **Provider Abstraction**: Normalize usage format across providers
3. **Caching**: Cache usage data to minimize API calls

## Success Criteria

1. ✅ Users can access token usage for all supported providers
2. ✅ Backward compatibility maintained
3. ✅ Consistent interface across providers
4. ✅ Comprehensive documentation and examples
5. ✅ >95% test coverage for new functionality

## Inspiration from Other Frameworks

### LangChain Python Approach

LangChain provides several methods for token tracking:

1. **Callback Context Managers**: `get_openai_callback()` tracks usage across multiple API calls
2. **Usage Metadata in Response Objects**: AIMessage objects include `usage_metadata` with standardized keys
3. **Provider-Specific Callbacks**: Separate callbacks for different LLM providers
4. **Cost Estimation**: Built-in cost calculation using provider pricing

**Key Benefits**: Simple context manager pattern, cumulative tracking across chains

### LlamaIndex Approach

LlamaIndex uses a **TokenCountingHandler** callback system:

1. **Event-Based Tracking**: Each API call creates a TokenCountingEvent with detailed metadata
2. **Cumulative Counters**: Properties for total, prompt, completion, and embedding tokens
3. **Custom Tokenizers**: Support for model-specific token counting
4. **Streaming Support**: Handles token counting during streaming responses

**Key Benefits**: Granular event tracking, separation of concerns via callbacks

### Go Library Patterns

Production Go LLM libraries typically include:

1. **Built-in Usage Tracking**: Token counts returned with response objects
2. **Cost Calculation**: Provider-specific pricing integration
3. **Unified Interfaces**: Single API across multiple providers
4. **Concurrency Support**: Go-native patterns for high-throughput usage

### Recommended Approach for agent-sdk-go

Based on this analysis, our implementation should:

1. **Follow Go Patterns**: Return usage information directly in response structs (not callbacks)
2. **Provider Abstraction**: Unified `TokenUsage` interface across all providers
3. **Graceful Degradation**: Return nil when usage information unavailable
4. **Future Extensibility**: Design for cost calculation and analytics features

## Future Enhancements

1. **Usage Analytics**: Built-in usage tracking and reporting (inspired by LangChain)
2. **Cost Estimation**: Provider-specific cost calculation using official pricing
3. **Usage Limits**: Automatic enforcement of usage thresholds
4. **Metrics Integration**: OpenTelemetry metrics for usage tracking
5. **Callback System**: Optional event-based tracking (inspired by LlamaIndex)
6. **Usage Dashboard**: Web UI for monitoring token usage across applications

---

*This plan addresses GitHub Issue #166: "[FEATURE] Show token usage"*