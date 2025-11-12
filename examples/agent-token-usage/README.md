# Agent Token Usage Tracking Example

This example demonstrates how to use the Agent SDK's token usage tracking feature to monitor and analyze the cost and performance of your AI agent executions.

## Features Demonstrated

1. **Backward Compatibility**: The regular `Run()` method continues to work unchanged
2. **Detailed Execution**: The new `RunDetailed()` method provides comprehensive usage information
3. **Token Usage Tracking**: Monitor input, output, total, and reasoning tokens
4. **Execution Analytics**: Track LLM calls, tool usage, and execution times
5. **Cost Calculation**: Calculate estimated costs based on token usage

## Key Components

### AgentResponse Structure

```go
type AgentResponse struct {
    Content          string                 // The agent's response
    Usage            *TokenUsage           // Token usage information
    AgentName        string                // Name of the executing agent
    Model            string                // LLM model used
    ExecutionSummary ExecutionSummary      // Execution statistics
    Metadata         map[string]interface{} // Additional metadata
}
```

### TokenUsage Structure

```go
type TokenUsage struct {
    InputTokens     int // Input/prompt tokens
    OutputTokens    int // Generated response tokens
    TotalTokens     int // Total tokens used
    ReasoningTokens int // Reasoning tokens (for supported models)
}
```

### ExecutionSummary Structure

```go
type ExecutionSummary struct {
    LLMCalls        int      // Number of LLM API calls
    ToolCalls       int      // Number of tools used
    SubAgentCalls   int      // Number of sub-agent calls
    ExecutionTimeMs int64    // Total execution time in milliseconds
    UsedTools       []string // Names of tools that were used
    UsedSubAgents   []string // Names of sub-agents that were called
}
```

## Usage Patterns

### 1. Basic Token Tracking

```go
// Create agent
agent, err := agent.NewAgent(
    agent.WithName("MyAgent"),
    agent.WithLLM(llm),
)

// Get detailed response with token usage
response, err := agent.RunDetailed(ctx, "Your query here")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Tokens used: %d\n", response.Usage.TotalTokens)
fmt.Printf("Execution time: %dms\n", response.ExecutionSummary.ExecutionTimeMs)
```

### 2. Cost Monitoring

```go
// Calculate cost based on provider pricing
costPerInputToken := 0.00003  // $0.03 per 1K tokens
costPerOutputToken := 0.00006 // $0.06 per 1K tokens

inputCost := float64(response.Usage.InputTokens) * costPerInputToken / 1000
outputCost := float64(response.Usage.OutputTokens) * costPerOutputToken / 1000
totalCost := inputCost + outputCost

fmt.Printf("Estimated cost: $%.6f\n", totalCost)
```

### 3. Performance Analysis

```go
// Track execution metrics
fmt.Printf("Performance Metrics:\n")
fmt.Printf("  LLM API Calls: %d\n", response.ExecutionSummary.LLMCalls)
fmt.Printf("  Execution Time: %dms\n", response.ExecutionSummary.ExecutionTimeMs)
fmt.Printf("  Tokens per Call: %.2f\n",
    float64(response.Usage.TotalTokens) / float64(response.ExecutionSummary.LLMCalls))
```

## Use Cases

### Development & Testing
- Monitor token usage during development to optimize prompts
- Track performance changes across different agent configurations
- Debug expensive operations by analyzing execution summaries

### Production Monitoring
- Implement cost alerts based on token usage thresholds
- Track usage patterns across different user queries
- Optimize agent performance based on execution metrics

### Multi-Tenant Applications
- Attribute costs to specific users or organizations
- Implement usage-based billing
- Monitor resource consumption per tenant

## Running the Example

1. Set your OpenAI API key:
   ```bash
   export OPENAI_API_KEY="your-api-key-here"
   ```

2. Run the example:
   ```bash
   go run main.go
   ```

The example will demonstrate:
- Regular agent execution (backward compatible)
- Detailed execution with full token tracking
- Cost calculation based on token usage
- Comparison of multiple executions

## Integration with Monitoring Systems

The detailed response data can be easily integrated with monitoring and analytics platforms:

```go
// Example: Send metrics to your monitoring system
response, err := agent.RunDetailed(ctx, query)
if err != nil {
    return err
}

// Log structured metrics
logger.Info("agent_execution",
    "agent_name", response.AgentName,
    "model", response.Model,
    "input_tokens", response.Usage.InputTokens,
    "output_tokens", response.Usage.OutputTokens,
    "total_tokens", response.Usage.TotalTokens,
    "execution_time_ms", response.ExecutionSummary.ExecutionTimeMs,
    "llm_calls", response.ExecutionSummary.LLMCalls,
)
```

## Notes

- Token usage tracking is only available when using `RunDetailed()`
- The regular `Run()` method maintains full backward compatibility
- Token usage information depends on the LLM provider's support for detailed responses
- All execution time and token metrics are captured automatically without any performance overhead when using the regular `Run()` method