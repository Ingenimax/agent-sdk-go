# Anthropic LLM Example

This example demonstrates how to use the Anthropic Claude API with the agent-sdk-go library.

## Prerequisites

- Go 1.20 or higher
- An Anthropic API key

## Setup

1. Set your Anthropic API key as an environment variable:
```bash
export ANTHROPIC_API_KEY=your-api-key
```

2. Optionally, set additional configuration variables:
```bash
export ANTHROPIC_MODEL=claude-3-7-sonnet-latest  # Default model
export ANTHROPIC_TEMPERATURE=0.7                 # Default temperature
export ANTHROPIC_TIMEOUT=60                      # Default timeout in seconds
export ANTHROPIC_BASE_URL=https://api.anthropic.com  # Default API endpoint
```

3. Run the example:
```bash
go run main.go
```

## Features Demonstrated

This example showcases:

1. **Basic Generation** - Simple text generation with Claude
2. **System Messages** - Using Claude 3.7 Sonnet with system messages for creative writing
3. **Step-by-Step Reasoning** - Generating solutions with step-by-step explanations
4. **Tool Usage** - Integrating Claude with tools like a calculator
5. **Agent Creation** - Building a complete AI agent with memory and specialized capabilities

## Supported Claude Models

The example supports the latest Claude models:

- `Claude35Haiku` - Fast and cost-effective (claude-3-5-haiku-latest)
- `Claude35Sonnet` - Balanced performance (claude-3-5-sonnet-latest)
- `Claude3Opus` - Highest capabilities (claude-3-opus-latest)
- `Claude37Sonnet` - Latest model with improved capabilities (claude-3-7-sonnet-latest)

## Code Structure

The main.go file contains five key functions:

- `basicGeneration`: Shows basic text generation with any Claude model
- `claudeSonnetGeneration`: Demonstrates using Claude-3.7-Sonnet with system messages
- `reasoningGeneration`: Shows how to get detailed step-by-step explanations
- `toolUsage`: Shows how to integrate tools like a calculator with Claude
- `createAgent`: Creates a complete agent with memory, system prompt, and organization context

## Important Notes

- Models must be explicitly specified when creating a client
- Tool usage requires proper context with organization ID
- Agent creation requires both organization ID and conversation ID
- The reasoning parameter is maintained for backward compatibility but not officially supported by the current API

For more detailed implementation, please refer to the `main.go` file. 