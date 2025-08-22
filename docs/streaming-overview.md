# Agent SDK Streaming Overview

## What is Streaming?

Streaming provides real-time response generation with Server-Sent Events (SSE) from multiple LLM providers (Anthropic and OpenAI). Instead of waiting for complete responses, you see tokens as they're generated.

## Key Benefits

- **Real-time feedback**: Users see responses as they're generated
- **Reduced perceived latency**: First tokens arrive faster than waiting for complete responses
- **Thinking visibility**: Expose Claude's reasoning/thinking process through events
- **Tool execution progress**: Show tool calls and results as they happen
- **Better debugging**: Enhanced visibility into agent decision-making process

## Architecture Overview

### Multi-LLM Streaming Support
```
┌─────────────┐     SSE      ┌──────────────┐
│  Anthropic  │◄─────────────┤ Anthropic    │
│     API     │              │   Client     │
└─────────────┘              └──────┬───────┘
                                     │
┌─────────────┐     SSE      ┌──────┼───────┐
│   OpenAI    │◄─────────────┤   OpenAI     │
│     API     │              │   Client     │
└─────────────┘              └──────┬───────┘
                                     │ Channel (unified interface)
                              ┌──────▼───────┐
                              │    Agent     │
                              │  RunStream   │
                              └──────┬───────┘
                                     │ Channel
                              ┌──────▼───────┐
                              │ gRPC Server  │
                              │  RunStream   │
                              └──────┬───────┘
                                     │ gRPC Stream (internal)
                              ┌──────▼───────┐
                              │ gRPC Client  │
                              └──────┬───────┘
                                     │
                              ┌──────▼───────┐
                              │ HTTP/SSE     │
                              │   Server     │
                              └──────┬───────┘
                                     │ SSE (browser-friendly)
                              ┌──────▼───────┐
                              │Browser/Web   │
                              │   Client     │
                              └──────────────┘
```

### Communication Protocols
- **Agent-to-Agent**: gRPC streaming (binary, efficient, type-safe)
- **Web Clients**: HTTP with SSE (browser-native, proxy-friendly)
- **LLM Providers**: Provider-specific SSE implementations

## Quick Start

### Basic Streaming Example
```go
// Create streaming-capable LLM
llm := anthropic.NewClient(apiKey, anthropic.WithModel(anthropic.Claude35Sonnet))

// Create agent
agent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithSystemPrompt("You are a helpful assistant."),
)

// Stream response
ctx := context.Background()
events, err := agent.RunStream(ctx, "Explain quantum computing step by step")

// Process streaming events
for event := range events {
    switch event.Type {
    case agent.EventContent:
        fmt.Print(event.Content)
    case agent.EventThinking:
        fmt.Printf("\n[Thinking] %s\n", event.ThinkingStep)
    case agent.EventToolCall:
        fmt.Printf("\n[Calling tool: %s]\n", event.ToolCall.Name)
    case agent.EventComplete:
        fmt.Println("\n[Complete]")
    }
}
```

### Microservice Streaming
```go
// Create and start microservice
service, err := microservice.CreateMicroservice(agent, microservice.Config{Port: 0})
service.Start()

// Simple streaming with event handlers
service.
    OnThinking(func(thinking string) {
        fmt.Printf("<thinking>%s</thinking>\n", thinking)
    }).
    OnContent(func(content string) {
        fmt.Printf("%s", content)
    }).
    OnComplete(func() {
        fmt.Println("Done!")
    })

// Execute with handlers
err := service.Stream(ctx, "Your question here")
```

## Core Concepts

### Event Types
- **Content**: Regular response content from the LLM
- **Thinking**: Reasoning process (Claude Extended Thinking, o1 reasoning)
- **Tool Call**: Tool execution with progress tracking
- **Tool Result**: Results from tool execution
- **Error**: Error conditions during streaming
- **Complete**: Stream completion signal

### Provider Support
- **Anthropic Claude**: Full SSE support with Extended Thinking
- **OpenAI GPT**: Delta streaming with reasoning models (o1, o4)
- **Reasoning Models**: Automatic parameter handling for temperature and tools

## Related Documentation

- [Extended Thinking Guide](./extended-thinking.md) - Claude's reasoning visibility
- [Reasoning Models Guide](./reasoning-models.md) - OpenAI o1/o4 model support
- [Microservice Streaming API](./microservice-streaming.md) - Simplified streaming APIs
- [Implementation Details](./streaming-implementation.md) - Technical architecture
- [Migration Guide](./streaming-migration.md) - Upgrading from non-streaming

## Migration from Non-Streaming

Streaming is backward compatible:
```go
// Existing code continues to work
response, err := agent.Run(ctx, "prompt")

// New streaming option
events, err := agent.RunStream(ctx, "prompt")
```

## Performance

- **Buffer Size**: Default 100 events, configurable
- **Latency**: First tokens arrive in ~100-200ms
- **Memory**: Minimal overhead with channel-based streaming
- **Throughput**: Handles 100+ concurrent streams

## Future Enhancements

- **Event Filtering**: Subscribe to specific event types
- **WebSocket Support**: Bidirectional communication
- **Compression**: gRPC compression for bandwidth optimization
- **Metrics**: Built-in streaming performance metrics