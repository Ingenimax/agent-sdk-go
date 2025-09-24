# Intermediate Messages in Streaming

## Overview

The agent-sdk-go now supports streaming intermediate messages during tool iterations. This feature allows you to see the LLM's reasoning and responses between tool calls in real-time, providing better visibility into the agent's thought process during complex multi-step operations.

## Background

When an LLM uses tools in multiple iterations (controlled by `maxIterations`), it typically:
1. Analyzes the prompt and decides which tool to call
2. Calls the tool and receives the result
3. Processes the result and decides if another tool call is needed
4. Repeats until it has enough information to provide a final answer

Previously, content generated between these tool calls was filtered out and only the final response was streamed to the user. With the new `IncludeIntermediateMessages` flag, you can now see all the intermediate reasoning.

## Configuration

### Setting Up StreamConfig

The `IncludeIntermediateMessages` flag is part of the `StreamConfig` structure:

```go
streamConfig := &interfaces.StreamConfig{
    BufferSize:                  100,    // Channel buffer size
    IncludeThinking:             true,   // Include thinking/reasoning events
    IncludeToolProgress:         true,   // Include tool execution progress
    IncludeIntermediateMessages: true,   // NEW: Include intermediate messages between iterations
}
```

### Default Behavior

By default, `IncludeIntermediateMessages` is set to `false` to maintain backward compatibility. This means existing applications will continue to work as before, only streaming the final response.

## Usage Examples

### Example 1: Basic Agent with Intermediate Messages

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
)

func main() {
    // Create LLM client
    llmClient := anthropic.NewAnthropicClient(
        apiKey,
        anthropic.WithModel("claude-3-5-haiku-20241022"),
    )

    // Configure streaming with intermediate messages
    streamConfig := &interfaces.StreamConfig{
        BufferSize:                  100,
        IncludeIntermediateMessages: true,
    }

    // Create agent with stream configuration
    streamingAgent, err := agent.NewAgent(
        agent.WithLLM(llmClient),
        agent.WithTools(tools),
        agent.WithMaxIterations(3),
        agent.WithStreamConfig(streamConfig),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Start streaming
    ctx := context.Background()
    eventChan, err := streamingAgent.RunStream(ctx, "Your prompt here")
    if err != nil {
        log.Fatal(err)
    }

    // Process events
    for event := range eventChan {
        switch event.Type {
        case interfaces.AgentEventContent:
            // This will now include intermediate messages
            fmt.Print(event.Content)
        case interfaces.AgentEventToolCall:
            fmt.Printf("Tool: %s\n", event.ToolCall.Name)
        }
    }
}
```

### Example 2: Direct LLM Streaming with Intermediate Messages

```go
// When using the LLM directly (not through an agent)
streamConfig := interfaces.StreamConfig{
    IncludeIntermediateMessages: true,
}

eventChan, err := llmClient.GenerateWithToolsStream(
    ctx,
    prompt,
    tools,
    interfaces.WithStreamConfig(streamConfig),
    interfaces.WithMaxIterations(3),
)
```

### Example 3: Using the Helper Function

```go
// Use the helper function for cleaner code
streamConfig := interfaces.DefaultStreamConfig()
interfaces.WithIncludeIntermediateMessages(true)(&streamConfig)

// Or combine multiple options
streamConfig := interfaces.StreamConfig{
    BufferSize: 200,
}
interfaces.WithIncludeIntermediateMessages(true)(&streamConfig)
```

## Use Cases

### When to Enable Intermediate Messages

1. **Debugging Complex Workflows**: See exactly how the agent reasons through multi-step problems
2. **User Transparency**: Show users the step-by-step process the AI follows
3. **Educational Applications**: Demonstrate AI reasoning for learning purposes
4. **Audit and Compliance**: Maintain complete logs of AI decision-making
5. **Interactive Applications**: Provide real-time feedback during long-running operations

### When to Keep It Disabled (Default)

1. **Production APIs**: When you only need the final result
2. **Performance-Critical Applications**: Reduce data transfer overhead
3. **Simple Q&A Systems**: Where intermediate steps add no value
4. **Token-Sensitive Applications**: Minimize token consumption display

## How It Works

### Without Intermediate Messages (Default)
```
User Input → LLM Thinks → Tool Call 1 → [Hidden Response] → Tool Call 2 → [Hidden Response] → Final Answer → User
```

### With Intermediate Messages Enabled
```
User Input → LLM Thinks → Tool Call 1 → [Streamed Response] → Tool Call 2 → [Streamed Response] → Final Answer → User
```

## Implementation Details

### Supported LLM Providers

Intermediate message streaming is implemented for:
- ✅ Anthropic (Claude models)
- ✅ OpenAI (GPT models including GPT-4, GPT-3.5, o1 models)
- ✅ Azure OpenAI (All supported models)
- ✅ Google Gemini (Gemini Pro, Gemini Flash, etc.)

### Technical Implementation

The feature works by controlling content filtering during tool iterations. Each LLM provider implements this differently:

#### Anthropic Implementation
Uses a `filterContentDeltas` flag in the streaming loop:

```go
// In the Anthropic client streaming implementation
filterContentDeltas := true // Default behavior
if params.StreamConfig != nil && params.StreamConfig.IncludeIntermediateMessages {
    filterContentDeltas = false // Stream intermediate content
}
```

#### OpenAI/Azure OpenAI Implementation
Captures content events during iterations and replays them if filtering is disabled:

```go
// Content filtering logic
if filterIntermediateContent && len(openaiTools) > 0 && iteration < maxIterations-1 {
    // Capture content for potential replay later
    iterationContentEvents = append(iterationContentEvents, contentEvent)
} else {
    // Stream content immediately
    eventChan <- contentEvent
}
```

#### Gemini Implementation
Uses a modified execution function that supports optional content filtering:

```go
// Filtering logic in Gemini
shouldFilter := filterIntermediateContent && len(tools) > 0 && iteration < maxIterations-1
toolCalls, hasContent, iterationContentEvents, err := c.executeStreamingRequestWithToolCaptureAndFilter(
    ctx, contents, config, eventCh, shouldFilter)
```

All implementations maintain backward compatibility by defaulting to filtered content (current behavior).

## Testing

Run the test suite to verify intermediate message functionality:

```bash
# Run the specific test
go test -v ./pkg/llm/anthropic -run TestStreamingIntermediateMessages

# With API key
ANTHROPIC_API_KEY=your_key go test -v ./pkg/llm/anthropic -run TestStreamingIntermediateMessages
```

## Example Output Comparison

### Without Intermediate Messages
```
🚀 Starting stream...
🔧 [Tool Call #1] calculator with args: {"operation":"add","a":15,"b":27}
✅ [Tool Result] 42.00
🔧 [Tool Call #2] calculator with args: {"operation":"multiply","a":42,"b":3}
✅ [Tool Result] 126.00
🔧 [Tool Call #3] calculator with args: {"operation":"divide","a":126,"b":2}
✅ [Tool Result] 63.00

The final result is 63. Here's how I calculated it:
1. First, I added 15 and 27 to get 42
2. Then, I multiplied 42 by 3 to get 126
3. Finally, I divided 126 by 2 to get 63
```

### With Intermediate Messages
```
🚀 Starting stream...
I'll solve this step by step.

First, let me add 15 and 27:
🔧 [Tool Call #1] calculator with args: {"operation":"add","a":15,"b":27}
✅ [Tool Result] 42.00

Good! 15 + 27 = 42. Now I'll multiply this result by 3:
🔧 [Tool Call #2] calculator with args: {"operation":"multiply","a":42,"b":3}
✅ [Tool Result] 126.00

Excellent! 42 × 3 = 126. Finally, let me divide this by 2:
🔧 [Tool Call #3] calculator with args: {"operation":"divide","a":126,"b":2}
✅ [Tool Result] 63.00

Perfect! The final result is 63. To summarize:
1. 15 + 27 = 42
2. 42 × 3 = 126
3. 126 ÷ 2 = 63
```

## Migration Guide

### For Existing Applications

No changes are required for existing applications. The default behavior remains unchanged with `IncludeIntermediateMessages` set to `false`.

### To Enable the Feature

1. Update your agent-sdk-go dependency to the latest version
2. Add the StreamConfig to your agent or LLM calls:

```go
// Before (no intermediate messages)
agent, _ := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithTools(tools),
)

// After (with intermediate messages)
streamConfig := &interfaces.StreamConfig{
    IncludeIntermediateMessages: true,
}
agent, _ := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithTools(tools),
    agent.WithStreamConfig(streamConfig),
)
```

## Performance Considerations

- **Network Traffic**: Enabling intermediate messages increases the amount of data streamed
- **Memory Usage**: No significant impact as messages are streamed, not buffered
- **Latency**: Slightly improved perceived responsiveness as users see progress sooner
- **Token Usage**: No change in actual LLM token consumption, only in what's displayed

## Troubleshooting

### Intermediate Messages Not Appearing

1. Verify `IncludeIntermediateMessages` is set to `true`
2. Ensure you're using a supported LLM provider (currently Anthropic)
3. Check that `maxIterations` is greater than 1
4. Confirm your tools are actually being called multiple times

### Too Much Output

If intermediate messages create too much output, consider:
- Keeping the feature disabled for production
- Implementing client-side filtering
- Using the feature only during development/debugging

## Future Enhancements

Planned improvements include:
- Granular control over which types of intermediate content to include
- Ability to format intermediate messages differently from final output
- Metadata tags to distinguish intermediate from final content
- Performance optimizations for high-throughput scenarios

## Contributing

We welcome contributions! This feature is now implemented across all major LLM providers. If you'd like to contribute:
1. Follow the existing implementation patterns
2. Add comprehensive tests for any new functionality
3. Update this documentation
4. Submit a pull request

## References

- [StreamConfig Interface](../pkg/interfaces/streaming.go)
- [Anthropic Streaming Implementation](../pkg/llm/anthropic/streaming.go)
- [Example Implementation](../examples/streaming_intermediate_messages/main.go)
- [Test Suite](../pkg/llm/anthropic/streaming_intermediate_test.go)