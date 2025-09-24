# Streaming Intermediate Messages Example

This example demonstrates the new `IncludeIntermediateMessages` feature that allows you to stream intermediate LLM responses during tool iterations.

## Overview

When an LLM uses tools across multiple iterations, it generates responses between each tool call. By default, these intermediate responses are filtered out and only the final response is streamed. This example shows how to enable streaming of these intermediate messages to provide better visibility into the agent's reasoning process.

## Features Demonstrated

- Configuring `StreamConfig` with `IncludeIntermediateMessages` flag
- Comparing output with and without intermediate messages
- Using tools that require multiple iterations
- Real-time streaming of LLM's step-by-step reasoning

## Running the Example

### Prerequisites

1. Set your Anthropic API key:
```bash
export ANTHROPIC_API_KEY=your_api_key_here
```

2. Ensure you have Go 1.19+ installed

### Run

```bash
go run main.go
```

## Expected Output

The example runs two scenarios:

### Scenario 1: Without Intermediate Messages (Default)
```
📋 SCENARIO 1: Without Intermediate Messages (Default)
------------------------------------------------------------
❌ Intermediate messages DISABLED - You'll only see the final result

🚀 Starting stream...

🔧 [Tool Call #1] calculator with args: {"operation":"add","a":15,"b":27}
✅ [Tool Result] 42.00

🔧 [Tool Call #2] calculator with args: {"operation":"multiply","a":42,"b":3}
✅ [Tool Result] 126.00

🔧 [Tool Call #3] calculator with args: {"operation":"divide","a":126,"b":2}
✅ [Tool Result] 63.00

The final result is 63. Here's how I calculated it step by step:
1. First, I added 15 and 27 to get 42
2. Then I multiplied 42 by 3 to get 126
3. Finally, I divided 126 by 2 to get 63

✨ Stream completed
------------------------------------------------------------
📊 Summary:
   - Tool calls made: 3
   - Total content length: 145 characters
   - Content during tool iterations: false
   ✅ Intermediate messages were correctly filtered!
```

### Scenario 2: With Intermediate Messages Enabled
```
📋 SCENARIO 2: With Intermediate Messages Enabled
------------------------------------------------------------
✅ Intermediate messages ENABLED - You'll see the LLM's thinking between tool calls

🚀 Starting stream...

I'll solve this step by step.

First, let me add 15 and 27:

🔧 [Tool Call #1] calculator with args: {"operation":"add","a":15,"b":27}
✅ [Tool Result] 42.00

Great! 15 + 27 = 42. Now let me multiply this result by 3:

🔧 [Tool Call #2] calculator with args: {"operation":"multiply","a":42,"b":3}
✅ [Tool Result] 126.00

Perfect! 42 × 3 = 126. Finally, let me divide this by 2:

🔧 [Tool Call #3] calculator with args: {"operation":"divide","a":126,"b":2}
✅ [Tool Result] 63.00

Excellent! The final result is 63.

To summarize the calculations:
1. 15 + 27 = 42
2. 42 × 3 = 126
3. 126 ÷ 2 = 63

✨ Stream completed
------------------------------------------------------------
📊 Summary:
   - Tool calls made: 3
   - Total content length: 287 characters
   - Content during tool iterations: true
   ✅ Intermediate messages were successfully streamed!
```

## Key Differences

1. **Content Volume**: With intermediate messages enabled, you see approximately 2x more content
2. **Real-time Feedback**: Users see the LLM's reasoning as it happens, not just at the end
3. **Transparency**: The thought process is visible, making the AI's decision-making more transparent
4. **User Experience**: Better for interactive applications where users want to see progress

## Code Structure

The example includes:

- `CalculatorTool`: A custom tool implementation that performs basic arithmetic
- `runStreamingDemo`: Helper function that configures and runs the streaming agent
- Comparison logic to demonstrate both modes side by side

## Configuration

The key configuration is in the `StreamConfig`:

```go
streamConfig := &interfaces.StreamConfig{
    BufferSize:                  100,
    IncludeToolProgress:         true,
    IncludeIntermediateMessages: includeIntermediate, // true or false
}
```

Then pass it to the agent:

```go
streamingAgent, err := agent.NewAgent(
    agent.WithLLM(llmClient),
    agent.WithTools(tool),
    agent.WithMaxIterations(4),
    agent.WithStreamConfig(streamConfig),
)
```

## Use Cases

This feature is particularly useful for:

1. **Educational Applications**: Show students how AI breaks down problems
2. **Debugging**: Understand why an agent made certain decisions
3. **User Trust**: Build confidence by showing the reasoning process
4. **Long-Running Tasks**: Provide feedback during complex multi-step operations
5. **Audit Trails**: Maintain complete logs of AI decision-making

## Related Documentation

- [Intermediate Messages Streaming Documentation](../../docs/intermediate-messages-streaming.md)
- [Agent Streaming Guide](../streaming/README.md)
- [Tools Documentation](../../docs/tools.md)