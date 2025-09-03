# Extended Thinking (Reasoning Tokens) Guide

## Overview

Extended Thinking (also known as Reasoning Tokens) is a feature that exposes the LLM's internal reasoning process through streaming events. This provides transparency into how the AI approaches problems, making it invaluable for debugging, education, and understanding AI decision-making.

## Anthropic Extended Thinking

### What is Extended Thinking?

Anthropic's Extended Thinking allows Claude to show its reasoning process before providing final answers. The model thinks through problems step-by-step, and you can see this thinking in real-time through streaming.

### Enabling Extended Thinking

```go
agent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithSystemPrompt(`Your system prompt here`),
    agent.WithLLMConfig(interfaces.LLMConfig{
        EnableReasoning: true,
        ReasoningBudget: 1024, // Minimum required by Anthropic
    }),
)
```

### Configuration Requirements

**Critical Requirements:**
- Temperature must be set to 1.0 when thinking is enabled (automatically handled)
- Budget tokens must be at least 1024 tokens
- Only supported models can use thinking tokens (Claude 3.5 Sonnet, etc.)

### Implementation Details

#### Core SSE Parsing
The Anthropic SSE parser handles thinking tokens in the `delta.thinking` field:

```go
type ContentBlockDeltaData struct {
    Type  string `json:"type"`
    Index int    `json:"index"`
    Delta struct {
        Type     string `json:"type"`
        Text     string `json:"text,omitempty"`
        Thinking string `json:"thinking,omitempty"` // Thinking content here
    } `json:"delta"`
}
```

#### Streaming Configuration
```go
if params.LLMConfig != nil && params.LLMConfig.EnableReasoning {
    if SupportsThinking(c.Model) {
        req.Thinking = &ReasoningSpec{Type: "enabled"}
        req.Temperature = 1.0 // Required by Anthropic

        if params.LLMConfig.ReasoningBudget > 0 {
            req.Thinking.BudgetTokens = params.LLMConfig.ReasoningBudget
        } else {
            req.Thinking.BudgetTokens = 1024 // Default minimum
        }
    }
}
```

### Streaming Visualization

The streaming handler automatically processes thinking events:

```go
case interfaces.AgentEventThinking:
    if !inThinkingMode {
        inThinkingMode = true
        thinkingBlockCount++
        fmt.Printf("\n💭 THINKING BLOCK #%d\n", thinkingBlockCount)
        fmt.Printf("%s\n", strings.Repeat("─", 60))
    }
    fmt.Printf("%s", event.ThinkingStep) // Gray text
```

### Example Output

```
💭 THINKING BLOCK #1
────────────────────────────────────────────────────────────
This is a comprehensive e-commerce market analysis project...
Let me plan the approach:
1. First, I'll use web search to gather current e-commerce market trends
2. Delegate to Research Assistant to organize this information
3. Have Data Analyst review the data and suggest analytical approaches
4. Calculate growth projections using available data
5. Synthesize findings into actionable recommendations

I should start by searching for current market data...
────────────────────────────────────────────────────────────

🔧 TOOL EXECUTION: web_search
├─ Arguments: {"query": "e-commerce market trends 2024 statistics"}
├─ Status: executing
└─ ⏳ Executing...
```

## Prompt Engineering for Action Execution

### The Action Problem

Initially, agents would create thinking blocks with good plans but wouldn't execute the planned actions (tools/calculations). This required specific prompt engineering.

### Solution: Action-Oriented Prompts

**System Prompt Enhancement:**
```go
agent.WithSystemPrompt(`You are a Project Manager coordinating a market analysis project.

EXECUTION PROTOCOL:
1. Think through the approach systematically and show your reasoning process
2. IMMEDIATELY start executing your plan - use tools and gather data right away
3. Use web search tool FIRST to get current market data and trends
4. Use calculator tool for all numerical analysis and projections
5. Act as both Research Assistant and Data Analyst in your analysis
6. Synthesize findings into comprehensive reports with concrete numbers
7. Provide specific, actionable recommendations

IMPORTANT: After thinking, you MUST take action! Start by using the web search tool to gather current e-commerce market data. Don't just plan - execute your plan immediately.`)
```

**Task Prompt Enhancement:**
```go
eventChan, err := streamingAgent.RunStream(ctx, `Execute a comprehensive e-commerce market analysis for Q1 2025 business planning:

IMMEDIATE ACTION PLAN:
1. START NOW: Use web search tool to find current e-commerce market data and trends
2. ANALYZE: Calculate growth rates and market size using calculator tool
3. PROJECT: Use calculator to model Q1 2025 market projections
4. SYNTHESIZE: Combine all findings into a comprehensive business report
5. RECOMMEND: Provide specific actions with supporting calculations

Begin immediately by searching for current e-commerce market trends and statistics. Don't just plan - execute each step and use your tools actively throughout the analysis.`)
```

**Key Strategies:**
- Use action verbs: "Execute", "START NOW", "Begin immediately"
- Be explicit about tool usage: "Use web search tool FIRST"
- Create urgency: "IMMEDIATELY start executing your plan"
- Provide specific instructions: "Don't just plan - execute each step"

## Troubleshooting Extended Thinking

### Common Issues and Solutions

#### 1. Empty Thinking Blocks
- **Symptom**: Thinking block detected but no content displayed
- **Cause**: Parser looking in wrong field (`delta.text` instead of `delta.thinking`)
- **Solution**: Use `blockDelta.Delta.Thinking` for thinking content

#### 2. HTTP 400 Temperature Error
- **Symptom**: "thinking.enabled requires temperature=1"
- **Cause**: Using temperature other than 1.0 with thinking enabled
- **Solution**: SDK automatically overrides temperature to 1.0 when thinking is enabled

#### 3. Budget Tokens Required Error
- **Symptom**: "thinking.enabled.budget_tokens: Field required"
- **Cause**: Not providing budget_tokens or using value less than 1024
- **Solution**: Set minimum 1024 tokens in ReasoningBudget

#### 4. Thinking But No Action
- **Symptom**: Agent thinks but doesn't use tools or take actions
- **Cause**: Prompt encourages planning but not execution
- **Solution**: Use action-oriented prompts with explicit execution commands

## Advanced Example

```go
// Create agent with Extended Thinking
streamingAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithTools(webSearchTool, calculatorTool),
    agent.WithSystemPrompt(`You are an AI assistant that thinks through problems systematically.

EXECUTION PROTOCOL:
1. Think through the approach systematically
2. IMMEDIATELY execute your plan using available tools
3. Show your reasoning process throughout
4. Provide concrete, actionable conclusions`),
    agent.WithLLMConfig(interfaces.LLMConfig{
        EnableReasoning: true,
        ReasoningBudget: 2048, // More budget for complex reasoning
    }),
)

// Stream with thinking enabled
events, err := streamingAgent.RunStream(ctx, `Analyze the current state of renewable energy adoption globally. Use tools to gather data and calculate trends.`)

// Process events with thinking visualization
for event := range events {
    switch event.Type {
    case interfaces.AgentEventThinking:
        // Gray thinking text
        fmt.Printf("%s%s%s", ColorGray, event.ThinkingStep, ColorReset)
    case interfaces.AgentEventContent:
        // White response text
        fmt.Printf("%s%s%s", ColorWhite, event.Content, ColorReset)
    case interfaces.AgentEventToolCall:
        fmt.Printf("\n🔧 TOOL: %s\n", event.ToolCall.Name)
    }
}
```

## Performance Considerations

- **Buffer Size**: Set to 500+ for complex operations with thinking
- **Token Budget**: Start with 1024, increase for complex reasoning (up to 4000)
- **Memory Usage**: Thinking content can be substantial, monitor memory usage
- **Latency**: First thinking tokens arrive quickly, full reasoning takes more time

## Best Practices

1. **Enable Thinking Selectively**: Only use for complex tasks that benefit from transparency
2. **Size Buffers Appropriately**: Larger buffers for thinking-heavy operations
3. **Use Action-Oriented Prompts**: Combine thinking with explicit execution instructions
4. **Monitor Token Usage**: Thinking tokens count toward total usage
5. **Handle Interruptions**: Allow users to interrupt long thinking processes
6. **Test with Real Tasks**: Verify thinking leads to better outcomes for your use case

## Model Support

### Supported Models
- Claude 3.5 Sonnet (claude-3-5-sonnet-20241022)
- Claude 3.5 Haiku (claude-3-5-haiku-20241022)

### Feature Availability
```go
func SupportsThinking(model string) bool {
    thinkingModels := []string{
        "claude-3-5-sonnet-20241022",
        "claude-3-5-haiku-20241022",
    }

    for _, supportedModel := range thinkingModels {
        if model == supportedModel {
            return true
        }
    }
    return false
}
```

## Related Documentation

- [Streaming Overview](./streaming-overview.md) - General streaming concepts
- [Reasoning Models](./reasoning-models.md) - OpenAI reasoning model support
- [Microservice Streaming](./microservice-streaming.md) - Simplified APIs
