# Ollama LLM Provider

This package provides integration with Ollama, a local LLM server that allows you to run various open-source language models locally.

## Features

- **Local Model Support**: Run models locally without external API calls
- **Multiple Model Support**: Support for various models like Llama2, Mistral, CodeLlama, etc.
- **Chat Completions**: Full chat conversation support
- **Tool Integration**: Basic tool integration (descriptive approach)
- **Model Management**: List and pull models
- **Retry Logic**: Built-in retry mechanism for reliability
- **Logging**: Integrated logging support
- **Structured Output**: JSON schema-based structured responses

## Prerequisites

1. **Install Ollama**: Follow the installation instructions at [ollama.ai](https://ollama.ai)
2. **Start Ollama Server**: Run `ollama serve` to start the server
3. **Pull a Model**: Run `ollama pull llama2` to download a model

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/llm/ollama"
    "github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func main() {
    // Create a logger
    logger := logging.New()

    	// Create Ollama client
	client := ollama.NewClient(
		ollama.WithModel("qwen3:0.6b"),
		ollama.WithLogger(logger),
		ollama.WithBaseURL("http://localhost:11434"),
	)

    // Generate text
    response, err := client.Generate(
        context.Background(),
        "Write a haiku about programming",
        ollama.WithTemperature(0.7),
        ollama.WithSystemMessage("You are a creative assistant who specializes in writing haikus."),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Generated text:", response)
}
```

## Configuration Options

### Client Options

- `WithModel(model string)`: Set the model to use (default: "qwen3:0.6b")
- `WithBaseURL(baseURL string)`: Set the Ollama server URL (default: "http://localhost:11434")
- `WithLogger(logger logging.Logger)`: Set a custom logger
- `WithRetry(opts ...retry.Option)`: Configure retry behavior
- `WithHTTPClient(httpClient *http.Client)`: Set a custom HTTP client

### Generation Options

- `WithTemperature(temperature float64)`: Control randomness (0.0 to 1.0)
- `WithTopP(topP float64)`: Nucleus sampling parameter
- `WithStopSequences(stopSequences []string)`: Stop generation at these sequences
- `WithSystemMessage(systemMessage string)`: Set system message for chat models
- `WithResponseFormat(format interfaces.ResponseFormat)`: Set structured output format

## API Methods

### Generate

Generate text from a prompt:

```go
response, err := client.Generate(
    ctx,
    "Explain quantum computing in simple terms",
    ollama.WithTemperature(0.8),
)
```

### Structured Output

Generate structured JSON responses using JSON schemas:

```go
// Define your response structure
type Person struct {
    Name        string `json:"name" description:"The person's full name"`
    Profession  string `json:"profession" description:"Their primary occupation"`
    Description string `json:"description" description:"A brief biography"`
    BirthDate   string `json:"birth_date,omitempty" description:"Date of birth"`
}

// Create response format automatically from struct
personFormat := structuredoutput.NewResponseFormat(Person{})

// Generate structured response
response, err := client.Generate(
    ctx,
    "Tell me about Albert Einstein",
    ollama.WithResponseFormat(*personFormat),
    ollama.WithTemperature(0.7),
)

// Unmarshal into your struct
var person Person
json.Unmarshal([]byte(response), &person)
fmt.Printf("Name: %s, Profession: %s\n", person.Name, person.Profession)
```

### Chat

Perform chat completions with message history:

```go
messages := []llm.Message{
    {
        Role:    "system",
        Content: "You are a helpful programming assistant.",
    },
    {
        Role:    "user",
        Content: "How do I implement a binary search in Go?",
    },
}

response, err := client.Chat(ctx, messages, &llm.GenerateParams{
    Temperature: 0.7,
})
```

### GenerateWithTools

Generate text with tool descriptions (basic implementation):

```go
tools := []interfaces.Tool{
    // Your tools here
}

response, err := client.GenerateWithTools(
    ctx,
    "What's the weather like?",
    tools,
    ollama.WithTemperature(0.7),
)
```

### Model Management

List available models:

```go
models, err := client.ListModels(ctx)
if err != nil {
    log.Fatal(err)
}

for _, model := range models {
    fmt.Println("Available model:", model)
}
```

Pull a new model:

```go
err := client.PullModel(ctx, "mistral")
if err != nil {
    log.Fatal(err)
}
```

## Environment Variables

You can configure the Ollama client using environment variables:

```bash
export OLLAMA_BASE_URL=http://localhost:11434
export OLLAMA_MODEL=qwen3:0.6b
```

## Supported Models

Ollama supports many models. Some popular ones include:

- `qwen3:0.6b`: Alibaba's Qwen3 0.6B model (fast and efficient)
- `qwen3:1.5b`: Alibaba's Qwen3 1.5B model
- `qwen3:7b`: Alibaba's Qwen3 7B model
- `llama2`: Meta's Llama 2 model
- `mistral`: Mistral AI's model
- `codellama`: Code-focused Llama model

To see all available models, visit the [Ollama Library](https://ollama.ai/library).

## Error Handling

The client includes comprehensive error handling:

- Network errors are retried automatically (if retry is configured)
- Invalid responses are properly handled
- Model not found errors are clearly reported

## Performance Considerations

- **Local Processing**: All inference happens locally, reducing latency
- **Model Loading**: Models are loaded into memory, subsequent requests are faster
- **Resource Usage**: Larger models require more RAM and GPU memory
- **Concurrent Requests**: Ollama handles concurrent requests efficiently

## Limitations

- **Tool Calling**: Ollama doesn't support structured tool calling like OpenAI/Anthropic. The implementation uses a descriptive approach.
- **Model Size**: Large models require significant system resources
- **Response Quality**: Quality depends on the specific model used

## Troubleshooting

### Common Issues

1. **Connection Refused**: Make sure Ollama server is running (`ollama serve`)
2. **Model Not Found**: Pull the model first (`ollama pull model-name`)
3. **Out of Memory**: Use a smaller model or increase system RAM
4. **Slow Responses**: Consider using a smaller model or better hardware

### Debug Mode

Enable debug logging to troubleshoot issues:

```go
logger := logging.New()
logger.SetLevel("debug")

client := ollama.NewClient(
    ollama.WithLogger(logger),
)
```

## Examples

See the `examples/llm/ollama/` directory for complete working examples:

- `main.go`: Basic text generation and chat examples
- `agent_integration/main.go`: Agent framework integration
- `structured_output/main.go`: Comprehensive structured output examples

### Running Examples

```bash
# Basic examples
go run examples/llm/ollama/main.go

# Agent integration
go run examples/llm/ollama/agent_integration/main.go

# Structured output examples
go run examples/llm/ollama/structured_output/main.go
```
