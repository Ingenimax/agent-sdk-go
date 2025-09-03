# Streaming Examples

This directory contains comprehensive examples demonstrating the streaming capabilities of the Agent SDK.

## Overview

The Agent SDK supports real-time streaming from multiple LLM providers:
- **Anthropic Claude**: Full SSE support with thinking events
- **OpenAI GPT**: Delta streaming with structured outputs and o1 reasoning

## Examples Included

### 1. Basic LLM Streaming
Demonstrates direct streaming from LLM providers with real-time token delivery.

### 2. Agent Streaming
Shows how agents can stream responses while maintaining context and memory.

### 3. Streaming with Tools
Illustrates tool execution during streaming, with progress events for tool calls.

### 4. Advanced Streaming Features
Showcases advanced features like custom buffer sizes, thinking events, and metrics.

## Prerequisites

Install dependencies:
```bash
go mod tidy
```

Set up API keys for your chosen provider:

### For Anthropic:
```bash
export ANTHROPIC_API_KEY=your_anthropic_key
export LLM_PROVIDER=anthropic
```

### For OpenAI:
```bash
export OPENAI_API_KEY=your_openai_key
export LLM_PROVIDER=openai
```

## Running the Examples

Run all examples:
```bash
go run main.go
```

## Example Output

```
🚀 Agent SDK Streaming Examples
===============================

Using provider: anthropic

📡 Example 1: Basic LLM Streaming
--------------------------------
Starting LLM streaming...
Response: Quantum computing is a revolutionary approach to computation that harnesses the principles of quantum mechanics...
[Stream completed]

🤖 Example 2: Agent Streaming
-----------------------------
Starting agent streaming...
Agent Response: Machine learning works through several key steps...
🤔 [Thinking: I should break this down into clear, digestible steps...]
[Agent streaming completed]

🛠️  Example 3: Streaming with Tools
----------------------------------
Starting streaming with tools...
Response: I'll help you calculate compound interest step by step...
🔧 [Tool Call #1: calculator]
   Arguments: {"operation": "power", "a": 1.0125, "b": 40}
   Status: executing
✅ [Tool Result: 1.6436186844245104]
[Streaming with tools completed - 3 tools used]

⚡ Example 4: Advanced Streaming Features
----------------------------------------
Starting advanced streaming with custom configuration...
[Stream started at 14:23:45.123]
Response: Let me think through the implications of quantum entanglement...
🧠 [Thinking #1: Quantum entanglement is a phenomenon where particles become correlated...]
📊 [Usage info: {input_tokens: 45, output_tokens: 312, total_tokens: 357}]
[Stream completed - Duration: 3.2s, Events: 45, Content: 1247 chars, Thinking events: 3]

✅ All streaming examples completed!
```

## Key Features Demonstrated

### Event Types
- **Content Events**: Real-time text streaming
- **Thinking Events**: Reasoning process visibility (Anthropic)
- **Tool Events**: Tool call progress and results
- **Error Events**: Robust error handling
- **Completion Events**: Stream lifecycle management

### Configuration Options
- **Buffer Sizes**: Configurable for performance tuning
- **Thinking Inclusion**: Toggle reasoning visibility
- **Tool Progress**: Control tool execution feedback
- **Memory Integration**: Context preservation across streams

### Advanced Capabilities
- **Multi-provider Support**: Seamless switching between LLM providers
- **Structured Outputs**: JSON schema streaming (OpenAI)
- **o1 Reasoning**: Special handling for reasoning models (OpenAI)
- **Context Management**: Multi-tenancy and conversation tracking
- **Metrics Collection**: Performance monitoring and usage tracking

## Architecture

```
User Application
       ↓
   Agent.RunStream()
       ↓
   LLM.GenerateStream()
       ↓
   SSE Parser (Provider-specific)
       ↓
   Channel Events
       ↓
   Event Processing
       ↓
   Real-time Output
```

## Error Handling

The examples demonstrate robust error handling:
- Connection failures are gracefully handled
- Partial responses are preserved
- Stream interruptions trigger proper cleanup
- Context cancellation stops streams immediately

## Performance Considerations

- Channel buffer sizes can be tuned for throughput
- Events include timestamps for latency analysis
- Memory usage is controlled through buffer management
- Context cancellation provides immediate cleanup

## Next Steps

- Explore the gRPC streaming examples for service-to-service communication
- Check out the HTTP/SSE examples for browser integration
- Review the test files for additional streaming scenarios
