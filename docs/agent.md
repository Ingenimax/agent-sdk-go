# Agent

This document explains how to use the Agent component of the Agent SDK.

## Overview

The Agent is the core component of the SDK that coordinates the LLM, memory, and tools to create an intelligent assistant that can understand and respond to user queries.

## Creating an Agent

To create a new agent, use the `NewAgent` function with various options:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// Create a new agent
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithSystemPrompt("You are a helpful AI assistant."),
)
if err != nil {
    log.Fatalf("Failed to create agent: %v", err)
}
```

## Agent Options

The Agent can be configured with various options:

### WithLLM

Sets the LLM provider for the agent:

```go
agent.WithLLM(openaiClient)
```

### WithMemory

Sets the memory system for the agent:

```go
agent.WithMemory(memory.NewConversationBuffer())
```

### WithTools

Adds tools to the agent:

```go
agent.WithTools(
    websearch.New(googleAPIKey, googleSearchEngineID),
    calculator.New(),
)
```

### WithSystemPrompt

Sets the system prompt for the agent:

```go
agent.WithSystemPrompt("You are a helpful AI assistant specialized in answering questions about science.")
```

### WithOrgID

Sets the organization ID for multi-tenancy:

```go
agent.WithOrgID("org-123")
```

### WithTracer

Sets the tracer for observability:

```go
agent.WithTracer(langfuse.New(langfuseSecretKey, langfusePublicKey))
```

### WithGuardrails

Sets the guardrails for safety:

```go
agent.WithGuardrails(guardrails.New(guardrailsConfigPath))
```

## Running the Agent

To run the agent with a user query:

```go
response, err := agent.Run(ctx, "What is the capital of France?")
if err != nil {
    log.Fatalf("Failed to run agent: %v", err)
}
fmt.Println(response)
```

## Streaming Responses

To stream the agent's response:

```go
stream, err := agent.RunStream(ctx, "Tell me a long story about a dragon")
if err != nil {
    log.Fatalf("Failed to run agent with streaming: %v", err)
}

for {
    chunk, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatalf("Error receiving stream: %v", err)
    }
    fmt.Print(chunk)
}
```

## Using Tools

The agent can use tools to perform actions or retrieve information:

```go
// Create tools
searchTool := websearch.New(googleAPIKey, googleSearchEngineID)
calculatorTool := calculator.New()

// Create agent with tools
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithTools(searchTool, calculatorTool),
    agent.WithSystemPrompt("You are a helpful AI assistant. Use tools when needed."),
)

// Run the agent with a query that might require tools
response, err := agent.Run(ctx, "What is the population of Tokyo multiplied by 2?")
```

## Advanced Usage

### Custom Tool Execution

You can implement custom tool execution logic:

```go
// Create a custom tool executor
executor := agent.NewToolExecutor(func(ctx context.Context, toolName string, input string) (string, error) {
    // Custom logic for executing tools
    if toolName == "custom_tool" {
        // Do something special
        return "Custom result", nil
    }

    // Fall back to default execution for other tools
    tool, found := toolRegistry.Get(toolName)
    if !found {
        return "", fmt.Errorf("tool not found: %s", toolName)
    }
    return tool.Run(ctx, input)
})

// Create agent with custom tool executor
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithTools(searchTool, calculatorTool),
    agent.WithToolExecutor(executor),
)
```

### Custom Message Processing

You can implement custom message processing:

```go
// Create a custom message processor
processor := agent.NewMessageProcessor(func(ctx context.Context, message interfaces.Message) (interfaces.Message, error) {
    // Process the message
    if message.Role == "user" {
        // Add metadata to user messages
        if message.Metadata == nil {
            message.Metadata = make(map[string]interface{})
        }
        message.Metadata["processed_at"] = time.Now()
    }
    return message, nil
})

// Create agent with custom message processor
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithMessageProcessor(processor),
)
```

## Example: Complete Agent Setup

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
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
    "github.com/Ingenimax/agent-sdk-go/pkg/tracing/langfuse"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create OpenAI client
    openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

    // Create tools
    searchTool := websearch.New(
        cfg.Tools.WebSearch.GoogleAPIKey,
        cfg.Tools.WebSearch.GoogleSearchEngineID,
    )

    // Create tracer
    tracer := langfuse.New(
        cfg.Tracing.Langfuse.SecretKey,
        cfg.Tracing.Langfuse.PublicKey,
    )

    // Create a new agent
    agent, err := agent.NewAgent(
        agent.WithLLM(openaiClient),
        agent.WithMemory(memory.NewConversationBuffer()),
        agent.WithTools(searchTool),
        agent.WithTracer(tracer),
        agent.WithSystemPrompt("You are a helpful AI assistant. Use tools when needed."),
    )
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Run the agent
    ctx := context.Background()
    response, err := agent.Run(ctx, "What's the latest news about artificial intelligence?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }

    fmt.Println(response)
}
```
