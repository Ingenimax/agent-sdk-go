<div align="center">
<img src="/docs/img/logo-header.png#gh-light-mode-only" alt="Ingenimax" width="600">
<img src="/docs/img/logo-header-inverted.png#gh-dark-mode-only" alt="Ingenimax" width="600">
</div>

# Agent Go SDK

A powerful Go framework for building production-ready AI agents that seamlessly integrates memory management, tool execution, multi-LLM support, and enterprise features into a flexible, extensible architecture.

## Features

### Core Capabilities
- 🧠 **Multi-Model Intelligence**: Seamless integration with OpenAI, Anthropic, and Google Vertex AI (Gemini models).
- 🔧 **Modular Tool Ecosystem**: Expand agent capabilities with plug-and-play tools for web search, data retrieval, and custom operations
- 📝 **Advanced Memory Management**: Persistent conversation tracking with buffer and vector-based retrieval options
- 🔌 **MCP Integration**: Support for Model Context Protocol (MCP) servers via HTTP and stdio transports

### Enterprise-Ready
- 🚦 **Built-in Guardrails**: Comprehensive safety mechanisms for responsible AI deployment
- 📈 **Complete Observability**: Integrated tracing and logging for monitoring and debugging
- 🏢 **Enterprise Multi-tenancy**: Securely support multiple organizations with isolated resources

### Development Experience
- 🛠️ **Structured Task Framework**: Plan, approve, and execute complex multi-step operations
- 📄 **Declarative Configuration**: Define sophisticated agents and tasks using intuitive YAML definitions
- 🧙 **Zero-Effort Bootstrapping**: Auto-generate complete agent configurations from simple system prompts

## Getting Started

### Prerequisites

- Go 1.23+
- Redis (optional, for distributed memory)

### Installation

Add the SDK to your Go project:

```bash
go get github.com/Ingenimax/agent-sdk-go
```

### Configuration

The SDK uses environment variables for configuration. Key variables include:

- `OPENAI_API_KEY`: Your OpenAI API key
- `OPENAI_MODEL`: The model to use (e.g., gpt-4o-mini)
- `LOG_LEVEL`: Logging level (debug, info, warn, error)
- `REDIS_ADDRESS`: Redis server address (if using Redis for memory)

See `.env.example` for a complete list of configuration options.

## Usage Examples

### Creating a Simple Agent

```go
package main

import (
	"context"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

func main() {
	// Create a logger
	logger := logging.New()

	// Get configuration
	cfg := config.Get()

	// Create a new agent with OpenAI
	openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey,
		openai.WithLogger(logger))

	agent, err := agent.NewAgent(
		agent.WithLLM(openaiClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(createTools(logger).List()...),
		agent.WithSystemPrompt("You are a helpful AI assistant. When you don't know the answer or need real-time information, use the available tools to find the information."),
		agent.WithName("ResearchAssistant"),
	)
	if err != nil {
		logger.Error(context.Background(), "Failed to create agent", map[string]interface{}{"error": err.Error()})
		return
	}

	// Create a context with organization ID and conversation ID
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-123")

	// Run the agent
	response, err := agent.Run(ctx, "What's the weather in San Francisco?")
	if err != nil {
		logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
		return
	}

	fmt.Println(response)
}

func createTools(logger logging.Logger) *tools.Registry {
	// Get configuration
	cfg := config.Get()

	// Create tools registry
	toolRegistry := tools.NewRegistry()

	// Add web search tool if API keys are available
	if cfg.Tools.WebSearch.GoogleAPIKey != "" && cfg.Tools.WebSearch.GoogleSearchEngineID != "" {
		searchTool := websearch.New(
			cfg.Tools.WebSearch.GoogleAPIKey,
			cfg.Tools.WebSearch.GoogleSearchEngineID,
		)
		toolRegistry.Register(searchTool)
	}

	return toolRegistry
}
```

### Creating an Agent with YAML Configuration

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
	// Get OpenAI API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OpenAI API key not provided. Set OPENAI_API_KEY environment variable.")
	}

	// Create the LLM client
	llm := openai.NewClient(apiKey)

	// Load agent configurations
	agentConfigs, err := agent.LoadAgentConfigsFromFile("agents.yaml")
	if err != nil {
		log.Fatalf("Failed to load agent configurations: %v", err)
	}

	// Load task configurations
	taskConfigs, err := agent.LoadTaskConfigsFromFile("tasks.yaml")
	if err != nil {
		log.Fatalf("Failed to load task configurations: %v", err)
	}

	// Create variables map for template substitution
	variables := map[string]string{
		"topic": "Artificial Intelligence",
	}

	// Create the agent for a specific task
	taskName := "research_task"
	agent, err := agent.CreateAgentForTask(taskName, agentConfigs, taskConfigs, variables, agent.WithLLM(llm))
	if err != nil {
		log.Fatalf("Failed to create agent for task: %v", err)
	}

	// Execute the task
	fmt.Printf("Executing task '%s' with topic '%s'...\n", taskName, variables["topic"])
	result, err := agent.ExecuteTaskFromConfig(context.Background(), taskName, taskConfigs, variables)
	if err != nil {
		log.Fatalf("Failed to execute task: %v", err)
	}

	// Print the result
	fmt.Println("\nTask Result:")
	fmt.Println(result)
}
```

Example YAML configurations:

**agents.yaml**:
```yaml
researcher:
  role: >
    {topic} Senior Data Researcher
  goal: >
    Uncover cutting-edge developments in {topic}
  backstory: >
    You're a seasoned researcher with a knack for uncovering the latest
    developments in {topic}. Known for your ability to find the most relevant
    information and present it in a clear and concise manner.

reporting_analyst:
  role: >
    {topic} Reporting Analyst
  goal: >
    Create detailed reports based on {topic} data analysis and research findings
  backstory: >
    You're a meticulous analyst with a keen eye for detail. You're known for
    your ability to turn complex data into clear and concise reports, making
    it easy for others to understand and act on the information you provide.
```

**tasks.yaml**:
```yaml
research_task:
  description: >
    Conduct a thorough research about {topic}
    Make sure you find any interesting and relevant information given
    the current year is 2025.
  expected_output: >
    A list with 10 bullet points of the most relevant information about {topic}
  agent: researcher

reporting_task:
  description: >
    Review the context you got and expand each topic into a full section for a report.
    Make sure the report is detailed and contains any and all relevant information.
  expected_output: >
    A fully fledged report with the main topics, each with a full section of information.
    Formatted as markdown without '```'
  agent: reporting_analyst
  output_file: "{topic}_report.md"
```

### Auto-Generating Agent Configurations

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
	// Load configuration
	cfg := config.Get()

	// Create LLM client
	openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

	// Create agent with auto-configuration from system prompt
	agent, err := agent.NewAgentWithAutoConfig(
		context.Background(),
		agent.WithLLM(openaiClient),
		agent.WithSystemPrompt("You are a travel advisor who helps users plan trips and vacations. You specialize in finding hidden gems and creating personalized itineraries based on travelers' preferences."),
		agent.WithName("Travel Assistant"),
	)
	if err != nil {
		panic(err)
	}

	// Access the generated configurations
	agentConfig := agent.GetGeneratedAgentConfig()
	taskConfigs := agent.GetGeneratedTaskConfigs()

	// Print generated agent details
	fmt.Printf("Generated Agent Role: %s\n", agentConfig.Role)
	fmt.Printf("Generated Agent Goal: %s\n", agentConfig.Goal)
	fmt.Printf("Generated Agent Backstory: %s\n", agentConfig.Backstory)

	// Print generated tasks
	fmt.Println("\nGenerated Tasks:")
	for taskName, taskConfig := range taskConfigs {
		fmt.Printf("- %s: %s\n", taskName, taskConfig.Description)
	}

	// Save the generated configurations to YAML files
	agentConfigMap := map[string]agent.AgentConfig{
		"Travel Assistant": *agentConfig,
	}

	// Save agent configs to file
	agentYaml, _ := os.Create("agent_config.yaml")
	defer agentYaml.Close()
	agent.SaveAgentConfigsToFile(agentConfigMap, agentYaml)

	// Save task configs to file
	taskYaml, _ := os.Create("task_config.yaml")
	defer taskYaml.Close()
	agent.SaveTaskConfigsToFile(taskConfigs, taskYaml)

	// Use the auto-configured agent
	response, err := agent.Run(context.Background(), "I want to plan a 3-day trip to Tokyo.")
	if err != nil {
		panic(err)
	}
	fmt.Println(response)
}
```

The auto-configuration feature uses LLM reasoning to derive a complete agent profile and associated tasks from a simple system prompt. The generated configurations include:

- **Agent Profile**: Role, goal, and backstory that define the agent's persona
- **Task Definitions**: Specialized tasks the agent can perform, with descriptions and expected outputs
- **Reusable YAML**: Save configurations for reuse in other applications

This approach dramatically reduces the effort needed to create specialized agents while ensuring consistency and quality.

### Using MCP Servers with an Agent

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func main() {
	logger := log.New(os.Stderr, "AGENT: ", log.LstdFlags)

	// Create OpenAI LLM client
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		logger.Fatal("Please set the OPENAI_API_KEY environment variable.")
	}
	llm := openai.NewClient(apiKey, openai.WithModel("gpt-4o-mini"))

	// Create MCP servers
	var mcpServers []interfaces.MCPServer

	// Connect to HTTP-based MCP server
	httpServer, err := mcp.NewHTTPServer(context.Background(), mcp.HTTPServerConfig{
		BaseURL: "http://localhost:8083/mcp",
	})
	if err != nil {
		logger.Printf("Warning: Failed to initialize HTTP MCP server: %v", err)
	} else {
		mcpServers = append(mcpServers, httpServer)
		logger.Println("Successfully initialized HTTP MCP server.")
	}

	// Connect to stdio-based MCP server
	stdioServer, err := mcp.NewStdioServer(context.Background(), mcp.StdioServerConfig{
		Command: "go",
		Args:    []string{"run", "./server-stdio/main.go"},
	})
	if err != nil {
		logger.Printf("Warning: Failed to initialize STDIO MCP server: %v", err)
	} else {
		mcpServers = append(mcpServers, stdioServer)
		logger.Println("Successfully initialized STDIO MCP server.")
	}

	// Create agent with MCP server support
	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMCPServers(mcpServers),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are an AI assistant that can use tools from MCP servers."),
		agent.WithName("MCPAgent"),
	)
	if err != nil {
		logger.Fatalf("Failed to create agent: %v", err)
	}

	// Create context with organization and conversation IDs
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "mcp-demo")

	// Run the agent with a query that will use MCP tools
	response, err := myAgent.Run(ctx, "What time is it right now?")
	if err != nil {
		logger.Fatalf("Agent run failed: %v", err)
	}

	fmt.Println("Agent response:", response)
}
```

## Architecture

The SDK follows a modular architecture with these key components:

- **Agent**: Coordinates the LLM, memory, and tools
- **LLM**: Interface to language model providers (OpenAI, Anthropic, Google Vertex AI)
- **Memory**: Stores conversation history and context
- **Tools**: Extend the agent's capabilities
- **Vector Store**: For semantic search and retrieval
- **Guardrails**: Ensures safe and responsible AI usage
- **Execution Plan**: Manages planning, approval, and execution of complex tasks
- **Configuration**: YAML-based agent and task definitions

### Supported LLM Providers

- **OpenAI**: GPT-4, GPT-3.5, and other OpenAI models
- **Anthropic**: Claude 3.5 Sonnet, Claude 3 Haiku, and other Claude models  
- **Google Vertex AI**: Gemini 1.5 Pro, Gemini 1.5 Flash, Gemini 2.0 Flash, and Gemini Pro Vision
  - Advanced reasoning modes (none, minimal, comprehensive)
  - Multimodal capabilities with vision models
  - Function calling and tool integration
  - Flexible authentication (ADC or service account files)
- **Ollama**: Local LLM server supporting various open-source models
  - Run models locally without external API calls
  - Support for Llama2, Mistral, CodeLlama, and other models
  - Model management (list, pull, switch models)
  - Local processing for reduced latency and privacy
- **vLLM**: High-performance local LLM inference with PagedAttention
  - Optimized for GPU inference with CUDA
  - Efficient memory management for large models
  - Support for Llama2, Mistral, CodeLlama, and other models
  - Model management (list, pull, switch models)
  - Local processing for reduced latency and privacy

## Examples

Check out the `cmd/examples` directory for complete examples:

- **Simple Agent**: Basic agent with system prompt
- **YAML Configuration**: Defining agents and tasks in YAML
- **Auto-Configuration**: Generating agent configurations from system prompts
- **Agent Config Wizard**: Interactive CLI for creating and using agents
- **MCP Integration**: Using Model Context Protocol servers with agents
- **Multi-LLM Support**: Examples using OpenAI, Anthropic, and Vertex AI
- **Vertex AI Integration**: Comprehensive examples with Gemini models, reasoning modes, and tools

### LLM Provider Examples

- `examples/llm/openai/`: OpenAI integration examples
- `examples/llm/anthropic/`: Anthropic Claude integration examples  
- `examples/llm/vertex/`: Google Vertex AI integration examples with Gemini models
- `examples/llm/ollama/`: Ollama local LLM integration examples
- `examples/llm/vllm/`: vLLM high-performance local LLM integration examples

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Documentation

For more detailed information, refer to the following documents:

- [Environment Variables](docs/environment_variables.md)
- [Memory](docs/memory.md)
- [Tracing](docs/tracing.md)
- [Vector Store](docs/vectorstore.md)
- [LLM](docs/llm.md)
- [Multitenancy](docs/multitenancy.md)
- [Task](docs/task.md)
- [Tools](docs/tools.md)
- [Agent](docs/agent.md)
- [Execution Plan](docs/execution_plan.md)
- [Guardrails](docs/guardrails.md)
- [MCP](docs/mcp.md)
