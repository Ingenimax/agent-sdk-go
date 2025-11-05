<div align="center">
<img src="/docs/img/logo-header.png#gh-light-mode-only" alt="Ingenimax" width="600">
<img src="/docs/img/logo-header-inverted.png#gh-dark-mode-only" alt="Ingenimax" width="600">
</div>

# Agent Go SDK

A powerful Go framework for building production-ready AI agents that seamlessly integrates memory management, tool execution, multi-LLM support, and enterprise features into a flexible, extensible architecture.

## Community

[![Discord](https://img.shields.io/badge/Discord-Join%20Our%20Community-5865F2?style=for-the-badge&logo=discord&logoColor=white)](https://discord.com/invite/MjJbDG2nQZ)

Join our Discord server to collaborate, share what you're building, and get community support for agent-sdk-go!

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

#### As a Go Library

Add the SDK to your Go project:

```bash
go get github.com/Ingenimax/agent-sdk-go
```

#### As a CLI Tool (Headless SDK)

**Option 1: Download Pre-built Binaries (Recommended)**

Download the latest release for your platform from [GitHub Releases](https://github.com/Ingenimax/agent-sdk-go/releases) and add it to your PATH.

**Option 2: Install via Go**

```bash
go install github.com/Ingenimax/agent-sdk-go/cmd/agent-cli@latest
```

**Option 3: Build from Source**

```bash
# Clone the repository
git clone https://github.com/Ingenimax/agent-sdk-go
cd agent-sdk-go

# Build the CLI tool
make build-cli

# Install to system PATH (optional)
make install
```

**Quick CLI Start:**
```bash
# Initialize configuration
agent-cli init

# Option 1: Set environment variables
export OPENAI_API_KEY=your_api_key_here

# Option 2: Use .env file (recommended)
cp env.example .env
# Edit .env with your API keys

# Run a simple query
agent-cli run "What's the weather in San Francisco?"

# Start interactive chat
agent-cli chat
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

### Structured Output with YAML Configuration

The SDK supports defining structured output (JSON responses) directly in YAML configuration files. This allows you to automatically apply structured output when creating agents from YAML and unmarshal responses directly into Go structs.

**agents.yaml with structured output**:
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
  response_format:
    type: "json_object"
    schema_name: "ResearchResult"
    schema_definition:
      type: "object"
      properties:
        findings:
          type: "array"
          items:
            type: "object"
            properties:
              title:
                type: "string"
                description: "Title of the finding"
              description:
                type: "string"
                description: "Detailed description"
              source:
                type: "string"
                description: "Source of the information"
        summary:
          type: "string"
          description: "Executive summary of findings"
        metadata:
          type: "object"
          properties:
            total_findings:
              type: "integer"
            research_date:
              type: "string"
```

**tasks.yaml with structured output**:
```yaml
research_task:
  description: >
    Conduct a thorough research about {topic}
    Make sure you find any interesting and relevant information.
  expected_output: >
    A structured JSON response with findings, summary, and metadata
  agent: researcher
  output_file: "{topic}_report.json"
  response_format:
    type: "json_object"
    schema_name: "ResearchResult"
    schema_definition:
      # Same schema as above
```

**Usage in Go code**:
```go
// Define your Go struct to match the YAML schema
type ResearchResult struct {
    Findings []struct {
        Title       string `json:"title"`
        Description string `json:"description"`
        Source      string `json:"source"`
    } `json:"findings"`
    Summary  string `json:"summary"`
    Metadata struct {
        TotalFindings int    `json:"total_findings"`
        ResearchDate  string `json:"research_date"`
    } `json:"metadata"`
}

// Create agent and execute task
agent, err := agent.CreateAgentForTask("research_task", agentConfigs, taskConfigs, variables, agent.WithLLM(llm))
result, err := agent.ExecuteTaskFromConfig(context.Background(), "research_task", taskConfigs, variables)

// Unmarshal structured output
var structured ResearchResult
err = json.Unmarshal([]byte(result), &structured)
```

For more details, see [Structured Output with YAML Configuration](docs/structured_output_yaml.md).

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

The SDK supports both **eager** and **lazy** MCP server initialization:

- **Eager**: MCP servers are initialized when the agent is created
- **Lazy**: MCP servers are initialized only when their tools are first called (recommended)

#### Lazy MCP Integration (Recommended)

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

func main() {
	// Create OpenAI LLM client
	apiKey := os.Getenv("OPENAI_API_KEY")
	llm := openai.NewClient(apiKey, openai.WithModel("gpt-4o-mini"))

	// Define lazy MCP configurations
	// Note: The CLI supports dynamic tool discovery, but the SDK requires explicit tool definitions
	lazyMCPConfigs := []agent.LazyMCPConfig{
		{
			Name:    "aws-api-server",
			Type:    "stdio",
			Command: "docker",
			Args:    []string{"run", "--rm", "-i", "public.ecr.aws/awslabs-mcp/awslabs/aws-api-mcp-server:latest"},
			Env:     []string{"AWS_REGION=us-west-2"},
			Tools: []agent.LazyMCPToolConfig{
				{
					Name:        "suggest_aws_commands",
					Description: "Suggest AWS CLI commands based on natural language",
					Schema:      map[string]interface{}{"type": "object", "properties": map[string]interface{}{"query": map[string]interface{}{"type": "string"}}},
				},
			},
		},
		{
			Name:    "kubectl-ai",
			Type:    "stdio",
			Command: "kubectl-ai",
			Args:    []string{"--mcp-server"},
			Tools: []agent.LazyMCPToolConfig{
				{
					Name:        "kubectl",
					Description: "Execute kubectl commands against Kubernetes cluster",
					Schema:      map[string]interface{}{"type": "object", "properties": map[string]interface{}{"command": map[string]interface{}{"type": "string"}}},
				},
			},
		},
	}

	// Create agent with lazy MCP configurations
	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithLazyMCPConfigs(lazyMCPConfigs),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are an AI assistant with access to AWS and Kubernetes tools."),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Use the agent - MCP servers will be initialized on first tool use
	response, err := myAgent.Run(context.Background(), "List my EC2 instances and show cluster pods")
	if err != nil {
		log.Fatalf("Failed to run agent: %v", err)
	}

	fmt.Println("Agent Response:", response)
}
```

#### Eager MCP Integration

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

## CLI Tool (Headless SDK)

The Agent SDK includes a powerful command-line interface for headless usage:

### CLI Features

- 🤖 **Multiple LLM Providers**: OpenAI, Anthropic, Google Vertex AI, Ollama, vLLM
- 💬 **Interactive Chat Mode**: Real-time conversations with persistent memory
- 📝 **Task Execution**: Run predefined tasks from YAML configurations
- 🎨 **Auto-Configuration**: Generate agent configs from simple prompts
- 🔧 **Flexible Configuration**: JSON-based configuration with environment variables
- 🛠️ **Rich Tool Integration**: Web search, GitHub, MCP servers, and more
- 🔌 **MCP Server Management**: Add, list, remove, and test MCP servers
- 📄 **.env File Support**: Automatic loading of environment variables from .env files

### CLI Commands

```bash
# Initialize configuration
agent-cli init

# Run agent with a single prompt
agent-cli run "Explain quantum computing in simple terms"

# Direct execution (no setup required)
agent-cli --prompt "What is 2+2?"

# Direct execution with MCP server
agent-cli --prompt "List my EC2 instances" \
  --mcp-config ./aws_api_server.json \
  --allowedTools "mcp__aws__suggest_aws_commands,mcp__aws__call_aws" \
  --dangerously-skip-permissions

# Execute predefined tasks
agent-cli task --agent-config=agents.yaml --task-config=tasks.yaml --task=research_task --topic="AI"

# Start interactive chat
agent-cli chat

# Generate configurations from system prompt
agent-cli generate --prompt="You are a travel advisor" --output=./configs

# List available resources
agent-cli list providers
agent-cli list models
agent-cli list tools

# Manage configuration
agent-cli config show
agent-cli config set provider anthropic

# Manage MCP servers
agent-cli mcp add --type=http --url=http://localhost:8083/mcp --name=my-server
agent-cli mcp list
agent-cli mcp remove --name=my-server

# Import/Export MCP servers from JSON config
agent-cli mcp import --file=mcp-servers.json
agent-cli mcp export --file=mcp-servers.json

# Direct execution with MCP servers and tool filtering
agent-cli --prompt "List my EC2 instances" \
  --mcp-config ./aws_api_server.json \
  --allowedTools "suggest_aws_commands,call_aws" \
  --dangerously-skip-permissions

# Kubernetes management with kubectl-ai
agent-cli --prompt "List all pods in the default namespace" \
  --mcp-config ./kubectl_ai.json \
  --allowedTools "kubectl" \
  --dangerously-skip-permissions
```

### Advanced MCP Features

The CLI now supports **dynamic tool discovery** and **flexible tool filtering**:

- **No Hardcoded Tools**: MCP servers define their own tools and schemas
- **Dynamic Discovery**: Tools are discovered when MCP servers are first initialized
- **Flexible Filtering**: Use `--allowedTools` to specify exactly which tools can be used
- **JSON Configuration**: Load MCP server configurations from external JSON files
- **Environment Variables**: Each MCP server can specify custom environment variables

**Popular MCP Servers:**
- **AWS API Server**: AWS CLI operations and suggestions
- **kubectl-ai**: Kubernetes cluster management via natural language
- **Filesystem Server**: File system operations and management
- **Database Server**: SQL query execution and database operations

### CLI Documentation

For complete CLI documentation, see: [CLI README](cmd/agent-cli/README.md)

## Examples

Check out the `cmd/examples` directory for complete examples:

- **Simple Agent**: Basic agent with system prompt
- **YAML Configuration**: Defining agents and tasks in YAML
- **Auto-Configuration**: Generating agent configurations from system prompts
- **Agent Config Wizard**: Interactive CLI for creating and using agents
- **MCP Integration**: Using Model Context Protocol servers with agents
- **Multi-LLM Support**: Examples using OpenAI, Azure OpenAI, Anthropic, and Vertex AI
- **Vertex AI Integration**: Comprehensive examples with Gemini models, reasoning modes, and tools

### LLM Provider Examples

- `examples/llm/openai/`: OpenAI integration examples
- `examples/llm/azureopenai/`: Azure OpenAI integration examples with deployment-based configuration
- `examples/llm/anthropic/`: Anthropic Claude integration examples
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
