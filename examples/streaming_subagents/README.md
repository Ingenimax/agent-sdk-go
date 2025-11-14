# Streaming Sub-Agents Example

This example demonstrates the **streaming sub-agent** feature in the Ingenimax Agent SDK, which allows real-time visibility into sub-agent execution.

## 🎯 Problem Solved

Previously, when a parent agent delegated tasks to sub-agents via `AgentTool`, the sub-agents would execute entirely in the background. Even though they generated thinking steps, tool calls, and other events, these were only returned as a final text blob. This made it impossible to:

- See sub-agent thinking in real-time
- Monitor sub-agent tool usage
- Understand the execution flow
- Provide real-time feedback to users

## ✨ Solution: Streaming Sub-Agents

With streaming sub-agents, all sub-agent events are forwarded to the parent agent's stream in real-time. This includes:

- **Thinking steps** (`<thinking>` tags and reasoning)
- **Tool calls** (what tools the sub-agent is using)
- **Tool results** (the results from tool executions)
- **Content** (the sub-agent's response)
- **Errors** (any failures during execution)

All events are tagged with metadata identifying which sub-agent generated them.

## 🔧 How It Works

### Architecture

```
Parent Agent (streaming)
   ↓
Calls Sub-Agent via AgentTool
   ↓
AgentTool detects:
  ✅ Sub-agent supports streaming (implements StreamingSubAgent)
  ✅ Parent has provided stream event channel in context
   ↓
AgentTool calls RunStream() instead of Run()
   ↓
Sub-agent stream events → Forwarded to parent's stream
   ↓
End user sees real-time sub-agent execution
```

### Key Components

1. **StreamingSubAgent Interface**
   ```go
   type StreamingSubAgent interface {
       SubAgent
       RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error)
   }
   ```

2. **Context-based Event Channel**
   ```go
   // LLM adds event channel to context before tool execution
   toolCtx := tools.WithStreamEventChan(ctx, eventChan)
   result, err := tool.Execute(toolCtx, args)
   ```

3. **Automatic Event Forwarding**
   ```go
   // AgentTool forwards all sub-agent events to parent
   for event := range eventChan {
       // Add sub-agent metadata
       event.Metadata["sub_agent"] = agentName
       event.Metadata["sub_agent_tool"] = toolName

       // Forward to parent
       parentEventChan <- event
   }
   ```

## 📋 Requirements

- OpenAI API key
- Go 1.21 or higher
- Agent SDK with streaming support

## 🚀 Running the Example

```bash
# Set your OpenAI API key
export OPENAI_API_KEY="your-api-key-here"

# Run the example
go run main.go
```

## 📊 Expected Output

You'll see output like this:

```
================================================================================
STREAMING SUB-AGENT EXAMPLE
================================================================================

User Query: Create a brief article about how AI agents work...

Streaming events (showing real-time sub-agent execution):
--------------------------------------------------------------------------------

[COORDINATOR THINKING]
I need to break this down into research and writing tasks. Let me delegate
the research to the research specialist first...

[COORDINATOR TOOL CALL: Research Specialist Agent]
Arguments: {"query": "Research key concepts about how AI agents work"}

[research_specialist activated]

[research_specialist THINKING]
Let me analyze the key components of AI agents:
1. Perception - how they gather information
2. Reasoning - how they make decisions
3. Action - how they execute tasks...

[research_specialist TOOL RESULT: web_search]
Result: Found information about AI agent architectures...

[Switching to writing_specialist]

[writing_specialist THINKING]
Based on the research, I'll structure this article to be accessible...

================================================================================
FINAL RESPONSE
================================================================================
# Understanding AI Agents: A Beginner's Guide

[Article content here...]
================================================================================
```

## 🎓 Key Learnings

### 1. Transparency
Users can see exactly what each agent is doing at every step.

### 2. Real-time Feedback
No more waiting for 2+ minutes wondering what's happening.

### 3. Debugging
Easy to identify where issues occur in multi-agent workflows.

### 4. Backwards Compatibility
Falls back gracefully to blocking execution when:
- Sub-agent doesn't support streaming
- Parent doesn't provide stream context
- Used in non-streaming scenarios

## 🔍 Implementation Details

### Creating a Streaming Sub-Agent

All agents created with `agent.New()` automatically support streaming:

```go
subAgent := agent.New(
    llm,
    agent.WithName("specialist"),
    agent.WithSystemPrompt("You are a specialist..."),
    agent.WithLLMConfig(llmConfig),
    agent.WithStreamConfig(streamConfig),  // Enable streaming features
)
```

### Wrapping as a Tool

```go
subAgentTool := tools.NewAgentTool(subAgent)
```

The `AgentTool` automatically:
- Detects if the agent supports streaming
- Checks for stream context from parent
- Uses streaming when both conditions are met
- Falls back to blocking execution otherwise

### Using in Parent Agent

```go
parentAgent := agent.New(
    llm,
    agent.WithName("coordinator"),
    agent.WithTools(subAgentTool),
    agent.WithStreamConfig(streamConfig),
)

// Execute with streaming
eventChan, err := parentAgent.RunStream(ctx, userQuery)
```

### Processing Events

```go
for event := range eventChan {
    // Check if event is from a sub-agent
    if subAgent, ok := event.Metadata["sub_agent"].(string); ok {
        fmt.Printf("[%s] ", subAgent)
    }

    switch event.Type {
    case interfaces.AgentEventThinking:
        fmt.Println("Thinking:", event.ThinkingStep)
    case interfaces.AgentEventToolCall:
        fmt.Println("Tool:", event.ToolCall.Name)
    case interfaces.AgentEventContent:
        fmt.Print(event.Content)
    }
}
```

## 🛠️ Customization

### Configuring Stream Behavior

```go
streamConfig := &interfaces.StreamConfig{
    BufferSize:                  100,   // Event buffer size
    IncludeThinking:             true,  // Include <thinking> tags
    IncludeToolProgress:         true,  // Include tool call events
    IncludeIntermediateMessages: true,  // Include messages between iterations
}
```

### Enabling Extended Thinking

```go
llmConfig := &interfaces.LLMConfig{
    Temperature:            0.7,
    MaxOutputTokens:        2000,
    ReasoningEffort:        interfaces.ReasoningEffortHigh,
    EnableExtendedThinking: true,  // Enable detailed reasoning
}
```

## 🔄 Fallback Behavior

The system gracefully handles cases where streaming isn't available:

| Scenario | Behavior |
|----------|----------|
| Sub-agent supports streaming + Parent has stream context | ✅ Uses streaming |
| Sub-agent doesn't support streaming | ⚠️ Falls back to blocking Run() |
| No stream context from parent | ⚠️ Falls back to blocking Run() |
| Non-streaming LLM | ⚠️ Falls back to blocking execution |

Fallback reasons are logged for debugging:
```
DBG Using non-streaming execution for sub-agent
    fallback_reason="no stream context from parent"
```

## 📚 Related Examples

- **[Basic Sub-Agents](../subagents/)** - Using sub-agents without streaming
- **[Multi-Agent Systems](../multiagent/)** - Complex agent hierarchies
- **[Tool Execution](../tools/)** - Creating custom tools

## 🤝 Contributing

Found an issue or have a suggestion? Please open an issue or submit a pull request!

## 📄 License

This example is part of the Ingenimax Agent SDK and follows the same license.
