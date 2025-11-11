# Token Usage Tracking Example

This example demonstrates the new token usage tracking feature added to the agent-sdk-go library.

## Overview

The library now provides two ways to interact with LLM providers:

1. **Traditional Methods** (backward compatible):
   - `Generate(ctx, prompt, options)` → returns `string`
   - `GenerateWithTools(ctx, prompt, tools, options)` → returns `string`

2. **New Detailed Methods** (with token usage):
   - `GenerateDetailed(ctx, prompt, options)` → returns `*LLMResponse`
   - `GenerateWithToolsDetailed(ctx, prompt, tools, options)` → returns `*LLMResponse`

## LLMResponse Structure

The new `LLMResponse` type provides rich information about the generation:

```go
type LLMResponse struct {
    Content    string                 // Generated text
    Model      string                 // Model used for generation
    StopReason string                 // Why generation stopped
    Usage      *TokenUsage           // Token usage information
    Metadata   map[string]interface{} // Provider-specific data
}

type TokenUsage struct {
    InputTokens     int // Tokens in the input/prompt
    OutputTokens    int // Tokens in the generated response
    TotalTokens     int // Total tokens used
    ReasoningTokens int // Reasoning tokens (for supported models)
}
```

## Running the Example

```bash
# Set your Anthropic API key
export ANTHROPIC_API_KEY="your-api-key-here"

# Run the example
cd examples/token-usage
go run main.go
```

## Features Demonstrated

1. **Basic Token Tracking**: Get input/output token counts for any generation
2. **Cost Estimation**: Calculate estimated costs based on token usage
3. **Backward Compatibility**: Existing code continues to work unchanged
4. **Tools Integration**: Token usage works with tool-calling workflows
5. **Provider Information**: Access model name, stop reason, and metadata

## Provider Support

| Provider | Token Usage Available | Notes |
|----------|----------------------|-------|
| Anthropic | ✅ Full Support | Input/Output tokens available |
| OpenAI | ✅ Full Support | Input/Output + Reasoning tokens |
| Ollama | ❌ Not Available | Local models don't provide usage data |
| vLLM | ❌ Not Available | Local models don't provide usage data |
| Azure OpenAI | ✅ Full Support | Similar to OpenAI |

When token usage is not available, the `Usage` field will be `nil`.

## Use Cases

- **Cost Monitoring**: Track token usage for billing and cost optimization
- **Performance Analysis**: Monitor token efficiency across different prompts
- **Rate Limiting**: Implement token-based rate limiting
- **Analytics**: Gather usage statistics for LLM applications
- **Debugging**: Understand token consumption patterns