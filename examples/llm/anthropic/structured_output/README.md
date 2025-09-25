# Anthropic Structured Output Example

This example demonstrates how to use structured output with the Anthropic Claude API to generate responses in specific JSON formats.

## Overview

Structured output ensures that Claude responds with data in a predefined format, making it easier to parse and integrate the responses into your applications. This is particularly useful for extracting structured information, generating consistent data formats, or building applications that need predictable response schemas.

## Features

- **Schema-based responses**: Define exactly what structure you want back
- **Multiple data types**: Support for strings, numbers, booleans, arrays, and nested objects
- **Consistent formatting**: Uses Claude's best practices for output consistency
- **Response prefilling**: Employs prefilling techniques to improve reliability
- **Example generation**: Automatically creates examples from schemas to guide Claude

## What This Example Does

The example shows four different use cases:

1. **Movie Review Structure**: Generate structured movie reviews with ratings, genres, pros/cons
2. **Weather Report Structure**: Create weather reports with temperature, conditions, forecast
3. **Product Analysis Structure**: Analyze products with features, pricing, target market
4. **Agent Integration**: Use structured output within an agent framework

## Prerequisites

- Go 1.19 or later
- An Anthropic API key

## Setup

1. Set your Anthropic API key:
```bash
export ANTHROPIC_API_KEY=your_api_key_here
```

2. Run the example:
```bash
cd examples/llm/anthropic/structured_output
go run main.go
```

## How It Works

### 1. Define Your Data Structure

```go
type MovieReview struct {
    Title       string   `json:"title" description:"The title of the movie"`
    Director    string   `json:"director" description:"The director of the movie"`
    Year        int      `json:"year" description:"The year the movie was released"`
    Genre       []string `json:"genre" description:"List of genres for the movie"`
    Rating      float64  `json:"rating" description:"Rating out of 10"`
    // ... more fields
}
```

### 2. Create Response Format

```go
responseFormat := structuredoutput.NewResponseFormat(MovieReview{})
```

### 3. Generate Structured Response

```go
response, err := client.Generate(
    ctx,
    "Review the movie 'Inception' (2010) directed by Christopher Nolan",
    anthropic.WithResponseFormat(*responseFormat),
    anthropic.WithSystemMessage("You are a professional movie critic."),
)
```

### 4. Parse JSON Response

```go
var review MovieReview
if err := json.Unmarshal([]byte(response), &review); err != nil {
    // handle error
}
```

## Best Practices Implemented

This implementation follows Claude's documentation best practices for consistency:

1. **Specify Desired Output Format**: Uses precise JSON schema definitions
2. **Prefill Claude's Response**: Starts the assistant message with `{` to enforce JSON output
3. **Constrain with Examples**: Automatically generates examples from the schema
4. **Clear Instructions**: Provides explicit formatting requirements

## Supported Data Types

- **string**: Text fields
- **int/float64**: Numeric values
- **bool**: Boolean values
- **[]string**: Arrays of strings
- **[]int**: Arrays of numbers
- **map[string]interface{}**: Object/dictionary types
- **nested structs**: Complex nested structures

## Error Handling

The example includes comprehensive error handling for:

- Missing API keys
- Network errors
- JSON parsing errors
- Invalid schema definitions

## Using with Agent Framework

You can also use structured output with the agent framework:

```go
agentInstance, err := agent.NewAgent(
    agent.WithLLM(client),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithSystemPrompt("You are a helpful assistant."),
    agent.WithResponseFormat(*responseFormat),
)
```

## Output Examples

The example will generate structured output like:

```json
{
  "title": "Inception",
  "director": "Christopher Nolan",
  "year": 2010,
  "genre": ["Sci-Fi", "Thriller", "Action"],
  "rating": 8.8,
  "summary": "A thief who enters people's dreams...",
  "strengths": ["Complex narrative", "Outstanding cinematography"],
  "weaknesses": ["Can be confusing", "Long runtime"],
  "recommended": true
}
```

## Notes

- The implementation uses response prefilling for better JSON consistency
- Schema validation ensures type safety
- Examples are automatically generated from struct tags
- Works with all Claude models that support the Anthropic API