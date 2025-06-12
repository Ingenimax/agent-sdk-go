# Vertex AI Client

This package provides a comprehensive Vertex AI client implementation for the agent-sdk-go project, enabling seamless integration with Google Cloud's Vertex AI generative models including the full Gemini family.

## Features

- **Multiple Gemini Models**: Support for Gemini 1.5 Pro, Gemini 1.5 Flash, Gemini 2.0 Flash, and Gemini Pro Vision
- **Advanced Tool Calling**: Full support for function calling with proper conversation flow handling
- **Intelligent Retry Logic**: Built-in exponential backoff retry mechanism with rate limit protection
- **Reasoning Modes**: Configurable reasoning approaches (none, minimal, comprehensive) for enhanced AI thinking
- **System Instructions**: Enhanced system message handling with reasoning integration
- **Flexible Authentication**: Support for Application Default Credentials (ADC) and service account files
- **Multi-Region Support**: Configurable Google Cloud regions for optimal latency
- **Comprehensive Logging**: Structured logging with configurable log levels
- **Rate Limit Management**: Built-in protection against API rate limits with smart backoff

## Installation

To use this client, you need to install the required dependencies:

```bash
go get cloud.google.com/go/vertexai/genai@v0.13.4
go get google.golang.org/api/option
go get github.com/cenkalti/backoff/v4
```

## Prerequisites

### Google Cloud Setup

1. **Enable Vertex AI API**: Enable the Vertex AI API in your Google Cloud project
   ```bash
   gcloud services enable aiplatform.googleapis.com --project=YOUR_PROJECT_ID
   ```

2. **Authentication**: Set up authentication using one of these methods:
   - **Application Default Credentials (ADC)**: Recommended for most use cases
   - **Service Account File**: For explicit credential management

### Authentication Setup

#### Option 1: Application Default Credentials (ADC)
```bash
# Install gcloud CLI
curl https://sdk.cloud.google.com | bash
exec -l $SHELL

# Authenticate
gcloud auth application-default login
gcloud config set project YOUR_PROJECT_ID
```

#### Option 2: Service Account Key File
```bash
# Create and download service account key
gcloud iam service-accounts create vertex-ai-service
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
    --member="serviceAccount:vertex-ai-service@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/aiplatform.user"
gcloud iam service-accounts keys create key.json \
    --iam-account=vertex-ai-service@YOUR_PROJECT_ID.iam.gserviceaccount.com

# Set environment variable
export GOOGLE_APPLICATION_CREDENTIALS="path/to/key.json"
```

## Usage

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/vertex"
)

func main() {
    ctx := context.Background()
    
    // Create client with default settings
    client, err := vertex.NewClient(ctx, "your-project-id")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Generate response
    response, err := client.Generate(ctx, "What is the capital of France?")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Response:", response)
}
```

### Advanced Configuration

```go
package main

import (
    "context"
    "log"
    "log/slog"
    "time"

    "github.com/Ingenimax/agent-sdk-go/pkg/llm/vertex"
)

func main() {
    ctx := context.Background()
    
    // Create client with custom configuration
    client, err := vertex.NewClient(ctx, "your-project-id",
        vertex.WithModel(vertex.ModelGemini15Flash),
        vertex.WithLocation("us-west1"),
        vertex.WithMaxRetries(5),
        vertex.WithRetryDelay(2*time.Second),
        vertex.WithReasoningMode(vertex.ReasoningModeComprehensive),
        vertex.WithLogger(slog.Default()),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Generate with system message and reasoning
    response, err := client.Generate(ctx, "Explain quantum entanglement",
        func(options *interfaces.GenerateOptions) {
            options.SystemMessage = "You are a physics professor explaining complex concepts to undergraduate students."
            options.LLMConfig = &interfaces.LLMConfig{
                Temperature: 0.7,
                TopP:        0.9,
                Reasoning:   "comprehensive",
            }
        },
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Response:", response)
}
```

### Using Service Account Credentials

```go
package main

import (
    "context"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/llm/vertex"
)

func main() {
    ctx := context.Background()
    
    // Create client with service account credentials
    client, err := vertex.NewClient(ctx, "your-project-id",
        vertex.WithCredentialsFile("path/to/service-account-key.json"),
        vertex.WithModel(vertex.ModelGemini15Pro),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()
}
```

### Tool Calling

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/vertex"
)

// Example tool implementation
type CalculatorTool struct{}

func (t *CalculatorTool) Name() string {
    return "calculator"
}

func (t *CalculatorTool) Description() string {
    return "Performs basic arithmetic operations"
}

func (t *CalculatorTool) Parameters() map[string]interfaces.ParameterSpec {
    return map[string]interfaces.ParameterSpec{
        "operation": {
            Type:        "string",
            Description: "The operation to perform: add, subtract, multiply, divide",
            Required:    true,
            Enum:        []interface{}{"add", "subtract", "multiply", "divide"},
        },
        "a": {
            Type:        "number",
            Description: "First number",
            Required:    true,
        },
        "b": {
            Type:        "number",
            Description: "Second number",
            Required:    true,
        },
    }
}

func (t *CalculatorTool) Execute(ctx context.Context, args string) (string, error) {
    // Parse args and perform calculation
    // This is a simplified example
    return "The calculation result is 42", nil
}

func main() {
    ctx := context.Background()
    
    client, err := vertex.NewClient(ctx, "your-project-id")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Define tools
    tools := []interfaces.Tool{
        &CalculatorTool{},
    }

    // Generate response with tools
    response, err := client.GenerateWithTools(ctx, 
        "What's 25 multiplied by 17?", 
        tools,
        func(options *interfaces.GenerateOptions) {
            options.SystemMessage = "You are a helpful assistant that can perform calculations."
        },
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Tool response:", response)
}
```

### Different Reasoning Modes

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/vertex"
)

func main() {
    ctx := context.Background()
    
    // Create clients with different reasoning modes
    clients := map[string]*vertex.Client{}
    
    modes := []vertex.ReasoningMode{
        vertex.ReasoningModeNone,
        vertex.ReasoningModeMinimal, 
        vertex.ReasoningModeComprehensive,
    }
    
    for _, mode := range modes {
        client, err := vertex.NewClient(ctx, "your-project-id",
            vertex.WithModel(vertex.ModelGemini15Pro),
            vertex.WithReasoningMode(mode),
        )
        if err != nil {
            log.Fatal(err)
        }
        clients[string(mode)] = client
        defer client.Close()
    }

    prompt := "How do neural networks learn?"

    for modeName, client := range clients {
        fmt.Printf("\n=== %s Reasoning ===\n", modeName)
        
        response, err := client.Generate(ctx, prompt,
            func(options *interfaces.GenerateOptions) {
                options.LLMConfig = &interfaces.LLMConfig{
                    Temperature: 0.3,
                }
            },
        )
        if err != nil {
            log.Printf("Error with %s: %v", modeName, err)
            continue
        }
        
        fmt.Println(response)
    }
}
```

### Multiple Models Comparison

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/Ingenimax/agent-sdk-go/pkg/llm/vertex"
)

func main() {
    ctx := context.Background()
    
    models := []string{
        vertex.ModelGemini15Pro,
        vertex.ModelGemini15Flash,
        vertex.ModelGemini20Flash,
    }
    
    prompt := "Write a short explanation of quantum computing"
    
    for _, model := range models {
        fmt.Printf("\n=== Testing %s ===\n", model)
        
        // Add rate limiting between requests
        if model != models[0] {
            time.Sleep(2 * time.Second)
        }
        
        client, err := vertex.NewClient(ctx, "your-project-id",
            vertex.WithModel(model),
        )
        if err != nil {
            log.Printf("Failed to create client for %s: %v", model, err)
            continue
        }
        
        start := time.Now()
        response, err := client.Generate(ctx, prompt)
        duration := time.Since(start)
        
        if err != nil {
            log.Printf("Error with %s: %v", model, err)
        } else {
            fmt.Printf("Model: %s\n", model)
            fmt.Printf("Duration: %v\n", duration)
            fmt.Printf("Response: %s\n", response)
        }
        
        client.Close()
    }
}
```

## Configuration Options

### Client Options

- `WithModel(model string)`: Set the Gemini model to use
- `WithLocation(location string)`: Set the Google Cloud region
- `WithLogger(logger *slog.Logger)`: Set custom logger
- `WithMaxRetries(maxRetries int)`: Set maximum retry attempts (default: 3)
- `WithRetryDelay(delay time.Duration)`: Set retry delay (default: 1 second)
- `WithReasoningMode(mode ReasoningMode)`: Set reasoning approach
- `WithCredentialsFile(path string)`: Set service account credentials file

### Available Models

- `ModelGemini15Pro`: "gemini-1.5-pro" (default) - Best for complex reasoning tasks
- `ModelGemini15Flash`: "gemini-1.5-flash" - Faster responses, good performance
- `ModelGemini20Flash`: "gemini-2.0-flash-exp" - Latest experimental model
- `ModelGeminiProVision`: "gemini-pro-vision" - Optimized for image understanding

### Reasoning Modes

- `ReasoningModeNone`: Standard response generation (default)
- `ReasoningModeMinimal`: Brief explanations when necessary
- `ReasoningModeComprehensive`: Detailed step-by-step reasoning

### Supported Google Cloud Regions

- `us-central1` (default) - Iowa, USA
- `us-west1` - Oregon, USA
- `us-east1` - South Carolina, USA
- `europe-west1` - Belgium
- `asia-northeast1` - Tokyo, Japan
- And other regions where Vertex AI is available

## Error Handling

The client includes comprehensive error handling with retry logic:

```go
client, err := vertex.NewClient(ctx, "your-project-id",
    vertex.WithMaxRetries(3),
    vertex.WithRetryDelay(time.Second*2),
)
if err != nil {
    // Handle client creation error
    log.Fatal(err)
}

response, err := client.Generate(ctx, "Hello")
if err != nil {
    // Handle generation error (after retries)
    log.Printf("Generation failed: %v", err)
}
```

## Rate Limiting

The client automatically handles rate limits with:

- **Exponential backoff retry**: Automatic retry with increasing delays
- **Smart delay calculation**: Randomized delays to prevent thundering herd
- **Configurable retry limits**: Adjust max retries based on your needs
- **Context awareness**: Respects context cancellation during retries

## Best Practices

1. **Resource Management**: Always call `defer client.Close()` to properly clean up resources
2. **Authentication**: Use Application Default Credentials (ADC) when possible for better security
3. **Error Handling**: Implement proper error handling for network and API failures
4. **Retry Configuration**: Adjust retry settings based on your application's requirements
5. **Location Selection**: Choose a location close to your users for better latency
6. **Model Selection**: 
   - Use Gemini 1.5 Flash for faster responses
   - Use Gemini 1.5 Pro for complex reasoning tasks
   - Use Gemini 2.0 Flash for latest capabilities (experimental)
7. **Rate Limiting**: Add delays between consecutive requests when making many API calls
8. **Reasoning Modes**: Use appropriate reasoning modes based on your use case

## Troubleshooting

### Common Issues

1. **Authentication Errors**
   ```
   Error: failed to create Vertex AI client: google: could not find default credentials
   ```
   - Solution: Set up ADC or provide service account credentials
   - Verify: `gcloud auth application-default login`

2. **Permission Denied Errors**
   ```
   Error: rpc error: code = PermissionDenied desc = Permission denied on resource project
   ```
   - Solution: Enable Vertex AI API and check IAM permissions
   - Command: `gcloud services enable aiplatform.googleapis.com`

3. **Rate Limit Errors**
   ```
   Error: rpc error: code = ResourceExhausted desc = Quota exceeded
   ```
   - Solution: The client automatically retries with backoff
   - Tip: Increase retry delay or add manual delays between requests

4. **Tool Calling Issues**
   ```
   Error: function call turn contains at least one function_call part which can not be mixed with function_response parts
   ```
   - Solution: This is handled automatically by the client's conversation flow management

### Debug Logging

Enable debug logging to troubleshoot issues:

```go
import "log/slog"

logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))

client, err := vertex.NewClient(ctx, "your-project-id",
    vertex.WithLogger(logger),
)
```

## Contributing

When contributing to this client:

1. Follow the established patterns from other LLM clients in the SDK
2. Ensure proper error handling and logging throughout
3. Add comprehensive tests for new features
4. Update documentation for any API changes
5. Test with multiple Gemini models and configurations
6. Verify rate limiting protection works correctly

## License

This client is part of the agent-sdk-go project and follows the same licensing terms. 