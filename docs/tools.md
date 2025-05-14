# Tools

This document explains how to use and create tools for the Agent SDK.

## Overview

Tools extend the capabilities of an agent by allowing it to perform actions or retrieve information from external systems. The Agent SDK provides a flexible framework for creating and using tools.

## Built-in Tools

The Agent SDK comes with several built-in tools:

### Web Search

Allows the agent to search the web for information:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"

searchTool := websearch.New(
    googleAPIKey,
    googleSearchEngineID,
)
```

### Calculator

Allows the agent to perform mathematical calculations:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"

calculatorTool := calculator.New()
```

### AWS Tools

Allows the agent to interact with AWS services:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools/aws"

// EC2 tool
ec2Tool := aws.NewEC2Tool()

// S3 tool
s3Tool := aws.NewS3Tool()
```

### Kubernetes Tools

Allows the agent to interact with Kubernetes clusters:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools/kubernetes"

kubeTool := kubernetes.New()
```

## Using Tools with an Agent

To use tools with an agent, pass them to the `WithTools` option:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

// Create tools
searchTool := websearch.New(googleAPIKey, googleSearchEngineID)
calculatorTool := calculator.New()

// Create agent with tools
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithTools(searchTool, calculatorTool),
)
```

## Creating Custom Tools

You can create custom tools by implementing the `interfaces.Tool` interface:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// WeatherTool is a custom tool for getting weather information
type WeatherTool struct {
    apiKey string
}

// NewWeatherTool creates a new weather tool
func NewWeatherTool(apiKey string) *WeatherTool {
    return &WeatherTool{
        apiKey: apiKey,
    }
}

// Name returns the name of the tool
func (t *WeatherTool) Name() string {
    return "weather"
}

// Description returns a description of what the tool does
func (t *WeatherTool) Description() string {
    return "Get current weather information for a location"
}

// Parameters returns the parameters that the tool accepts
func (t *WeatherTool) Parameters() map[string]interfaces.ParameterSpec {
    return map[string]interfaces.ParameterSpec{
        "location": {
            Type:        "string",
            Description: "The location to get weather for (e.g., 'New York', 'Tokyo')",
            Required:    true,
        },
        "units": {
            Type:        "string",
            Description: "The units to use (metric or imperial)",
            Required:    false,
            Default:     "metric",
            Enum:        []interface{}{"metric", "imperial"},
        },
    }
}

// Run executes the tool with the given input
func (t *WeatherTool) Run(ctx context.Context, input string) (string, error) {
    // Parse the input and call a weather API
    // This is a simplified example
    return "The weather in " + input + " is sunny and 25°C", nil
}

// Execute executes the tool with the given arguments
func (t *WeatherTool) Execute(ctx context.Context, args string) (string, error) {
    // Parse the JSON arguments and call a weather API
    // This is a simplified example
    return "The weather is sunny and 25°C", nil
}
```

## Tool Registry

The Tool Registry manages a collection of tools:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools"

// Create a tool registry
registry := tools.NewRegistry()

// Register tools
registry.Register(websearch.New(googleAPIKey, googleSearchEngineID))
registry.Register(calculator.New())

// Get a tool by name
tool, found := registry.Get("websearch")
if found {
    result, err := tool.Run(ctx, "latest AI news")
    // ...
}

// Get all registered tools
allTools := registry.List()
```

## Tool Execution

The Agent SDK provides a flexible way to execute tools:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools"

// Create a tool executor
executor := tools.NewExecutor(registry)

// Execute a tool by name
result, err := executor.Execute(ctx, "websearch", "latest AI news")
if err != nil {
    log.Fatalf("Failed to execute tool: %v", err)
}
fmt.Println(result)
```

## Advanced Tool Usage

### Tool with Authentication

You can create tools that require authentication:

```go
// Create a tool with authentication
type AuthenticatedTool struct {
    apiKey string
}

func (t *AuthenticatedTool) Run(ctx context.Context, input string) (string, error) {
    // Use the API key for authentication
    client := &http.Client{}
    req, err := http.NewRequestWithContext(ctx, "GET", "https://api.example.com/data", nil)
    if err != nil {
        return "", err
    }

    // Add authentication header
    req.Header.Add("Authorization", "Bearer "+t.apiKey)

    // Make the request
    resp, err := client.Do(req)
    // ...
}
```

### Tool with Rate Limiting

You can create tools with rate limiting:

```go
import (
    "context"
    "time"
    "golang.org/x/time/rate"
)

// Create a rate-limited tool
type RateLimitedTool struct {
    limiter *rate.Limiter
    tool    interfaces.Tool
}

func NewRateLimitedTool(tool interfaces.Tool, rps float64) *RateLimitedTool {
    return &RateLimitedTool{
        limiter: rate.NewLimiter(rate.Limit(rps), 1),
        tool:    tool,
    }
}

func (t *RateLimitedTool) Run(ctx context.Context, input string) (string, error) {
    // Wait for rate limit
    if err := t.limiter.Wait(ctx); err != nil {
        return "", err
    }

    // Run the underlying tool
    return t.tool.Run(ctx, input)
}

// Implement other Tool interface methods...
```

## Example: Complete Tool Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create OpenAI client
    openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

    // Create tool registry
    registry := tools.NewRegistry()

    // Register tools
    if cfg.Tools.WebSearch.GoogleAPIKey != "" && cfg.Tools.WebSearch.GoogleSearchEngineID != "" {
        searchTool := websearch.New(
            cfg.Tools.WebSearch.GoogleAPIKey,
            cfg.Tools.WebSearch.GoogleSearchEngineID,
        )
        registry.Register(searchTool)
    }

    // Register calculator tool
    registry.Register(calculator.New())

    // Create a custom weather tool
    weatherTool := NewWeatherTool("your-weather-api-key")
    registry.Register(weatherTool)

    // Create a new agent with tools
    agent, err := agent.NewAgent(
        agent.WithLLM(openaiClient),
        agent.WithMemory(memory.NewConversationBuffer()),
        agent.WithTools(registry.List()...),
        agent.WithSystemPrompt("You are a helpful AI assistant. Use tools when needed."),
    )
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Run the agent with a query that might require tools
    ctx := context.Background()
    response, err := agent.Run(ctx, "What's the weather in New York and what's 123 * 456?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }

    fmt.Println(response)
}

// WeatherTool implementation (as shown in the custom tool example)
```
