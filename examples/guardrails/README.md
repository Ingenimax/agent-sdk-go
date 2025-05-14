# Guardrails Example

This example demonstrates how to implement guardrails for LLMs and tools in the Agent SDK.

## Features

- Content filtering to block or redact harmful content
- Token limiting to control response length
- PII (Personally Identifiable Information) filtering
- Tool usage restrictions
- Rate limiting to prevent excessive API calls

## Usage

### Prerequisites

- Set the `OPENAI_API_KEY` environment variable with your OpenAI API key
- For web search tool functionality, set `GOOGLE_API_KEY` and `GOOGLE_SEARCH_ENGINE_ID`

```bash
export OPENAI_API_KEY=your_openai_api_key
export GOOGLE_API_KEY=your_google_api_key
export GOOGLE_SEARCH_ENGINE_ID=your_search_engine_id
```

### Running the Example

```bash
go run main.go
```

## Code Explanation

### Creating Guardrails

```go
// Content filter
contentFilter := guardrails.NewContentFilter(
    []string{"hate", "violence", "profanity", "sexual"},
    guardrails.RedactAction,
)

// Token limit
tokenLimit := guardrails.NewTokenLimit(
    100,
    nil, // Use simple token counter
    guardrails.RedactAction,
    "end",
)

// PII filter
piiFilter := guardrails.NewPiiFilter(
    guardrails.RedactAction,
)

// Tool restriction
toolRestriction := guardrails.NewToolRestriction(
    []string{"web_search", "calculator"},
    guardrails.BlockAction,
)

// Rate limit
rateLimit := guardrails.NewRateLimit(
    10, // 10 requests per minute
    guardrails.BlockAction,
)
```

### Creating a Guardrails Pipeline

```go
pipeline := guardrails.NewPipeline(
    []guardrails.Guardrail{
        contentFilter,
        tokenLimit,
        piiFilter,
        toolRestriction,
        rateLimit,
    },
    logger,
)
```

### Applying Guardrails to LLM

```go
openaiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
llmWithGuardrails := guardrails.NewLLMMiddleware(openaiClient, pipeline)

// Use the guarded LLM
response, err := llmWithGuardrails.Generate(ctx, prompt, nil)
```

### Applying Guardrails to Tools

```go
tool := websearch.New(
    os.Getenv("GOOGLE_API_KEY"),
    os.Getenv("GOOGLE_SEARCH_ENGINE_ID"),
)
toolWithGuardrails := guardrails.NewToolMiddleware(tool, pipeline)

// Use the guarded tool
output, err := toolWithGuardrails.Run(ctx, input)
```

## Available Actions

When a guardrail is triggered, it can take one of these actions:

- `RedactAction` - Redacts or modifies the content to remove problematic parts
- `BlockAction` - Blocks the request entirely and returns an error
- `LogAction` - Allows the request but logs the violation

## Customization

You can create custom guardrails by implementing the `Guardrail` interface:

```go
type Guardrail interface {
    ProcessLLMRequest(ctx context.Context, request *LLMRequest) (*LLMRequest, error)
    ProcessLLMResponse(ctx context.Context, response *LLMResponse) (*LLMResponse, error)
    ProcessToolRequest(ctx context.Context, request *ToolRequest) (*ToolRequest, error)
    ProcessToolResponse(ctx context.Context, response *ToolResponse) (*ToolResponse, error)
}
```
