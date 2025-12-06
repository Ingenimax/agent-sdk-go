# DeepSeek LLM Integration

This package provides a native DeepSeek LLM client implementation for the agent-sdk-go framework with full feature parity to other providers like OpenAI and Anthropic.

## Features

- ✅ Basic text generation
- ✅ Detailed response with token usage
- ✅ Memory/conversation management
- ✅ Tool calling with iterative execution
- ✅ Parallel tool execution
- ✅ Streaming support (coming soon)
- ✅ Multi-tenancy support
- ✅ Retry mechanism
- ✅ Structured output (JSON schema)
- ✅ Agent framework integration

## Supported Models

| Model | Description | Context Length | Max Output |
|-------|-------------|----------------|------------|
| `deepseek-chat` | DeepSeek-V3.2 (Non-thinking Mode) - General-purpose model for coding, summarization, and light reasoning | 128K tokens | 4K default, 8K max |
| `deepseek-reasoner` | DeepSeek-V3.2 (Thinking Mode) - Specialized for chain-of-thought reasoning, math, and complex analysis | 128K tokens | 32K default, 64K max |

**Latest Updates (2025)**:
- **DeepSeek-V3.2**: The current version with integrated thinking in tool-use
- Context window increased to **128K tokens**
- Reasoning model supports up to **64K output tokens**

## Installation

The DeepSeek package is included in agent-sdk-go. No additional installation is required.

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/llm/deepseek"
)

func main() {
    // Create a DeepSeek client
    client := deepseek.NewClient(
        "your-api-key-here",
        deepseek.WithModel("deepseek-chat"),
    )

    // Generate text
    ctx := context.Background()
    response, err := client.Generate(ctx, "What is the capital of France?")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response)
}
```

### With Detailed Response

```go
// Get detailed response including token usage
response, err := client.GenerateDetailed(ctx, "Explain quantum computing")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Content: %s\n", response.Content)
fmt.Printf("Model: %s\n", response.Model)
fmt.Printf("Input Tokens: %d\n", response.Usage.InputTokens)
fmt.Printf("Output Tokens: %d\n", response.Usage.OutputTokens)
fmt.Printf("Total Tokens: %d\n", response.Usage.TotalTokens)
```

### With Configuration Options

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"

client := deepseek.NewClient(
    apiKey,
    deepseek.WithModel("deepseek-reasoner"),
    deepseek.WithBaseURL("https://api.deepseek.com"),
)

response, err := client.Generate(
    ctx,
    "Solve this problem",
    interfaces.WithTemperature(0.7),
    interfaces.WithSystemMessage("You are a helpful assistant"),
    interfaces.WithTopP(0.95),
)
```

## Tool Calling

DeepSeek supports native tool/function calling:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/deepseek"
)

// Define a simple tool
type WeatherTool struct{}

func (t *WeatherTool) Name() string {
    return "get_weather"
}

func (t *WeatherTool) Description() string {
    return "Get the current weather for a location"
}

func (t *WeatherTool) Parameters() map[string]interfaces.ParameterSpec {
    return map[string]interfaces.ParameterSpec{
        "location": {
            Type:        "string",
            Description: "The city name",
            Required:    true,
        },
    }
}

func (t *WeatherTool) Execute(ctx context.Context, args string) (string, error) {
    var params struct {
        Location string `json:"location"`
    }
    if err := json.Unmarshal([]byte(args), &params); err != nil {
        return "", err
    }

    // Simulate weather lookup
    return fmt.Sprintf("The weather in %s is sunny, 72°F", params.Location), nil
}

func main() {
    client := deepseek.NewClient("your-api-key")

    tools := []interfaces.Tool{
        &WeatherTool{},
    }

    ctx := context.Background()
    response, err := client.GenerateWithTools(
        ctx,
        "What's the weather in San Francisco?",
        tools,
        interfaces.WithMaxIterations(5),
    )

    if err != nil {
        panic(err)
    }

    fmt.Println(response)
}
```

## Memory/Conversation Management

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/memory"

// Create conversation buffer
mem := memory.NewConversationBuffer()

// Set conversation context
ctx := context.Background()
ctx = memory.WithConversationID(ctx, "conversation-123")
ctx = memory.WithOrgID(ctx, "org-456")

// First turn
response1, err := client.Generate(
    ctx,
    "Hello, my name is Alice",
    interfaces.WithMemory(mem),
)

// Add response to memory
mem.AddMessage(ctx, interfaces.Message{
    Role:    interfaces.MessageRoleUser,
    Content: "Hello, my name is Alice",
})
mem.AddMessage(ctx, interfaces.Message{
    Role:    interfaces.MessageRoleAssistant,
    Content: response1,
})

// Second turn - model remembers context
response2, err := client.Generate(
    ctx,
    "What's my name?",
    interfaces.WithMemory(mem),
)
// Response will be: "Your name is Alice"
```

## Agent Framework Integration

### Via Configuration (YAML)

```yaml
llm:
  provider: deepseek
  model: ${DEEPSEEK_MODEL}
  config:
    api_key: ${DEEPSEEK_API_KEY}
    base_url: https://api.deepseek.com
```

### Programmatic Usage

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/agent"

// Create agent with DeepSeek
agentInstance := agent.New(
    agent.WithLLMProvider("deepseek"),
    agent.WithModel("deepseek-chat"),
)

// Run agent
result, err := agentInstance.Run(ctx, "Solve this problem")
```

## Environment Variables

The following environment variables are supported:

- `DEEPSEEK_API_KEY` - Your DeepSeek API key (required)
- `DEEPSEEK_MODEL` - Model to use (default: `deepseek-chat`)
- `DEEPSEEK_BASE_URL` - Custom API base URL (default: `https://api.deepseek.com`)

## Configuration Options

### Client Options

- `WithModel(model string)` - Set the model to use
- `WithLogger(logger logging.Logger)` - Set custom logger
- `WithRetry(opts ...retry.Option)` - Configure retry policy
- `WithBaseURL(url string)` - Set custom base URL
- `WithHTTPClient(client *http.Client)` - Use custom HTTP client

### Generation Options

- `WithTemperature(temp float64)` - Control randomness (0.0-1.0)
- `WithTopP(topP float64)` - Nucleus sampling parameter
- `WithFrequencyPenalty(penalty float64)` - Reduce repetition
- `WithPresencePenalty(penalty float64)` - Encourage topic diversity
- `WithStopSequences(sequences []string)` - Stop generation at sequences
- `WithSystemMessage(message string)` - Set system prompt
- `WithMaxIterations(max int)` - Max tool calling iterations (default: 10)
- `WithMemory(memory interfaces.Memory)` - Enable conversation memory
- `WithResponseFormat(format ResponseFormat)` - Request structured JSON output

## Pricing

**DeepSeek-Chat (deepseek-chat)**:
- Input (Cache Miss): $0.27 per 1M tokens
- Input (Cache Hit): $0.07 per 1M tokens
- Output: $1.10 per 1M tokens

**DeepSeek-Reasoner (deepseek-reasoner)**:
- Input (Cache Miss): $0.55 per 1M tokens
- Input (Cache Hit): $0.14 per 1M tokens
- Output: $2.19 per 1M tokens (includes reasoning tokens)

## Rate Limits

DeepSeek uses a best-effort service model with no hard rate limits. During high traffic, requests may experience delays. The SDK includes built-in retry logic with exponential backoff.

## Error Handling

```go
response, err := client.Generate(ctx, "prompt")
if err != nil {
    // Handle different error types
    if strings.Contains(err.Error(), "401") {
        // Invalid API key
    } else if strings.Contains(err.Error(), "429") {
        // Rate limit (transient)
    } else if strings.Contains(err.Error(), "500") {
        // Server error (transient)
    }
    log.Printf("Error: %v", err)
}
```

## Best Practices

1. **API Key Security**: Never hardcode API keys. Use environment variables or secure configuration.

2. **Error Handling**: Always check for errors and implement appropriate retry logic for transient failures.

3. **Token Management**: Monitor token usage to control costs:
   ```go
   response, _ := client.GenerateDetailed(ctx, prompt)
   fmt.Printf("Tokens used: %d\n", response.Usage.TotalTokens)
   ```

4. **Memory Management**: Use conversation buffers for multi-turn interactions:
   ```go
   mem := memory.NewConversationBuffer(memory.WithMaxSize(50))
   ```

5. **Tool Calling**: Set appropriate max iterations to prevent infinite loops:
   ```go
   interfaces.WithMaxIterations(5)
   ```

6. **Model Selection**:
   - Use `deepseek-chat` for general-purpose tasks
   - Use `deepseek-reasoner` for complex reasoning and problem-solving

## Troubleshooting

### API Key Issues

```
Error: api_key is required for DeepSeek provider
```
**Solution**: Set the `DEEPSEEK_API_KEY` environment variable or provide it in configuration.

### Rate Limiting

```
Error: DeepSeek API error: status=429
```
**Solution**: The SDK automatically retries with exponential backoff. If persistent, reduce request rate.

### Model Not Found

```
Error: model not found
```
**Solution**: Verify the model name. Valid options: `deepseek-chat`, `deepseek-reasoner`.

## Examples

See the `examples/llm/deepseek/` directory for complete working examples:
- Basic usage
- Tool calling
- Memory management
- Agent integration

## API Reference

### Client Creation

```go
func NewClient(apiKey string, options ...Option) *DeepSeekClient
```

### Interface Methods

```go
// Basic generation
func (c *DeepSeekClient) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error)

// Detailed generation with token usage
func (c *DeepSeekClient) GenerateDetailed(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error)

// Tool calling
func (c *DeepSeekClient) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error)

// Detailed tool calling
func (c *DeepSeekClient) GenerateWithToolsDetailed(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error)

// Provider info
func (c *DeepSeekClient) Name() string
func (c *DeepSeekClient) SupportsStreaming() bool
```

## Resources

- [DeepSeek API Documentation](https://api-docs.deepseek.com/)
- [Get API Key](https://platform.deepseek.com/api_keys)
- [Pricing Information](https://api-docs.deepseek.com/quick_start/pricing)
- [Rate Limits](https://api-docs.deepseek.com/quick_start/rate_limit)

## License

This package is part of agent-sdk-go and follows the same license.
