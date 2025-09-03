# Azure OpenAI LLM Example

This example demonstrates how to use the Azure OpenAI client from the Agent SDK.

## Features

- Direct text generation using the `Generate` method
- Chat completion using the `Chat` method
- Multi-turn conversations
- Reasoning mode variations (none, minimal, comprehensive)
- Parameter configuration (temperature, top_p, penalties, stop sequences)
- Structured output (JSON schema)
- **Basic streaming** with real-time content delivery
- **Streaming with reasoning** for complex explanations
- **Advanced streaming** with custom configurations and metrics
- Comprehensive error handling and logging

## Prerequisites

Before running this example, you need:

1. An Azure subscription with Azure OpenAI service enabled
2. A deployed model in Azure OpenAI (e.g., GPT-4, GPT-3.5-turbo)
3. The following environment variables set:

```bash
# Option 1: Using Base URL (traditional approach)
export AZURE_OPENAI_API_KEY="your-api-key-here"
export AZURE_OPENAI_BASE_URL="https://your-resource.openai.azure.com"
export AZURE_OPENAI_DEPLOYMENT="your-deployment-name"
export AZURE_OPENAI_API_VERSION="2024-08-01-preview"  # Optional

# Option 2: Using Region and Resource Name (recommended)
export AZURE_OPENAI_API_KEY="your-api-key-here"
export AZURE_OPENAI_REGION="eastus"
export AZURE_OPENAI_RESOURCE_NAME="your-resource-name"
export AZURE_OPENAI_DEPLOYMENT="your-deployment-name"
export AZURE_OPENAI_API_VERSION="2024-08-01-preview"  # Optional
```

### Getting Azure OpenAI Credentials

1. **API Key**: Found in your Azure OpenAI resource under "Keys and Endpoint"
2. **Base URL**: The endpoint URL from your Azure OpenAI resource (without the `/openai/deployments/` part)
3. **Region**: The Azure region where your resource is deployed (e.g., "eastus", "westus2")
4. **Resource Name**: The name of your Azure OpenAI resource
5. **Deployment Name**: The name you gave to your model deployment in Azure OpenAI Studio (this serves as both the deployment and model identifier)
6. **API Version**: The Azure OpenAI API version (optional, defaults to `2024-08-01-preview`, required for structured output)

## Running the Example

```bash
# Set your environment variables
export AZURE_OPENAI_API_KEY="your-api-key"
export AZURE_OPENAI_BASE_URL="https://your-resource.openai.azure.com"
export AZURE_OPENAI_DEPLOYMENT="your-deployment-name"

# Run the example
go run main.go
```

## Code Explanation

### Creating the Client

```go
// Option 1: Using Base URL (traditional approach)
client := azureopenai.NewClient(
    apiKey,
    baseURL,
    deployment,    // Deployment name serves as the model identifier
    azureopenai.WithLogger(logger),
    azureopenai.WithAPIVersion(apiVersion), // Optional
)

// Option 2: Using Region and Resource Name (recommended)
client := azureopenai.NewClientFromRegion(
    apiKey,
    region,        // e.g., "eastus"
    resourceName,  // e.g., "my-openai-resource"
    deployment,    // e.g., "gpt-4-deployment" (this is your model identifier)
    azureopenai.WithLogger(logger),
    azureopenai.WithAPIVersion(apiVersion),
)
```

### Text Generation

```go
response, err := client.Generate(
    ctx,
    "Write a haiku about programming",
    azureopenai.WithSystemMessage("You are a creative assistant."),
    azureopenai.WithTemperature(0.7),
)
```

### Chat Completion

```go
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

response, err := client.Chat(ctx, messages, nil)
```

### Reasoning Modes

The example demonstrates **two types of reasoning**:

#### 1. Prompt-Based Reasoning (WithReasoning)
This uses system message enhancement to encourage the model to explain its thinking:

- **none**: Direct, concise answers without explanation (e.g., simple calculations)
- **minimal**: Brief explanations of the thought process (e.g., scientific concepts)
- **comprehensive**: Detailed step-by-step reasoning (e.g., complex problem-solving)

#### 2. Native Reasoning Models (WithReasoning from interfaces)
For models like o1-preview, o1-mini that have built-in reasoning capabilities:

- Uses the model's native reasoning tokens
- Automatically detected based on model name
- Requires temperature = 1.0
- Provides internal "thinking" process

```go
// Prompt-based reasoning (works with any model)
response, err := client.Generate(
    ctx,
    "What is 15 * 24?",
    azureopenai.WithReasoning("none"), // System message enhancement
)

response, err := client.Generate(
    ctx,
    "Explain why the sky appears blue",
    azureopenai.WithReasoning("minimal"), // Brief explanations
)

response, err := client.Generate(
    ctx,
    "How would you design a recommendation system?",
    azureopenai.WithReasoning("comprehensive"), // Detailed reasoning
)

// Native reasoning (only for o1-preview, o1-mini, etc.)
response, err := client.Generate(
    ctx,
    "Solve this step by step: complex math problem",
    interfaces.WithReasoning(true), // Enables native reasoning tokens
    azureopenai.WithTemperature(1.0), // Required for reasoning models
)
```

### Streaming Examples

The example includes several streaming demonstrations:

#### Basic Streaming
```go
eventChan, err := client.GenerateStream(
    ctx,
    "Tell me a short story about a robot learning to paint",
    azureopenai.WithTemperature(0.8),
)

for event := range eventChan {
    switch event.Type {
    case interfaces.StreamEventContentDelta:
        fmt.Print(event.Content) // Print content as it arrives
    case interfaces.StreamEventError:
        log.Printf("Stream error: %v", event.Error)
    }
}
```

#### Streaming with Reasoning
```go
eventChan, err := client.GenerateStream(
    ctx,
    "Explain the process of photosynthesis step by step",
    azureopenai.WithReasoning("comprehensive"),
    azureopenai.WithTemperature(0.4),
)
```

#### Advanced Streaming with Custom Configuration
```go
streamConfig := interfaces.StreamConfig{
    BufferSize: 50, // Custom buffer size
}

eventChan, err := client.GenerateStream(
    ctx,
    "Write a haiku about technology and nature",
    azureopenai.WithReasoning("minimal"),
    interfaces.WithStreamConfig(streamConfig),
)
```

### Structured Output

```go
jsonSchema := map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "language": map[string]interface{}{
            "type": "string",
            "description": "The programming language name",
        },
        "benefits": map[string]interface{}{
            "type": "array",
            "items": map[string]interface{}{
                "type": "string",
            },
        },
    },
    "required": []string{"language", "benefits"},
}

response, err := client.Generate(
    ctx,
    "Describe the Go programming language",
    azureopenai.WithResponseFormat(interfaces.ResponseFormat{
        Name:   "language_info",
        Schema: jsonSchema,
    }),
)
```

## Available Options

The Azure OpenAI client provides several option functions for configuring requests:

- `WithTemperature(float64)` - Controls randomness (0.0 to 1.0)
- `WithTopP(float64)` - Controls diversity via nucleus sampling
- `WithFrequencyPenalty(float64)` - Reduces repetition of token sequences
- `WithPresencePenalty(float64)` - Reduces repetition of topics
- `WithStopSequences([]string)` - Specifies sequences where generation should stop
- `WithSystemMessage(string)` - Sets the system message
- `WithResponseFormat(ResponseFormat)` - Enables structured JSON output
- `WithReasoning(string)` - Controls reasoning verbosity

## Azure-Specific Configuration

### Deployment Names

Unlike the standard OpenAI API, Azure OpenAI uses deployment names to route requests to specific models. Make sure your deployment name matches exactly what you created in Azure OpenAI Studio.

### API Versions

Azure OpenAI uses API versioning. The client defaults to `2024-02-15-preview`, but you can specify a different version:

```go
client := azureopenai.NewClient(
    apiKey,
    baseURL,
    deployment,
    azureopenai.WithAPIVersion("2023-12-01-preview"),
)
```

### Endpoint Structure

The client automatically constructs the correct Azure OpenAI endpoint:
- Input: `https://your-resource.openai.azure.com`
- Constructed: `https://your-resource.openai.azure.com/openai/deployments/your-deployment`

## Error Handling

The example includes comprehensive error handling:

```go
if err != nil {
    logger.Error(ctx, "Operation failed", map[string]interface{}{
        "error": err.Error(),
    })
    os.Exit(1)
}
```

## Expected Output

When you run the example, you should see output similar to:

```
INFO: Azure OpenAI client created
INFO: Testing simple text generation...
INFO: Generated haiku: Code flows like water...
INFO: Testing chat completion...
INFO: Chat response: In Go, error handling is explicit...
INFO: Testing reasoning modes...
INFO: Testing basic streaming...

[STREAM START] Once upon a time, in a small workshop...
[STREAM COMPLETE]
INFO: Stream finished

INFO: Testing reasoning modes with detailed examples...
INFO: Testing streaming with reasoning...

=== STREAMING WITH REASONING ===
[REASONING STREAM] Photosynthesis is a complex process...
[REASONING COMPLETE]

INFO: Testing advanced streaming with custom configuration...

=== ADVANCED STREAMING ===
[15:04:05] Stream started
Silicon dreams merge
With ancient oak's whispered song—
Progress finds its roots
[15:04:07] Content complete

INFO: All Azure OpenAI tests completed successfully!
```

## Troubleshooting

### Common Issues

1. **Authentication Error**: Verify your API key is correct and has the necessary permissions
2. **Deployment Not Found**: Ensure the deployment name matches exactly (case-sensitive)
3. **Endpoint Error**: Check that your base URL is correct and doesn't include the deployment path
4. **API Version Error**: Try using the default API version or a supported version

### Debug Logging

The example uses structured logging. You can increase log verbosity to debug issues:

```go
logger := logging.New() // Add debug level configuration if needed
```

## Additional Examples

For more advanced usage, see:
- Tool integration examples in `examples/tools/`
- Streaming examples in `examples/streaming/`
- Agent integration examples in `examples/agent/`

## Tool Integration

The Azure OpenAI client also supports tool calling with the `GenerateWithTools` method. See the agent examples for demonstrations of tool integration.
