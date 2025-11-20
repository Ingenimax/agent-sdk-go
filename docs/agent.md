# Agent

This document explains how to use the Agent component of the Agent SDK.

## Overview

The Agent is the core component of the SDK that coordinates the LLM, memory, and tools to create an intelligent assistant that can understand and respond to user queries.

## Creating an Agent

There are two main ways to create an agent: using Go code with options or loading from a YAML configuration file.

### Method 1: Using Go Code with Options

To create a new agent programmatically, use the `NewAgent` function with various options:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// Create a new agent
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithSystemPrompt("You are a helpful AI assistant."),
)
if err != nil {
    log.Fatalf("Failed to create agent: %v", err)
}
```

### Method 2: Using YAML Configuration

You can load agent configurations from YAML files using `LoadAgentConfigsFromFile` and create agents with `NewAgentFromConfig`:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

// Load agent configurations from YAML file
configs, err := agent.LoadAgentConfigsFromFile("agents.yaml")
if err != nil {
    log.Fatalf("Failed to load agent configs: %v", err)
}

// Create LLM client
llm := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

// Create agent from configuration
agentInstance, err := agent.NewAgentFromConfig("file_analyzer", configs, nil, agent.WithLLM(llm))
if err != nil {
    log.Fatalf("Failed to create agent from config: %v", err)
}
```

## Agent Options

The Agent can be configured with various options:

### WithLLM

Sets the LLM provider for the agent:

```go
agent.WithLLM(openaiClient)
```

### WithMemory

Sets the memory system for the agent:

```go
agent.WithMemory(memory.NewConversationBuffer())
```

### WithTools

Adds tools to the agent:

```go
agent.WithTools(
    websearch.New(googleAPIKey, googleSearchEngineID),
    calculator.New(),
)
```

### WithSystemPrompt

Sets the system prompt for the agent:

```go
agent.WithSystemPrompt("You are a helpful AI assistant specialized in answering questions about science.")
```

### WithOrgID

Sets the organization ID for multi-tenancy:

```go
agent.WithOrgID("org-123")
```

### WithTracer

Sets the tracer for observability:

```go
agent.WithTracer(langfuse.New(langfuseSecretKey, langfusePublicKey))
```

### WithGuardrails

Sets the guardrails for safety:

```go
agent.WithGuardrails(guardrails.New(guardrailsConfigPath))
```

## YAML Configuration

The YAML configuration system provides a powerful way to define agent configurations declaratively. Here's the complete structure and capabilities:

### Basic Agent Configuration

```yaml
# Example agent configuration
my_agent:
  role: "Data Analysis Expert"
  goal: "Analyze data and provide insights"
  backstory: "Expert in data analysis with years of experience"

  # Behavioral settings
  max_iterations: 10
  require_plan_approval: false

  # LLM configuration
  llm_config:
    temperature: 0.3
    top_p: 0.9
    enable_reasoning: true
    reasoning_budget: 20000

  # Stream configuration
  stream_config:
    buffer_size: 100
    include_tool_progress: true
    include_intermediate_messages: false

  # Runtime settings
  runtime:
    log_level: "info"
    enable_tracing: true
    enable_metrics: true
    timeout: "30m"
```

### MCP Server Configuration

Configure Model Context Protocol (MCP) servers for extended capabilities:

```yaml
my_agent:
  mcp:
    mcpServers:
      filesystem:
        command: "npx"
        args: ["-y", "@modelcontextprotocol/server-filesystem", "."]

      database:
        command: "python"
        args: ["-m", "mcp_server_database"]
        env:
          DATABASE_URL: "${DATABASE_URL}"
```

### Tool Configuration

Configure various types of tools for your agent:

```yaml
my_agent:
  tools:
    # Built-in tools
    - type: "builtin"
      name: "calculator"
      enabled: true

    - type: "builtin"
      name: "websearch"
      enabled: true
      config:
        api_key: "${GOOGLE_API_KEY}"
        search_engine_id: "${GOOGLE_SEARCH_ENGINE_ID}"

    # Custom tools
    - type: "custom"
      name: "custom_analyzer"
      description: "Custom analysis tool"
      config:
        endpoint: "https://api.example.com/analyze"

    # Agent tools (calling other agents)
    - type: "agent"
      name: "specialist_agent"
      url: "http://specialist-service:8080"
      timeout: "5m"
```

### Memory Configuration

Configure different memory backends:

```yaml
my_agent:
  memory:
    # Buffer memory (default)
    type: "buffer"
    config:
      max_tokens: 4000

  # OR Redis memory
  # memory:
  #   type: "redis"
  #   config:
  #     address: "localhost:6379"
  #     db: 0

  # OR Vector memory
  # memory:
  #   type: "vector"
  #   config:
  #     provider: "weaviate"
  #     endpoint: "http://localhost:8080"
```

### Sub-Agents Configuration

Create hierarchical agent structures with sub-agents:

```yaml
main_agent:
  role: "Project Coordinator"
  goal: "Coordinate complex projects"
  backstory: "Expert project manager"

  sub_agents:
    code_analyzer:
      role: "Code Analysis Specialist"
      goal: "Analyze code for quality and structure"
      backstory: "Expert at code analysis"
      max_iterations: 8
      llm_config:
        temperature: 0.2
      mcp:
        mcpServers:
          filesystem:
            command: "npx"
            args: ["-y", "@modelcontextprotocol/server-filesystem", "."]

    document_reviewer:
      role: "Documentation Specialist"
      goal: "Review and improve documentation"
      backstory: "Technical writing expert"
      tools:
        - type: "builtin"
          name: "text_processor"
          enabled: true
```

### Response Format Configuration

Configure structured output formats:

```yaml
my_agent:
  response_format:
    type: "json_schema"
    schema_name: "analysis_result"
    schema_definition:
      type: "object"
      properties:
        summary:
          type: "string"
          description: "Brief summary of analysis"
        score:
          type: "number"
          description: "Analysis score from 0-100"
        recommendations:
          type: "array"
          items:
            type: "string"
      required: ["summary", "score"]
```

### Environment Variable Expansion

YAML configurations support environment variable expansion using `${VARIABLE_NAME}` syntax:

```yaml
my_agent:
  tools:
    - type: "builtin"
      name: "websearch"
      config:
        api_key: "${GOOGLE_API_KEY}"        # Expands to env var
        search_engine_id: "${GOOGLE_CSE_ID}"

  mcp:
    mcpServers:
      database:
        env:
          DATABASE_URL: "${DATABASE_URL}"   # Environment variables for MCP servers
```

### Complete YAML Example

Here's a comprehensive example showing all features:

```yaml
# agents.yaml
file_analyzer:
  role: "File Analysis Coordinator"
  goal: "Analyze files and directories, coordinate with specialized analysis teams"
  backstory: "Expert file analyzer who coordinates with specialized teams"

  max_iterations: 10
  require_plan_approval: false

  llm_config:
    temperature: 0.3
    enable_reasoning: true
    reasoning_budget: 15000

  stream_config:
    include_tool_progress: true
    include_intermediate_messages: false

  runtime:
    log_level: "info"
    enable_tracing: true
    timeout: "15m"

  mcp:
    mcpServers:
      filesystem:
        command: "npx"
        args: ["-y", "@modelcontextprotocol/server-filesystem", "."]

  tools:
    - type: "builtin"
      name: "calculator"
      enabled: true

  memory:
    type: "buffer"
    config:
      max_tokens: 8000

  sub_agents:
    code_analyzer:
      role: "Code Analysis Specialist"
      goal: "Analyze code files for structure, patterns, and quality"
      backstory: "Expert at reading and understanding code"
      max_iterations: 8
      llm_config:
        temperature: 0.2
      mcp:
        mcpServers:
          filesystem:
            command: "npx"
            args: ["-y", "@modelcontextprotocol/server-filesystem", "."]

    document_analyzer:
      role: "Document Analysis Specialist"
      goal: "Analyze text files, documentation, and configuration files"
      backstory: "Specialist in understanding documentation and config files"
      tools:
        - type: "builtin"
          name: "text_processor"
          enabled: true
```

## Running the Agent

To run the agent with a user query:

```go
response, err := agent.Run(ctx, "What is the capital of France?")
if err != nil {
    log.Fatalf("Failed to run agent: %v", err)
}
fmt.Println(response)
```

## Streaming Responses

To stream the agent's response:

```go
stream, err := agent.RunStream(ctx, "Tell me a long story about a dragon")
if err != nil {
    log.Fatalf("Failed to run agent with streaming: %v", err)
}

for {
    chunk, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatalf("Error receiving stream: %v", err)
    }
    fmt.Print(chunk)
}
```

## Using Tools

The agent can use tools to perform actions or retrieve information:

```go
// Create tools
searchTool := websearch.New(googleAPIKey, googleSearchEngineID)
calculatorTool := calculator.New()

// Create agent with tools
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithTools(searchTool, calculatorTool),
    agent.WithSystemPrompt("You are a helpful AI assistant. Use tools when needed."),
)

// Run the agent with a query that might require tools
response, err := agent.Run(ctx, "What is the population of Tokyo multiplied by 2?")
```

## Advanced Usage

### Custom Tool Execution

You can implement custom tool execution logic:

```go
// Create a custom tool executor
executor := agent.NewToolExecutor(func(ctx context.Context, toolName string, input string) (string, error) {
    // Custom logic for executing tools
    if toolName == "custom_tool" {
        // Do something special
        return "Custom result", nil
    }

    // Fall back to default execution for other tools
    tool, found := toolRegistry.Get(toolName)
    if !found {
        return "", fmt.Errorf("tool not found: %s", toolName)
    }
    return tool.Run(ctx, input)
})

// Create agent with custom tool executor
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithTools(searchTool, calculatorTool),
    agent.WithToolExecutor(executor),
)
```

### Custom Message Processing

You can implement custom message processing:

```go
// Create a custom message processor
processor := agent.NewMessageProcessor(func(ctx context.Context, message interfaces.Message) (interfaces.Message, error) {
    // Process the message
    if message.Role == "user" {
        // Add metadata to user messages
        if message.Metadata == nil {
            message.Metadata = make(map[string]interface{})
        }
        message.Metadata["processed_at"] = time.Now()
    }
    return message, nil
})

// Create agent with custom message processor
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithMessageProcessor(processor),
)
```

## Examples

### Example 1: Programmatic Agent Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
    "github.com/Ingenimax/agent-sdk-go/pkg/tracing/langfuse"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create OpenAI client
    openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

    // Create tools
    searchTool := websearch.New(
        cfg.Tools.WebSearch.GoogleAPIKey,
        cfg.Tools.WebSearch.GoogleSearchEngineID,
    )

    // Create tracer
    tracer := langfuse.New(
        cfg.Tracing.Langfuse.SecretKey,
        cfg.Tracing.Langfuse.PublicKey,
    )

    // Create a new agent
    agent, err := agent.NewAgent(
        agent.WithLLM(openaiClient),
        agent.WithMemory(memory.NewConversationBuffer()),
        agent.WithTools(searchTool),
        agent.WithTracer(tracer),
        agent.WithSystemPrompt("You are a helpful AI assistant. Use tools when needed."),
    )
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Run the agent
    ctx := context.Background()
    response, err := agent.Run(ctx, "What's the latest news about artificial intelligence?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }

    fmt.Println(response)
}
```

### Example 2: YAML-based Agent Setup

First, create an `agents.yaml` configuration file:

```yaml
# agents.yaml
research_assistant:
  role: "Research Assistant"
  goal: "Help users find and analyze information"
  backstory: "Experienced researcher with access to web search capabilities"

  max_iterations: 15
  require_plan_approval: false

  llm_config:
    temperature: 0.7
    enable_reasoning: true

  tools:
    - type: "builtin"
      name: "websearch"
      enabled: true
      config:
        api_key: "${GOOGLE_API_KEY}"
        search_engine_id: "${GOOGLE_SEARCH_ENGINE_ID}"

  memory:
    type: "buffer"
    config:
      max_tokens: 4000

  runtime:
    log_level: "info"
    enable_tracing: true
```

Then load and use the agent in Go:

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
    // Create LLM client
    llm := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

    // Load agent configurations from YAML
    configs, err := agent.LoadAgentConfigsFromFile("agents.yaml")
    if err != nil {
        log.Fatalf("Failed to load agent configs: %v", err)
    }

    // Create agent from configuration
    agentInstance, err := agent.NewAgentFromConfig("research_assistant", configs, nil, agent.WithLLM(llm))
    if err != nil {
        log.Fatalf("Failed to create agent from config: %v", err)
    }

    // Run the agent
    result, err := agentInstance.Run(context.Background(), "What are the latest developments in renewable energy?")
    if err != nil {
        log.Fatal(err)
    }

    println(result)
}
```

### Example 3: Multi-Agent System with YAML

```yaml
# multi_agents.yaml
project_manager:
  role: "Project Manager"
  goal: "Coordinate development projects and delegate tasks"
  backstory: "Experienced project manager with technical background"

  max_iterations: 20
  require_plan_approval: true

  llm_config:
    temperature: 0.5
    enable_reasoning: true

  sub_agents:
    code_reviewer:
      role: "Code Review Specialist"
      goal: "Review code quality, style, and best practices"
      backstory: "Senior developer with expertise in code quality"
      max_iterations: 10
      llm_config:
        temperature: 0.3
      mcp:
        mcpServers:
          filesystem:
            command: "npx"
            args: ["-y", "@modelcontextprotocol/server-filesystem", "."]

    documentation_writer:
      role: "Technical Writer"
      goal: "Create and maintain project documentation"
      backstory: "Technical writing specialist"
      tools:
        - type: "builtin"
          name: "text_processor"
          enabled: true

    qa_tester:
      role: "QA Engineer"
      goal: "Ensure software quality through testing"
      backstory: "Quality assurance expert"
      tools:
        - type: "builtin"
          name: "test_runner"
          enabled: true
```

## Configuration Reference

For complete configuration options and examples, see:

- **Basic Configuration**: Simple agent setup with role, goal, and backstory
- **LLM Configuration**: Temperature, reasoning, and model-specific settings
- **Tool Configuration**: Built-in, custom, MCP, and agent tools
- **Memory Configuration**: Buffer, Redis, and vector memory backends
- **MCP Integration**: Model Context Protocol server configuration
- **Sub-Agents**: Hierarchical agent structures
- **Runtime Settings**: Logging, tracing, timeouts, and metrics
- **Response Formats**: Structured JSON schema outputs
- **Environment Variables**: Dynamic configuration with `${VAR}` syntax
