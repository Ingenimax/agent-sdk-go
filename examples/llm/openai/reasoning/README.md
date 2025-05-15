# OpenAI Reasoning Example

This example demonstrates the use of the reasoning capabilities in the OpenAI client.

## Overview

The reasoning feature allows you to specify how extensively the model should explain its thought process when generating a response. This is implemented by modifying the system message to include specific prompting instructions.

Three modes of reasoning are supported:

- **none**: The model provides a direct, concise answer without explaining its reasoning or showing calculations. This produces the shortest, most to-the-point responses.
- **minimal**: The model briefly explains its thought process along with the answer, showing basic working but keeping explanations concise.
- **comprehensive**: The model provides a detailed step-by-step explanation of its reasoning process with thorough explanations of each step.

## Running the Example

To run this example, set your OpenAI API key and execute the main.go file:

```bash
export OPENAI_API_KEY=your_api_key_here
go run main.go
```

## Example Code

The example demonstrates:

1. Basic usage with different reasoning modes (none, minimal, comprehensive)
2. Combining reasoning with a custom system message
3. Using reasoning with the Chat API

## Example Usage in Your Code

To use reasoning in your own code:

```go
// Using Generate method with reasoning
response, err := client.Generate(
    ctx,
    "Your question here",
    openai.WithReasoning("comprehensive"), // Options: "none", "minimal", "comprehensive"
    openai.WithTemperature(0.2),
)

// Using Chat method with reasoning
messages := []llm.Message{
    {
        Role:    "system",
        Content: "You are a helpful assistant.",
    },
    {
        Role:    "user",
        Content: "Your question here",
    },
}

response, err := client.Chat(
    ctx,
    messages,
    &llm.GenerateParams{
        Temperature: 0.3,
        Reasoning:   "comprehensive", // Options: "none", "minimal", "comprehensive"
    },
)
```
