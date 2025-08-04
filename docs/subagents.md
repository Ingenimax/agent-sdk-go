# Sub-Agents Feature Documentation

## Overview

The sub-agents feature allows you to compose complex AI systems by combining specialized agents into a hierarchical structure. Each sub-agent can focus on a specific domain or task, while a main orchestrator agent delegates work appropriately.

## Key Concepts

### Agent Composition
Agents can now have other agents as sub-agents, which are automatically wrapped as tools that can be invoked by the parent agent's LLM.

### Automatic Tool Wrapping
When you add sub-agents using `WithAgents()`, they are automatically converted into tools that the parent agent can use. The tool name follows the pattern `{AgentName}_agent`.

### Description Importance
The `WithDescription()` option is crucial for sub-agents. It provides the context the parent agent's LLM needs to understand when to delegate tasks to each sub-agent.

## Usage

### Basic Setup

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

// Create specialized sub-agents
mathAgent, _ := agent.NewAgent(
    agent.WithName("MathAgent"),
    agent.WithDescription("Handles complex mathematical calculations"),
    agent.WithLLM(llm),
    agent.WithSystemPrompt("You are a mathematics expert..."),
)

researchAgent, _ := agent.NewAgent(
    agent.WithName("ResearchAgent"),
    agent.WithDescription("Performs research and information retrieval"),
    agent.WithLLM(llm),
    agent.WithSystemPrompt("You are a research specialist..."),
)

// Create main agent with sub-agents
mainAgent, _ := agent.NewAgent(
    agent.WithName("MainAgent"),
    agent.WithLLM(llm),
    agent.WithAgents(mathAgent, researchAgent),
    agent.WithSystemPrompt("You orchestrate tasks using specialized agents..."),
)
```

### Advanced Configuration

```go
// Configure with specific tools for sub-agents
mathAgent, _ := agent.NewAgent(
    agent.WithName("MathAgent"),
    agent.WithDescription("Mathematical computations expert"),
    agent.WithLLM(llm),
    agent.WithTools(calculator.New()),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithSystemPrompt("..."),
)

// Set maximum iterations for tool calls (sub-agent invocations)
mainAgent, _ := agent.NewAgent(
    agent.WithName("MainAgent"),
    agent.WithLLM(llm),
    agent.WithAgents(mathAgent, researchAgent),
    agent.WithMaxIterations(3), // Allow up to 3 sub-agent calls
)
```

## Architecture

### Component Structure

```
MainAgent
├── LLM (Decision Maker)
├── Memory
├── Regular Tools
└── Sub-Agent Tools (Auto-generated)
    ├── MathAgent_agent (Tool Wrapper)
    │   └── MathAgent (Actual Agent)
    └── ResearchAgent_agent (Tool Wrapper)
        └── ResearchAgent (Actual Agent)
```

### Tool Wrapper (AgentTool)

The `AgentTool` wrapper (`pkg/tools/agent_tool.go`) implements the `interfaces.Tool` interface and:
- Manages context propagation
- Handles timeouts (default: 30 seconds)
- Tracks recursion depth
- Provides error handling

### Context Management

Sub-agent invocations include:
- Parent agent identification
- Recursion depth tracking
- Invocation ID for tracing
- Timeout management

## Safety Features

### Circular Dependency Detection

The system automatically detects and prevents circular dependencies during agent initialization:

```go
// This will fail with an error
agent1 := NewAgent(WithName("A"), WithLLM(llm))
agent2 := NewAgent(WithName("B"), WithLLM(llm), WithAgents(agent1))
agent1.subAgents = append(agent1.subAgents, agent2) // Circular!
```

### Recursion Depth Limits

Maximum recursion depth is enforced (default: 5 levels) to prevent infinite loops:

```go
// Context tracking prevents deep recursion
ctx = WithSubAgentContext(ctx, "Parent", "Child")
if err := ValidateRecursionDepth(ctx); err != nil {
    // Maximum depth exceeded
}
```

### Validation

Agent trees are validated for:
- Circular dependencies
- Maximum depth violations
- Required components (LLM, name)

## Best Practices

### 1. Clear Descriptions

Always provide clear, specific descriptions for sub-agents:

```go
agent.WithDescription("Specializes in Python code generation, debugging, and optimization")
```

### 2. System Prompts

Include sub-agent information in the main agent's system prompt:

```go
systemPrompt := `You have access to these specialized agents:
- MathAgent: For mathematical calculations
- ResearchAgent: For information retrieval
Delegate tasks when their expertise would help.`
```

### 3. Error Handling

Always handle potential errors from sub-agent calls:

```go
result, err := mainAgent.Run(ctx, query)
if err != nil {
    // Handle sub-agent failures gracefully
}
```

### 4. Memory Management

Each agent maintains its own memory, consider sharing strategies:

```go
// Option 1: Separate memory per agent
mathAgent.WithMemory(memory.NewConversationBuffer())

// Option 2: Shared memory (if needed)
sharedMem := memory.NewConversationBuffer()
agent1.WithMemory(sharedMem)
agent2.WithMemory(sharedMem)
```

## Testing

### Unit Tests

Test sub-agent functionality:

```go
func TestSubAgentDelegation(t *testing.T) {
    // Create mock LLM
    mockLLM := &MockLLM{}
    
    // Create and test sub-agents
    subAgent := NewAgent(
        WithName("SubAgent"),
        WithLLM(mockLLM),
    )
    
    mainAgent := NewAgent(
        WithName("Main"),
        WithLLM(mockLLM),
        WithAgents(subAgent),
    )
    
    // Verify sub-agent is registered
    assert.True(t, mainAgent.HasSubAgent("SubAgent"))
}
```

### Integration Tests

Test with actual LLM calls (requires API key):

```go
// +build integration

func TestSubAgentsIntegration(t *testing.T) {
    llm := openai.NewClient(apiKey)
    // ... create agents and test real delegation
}
```

## Performance Considerations

### Timeouts

Configure timeouts for sub-agent calls:

```go
agentTool := tools.NewAgentTool(subAgent).WithTimeout(60 * time.Second)
```

### Concurrency

Sub-agents are called sequentially by default. For parallel execution, implement custom orchestration logic.

### Caching

Consider implementing response caching for frequently called sub-agents to reduce API calls.

## Debugging

### Tracing

Use the context values for debugging:

```go
subAgentName := GetSubAgentName(ctx)
parentAgent := GetParentAgent(ctx)
depth := GetRecursionDepth(ctx)
invocationID := GetInvocationID(ctx)
```

### Logging

Enable debug logging to trace sub-agent calls:

```go
logger := logging.New()
debugOption := logging.WithLevel("debug")
debugOption(logger)
```

## Migration Guide

### From Orchestration Pattern

If you were using the orchestration pattern with handoffs:

**Before:**
```go
// Manual handoff with special markers
response := "[HANDOFF:math:needs calculation]"
```

**After:**
```go
// Automatic delegation through sub-agents
mainAgent := NewAgent(
    WithAgents(mathAgent),
)
```

### From Multiple Separate Agents

**Before:**
```go
// Manually calling different agents
if needsMath {
    result = mathAgent.Run(ctx, query)
} else {
    result = generalAgent.Run(ctx, query)
}
```

**After:**
```go
// Single entry point with automatic delegation
mainAgent := NewAgent(WithAgents(mathAgent, generalAgent))
result = mainAgent.Run(ctx, query)
```

## Limitations

1. **Sequential Execution**: Sub-agents are called one at a time
2. **Depth Limit**: Maximum 5 levels of nesting by default
3. **Timeout**: 30-second default timeout per sub-agent call
4. **Memory Isolation**: Each agent has separate memory by default

## Future Enhancements

Potential improvements for the sub-agents feature:
- Parallel sub-agent execution
- Dynamic sub-agent discovery
- Cross-agent memory sharing protocols
- Sub-agent result caching
- Priority-based delegation
- Load balancing across sub-agents