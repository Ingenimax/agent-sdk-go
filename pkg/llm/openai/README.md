# OpenAI Client Package

This package provides a client for interacting with the OpenAI API, implementing the `interfaces.LLM` interface.

## Features

- Text generation with the `Generate` method
- Chat completion with the `Chat` method
- Tool integration with the `GenerateWithTools` method
- Configurable options for model parameters
- Direct implementation of the `interfaces.LLM` interface

## Usage

### Creating a Client

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"

// Create a client with default settings
client := openai.NewClient(apiKey)

// Create a client with a specific model
client := openai.NewClient(
    apiKey,
    openai.WithModel("gpt-4o-mini"),
)
```

### Text Generation

```go
response, err := client.Generate(
    context.Background(),
    "Write a haiku about programming",
    openai.WithTemperature(0.7),
)
```

### Chat Completion

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/llm"

messages := []llm.Message{
    {
        Role:    "system",
        Content: "You are a helpful programming assistant.",
    },
    {
        Role:    "user",
        Content: "What's the best way to handle errors in Go?",
    },
}

response, err := client.Chat(context.Background(), messages, nil)
```

### Tool Integration

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"

// Define tools
tools := []interfaces.Tool{...}

// Generate with tools
response, err := client.GenerateWithTools(
    context.Background(),
    "What's the weather in San Francisco?",
    tools,
    openai.WithTemperature(0.7),
)
```

### Available Options

The OpenAI client provides several option functions for configuring requests:

- `WithTemperature(float64)` - Controls randomness (0.0 to 1.0)
- `WithTopP(float64)` - Controls diversity via nucleus sampling
- `WithFrequencyPenalty(float64)` - Reduces repetition of token sequences
- `WithPresencePenalty(float64)` - Reduces repetition of topics
- `WithStopSequences([]string)` - Specifies sequences where generation should stop

## Content input

You can send images and other media with the text. With an agent, call
`RunWithContent`. With the client directly, pass the parts with
`interfaces.WithParts`; the parts replace the prompt, so include the text as a
text part:

```go
resp, err := client.Generate(ctx, "", interfaces.WithParts(
    interfaces.NewPartFromBytes(pdf, "application/pdf"),
    interfaces.NewPartFromText("Summarize this report."),
))
```

The client sends PNG, JPEG, WebP, and non-animated GIF images, inline or by
URL, and PDFs inline only. Chat Completions doesn't take a PDF by URL. When
you set a PDF's `DisplayName`, the client sends it as the file name. The client returns an error that wraps
`interfaces.ErrContentUnsupported` for any other part.

See [Content input](../../../docs/content-input.md) and the
[runnable example](../../../examples/content/openai).

## Integration with Agents

The OpenAI client can be directly used with agents:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

// Create OpenAI client
openaiClient := openai.NewClient(apiKey)

// Create agent with the OpenAI client
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    // ... other options
)
```
