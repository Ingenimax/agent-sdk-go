# OpenAI LLM Example

This example demonstrates how to use the OpenAI client from the Agent SDK.

## Features

- Direct text generation using the `Generate` method
- Chat completion using the `Chat` method
- Configuration options for model parameters

## Usage

### Prerequisites

- Set the `OPENAI_API_KEY` environment variable with your OpenAI API key

```bash
export OPENAI_API_KEY=your_api_key_here
```

### Running the Example

```bash
go run main.go
```

## Code Explanation

### Creating the Client

```go
client := openai.NewClient(
    apiKey,
    openai.WithModel("gpt-4o-mini"), // Optional: specify model
)
```

### Text Generation

```go
response, err := client.Generate(
    context.Background(),
    "Write a haiku about programming",
    openai.WithTemperature(0.7),
    openai.WithMaxTokens(50),
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

response, err := client.Chat(context.Background(), messages, nil)
```

### Available Options

The OpenAI client provides several option functions for configuring requests:

- `WithTemperature(float64)` - Controls randomness (0.0 to 1.0)
- `WithMaxTokens(int)` - Sets maximum response length
- `WithTopP(float64)` - Controls diversity via nucleus sampling
- `WithFrequencyPenalty(float64)` - Reduces repetition of token sequences
- `WithPresencePenalty(float64)` - Reduces repetition of topics
- `WithStopSequences([]string)` - Specifies sequences where generation should stop

## Tool Integration

The OpenAI client also supports tool calling with the `GenerateWithTools` method. See the agent examples for demonstrations of tool integration.

## Additional Examples

### Reasoning Support

This package includes an example demonstrating how to use the reasoning capability:

```bash
cd reasoning
go run main.go
```

The reasoning example shows how to control the verbosity and detail of the model's thought process through the `WithReasoning` option. See the [reasoning example README](reasoning/README.md) for more details.
