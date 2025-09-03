# vLLM LLM Provider

This package provides integration with vLLM, a fast and efficient library for LLM inference and serving. vLLM is designed for high-performance, low-latency inference of large language models.

## Features

- **High Performance**: Optimized for fast inference with PagedAttention
- **Memory Efficiency**: Efficient memory management for large models
- **OpenAI-Compatible API**: Uses OpenAI-compatible REST API
- **Multiple Model Support**: Support for various models like Llama2, Mistral, CodeLlama, etc.
- **Chat Completions**: Full chat conversation support
- **Tool Integration**: Basic tool integration (descriptive approach)
- **Model Management**: List available models
- **Retry Logic**: Built-in retry mechanism for reliability
- **Logging**: Integrated logging support
- **Structured Output**: JSON schema-based structured responses
- **Beam Search**: Support for beam search inference
- **Batch Processing**: Efficient batch processing capabilities

## Prerequisites

1. **Install vLLM**: Follow the installation instructions at [vLLM GitHub](https://github.com/vllm-project/vllm)
2. **Start vLLM Server**: Run vLLM server with your model
3. **Model Files**: Ensure your model is available for vLLM

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/llm/vllm"
    "github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func main() {
    // Create a logger
    logger := logging.New()

    // Create vLLM client
    client := vllm.NewClient(
        vllm.WithModel("llama-2-7b"),
        vllm.WithLogger(logger),
        vllm.WithBaseURL("http://localhost:8000"),
    )

    // Generate text
    response, err := client.Generate(
        context.Background(),
        "Write a haiku about programming",
        vllm.WithTemperature(0.7),
        vllm.WithSystemMessage("You are a creative assistant who specializes in writing haikus."),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Generated text:", response)
}
```

## Configuration Options

### Client Options

- `WithModel(model string)`: Set the model to use (default: "llama-2-7b")
- `WithBaseURL(baseURL string)`: Set the vLLM server URL (default: "http://localhost:8000")
- `WithLogger(logger logging.Logger)`: Set a custom logger
- `WithRetry(opts ...retry.Option)`: Configure retry policy
- `WithHTTPClient(httpClient *http.Client)`: Set a custom HTTP client

### Generation Options

- `WithTemperature(temperature float64)`: Controls randomness (0.0 to 1.0)
- `WithTopP(topP float64)`: Controls diversity via nucleus sampling
- `WithStopSequences(stopSequences []string)`: Specifies sequences where generation should stop
- `WithSystemMessage(systemMessage string)`: Sets a system message for the model
- `WithResponseFormat(format interfaces.ResponseFormat)`: Sets expected response format

## Usage Examples

### Basic Text Generation

```go
response, err := client.Generate(
    ctx,
    "Explain quantum computing in simple terms",
    vllm.WithTemperature(0.7),
    vllm.WithSystemMessage("You are a helpful physics teacher."),
)
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
    vllm.WithTemperature(0.7),
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

### Structured Output

Use structured output with automatic schema generation:

```go
type Person struct {
    Name        string   `json:"name" description:"The person's full name"`
    Profession  string   `json:"profession" description:"Their primary occupation"`
    Description string   `json:"description" description:"A brief biography"`
    Companies   []Company `json:"companies,omitempty" description:"Companies they've worked for"`
}

type Company struct {
    Name        string `json:"name" description:"Company name"`
    Country     string `json:"country" description:"Country where company is headquartered"`
    Description string `json:"description" description:"Brief description of the company"`
}

// Create response format automatically
personFormat := structuredoutput.NewResponseFormat(Person{})

// Generate structured response
response, err := client.Generate(
    ctx,
    "Tell me about Albert Einstein",
    vllm.WithResponseFormat(*personFormat),
)

// Unmarshal directly into struct
var person Person
json.Unmarshal([]byte(response), &person)
```

## Environment Variables

You can configure the vLLM client using environment variables:

```bash
export VLLM_BASE_URL=http://localhost:8000
export VLLM_MODEL=llama-2-7b
```

## Supported Models

vLLM supports many models. Some popular ones include:

- `llama-2-7b`: Meta's Llama 2 7B model
- `llama-2-13b`: Meta's Llama 2 13B model
- `llama-2-70b`: Meta's Llama 2 70B model
- `mistral-7b`: Mistral AI's 7B model
- `codellama-7b`: Code-focused Llama model
- `vicuna-7b`: Vicuna fine-tuned model

## Performance Considerations

- **PagedAttention**: vLLM uses PagedAttention for efficient memory management
- **GPU Optimization**: Optimized for GPU inference with CUDA
- **Batch Processing**: Efficient handling of multiple requests
- **Memory Efficiency**: Better memory usage than traditional inference
- **Low Latency**: Designed for high-throughput, low-latency inference

## Error Handling

The client includes comprehensive error handling:

- Network errors are retried automatically (if retry is configured)
- Invalid responses are properly handled
- Model not found errors are clearly reported
- Memory allocation errors are handled gracefully

## API Endpoints

The vLLM client uses these API endpoints:

- `POST /v1/completions`: Generate text from a prompt
- `POST /v1/chat/completions`: Chat completion with messages
- `GET /v1/models`: List available models

## Comparison with Other Providers

| Feature | vLLM | Ollama | OpenAI | VertexAI |
|---------|------|--------|--------|----------|
| Local Inference | ✅ | ✅ | ❌ | ❌ |
| Performance | Very High | Medium | High | High |
| Memory Efficiency | Very High | Medium | High | High |
| Model Management | ✅ | ✅ | ❌ | ❌ |
| Structured Output | ✅ | ✅ | ✅ | ✅ |
| Tool Integration | Basic | Basic | Full | Full |
| Cost | Low | Low | High | High |
| GPU Optimization | Excellent | Good | N/A | N/A |

## Starting vLLM Server

### Basic Setup

```bash
# Install vLLM
pip install vllm

# Start server with a model
python -m vllm.entrypoints.openai.api_server \
    --model llama-2-7b \
    --host 0.0.0.0 \
    --port 8000
```

### Advanced Configuration

```bash
# Start with specific settings
python -m vllm.entrypoints.openai.api_server \
    --model llama-2-7b \
    --host 0.0.0.0 \
    --port 8000 \
    --tensor-parallel-size 2 \
    --gpu-memory-utilization 0.9 \
    --max-model-len 4096
```

## Troubleshooting

### Common Issues

1. **Connection Refused**: Ensure the vLLM server is running on the correct port
2. **Model Not Found**: Check if the model is loaded using `ListModels()`
3. **Out of Memory**: Reduce batch size or use smaller models
4. **Slow Response**: Check GPU utilization and model size

### Debug Mode

Enable debug logging to troubleshoot issues:

```go
logger := logging.New()
logger.SetLevel(logging.DebugLevel)

client := vllm.NewClient(
    vllm.WithLogger(logger),
)
```

### Performance Tuning

- **GPU Memory**: Adjust `--gpu-memory-utilization` based on your GPU
- **Batch Size**: Optimize batch size for your use case
- **Model Size**: Use smaller models for faster inference
- **Tensor Parallel**: Use multiple GPUs for large models

## Contributing

When contributing to the vLLM client:

1. Follow the existing code patterns
2. Add comprehensive tests for new features
3. Update documentation for API changes
4. Ensure backward compatibility
5. Test with multiple model types
6. Consider vLLM-specific optimizations
