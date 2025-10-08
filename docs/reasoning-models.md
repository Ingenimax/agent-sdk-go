# OpenAI Reasoning Models Guide

## Overview

OpenAI's reasoning models (o1, o3, o4, GPT-5 series) have special capabilities and parameter restrictions. The Agent SDK automatically handles these restrictions to prevent common configuration errors.

## Supported Reasoning Models

The SDK automatically detects these reasoning models:

- **o1 series**: `o1-mini`, `o1-preview`
- **o3 series**: `o3-`, `o3-mini`
- **o4 series**: `o4-`, `o4-mini` ✅
- **GPT-5 series**: `gpt-5`, `gpt-5-mini`, `gpt-5-nano`

## Automatic Parameter Handling

### Temperature Auto-Configuration

Reasoning models only support temperature=1 (the default). The SDK automatically omits the temperature parameter for these models:

```go
// SDK automatically handles this
if !isReasoningModel(c.Model) {
    streamParams.Temperature = openai.Float(params.LLMConfig.Temperature)
}
// For reasoning models, temperature parameter is omitted entirely
```

**Before (manual configuration):**
```bash
# This would cause an error:
400 Bad Request: "Unsupported value: 'temperature' does not support 0.7 with this model. Only the default (1) value is supported."
```

**After (automatic handling):**
```go
// Works automatically - temperature is ignored for reasoning models
client := openai.NewClient(apiKey, openai.WithModel("o4-mini"))
agent, err := agent.NewAgent(
    agent.WithLLM(client),
    agent.WithLLMConfig(interfaces.LLMConfig{
        Temperature: 0.7, // Automatically ignored for o4-mini
    }),
)
```

### Reasoning Effort Control

Reasoning models support the `reasoning_effort` parameter to control computational effort. Valid values: `"minimal"`, `"low"`, `"medium"`, `"high"`.

```go
// SDK automatically sets reasoning_effort for reasoning models
if isReasoningModel(c.Model) && params.LLMConfig.Reasoning != "" {
    req.ReasoningEffort = shared.ReasoningEffort(params.LLMConfig.Reasoning)
}
```

**Example:**
```go
agent, err := agent.NewAgent(
    agent.WithLLM(client),
    agent.WithLLMConfig(interfaces.LLMConfig{
        Reasoning: "low", // minimal, low, medium, or high
    }),
)
```

### Parallel Tool Calls Auto-Configuration

Reasoning models don't support parallel tool calls. The SDK automatically omits this parameter:

```go
// Only set ParallelToolCalls for non-reasoning models
if !isReasoningModel(c.Model) {
    req.ParallelToolCalls = openai.Bool(true)
}
// For reasoning models, parallel_tool_calls parameter is omitted entirely
```

**Before (manual configuration):**
```bash
# This would cause an error:
400 Bad Request: "Unsupported parameter: 'parallel_tool_calls' is not supported with this model."
```

**After (automatic handling):**
```go
// Works automatically - parallel tool calls omitted for reasoning models
events, err := agent.RunStream(ctx, "Complex reasoning task")
// SDK handles parameter restrictions transparently
```

### System Message Restrictions

Reasoning models have specific system message restrictions that are automatically handled:

```go
// Reasoning models don't support system messages in some configurations
if params.SystemMessage != "" && !isReasoningModel(c.Model) {
    messages = append([]openai.ChatCompletionMessageParamUnion{
        openai.SystemMessage(params.SystemMessage),
    }, messages...)
}
```

## Usage Examples

### Basic Reasoning Model Usage

```go
// Create OpenAI client with reasoning model
client := openai.NewClient(
    apiKey,
    openai.WithModel("o4-mini"), // Reasoning model
)

// Create agent - SDK handles all parameter restrictions
agent, err := agent.NewAgent(
    agent.WithLLM(client),
    agent.WithLLMConfig(interfaces.LLMConfig{
        Temperature:     0.7,  // Ignored for reasoning models
        EnableReasoning: true, // Enable reasoning token support
        ReasoningBudget: 1024, // Budget for reasoning tokens (when supported)
    }),
    agent.WithSystemPrompt("You are a helpful assistant."),
)

// Stream works without parameter errors
events, err := agent.RunStream(ctx, "Solve this complex problem step by step")
```

### Reasoning with Tools

```go
// Reasoning models with tools (no parallel tool calls)
agent, err := agent.NewAgent(
    agent.WithLLM(client),
    agent.WithTools(calculatorTool, webSearchTool),
    agent.WithLLMConfig(interfaces.LLMConfig{
        EnableReasoning: true,
    }),
)

// Tools execute sequentially (no parallel execution for reasoning models)
events, err := agent.RunStream(ctx, "Research and calculate market projections")
```

## Model Capabilities

### o1 Series (o1-mini, o1-preview)
- **Reasoning**: Built-in reasoning capabilities
- **Tools**: Limited tool support
- **Temperature**: Fixed at 1.0
- **System Messages**: Not supported
- **Streaming**: Delta streaming with usage information

### o4 Series (o4-mini)
- **Reasoning**: Advanced reasoning capabilities
- **Tools**: Full tool support (sequential only)
- **Temperature**: Fixed at 1.0
- **System Messages**: Limited support
- **Streaming**: Full streaming support with tool execution

### Future Models (o3, GPT-5)
The SDK is prepared for future reasoning models with the same parameter restrictions.

## Implementation Details

### Reasoning Model Detection

```go
func isReasoningModel(model string) bool {
    reasoningModels := []string{
        "o1-", "o1-mini", "o1-preview",
        "o3-", "o3-mini",
        "o4-", "o4-mini",
        "gpt-5", "gpt-5-mini", "gpt-5-nano",
    }

    for _, prefix := range reasoningModels {
        if strings.HasPrefix(model, prefix) {
            return true
        }
    }
    return false
}
```

### Automatic Configuration Flow

1. **Model Detection**: `isReasoningModel()` checks if model is a reasoning model
2. **Parameter Omission**: Temperature and parallel_tool_calls omitted for reasoning models
3. **System Message Handling**: System messages handled appropriately for model type
4. **Tool Configuration**: Sequential tool execution for reasoning models
5. **Streaming Setup**: Appropriate streaming configuration for model capabilities
6. **Reasoning Effort**: `reasoning_effort` parameter set for reasoning models (minimal/low/medium/high)

## Error Prevention

The SDK prevents these common errors automatically:

### Temperature Errors
```bash
# Prevented automatically:
POST "https://api.openai.com/v1/chat/completions": 400 Bad Request {
    "message": "Unsupported value: 'temperature' does not support 0.7 with this model. Only the default (1) value is supported.",
    "type": "invalid_request_error",
    "param": "temperature",
    "code": "unsupported_value"
}
```

### Parallel Tool Calls Errors
```bash
# Prevented automatically:
POST "https://api.openai.com/v1/chat/completions": 400 Bad Request {
    "message": "Unsupported parameter: 'parallel_tool_calls' is not supported with this model.",
    "type": "invalid_request_error",
    "param": "parallel_tool_calls",
    "code": "unsupported_parameter"
}
```

## Debugging Reasoning Models

### Logging Configuration Overrides

```go
2025-08-22T16:28:28-03:00 DBG Overriding temperature for reasoning model
    forced_temperature=1
    model=o4-mini
    reason="reasoning models only support temperature = 1"
    requested_temperature=0.3
```

### Streaming Debug Information

```go
2025-08-22T16:28:28-03:00 DBG Creating OpenAI streaming request
    is_reasoning_model=true
    model=o4-mini
    temperature=1
    parallel_tools=false
    reasoning_effort=low
```

## Best Practices

1. **Let SDK Handle Parameters**: Don't try to manually configure reasoning model parameters
2. **Control Reasoning Effort**: Use `Reasoning: "low"` or `"medium"` to balance speed vs. quality
3. **Sequential Tools**: Design tool workflows for sequential execution with reasoning models
4. **Test Thoroughly**: Reasoning models may behave differently than standard models
5. **Monitor Usage**: Higher reasoning effort will increase token usage and latency

## Future Reasoning Token Support

OpenAI is working on exposing reasoning tokens for streaming. When available, the SDK will support:

```go
// Future reasoning token streaming (when supported by OpenAI)
case interfaces.AgentEventThinking:
    fmt.Printf("[Reasoning] %s\n", event.ThinkingStep)
```

The SDK is prepared to handle reasoning tokens when OpenAI's Go SDK supports them.

## Migration from Standard Models

Switching to reasoning models is transparent:

```go
// Before: Standard model
client := openai.NewClient(apiKey, openai.WithModel("gpt-4-turbo"))

// After: Reasoning model (no code changes needed)
client := openai.NewClient(apiKey, openai.WithModel("o4-mini"))

// Same agent configuration works for both
agent, err := agent.NewAgent(
    agent.WithLLM(client),
    agent.WithLLMConfig(interfaces.LLMConfig{
        Temperature: 0.7, // Handled appropriately for each model type
    }),
)
```

## Related Documentation

- [Streaming Overview](./streaming-overview.md) - General streaming concepts
- [Extended Thinking](./extended-thinking.md) - Anthropic reasoning visibility
- [Microservice Streaming](./microservice-streaming.md) - Simplified APIs
