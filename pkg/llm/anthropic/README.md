# Anthropic Client for Agent SDK

This package provides an implementation of the Anthropic API client for use with the Agent SDK, supporting Claude models including Claude-3.5-Haiku, Claude-3.5-Sonnet, Claude-3-Opus, and Claude-3.7-Sonnet.

## Supported Models

The client supports the following Claude models:

- `Claude35Haiku` (claude-3-5-haiku-latest) - Fast and cost-effective
- `Claude35Sonnet` (claude-3-5-sonnet-latest) - Balanced performance and capabilities
- `Claude3Opus` (claude-3-opus-latest) - Most powerful model with highest capabilities
- `Claude37Sonnet` (claude-3-7-sonnet-latest) - Latest model with improved capabilities

## Important: Model Specification Required

When creating an Anthropic client, you must explicitly specify the model to use via the `WithModel` option. The client will log a warning if no model is specified, but it's strongly recommended to always specify the model explicitly:

```go
// Always specify the model with WithModel
client := anthropic.NewClient(
	apiKey,
	anthropic.WithModel(anthropic.Claude37Sonnet), // Always specify the model
)
```

## API Version Note

This client uses the Anthropic API version 2023-06-01. Some features may not be supported in this version:

- The `reasoning` parameter is maintained for backward compatibility but is not officially supported in the current API.
- The `organization` parameter is not supported in the current API version.

## Usage Examples

### Basic Usage

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("ANTHROPIC_API_KEY environment variable not set")
		os.Exit(1)
	}

	// Create a new client with model explicitly specified
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(anthropic.Claude37Sonnet), // Always specify the model
	)

	// Create a client with custom settings
	// client := anthropic.NewClient(
	//     apiKey,
	//     anthropic.WithModel(anthropic.Claude37Sonnet), // Model is required
	//     anthropic.WithBaseURL("https://api.anthropic.com"),
	// )

	// Generate text
	ctx := context.Background()
	response, err := client.Generate(ctx, "Explain quantum computing in simple terms")
	if err != nil {
		fmt.Printf("Error generating text: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(response)
}
```

### Loading Configuration from Environment Variables

If your application needs to load configuration from environment variables:

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("ANTHROPIC_API_KEY environment variable not set")
		os.Exit(1)
	}

	// Get model from environment - REQUIRED
	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		// Default to Claude37Sonnet if not specified, but better to require it
		model = anthropic.Claude37Sonnet
		fmt.Println("Warning: ANTHROPIC_MODEL not set, using Claude37Sonnet")
	}

	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	timeout := 60
	if timeoutStr := os.Getenv("ANTHROPIC_TIMEOUT"); timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil && t > 0 {
			timeout = t
		}
	}

	temperature := 0.7
	if tempStr := os.Getenv("ANTHROPIC_TEMPERATURE"); tempStr != "" {
		if t, err := strconv.ParseFloat(tempStr, 64); err == nil {
			temperature = t
		}
	}

	// Create client with config from environment variables
	// Note that we always specify the model explicitly
	client := anthropic.NewClient(
		apiKey, 
		anthropic.WithModel(model), // Model is required
		anthropic.WithBaseURL(baseURL),
		anthropic.WithHTTPClient(&http.Client{Timeout: time.Duration(timeout) * time.Second}),
	)

	// Generate text
	ctx := context.Background()
	response, err := client.Generate(
		ctx, 
		"Explain quantum computing in simple terms",
		anthropic.WithTemperature(temperature),
	)
	if err != nil {
		fmt.Printf("Error generating text: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(response)
}
```

### Step-by-Step Reasoning

While the "reasoning" parameter is not officially supported in the current API version, Claude models naturally provide step-by-step reasoning for many types of problems:

```go
response, err := client.Generate(
    ctx, 
    "How would you solve this equation: 3x + 7 = 22?",
    // WithReasoning is maintained for backward compatibility but not officially supported
    anthropic.WithReasoning("comprehensive") 
)
```

### Chat Interface

The client also supports a chat interface for multi-turn conversations:

```go
messages := []llm.Message{
    {Role: "system", Content: "You are a helpful assistant."},
    {Role: "user", Content: "Tell me about the history of artificial intelligence."},
}

params := &llm.GenerateParams{
    Temperature: 0.7,
}

response, err := client.Chat(ctx, messages, params)
```

### Using Tools

The client supports tool calling with Claude models. Note that you need to provide an organization ID in the context:

```go
// Create context with organization ID
ctx = multitenancy.WithOrgID(ctx, "your-org-id")

// Define tools
tools := []interfaces.Tool{
    // Your tool implementations here
}

response, err := client.GenerateWithTools(
    ctx,
    "What's the weather in New York?",
    tools,
    anthropic.WithSystemMessage("You are a helpful assistant. Use tools when appropriate."),
    anthropic.WithTemperature(0.7)
)
```

### Creating an Agent

When creating an agent with the Anthropic client, you must provide both an organization ID and a conversation ID in the context:

```go
// Set up context with required values
ctx = context.Background()
ctx = multitenancy.WithOrgID(ctx, "your-org-id")
ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-id")

// Create memory store
memoryStore := memory.NewConversationBuffer()

// Create agent
agentInstance, err := agent.NewAgent(
    agent.WithLLM(client),
    agent.WithMemory(memoryStore),
    agent.WithSystemPrompt("You are a helpful AI assistant."),
)

// Run agent
response, err := agentInstance.Run(ctx, "Tell me about quantum computing")
```

## Configuration Options

Options for configuring the Anthropic client:

- `WithModel(model string)` - Set the model to use (e.g., `anthropic.Claude37Sonnet`)
- `WithBaseURL(baseURL string)` - Set a custom API endpoint
- `WithHTTPClient(client *http.Client)` - Set a custom HTTP client
- `WithLogger(logger logging.Logger)` - Set a custom logger
- `WithRetry(opts ...retry.Option)` - Configure retry policy

Options for generate requests:

- `WithTemperature(temperature float64)` - Control randomness (0.0 to 1.0)
- `WithTopP(topP float64)` - Alternative to temperature for nucleus sampling
- `WithSystemMessage(message string)` - Set system message
- `WithStopSequences(sequences []string)` - Set stop sequences
- `WithFrequencyPenalty(penalty float64)` - Set frequency penalty
- `WithPresencePenalty(penalty float64)` - Set presence penalty
- `WithReasoning(reasoning string)` - Maintained for compatibility but not officially supported

