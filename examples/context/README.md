# Context Example

This example demonstrates how to use the context package in the Agent SDK to manage and share state across different components.

## Features

- Creating and managing context with organization, conversation, user, and request IDs
- Adding memory to context for conversation history
- Adding tools to context for agent capabilities
- Adding LLM to context for language model interactions
- Adding environment variables to context for configuration
- Using context with timeout for managing long-running operations

## Usage

### Prerequisites

- No special prerequisites are needed for this example
- For a real application, you would need API keys for OpenAI and any tools you use

### Running the Example

```bash
go run main.go
```

## Code Explanation

### Creating a New Context

```go
// Create a new context
ctx := pkgcontext.New()
```

### Setting IDs

```go
// Set organization and conversation IDs
ctx = ctx.WithOrganizationID("example-org")
ctx = ctx.WithConversationID("example-conversation")
ctx = ctx.WithUserID("example-user")
ctx = ctx.WithRequestID("example-request")
```

### Adding Memory

```go
// Add memory
memory := memory.NewConversationBuffer()
ctx = ctx.WithMemory(memory)
```

### Adding Tools

```go
// Add tools
toolRegistry := tools.NewRegistry()
searchTool := websearch.New("api-key", "engine-id")
toolRegistry.Register(searchTool)
ctx = ctx.WithTools(toolRegistry)
```

### Adding LLM

```go
// Add LLM
openaiClient := openai.NewClient("api-key")
ctx = ctx.WithLLM(openaiClient)
```

### Adding Environment Variables

```go
// Add environment variables
ctx = ctx.WithEnvironment("temperature", 0.7)
ctx = ctx.WithEnvironment("max_tokens", 1000)
```

### Using Context with Timeout

```go
// Create a context with timeout
ctxWithTimeout, cancel := ctx.WithTimeout(5 * time.Second)
defer cancel()

// Simulate a long-running operation
select {
case <-time.After(1 * time.Second):
    fmt.Println("Operation completed successfully")
case <-ctxWithTimeout.Done():
    fmt.Println("Operation timed out")
}
```

### Retrieving Values from Context

```go
// Access components from context
if _, ok := ctx.Memory(); ok {
    fmt.Println("Memory found in context")
    // Use memory...
}

if tools, ok := ctx.Tools(); ok {
    fmt.Println("Tools found in context:")
    for _, tool := range tools.List() {
        fmt.Printf("- %s: %s\n", tool.Name(), tool.Description())
    }
}

if _, ok := ctx.LLM(); ok {
    fmt.Println("LLM found in context")
    // Use LLM...
}

if temp, ok := ctx.Environment("temperature"); ok {
    fmt.Printf("Temperature: %v\n", temp)
}
```

## Benefits of Using Context

1. **Centralized State Management**: Keep all related state in one place
2. **Type Safety**: Strongly typed access to context values
3. **Immutability**: Each context modification returns a new context
4. **Cancellation and Timeouts**: Built-in support for cancellation and timeouts
5. **Standardized Access**: Consistent way to access components across the application

## Use Cases

- **Agent Systems**: Share memory, tools, and LLM between agent components
- **Multi-tenant Applications**: Manage organization and user IDs
- **Request Handling**: Track request IDs and set timeouts
- **Configuration**: Store and retrieve environment variables

## Customization

You can customize this example by:
- Adding more components to the context
- Implementing custom context keys for your specific needs
- Using context with different timeout values
- Integrating with your own memory, tools, or LLM implementations
