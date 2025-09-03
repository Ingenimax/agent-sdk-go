# Azure OpenAI Client Package

This package provides a client for interacting with Azure OpenAI services, implementing the `interfaces.LLM` interface. It's designed to work seamlessly with Azure's OpenAI deployments while maintaining compatibility with the existing Agent SDK patterns.

## Features

- Text generation with the `Generate` method
- Chat completion with the `Chat` method
- Tool integration with the `GenerateWithTools` method
- Streaming support with `GenerateStream` and `GenerateWithToolsStream`
- Configurable options for model parameters
- Azure-specific authentication and endpoint handling
- Support for custom deployments and API versions
- Full compatibility with the `interfaces.LLM` interface

## Prerequisites

Before using this client, you need:

1. An Azure subscription with Azure OpenAI service enabled
2. A deployed model in Azure OpenAI (e.g., GPT-4, GPT-3.5-turbo)
3. The following information from your Azure OpenAI resource:
   - API Key
   - Endpoint URL (base URL)
   - Deployment name
   - API version (optional, defaults to `2024-02-15-preview`)

## Usage

### Creating a Client

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/llm/azureopenai"

// Option 1: Using Base URL (traditional approach)
client := azureopenai.NewClient(
    "your-api-key",
    "https://your-resource.openai.azure.com",
    "your-deployment-name", // Deployment name serves as model identifier
)

// Option 2: Using Region and Resource Name (recommended)
client := azureopenai.NewClientFromRegion(
    "your-api-key",
    "eastus",              // Azure region
    "your-resource-name",  // Azure OpenAI resource name
    "your-deployment-name", // Deployment name serves as model identifier
)

// Client with custom options
client := azureopenai.NewClientFromRegion(
    "your-api-key",
    "eastus",
    "your-resource-name",
    "your-deployment-name", // Deployment name serves as model identifier
    azureopenai.WithAPIVersion("2024-08-01-preview"),
    azureopenai.WithRegion("eastus"),
    azureopenai.WithResourceName("your-resource-name"),
    azureopenai.WithLogger(customLogger),
)
```

### Text Generation

```go
response, err := client.Generate(
    context.Background(),
    "Write a haiku about programming",
    azureopenai.WithTemperature(0.7),
    azureopenai.WithSystemMessage("You are a creative assistant."),
)
if err != nil {
    log.Fatal(err)
}
fmt.Println(response)
```

### Chat Completion

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/llm"

messages := []llm.Message{
    {
        Role:    "system",
        Content: "You are a helpful programming assistant.",
    },
    {
        Role:    "user",
        Content: "What's the best way to handle errors in Go?",
    },
}

response, err := client.Chat(context.Background(), messages, nil)
if err != nil {
    log.Fatal(err)
}
fmt.Println(response)
```

### Tool Integration

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"

// Define your tools
tools := []interfaces.Tool{
    // Your tool implementations
}

// Generate with tools
response, err := client.GenerateWithTools(
    context.Background(),
    "What's the weather in San Francisco?",
    tools,
    azureopenai.WithTemperature(0.7),
    azureopenai.WithMaxIterations(3),
)
```

### Streaming

```go
// Basic streaming
eventChan, err := client.GenerateStream(
    context.Background(),
    "Tell me a story",
    azureopenai.WithTemperature(0.8),
)
if err != nil {
    log.Fatal(err)
}

for event := range eventChan {
    switch event.Type {
    case interfaces.StreamEventContentDelta:
        fmt.Print(event.Content)
    case interfaces.StreamEventError:
        log.Printf("Stream error: %v", event.Error)
    case interfaces.StreamEventMessageStop:
        fmt.Println("\nStream completed")
    }
}

// Streaming with reasoning
eventChan, err := client.GenerateStream(
    context.Background(),
    "Explain quantum computing step by step",
    azureopenai.WithReasoning("comprehensive"),
    azureopenai.WithTemperature(0.4),
)

// Advanced streaming with custom configuration
streamConfig := interfaces.StreamConfig{
    BufferSize: 100,
}

eventChan, err := client.GenerateStream(
    context.Background(),
    "Write a technical explanation",
    azureopenai.WithReasoning("minimal"),
    interfaces.WithStreamConfig(streamConfig),
)

// Streaming with tools
eventChan, err := client.GenerateWithToolsStream(
    context.Background(),
    "Research the latest AI developments",
    tools,
    azureopenai.WithTemperature(0.7),
)
```

## Configuration Options

The Azure OpenAI client provides several configuration options:

### Client Options (used during client creation)

- `WithModel(string)` - Sets the model name (for logging/reference, defaults to deployment name)
- `WithDeployment(string)` - Sets the Azure deployment name (also used as model identifier)
- `WithAPIVersion(string)` - Sets the Azure OpenAI API version
- `WithRegion(string)` - Sets the Azure region
- `WithResourceName(string)` - Sets the Azure resource name
- `WithLogger(logging.Logger)` - Sets a custom logger
- `WithRetry(...retry.Option)` - Configures retry behavior
- `WithBaseURL(string)` - Updates the base URL (recreates clients)

### Generation Options (used during API calls)

- `WithTemperature(float64)` - Controls randomness (0.0 to 1.0)
- `WithTopP(float64)` - Controls diversity via nucleus sampling
- `WithFrequencyPenalty(float64)` - Reduces repetition of token sequences
- `WithPresencePenalty(float64)` - Reduces repetition of topics
- `WithStopSequences([]string)` - Specifies sequences where generation should stop
- `WithSystemMessage(string)` - Sets the system message
- `WithResponseFormat(interfaces.ResponseFormat)` - Enables structured output
- `WithReasoning(string)` - Controls reasoning verbosity ("none", "minimal", "comprehensive")

## Azure-Specific Features

### Deployment-Based Routing

Unlike the standard OpenAI API, Azure OpenAI uses deployment names to route requests to specific models. The client automatically handles this by:

1. Using the deployment name as the model parameter in API requests
2. Constructing the correct Azure OpenAI endpoint URL
3. Adding the API version as a query parameter

### API Versioning

Azure OpenAI uses API versioning through query parameters. You can specify the version:

```go
client := azureopenai.NewClient(
    apiKey,
    baseURL,
    deployment,
    azureopenai.WithAPIVersion("2024-08-01-preview"), // Required for structured output
)
```

**Important API Version Requirements:**
- **Structured Output (JSON Schema)**: Requires `2024-08-01-preview` or later
- **Basic functionality**: Works with `2024-02-15-preview` or later
- **Default**: `2024-08-01-preview` (for maximum feature compatibility)

### Authentication

The client uses API key authentication, which is passed in the `Authorization` header as `Bearer {api-key}`.

## Reasoning Models Support

The client automatically detects reasoning models (o1-preview, o1-mini, etc.) and adjusts parameters accordingly:

- Forces temperature to 1.0 for reasoning models
- Disables top_p parameter for reasoning models
- Disables parallel tool calls for reasoning models
- Includes usage information in streaming for reasoning models

## Error Handling

The client provides comprehensive error handling:

- Network errors are wrapped with context
- API errors include deployment and model information
- Retry mechanisms can be configured for transient failures
- Streaming errors are sent through the event channel

## Memory Integration

The client supports memory integration for conversation history:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/memory"

mem := memory.NewConversationBuffer(10) // Keep last 10 messages

response, err := client.Generate(
    ctx,
    "Continue our conversation",
    azureopenai.WithMemory(mem),
)
```

## Tracing Integration

Tool calls are automatically traced when using the tracing package:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tracing"

// Tool calls will be automatically added to the tracing context
response, err := client.GenerateWithTools(ctx, prompt, tools)
```

## Environment Variables

For convenience, you can use environment variables:

```bash
# Option 1: Using Base URL
export AZURE_OPENAI_API_KEY="your-api-key"
export AZURE_OPENAI_BASE_URL="https://your-resource.openai.azure.com"
export AZURE_OPENAI_DEPLOYMENT="your-deployment-name"
export AZURE_OPENAI_API_VERSION="2024-08-01-preview"

# Option 2: Using Region and Resource Name (recommended)
export AZURE_OPENAI_API_KEY="your-api-key"
export AZURE_OPENAI_REGION="eastus"
export AZURE_OPENAI_RESOURCE_NAME="your-resource-name"
export AZURE_OPENAI_DEPLOYMENT="your-deployment-name"
export AZURE_OPENAI_API_VERSION="2024-08-01-preview"
```

Then in your code:

```go
// Option 1: Using Base URL
client := azureopenai.NewClient(
    os.Getenv("AZURE_OPENAI_API_KEY"),
    os.Getenv("AZURE_OPENAI_BASE_URL"),
    os.Getenv("AZURE_OPENAI_DEPLOYMENT"),
    azureopenai.WithAPIVersion(os.Getenv("AZURE_OPENAI_API_VERSION")),
)

// Option 2: Using Region and Resource Name (recommended)
client := azureopenai.NewClientFromRegion(
    os.Getenv("AZURE_OPENAI_API_KEY"),
    os.Getenv("AZURE_OPENAI_REGION"),
    os.Getenv("AZURE_OPENAI_RESOURCE_NAME"),
    os.Getenv("AZURE_OPENAI_DEPLOYMENT"),
    azureopenai.WithAPIVersion(os.Getenv("AZURE_OPENAI_API_VERSION")),
)
```

## Differences from Standard OpenAI Client

1. **Deployment Names**: Uses Azure deployment names as both deployment and model identifiers in API calls
2. **Endpoint Structure**: Constructs Azure-specific endpoint URLs
3. **API Versioning**: Includes API version as query parameter
4. **Authentication**: Uses Azure OpenAI authentication format
5. **Configuration**: Requires additional Azure-specific parameters (deployment, API version)
6. **Model Identification**: Deployment name serves as the model identifier (no separate model parameter needed)

## Best Practices

1. **Use specific API versions** to ensure consistent behavior
2. **Configure retry policies** for production use
3. **Monitor token usage** especially with reasoning models
4. **Use structured output** for reliable data extraction
5. **Implement proper error handling** for network and API errors
6. **Use memory integration** for multi-turn conversations
7. **Configure appropriate timeouts** for your use case

## Examples

See the `examples/llm/azureopenai/` directory for complete working examples demonstrating various features and use cases.

## Testing

Run the tests with:

```bash
go test ./pkg/llm/azureopenai/...
```

For integration tests with real Azure OpenAI services, set the required environment variables and uncomment the integration test functions.
