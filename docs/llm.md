# LLM Providers

This document explains how to use the LLM (Large Language Model) providers in the Agent SDK.

## Overview

The Agent SDK supports multiple LLM providers, including OpenAI, Anthropic, and Azure OpenAI. Each provider has its own implementation but shares a common interface.

## Supported Providers

### OpenAI

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create OpenAI client
client := openai.NewClient(cfg.LLM.OpenAI.APIKey)

// Optional: Configure the client
client = openai.NewClient(
    cfg.LLM.OpenAI.APIKey,
    openai.WithModel("gpt-4o-mini"),
    openai.WithTemperature(0.7),
)
```

### Anthropic

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create Anthropic client
client := anthropic.NewClient(cfg.LLM.Anthropic.APIKey)

// Optional: Configure the client
client = anthropic.NewClient(
    cfg.LLM.Anthropic.APIKey,
    anthropic.WithModel("claude-3-opus-20240229"),
    anthropic.WithTemperature(0.7),
)
```

### Azure OpenAI

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/azure"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create Azure OpenAI client
client := azure.NewClient(
    cfg.LLM.AzureOpenAI.APIKey,
    cfg.LLM.AzureOpenAI.Endpoint,
    cfg.LLM.AzureOpenAI.Deployment,
)

// Optional: Configure the client
client = azure.NewClient(
    cfg.LLM.AzureOpenAI.APIKey,
    cfg.LLM.AzureOpenAI.Endpoint,
    cfg.LLM.AzureOpenAI.Deployment,
    azure.WithAPIVersion("2023-05-15"),
    azure.WithTemperature(0.7),
)
```

## Using LLM Providers

### Text Generation

Generate text based on a prompt:

```go
import "context"

// Generate text
response, err := client.Generate(context.Background(), "What is the capital of France?")
if err != nil {
    log.Fatalf("Failed to generate text: %v", err)
}
fmt.Println(response)
```

### Chat Completion

Generate a response to a conversation:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm"
)

// Create messages
messages := []llm.Message{
    {
        Role:    "system",
        Content: "You are a helpful AI assistant.",
    },
    {
        Role:    "user",
        Content: "What is the capital of France?",
    },
}

// Generate chat completion
response, err := client.Chat(context.Background(), messages)
if err != nil {
    log.Fatalf("Failed to generate chat completion: %v", err)
}
fmt.Println(response)
```

### Generation with Tools

Generate a response that can use tools:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

// Create tools
searchTool := websearch.New(googleAPIKey, googleSearchEngineID)

// Generate with tools
response, err := client.GenerateWithTools(
    context.Background(),
    "What's the weather in San Francisco?",
    []interfaces.Tool{searchTool},
)
if err != nil {
    log.Fatalf("Failed to generate with tools: %v", err)
}
fmt.Println(response)
```

## Configuration Options

### Common Options

These options are available for all LLM providers:

```go
// Temperature controls randomness (0.0 to 1.0)
WithTemperature(0.7)

// TopP controls diversity via nucleus sampling
WithTopP(0.9)

// FrequencyPenalty reduces repetition of token sequences
WithFrequencyPenalty(0.0)

// PresencePenalty reduces repetition of topics
WithPresencePenalty(0.0)

// StopSequences specifies sequences that stop generation
WithStopSequences([]string{"###"})

// Reasoning controls how the model explains its thinking
// Options: "none", "minimal", "comprehensive"
WithReasoning("minimal")
```

### Reasoning Levels

The reasoning feature allows you to control how extensively the model explains its thought process:

- **none**: The model provides direct, concise answers without explaining its reasoning or showing calculations. This produces the shortest responses.
- **minimal**: The model briefly explains its thought process along with the answer, showing basic working but keeping explanations concise.
- **comprehensive**: The model provides a detailed step-by-step explanation of its reasoning process with thorough explanations of each step.

Example:

```go
// Generate with minimal reasoning
response, err := client.Generate(
    context.Background(),
    "What is the probability of rolling a sum of 7 with two standard dice?",
    openai.WithReasoning("minimal"),
)

// Generate with comprehensive reasoning using Chat method
messages := []llm.Message{
    {
        Role:    "system",
        Content: "You are a helpful math tutor.",
    },
    {
        Role:    "user",
        Content: "Explain the Pythagorean theorem.",
    },
}

response, err := client.Chat(
    context.Background(),
    messages,
    &llm.GenerateParams{
        Temperature: 0.3,
        Reasoning:   "comprehensive",
    },
)
```

### Provider-Specific Options

#### OpenAI

```go
// Model specifies which model to use
openai.WithModel("gpt-4")

// BaseURL specifies a custom API endpoint
openai.WithBaseURL("https://api.openai.com/v1")

// Timeout specifies the request timeout
openai.WithTimeout(60 * time.Second)

// Reasoning controls how the model explains its thinking
openai.WithReasoning("comprehensive") // Options: "none", "minimal", "comprehensive"
```

#### Anthropic

```go
// Model specifies which model to use
anthropic.WithModel("claude-3-opus-20240229")

// BaseURL specifies a custom API endpoint
anthropic.WithBaseURL("https://api.anthropic.com")

// Timeout specifies the request timeout
anthropic.WithTimeout(60 * time.Second)

// Reasoning controls how the model explains its thinking
anthropic.WithReasoning("comprehensive") // Options: "none", "minimal", "comprehensive"
```
## Multi-tenancy with LLM Providers

When using LLM providers with multi-tenancy, you can specify the organization ID:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create context with organization ID
ctx := context.Background()
ctx = multitenancy.WithOrgID(ctx, "org-123")

// Generate text for this organization
response, err := client.Generate(ctx, "What is the capital of France?")
```

## Implementing Custom LLM Providers

You can implement custom LLM providers by implementing the `interfaces.LLM` interface:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// CustomLLM is a custom LLM implementation
type CustomLLM struct {
    // Add your fields here
}

// NewCustomLLM creates a new custom LLM
func NewCustomLLM() *CustomLLM {
    return &CustomLLM{}
}

// Generate generates text based on the provided prompt
func (l *CustomLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
    // Apply options
    opts := &interfaces.GenerateOptions{}
    for _, option := range options {
        option(opts)
    }

    // Implement your generation logic here
    return "Generated text", nil
}

// GenerateWithTools generates text and can use tools
func (l *CustomLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
    // Apply options
    opts := &interfaces.GenerateOptions{}
    for _, option := range options {
        option(opts)
    }

    // Implement your generation with tools logic here
    return "Generated text with tools", nil
}

// Name returns the name of the LLM provider
func (l *CustomLLM) Name() string {
    return "custom-llm"
}
```

## Example: Complete LLM Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/config"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/azure"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create context
    ctx := context.Background()

    // Create LLM client based on configuration
    var client llm.Provider
    switch cfg.LLM.Provider {
    case "openai":
        client = openai.NewClient(
            cfg.LLM.OpenAI.APIKey,
            openai.WithModel(cfg.LLM.OpenAI.Model),
            openai.WithTemperature(cfg.LLM.OpenAI.Temperature),
        )
    case "anthropic":
        client = anthropic.NewClient(
            cfg.LLM.Anthropic.APIKey,
            anthropic.WithModel(cfg.LLM.Anthropic.Model),
            anthropic.WithTemperature(cfg.LLM.Anthropic.Temperature),
        )
    case "azure":
        client = azure.NewClient(
            cfg.LLM.AzureOpenAI.APIKey,
            cfg.LLM.AzureOpenAI.Endpoint,
            cfg.LLM.AzureOpenAI.Deployment,
            azure.WithAPIVersion(cfg.LLM.AzureOpenAI.APIVersion),
            azure.WithTemperature(cfg.LLM.AzureOpenAI.Temperature),
        )
    default:
        log.Fatalf("Unsupported LLM provider: %s", cfg.LLM.Provider)
    }

    // Generate text
    response, err := client.Generate(ctx, "What is the capital of France?")
    if err != nil {
        log.Fatalf("Failed to generate text: %v", err)
    }
    fmt.Println("Generated text:", response)

    // Generate chat completion
    messages := []llm.Message{
        {
            Role:    "system",
            Content: "You are a helpful AI assistant.",
        },
        {
            Role:    "user",
            Content: "What is the capital of Germany?",
        },
    }
    chatResponse, err := client.Chat(ctx, messages)
    if err != nil {
        log.Fatalf("Failed to generate chat completion: %v", err)
    }
    fmt.Println("Chat response:", chatResponse)
}
