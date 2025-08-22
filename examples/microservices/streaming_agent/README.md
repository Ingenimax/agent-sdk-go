# Streaming Agent Microservice Example

This example demonstrates how to create a streaming agent wrapped in a microservice and consume its streaming responses via gRPC. It showcases the Agent SDK's full streaming capabilities including thinking events, tool calls, and real-time response formatting.

## Features

- ✅ **Full gRPC Streaming**: Uses the SDK's built-in gRPC streaming support
- ✅ **Thinking Events**: Shows AI reasoning process with `<thinking>` tags
- ✅ **Tool Integration**: Calculator tool with streaming execution progress
- ✅ **Rich Event Types**: Content, thinking, tool calls, results, errors, and completion
- ✅ **Color-coded Output**: Different colors for different event types
- ✅ **Multi-provider Support**: Works with both Anthropic and OpenAI

## Architecture

```
┌─────────────────┐    gRPC Stream    ┌─────────────────┐
│   gRPC Client   │◄─────────────────►│  Microservice   │
│                 │                   │                 │
│ • Event Handler │                   │ • Agent Wrapper │
│ • Formatter     │                   │ • gRPC Server   │ 
│ • Color Output  │                   │ • Health Check  │
└─────────────────┘                   └─────────────────┘
                                              │
                                              ▼
                                    ┌─────────────────┐
                                    │ Streaming Agent │
                                    │                 │
                                    │ • LLM Client    │
                                    │ • Tools         │
                                    │ • Thinking      │
                                    └─────────────────┘
```

## Prerequisites

- Go 1.21 or higher
- Agent SDK Go module
- API key for your chosen LLM provider

## Setup

### 1. Environment Variables

Choose your LLM provider and set the corresponding API key:

**For Anthropic (Claude):**
```bash
export ANTHROPIC_API_KEY="your_anthropic_api_key_here"
export LLM_PROVIDER="anthropic"  # optional, this is the default
```

**For OpenAI (GPT):**
```bash
export OPENAI_API_KEY="your_openai_api_key_here"
export LLM_PROVIDER="openai"
```

### 2. Run the Example

```bash
cd examples/microservices/streaming_agent

# Run the simplified streaming example
go run main.go
```

## Example Output

When you run the example, you'll see colorized streaming output with logging like this:

```
Simplified Streaming Agent Microservice Example
==============================================

Using provider: anthropic

1. Creating streaming agent with thinking support...
Created streaming agent: StreamingThinkingAgent

2. Creating microservice wrapper...
Starting microservice on port 52341...
Microservice ready on port 52341

3. Setting up event handlers and streaming...
Query: Explain the concept of recursion in programming with a simple example.
--------------------------------------------------------------------------------
[LOG] Setting up event handlers
[LOG] Starting stream execution

[LOG] Entering thinking mode
<thinking>
The user is asking me to explain recursion in programming. This is a fundamental concept in computer science. Let me think about how to explain this clearly with a good example.

Recursion is when a function calls itself to solve a problem by breaking it down into smaller, similar subproblems. I should explain:
1. The basic concept
2. The key components (base case, recursive case)
3. A simple, clear example
4. Maybe mention why it's useful

A classic example would be calculating factorial, or maybe something even simpler like counting down from a number.
</thinking>

[LOG] Exiting thinking mode, displaying content
Recursion is a programming technique where a function calls itself to solve a problem by breaking it down into smaller, similar subproblems.

## Key Components of Recursion:

1. **Base Case**: A condition that stops the recursion
2. **Recursive Case**: The function calling itself with modified parameters

## Simple Example - Countdown Function:

```python
def countdown(n):
    # Base case: stop when we reach 0
    if n <= 0:
        print("Blast off!")
        return
    
    # Print current number
    print(n)
    
    # Recursive case: call countdown with n-1
    countdown(n - 1)

# Usage
countdown(3)
```

**Output:**
```
3
2
1
Blast off!
```

## How it works:
- `countdown(3)` prints 3, then calls `countdown(2)`
- `countdown(2)` prints 2, then calls `countdown(1)`  
- `countdown(1)` prints 1, then calls `countdown(0)`
- `countdown(0)` hits the base case and prints "Blast off!"

Each function call waits for the inner call to complete before finishing, creating a natural "unwinding" process.

[LOG] Stream completed successfully
Stream completed
```

## Event Types

The example handles all streaming event types:

| Event Type | Color | Description | Example |
|------------|-------|-------------|---------|
| `THINKING` | Gray | AI reasoning process | `<thinking>Let me think...</thinking>` |
| `CONTENT` | White | Regular response text | Main answer content |
| `TOOL_CALL` | Yellow | Tool execution start | `🔧 Tool Call: calculator` |
| `TOOL_RESULT` | Green | Tool execution result | `✅ Tool Result: 345` |
| `ERROR` | Red | Error messages | `❌ Error: Connection failed` |
| `COMPLETE` | Green | Stream completion | `✅ Stream completed` |

## Code Structure

### 1. Agent Creation

```go
streamingAgent, err := agent.NewAgent(
    agent.WithName("StreamingThinkingAgent"),
    agent.WithLLM(llm),
    agent.WithTools(calculator.NewCalculatorTool()),
    agent.WithLLMConfig(interfaces.LLMConfig{
        EnableReasoning: true, // Enable thinking
        ReasoningBudget: 2048,
    }),
)
```

### 2. Microservice Wrapper

```go
service, err := microservice.CreateMicroservice(streamingAgent, microservice.Config{
    Port: 0, // Auto-assign port
})
service.Start()
service.WaitForReady(10 * time.Second)
```

### 3. Simplified Streaming API

The SDK now provides two convenient ways to consume streaming responses:

#### Option A: Direct Channel-based API

```go
events, err := service.RunStream(ctx, "Your question here")
if err != nil {
    return err
}

for event := range events {
    switch event.Type {
    case interfaces.AgentEventThinking:
        fmt.Printf("<thinking>%s</thinking>\n", event.ThinkingStep)
    case interfaces.AgentEventContent:
        fmt.Printf("%s", event.Content)
    case interfaces.AgentEventComplete:
        return nil
    }
}
```

#### Option B: Event Handler Registration

```go
service.
    OnThinking(func(thinking string) {
        fmt.Printf("<thinking>%s</thinking>\n", thinking)
    }).
    OnContent(func(content string) {
        fmt.Printf("%s", content)
    }).
    OnComplete(func() {
        fmt.Println("Stream completed")
    })

// Execute with registered handlers
err := service.Stream(ctx, "Your question here")
```

### 4. Manual gRPC Client (Advanced)

For advanced use cases, you can still use the raw gRPC client:

```go
client := pb.NewAgentServiceClient(conn)
stream, err := client.RunStream(ctx, &pb.RunRequest{
    Input: "Your question here",
})

for {
    response, err := stream.Recv()
    // Process streaming events...
}
```

### 5. Event Processing

The example demonstrates the **Event Handler Registration** approach using the simplified streaming API:

- **Chainable Handlers** - Uses `OnThinking`, `OnContent`, `OnToolCall`, etc. with `Stream()`
- **Built-in Logging** - Each handler includes colored log messages showing event flow
- **Smart Thinking Mode** - Automatically detects and formats thinking sections with `<thinking>` tags

Features include:
- Color-coded output (blue for logs, gray for thinking, white for content)
- Automatic thinking mode detection and transitions  
- Tool execution progress tracking with detailed logging
- Error handling and stream completion logging

## Supported LLM Providers

### Anthropic Claude
- **Models**: Claude 3.5 Sonnet (default), Claude 3 Opus, Claude 3 Haiku
- **Thinking Support**: ✅ Full extended thinking support
- **Tool Streaming**: ✅ Real-time tool execution
- **Setup**: Set `ANTHROPIC_API_KEY` environment variable

### OpenAI GPT
- **Models**: GPT-4, GPT-4 Turbo, GPT-3.5 Turbo
- **Reasoning Models**: o1-preview, o1-mini, o3-mini, o4-mini, GPT-5 series (with reasoning tokens)
- **Tool Streaming**: ✅ Function calling with streaming
- **Temperature**: Automatically set to 1.0 for reasoning models (o1, o3, o4, GPT-5 series)
- **Setup**: Set `OPENAI_API_KEY` environment variable

## Customization

### Adding Custom Tools

```go
import "your-custom-tool-package"

agent.NewAgent(
    // ... other options ...
    agent.WithTools(
        calculator.NewCalculatorTool(),
        yourcustom.NewCustomTool(),
    ),
)
```

### Modifying Stream Processing

The `streamRequest` function can be modified to:
- Filter specific event types
- Add custom formatting
- Save streaming data to files
- Integrate with web interfaces

### Changing LLM Configuration

```go
agent.WithLLMConfig(interfaces.LLMConfig{
    Temperature:     0.3,      // Lower for more focused responses
    EnableReasoning: true,     // Enable/disable thinking
    ReasoningBudget: 4096,     // Increase thinking token budget
})
```

## Troubleshooting

### Common Issues

1. **"API key required" error**
   - Make sure you've set the correct environment variable
   - Verify the API key is valid and has sufficient credits

2. **"Microservice failed to become ready"**
   - Check if the port is available
   - Ensure no firewall is blocking the connection
   - Try increasing the timeout in `WaitForReady`

3. **"Stream error: EOF"**
   - This usually indicates normal stream completion
   - Check for any preceding error messages

4. **No thinking events appear**
   - Verify `EnableReasoning: true` is set in LLMConfig
   - Ensure your LLM provider supports thinking/reasoning
   - Try with Anthropic Claude models which have better thinking support

### Debug Mode

To see more detailed debug information, you can add debug prints or check the microservice logs.

## Next Steps

- Try integrating this with web interfaces using WebSockets
- Experiment with different LLM models and configurations
- Add more sophisticated tools and see their streaming execution
- Build multi-agent streaming conversations

## Related Examples

- `examples/streaming/` - Direct agent streaming without microservices
- `examples/advanced_agent_streaming/` - Advanced streaming with multiple agents
- `examples/microservices/simple_mixed/` - Basic microservice example without streaming