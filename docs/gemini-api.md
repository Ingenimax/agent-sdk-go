# Gemini API Integration

This document provides comprehensive information about the Gemini API integration in the Agent SDK for Go.

## Overview

The Gemini client in the Agent SDK for Go provides a unified interface for interacting with Google's powerful Gemini models through both the **Gemini API** and the **Vertex AI** platform. This allows developers to choose the backend that best suits their needs while using the same consistent SDK. Gemini models provide state-of-the-art capabilities including:

- **Multi-modal understanding** (text, images, video, audio)
- **Function calling** for tool integration
- **Structured output** with JSON schema validation
- **Streaming responses** for real-time applications
- **Advanced reasoning** capabilities
- **Long context** processing (up to 2M tokens)

## Supported Models

| Model | Context Length | Vision | Audio | Tools | Thinking | Backend Support | Best For |
|-------|----------------|---------|-------|-------|----------|-----------------|-----------|
| `gemini-2.5-pro` | 2M tokens | ✅ | ✅ | ✅ | ✅ | Gemini API, Vertex API | Complex reasoning, multimodal |
| `gemini-2.5-flash` | 1M tokens | ✅ | ✅ | ✅ | ✅ | Gemini API, Vertex API | Balanced speed & capability |
| `gemini-2.5-flash-lite` | 32K tokens | ❌ | ❌ | ✅ | ❌ | Gemini API, Vertex API | Fast text processing |
| `gemini-1.5-pro` | 2M tokens | ✅ | ❌ | ✅ | ❌ | Gemini API | Legacy complex tasks |
| `gemini-1.5-flash` | 1M tokens | ✅ | ❌ | ✅ | ❌ | Gemini API | Legacy balanced performance |

**Note**: Model availability varies by backend:
- **Gemini API**: Supports all models listed above
- **Vertex AI**: Has a different set of available models. Some models like Gemini 1.5 series are not supported in Vertex AI. See [Vertex AI Model Garden](https://cloud.google.com/vertex-ai/generative-ai/docs/model-garden/available-models) for the complete list of supported models.

## Installation and Setup

### Prerequisites

1. **API Key**: Obtain your API key from [Google AI Studio](https://aistudio.google.com/app/apikey)
2. **Go Version**: Go 1.24+ required
3. **Dependencies**: The SDK automatically includes required Google API dependencies

### Environment Setup

Set your API key as an environment variable:

```bash
export GEMINI_API_KEY="your-api-key-here"
```

### Basic Client Creation

#### Option 1: Direct Client Creation

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
)

func main() {
    client, err := gemini.NewClient(
        context.Background(),
        gemini.WithAPIKey(os.Getenv("GEMINI_API_KEY")),
        gemini.WithModel(gemini.ModelGemini25Flash),
    )
    if err != nil {
        log.Fatal(err)
    }

    response, err := client.Generate(context.Background(), "Hello, world!")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response)
}
```

#### Option 2: Shared Utility (Recommended)

For multi-provider support with auto-detection:

```go
package main

import (
    "context"
    "log"

    "github.com/Ingenimax/agent-sdk-go/examples/microservices/shared"
)

func main() {
    // Auto-detects provider based on available API keys
    llm, err := shared.CreateLLM()
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Using LLM: %s\n", shared.GetProviderInfo())

    response, err := llm.Generate(context.Background(), "Hello, world!")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response)
}
```

#### Option 3: Vertex AI Backend

For users on Google Cloud Platform, the client can use the Vertex AI backend, which authenticates using Application Default Credentials (e.g., a service account) instead of an API key.

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
    "google.golang.org/genai"
)

func main() {
    // Assumes you have authenticated with GCP CLI or have a service account set up
    client, err := gemini.NewClient(
        context.Background(),
        gemini.WithBackend(genai.BackendVertexAI),
        gemini.WithProjectID("your-gcp-project-id"),
        gemini.WithLocation("us-central1"), // Optional, defaults to us-central1
        gemini.WithModel(gemini.ModelGemini15Pro),
    )
    if err != nil {
        log.Fatal(err)
    }

    response, err := client.Generate(context.Background(), "Hello from Vertex AI!")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response)
}
```

## Core Features

### 1. Text Generation

Basic text generation with customizable parameters:

```go
response, err := client.Generate(ctx, "Explain quantum computing",
    gemini.WithTemperature(0.7),
    gemini.WithTopP(0.9),
    gemini.WithStopSequences([]string{"END", "STOP"}),
    gemini.WithSystemMessage("You are a physics professor."),
)
```

### 2. Function Calling

Integrate external tools and APIs:

```go
// Define your tools
tools := []interfaces.Tool{
    calculator.New(),
    websearch.New(),
}

// Generate with tools
response, err := client.GenerateWithTools(ctx,
    "What's the weather in Tokyo and calculate 15 * 23?",
    tools,
    gemini.WithSystemMessage("Use tools when needed."),
)
```

### 3. Structured Output

Generate JSON responses with automatic schema generation:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"

// Define your response structure
type TextAnalysis struct {
    Summary    string   `json:"summary"`
    Confidence float64  `json:"confidence"`
    Categories []string `json:"categories"`
}

// Generate with structured output
responseFormat := structuredoutput.NewResponseFormat(TextAnalysis{})
response, err := client.Generate(ctx, "Analyze this text...",
    gemini.WithResponseFormat(responseFormat),
)

// Parse the structured response
var result TextAnalysis
if err := json.Unmarshal([]byte(response), &result); err != nil {
    log.Fatal(err)
}
fmt.Printf("Summary: %s, Confidence: %.2f\n", result.Summary, result.Confidence)
```

### 4. Streaming Responses

Process responses in real-time:

```go
stream, err := client.GenerateStream(ctx, "Tell me a story")
if err != nil {
    log.Fatal(err)
}

for event := range stream {
    switch event.Type {
    case interfaces.StreamEventContentDelta:
        fmt.Print(event.Content)
    case interfaces.StreamEventError:
        fmt.Printf("Error: %v\n", event.Error)
    case interfaces.StreamEventMessageStop:
        fmt.Println("\n[Complete]")
    }
}
```

### 5. Reasoning Modes

For models that support native "thinking" capabilities (like the `gemini-2.5-pro` and `gemini-2.5-flash` series), you can enable options to receive the model's thought process alongside the final answer. This is useful for debugging, understanding the model's logic, and building more transparent applications.

```go
// Enable thinking to get the model's thought process
// The thought process is delivered via the StreamEventThinking event in streaming mode.
response, err := client.Generate(ctx, "Solve this riddle: ...",
    gemini.WithThinking(true))

// You can also set a budget for thinking tokens
budget := int32(4096)
response, err := client.Generate(ctx, "Analyze this document...",
    gemini.WithThinkingBudget(budget))

// For models that do not support native thinking, the `WithReasoning` option
// can be used to guide the verbosity of the explanation by modifying the system prompt.
response, err := client.Generate(ctx, "Explain black holes",
    gemini.WithReasoning(gemini.ReasoningModeComprehensive), // "comprehensive", "minimal", or "none"
    gemini.WithSystemMessage("You are a physics teacher."),
)
```

## Agent Integration

### Creating Agents with Gemini

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// Create Gemini client
geminiClient, err := gemini.NewClient(context.Background(),
    gemini.WithAPIKey(apiKey),
    gemini.WithModel(gemini.ModelGemini25Flash))

// Create agent
myAgent, err := agent.NewAgent(
    agent.WithLLM(geminiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithTools(tools...),
    agent.WithSystemPrompt("You are a helpful assistant."),
    agent.WithName("GeminiAgent"),
)

// Use the agent
response, err := myAgent.Run(ctx, "Help me plan a trip to Japan")
```

### Memory and Context Management

The Gemini integration fully supports the Agent SDK's memory system:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/memory"

// Conversation buffer for short-term memory
buffer := memory.NewConversationBuffer()

// Vector retriever for long-term memory
vectorMemory := memory.NewVectorRetriever(vectorStore, embedder)

agent, err := agent.NewAgent(
    agent.WithLLM(geminiClient),
    agent.WithMemory(buffer),
    // ... other options
)
```

### Multi-tenancy Support

Support multiple organizations and users:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"

// Add organization context
ctx = multitenancy.WithOrgID(ctx, "org-123")
ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-456")

response, err := agent.Run(ctx, "What did we discuss yesterday?")
```

## Advanced Features

### 1. Multi-modal Capabilities

Gemini models support vision and audio understanding:

```go
// Note: Multi-modal support requires additional implementation
// for file upload and content handling
```

### 2. Safety and Content Filtering

Gemini includes built-in safety filtering:

```go
// Safety settings are automatically applied
// Default settings block medium and high risk content for:
// - Harassment
// - Hate speech
// - Sexually explicit content
// - Dangerous content
```

### 3. Error Handling and Retries

Robust error handling with retry support:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/retry"

client, err := gemini.NewClient(context.Background(),
    gemini.WithAPIKey(apiKey),
    gemini.WithRetry(
        retry.WithMaxAttempts(3),
        retry.WithBackoffCoefficient(2.0),
    ),
)
```

### 4. Custom Logging

Integrate with your logging system:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/logging"

logger := logging.New()
client, err := gemini.NewClient(context.Background(),
    gemini.WithAPIKey(apiKey),
    gemini.WithLogger(logger))
```

## Configuration Options

### Client Options

The `NewClient` function is highly configurable using options. Here are examples for both Gemini API and Vertex AI backends.

```go
// Example for Gemini API Backend
client, err := gemini.NewClient(ctx,
    // Authentication
    gemini.WithAPIKey(apiKey),

    // Model selection
    gemini.WithModel(gemini.ModelGemini25Pro),

    // Logging
    gemini.WithLogger(customLogger),

    // Retry configuration
    gemini.WithRetry(
        retry.WithMaxAttempts(5),
    ),
)

// Example for Vertex AI Backend
vertexClient, err := gemini.NewClient(ctx,
    // Backend selection
    gemini.WithBackend(genai.BackendVertexAI),

    // Vertex AI configuration
    gemini.WithProjectID("your-gcp-project-id"),
    gemini.WithLocation("us-central1"),
    gemini.WithCredentialsFile("/path/to/your/credentials.json"), // Optional: for service accounts

    // Model selection
    gemini.WithModel(gemini.ModelGemini15Pro), // Note: Vertex has a different set of available models
)
```

### Generation Options

```go
response, err := client.Generate(ctx, prompt,
    // Temperature controls randomness (0.0 - 1.0)
    gemini.WithTemperature(0.7),

    // TopP controls nucleus sampling (0.0 - 1.0)
    gemini.WithTopP(0.9),

    // Stop sequences
    gemini.WithStopSequences([]string{"STOP", "END"}),

    // System message
    gemini.WithSystemMessage("You are an expert assistant."),

    // Reasoning mode
    gemini.WithReasoning("comprehensive"),

    // Structured output
    gemini.WithResponseFormat(responseFormat),
)
```

## Performance and Best Practices

### Model Selection

- **Gemini 2.5 Pro**: Complex reasoning, research, analysis
- **Gemini 2.5 Flash**: General purpose, balanced performance
- **Gemini 2.5 Flash Lite**: Simple tasks, high throughput

### Token Management

```go
capabilities := gemini.GetModelCapabilities(model)
fmt.Printf("Max input tokens: %d\n", capabilities.MaxInputTokens)
fmt.Printf("Max output tokens: %d\n", capabilities.MaxOutputTokens)
```

### Streaming for Long Responses

Use streaming for better user experience:

```go
// For long responses, always use streaming
if expectedLongResponse {
    stream, err := client.GenerateStream(ctx, prompt)
    // Handle stream events
}
```

### Tool Calling Optimization

- Limit the number of tools (max 20-30)
- Provide clear, specific tool descriptions
- Use structured parameters with types and enums
- Handle tool errors gracefully

### Error Handling Patterns

```go
response, err := client.Generate(ctx, prompt)
if err != nil {
    // Handle different error types
    switch {
    case strings.Contains(err.Error(), "rate limit"):
        // Implement exponential backoff
        time.Sleep(time.Second * 2)
        // Retry
    case strings.Contains(err.Error(), "content filter"):
        // Handle content filtering
        return "Content was filtered for safety"
    default:
        // Generic error handling
        return fmt.Sprintf("Error: %v", err)
    }
}
```

## Troubleshooting

### Common Issues

1. **Authentication Errors**
   ```
   Error: API key not valid
   ```
   - Verify API key is correct
   - Check key permissions in Google AI Studio
   - Ensure environment variable is set

2. **Model Not Found**
   ```
   Error: model not found
   ```
   - Use supported model names (see constants)
   - Check for typos in model name

3. **Rate Limiting**
   ```
   Error: rate limit exceeded
   ```
   - Implement exponential backoff
   - Reduce request frequency
   - Consider upgrading API quota

4. **Content Filtering**
   ```
   Error: content blocked by safety filter
   ```
   - Review content for policy violations
   - Rephrase sensitive content
   - Understand safety categories

5. **Context Length Exceeded**
   ```
   Error: context length exceeded
   ```
   - Use appropriate model for context size
   - Implement context truncation
   - Consider conversation summarization

### Debug Logging

Enable detailed logging for troubleshooting:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/logging"

logger := logging.New()
client, err := gemini.NewClient(apiKey, gemini.WithLogger(logger))
```

### Performance Monitoring

Monitor key metrics:
- Request/response latency
- Token usage per request
- Error rates by error type
- Tool execution times

## API Reference

### Types and Constants

```go
// Models
const (
    ModelGemini25Pro      = "gemini-2.5-pro"
    ModelGemini25Flash    = "gemini-2.5-flash"
    ModelGemini25FlashLite = "gemini-2.5-flash-lite"
    ModelGemini15Pro      = "gemini-1.5-pro"
    ModelGemini15Flash    = "gemini-1.5-flash"
    DefaultModel          = ModelGemini25Flash
)

// Reasoning modes
type ReasoningMode string
const (
    ReasoningModeNone          ReasoningMode = "none"
    ReasoningModeMinimal       ReasoningMode = "minimal"
    ReasoningModeComprehensive ReasoningMode = "comprehensive"
)
```

### Client Methods

```go
// Core generation methods
Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error)
GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error)

// Streaming methods
GenerateStream(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error)
GenerateWithToolsStream(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error)

// Utility methods
Name() string
SupportsStreaming() bool
GetModel() string
```

### Option Functions

```go
// Client configuration
WithAPIKey(apiKey string) Option // For Gemini API backend
WithBackend(backend genai.Backend) Option // genai.BackendGeminiAPI or genai.BackendVertexAI
WithProjectID(projectID string) Option // For Vertex AI backend
WithLocation(location string) Option // For Vertex AI backend
WithCredentialsFile(path string) Option // For Vertex AI with a service account file
WithModel(model string) Option
WithLogger(logger logging.Logger) Option
WithRetry(opts ...retry.Option) Option
WithBaseURL(baseURL string) Option // For Gemini API backend
WithClient(client *genai.Client) Option // Use an existing genai.Client

// Generation options
WithTemperature(temperature float64) interfaces.GenerateOption
WithTopP(topP float64) interfaces.GenerateOption
WithStopSequences(sequences []string) interfaces.GenerateOption
WithSystemMessage(message string) interfaces.GenerateOption
WithReasoning(mode string) interfaces.GenerateOption
WithResponseFormat(format interfaces.ResponseFormat) interfaces.GenerateOption
```

### Model Capabilities

```go
type ModelCapabilities struct {
    SupportsStreaming    bool
    SupportsToolCalling  bool
    SupportsVision       bool
    SupportsAudio        bool
    MaxInputTokens       int
    MaxOutputTokens      int
    SupportedMimeTypes   []string
}

// Utility functions
GetModelCapabilities(model string) ModelCapabilities
IsVisionModel(model string) bool
IsAudioModel(model string) bool
SupportsToolCalling(model string) bool
```

## Examples

See the `examples/llm/gemini/` directory for comprehensive examples:

- **Basic Usage** (`main.go`) - Core features demonstration with structured output

See the `examples/microservices/` directory for microservices examples with Gemini support:

- **Basic Microservice** (`basic_microservice/main.go`) - Simple agent microservice
- **Mixed Agents** (`mixed_agents/main.go`) - Local and remote agent coordination
- **Simple Mixed** (`simple_mixed/main.go`) - Simplified mixed agent architecture
- **Streaming Agent** (`streaming_agent/main.go`) - Real-time streaming responses

See the `examples/advanced_agent_streaming/` directory for advanced streaming:

- **Advanced Agent Streaming** (`main.go`) - Multi-agent coordination with streaming

### Multi-LLM Provider Support

All examples now support Gemini alongside OpenAI and Anthropic with auto-detection:

```bash
# Auto-detection based on available API keys
export GEMINI_API_KEY="your_key"
go run examples/microservices/basic_microservice/main.go

# Explicit provider selection
export GEMINI_API_KEY="your_key"
export LLM_PROVIDER="gemini"
export GEMINI_MODEL="gemini-2.5-flash"
go run examples/advanced_agent_streaming/main.go
```

## Support and Resources

### Documentation
- [Gemini API Documentation](https://ai.google.dev/gemini-api/docs)
- [Google AI Studio](https://aistudio.google.com/)
- [Agent SDK Documentation](../README.md)

### Community
- [GitHub Issues](https://github.com/Ingenimax/agent-sdk-go/issues)
- [Discussions](https://github.com/Ingenimax/agent-sdk-go/discussions)

### Getting Help

1. Check this documentation and examples
2. Search existing GitHub issues
3. Review Gemini API documentation
4. Create a new issue with detailed reproduction steps

## Changelog

### v1.1.0 (Multi-LLM Provider Support)
- Added comprehensive Gemini support across all examples
- Implemented shared LLM utility for auto-detection
- Updated structured output to use official `structuredoutput` package
- Added thinking token support for Gemini 2.5 models
- Created unified configuration patterns
- Updated documentation with multi-provider examples

### v1.0.0 (Initial Release)
- Basic text generation
- Function calling support
- Structured output with JSON schemas
- Streaming responses
- Reasoning modes
- Agent SDK integration
- Multi-tenancy support
- Comprehensive error handling
- Full test coverage
- Examples and documentation
