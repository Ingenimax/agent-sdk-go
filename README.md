# ![Ingenimax](/docs/img/logo-header.png#gh-light-mode-only) ![Ingenimax](/docs/img/logo-header-inverted.png#gh-dark-mode-only)

# Agent Go SDK

A Go-based SDK for building AI agents with various capabilities like memory, tools, LLM integration, and more.

## Features

- 🧠 **Multiple LLM Providers**: Integration with OpenAI, Anthropic, and more
- 🔧 **Extensible Tools System**: Easily add capabilities to your agents
- 📝 **Memory Management**: Store and retrieve conversation history
- 🔍 **Vector Store Integration**: Semantic search capabilities
- 🛠️ **Task Execution**: Plan and execute complex tasks
- 🚦 **Guardrails**: Safety mechanisms for responsible AI
- 📈 **Observability**: Tracing and logging for debugging
- 🏢 **Multi-tenancy**: Support for multiple organizations
- 📄 **YAML Configuration**: Define agents and tasks using YAML files
- 🧙 **Auto-Configuration**: Generate agent configurations from system prompts

## Getting Started

### Prerequisites

- Go 1.21+
- Redis (optional, for distributed memory)

### Installation

Add the SDK to your Go project:

```bash
go get github.com/Ingenimax/agent-sdk-go
```

### Configuration

The SDK uses environment variables for configuration. Key variables include:

- `OPENAI_API_KEY`: Your OpenAI API key
- `OPENAI_MODEL`: The model to use (e.g., gpt-4-turbo)
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
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

func main() {
	// Create OpenAI client
	openaiClient := openai.NewClient("your-api-key")

	// Create a memory store
	memoryStore := memory.NewConversationBuffer()

	// Create a new agent
	agentInstance, err := agent.NewAgent(
		agent.WithLLM(openaiClient),
		agent.WithMemory(memoryStore),
		agent.WithSystemPrompt("You are a helpful AI assistant."),
	)
	if err != nil {
		panic(err)
	}

	// Run the agent
	response, err := agentInstance.Run(context.Background(), "What is the capital of France?")
	if err != nil {
		panic(err)
	}

	fmt.Println(response)
}
```

### Adding Tools to an Agent

```go
import (
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

// Create a tools registry
toolRegistry := tools.NewRegistry()

// Add the web search tool
searchTool := websearch.New(
	"your-google-api-key",
	"your-search-engine-id",
)
toolRegistry.Register(searchTool)

// Create agent with tools
agent, err := agent.NewAgent(
	agent.WithLLM(openaiClient),
	agent.WithMemory(memoryStore),
	agent.WithTools(toolRegistry.List()...),
	agent.WithSystemPrompt("You are a helpful AI assistant with web search abilities."),
)
```

### Creating an Agent with YAML Configuration

```go
// Load agent configurations from YAML file
agentConfigs, err := agent.LoadAgentConfigsFromFile("agent_config.yaml")
if err != nil {
    panic(err)
}

// Load task configurations from YAML file
taskConfigs, err := agent.LoadTaskConfigsFromFile("task_config.yaml")
if err != nil {
    panic(err)
}

// Variables for template substitution
variables := map[string]string{
    "topic": "Climate Change",
}

// Create agent from YAML configuration
agent, err := agent.NewAgentFromConfig(
    "Research Assistant",
    agentConfigs,
    variables,
    agent.WithLLM(openaiClient),
)
if err != nil {
    panic(err)
}

// Execute a task defined in YAML
result, err := agent.ExecuteTaskFromConfig(context.Background(), "research_task", taskConfigs, variables)
if err != nil {
    panic(err)
}
fmt.Println(result)
```

Example YAML configurations:

**agent_config.yaml**:
```yaml
Research Assistant:
  role: "{topic} Research Specialist"
  goal: "To gather, analyze, and summarize information about {topic}"
  backstory: "You are an expert researcher with years of experience in {topic} studies."
```

**task_config.yaml**:
```yaml
research_task:
  description: "Find the latest research papers on {topic} and summarize their key findings."
  expected_output: "A concise summary of recent research findings on {topic}."
  agent: "Research Assistant"
  output_file: "research_summary_{topic}.md"
```

### Auto-Generating Agent Configurations

```go
// Create agent with auto-configuration from system prompt
agent, err := agent.NewAgentWithAutoConfig(
    context.Background(),
    agent.WithLLM(openaiClient),
    agent.WithSystemPrompt("You are a travel advisor who helps users plan trips and vacations."),
    agent.WithName("Travel Assistant"),
)
if err != nil {
    panic(err)
}

// Access the generated configurations
agentConfig := agent.GetGeneratedAgentConfig()
taskConfigs := agent.GetGeneratedTaskConfigs()

// Save the generated configurations to YAML files
agentConfigMap := map[string]agent.AgentConfig{
    "Travel Assistant": *agentConfig,
}

// Save agent configs
agentYaml, _ := os.Create("agent_config.yaml")
defer agentYaml.Close()
agent.SaveAgentConfigsToFile(agentConfigMap, agentYaml)

// Save task configs
taskYaml, _ := os.Create("task_config.yaml")
defer taskYaml.Close()
agent.SaveTaskConfigsToFile(taskConfigs, taskYaml)
```

### Using Execution Plans with Approval

```go
// Create an agent that requires plan approval
agent, err := agent.NewAgent(
	agent.WithLLM(openaiClient),
	agent.WithMemory(memoryStore),
	agent.WithTools(toolRegistry.List()...),
	agent.WithSystemPrompt("You can help with complex tasks that require planning."),
	agent.WithRequirePlanApproval(true), // Enable execution plan workflow
)

// When the agent generates a plan, you can get it using ListTasks
plans := agent.ListTasks()

// Approve a plan by task ID
response, err := agent.ApproveExecutionPlan(ctx, plans[0])

// Or modify a plan with user feedback
modifiedPlan, err := agent.ModifyExecutionPlan(ctx, plans[0], "Change step 2 to use a different tool")
```

## Architecture

The SDK follows a modular architecture with these key components:

- **Agent**: Coordinates the LLM, memory, and tools
- **LLM**: Interface to language model providers
- **Memory**: Stores conversation history and context
- **Tools**: Extend the agent's capabilities
- **Vector Store**: For semantic search and retrieval
- **Guardrails**: Ensures safe and responsible AI usage
- **Execution Plan**: Manages planning, approval, and execution of complex tasks
- **Configuration**: YAML-based agent and task definitions

## Examples

Check out the `cmd/examples` directory for complete examples:

- **Simple Agent**: Basic agent with system prompt
- **YAML Configuration**: Defining agents and tasks in YAML
- **Auto-Configuration**: Generating agent configurations from system prompts
- **Agent Config Wizard**: Interactive CLI for creating and using agents
- **Combined Config Example**: Shows both YAML and auto-configuration approaches

## License

This project is licensed under the MIT License - see the LICENSE file for details.

# StarOps Agent

A PlatformOps assistant that helps with deployment and management tasks.

## Features

- Interactive command-line interface
- Natural language understanding for deployment requests
- Multi-agent architecture with specialist agents
- Detailed deployment planning
- Task management system with approval workflow

## Task Management

The StarOps Agent includes a task management system that provides tracking and approval for deployment tasks:

### Task Commands

- `task list` - List all your tasks
- `task get <task-id>` - View details of a specific task
- `task approve <task-id>` - Approve a task's execution plan
- `task reject <task-id> <feedback>` - Reject a task's plan with feedback for improvement

### Task Workflow

1. Create a task by asking the agent to deploy something (e.g., "deploy llama 3.3 on kserve")
2. The agent will create a task and provide you with the task ID
3. Check the status with `task get <task-id>`
4. Once planning is complete, review the plan and either:
   - Approve it with `task approve <task-id>`
   - Reject it with `task reject <task-id> The plan needs more security measures`
5. If approved, the task will be executed
6. If rejected, the task will be replanned with your feedback

### Task Statuses

- `pending` - Task has been created but planning hasn't started
- `planning` - Task plan is being created
- `awaiting_approval` - Plan is ready for your review
- `executing` - Plan is being executed
- `completed` - Task has been completed successfully
- `failed` - Task has failed

## Usage

1. Run the application:
   ```
   go run main.go
   ```

2. Ask the agent to deploy something:
   ```
   Enter your query: deploy llama 3.3 on kserve
   ```

3. Follow the task workflow described above to manage your deployment.

## Development

The StarOps Agent is built with a modular architecture:

- `internal/agent` - Agent implementation and factory
- `internal/planning` - Deployment planning and project management
- `internal/services` - Task management and other services
- `internal/models` - Data models for tasks and plans
- `internal/conversation` - Session handling and user interaction
- `internal/prompts` - System prompts for different agents

## License

Copyright © 2025 Ingenimax Inc.
