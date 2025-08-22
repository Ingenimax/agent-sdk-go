# Advanced Agent Streaming: 1 Agent + 2 Subagents + 2 Tools

This example demonstrates advanced streaming capabilities with a focused architecture: **1 main agent coordinating 2 specialized subagents using 2 tools**. This showcases the Agent SDK's streaming implementation with multi-agent coordination, Anthropic Extended Thinking, and reliable final answer delivery.

## Architecture Overview

```
Project Manager (Main Agent)
├── Tool 1: Market Data Lookup (Mock)
├── Tool 2: Trend Analysis (Mock) 
├── Subagent 1: Research Assistant
└── Subagent 2: Data Analyst
```

### 🎯 **Core Components**

#### **1 Main Agent: Project Manager**
- **Role**: Orchestrates the entire workflow with market analysis focus
- **Tools**: Market Data Lookup + Trend Analysis (simplified mock tools)
- **Capabilities**: Extended thinking tokens, iterative tool execution, final answer synthesis
- **Responsibilities**: Task breakdown, data gathering, analysis, and comprehensive reporting

#### **2 Subagents** 
- **Research Assistant**: Trend analysis and information organization
- **Data Analyst**: Market data lookup and statistical analysis

#### **2 Tools**
- **Market Data Lookup**: Provides market statistics (growth rates, market size, segments, regions)
- **Trend Analysis**: Offers market forecasting insights (global, mobile, social, B2B trends)

### 🎬 **Advanced Streaming Features**
- **Real-time agent streaming** with minimal, clean visualization
- **Anthropic Extended Thinking** - Gray thinking content, white final answers
- **Iterative tool execution** with max iterations control
- **Final answer synthesis** - Always delivers conclusions after tool execution
- **Performance metrics** and monitoring
- **Smart thinking mode detection** - Automatic transition from thinking to content

### 🧠 **AI Capabilities Demonstrated**
- **Extended Thinking Tokens** - Real-time reasoning process in gray text
- **Tool Integration** - Seamless mock tool execution within streaming  
- **Iterative Tool Calling** - Multiple tool calls with result feedback to LLM
- **Final Answer Generation** - Guaranteed synthesis after tool completion
- **Agent Coordination** - Multi-agent task delegation and synthesis

## Example Scenario

The example runs a comprehensive **E-commerce Market Analysis for Q1 2025 Business Planning** that demonstrates:

1. **Project Coordination** - Main agent plans and delegates tasks
2. **Information Gathering** - Research assistant capabilities
3. **Data Analysis** - Analyst performs calculations and projections
4. **Tool Usage** - Calculator and web search integration
5. **Synthesis** - Main agent combines findings into actionable recommendations

## Environment Variables

### Required
```bash
# LLM Provider (choose one)
export ANTHROPIC_API_KEY="your_anthropic_key"
export OPENAI_API_KEY="your_openai_key"

# Provider selection (optional, defaults to anthropic)
export LLM_PROVIDER="anthropic"  # or "openai"
```

### Optional (for enhanced functionality)
```bash
# Web search capabilities
export SERPER_API_KEY="your_serper_key"

# GitHub repository analysis
export GITHUB_TOKEN="your_github_token"
```

## Usage

### Basic Usage
```bash
cd examples/advanced_agent_streaming
go run main.go
```

### With Full API Integration
```bash
export ANTHROPIC_API_KEY="your_key"
export SERPER_API_KEY="your_serper_key" 
export GITHUB_TOKEN="your_github_token"
go run main.go
```

### With OpenAI (Full Support)
```bash
export OPENAI_API_KEY="your_key"
export LLM_PROVIDER="openai"
go run main.go
```

**Supported Models:**
- `gpt-4o` - Full streaming, tools, and reasoning support
- `gpt-4-turbo` - Full streaming and tools support  
- `o1-mini`, `o1-preview` - Built-in reasoning only (no external tools)

## Output Features

### 🎨 **Rich Terminal Visualization**
- **Color-coded output** for different event types
- **Progress indicators** for long-running operations
- **Thinking process visualization** with expandable blocks
- **Tool execution tracking** with timing information
- **Performance metrics** dashboard

### 📊 **Streaming Metrics**
- Real-time event counting
- Content length tracking
- Events per second measurement
- Tool usage statistics
- Error rate monitoring

### 🔧 **Tool Execution Tracking**
```
TOOL EXECUTION: market_data_lookup
├─ Arguments: {"query": "growth_rates"}
├─ Status: executing
└─ Executing...
├─ Result: Market Growth Rates:
│   - E-commerce: 16.5% YoY
│   - Mobile commerce: 23.2% YoY
│   - B2B e-commerce: 18.7% YoY
│   - Social commerce: 34.1% YoY
└─ Duration: <nil>
```

### 💭 **Thinking Process Display**
```
THINKING BLOCK #1
────────────────────────────────────────────────────────────
Let me analyze this request and execute a plan for comprehensive 
e-commerce market analysis.

I need to:
1. Analyze current e-commerce trends and growth patterns
2. Calculate market projections for Q1 2025  
3. Identify opportunities/risks for expansion
4. Provide data-backed recommendations

I'll start with trend analysis tool to understand overall trends,
then use market data lookup for specific statistics...
────────────────────────────────────────────────────────────
Final response:
Based on the comprehensive analysis using both trend analysis and 
market data tools, here are my findings and recommendations...
```

## Architecture

### Agent Hierarchy
```
Project Manager (Main Agent)
├── Research Assistant
│   └── Trend Analysis Tool
├── Data Analyst
│   └── Market Data Lookup Tool
└── Main Agent Tools
    ├── Market Data Lookup Tool
    └── Trend Analysis Tool
```

### Streaming Flow
```
LLM (Anthropic/OpenAI)
  ↓ SSE Events
Agent Layer
  ↓ Agent Events
Tool Execution
  ↓ Tool Results
Subagent Delegation  
  ↓ Coordinated Results
Terminal Display
```

## Recent Improvements

### 🔧 **Streaming Reliability Fixes**
- **Final Answer Delivery**: Fixed issue where agents would complete tool execution but not display final synthesized responses
- **Smart Thinking Mode Detection**: Improved transition from thinking (gray) to final content (white)
- **Iterative Tool Calling**: Proper implementation of tool result feedback to LLM for continued conversation
- **Debug Logging**: Comprehensive logging for troubleshooting streaming issues

### 🛠️ **Tool System Improvements**  
- **Simplified Mock Tools**: Replaced complex calculator with reliable mock data tools
- **No External Dependencies**: All tools work without API keys for easy testing
- **Consistent Tool Responses**: Predictable mock data for demonstration purposes

### 🎨 **Minimal Styling**
- **Clean Output**: Removed excessive colors and emojis as requested
- **Focus on Content**: Gray for thinking, white for answers - minimal distraction
- **Professional Appearance**: Clean terminal output suitable for production demos

### 🤖 **OpenAI Implementation Completed**
- **Iterative Tool Calling**: Added missing iterative tool loop to `GenerateWithToolsStream` matching Anthropic implementation
- **Final Answer Synthesis**: Fixed critical bug where OpenAI streaming would execute tools but never provide final answers
- **Tool Result Feedback**: Implemented proper tool result feedback to LLM across iterations with `maxIterations` control
- **Parameter Handling**: Fixed tool message parameter order (`ToolMessage(content, tool_call_id)` vs `ToolMessage(tool_call_id, content)`)
- **o1 Model Support**: Added o1 model compatibility (no system messages, no custom temperature, no tools)
- **Multi-Model Support**: Both Anthropic and OpenAI streaming now work identically with reasoning and tools

## Configuration Options

### Streaming Configuration
```go
streamConfig := interfaces.StreamConfig{
    BufferSize:          1000, // Large buffer for complex ops
    IncludeThinking:     true, // Show reasoning process
    IncludeToolProgress: true, // Track tool execution
}
```

### Performance Tuning
- **Buffer sizes** from 500-1000 for complex operations
- **Concurrent tool execution** for parallel processing
- **Memory optimization** for long conversations
- **Error recovery** with graceful degradation

## Mock Mode

When API keys are not provided, the example runs in mock mode with simulated:
- Web search results
- GitHub repository data  
- Realistic response timing
- Full streaming visualization

This allows testing the streaming architecture without external dependencies.

## Advanced Features

### Performance Monitoring
Real-time tracking of:
- Event processing speed
- Tool execution timing
- Memory usage patterns
- Error rates and recovery

### Error Handling
- Graceful degradation to mock tools
- Retry mechanisms for transient failures
- Clear error reporting in stream
- Recovery and continuation

### Scalability Features
- Large buffer sizes for high-throughput scenarios
- Efficient event processing
- Memory-conscious design
- Concurrent tool execution

## Integration Examples

This example demonstrates patterns for:
- **Production deployment** of streaming agents
- **Multi-tool coordination** in real applications
- **Subagent architecture** for complex workflows
- **Performance monitoring** and optimization
- **Error handling** and resilience patterns

## Dependencies

- Agent SDK Go framework
- LLM providers (Anthropic Claude / OpenAI GPT)
- Web search API (Serper - optional)
- GitHub API (optional)
- Terminal with color support for best experience

## Learning Outcomes

After running this example, you'll understand:
- How to implement complex streaming agent architectures
- Multi-agent coordination patterns
- Tool integration with streaming
- Performance monitoring and optimization
- Production-ready error handling
- Advanced terminal visualization techniques