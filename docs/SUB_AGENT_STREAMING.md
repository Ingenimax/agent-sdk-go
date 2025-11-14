# Sub-Agent Streaming in Agent SDK Go

## Overview

The Agent SDK Go supports **real-time streaming** of sub-agent execution. When a parent agent delegates tasks to sub-agents, the sub-agent's output (including thinking steps, tool calls, and content) is streamed back to the parent agent in real-time rather than being returned as a single text block at the end.

## How It Works

### Architecture

```
Parent Agent (streaming)
    ↓ RunStream(ctx, input)
    ↓ Adds event channel to context
    ↓
LLM calls AgentTool.Execute(ctx, args)
    ↓ Checks if sub-agent supports streaming
    ↓ Checks if context has event channel
    ↓
Sub-Agent.RunStream(ctx, input)
    ↓ Generates events (thinking, tool calls, content)
    ↓ Events forwarded to parent's channel
    ↓
Parent Agent streams all events to user
```

### Key Components

1. **StreamingSubAgent Interface** (`pkg/tools/agent_tool.go`)
   ```go
   type StreamingSubAgent interface {
       SubAgent
       RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error)
   }
   ```

2. **Agent Implementation**
   - The `*Agent` type automatically implements `StreamingSubAgent`
   - Has both `Run()` (blocking) and `RunStream()` (streaming) methods

3. **AgentTool Wrapper**
   - Wraps sub-agents as tools that can be called by parent agents
   - Automatically detects if streaming is possible
   - Forwards sub-agent events to parent's event channel

4. **Context Propagation**
   - Parent agent adds its event channel to the context
   - AgentTool checks for this channel and uses it if present

## Requirements for Streaming

For sub-agent streaming to work, **ALL** of the following must be true:

1. ✅ **Parent agent must be called with `RunStream()`**
   - Using `Run()` will not enable streaming

2. ✅ **Sub-agent must implement `StreamingSubAgent`**
   - The `*Agent` type automatically implements this

3. ✅ **LLM must support streaming**
   - Anthropic Claude ✅
   - OpenAI GPT ✅
   - Azure OpenAI ✅
   - Gemini ✅
   - Custom LLM must implement `interfaces.StreamingLLM`

4. ✅ **StreamConfig should be set** (optional but recommended)
   ```go
   streamConfig := &interfaces.StreamConfig{
       ThinkingEnabled: true,
   }
   agent.WithStreamConfig(streamConfig)
   ```

## Usage Example

### Basic Sub-Agent Setup

```go
package main

import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

func main() {
    ctx := context.Background()
    llm := anthropic.NewClient(apiKey)

    // Configure LLM with reasoning
    llmConfig := &interfaces.LLMConfig{
        ReasoningEffort: interfaces.ReasoningEffortHigh,
    }

    // Enable streaming with thinking
    streamConfig := &interfaces.StreamConfig{
        ThinkingEnabled: true,
    }

    // Create sub-agent
    subAgent, _ := agent.NewAgent(
        agent.WithLLM(llm),
        agent.WithMemory(memory.NewInMemory()),
        agent.WithName("MathAgent"),
        agent.WithDescription("Specialized math agent"),
        agent.WithSystemPrompt("You are a math specialist..."),
        agent.WithRequirePlanApproval(false),
        agent.WithLLMConfig(*llmConfig),
        agent.WithStreamConfig(streamConfig),  // Enable streaming
    )

    // Create parent agent with sub-agent
    parentAgent, _ := agent.NewAgent(
        agent.WithLLM(llm),
        agent.WithMemory(memory.NewInMemory()),
        agent.WithName("Coordinator"),
        agent.WithDescription("Main coordinator"),
        agent.WithSystemPrompt("You coordinate tasks..."),
        agent.WithRequirePlanApproval(false),
        agent.WithAgents(subAgent),  // Sub-agent wrapped as tool
        agent.WithLLMConfig(*llmConfig),
        agent.WithStreamConfig(streamConfig),  // Enable streaming
    )

    // Run with streaming (IMPORTANT!)
    eventChan, _ := parentAgent.RunStream(ctx, "Calculate 157 × 234")

    // Process events
    for event := range eventChan {
        // Check if event is from sub-agent
        if subAgentName, ok := event.Metadata["sub_agent"].(string); ok {
            // This event came from a sub-agent
            handleSubAgentEvent(subAgentName, event)
        } else {
            // This event came from parent agent
            handleParentEvent(event)
        }
    }
}
```

### Processing Sub-Agent Events

Sub-agent events have special metadata:

```go
for event := range eventChan {
    switch event.Type {
    case interfaces.AgentEventThinking:
        if subAgent, ok := event.Metadata["sub_agent"].(string); ok {
            fmt.Printf("[SUB-AGENT: %s THINKING] %s\n", subAgent, event.ThinkingStep)
        }

    case interfaces.AgentEventContent:
        if subAgent, ok := event.Metadata["sub_agent"].(string); ok {
            fmt.Printf("[SUB-AGENT: %s] %s", subAgent, event.Content)
        }

    case interfaces.AgentEventToolCall:
        if subAgent, ok := event.Metadata["sub_agent"].(string); ok {
            fmt.Printf("[SUB-AGENT: %s TOOL] %s\n", subAgent, event.ToolCall.Name)
        }
    }
}
```

## Troubleshooting

### Problem: Sub-agent output only appears at the end

**Symptoms:**
- Sub-agent executes correctly
- Sub-agent generates thinking and output
- BUT all appears as a single text block at the end

**Solution:** Check the logs for this message:
```
Using NON-STREAMING execution for sub-agent
```

Look at the `fallback_reason` in the logs:

1. **"agent doesn't support streaming"**
   - The sub-agent doesn't implement `StreamingSubAgent`
   - Solution: Ensure you're using `*Agent` from this SDK

2. **"no stream context from parent"**
   - Parent agent didn't add event channel to context
   - Solution: Call parent with `RunStream()` instead of `Run()`

3. **"agent doesn't support streaming and no stream context"**
   - Both issues above
   - Solution: Use `RunStream()` on parent and ensure sub-agent is `*Agent`

### Problem: Events are being dropped

**Symptoms:**
```
Parent event channel full, dropping sub-agent event
```

**Solution:**
- The event channel buffer is full
- Parent is not consuming events fast enough
- Consider increasing buffer size in `StreamConfig`
- Ensure your event processing loop is non-blocking

### Problem: No sub-agent tool calls

**Symptoms:**
- No tool calls detected
- Parent agent doesn't delegate to sub-agent

**Solutions:**
1. Check parent's system prompt - make it clear when to use sub-agent
2. Verify sub-agent tool is registered: `len(parentAgent.GetTools())`
3. Check LLM is actually calling the tool (review logs)

## Logging and Debugging

### Enable Detailed Logging

The SDK includes detailed logging for debugging sub-agent streaming:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/logging"

logger := logging.New()
logger.SetLevel(logging.LevelDebug)

agent.WithLogger(logger)
```

### Key Log Messages

1. **Streaming Capability Check**
   ```
   Sub-agent streaming capability check
   - supports_stream: true/false
   - has_stream_ctx: true/false
   ```

2. **Streaming Execution**
   ```
   Using STREAMING execution for sub-agent
   - message: Sub-agent output will be streamed to parent in real-time
   ```

3. **Event Forwarding**
   ```
   Forwarded sub-agent event to parent
   - event_type: content/thinking/tool_call
   - event_num: 1, 2, 3...
   ```

4. **Completion Summary**
   ```
   Sub-agent streaming completed
   - events_received: 50
   - events_forward: 50
   - events_dropped: 0
   ```

## Best Practices

1. **Always Use RunStream() for Real-time Output**
   ```go
   // Good
   eventChan, _ := agent.RunStream(ctx, input)

   // Bad (for streaming)
   result, _ := agent.Run(ctx, input)
   ```

2. **Configure Streaming on Both Parent and Sub-Agent**
   ```go
   streamConfig := &interfaces.StreamConfig{
       ThinkingEnabled: true,
   }
   // Apply to both parent and sub-agent
   ```

3. **Process Events Asynchronously**
   ```go
   go func() {
       for event := range eventChan {
           // Process event
       }
   }()
   ```

4. **Check Sub-Agent Metadata**
   ```go
   if subAgent, ok := event.Metadata["sub_agent"].(string); ok {
       // This is a sub-agent event
   }
   ```

5. **Handle All Event Types**
   - `AgentEventThinking` - Reasoning steps
   - `AgentEventContent` - Text output
   - `AgentEventToolCall` - Tool invocations
   - `AgentEventToolResult` - Tool results
   - `AgentEventError` - Errors
   - `AgentEventComplete` - Completion

## Testing

See `examples/sub_agent_streaming_example.go` for a complete working example that:
- Creates parent and sub-agents
- Configures streaming properly
- Tracks all events
- Validates streaming is working
- Provides detailed diagnostics

Run it:
```bash
export ANTHROPIC_API_KEY=your_key
go run examples/sub_agent_streaming_example.go
```

## Implementation Details

### Interface Definition

```go
// SubAgent interface (required methods)
type SubAgent interface {
    Run(ctx context.Context, input string) (string, error)
    GetName() string
    GetDescription() string
}

// StreamingSubAgent adds streaming capability
type StreamingSubAgent interface {
    SubAgent
    RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error)
}
```

### Context Key

The event channel is stored in context with key:
```go
const streamEventChan contextKey = "stream_event_chan"
```

### Event Metadata

Sub-agent events include metadata:
```go
event.Metadata["sub_agent"] = "SubAgentName"
event.Metadata["sub_agent_tool"] = "SubAgentName_agent"
```

## FAQ

**Q: Can I have multiple levels of sub-agents?**
A: Yes, the SDK supports up to 5 levels of nesting. Each level streams to its parent.

**Q: What happens if the sub-agent doesn't support streaming?**
A: The AgentTool automatically falls back to blocking execution with `Run()`.

**Q: Do I need to configure anything special for streaming?**
A: Just use `RunStream()` on the parent agent. The rest is automatic.

**Q: Can I disable sub-agent streaming?**
A: Yes, call the parent agent with `Run()` instead of `RunStream()`.

**Q: How do I know if streaming is working?**
A: Check for `sub_agent` metadata in events, or check logs for "Using STREAMING execution".

## Additional Resources

- Example: `examples/sub_agent_streaming_example.go`
- Tests: `pkg/agent/subagent_test.go`
- Source: `pkg/tools/agent_tool.go`
- Agent Streaming: `pkg/agent/streaming.go`
