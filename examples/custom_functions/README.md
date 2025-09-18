# Custom Functions Example

This example demonstrates how to use custom run functions with the Agent SDK. Custom functions allow you to replace the default agent behavior with your own custom logic while still having access to all agent components (LLM, memory, tools, logger, etc.).

## Features Demonstrated

1. **Simple Custom Function**: Basic data processing without LLM
2. **AI-Enhanced Custom Function**: Uses the agent's LLM for preprocessing
3. **Custom Streaming Function**: Provides real-time streaming updates

## Custom Function Types

### CustomRunFunction
```go
type CustomRunFunction func(ctx context.Context, input string, agent *Agent) (string, error)
```

### CustomRunStreamFunction
```go
type CustomRunStreamFunction func(ctx context.Context, input string, agent *Agent) (<-chan interfaces.AgentStreamEvent, error)
```

## Usage

### Basic Custom Function
```go
func customProcessor(ctx context.Context, input string, agent *agent.Agent) (string, error) {
    // Your custom logic here
    logger := agent.GetLogger()
    memory := agent.GetMemory()

    // Process input
    result := strings.ToUpper(input)

    // Store in memory
    memory.AddMessage(ctx, interfaces.Message{
        Role: "assistant",
        Content: result,
    })

    return result, nil
}

// Create agent with custom function
agent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithCustomRunFunction(customProcessor),
    agent.WithName("CustomAgent"),
)
```

### Custom Streaming Function
```go
func customStreamProcessor(ctx context.Context, input string, agent *agent.Agent) (<-chan interfaces.AgentStreamEvent, error) {
    eventChan := make(chan interfaces.AgentStreamEvent, 10)

    go func() {
        defer close(eventChan)

        // Send events
        eventChan <- interfaces.AgentStreamEvent{
            Type: interfaces.AgentEventContent,
            Content: "Processing...",
            Timestamp: time.Now(),
        }

        // Send completion
        eventChan <- interfaces.AgentStreamEvent{
            Type: interfaces.AgentEventComplete,
            Timestamp: time.Now(),
        }
    }()

    return eventChan, nil
}

// Create agent with custom streaming
agent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithCustomRunStreamFunction(customStreamProcessor),
    agent.WithName("StreamAgent"),
)
```

## Available Agent Methods for Custom Functions

- `agent.GetLLM()` - Access the LLM for generation
- `agent.GetMemory()` - Access conversation memory
- `agent.GetTools()` - Access available tools
- `agent.GetLogger()` - Access logging
- `agent.GetTracer()` - Access tracing
- `agent.GetSystemPrompt()` - Access system prompt

## Running the Example

1. Set your OpenAI API key:
   ```bash
   export OPENAI_API_KEY="your-api-key-here"
   ```

2. Run the example:
   ```bash
   go run main.go
   ```

## Example Output

The example will show three different custom functions in action:

1. **Simple Processing**: Converts input to uppercase with timestamp
2. **AI-Enhanced**: Uses LLM to analyze and summarize input
3. **Streaming**: Processes words one by one with real-time updates

## Use Cases

- **Data Processing**: Custom data transformation logic
- **Multi-step Workflows**: Complex business logic with LLM integration
- **Domain-Specific Logic**: Industry-specific processing
- **Real-time Processing**: Streaming updates for long-running operations
- **Hybrid AI/Traditional**: Combine LLM with custom algorithms

## Benefits

- **Full Control**: Complete control over execution logic
- **Agent Integration**: Access to all agent components
- **Backwards Compatible**: Fallback to default behavior when no custom function is set
- **Streaming Support**: Real-time updates for better user experience
- **gRPC Compatible**: Works seamlessly with microservice architecture