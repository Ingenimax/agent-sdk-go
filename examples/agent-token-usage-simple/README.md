# Agent Token Usage Tracking with OpenAI

This example demonstrates how to use the Agent SDK's new `RunDetailed()` method to track token usage and execution costs with OpenAI's GPT models.

## Features Demonstrated

1. **Backward Compatibility**: Regular `Run()` method continues to work unchanged
2. **Detailed Token Tracking**: New `RunDetailed()` method provides comprehensive usage information
3. **Cost Calculation**: Real-time cost estimation based on current OpenAI pricing
4. **Execution Analytics**: Performance metrics including timing and LLM call counts
5. **Monitoring Insights**: Usage patterns and optimization suggestions

## Prerequisites

1. **OpenAI API Key**: Set your OpenAI API key as an environment variable:
   ```bash
   export OPENAI_API_KEY="your-api-key-here"
   ```

2. **Go Dependencies**: Ensure you have the Agent SDK installed:
   ```bash
   go mod tidy
   ```

## Running the Example

```bash
cd examples/agent-token-usage-simple
go run main.go
```

## What You'll See

### 1. Regular Execution (Backward Compatible)
```
=== Example 1: Regular Run (backward compatible) ===
Response: 2+2 equals 4.
```

### 2. Detailed Execution with Token Tracking
```
=== Example 2: RunDetailed (with comprehensive tracking) ===
Response: Machine learning is a subset of artificial intelligence that enables computers to learn and improve from experience without being explicitly programmed. It uses algorithms to analyze data, identify patterns, and make predictions or decisions. Common applications include recommendation systems, image recognition, and natural language processing.

Agent Information:
  Agent Name: TokenTrackingAgent
  Model Used: gpt-4o-mini

Token Usage:
  Input Tokens: 45
  Output Tokens: 58
  Total Tokens: 103

Cost Estimation (GPT-4o-mini rates):
  Input Cost: $0.000007
  Output Cost: $0.000035
  Total Cost: $0.000042

Execution Analytics:
  LLM Calls Made: 1
  Execution Time: 1247ms
  Tools Used: 0
```

### 3. Multiple Queries with Cost Tracking
The example runs multiple queries and tracks cumulative costs, providing insights like:
```
=== Session Summary ===
Total Tokens Across All Queries: 387
Total Session Cost: $0.000156
```

### 4. Monitoring and Optimization
```
💡 Optimization Suggestions:
  - Consider shorter prompts or more specific queries
  - Use a smaller model for simple tasks
  - Implement response caching for repeated queries
```

## Key Code Patterns

### Basic Detailed Execution
```go
// Create agent
agent, err := agent.NewAgent(
    agent.WithName("TokenTrackingAgent"),
    agent.WithLLM(llm),
    agent.WithSystemPrompt("You are a helpful assistant."),
)

// Get detailed response with token usage
response, err := agent.RunDetailed(ctx, "Your query here")

// Access token usage
if response.Usage != nil {
    fmt.Printf("Tokens used: %d\n", response.Usage.TotalTokens)
    fmt.Printf("Cost: $%.6f\n", calculateCost(response.Usage))
}
```

### Cost Calculation
```go
// GPT-4o-mini pricing (as of 2024)
inputCost := float64(usage.InputTokens) * 0.000150 / 1000
outputCost := float64(usage.OutputTokens) * 0.000600 / 1000
totalCost := inputCost + outputCost
```

### Performance Monitoring
```go
fmt.Printf("Execution time: %dms\n", response.ExecutionSummary.ExecutionTimeMs)
fmt.Printf("LLM calls: %d\n", response.ExecutionSummary.LLMCalls)
```

## Use Cases

### Development & Testing
- Monitor token usage during prompt engineering
- Compare costs across different models and configurations
- Optimize queries for better cost efficiency

### Production Monitoring
- Track usage costs in real-time
- Set up cost alerts and budgets
- Monitor performance across different user queries

### Cost Management
- Attribute costs to specific users or features
- Implement usage-based billing
- Optimize model selection based on query complexity

## Integration with Monitoring Systems

The detailed response data can be easily integrated with logging and monitoring platforms:

```go
// Example: Log structured metrics
logger.Info("agent_execution",
    "agent_name", response.AgentName,
    "model", response.Model,
    "total_tokens", response.Usage.TotalTokens,
    "cost", calculateCost(response.Usage),
    "execution_time_ms", response.ExecutionSummary.ExecutionTimeMs,
)
```

## Notes

- **Zero Breaking Changes**: Existing `Run()` method works unchanged
- **Backward Compatibility**: All existing code continues to work
- **Opt-in Tracking**: Use `RunDetailed()` only when you need usage information
- **Real-time Costs**: Pricing in examples reflects current OpenAI rates (update as needed)
- **Model Support**: Works with all OpenAI models that support the SDK

## Next Steps

1. **Integrate into your application** using the `RunDetailed()` method
2. **Set up monitoring** using the execution analytics
3. **Implement cost controls** based on usage patterns
4. **Optimize prompts** using the token usage insights