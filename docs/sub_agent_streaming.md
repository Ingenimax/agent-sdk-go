# Sub-Agent Streaming

## Overview

The Agent SDK now supports **real-time streaming from sub-agents to parent agents**. This allows you to see the sub-agent's thinking process, tool calls, and content generation in real-time as events flow through the agent hierarchy.


## How It Works

### Architecture

```
Parent Agent (RunStream)
  ↓
  Injects StreamForwarder into context
  ↓
  Calls LLM with tools (including AgentTools)
  ↓
  LLM executes AgentTool.Execute()
  ↓
  AgentTool detects StreamForwarder in context
  ↓
  Calls SubAgent.RunStream() instead of Run()
  ↓
  Forwards all sub-agent events to parent's stream
  ↓
  Parent streams everything in real-time
```

### Key Components

#### 1. **SubAgent Interface** (`pkg/tools/agent_tool.go`)

The interface now includes `RunStream()`:

```go
type SubAgent interface {
    Run(ctx context.Context, input string) (string, error)
    RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error)
    GetName() string
    GetDescription() string
}
```

All `*Agent` instances implement this interface, so they can be used as sub-agents.

#### 2. **StreamForwarder** (`pkg/interfaces/streaming.go`)

A function type that forwards stream events:

```go
type StreamForwarder func(event AgentStreamEvent)
```

This is stored in the context using `StreamForwarderKey`.

#### 3. **AgentTool Streaming** (`pkg/tools/agent_tool.go`)

The `AgentTool.Run()` method now:
- Checks for a `StreamForwarder` in the context
- If found, uses `RunStream()` and forwards all events
- If not found, falls back to blocking `Run()`

#### 4. **Context Injection** (`pkg/agent/streaming.go`)

When an agent runs with streaming, it:
- Creates a `StreamForwarder` that writes to its event channel
- Adds it to the context before calling the LLM
- The LLM passes this context when executing tools

## Usage Examples

### Basic Sub-Agent Streaming

```go
package main

import (
    "context"
    "fmt"
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
    // Create a sub-agent
    subAgent, _ := agent.NewAgent(
        agent.WithName("AnalyzerAgent"),
        agent.WithDescription("Analyzes data"),
        agent.WithLLM(openai.NewClient(openai.Config{
            APIKey: "your-api-key",
            Model:  "gpt-4",
        })),
        agent.WithStreamConfig(&interfaces.StreamConfig{
            IncludeThinking: true,
        }),
    )

    // Create parent agent with sub-agent
    parentAgent, _ := agent.NewAgent(
        agent.WithName("CoordinatorAgent"),
        agent.WithDescription("Coordinates tasks"),
        agent.WithLLM(openai.NewClient(openai.Config{
            APIKey: "your-api-key",
            Model:  "gpt-4",
        })),
        agent.WithAgents(subAgent),
        agent.WithStreamConfig(&interfaces.StreamConfig{
            IncludeThinking: true,
        }),
    )

    // Stream execution
    ctx := context.Background()
    eventChan, _ := parentAgent.RunStream(ctx, "Analyze this data and provide insights")

    // Process events in real-time
    for event := range eventChan {
        switch event.Type {
        case interfaces.AgentEventThinking:
            fmt.Printf("[THINKING] %s\n", event.ThinkingStep)
        case interfaces.AgentEventContent:
            fmt.Printf("[CONTENT] %s", event.Content)
        case interfaces.AgentEventToolCall:
            fmt.Printf("[TOOL CALL] %s\n", event.ToolCall.Name)
        case interfaces.AgentEventToolResult:
            fmt.Printf("[TOOL RESULT] %s\n", event.ToolCall.Result)
        case interfaces.AgentEventComplete:
            fmt.Println("\n[COMPLETE]")
        }
    }
}
```

### Multi-Level Nested Streaming

```go
// Create a 3-level hierarchy
level2Agent, _ := agent.NewAgent(
    agent.WithName("SpecialistAgent"),
    agent.WithLLM(llm),
)

level1Agent, _ := agent.NewAgent(
    agent.WithName("MiddleAgent"),
    agent.WithLLM(llm),
    agent.WithAgents(level2Agent),
)

mainAgent, _ := agent.NewAgent(
    agent.WithName("MainAgent"),
    agent.WithLLM(llm),
    agent.WithAgents(level1Agent),
)

// All levels stream through to the main agent!
eventChan, _ := mainAgent.RunStream(ctx, "Complex task")
```

### Manual Tool Usage (Advanced)

You can also manually add a stream forwarder when calling tools:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools"

// Create a forwarder
forwarder := func(event interfaces.AgentStreamEvent) {
    fmt.Printf("Sub-agent event: %v\n", event)
}

// Add to context
ctx := tools.WithStreamForwarder(context.Background(), forwarder)

// Execute tool with streaming
agentTool := tools.NewAgentTool(subAgent)
result, _ := agentTool.Run(ctx, "task")
```

## Event Flow Example

When a parent agent with streaming calls a sub-agent, you'll see events like:

```
[PARENT THINKING] Analyzing which sub-agent to use...
[TOOL CALL] AnalyzerAgent_agent
[SUB-AGENT THINKING] Understanding the request...
[SUB-AGENT CONTENT] Based on the analysis...
[SUB-AGENT COMPLETE]
[TOOL RESULT] Sub-agent analysis complete
[PARENT CONTENT] Here's the coordinated response...
[COMPLETE]
```

## Backward Compatibility

The implementation is fully backward compatible:

1. **Non-streaming agents**: Still work with `Run()` method
2. **Sub-agents without streaming**: Automatically fall back to `Run()`
3. **No stream forwarder**: Tools use blocking execution
4. **Existing code**: No changes needed to existing implementations

## Performance Considerations

### Benefits
- ✅ Real-time feedback for long-running sub-agents
- ✅ Better user experience with progressive updates
- ✅ Visibility into sub-agent decision-making
- ✅ Easier debugging of multi-agent systems

### Trade-offs
- Slightly more overhead from event forwarding
- Channel buffer management (default: 100 events)
- Context passing through call stack

### Optimization Tips

1. **Adjust buffer sizes** based on event volume:
   ```go
   agent.WithStreamConfig(&interfaces.StreamConfig{
       BufferSize: 500, // For high-volume streaming
   })
   ```

2. **Filter events** if you only need certain types:
   ```go
   for event := range eventChan {
       if event.Type == interfaces.AgentEventContent {
           // Only process content events
       }
   }
   ```

3. **Control thinking output**:
   ```go
   agent.WithStreamConfig(&interfaces.StreamConfig{
       IncludeThinking: false, // Reduce event volume
   })
   ```

## Testing

Comprehensive tests are available:
- Unit tests: `pkg/tools/agent_tool_test.go`
- Integration tests: `pkg/agent/subagent_streaming_test.go`
- Demo: `examples/subagents/streaming_demo.go`

Run tests:
```bash
go test ./pkg/agent/... -v -run TestSubAgentStreaming
go test ./pkg/tools/... -v
```

Run the demo:
```bash
export ANTHROPIC_API_KEY=your_api_key_here
go run examples/subagents/streaming_demo.go
```

**Note:** The demo uses Claude 3.5 Sonnet with extended thinking enabled, so you'll see actual reasoning/thinking tokens streamed in real-time!

## Implementation Details

### Files Modified

1. **`pkg/interfaces/streaming.go`**
   - Added `StreamForwarder` type
   - Added `StreamForwarderKey` context key

2. **`pkg/tools/agent_tool.go`**
   - Updated `SubAgent` interface to include `RunStream()`
   - Added `runWithStreaming()` method
   - Modified `Run()` to check for stream forwarder

3. **`pkg/agent/streaming.go`**
   - Inject stream forwarder into context before LLM execution
   - Forward sub-agent events to parent's event channel

4. **`pkg/tools/agent_tool_test.go`**
   - Updated `MockSubAgent` to implement `RunStream()`

5. **`pkg/agent/subagent_streaming_test.go`**
   - Comprehensive integration tests for streaming

6. **`examples/subagents/streaming_demo.go`**
   - Working demonstration of the feature

### Context Flow

```go
// 1. Parent agent creates forwarder
forwarder := func(event AgentStreamEvent) {
    eventChan <- event
}

// 2. Add to context
ctx = context.WithValue(ctx, StreamForwarderKey, forwarder)

// 3. LLM receives context, passes to tools
llm.GenerateWithToolsStream(ctx, prompt, tools)

// 4. Tool execution receives context
tool.Execute(ctx, args)

// 5. AgentTool detects forwarder
if fw, ok := ctx.Value(StreamForwarderKey).(StreamForwarder); ok {
    // Use streaming!
}
```

## Troubleshooting

### Sub-agent not streaming?

**Check:**
1. Sub-agent's LLM supports streaming (`SupportsStreaming() == true`)
2. Stream config is set on sub-agent
3. Parent agent is using `RunStream()` not `Run()`
4. No blocking operations in sub-agent's custom functions

### Events not appearing?

**Check:**
1. Buffer size sufficient for event volume
2. Context not being canceled prematurely
3. No deadlocks in event channel
4. Proper error handling in stream forwarder

### Performance issues?

**Check:**
1. Reduce buffer size if memory constrained
2. Filter unnecessary event types
3. Disable thinking events if not needed
4. Use appropriate stream delays in LLM

## Future Enhancements

Potential improvements:
- Event aggregation for high-volume streams
- Priority-based event forwarding
- Selective event filtering at tool level
- Stream multiplexing for parallel sub-agents
- Event replay and debugging tools

## Summary

Sub-agent streaming transforms the agent SDK from a black-box execution model to a transparent, real-time system where you can observe every step of the decision-making process across multiple agent levels. This is crucial for:

- **User Experience**: Progressive feedback instead of long waits
- **Debugging**: Visibility into sub-agent reasoning
- **Monitoring**: Real-time observability of agent hierarchies
- **Trust**: Understanding what agents are doing in real-time

The implementation is backward compatible, performant, and easy to use!
