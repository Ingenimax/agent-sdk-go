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

### Remote Agent Streaming with Authentication

#### Raw Event Channel Approach
```go
import "github.com/Ingenimax/agent-sdk-go/pkg/grpc/client"

// Create remote agent client
remoteClient := client.NewRemoteAgentClient(client.RemoteAgentConfig{
    URL:     "localhost:8080",
    Timeout: 5 * time.Minute,
})

// Connect to remote agent
err := remoteClient.Connect()
if err != nil {
    log.Fatal("Failed to connect:", err)
}
defer remoteClient.Disconnect()

// Stream with authentication (raw event channel)
ctx := context.Background()
authToken := "your-jwt-token-here"
events, err := remoteClient.RunStreamWithAuth(ctx, "Explain machine learning", authToken)
if err != nil {
    log.Fatal("Stream failed:", err)
}

// Process streaming events manually
for event := range events {
    switch event.Type {
    case interfaces.AgentEventContent:
        fmt.Print(event.Content)
    case interfaces.AgentEventThinking:
        fmt.Printf("\n[Thinking] %s\n", event.ThinkingStep)
    case interfaces.AgentEventToolCall:
        fmt.Printf("\n[Calling tool: %s]\n", event.ToolCall.Name)
    case interfaces.AgentEventComplete:
        fmt.Println("\n[Complete]")
    case interfaces.AgentEventError:
        fmt.Printf("\n[Error] %v\n", event.Error)
    }
}
```

#### Fluent Handler API Approach (Recommended)
```go
import "github.com/Ingenimax/agent-sdk-go/pkg/grpc/client"

// Create remote agent client
remoteClient := client.NewRemoteAgentClient(client.RemoteAgentConfig{
    URL:     "localhost:8080",
    Timeout: 5 * time.Minute,
})

// Connect to remote agent
err := remoteClient.Connect()
if err != nil {
    log.Fatal("Failed to connect:", err)
}
defer remoteClient.Disconnect()

// Set up event handlers with fluent API
remoteClient.
    OnThinking(func(thinking string) {
        fmt.Printf("\n[🤔 Thinking] %s\n", thinking)
    }).
    OnContent(func(content string) {
        fmt.Print(content)
    }).
    OnToolCall(func(toolCall *interfaces.ToolCallEvent) {
        fmt.Printf("\n[🔧 Tool] %s\n", toolCall.Name)
    }).
    OnToolResult(func(toolCall *interfaces.ToolCallEvent) {
        fmt.Printf("[✅ Result] %s\n", toolCall.Result)
    }).
    OnError(func(err error) {
        fmt.Printf("\n[❌ Error] %v\n", err)
    }).
    OnComplete(func() {
        fmt.Println("\n[✨ Done!]")
    })

// Execute with handlers and authentication
ctx := context.Background()
err = remoteClient.StreamWithAuth(ctx, "Explain machine learning", "your-jwt-token")
if err != nil {
    log.Fatal("Stream failed:", err)
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

### API Consistency

Both `AgentMicroservice` and `RemoteAgentClient` now provide the same fluent handler API:

| Method | AgentMicroservice | RemoteAgentClient | Description |
|--------|------------------|-------------------|-------------|
| `OnThinking()` | ✅ | ✅ | Handle reasoning/thinking events |
| `OnContent()` | ✅ | ✅ | Handle content/text generation |
| `OnToolCall()` | ✅ | ✅ | Handle tool execution start |
| `OnToolResult()` | ✅ | ✅ | Handle tool execution results |
| `OnError()` | ✅ | ✅ | Handle error events |
| `OnComplete()` | ✅ | ✅ | Handle completion events |
| `Stream()` | ✅ | ✅ | Execute with handlers |
| `StreamWithAuth()` | ❌ | ✅ | Execute with handlers + auth |

This means you can switch between local microservice and remote client with minimal code changes!

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
- **Remote Agents**: gRPC streaming with authentication support via `RunStreamWithAuth`

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