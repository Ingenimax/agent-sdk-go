# Tracing

This document explains how to use the Tracing component of the Agent SDK.

## Overview

Tracing provides observability into the behavior of your agents, allowing you to monitor, debug, and analyze their performance. The Agent SDK supports multiple tracing backends, including Langfuse and OpenTelemetry.

## Enabling Tracing

### Langfuse

[Langfuse](https://langfuse.com/) is a specialized observability platform for LLM applications:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/tracing"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create Langfuse tracer
langfuseTracer, err := tracing.NewLangfuseTracer(tracing.LangfuseConfig{
    Enabled:     cfg.Tracing.Langfuse.Enabled,
    SecretKey:   cfg.Tracing.Langfuse.SecretKey,
    PublicKey:   cfg.Tracing.Langfuse.PublicKey,
    Host:        cfg.Tracing.Langfuse.Host,
    Environment: cfg.Tracing.Langfuse.Environment,
})
if err != nil {
    log.Fatalf("Failed to create Langfuse tracer: %v", err)
}

// Convert to unified interface
tracer := langfuseTracer.AsInterfaceTracer()
```

### OpenTelemetry

[OpenTelemetry](https://opentelemetry.io/) is a vendor-neutral observability framework:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/tracing"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create OpenTelemetry tracer using the unified interface
tracer, err := tracing.NewOTelTracer(tracing.OTelConfig{
    Enabled:          cfg.Tracing.OpenTelemetry.Enabled,
    ServiceName:      cfg.Tracing.OpenTelemetry.ServiceName,
    CollectorEndpoint: cfg.Tracing.OpenTelemetry.CollectorEndpoint,
})
if err != nil {
    log.Fatalf("Failed to create OpenTelemetry tracer: %v", err)
}
```

## Using Tracing with an Agent

To use tracing with an agent, pass it to the `WithTracer` option:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/tracing"
)

// Create Langfuse tracer
langfuseTracer, err := tracing.NewLangfuseTracer(tracing.LangfuseConfig{
    Enabled:     true,
    SecretKey:   secretKey,
    PublicKey:   publicKey,
    Host:        "https://cloud.langfuse.com",
    Environment: "development",
})
if err != nil {
    log.Fatalf("Failed to create Langfuse tracer: %v", err)
}

// Convert to unified interface
tracer := langfuseTracer.AsInterfaceTracer()

// Create agent with tracer
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithTracer(tracer),
)
```

## Manual Tracing

You can also use the tracer directly for manual instrumentation:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Start a trace
ctx, span := tracer.StartSpan(context.Background(), "my-operation")
defer span.End()

// Add attributes to the span
span.SetAttribute("key", "value")

// Record events
span.AddEvent("something-happened")

// Record errors
span.RecordError(err)
```

## Unified Tracing Middleware

The Agent SDK provides unified tracing middleware that works with any tracer implementing the `interfaces.Tracer` interface:

### LLM Tracing Middleware

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/tracing"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

// Create LLM client
llm := openai.NewClient(apiKey)

// Wrap with unified tracing middleware
tracedLLM := tracing.NewTracedLLM(llm, tracer)

// Use with agent
agent, err := agent.NewAgent(
    agent.WithLLM(tracedLLM),
    // ... other options
)
```

### Memory Tracing Middleware

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/tracing"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// Create memory store
mem := memory.NewConversationBuffer()

// Wrap with unified tracing middleware
tracedMemory := tracing.NewTracedMemory(mem, tracer)

// Use with agent
agent, err := agent.NewAgent(
    agent.WithMemory(tracedMemory),
    // ... other options
)
```

### Migration Guide (deprecated APIs)

The following legacy helpers are deprecated in favor of the NewTracedLLM and NewTracedMemory. Please migrate as shown below.

- Old: `tracing.NewLLMMiddleware(llm interfaces.LLM, tracer *LangfuseTracer)`
- Replace with: `tracing.NewTracedLLM(llm, langfuseTracer.AsInterfaceTracer())`

```go
// Before (deprecated)
wrapped := tracing.NewLLMMiddleware(llm, langfuseTracer)

// After
tracer := langfuseTracer.AsInterfaceTracer()
wrapped := tracing.NewTracedLLM(llm, tracer)
```

- Old: `tracing.NewOTELLLMMiddleware(llm interfaces.LLM, tracer *OTELLangfuseTracer)`
- Replace with: `tracing.NewTracedLLM(llm, tracer)`

```go
// Before (deprecated)
wrapped := tracing.NewOTELLLMMiddleware(llm, otelLangfuseTracer)

// After
wrapped := tracing.NewTracedLLM(llm, otelLangfuseTracer)
```

- Old: `tracing.NewMemoryOTelMiddleware(memory interfaces.Memory, tracer *OTelTracer)`
- Replace with: `tracing.NewTracedMemory(memory, tracer)`

```go
// Before (deprecated)
tracedMemory := tracing.NewMemoryOTelMiddleware(mem, otelTracer)

// After
tracedMemory := tracing.NewTracedMemory(mem, otelTracer)
```

Notes:
- Prefer creating a single `interfaces.Tracer` and pass it to all traced components via `NewTracedLLM` and `NewTracedMemory`.
- When using `LangfuseTracer`, call `AsInterfaceTracer()` to obtain the unified tracer.

## Tracing LLM Calls

The Agent SDK automatically traces LLM calls when a tracer is configured:

```go
// The agent will automatically trace LLM calls
response, err := agent.Run(ctx, "What is the capital of France?")
```

You can also manually trace LLM calls:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm"
)

// Start a trace for the LLM call
ctx, span := tracer.StartSpan(ctx, "llm-generate")
defer span.End()

// Set LLM-specific attributes (privacy-safe)
span.SetAttribute("llm.model", "gpt-4")
span.SetAttribute("llm.prompt.length", len(prompt))
span.SetAttribute("llm.prompt.hash", hashString(prompt))

// Make the LLM call
response, err := client.Generate(ctx, prompt)

// Record the response (privacy-safe)
span.SetAttribute("llm.response.length", len(response))
span.SetAttribute("llm.response.hash", hashString(response))
if err != nil {
    span.RecordError(err)
}
```

## Tracing Tool Calls

The Agent SDK automatically traces tool calls when a tracer is configured:

```go
// The agent will automatically trace tool calls
response, err := agent.Run(ctx, "What's the weather in San Francisco?")
```

You can also manually trace tool calls:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Start a trace for the tool call
ctx, span := tracer.StartSpan(ctx, "tool-execute")
defer span.End()

// Set tool-specific attributes
span.SetAttribute("tool.name", tool.Name())
span.SetAttribute("tool.input", input)

// Execute the tool
result, err := tool.Run(ctx, input)

// Record the result
span.SetAttribute("tool.result", result)
if err != nil {
    span.RecordError(err)
}
```

## Multi-tenancy with Tracing

When using tracing with multi-tenancy, you can include the organization ID in the traces:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create context with organization ID
ctx := context.Background()
ctx = multitenancy.WithOrgID(ctx, "org-123")

// The organization ID will be included in the traces
response, err := agent.Run(ctx, "What is the capital of France?")
```

## Viewing Traces

### Langfuse

To view traces in Langfuse:

1. Log in to your Langfuse account at https://cloud.langfuse.com
2. Navigate to the "Traces" section
3. Filter and search for your traces

### OpenTelemetry

To view OpenTelemetry traces, you need a compatible backend such as Jaeger, Zipkin, or a cloud observability platform:

1. Configure your OpenTelemetry collector to send traces to your backend
2. Access your backend's UI to view and analyze traces

## Creating Custom Tracers

You can implement custom tracers by implementing the `interfaces.Tracer` interface:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// CustomTracer is a custom tracer implementation
type CustomTracer struct {
    // Add your fields here
}

// NewCustomTracer creates a new custom tracer
func NewCustomTracer() *CustomTracer {
    return &CustomTracer{}
}

// StartSpan starts a new span
func (t *CustomTracer) StartSpan(ctx context.Context, name string) (context.Context, interfaces.Span) {
    // Implement your logic to start a span
    return ctx, &CustomSpan{}
}

// CustomSpan is a custom span implementation
type CustomSpan struct {
    // Add your fields here
}

// SetAttribute sets an attribute on the span
func (s *CustomSpan) SetAttribute(key string, value interface{}) {
    // Implement your logic to set an attribute
}

// AddEvent adds an event to the span
func (s *CustomSpan) AddEvent(name string) {
    // Implement your logic to add an event
}

// RecordError records an error on the span
func (s *CustomSpan) RecordError(err error) {
    // Implement your logic to record an error
}

// End ends the span
func (s *CustomSpan) End() {
    // Implement your logic to end the span
}
```

## Example: Complete Tracing Setup

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
    "github.com/Ingenimax/agent-sdk-go/pkg/tracing"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create OpenAI client
    openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

    // Create Langfuse tracer
    langfuseTracer, err := tracing.NewLangfuseTracer(tracing.LangfuseConfig{
        Enabled:     cfg.Tracing.Langfuse.Enabled,
        SecretKey:   cfg.Tracing.Langfuse.SecretKey,
        PublicKey:   cfg.Tracing.Langfuse.PublicKey,
        Host:        cfg.Tracing.Langfuse.Host,
        Environment: cfg.Tracing.Langfuse.Environment,
    })
    if err != nil {
        log.Fatalf("Failed to create Langfuse tracer: %v", err)
    }

    // Convert to unified interface
    tracer := langfuseTracer.AsInterfaceTracer()

    // Create tools
    searchTool := websearch.New(
        cfg.Tools.WebSearch.GoogleAPIKey,
        cfg.Tools.WebSearch.GoogleSearchEngineID,
    )

    // Wrap LLM and memory with unified tracing middleware
    tracedLLM := tracing.NewTracedLLM(openaiClient, tracer)
    tracedMemory := tracing.NewTracedMemory(memory.NewConversationBuffer(), tracer)

    // Create a new agent with traced components
    agent, err := agent.NewAgent(
        agent.WithLLM(tracedLLM),
        agent.WithMemory(tracedMemory),
        agent.WithTools(searchTool),
        agent.WithSystemPrompt("You are a helpful AI assistant. Use tools when needed."),
    )
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Create context with trace session
    ctx := context.Background()
    ctx, span := tracer.StartTraceSession(ctx, "user-session-123")
    defer span.End()

    // Add session attributes
    span.SetAttribute("session.id", "session-123")
    span.SetAttribute("user.id", "user-456")

    // Run the agent
    response, err := agent.Run(ctx, "What's the latest news about artificial intelligence?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
        span.RecordError(err)
    }

    // Record the response (privacy-safe)
    span.SetAttribute("response.length", len(response))
    span.SetAttribute("response.hash", tracing.HashString(response))

    fmt.Println(response)
}
