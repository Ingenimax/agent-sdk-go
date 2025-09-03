# Gemini LLM Examples

This directory contains examples demonstrating how to use the Gemini API integration in the Agent SDK.

## Prerequisites

Before running these examples, you need:

1. **Google Cloud API Key**: Get your API key from the [Google AI Studio](https://aistudio.google.com/app/apikey)
2. **Environment Variable**: Set your API key as an environment variable:
   ```bash
   export GEMINI_API_KEY="your-api-key-here"
   ```

## Available Examples

### Basic Usage (`main.go`)
Demonstrates basic text generation with the Gemini API, including:
- Simple text generation
- Structured output with JSON schemas
- Function calling with tools
- Streaming responses
- Different reasoning modes

### Agent Integration (`agent_integration/main.go`)
Shows how to integrate Gemini with the Agent SDK framework:
- Creating agents with Gemini LLM
- Using memory and tools
- Multi-modal capabilities

### Structured Output (`structured_output/main.go`)
Advanced example of structured output generation:
- Complex JSON schemas
- Data extraction and formatting
- Response validation

### Streaming Example (`streaming/main.go`)
Demonstrates streaming capabilities:
- Real-time response streaming
- Tool execution with streaming
- Event handling

### Multi-modal Example (`multimodal/main.go`)
Shows vision and audio capabilities:
- Image analysis and description
- Document processing
- Video understanding

## Supported Models

The Gemini integration supports multiple models:

- `gemini-2.5-pro-latest` - Most capable model with vision, audio, and tool calling
- `gemini-2.5-flash-latest` - Fast model with vision, audio, and tool calling
- `gemini-2.5-flash-lite-latest` - Fastest model, text-only
- `gemini-1.5-pro` - Previous generation with vision and tool calling
- `gemini-1.5-flash` - Previous generation fast model with vision

## Features

### Text Generation
```go
client, err := gemini.NewClient(apiKey, gemini.WithModel(gemini.ModelGemini25Flash))
response, err := client.Generate(ctx, "Write a haiku about AI")
```

### Function Calling
```go
tools := []interfaces.Tool{calculator.New(), websearch.New()}
response, err := client.GenerateWithTools(ctx, "What's 15 * 7?", tools)
```

### Structured Output
```go
schema := interfaces.JSONSchema{
    "type": "object",
    "properties": map[string]interface{}{
        "summary": {"type": "string"},
        "confidence": {"type": "number"},
    },
}
response, err := client.Generate(ctx, prompt, gemini.WithResponseFormat(interfaces.ResponseFormat{
    Type: interfaces.ResponseFormatJSON,
    Schema: schema,
}))
```

### Streaming
```go
stream, err := client.GenerateStream(ctx, "Tell me a story")
for event := range stream {
    switch event.Type {
    case interfaces.StreamEventContentDelta:
        fmt.Print(event.Content)
    case interfaces.StreamEventError:
        fmt.Printf("Error: %v\n", event.Error)
    }
}
```

### Reasoning Modes
```go
// Comprehensive reasoning - detailed step-by-step explanations
response, err := client.Generate(ctx, "Solve this math problem: 2x + 5 = 13",
    gemini.WithReasoning("comprehensive"))

// Minimal reasoning - brief explanations
response, err := client.Generate(ctx, prompt, gemini.WithReasoning("minimal"))

// No reasoning - direct answers only
response, err := client.Generate(ctx, prompt, gemini.WithReasoning("none"))
```

## Running Examples

1. Set your API key:
   ```bash
   export GEMINI_API_KEY="your-api-key-here"
   ```

2. Run the basic example:
   ```bash
   cd examples/llm/gemini
   go run main.go
   ```

3. Run specific examples:
   ```bash
   go run agent_integration/main.go
   go run structured_output/main.go
   go run streaming/main.go
   ```

## Configuration Options

The Gemini client supports various configuration options:

```go
client, err := gemini.NewClient(apiKey,
    gemini.WithModel(gemini.ModelGemini25Pro),
    gemini.WithLogger(logger),
    gemini.WithRetry(retry.WithMaxRetries(3)),
    gemini.WithBaseURL("https://custom-api.example.com"),
)
```

## Error Handling

The client includes comprehensive error handling:
- Network timeouts and retries
- API rate limiting
- Invalid request formatting
- Content filtering and safety

## Safety and Content Filtering

Gemini includes built-in safety filtering. You can configure safety settings:

```go
// Default safety settings are applied automatically
// Blocks medium and high risk content for:
// - Harassment
// - Hate speech
// - Sexually explicit content
// - Dangerous content
```

## Best Practices

1. **API Key Security**: Never hardcode API keys. Use environment variables or secure key management.

2. **Model Selection**: Choose the right model for your use case:
   - Use Flash models for speed
   - Use Pro models for complex reasoning
   - Use Lite models for simple text tasks

3. **Error Handling**: Always handle errors appropriately and implement retry logic for production use.

4. **Rate Limiting**: Be aware of API rate limits and implement appropriate backoff strategies.

5. **Content Filtering**: Understand Gemini's content filtering and safety features.

6. **Token Management**: Monitor token usage, especially with large context models.

## Troubleshooting

### Common Issues

1. **Authentication Error**: Verify your API key is correct and has proper permissions.

2. **Model Not Found**: Ensure you're using a supported model name.

3. **Rate Limiting**: Implement exponential backoff for rate limit errors.

4. **Content Filtered**: Check if your content triggers safety filters.

5. **Network Timeouts**: Configure appropriate timeout values for your use case.

### Debug Logging

Enable debug logging to troubleshoot issues:

```go
logger := logging.New()
client, err := gemini.NewClient(apiKey, gemini.WithLogger(logger))
```

## Support

For issues specific to the Agent SDK Gemini integration, please check:
- [Agent SDK Documentation](../../docs/)
- [GitHub Issues](https://github.com/Ingenimax/agent-sdk-go/issues)

For Gemini API-specific questions:
- [Gemini API Documentation](https://ai.google.dev/gemini-api/docs)
- [Google AI Studio](https://aistudio.google.com/)
