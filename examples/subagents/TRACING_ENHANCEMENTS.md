# Sub-Agents Tracing and Logging Enhancements

## Overview

We've enhanced the sub-agents system with comprehensive tracing and debug logging to provide full observability into sub-agent invocations.

## What Was Added

### 1. Enhanced AgentTool with Tracing Support

**File**: `pkg/tools/agent_tool.go`

#### New Features:
- **Logger Integration**: Each AgentTool now has a configurable logger
- **Tracer Support**: Full OpenTelemetry/Langfuse span creation and management
- **Debug Logging**: Detailed logs for every sub-agent invocation
- **Performance Tracking**: Duration measurement for all sub-agent calls
- **Error Tracking**: Comprehensive error logging and span events

#### New Methods:
```go
// Configure logging and tracing
agentTool.WithLogger(logger)
agentTool.WithTracer(tracer)
```

### 2. Automatic Tracer Propagation

**File**: `pkg/agent/agent.go`

#### New Features:
- **Auto-configuration**: Parent agent's tracer automatically passed to sub-agent tools
- **Context Integration**: Proper context propagation through tracing.WithAgentName()

#### New Method:
```go
// Internal method to configure sub-agent tools
func (a *Agent) configureSubAgentTools()
```

### 3. Debug Logging in Example

**File**: `examples/subagents/main.go`

- **Debug Level**: Enabled debug logging to see detailed sub-agent traces
- **Enhanced Visibility**: All sub-agent calls now produce detailed logs

## Tracing Information Captured

### OpenTelemetry/Langfuse Spans

Each sub-agent call creates a span named `sub_agent.{AgentName}` with these attributes:

```go
span.SetAttribute("sub_agent.name", agentName)
span.SetAttribute("sub_agent.input", input)  
span.SetAttribute("sub_agent.tool_name", toolName)
span.SetAttribute("sub_agent.response", result)
span.SetAttribute("sub_agent.duration_ms", duration.Milliseconds())
span.SetAttribute("sub_agent.response_length", len(result))
span.SetAttribute("sub_agent.success", true)
```

### Error Tracking
```go
span.AddEvent("error", map[string]interface{}{
    "error": err.Error(),
})
span.SetAttribute("sub_agent.error", err.Error())
```

## Debug Log Messages

### 1. Tool Execution Start
```json
{
  "level": "DEBUG",
  "msg": "Sub-agent tool execution started",
  "sub_agent": "MathAgent",
  "tool_name": "MathAgent_agent", 
  "raw_args": "{\"query\":\"sum 1 + 333\"}"
}
```

### 2. Parameter Parsing
```json
{
  "level": "DEBUG",
  "msg": "Sub-agent tool parameters parsed",
  "sub_agent": "MathAgent",
  "tool_name": "MathAgent_agent",
  "parsed_query": "sum 1 + 333",
  "parsed_context": null
}
```

### 3. Sub-Agent Invocation
```json
{
  "level": "DEBUG", 
  "msg": "Invoking sub-agent",
  "sub_agent": "MathAgent",
  "tool_name": "MathAgent_agent",
  "input_prompt": "sum 1 + 333",
  "recursion_depth": 1,
  "timeout": "30s"
}
```

### 4. Execution Completion
```json
{
  "level": "DEBUG",
  "msg": "Sub-agent execution completed", 
  "sub_agent": "MathAgent",
  "tool_name": "MathAgent_agent",
  "input_prompt": "sum 1 + 333",
  "response": "334",
  "duration": "2.5s",
  "response_len": 3
}
```

### 5. Error Logging
```json
{
  "level": "ERROR",
  "msg": "Sub-agent execution failed",
  "sub_agent": "MathAgent", 
  "error": "calculation failed",
  "duration": "1.2s"
}
```

## How to Use

### 1. Enable Debug Logging
```go
logger := logging.New()
debugOption := logging.WithLevel("debug")
debugOption(logger)
```

### 2. Add Tracer to Main Agent
```go
// If you have a tracer (Langfuse, OTEL, etc.)
mainAgent := agent.NewAgent(
    agent.WithTracer(tracer),
    agent.WithAgents(subAgent1, subAgent2),
    // ... other options
)
```

### 3. Run and Observe
When you run queries, you'll now see:

#### Console Output:
```
2025-08-04T12:45:33-03:00 DBG Sub-agent tool execution started sub_agent=MathAgent tool_name=MathAgent_agent raw_args={"query":"sum 1 + 333"}
2025-08-04T12:45:33-03:00 DBG Sub-agent tool parameters parsed sub_agent=MathAgent parsed_query="sum 1 + 333"
2025-08-04T12:45:33-03:00 DBG Invoking sub-agent sub_agent=MathAgent input_prompt="sum 1 + 333" recursion_depth=1 timeout=30s
2025-08-04T12:45:35-03:00 DBG Sub-agent execution completed sub_agent=MathAgent response="334" duration=2.1s response_len=3
```

#### Tracing Systems:
- **Langfuse**: Sub-agent calls appear as nested spans with full context
- **OpenTelemetry**: Distributed traces show the complete call hierarchy
- **Custom Tracers**: All span attributes and events are properly recorded

## Benefits

### 1. **Complete Observability**
- Every sub-agent call is tracked with timing, input, output, and errors
- Full call hierarchy visible in tracing systems

### 2. **Performance Monitoring**
- Duration tracking for each sub-agent invocation
- Timeout and recursion depth monitoring

### 3. **Debugging Support**
- Detailed logs show exactly what's passed to each sub-agent
- Error context helps identify issues quickly

### 4. **Production Ready**
- Proper error handling and graceful degradation
- Non-blocking logging that won't impact performance

## Example Trace Hierarchy

```
MainAgent.Run
├── sub_agent.MathAgent (span)
│   ├── input: "sum 1 + 333"
│   ├── duration: 2.1s
│   ├── response: "334"
│   └── success: true
└── response returned to user
```

## Configuration Options

### AgentTool Configuration
```go
agentTool := tools.NewAgentTool(subAgent)
    .WithTimeout(60 * time.Second)
    .WithLogger(customLogger)
    .WithTracer(customTracer)
```

### Context Tracking
The system automatically tracks:
- Recursion depth (prevents infinite loops)
- Parent-child relationships
- Invocation IDs for unique call identification
- Performance metrics

This comprehensive tracing system provides full visibility into your sub-agent operations for both development and production environments.