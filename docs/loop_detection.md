# Loop Detection in Tool Calling

## Overview

The Agent SDK now includes automatic loop detection to prevent agents from getting stuck in repetitive tool-calling patterns when `WithMaxIterations` is set to high values.

## Problem

When agents are configured with high iteration limits (e.g., `WithMaxIterations(10)`), they may:
- Call the same tool with identical parameters multiple times
- Get stuck in patterns without making progress
- Exhaust iterations without providing meaningful results

## Solution

The SDK implements a **warning-based loop detection system** that:
1. Tracks all tool calls with their parameters
2. Detects when identical calls are repeated
3. Adds warnings to tool results to inform the LLM
4. Preserves LLM autonomy (non-blocking approach)

## How It Works

### Detection Mechanism

```go
// For each tool call, we create a cache key
cacheKey := toolName + ":" + toolArguments

// Track the count
toolCallHistory[cacheKey]++

// If repeated, add warning to result
if toolCallHistory[cacheKey] > 1 {
    warning := fmt.Sprintf("\n\n[WARNING: This is call #%d to %s with identical parameters. You may be in a loop. Consider using the available information to provide a final answer.]",
        toolCallHistory[cacheKey],
        toolName)
    toolResult += warning
}
```

### Features

1. **Non-Blocking**: The LLM can still make legitimate repeated calls if necessary
2. **Clear Feedback**: Warnings explicitly tell the LLM it's repeating
3. **Progressive Counting**: Shows call count to indicate severity
4. **Universal**: Works across OpenAI, Anthropic, and Vertex AI clients

## Memory Integration

The implementation also improves memory usage:
- Messages are initialized from memory history
- Prevents duplicate user messages
- Maintains full conversation context
- Tool calls are properly stored in memory
- **Tool names are stored in metadata** for better context

### Tool Name Storage

Tool messages now include the tool name in metadata:

```go
_ = params.Memory.AddMessage(ctx, interfaces.Message{
    Role:       "tool",
    Content:    toolResult,
    ToolCallID: toolCall.ID,
    Metadata: map[string]interface{}{
        "tool_name": toolName,  // <- Tool name stored here
    },
})
```

When formatting memory history into prompts, tool names are included in role markers:
```
USER: Calculate 15 * 23
ASSISTANT: I'll use the calculator tool for that.
TOOL[calculator]: 345
ASSISTANT: The result is 345.
```

This makes it clear which tool generated which response in the conversation context.

## Configuration

No additional configuration required. Loop detection is automatically enabled when using tools:

```go
agent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithTools(myTool1, myTool2),
    agent.WithMaxIterations(10), // Safe even with high values
)
```

## Example Warning

When a loop is detected, the tool result includes:

```
Original tool result here...

[WARNING: This is call #3 to search_tool with identical parameters. You may be in a loop. Consider using the available information to provide a final answer.]
```

## Benefits

1. **Prevents Infinite Loops**: Agents recognize repetitive behavior
2. **Saves API Costs**: Reduces unnecessary tool executions
3. **Better User Experience**: Faster, more conclusive responses
4. **Debugging Aid**: Warnings appear in logs for troubleshooting
5. **Maintains Flexibility**: Legitimate repeated calls still work

## Logging

Loop detection events are logged at WARN level:

```
WARN Repetitive tool call detected toolName=search_tool callCount=3
```

## Testing

Run the demo to see loop detection in action:

```bash
cd examples/loop_detection_demo
export OPENAI_API_KEY=your_key_here
go run main.go
```

## Implementation Details

### Supported Clients
- ✅ OpenAI (`pkg/llm/openai/client.go`)
- ✅ Anthropic (`pkg/llm/anthropic/client.go`)
- ✅ Vertex AI (`pkg/llm/vertex/client.go`)

### Key Changes
1. Tool call history tracking per execution
2. Memory-based message initialization
3. Warning injection into tool results
4. Improved logging for debugging

## Best Practices

1. **Set Reasonable Iterations**: Even with loop detection, use appropriate `WithMaxIterations` values
2. **Clear System Prompts**: Help the agent understand when to conclude
3. **Monitor Warnings**: Check logs for repetitive patterns
4. **Tool Design**: Design tools that provide conclusive results when possible

## Future Enhancements

Potential improvements for consideration:
- Configurable warning thresholds
- Pattern detection across different tools
- Caching of tool results
- Automatic iteration reduction on loop detection
- Metrics and monitoring integration
