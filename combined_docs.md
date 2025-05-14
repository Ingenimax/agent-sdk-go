# Aggregated Markdown Documentation

*Generated on: combined_docs.md*

##

### File: CONTRIBUTING.md

*Path: CONTRIBUTING.md*

#### Contributing to the Agent SDK

Thank you for your interest in contributing to the Agent SDK! This document provides guidelines and instructions for contributing to this project.

##### Code of Conduct

Please be respectful to all contributors and users. We aim to foster an open and welcoming environment.

##### Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** to your local machine
3. **Create a branch** for your changes
4. **Make your changes** and test them
5. **Push your branch** to your fork
6. **Create a pull request**

##### Development Environment

1. Install Go (version 1.21 or later recommended)
2. Set up your IDE with Go support (GoLand, VSCode with Go extensions, etc.)
3. Install required dependencies:
   ```bash
   go mod download
   ```

##### Code Style

We follow standard Go code style and conventions:

1. Use `gofmt` or `goimports` to format your code
2. Follow [Effective Go](https://golang.org/doc/effective_go) guidelines
3. Document all exported types, functions, and methods

##### Testing

Please include tests for any new functionality or bug fixes:

1. Unit tests should be added for all new functions and methods
2. Integration tests should be added for significant components
3. Run tests before submitting a pull request:
   ```bash
   go test ./...
   ```

##### Pull Request Process

1. Ensure your code passes all tests and linting checks
2. Update documentation to reflect any changes
3. Include a clear description of the changes in your pull request
4. Reference any related issues in your pull request

##### Adding New Features

When adding new features, please follow these guidelines:

1. **Discuss before implementing**: Open an issue to discuss significant new features before implementing them
2. **Be consistent**: Follow the existing architecture and patterns
3. **Documentation**: Add documentation for all new features
4. **Examples**: Add examples showing how to use new features

##### Reporting Bugs

When reporting bugs, please include:

1. A clear description of the issue
2. Steps to reproduce the bug
3. Expected and actual behavior
4. Environment details (Go version, OS, etc.)

##### License

By contributing to this project, you agree that your contributions will be licensed under the project's license.

Thank you for contributing to the Agent SDK!

---

### File: README.md

*Path: README.md*

#### ![Ingenimax](/docs/img/logo-header.png#gh-light-mode-only) ![Ingenimax](/docs/img/logo-header-inverted.png#gh-dark-mode-only)

#### Agent Go SDK

A Go-based SDK for building AI agents with various capabilities like memory, tools, LLM integration, and more.

##### Features

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

##### Getting Started

###### Prerequisites

- Go 1.23+
- Redis (optional, for distributed memory)

###### Installation

Add the SDK to your Go project:

```bash
go get github.com/Ingenimax/agent-sdk-go
```

###### Configuration

The SDK uses environment variables for configuration. Key variables include:

- `OPENAI_API_KEY`: Your OpenAI API key
- `OPENAI_MODEL`: The model to use (e.g., gpt-4o-mini)
- `LOG_LEVEL`: Logging level (debug, info, warn, error)
- `REDIS_ADDRESS`: Redis server address (if using Redis for memory)

See `.env.example` for a complete list of configuration options.

##### Usage Examples

###### Creating a Simple Agent

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

###### Creating an Agent with YAML Configuration

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

###### Auto-Generating Agent Configurations

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

###### Using Execution Plans with Approval

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

##### Architecture

The SDK follows a modular architecture with these key components:

- **Agent**: Coordinates the LLM, memory, and tools
- **LLM**: Interface to language model providers
- **Memory**: Stores conversation history and context
- **Tools**: Extend the agent's capabilities
- **Vector Store**: For semantic search and retrieval
- **Guardrails**: Ensures safe and responsible AI usage
- **Execution Plan**: Manages planning, approval, and execution of complex tasks
- **Configuration**: YAML-based agent and task definitions

##### Examples

Check out the `cmd/examples` directory for complete examples:

- **Simple Agent**: Basic agent with system prompt
- **YAML Configuration**: Defining agents and tasks in YAML
- **Auto-Configuration**: Generating agent configurations from system prompts
- **Agent Config Wizard**: Interactive CLI for creating and using agents

##### License

This project is licensed under the MIT License - see the LICENSE file for details.


---

## docs

### File: agent.md

*Path: docs/agent.md*

#### Agent

This document explains how to use the Agent component of the Agent SDK.

##### Overview

The Agent is the core component of the SDK that coordinates the LLM, memory, and tools to create an intelligent assistant that can understand and respond to user queries.

##### Creating an Agent

To create a new agent, use the `NewAgent` function with various options:

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

##### Agent Options

The Agent can be configured with various options:

###### WithLLM

Sets the LLM provider for the agent:

```go
agent.WithLLM(openaiClient)
```

###### WithMemory

Sets the memory system for the agent:

```go
agent.WithMemory(memory.NewConversationBuffer())
```

###### WithTools

Adds tools to the agent:

```go
agent.WithTools(
    websearch.New(googleAPIKey, googleSearchEngineID),
    calculator.New(),
)
```

###### WithSystemPrompt

Sets the system prompt for the agent:

```go
agent.WithSystemPrompt("You are a helpful AI assistant specialized in answering questions about science.")
```

###### WithOrgID

Sets the organization ID for multi-tenancy:

```go
agent.WithOrgID("org-123")
```

###### WithTracer

Sets the tracer for observability:

```go
agent.WithTracer(langfuse.New(langfuseSecretKey, langfusePublicKey))
```

###### WithGuardrails

Sets the guardrails for safety:

```go
agent.WithGuardrails(guardrails.New(guardrailsConfigPath))
```

##### Running the Agent

To run the agent with a user query:

```go
response, err := agent.Run(ctx, "What is the capital of France?")
if err != nil {
    log.Fatalf("Failed to run agent: %v", err)
}
fmt.Println(response)
```

##### Streaming Responses

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

##### Using Tools

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

##### Advanced Usage

###### Custom Tool Execution

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

###### Custom Message Processing

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

##### Example: Complete Agent Setup

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

---

### File: environment_variables.md

*Path: docs/environment_variables.md*

#### Environment Variables

This document lists all environment variables used by the Agent SDK.

##### LLM Configuration

###### OpenAI

- `OPENAI_API_KEY`: API key for OpenAI
- `OPENAI_MODEL`: Model to use (default: "gpt-4o-mini")
- `OPENAI_TEMPERATURE`: Temperature for generation (default: 0.7)
- `OPENAI_MAX_TOKENS`: Maximum tokens to generate (default: 2048)
- `OPENAI_BASE_URL`: Base URL for API calls (default: "https://api.openai.com/v1")
- `OPENAI_TIMEOUT_SECONDS`: Timeout in seconds (default: 60)

###### Anthropic

- `ANTHROPIC_API_KEY`: API key for Anthropic
- `ANTHROPIC_MODEL`: Model to use (default: "claude-3-haiku-20240307")
- `ANTHROPIC_TEMPERATURE`: Temperature for generation (default: 0.7)
- `ANTHROPIC_MAX_TOKENS`: Maximum tokens to generate (default: 2048)
- `ANTHROPIC_BASE_URL`: Base URL for API calls (default: "https://api.anthropic.com")
- `ANTHROPIC_TIMEOUT_SECONDS`: Timeout in seconds (default: 60)

###### Azure OpenAI

- `AZURE_OPENAI_API_KEY`: API key for Azure OpenAI
- `AZURE_OPENAI_ENDPOINT`: Endpoint for Azure OpenAI
- `AZURE_OPENAI_DEPLOYMENT`: Deployment name
- `AZURE_OPENAI_API_VERSION`: API version (default: "2023-05-15")
- `AZURE_OPENAI_TEMPERATURE`: Temperature for generation (default: 0.7)
- `AZURE_OPENAI_MAX_TOKENS`: Maximum tokens to generate (default: 2048)
- `AZURE_OPENAI_TIMEOUT_SECONDS`: Timeout in seconds (default: 60)

##### Memory Configuration

###### Redis

- `REDIS_URL`: Redis URL (default: "localhost:6379")
- `REDIS_PASSWORD`: Redis password
- `REDIS_DB`: Redis database number (default: 0)

##### VectorStore Configuration

###### Weaviate

- `WEAVIATE_URL`: Weaviate URL
- `WEAVIATE_API_KEY`: Weaviate API key
- `WEAVIATE_SCHEME`: Weaviate scheme (default: "http")
- `WEAVIATE_HOST`: Weaviate host (default: "localhost:8080")
- `WEAVIATE_CLASS_NAME`: Weaviate class name (default: "Document")

###### Pinecone

- `PINECONE_API_KEY`: Pinecone API key
- `PINECONE_ENVIRONMENT`: Pinecone environment
- `PINECONE_INDEX`: Pinecone index name

##### DataStore Configuration

###### Supabase

- `SUPABASE_URL`: Supabase URL
- `SUPABASE_API_KEY`: Supabase API key
- `SUPABASE_TABLE`: Supabase table name (default: "documents")

##### Tools Configuration

###### Web Search

- `GOOGLE_API_KEY`: Google API key for web search
- `GOOGLE_SEARCH_ENGINE_ID`: Google Search Engine ID

###### AWS

- `AWS_ACCESS_KEY_ID`: AWS access key ID
- `AWS_SECRET_ACCESS_KEY`: AWS secret access key
- `AWS_REGION`: AWS region (default: "us-east-1")

###### Kubernetes

- `KUBECONFIG`: Path to kubeconfig file
- `KUBE_CONTEXT`: Kubernetes context to use

##### Tracing Configuration

###### Langfuse

- `LANGFUSE_ENABLED`: Enable Langfuse tracing (default: false)
- `LANGFUSE_SECRET_KEY`: Langfuse secret key
- `LANGFUSE_PUBLIC_KEY`: Langfuse public key
- `LANGFUSE_HOST`: Langfuse host (default: "https://cloud.langfuse.com")
- `LANGFUSE_ENVIRONMENT`: Environment name (default: "development")

###### OpenTelemetry

- `OTEL_ENABLED`: Enable OpenTelemetry tracing (default: false)
- `OTEL_SERVICE_NAME`: Service name (default: "agent-sdk")
- `OTEL_COLLECTOR_ENDPOINT`: Collector endpoint (default: "localhost:4317")

##### Multitenancy Configuration

- `MULTITENANCY_ENABLED`: Enable multitenancy (default: false)
- `DEFAULT_ORG_ID`: Default organization ID (default: "default")

##### Guardrails Configuration

- `GUARDRAILS_ENABLED`: Enable guardrails (default: false)
- `GUARDRAILS_CONFIG_PATH`: Path to guardrails configuration file

---

### File: execution_plan.md

*Path: docs/execution_plan.md*

#### Execution Plan Package

The `executionplan` package provides a structured way to create, modify, and execute plans for complex tasks. This package enables transparency and control over actions that an AI agent might take by presenting a plan to the user for approval before execution.

##### Overview

The execution plan system consists of several key components:

1. **ExecutionPlan**: A struct representing a plan with steps to execute
2. **Generator**: Creates and modifies execution plans
3. **Executor**: Executes approved plans
4. **Store**: Manages storage and retrieval of plans

This architecture allows for a clear separation of concerns and greater flexibility in how plans are created, stored, and executed.

##### Key Concepts

###### ExecutionPlan

An `ExecutionPlan` represents a set of steps that need to be executed to accomplish a task. Each plan has:

- A list of execution steps
- A high-level description
- A unique task ID
- A status (draft, pending approval, approved, executing, completed, failed, cancelled)
- Timestamps for creation and updates
- A flag indicating whether the user has approved the plan

###### ExecutionStep

Each `ExecutionStep` represents a single action within a plan. Steps contain:

- The name of the tool to execute
- Input for the tool
- A description of what the step will accomplish
- Parameters for the tool execution

###### Plan Status

An execution plan can be in one of the following statuses:

- `StatusDraft`: The plan is in a draft state and not yet ready for approval
- `StatusPendingApproval`: The plan is waiting for user approval
- `StatusApproved`: The plan has been approved by the user
- `StatusExecuting`: The plan is currently being executed
- `StatusCompleted`: The plan has been successfully executed
- `StatusFailed`: The plan execution failed
- `StatusCancelled`: The plan was cancelled by the user

##### Using the Package

###### Creating a Generator

The `Generator` is responsible for creating and modifying execution plans:

```go
// Create a generator
generator := executionplan.NewGenerator(
    llmClient,  // An LLM implementation
    tools,      // List of available tools
    systemPrompt, // Optional system prompt for the LLM
)

// Generate an execution plan
plan, err := generator.GenerateExecutionPlan(ctx, userInput)
if err != nil {
    // Handle error
}

// Modify an execution plan based on user feedback
modifiedPlan, err := generator.ModifyExecutionPlan(ctx, plan, userFeedback)
if err != nil {
    // Handle error
}
```

###### Creating an Executor

The `Executor` is responsible for executing approved plans:

```go
// Create an executor
executor := executionplan.NewExecutor(tools)

// Execute an approved plan
result, err := executor.ExecutePlan(ctx, approvedPlan)
if err != nil {
    // Handle error
}

// Cancel a plan
executor.CancelPlan(plan)

// Get plan status
status := executor.GetPlanStatus(plan)
```

###### Using the Store

The `Store` is responsible for storing and retrieving plans:

```go
// Create a store
store := executionplan.NewStore()

// Store a plan
store.StorePlan(plan)

// Get a plan by task ID
plan, exists := store.GetPlanByTaskID(taskID)

// List all plans
plans := store.ListPlans()

// Delete a plan
deleted := store.DeletePlan(taskID)
```

###### Formatting Plans for Display

The package provides a function to format plans for display:

```go
// Format a plan for display
formattedPlan := executionplan.FormatExecutionPlan(plan)
fmt.Println(formattedPlan)
```

##### Integration with Agents

The most common use case is to integrate the execution plan package with the Agent SDK:

```go
// Create an agent with execution plan support
agent, err := agent.NewAgent(
    agent.WithLLM(llmClient),
    agent.WithTools(tools...),
    agent.WithRequirePlanApproval(true), // Enable execution plan workflow
)

// Generate a plan
plan, err := agent.GenerateExecutionPlan(ctx, userInput)

// Modify a plan
modifiedPlan, err := agent.ModifyExecutionPlan(ctx, plan, userFeedback)

// Approve and execute a plan
result, err := agent.ApproveExecutionPlan(ctx, plan)
```

##### Advanced Customization

###### Custom Plan Generation

You can implement custom plan generation by creating your own `Generator` implementation:

```go
type CustomGenerator struct {
    // Your fields here
}

func (g *CustomGenerator) GenerateExecutionPlan(ctx context.Context, input string) (*executionplan.ExecutionPlan, error) {
    // Your custom implementation
}
```

###### Custom Plan Execution

Similarly, you can implement custom plan execution:

```go
type CustomExecutor struct {
    // Your fields here
}

func (e *CustomExecutor) ExecutePlan(ctx context.Context, plan *executionplan.ExecutionPlan) (string, error) {
    // Your custom implementation
}
```

##### Best Practices

1. **Clear Step Descriptions**: Ensure each step has a clear, human-readable description
2. **Appropriate Granularity**: Break complex tasks into appropriate steps - not too many, not too few
3. **Parameter Validation**: Validate parameters before executing steps
4. **Error Handling**: Implement proper error handling during execution
5. **User Feedback**: Provide clear feedback to users during the approval process
6. **Security**: Consider security implications of each step before execution

##### Example

Here's a complete example of using the execution plan package:

```go
package main

import (
    "context"
    "fmt"

    "github.com/Ingenimax/agent-sdk-go/pkg/executionplan"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
    // Create an LLM client
    llmClient := openai.NewClient("your-api-key")

    // Create some tools
    tools := []interfaces.Tool{
        // Your tools here
    }

    // Create a generator
    generator := executionplan.NewGenerator(llmClient, tools, "You are a helpful assistant")

    // Generate a plan
    plan, err := generator.GenerateExecutionPlan(context.Background(), "Deploy a web application")
    if err != nil {
        panic(err)
    }

    // Format the plan for display
    formattedPlan := executionplan.FormatExecutionPlan(plan)
    fmt.Println(formattedPlan)

    // Create an executor
    executor := executionplan.NewExecutor(tools)

    // Execute the plan (after user approval)
    plan.UserApproved = true
    result, err := executor.ExecutePlan(context.Background(), plan)
    if err != nil {
        panic(err)
    }

    fmt.Println(result)
}
```

---

### File: guardrails.md

*Path: docs/guardrails.md*

#### Guardrails

This document explains how to use the Guardrails component of the Agent SDK.

##### Overview

Guardrails provide safety mechanisms to ensure that your agents behave responsibly and ethically. They can filter, modify, or block responses that violate policies or contain harmful content.

##### Enabling Guardrails

To enable guardrails, set the `GUARDRAILS_ENABLED` environment variable to `true`:

```bash
export GUARDRAILS_ENABLED=true
```

You can also specify a configuration file:

```bash
export GUARDRAILS_CONFIG_PATH=/path/to/guardrails.yaml
```

##### Using Guardrails with an Agent

To use guardrails with an agent, pass them to the `WithGuardrails` option:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/guardrails"
)

// Create guardrails
gr := guardrails.New(guardrails.WithConfigPath("/path/to/guardrails.yaml"))

// Create agent with guardrails
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithGuardrails(gr),
)
```

##### Guardrails Configuration

Guardrails are configured using a YAML file. Here's an example configuration:

```yaml
#### guardrails.yaml
version: 1
rules:
  - name: no_harmful_content
    description: Block harmful content
    patterns:
      - type: regex
        pattern: "(?i)(how to (make|create|build) (a )?(bomb|explosive|weapon))"
    action: block
    message: "I cannot provide information on creating harmful devices."

  - name: no_personal_data
    description: Redact personal data
    patterns:
      - type: regex
        pattern: "(?i)\\b\\d{3}-\\d{2}-\\d{4}\\b"  # SSN
      - type: regex
        pattern: "(?i)\\b\\d{16}\\b"  # Credit card
    action: redact
    replacement: "[REDACTED]"

  - name: no_profanity
    description: Filter profanity
    patterns:
      - type: wordlist
        words: ["badword1", "badword2", "badword3"]
    action: filter
    replacement: "****"

  - name: topic_restriction
    description: Restrict to certain topics
    topics:
      allowed: ["technology", "science", "education"]
      blocked: ["politics", "religion", "adult"]
    action: block
    message: "I can only discuss technology, science, and education topics."
```

##### Rule Types

###### Regex Rules

Regex rules match patterns using regular expressions:

```yaml
- name: no_email_addresses
  description: Redact email addresses
  patterns:
    - type: regex
      pattern: "(?i)\\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}\\b"
  action: redact
  replacement: "[EMAIL REDACTED]"
```

###### Wordlist Rules

Wordlist rules match specific words or phrases:

```yaml
- name: no_profanity
  description: Filter profanity
  patterns:
    - type: wordlist
      words: ["badword1", "badword2", "badword3"]
  action: filter
  replacement: "****"
```

###### Topic Rules

Topic rules restrict or allow certain topics:

```yaml
- name: topic_restriction
  description: Restrict to certain topics
  topics:
    allowed: ["technology", "science", "education"]
    blocked: ["politics", "religion", "adult"]
  action: block
  message: "I can only discuss technology, science, and education topics."
```

###### Semantic Rules

Semantic rules use embeddings to detect semantic similarity:

```yaml
- name: no_harmful_instructions
  description: Block harmful instructions
  semantic:
    examples:
      - "How to hack into a computer"
      - "How to steal someone's identity"
      - "How to make a dangerous chemical"
    threshold: 0.8
  action: block
  message: "I cannot provide potentially harmful instructions."
```

##### Actions

###### Block

The `block` action prevents the response from being sent and returns a custom message:

```yaml
action: block
message: "I cannot provide that information."
```

###### Redact

The `redact` action replaces matched content with a replacement string:

```yaml
action: redact
replacement: "[REDACTED]"
```

###### Filter

The `filter` action replaces matched content with a replacement string but is typically used for less sensitive content:

```yaml
action: filter
replacement: "****"
```

###### Log

The `log` action logs the matched content but allows the response to be sent:

```yaml
action: log
```

##### Using Guardrails Programmatically

You can also use guardrails programmatically:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/guardrails"
)

// Create guardrails
gr := guardrails.New()

// Add a rule
gr.AddRule(&guardrails.Rule{
    Name:        "no_harmful_content",
    Description: "Block harmful content",
    Patterns: []guardrails.Pattern{
        {
            Type:    "regex",
            Pattern: "(?i)(how to (make|create|build) (a )?(bomb|explosive|weapon))",
        },
    },
    Action:  "block",
    Message: "I cannot provide information on creating harmful devices.",
})

// Check content
result, err := gr.Check(context.Background(), "How to make a bomb")
if err != nil {
    log.Fatalf("Failed to check content: %v", err)
}

if result.Blocked {
    fmt.Println("Content was blocked:", result.Message)
} else if result.Modified {
    fmt.Println("Content was modified:", result.Content)
} else {
    fmt.Println("Content passed guardrails:", result.Content)
}
```

##### Multi-tenancy with Guardrails

When using guardrails with multi-tenancy, you can have different guardrails for different organizations:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/guardrails"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create guardrails for different organizations
orgGuardrails := map[string]interfaces.Guardrails{
    "org-123": guardrails.New(guardrails.WithConfigPath("/path/to/org123-guardrails.yaml")),
    "org-456": guardrails.New(guardrails.WithConfigPath("/path/to/org456-guardrails.yaml")),
}

// Create a multi-tenant guardrails provider
gr := guardrails.NewMultiTenant(orgGuardrails, guardrails.New()) // Default guardrails as fallback

// Create agent with multi-tenant guardrails
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithGuardrails(gr),
)

// Create context with organization ID
ctx := context.Background()
ctx = multitenancy.WithOrgID(ctx, "org-123")

// The appropriate guardrails for org-123 will be used
response, err := agent.Run(ctx, "What is the capital of France?")
```

##### Creating Custom Guardrails

You can implement custom guardrails by implementing the `interfaces.Guardrails` interface:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// CustomGuardrails is a custom guardrails implementation
type CustomGuardrails struct {
    // Add your fields here
}

// NewCustomGuardrails creates a new custom guardrails
func NewCustomGuardrails() *CustomGuardrails {
    return &CustomGuardrails{}
}

// Check checks content against guardrails
func (g *CustomGuardrails) Check(ctx context.Context, content string) (*interfaces.GuardrailsResult, error) {
    // Implement your logic to check content

    // Example: Block content containing "forbidden"
    if strings.Contains(strings.ToLower(content), "forbidden") {
        return &interfaces.GuardrailsResult{
            Blocked: true,
            Message: "This content is not allowed.",
        }, nil
    }

    // Example: Redact email addresses
    emailRegex := regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
    if emailRegex.MatchString(content) {
        modified := emailRegex.ReplaceAllString(content, "[EMAIL REDACTED]")
        return &interfaces.GuardrailsResult{
            Modified: true,
            Content:  modified,
        }, nil
    }

    // Content passed guardrails
    return &interfaces.GuardrailsResult{
        Content: content,
    }, nil
}
```

##### Example: Complete Guardrails Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
    "github.com/Ingenimax/agent-sdk-go/pkg/guardrails"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create OpenAI client
    openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

    // Create guardrails
    gr := guardrails.New(
        guardrails.WithConfigPath(cfg.Guardrails.ConfigPath),
    )

    // Create a new agent with guardrails
    agent, err := agent.NewAgent(
        agent.WithLLM(openaiClient),
        agent.WithMemory(memory.NewConversationBuffer()),
        agent.WithGuardrails(gr),
        agent.WithSystemPrompt("You are a helpful AI assistant."),
    )
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Run the agent
    ctx := context.Background()

    // Safe query
    response1, err := agent.Run(ctx, "What is the capital of France?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }
    fmt.Println("Safe query response:", response1)

    // Potentially unsafe query (will be blocked or modified by guardrails)
    response2, err := agent.Run(ctx, "How do I hack into a computer?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }
    fmt.Println("Unsafe query response:", response2)
}

---

### File: llm.md

*Path: docs/llm.md*

#### LLM Providers

This document explains how to use the LLM (Large Language Model) providers in the Agent SDK.

##### Overview

The Agent SDK supports multiple LLM providers, including OpenAI, Anthropic, and Azure OpenAI. Each provider has its own implementation but shares a common interface.

##### Supported Providers

###### OpenAI

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create OpenAI client
client := openai.NewClient(cfg.LLM.OpenAI.APIKey)

// Optional: Configure the client
client = openai.NewClient(
    cfg.LLM.OpenAI.APIKey,
    openai.WithModel("gpt-4o-mini"),
    openai.WithTemperature(0.7),
)
```

###### Anthropic

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create Anthropic client
client := anthropic.NewClient(cfg.LLM.Anthropic.APIKey)

// Optional: Configure the client
client = anthropic.NewClient(
    cfg.LLM.Anthropic.APIKey,
    anthropic.WithModel("claude-3-opus-20240229"),
    anthropic.WithTemperature(0.7),
)
```

###### Azure OpenAI

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/azure"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create Azure OpenAI client
client := azure.NewClient(
    cfg.LLM.AzureOpenAI.APIKey,
    cfg.LLM.AzureOpenAI.Endpoint,
    cfg.LLM.AzureOpenAI.Deployment,
)

// Optional: Configure the client
client = azure.NewClient(
    cfg.LLM.AzureOpenAI.APIKey,
    cfg.LLM.AzureOpenAI.Endpoint,
    cfg.LLM.AzureOpenAI.Deployment,
    azure.WithAPIVersion("2023-05-15"),
    azure.WithTemperature(0.7),
)
```

##### Using LLM Providers

###### Text Generation

Generate text based on a prompt:

```go
import "context"

// Generate text
response, err := client.Generate(context.Background(), "What is the capital of France?")
if err != nil {
    log.Fatalf("Failed to generate text: %v", err)
}
fmt.Println(response)
```

###### Chat Completion

Generate a response to a conversation:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm"
)

// Create messages
messages := []llm.Message{
    {
        Role:    "system",
        Content: "You are a helpful AI assistant.",
    },
    {
        Role:    "user",
        Content: "What is the capital of France?",
    },
}

// Generate chat completion
response, err := client.Chat(context.Background(), messages)
if err != nil {
    log.Fatalf("Failed to generate chat completion: %v", err)
}
fmt.Println(response)
```

###### Generation with Tools

Generate a response that can use tools:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

// Create tools
searchTool := websearch.New(googleAPIKey, googleSearchEngineID)

// Generate with tools
response, err := client.GenerateWithTools(
    context.Background(),
    "What's the weather in San Francisco?",
    []interfaces.Tool{searchTool},
)
if err != nil {
    log.Fatalf("Failed to generate with tools: %v", err)
}
fmt.Println(response)
```

##### Configuration Options

###### Common Options

These options are available for all LLM providers:

```go
// Temperature controls randomness (0.0 to 1.0)
WithTemperature(0.7)

// TopP controls diversity via nucleus sampling
WithTopP(0.9)

// FrequencyPenalty reduces repetition of token sequences
WithFrequencyPenalty(0.0)

// PresencePenalty reduces repetition of topics
WithPresencePenalty(0.0)

// StopSequences specifies sequences that stop generation
WithStopSequences([]string{"###"})
```

###### Provider-Specific Options

####### OpenAI

```go
// Model specifies which model to use
openai.WithModel("gpt-4")

// BaseURL specifies a custom API endpoint
openai.WithBaseURL("https://api.openai.com/v1")

// Timeout specifies the request timeout
openai.WithTimeout(60 * time.Second)
```

####### Anthropic

```go
// Model specifies which model to use
anthropic.WithModel("claude-3-opus-20240229")

// BaseURL specifies a custom API endpoint
anthropic.WithBaseURL("https://api.anthropic.com")

// Timeout specifies the request timeout
anthropic.WithTimeout(60 * time.Second)
```

####### Azure OpenAI

```go
// APIVersion specifies the API version to use
azure.WithAPIVersion("2023-05-15")

// Timeout specifies the request timeout
azure.WithTimeout(60 * time.Second)
```

##### Multi-tenancy with LLM Providers

When using LLM providers with multi-tenancy, you can specify the organization ID:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create context with organization ID
ctx := context.Background()
ctx = multitenancy.WithOrgID(ctx, "org-123")

// Generate text for this organization
response, err := client.Generate(ctx, "What is the capital of France?")
```

##### Implementing Custom LLM Providers

You can implement custom LLM providers by implementing the `interfaces.LLM` interface:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// CustomLLM is a custom LLM implementation
type CustomLLM struct {
    // Add your fields here
}

// NewCustomLLM creates a new custom LLM
func NewCustomLLM() *CustomLLM {
    return &CustomLLM{}
}

// Generate generates text based on the provided prompt
func (l *CustomLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
    // Apply options
    opts := &interfaces.GenerateOptions{}
    for _, option := range options {
        option(opts)
    }

    // Implement your generation logic here
    return "Generated text", nil
}

// GenerateWithTools generates text and can use tools
func (l *CustomLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
    // Apply options
    opts := &interfaces.GenerateOptions{}
    for _, option := range options {
        option(opts)
    }

    // Implement your generation with tools logic here
    return "Generated text with tools", nil
}

// Name returns the name of the LLM provider
func (l *CustomLLM) Name() string {
    return "custom-llm"
}
```

##### Example: Complete LLM Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/config"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/azure"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create context
    ctx := context.Background()

    // Create LLM client based on configuration
    var client llm.Provider
    switch cfg.LLM.Provider {
    case "openai":
        client = openai.NewClient(
            cfg.LLM.OpenAI.APIKey,
            openai.WithModel(cfg.LLM.OpenAI.Model),
            openai.WithTemperature(cfg.LLM.OpenAI.Temperature),
        )
    case "anthropic":
        client = anthropic.NewClient(
            cfg.LLM.Anthropic.APIKey,
            anthropic.WithModel(cfg.LLM.Anthropic.Model),
            anthropic.WithTemperature(cfg.LLM.Anthropic.Temperature),
        )
    case "azure":
        client = azure.NewClient(
            cfg.LLM.AzureOpenAI.APIKey,
            cfg.LLM.AzureOpenAI.Endpoint,
            cfg.LLM.AzureOpenAI.Deployment,
            azure.WithAPIVersion(cfg.LLM.AzureOpenAI.APIVersion),
            azure.WithTemperature(cfg.LLM.AzureOpenAI.Temperature),
        )
    default:
        log.Fatalf("Unsupported LLM provider: %s", cfg.LLM.Provider)
    }

    // Generate text
    response, err := client.Generate(ctx, "What is the capital of France?")
    if err != nil {
        log.Fatalf("Failed to generate text: %v", err)
    }
    fmt.Println("Generated text:", response)

    // Generate chat completion
    messages := []llm.Message{
        {
            Role:    "system",
            Content: "You are a helpful AI assistant.",
        },
        {
            Role:    "user",
            Content: "What is the capital of Germany?",
        },
    }
    chatResponse, err := client.Chat(ctx, messages)
    if err != nil {
        log.Fatalf("Failed to generate chat completion: %v", err)
    }
    fmt.Println("Chat response:", chatResponse)
}

---

### File: memory.md

*Path: docs/memory.md*

#### Memory

This document explains how to use the Memory component of the Agent SDK.

##### Overview

Memory allows an agent to remember previous interactions and maintain context across multiple turns of conversation. The Agent SDK provides several memory implementations to suit different needs.

##### Memory Types

###### Conversation Buffer

The simplest memory type that stores all messages in a buffer:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/memory"

// Create a conversation buffer memory
mem := memory.NewConversationBuffer()
```

###### Conversation Buffer Window

Stores only the most recent N messages:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/memory"

// Create a conversation buffer window memory with a window size of 10 messages
mem := memory.NewConversationBufferWindow(10)
```

###### Redis Memory

Stores messages in Redis for persistence:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/memory/redis"

// Create a Redis memory
mem := redis.New(
    "localhost:6379", // Redis URL
    "",               // Redis password (empty for no password)
    0,                // Redis database number
)
```

##### Using Memory with an Agent

To use memory with an agent, pass it to the `WithMemory` option:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// Create memory
mem := memory.NewConversationBuffer()

// Create agent with memory
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(mem),
)
```

##### Working with Messages

###### Adding Messages

You can add messages to memory directly:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// Create memory
mem := memory.NewConversationBuffer()

// Create context
ctx := context.Background()

// Add a user message
err := mem.AddMessage(ctx, interfaces.Message{
    Role:    "user",
    Content: "Hello, how are you?",
})
if err != nil {
    log.Fatalf("Failed to add message: %v", err)
}

// Add an assistant message
err = mem.AddMessage(ctx, interfaces.Message{
    Role:    "assistant",
    Content: "I'm doing well, thank you! How can I help you today?",
})
if err != nil {
    log.Fatalf("Failed to add message: %v", err)
}
```

###### Retrieving Messages

You can retrieve messages from memory:

```go
// Get all messages
messages, err := mem.GetMessages(ctx)
if err != nil {
    log.Fatalf("Failed to get messages: %v", err)
}

// Get only user messages
userMessages, err := mem.GetMessages(ctx, interfaces.WithRoles("user"))
if err != nil {
    log.Fatalf("Failed to get user messages: %v", err)
}

// Get the last 5 messages
recentMessages, err := mem.GetMessages(ctx, interfaces.WithLimit(5))
if err != nil {
    log.Fatalf("Failed to get recent messages: %v", err)
}
```

###### Clearing Memory

You can clear all messages from memory:

```go
err := mem.Clear(ctx)
if err != nil {
    log.Fatalf("Failed to clear memory: %v", err)
}
```

##### Multi-tenancy with Memory

When using memory with multi-tenancy, you need to include the organization ID in the context:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create context with organization ID
ctx := context.Background()
ctx = multitenancy.WithOrgID(ctx, "org-123")

// Add a message for this organization
err := mem.AddMessage(ctx, interfaces.Message{
    Role:    "user",
    Content: "Hello from org-123",
})

// Switch to a different organization
ctx = multitenancy.WithOrgID(context.Background(), "org-456")

// Add a message for the other organization
err = mem.AddMessage(ctx, interfaces.Message{
    Role:    "user",
    Content: "Hello from org-456",
})

// Messages are isolated by organization
```

##### Conversation IDs

You can use conversation IDs to manage multiple conversations:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// Create context with conversation ID
ctx := context.Background()
ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-123")

// Add a message to this conversation
err := mem.AddMessage(ctx, interfaces.Message{
    Role:    "user",
    Content: "Hello from conversation 123",
})

// Switch to a different conversation
ctx = context.WithValue(context.Background(), memory.ConversationIDKey, "conversation-456")

// Add a message to the other conversation
err = mem.AddMessage(ctx, interfaces.Message{
    Role:    "user",
    Content: "Hello from conversation 456",
})

// Messages are isolated by conversation ID
```

##### Creating Custom Memory Implementations

You can create custom memory implementations by implementing the `interfaces.Memory` interface:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// CustomMemory is a custom memory implementation
type CustomMemory struct {
    messages map[string][]interfaces.Message
}

// NewCustomMemory creates a new custom memory
func NewCustomMemory() *CustomMemory {
    return &CustomMemory{
        messages: make(map[string][]interfaces.Message),
    }
}

// AddMessage adds a message to memory
func (m *CustomMemory) AddMessage(ctx context.Context, message interfaces.Message) error {
    // Get conversation ID from context
    convID := getConversationID(ctx)

    // Add message to the conversation
    m.messages[convID] = append(m.messages[convID], message)

    return nil
}

// GetMessages retrieves messages from memory
func (m *CustomMemory) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
    // Get conversation ID from context
    convID := getConversationID(ctx)

    // Apply options
    opts := &interfaces.GetMessagesOptions{}
    for _, option := range options {
        option(opts)
    }

    // Get messages for the conversation
    messages := m.messages[convID]

    // Apply limit if specified
    if opts.Limit > 0 && opts.Limit < len(messages) {
        start := len(messages) - opts.Limit
        messages = messages[start:]
    }

    // Filter by role if specified
    if len(opts.Roles) > 0 {
        filtered := make([]interfaces.Message, 0)
        for _, msg := range messages {
            for _, role := range opts.Roles {
                if msg.Role == role {
                    filtered = append(filtered, msg)
                    break
                }
            }
        }
        messages = filtered
    }

    return messages, nil
}

// Clear clears the memory
func (m *CustomMemory) Clear(ctx context.Context) error {
    // Get conversation ID from context
    convID := getConversationID(ctx)

    // Clear messages for the conversation
    delete(m.messages, convID)

    return nil
}

// Helper function to get conversation ID from context
func getConversationID(ctx context.Context) string {
    // Get organization ID
    orgID := "default"
    if id := ctx.Value(multitenancy.OrgIDKey); id != nil {
        if s, ok := id.(string); ok {
            orgID = s
        }
    }

    // Get conversation ID
    convID := "default"
    if id := ctx.Value(memory.ConversationIDKey); id != nil {
        if s, ok := id.(string); ok {
            convID = s
        }
    }

    // Combine org ID and conversation ID
    return orgID + ":" + convID
}
```

##### Example: Complete Memory Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory/redis"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create OpenAI client
    openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

    // Create memory
    var mem interfaces.Memory
    if cfg.Memory.Redis.URL != "" {
        // Use Redis memory if configured
        mem = redis.New(
            cfg.Memory.Redis.URL,
            cfg.Memory.Redis.Password,
            cfg.Memory.Redis.DB,
        )
    } else {
        // Fall back to in-memory buffer
        mem = memory.NewConversationBuffer()
    }

    // Create a new agent with memory
    agent, err := agent.NewAgent(
        agent.WithLLM(openaiClient),
        agent.WithMemory(mem),
        agent.WithSystemPrompt("You are a helpful AI assistant."),
    )
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Create context with organization ID and conversation ID
    ctx := context.Background()
    ctx = multitenancy.WithOrgID(ctx, "org-123")
    ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-123")

    // Run the agent with the first query
    response1, err := agent.Run(ctx, "Hello, who are you?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }
    fmt.Println("Response 1:", response1)

    // Run the agent with a follow-up query (memory will be used)
    response2, err := agent.Run(ctx, "What did I just ask you?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }
    fmt.Println("Response 2:", response2)
}

---

### File: multitenancy.md

*Path: docs/multitenancy.md*

#### Multitenancy

This document explains how to use the multitenancy features of the Agent SDK.

##### Overview

Multitenancy allows you to use a single instance of the Agent SDK to serve multiple organizations or users, with isolated data and configurations for each tenant.

##### Enabling Multitenancy

To enable multitenancy, set the `MULTITENANCY_ENABLED` environment variable to `true`:

```bash
export MULTITENANCY_ENABLED=true
```

You can also set a default organization ID with the `DEFAULT_ORG_ID` environment variable:

```bash
export DEFAULT_ORG_ID=my-default-org
```

##### Using Multitenancy in Your Code

###### Setting the Organization ID in Context

To specify which organization's resources to use, add the organization ID to the context:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create a context with an organization ID
ctx := context.Background()
ctx = multitenancy.WithOrgID(ctx, "org-123")

// Use this context when calling agent methods
response, err := agent.Run(ctx, "What is the capital of France?")
```

###### Getting the Organization ID from Context

To retrieve the organization ID from a context:

```go
orgID := multitenancy.GetOrgID(ctx)
```

If no organization ID is set in the context, this will return the default organization ID.

##### Multitenancy with Different Components

###### LLM Providers

Each organization can have its own LLM configuration:

```go
// Create an LLM provider for a specific organization
llmProvider := openai.NewClient(
    cfg.LLM.OpenAI.APIKey,
    openai.WithOrgID("org-123"),
)
```

###### Memory

Memory is automatically isolated by organization ID:

```go
// Create a memory system
mem := memory.NewConversationBuffer()

// When using the memory with a context that has an organization ID,
// the data will be isolated to that organization
agent.WithMemory(mem)
```

###### Vector Stores

Vector stores can be partitioned by organization:

```go
// Create a vector store with organization isolation
vectorStore := weaviate.New(
    cfg.VectorStore.Weaviate.URL,
    weaviate.WithOrgID("org-123"),
)
```

###### Data Stores

Data stores can also be isolated by organization:

```go
// Create a data store with organization isolation
dataStore := supabase.New(
    cfg.DataStore.Supabase.URL,
    cfg.DataStore.Supabase.APIKey,
    supabase.WithOrgID("org-123"),
)
```

##### Best Practices

1. **Always use contexts**: Pass the context with the organization ID to all methods that accept a context.

2. **Set default organization ID**: Always set a default organization ID to handle cases where no organization ID is provided.

3. **Validate organization IDs**: Implement validation to ensure that organization IDs are valid and that users have access to the requested organization.

4. **Audit access**: Log organization ID access for security auditing.

##### Example: Complete Multitenancy Setup

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
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create OpenAI client
    openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

    // Create a new agent
    agent, err := agent.NewAgent(
        agent.WithLLM(openaiClient),
        agent.WithMemory(memory.NewConversationBuffer()),
        agent.WithSystemPrompt("You are a helpful AI assistant."),
    )
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Create a context with organization ID
    ctx := context.Background()
    ctx = multitenancy.WithOrgID(ctx, "org-123")

    // Run the agent with the organization context
    response, err := agent.Run(ctx, "What is the capital of France?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }

    fmt.Println(response)

    // Switch to a different organization
    ctx = multitenancy.WithOrgID(context.Background(), "org-456")

    // Run the agent with the new organization context
    response, err = agent.Run(ctx, "What is the capital of Germany?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }

    fmt.Println(response)
}
```


---

### File: task.md

*Path: docs/task.md*

#### Task Execution Package

The task execution package provides functionality for executing tasks synchronously and asynchronously, including API calls and Temporal workflows.

##### Features

- Execute tasks synchronously and asynchronously
- Built-in retry mechanism with configurable retry policies
- API client for making HTTP requests
- Temporal workflow integration
- Task cancellation and status tracking

##### Usage

###### Basic Task Execution

```go
// Create a task executor
executor := agentsdk.NewTaskExecutor()

// Register a task
executor.RegisterTask("hello", func(ctx context.Context, params interface{}) (interface{}, error) {
    name, ok := params.(string)
    if !ok {
        name = "World"
    }
    return fmt.Sprintf("Hello, %s!", name), nil
})

// Execute the task synchronously
result, err := executor.ExecuteSync(context.Background(), "hello", "John", nil)
if err != nil {
    fmt.Printf("Error: %v\n", err)
} else {
    fmt.Printf("Result: %v\n", result.Data)
}

// Execute the task asynchronously
resultChan, err := executor.ExecuteAsync(context.Background(), "hello", "Jane", nil)
if err != nil {
    fmt.Printf("Error: %v\n", err)
} else {
    result := <-resultChan
    fmt.Printf("Result: %v\n", result.Data)
}
```

###### API Task Execution

```go
// Create an API client
apiClient := agentsdk.NewAPIClient("https://api.example.com", 10*time.Second)

// Register an API task
executor.RegisterTask("get_data", agentsdk.APITask(apiClient, task.APIRequest{
    Method: "GET",
    Path:   "/data",
    Query:  map[string]string{"limit": "10"},
}))

// Execute the API task with retry policy
timeout := 5 * time.Second
retryPolicy := &interfaces.RetryPolicy{
    MaxRetries:        3,
    InitialBackoff:    100 * time.Millisecond,
    MaxBackoff:        1 * time.Second,
    BackoffMultiplier: 2.0,
}

result, err := executor.ExecuteSync(context.Background(), "get_data", nil, &interfaces.TaskOptions{
    Timeout:     &timeout,
    RetryPolicy: retryPolicy,
})
```

###### Temporal Workflow Execution

```go
// Create a Temporal client
temporalClient := agentsdk.NewTemporalClient(task.TemporalConfig{
    HostPort:                 "localhost:7233",
    Namespace:                "default",
    TaskQueue:                "example",
    WorkflowIDPrefix:         "example-",
    WorkflowExecutionTimeout: 10 * time.Minute,
    WorkflowRunTimeout:       5 * time.Minute,
    WorkflowTaskTimeout:      10 * time.Second,
})

// Register a Temporal workflow task
executor.RegisterTask("example_workflow", agentsdk.TemporalWorkflowTask(temporalClient, "ExampleWorkflow"))

// Execute the Temporal workflow task
result, err := executor.ExecuteSync(context.Background(), "example_workflow", map[string]interface{}{
    "input": "example input",
}, nil)
```

##### Task Options

You can configure task execution with the following options:

```go
options := &interfaces.TaskOptions{
    // Timeout specifies the maximum duration for task execution
    Timeout: &timeout,

    // RetryPolicy specifies the retry policy for the task
    RetryPolicy: &interfaces.RetryPolicy{
        MaxRetries:        3,
        InitialBackoff:    100 * time.Millisecond,
        MaxBackoff:        1 * time.Second,
        BackoffMultiplier: 2.0,
    },

    // Metadata contains additional information for the task execution
    Metadata: map[string]interface{}{
        "purpose": "example",
    },
}
```

##### Task Result

The task result contains the following information:

```go
type TaskResult struct {
    // Data contains the result data
    Data interface{}

    // Error contains any error that occurred during task execution
    Error error

    // Metadata contains additional information about the task execution
    Metadata map[string]interface{}
}
```

---

### File: tools.md

*Path: docs/tools.md*

#### Tools

This document explains how to use and create tools for the Agent SDK.

##### Overview

Tools extend the capabilities of an agent by allowing it to perform actions or retrieve information from external systems. The Agent SDK provides a flexible framework for creating and using tools.

##### Built-in Tools

The Agent SDK comes with several built-in tools:

###### Web Search

Allows the agent to search the web for information:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"

searchTool := websearch.New(
    googleAPIKey,
    googleSearchEngineID,
)
```

###### Calculator

Allows the agent to perform mathematical calculations:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"

calculatorTool := calculator.New()
```

###### AWS Tools

Allows the agent to interact with AWS services:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools/aws"

// EC2 tool
ec2Tool := aws.NewEC2Tool()

// S3 tool
s3Tool := aws.NewS3Tool()
```

###### Kubernetes Tools

Allows the agent to interact with Kubernetes clusters:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools/kubernetes"

kubeTool := kubernetes.New()
```

##### Using Tools with an Agent

To use tools with an agent, pass them to the `WithTools` option:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

// Create tools
searchTool := websearch.New(googleAPIKey, googleSearchEngineID)
calculatorTool := calculator.New()

// Create agent with tools
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithTools(searchTool, calculatorTool),
)
```

##### Creating Custom Tools

You can create custom tools by implementing the `interfaces.Tool` interface:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// WeatherTool is a custom tool for getting weather information
type WeatherTool struct {
    apiKey string
}

// NewWeatherTool creates a new weather tool
func NewWeatherTool(apiKey string) *WeatherTool {
    return &WeatherTool{
        apiKey: apiKey,
    }
}

// Name returns the name of the tool
func (t *WeatherTool) Name() string {
    return "weather"
}

// Description returns a description of what the tool does
func (t *WeatherTool) Description() string {
    return "Get current weather information for a location"
}

// Parameters returns the parameters that the tool accepts
func (t *WeatherTool) Parameters() map[string]interfaces.ParameterSpec {
    return map[string]interfaces.ParameterSpec{
        "location": {
            Type:        "string",
            Description: "The location to get weather for (e.g., 'New York', 'Tokyo')",
            Required:    true,
        },
        "units": {
            Type:        "string",
            Description: "The units to use (metric or imperial)",
            Required:    false,
            Default:     "metric",
            Enum:        []interface{}{"metric", "imperial"},
        },
    }
}

// Run executes the tool with the given input
func (t *WeatherTool) Run(ctx context.Context, input string) (string, error) {
    // Parse the input and call a weather API
    // This is a simplified example
    return "The weather in " + input + " is sunny and 25°C", nil
}

// Execute executes the tool with the given arguments
func (t *WeatherTool) Execute(ctx context.Context, args string) (string, error) {
    // Parse the JSON arguments and call a weather API
    // This is a simplified example
    return "The weather is sunny and 25°C", nil
}
```

##### Tool Registry

The Tool Registry manages a collection of tools:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools"

// Create a tool registry
registry := tools.NewRegistry()

// Register tools
registry.Register(websearch.New(googleAPIKey, googleSearchEngineID))
registry.Register(calculator.New())

// Get a tool by name
tool, found := registry.Get("websearch")
if found {
    result, err := tool.Run(ctx, "latest AI news")
    // ...
}

// Get all registered tools
allTools := registry.List()
```

##### Tool Execution

The Agent SDK provides a flexible way to execute tools:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/tools"

// Create a tool executor
executor := tools.NewExecutor(registry)

// Execute a tool by name
result, err := executor.Execute(ctx, "websearch", "latest AI news")
if err != nil {
    log.Fatalf("Failed to execute tool: %v", err)
}
fmt.Println(result)
```

##### Advanced Tool Usage

###### Tool with Authentication

You can create tools that require authentication:

```go
// Create a tool with authentication
type AuthenticatedTool struct {
    apiKey string
}

func (t *AuthenticatedTool) Run(ctx context.Context, input string) (string, error) {
    // Use the API key for authentication
    client := &http.Client{}
    req, err := http.NewRequestWithContext(ctx, "GET", "https://api.example.com/data", nil)
    if err != nil {
        return "", err
    }

    // Add authentication header
    req.Header.Add("Authorization", "Bearer "+t.apiKey)

    // Make the request
    resp, err := client.Do(req)
    // ...
}
```

###### Tool with Rate Limiting

You can create tools with rate limiting:

```go
import (
    "context"
    "time"
    "golang.org/x/time/rate"
)

// Create a rate-limited tool
type RateLimitedTool struct {
    limiter *rate.Limiter
    tool    interfaces.Tool
}

func NewRateLimitedTool(tool interfaces.Tool, rps float64) *RateLimitedTool {
    return &RateLimitedTool{
        limiter: rate.NewLimiter(rate.Limit(rps), 1),
        tool:    tool,
    }
}

func (t *RateLimitedTool) Run(ctx context.Context, input string) (string, error) {
    // Wait for rate limit
    if err := t.limiter.Wait(ctx); err != nil {
        return "", err
    }

    // Run the underlying tool
    return t.tool.Run(ctx, input)
}

// Implement other Tool interface methods...
```

##### Example: Complete Tool Setup

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
    "github.com/Ingenimax/agent-sdk-go/pkg/tools"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create OpenAI client
    openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

    // Create tool registry
    registry := tools.NewRegistry()

    // Register tools
    if cfg.Tools.WebSearch.GoogleAPIKey != "" && cfg.Tools.WebSearch.GoogleSearchEngineID != "" {
        searchTool := websearch.New(
            cfg.Tools.WebSearch.GoogleAPIKey,
            cfg.Tools.WebSearch.GoogleSearchEngineID,
        )
        registry.Register(searchTool)
    }

    // Register calculator tool
    registry.Register(calculator.New())

    // Create a custom weather tool
    weatherTool := NewWeatherTool("your-weather-api-key")
    registry.Register(weatherTool)

    // Create a new agent with tools
    agent, err := agent.NewAgent(
        agent.WithLLM(openaiClient),
        agent.WithMemory(memory.NewConversationBuffer()),
        agent.WithTools(registry.List()...),
        agent.WithSystemPrompt("You are a helpful AI assistant. Use tools when needed."),
    )
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Run the agent with a query that might require tools
    ctx := context.Background()
    response, err := agent.Run(ctx, "What's the weather in New York and what's 123 * 456?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }

    fmt.Println(response)
}

// WeatherTool implementation (as shown in the custom tool example)
```

---

### File: tracing.md

*Path: docs/tracing.md*

#### Tracing

This document explains how to use the Tracing component of the Agent SDK.

##### Overview

Tracing provides observability into the behavior of your agents, allowing you to monitor, debug, and analyze their performance. The Agent SDK supports multiple tracing backends, including Langfuse and OpenTelemetry.

##### Enabling Tracing

###### Langfuse

[Langfuse](https://langfuse.com/) is a specialized observability platform for LLM applications:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/tracing/langfuse"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create Langfuse tracer
tracer := langfuse.New(
    cfg.Tracing.Langfuse.SecretKey,
    cfg.Tracing.Langfuse.PublicKey,
    langfuse.WithHost(cfg.Tracing.Langfuse.Host),
    langfuse.WithEnvironment(cfg.Tracing.Langfuse.Environment),
)
```

###### OpenTelemetry

[OpenTelemetry](https://opentelemetry.io/) is a vendor-neutral observability framework:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/tracing/otel"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create OpenTelemetry tracer
tracer, err := otel.New(
    cfg.Tracing.OpenTelemetry.ServiceName,
    otel.WithCollectorEndpoint(cfg.Tracing.OpenTelemetry.CollectorEndpoint),
)
if err != nil {
    log.Fatalf("Failed to create OpenTelemetry tracer: %v", err)
}
defer tracer.Shutdown()
```

##### Using Tracing with an Agent

To use tracing with an agent, pass it to the `WithTracer` option:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/tracing/langfuse"
)

// Create tracer
tracer := langfuse.New(secretKey, publicKey)

// Create agent with tracer
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithTracer(tracer),
)
```

##### Manual Tracing

You can also use the tracer directly for manual instrumentation:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Start a trace
ctx, span := tracer.StartSpan(context.Background(), "my-operation")
defer span.End()

// Add attributes to the span
span.SetAttribute("key", "value")

// Record events
span.AddEvent("something-happened")

// Record errors
span.RecordError(err)
```

##### Tracing LLM Calls

The Agent SDK automatically traces LLM calls when a tracer is configured:

```go
// The agent will automatically trace LLM calls
response, err := agent.Run(ctx, "What is the capital of France?")
```

You can also manually trace LLM calls:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm"
)

// Start a trace for the LLM call
ctx, span := tracer.StartSpan(ctx, "llm-generate")
defer span.End()

// Set LLM-specific attributes
span.SetAttribute("llm.model", "gpt-4")
span.SetAttribute("llm.prompt", prompt)

// Make the LLM call
response, err := client.Generate(ctx, prompt)

// Record the response
span.SetAttribute("llm.response", response)
if err != nil {
    span.RecordError(err)
}
```

##### Tracing Tool Calls

The Agent SDK automatically traces tool calls when a tracer is configured:

```go
// The agent will automatically trace tool calls
response, err := agent.Run(ctx, "What's the weather in San Francisco?")
```

You can also manually trace tool calls:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Start a trace for the tool call
ctx, span := tracer.StartSpan(ctx, "tool-execute")
defer span.End()

// Set tool-specific attributes
span.SetAttribute("tool.name", tool.Name())
span.SetAttribute("tool.input", input)

// Execute the tool
result, err := tool.Run(ctx, input)

// Record the result
span.SetAttribute("tool.result", result)
if err != nil {
    span.RecordError(err)
}
```

##### Multi-tenancy with Tracing

When using tracing with multi-tenancy, you can include the organization ID in the traces:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create context with organization ID
ctx := context.Background()
ctx = multitenancy.WithOrgID(ctx, "org-123")

// The organization ID will be included in the traces
response, err := agent.Run(ctx, "What is the capital of France?")
```

##### Viewing Traces

###### Langfuse

To view traces in Langfuse:

1. Log in to your Langfuse account at https://cloud.langfuse.com
2. Navigate to the "Traces" section
3. Filter and search for your traces

###### OpenTelemetry

To view OpenTelemetry traces, you need a compatible backend such as Jaeger, Zipkin, or a cloud observability platform:

1. Configure your OpenTelemetry collector to send traces to your backend
2. Access your backend's UI to view and analyze traces

##### Creating Custom Tracers

You can implement custom tracers by implementing the `interfaces.Tracer` interface:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// CustomTracer is a custom tracer implementation
type CustomTracer struct {
    // Add your fields here
}

// NewCustomTracer creates a new custom tracer
func NewCustomTracer() *CustomTracer {
    return &CustomTracer{}
}

// StartSpan starts a new span
func (t *CustomTracer) StartSpan(ctx context.Context, name string) (context.Context, interfaces.Span) {
    // Implement your logic to start a span
    return ctx, &CustomSpan{}
}

// CustomSpan is a custom span implementation
type CustomSpan struct {
    // Add your fields here
}

// SetAttribute sets an attribute on the span
func (s *CustomSpan) SetAttribute(key string, value interface{}) {
    // Implement your logic to set an attribute
}

// AddEvent adds an event to the span
func (s *CustomSpan) AddEvent(name string) {
    // Implement your logic to add an event
}

// RecordError records an error on the span
func (s *CustomSpan) RecordError(err error) {
    // Implement your logic to record an error
}

// End ends the span
func (s *CustomSpan) End() {
    // Implement your logic to end the span
}
```

##### Example: Complete Tracing Setup

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
    "github.com/Ingenimax/agent-sdk-go/pkg/tracing/langfuse"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create OpenAI client
    openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

    // Create tracer
    tracer := langfuse.New(
        cfg.Tracing.Langfuse.SecretKey,
        cfg.Tracing.Langfuse.PublicKey,
        langfuse.WithHost(cfg.Tracing.Langfuse.Host),
        langfuse.WithEnvironment(cfg.Tracing.Langfuse.Environment),
    )

    // Create tools
    searchTool := websearch.New(
        cfg.Tools.WebSearch.GoogleAPIKey,
        cfg.Tools.WebSearch.GoogleSearchEngineID,
    )

    // Create a new agent with tracer
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

    // Create context with trace ID
    ctx := context.Background()
    ctx, span := tracer.StartSpan(ctx, "user-session")
    defer span.End()

    // Add session attributes
    span.SetAttribute("session.id", "session-123")
    span.SetAttribute("user.id", "user-456")

    // Run the agent
    response, err := agent.Run(ctx, "What's the latest news about artificial intelligence?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
        span.RecordError(err)
    }

    // Record the response
    span.SetAttribute("response", response)

    fmt.Println(response)
}

---

### File: vectorstore.md

*Path: docs/vectorstore.md*

#### Vector Store

This document explains how to use the Vector Store component of the Agent SDK.

##### Overview

Vector stores are used to store and retrieve vector embeddings, which are numerical representations of text that capture semantic meaning. They enable semantic search and retrieval of information based on similarity.

##### Supported Vector Stores

###### Weaviate

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/weaviate"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create Weaviate vector store
store := weaviate.New(
    cfg.VectorStore.Weaviate.URL,
    weaviate.WithAPIKey(cfg.VectorStore.Weaviate.APIKey),
    weaviate.WithClassName("Document"),
)
```

###### Pinecone

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/pinecone"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create Pinecone vector store
store := pinecone.New(
    cfg.VectorStore.Pinecone.APIKey,
    cfg.VectorStore.Pinecone.Environment,
    cfg.VectorStore.Pinecone.Index,
)
```

##### Using Vector Stores

###### Adding Documents

Add documents to the vector store:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Create documents
docs := []interfaces.Document{
    {
        ID:      "doc1",
        Content: "This is the first document about artificial intelligence.",
        Metadata: map[string]interface{}{
            "source": "article",
            "author": "John Doe",
        },
    },
    {
        ID:      "doc2",
        Content: "This is the second document about machine learning.",
        Metadata: map[string]interface{}{
            "source": "book",
            "author": "Jane Smith",
        },
    },
}

// Add documents to the vector store
err := store.AddDocuments(context.Background(), docs)
if err != nil {
    log.Fatalf("Failed to add documents: %v", err)
}
```

###### Searching Documents

Search for documents by similarity:

```go
// Search for documents similar to a query
results, err := store.Search(
    context.Background(),
    "What is artificial intelligence?",
    interfaces.WithLimit(5),
)
if err != nil {
    log.Fatalf("Failed to search documents: %v", err)
}

// Print search results
for _, result := range results {
    fmt.Printf("Document ID: %s\n", result.ID)
    fmt.Printf("Content: %s\n", result.Content)
    fmt.Printf("Score: %f\n", result.Score)
    fmt.Println("Metadata:", result.Metadata)
    fmt.Println()
}
```

###### Retrieving Documents

Retrieve documents by ID:

```go
// Retrieve documents by ID
docs, err := store.GetDocuments(
    context.Background(),
    []string{"doc1", "doc2"},
)
if err != nil {
    log.Fatalf("Failed to retrieve documents: %v", err)
}

// Print retrieved documents
for _, doc := range docs {
    fmt.Printf("Document ID: %s\n", doc.ID)
    fmt.Printf("Content: %s\n", doc.Content)
    fmt.Println("Metadata:", doc.Metadata)
    fmt.Println()
}
```

###### Deleting Documents

Delete documents from the vector store:

```go
// Delete documents by ID
err := store.DeleteDocuments(
    context.Background(),
    []string{"doc1"},
)
if err != nil {
    log.Fatalf("Failed to delete documents: %v", err)
}
```

##### Configuration Options

###### Weaviate Options

```go
// Set the API key
weaviate.WithAPIKey("your-api-key")

// Set the scheme (http or https)
weaviate.WithScheme("https")

// Set the host
weaviate.WithHost("localhost:8080")

// Set the class name
weaviate.WithClassName("Document")

// Set the organization ID for multi-tenancy
weaviate.WithOrgID("org-123")
```

###### Pinecone Options

```go
// Set the namespace
pinecone.WithNamespace("default")

// Set the organization ID for multi-tenancy
pinecone.WithOrgID("org-123")
```

##### Multi-tenancy with Vector Stores

When using vector stores with multi-tenancy, you can specify the organization ID:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create context with organization ID
ctx := context.Background()
ctx = multitenancy.WithOrgID(ctx, "org-123")

// Add documents for this organization
err := store.AddDocuments(ctx, docs)

// Search documents for this organization
results, err := store.Search(ctx, "artificial intelligence")
```

##### Creating Custom Vector Store Implementations

You can implement custom vector stores by implementing the `interfaces.VectorStore` interface:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// CustomVectorStore is a custom vector store implementation
type CustomVectorStore struct {
    // Add your fields here
}

// NewCustomVectorStore creates a new custom vector store
func NewCustomVectorStore() *CustomVectorStore {
    return &CustomVectorStore{}
}

// AddDocuments adds documents to the vector store
func (s *CustomVectorStore) AddDocuments(ctx context.Context, documents []interfaces.Document) error {
    // Implement your logic to add documents
    return nil
}

// Search searches for documents similar to the query
func (s *CustomVectorStore) Search(ctx context.Context, query string, options ...interfaces.SearchOption) ([]interfaces.SearchResult, error) {
    // Apply options
    opts := &interfaces.SearchOptions{}
    for _, option := range options {
        option(opts)
    }

    // Implement your search logic
    return []interfaces.SearchResult{
        {
            ID:      "doc1",
            Content: "This is a document about artificial intelligence.",
            Score:   0.95,
            Metadata: map[string]interface{}{
                "source": "article",
            },
        },
    }, nil
}

// GetDocuments retrieves documents by ID
func (s *CustomVectorStore) GetDocuments(ctx context.Context, ids []string) ([]interfaces.Document, error) {
    // Implement your logic to retrieve documents
    return []interfaces.Document{
        {
            ID:      "doc1",
            Content: "This is a document about artificial intelligence.",
            Metadata: map[string]interface{}{
                "source": "article",
            },
        },
    }, nil
}

// DeleteDocuments deletes documents by ID
func (s *CustomVectorStore) DeleteDocuments(ctx context.Context, ids []string) error {
    // Implement your logic to delete documents
    return nil
}

// Name returns the name of the vector store
func (s *CustomVectorStore) Name() string {
    return "custom-vector-store"
}
```

##### Using Vector Stores with Embeddings

Vector stores typically work with embedding models to convert text to vectors:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/embedding"
    "github.com/Ingenimax/agent-sdk-go/pkg/embedding/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/weaviate"
)

// Create embedding model
embedder := openai.NewEmbedder(cfg.Embedding.OpenAI.APIKey)

// Create vector store with embedder
store := weaviate.New(
    cfg.VectorStore.Weaviate.URL,
    weaviate.WithAPIKey(cfg.VectorStore.Weaviate.APIKey),
    weaviate.WithEmbedder(embedder),
)

// Add documents (embeddings will be generated automatically)
err := store.AddDocuments(context.Background(), docs)

// Search (query will be embedded automatically)
results, err := store.Search(context.Background(), "artificial intelligence")
```

##### Example: Complete Vector Store Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/config"
    "github.com/Ingenimax/agent-sdk-go/pkg/embedding/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/weaviate"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create embedding model
    embedder := openai.NewEmbedder(cfg.Embedding.OpenAI.APIKey)

    // Create vector store
    store := weaviate.New(
        cfg.VectorStore.Weaviate.URL,
        weaviate.WithAPIKey(cfg.VectorStore.Weaviate.APIKey),
        weaviate.WithEmbedder(embedder),
    )

    // Create context
    ctx := context.Background()

    // Create documents
    docs := []interfaces.Document{
        {
            ID:      "doc1",
            Content: "Artificial intelligence (AI) is intelligence demonstrated by machines.",
            Metadata: map[string]interface{}{
                "source": "wikipedia",
                "topic":  "AI",
            },
        },
        {
            ID:      "doc2",
            Content: "Machine learning is a subset of artificial intelligence.",
            Metadata: map[string]interface{}{
                "source": "textbook",
                "topic":  "ML",
            },
        },
        {
            ID:      "doc3",
            Content: "Deep learning is a type of machine learning based on artificial neural networks.",
            Metadata: map[string]interface{}{
                "source": "research paper",
                "topic":  "DL",
            },
        },
    }

    // Add documents to the vector store
    err := store.AddDocuments(ctx, docs)
    if err != nil {
        log.Fatalf("Failed to add documents: %v", err)
    }
    fmt.Println("Added documents to vector store")

    // Search for documents
    results, err := store.Search(
        ctx,
        "What is artificial intelligence?",
        interfaces.WithLimit(2),
    )
    if err != nil {
        log.Fatalf("Failed to search documents: %v", err)
    }

    // Print search results
    fmt.Println("Search results:")
    for i, result := range results {
        fmt.Printf("%d. Document ID: %s (Score: %.4f)\n", i+1, result.ID, result.Score)
        fmt.Printf("   Content: %s\n", result.Content)
        fmt.Printf("   Metadata: %v\n", result.Metadata)
    }
}

---

## pkg > embedding

### File: README.md

*Path: pkg/embedding/README.md*

#### Enhanced Embedding Package

This package provides advanced embedding generation and manipulation capabilities for the Agent SDK. It includes features for configuring embedding models, batch processing, similarity calculations, and metadata filtering.

##### Features

- **Configurable Embedding Generation**: Fine-tune embedding parameters such as dimensions, encoding format, and truncation behavior.
- **Batch Processing**: Generate embeddings for multiple texts in a single API call.
- **Similarity Calculations**: Calculate similarity between embeddings using different metrics (cosine, euclidean, dot product).
- **Advanced Metadata Filtering**: Create complex filter conditions for precise document retrieval.

##### Usage

###### Basic Embedding Generation

```go
// Create an embedder with default configuration
embedder := embedding.NewOpenAIEmbedder(apiKey, "text-embedding-3-small")

// Generate an embedding
vector, err := embedder.Embed(ctx, "Your text here")
if err != nil {
    // Handle error
}
```

###### Custom Configuration

```go
// Create a custom configuration
config := embedding.DefaultEmbeddingConfig()
config.Model = "text-embedding-3-large"
config.Dimensions = 1536
config.SimilarityMetric = "cosine"

// Create an embedder with custom configuration
embedder := embedding.NewOpenAIEmbedderWithConfig(apiKey, config)

// Generate an embedding with custom configuration
vector, err := embedder.EmbedWithConfig(ctx, "Your text here", config)
if err != nil {
    // Handle error
}
```

###### Batch Processing

```go
// Generate embeddings for multiple texts
texts := []string{
    "First text",
    "Second text",
    "Third text",
}

vectors, err := embedder.EmbedBatch(ctx, texts)
if err != nil {
    // Handle error
}
```

###### Similarity Calculation

```go
// Calculate similarity between two vectors
similarity, err := embedder.CalculateSimilarity(vector1, vector2, "cosine")
if err != nil {
    // Handle error
}
```

##### Metadata Filtering

The package includes powerful metadata filtering capabilities for precise document retrieval.

###### Simple Filters

```go
// Create a simple filter
filter := embedding.NewMetadataFilter("category", "=", "science")

// Create a filter group
filterGroup := embedding.NewMetadataFilterGroup("and", filter)
```

###### Complex Filters

```go
// Create a complex filter group
filterGroup := embedding.NewMetadataFilterGroup("and",
    embedding.NewMetadataFilter("category", "=", "science"),
    embedding.NewMetadataFilter("published_date", ">", "2023-01-01"),
)

// Add another filter
filterGroup.AddFilter(embedding.NewMetadataFilter("author", "=", "John Doe"))

// Create a sub-group with OR logic
subGroup := embedding.NewMetadataFilterGroup("or",
    embedding.NewMetadataFilter("tags", "contains", "physics"),
    embedding.NewMetadataFilter("tags", "contains", "chemistry"),
)

// Add the sub-group to the main group
filterGroup.AddSubGroup(subGroup)
```

###### Using Filters with Vector Store

```go
// Convert filter group to map for vector store
filterMap := embedding.FilterToMap(filterGroup)

// Use with vector store search
results, err := store.Search(ctx, "query", 10,
    interfaces.WithEmbedding(true),
    interfaces.WithFilters(filterMap),
)
```

###### Filtering Documents in Memory

```go
// Filter documents in memory
filteredDocs := embedding.ApplyFilters(documents, filterGroup)
```

##### Supported Operators

###### Comparison Operators

- `=`, `==`, `eq`: Equal
- `!=`, `<>`, `ne`: Not equal
- `>`, `gt`: Greater than
- `>=`, `gte`: Greater than or equal
- `<`, `lt`: Less than
- `<=`, `lte`: Less than or equal
- `contains`: String contains
- `in`: Value in collection
- `not_in`: Value not in collection

###### Logical Operators

- `and`: All conditions must be true
- `or`: At least one condition must be true

##### Configuration Options

###### Embedding Models

- `text-embedding-3-small`: Smaller, faster model (1536 dimensions by default)
- `text-embedding-3-large`: Larger, more accurate model (3072 dimensions by default)
- `text-embedding-ada-002`: Legacy model (1536 dimensions)

###### Dimensions

Specify the dimensionality of the embedding vectors. Only supported by some models.

###### Encoding Format

- `float`: Standard floating-point format
- `base64`: Base64-encoded format for more compact storage

###### Truncation

- `none`: Error on token limit overflow
- `truncate`: Truncate text to fit within token limit

###### Similarity Metrics

- `cosine`: Cosine similarity (default)
- `euclidean`: Euclidean distance (converted to similarity score)
- `dot_product`: Dot product

---

## pkg > llm > anthropic

### File: README.md

*Path: pkg/llm/anthropic/README.md*

#### Anthropic Client for Agent SDK

This package provides an implementation of the Anthropic API client for use with the Agent SDK, supporting Claude models including Claude-3.5-Haiku, Claude-3.5-Sonnet, Claude-3-Opus, and Claude-3.7-Sonnet.

##### Supported Models

The client supports the following Claude models:

- `Claude35Haiku` (claude-3-5-haiku-latest) - Fast and cost-effective
- `Claude35Sonnet` (claude-3-5-sonnet-latest) - Balanced performance and capabilities
- `Claude3Opus` (claude-3-opus-latest) - Most powerful model with highest capabilities
- `Claude37Sonnet` (claude-3-7-sonnet-latest) - Latest model with improved capabilities

##### Important: Model Specification Required

When creating an Anthropic client, you must explicitly specify the model to use via the `WithModel` option. The client will log a warning if no model is specified, but it's strongly recommended to always specify the model explicitly:

```go
// Always specify the model with WithModel
client := anthropic.NewClient(
	apiKey,
	anthropic.WithModel(anthropic.Claude37Sonnet), // Always specify the model
)
```

##### API Version Note

This client uses the Anthropic API version 2023-06-01. Some features may not be supported in this version:

- The `reasoning` parameter is maintained for backward compatibility but is not officially supported in the current API.
- The `organization` parameter is not supported in the current API version.

##### Usage Examples

###### Basic Usage

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("ANTHROPIC_API_KEY environment variable not set")
		os.Exit(1)
	}

	// Create a new client with model explicitly specified
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(anthropic.Claude37Sonnet), // Always specify the model
	)

	// Create a client with custom settings
	// client := anthropic.NewClient(
	//     apiKey,
	//     anthropic.WithModel(anthropic.Claude37Sonnet), // Model is required
	//     anthropic.WithBaseURL("https://api.anthropic.com"),
	// )

	// Generate text
	ctx := context.Background()
	response, err := client.Generate(ctx, "Explain quantum computing in simple terms")
	if err != nil {
		fmt.Printf("Error generating text: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(response)
}
```

###### Loading Configuration from Environment Variables

If your application needs to load configuration from environment variables:

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("ANTHROPIC_API_KEY environment variable not set")
		os.Exit(1)
	}

	// Get model from environment - REQUIRED
	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		// Default to Claude37Sonnet if not specified, but better to require it
		model = anthropic.Claude37Sonnet
		fmt.Println("Warning: ANTHROPIC_MODEL not set, using Claude37Sonnet")
	}

	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	timeout := 60
	if timeoutStr := os.Getenv("ANTHROPIC_TIMEOUT"); timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil && t > 0 {
			timeout = t
		}
	}

	temperature := 0.7
	if tempStr := os.Getenv("ANTHROPIC_TEMPERATURE"); tempStr != "" {
		if t, err := strconv.ParseFloat(tempStr, 64); err == nil {
			temperature = t
		}
	}

	// Create client with config from environment variables
	// Note that we always specify the model explicitly
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(model), // Model is required
		anthropic.WithBaseURL(baseURL),
		anthropic.WithHTTPClient(&http.Client{Timeout: time.Duration(timeout) * time.Second}),
	)

	// Generate text
	ctx := context.Background()
	response, err := client.Generate(
		ctx,
		"Explain quantum computing in simple terms",
		anthropic.WithTemperature(temperature),
	)
	if err != nil {
		fmt.Printf("Error generating text: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(response)
}
```

###### Step-by-Step Reasoning

While the "reasoning" parameter is not officially supported in the current API version, Claude models naturally provide step-by-step reasoning for many types of problems:

```go
response, err := client.Generate(
    ctx,
    "How would you solve this equation: 3x + 7 = 22?",
    // WithReasoning is maintained for backward compatibility but not officially supported
    anthropic.WithReasoning("comprehensive")
)
```

###### Chat Interface

The client also supports a chat interface for multi-turn conversations:

```go
messages := []llm.Message{
    {Role: "system", Content: "You are a helpful assistant."},
    {Role: "user", Content: "Tell me about the history of artificial intelligence."},
}

params := &llm.GenerateParams{
    Temperature: 0.7,
}

response, err := client.Chat(ctx, messages, params)
```

###### Using Tools

The client supports tool calling with Claude models. Note that you need to provide an organization ID in the context:

```go
// Create context with organization ID
ctx = multitenancy.WithOrgID(ctx, "your-org-id")

// Define tools
tools := []interfaces.Tool{
    // Your tool implementations here
}

response, err := client.GenerateWithTools(
    ctx,
    "What's the weather in New York?",
    tools,
    anthropic.WithSystemMessage("You are a helpful assistant. Use tools when appropriate."),
    anthropic.WithTemperature(0.7)
)
```

###### Creating an Agent

When creating an agent with the Anthropic client, you must provide both an organization ID and a conversation ID in the context:

```go
// Set up context with required values
ctx = context.Background()
ctx = multitenancy.WithOrgID(ctx, "your-org-id")
ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-id")

// Create memory store
memoryStore := memory.NewConversationBuffer()

// Create agent
agentInstance, err := agent.NewAgent(
    agent.WithLLM(client),
    agent.WithMemory(memoryStore),
    agent.WithSystemPrompt("You are a helpful AI assistant."),
)

// Run agent
response, err := agentInstance.Run(ctx, "Tell me about quantum computing")
```

##### Configuration Options

Options for configuring the Anthropic client:

- `WithModel(model string)` - Set the model to use (e.g., `anthropic.Claude37Sonnet`)
- `WithBaseURL(baseURL string)` - Set a custom API endpoint
- `WithHTTPClient(client *http.Client)` - Set a custom HTTP client
- `WithLogger(logger logging.Logger)` - Set a custom logger
- `WithRetry(opts ...retry.Option)` - Configure retry policy

Options for generate requests:

- `WithTemperature(temperature float64)` - Control randomness (0.0 to 1.0)
- `WithTopP(topP float64)` - Alternative to temperature for nucleus sampling
- `WithSystemMessage(message string)` - Set system message
- `WithStopSequences(sequences []string)` - Set stop sequences
- `WithFrequencyPenalty(penalty float64)` - Set frequency penalty
- `WithPresencePenalty(penalty float64)` - Set presence penalty
- `WithReasoning(reasoning string)` - Maintained for compatibility but not officially supported



---

## pkg > llm > openai

### File: README.md

*Path: pkg/llm/openai/README.md*

#### OpenAI Client Package

This package provides a client for interacting with the OpenAI API, implementing the `interfaces.LLM` interface.

##### Features

- Text generation with the `Generate` method
- Chat completion with the `Chat` method
- Tool integration with the `GenerateWithTools` method
- Configurable options for model parameters
- Direct implementation of the `interfaces.LLM` interface

##### Usage

###### Creating a Client

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"

// Create a client with default settings
client := openai.NewClient(apiKey)

// Create a client with a specific model
client := openai.NewClient(
    apiKey,
    openai.WithModel("gpt-4o-mini"),
)
```

###### Text Generation

```go
response, err := client.Generate(
    context.Background(),
    "Write a haiku about programming",
    openai.WithTemperature(0.7),
)
```

###### Chat Completion

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/llm"

messages := []llm.Message{
    {
        Role:    "system",
        Content: "You are a helpful programming assistant.",
    },
    {
        Role:    "user",
        Content: "What's the best way to handle errors in Go?",
    },
}

response, err := client.Chat(context.Background(), messages, nil)
```

###### Tool Integration

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"

// Define tools
tools := []interfaces.Tool{...}

// Generate with tools
response, err := client.GenerateWithTools(
    context.Background(),
    "What's the weather in San Francisco?",
    tools,
    openai.WithTemperature(0.7),
)
```

###### Available Options

The OpenAI client provides several option functions for configuring requests:

- `WithTemperature(float64)` - Controls randomness (0.0 to 1.0)
- `WithTopP(float64)` - Controls diversity via nucleus sampling
- `WithFrequencyPenalty(float64)` - Reduces repetition of token sequences
- `WithPresencePenalty(float64)` - Reduces repetition of topics
- `WithStopSequences([]string)` - Specifies sequences where generation should stop

##### Integration with Agents

The OpenAI client can be directly used with agents:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

// Create OpenAI client
openaiClient := openai.NewClient(apiKey)

// Create agent with the OpenAI client
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    // ... other options
)
```

---

## pkg > task

### File: README.md

*Path: pkg/task/README.md*

#### Task Execution Package

The task execution package provides functionality for executing tasks synchronously and asynchronously, including API calls and Temporal workflows.

##### Features

- Execute tasks synchronously and asynchronously
- Built-in retry mechanism with configurable retry policies
- API client for making HTTP requests
- Temporal workflow integration
- Task cancellation and status tracking
- Task adapter pattern for integrating with agent-specific models

##### Usage

###### Basic Task Execution

```go
// Create a task executor
executor := agentsdk.NewTaskExecutor()

// Register a task
executor.RegisterTask("hello", func(ctx context.Context, params interface{}) (interface{}, error) {
    name, ok := params.(string)
    if !ok {
        name = "World"
    }
    return fmt.Sprintf("Hello, %s!", name), nil
})

// Execute the task synchronously
result, err := executor.ExecuteSync(context.Background(), "hello", "John", nil)
if err != nil {
    fmt.Printf("Error: %v\n", err)
} else {
    fmt.Printf("Result: %v\n", result.Data)
}

// Execute the task asynchronously
resultChan, err := executor.ExecuteAsync(context.Background(), "hello", "Jane", nil)
if err != nil {
    fmt.Printf("Error: %v\n", err)
} else {
    result := <-resultChan
    fmt.Printf("Result: %v\n", result.Data)
}
```

###### API Task Execution

```go
// Create an API client
apiClient := agentsdk.NewAPIClient("https://api.example.com", 10*time.Second)

// Register an API task
executor.RegisterTask("get_data", agentsdk.APITask(apiClient, task.APIRequest{
    Method: "GET",
    Path:   "/data",
    Query:  map[string]string{"limit": "10"},
}))

// Execute the API task with retry policy
timeout := 5 * time.Second
retryPolicy := &interfaces.RetryPolicy{
    MaxRetries:        3,
    InitialBackoff:    100 * time.Millisecond,
    MaxBackoff:        1 * time.Second,
    BackoffMultiplier: 2.0,
}

result, err := executor.ExecuteSync(context.Background(), "get_data", nil, &interfaces.TaskOptions{
    Timeout:     &timeout,
    RetryPolicy: retryPolicy,
})
```

###### Using the Task Adapter Pattern

The task adapter pattern allows you to use your own agent-specific models while still leveraging the SDK's task management functionality. This pattern separates the concerns of the SDK from your agent-specific implementations.

####### Default Implementation

The SDK provides a default implementation of the task models and adapter that you can use directly:

```go
package main

import (
    "context"
    "fmt"
    "github.com/Ingenimax/agent-sdk-go/pkg/logging"
    "github.com/Ingenimax/agent-sdk-go/pkg/task"
)

func main() {
    ctx := context.Background()
    logger := logging.New()

    // Create the SDK task service
    sdkTaskService := task.NewInMemoryTaskService(logger, nil, nil)

    // Create the default adapter
    adapter := task.NewDefaultTaskAdapter(logger)

    // Create the agent task service with default models
    taskService := task.NewAgentTaskService(
        logger,
        sdkTaskService,
        adapter,
    )

    // Create a task using the default models
    newTask, err := taskService.CreateTask(ctx, task.DefaultCreateRequest{
        Description: "Deploy a new service",
        UserID:      "user123",
        Title:       "Service Deployment",
        TaskKind:    "deployment",
    })

    if err != nil {
        panic(err)
    }

    fmt.Printf("Created task: %s\n", newTask.ID)
}
```

####### Custom Implementation

Alternatively, you can create your own models and adapter:

```go
// Define your agent-specific task models
type MyTask struct {
    ID          string
    Name        string
    Status      string
    CreatedAt   time.Time
    CompletedAt *time.Time
}

type MyCreateRequest struct {
    Name   string
    UserID string
}

type MyApprovalRequest struct {
    Approved bool
    Comment  string
}

type MyTaskUpdate struct {
    Type   string
    ID     string
    Status string
}

// Implement the TaskAdapter interface
type MyTaskAdapter struct {
    logger logging.Logger
}

// Create a new adapter
func NewMyTaskAdapter(logger logging.Logger) task.TaskAdapter[MyTask, MyCreateRequest, MyApprovalRequest, MyTaskUpdate] {
    return &MyTaskAdapter{
        logger: logger,
    }
}

// Implement conversion methods
func (a *MyTaskAdapter) ConvertCreateRequest(req MyCreateRequest) task.CreateTaskRequest {
    return task.CreateTaskRequest{
        Description: req.Name,
        UserID:      req.UserID,
        Metadata:    make(map[string]interface{}),
    }
}

func (a *MyTaskAdapter) ConvertApproveRequest(req MyApprovalRequest) task.ApproveTaskPlanRequest {
    return task.ApproveTaskPlanRequest{
        Approved: req.Approved,
        Feedback: req.Comment,
    }
}

func (a *MyTaskAdapter) ConvertTaskUpdates(updates []MyTaskUpdate) []task.TaskUpdate {
    sdkUpdates := make([]task.TaskUpdate, len(updates))
    for i, update := range updates {
        sdkUpdates[i] = task.TaskUpdate{
            Type:   update.Type,
            StepID: update.ID,
            Status: update.Status,
        }
    }
    return sdkUpdates
}

func (a *MyTaskAdapter) ConvertTask(sdkTask *task.Task) MyTask {
    if sdkTask == nil {
        return MyTask{}
    }

    return MyTask{
        ID:          sdkTask.ID,
        Name:        sdkTask.Description,
        Status:      string(sdkTask.Status),
        CreatedAt:   sdkTask.CreatedAt,
        CompletedAt: sdkTask.CompletedAt,
    }
}

func (a *MyTaskAdapter) ConvertTasks(sdkTasks []*task.Task) []MyTask {
    tasks := make([]MyTask, len(sdkTasks))
    for i, sdkTask := range sdkTasks {
        tasks[i] = a.ConvertTask(sdkTask)
    }
    return tasks
}

// Use the adapter with a task service
type MyTaskService struct {
    sdkService task.Service
    adapter    task.TaskAdapter[MyTask, MyCreateRequest, MyApprovalRequest, MyTaskUpdate]
}

func NewMyTaskService(sdkService task.Service, adapter task.TaskAdapter[MyTask, MyCreateRequest, MyApprovalRequest, MyTaskUpdate]) *MyTaskService {
    return &MyTaskService{
        sdkService: sdkService,
        adapter:    adapter,
    }
}

// Create a task using your own models
func (s *MyTaskService) CreateTask(ctx context.Context, req MyCreateRequest) (MyTask, error) {
    // Convert to SDK request
    sdkReq := s.adapter.ConvertCreateRequest(req)

    // Create task using SDK service
    sdkTask, err := s.sdkService.CreateTask(ctx, sdkReq)
    if err != nil {
        return MyTask{}, err
    }

    // Convert back to your model
    return s.adapter.ConvertTask(sdkTask), nil
}
```

###### Using the Agent Task Service

The SDK provides an `AgentTaskService` that you can use to work with your agent-specific models. It wraps the adapter service and provides a simpler interface:

```go
// Create the agent task service
taskService := task.NewAgentTaskService(
    ctx,
    logger,
    sdkTaskService,
    myAdapter,
)

// Use the service with your agent-specific models
myTask, err := taskService.CreateTask(ctx, myCreateRequest)
if err != nil {
    // Handle error
}

// Get a task
myTask, err = taskService.GetTask(ctx, "task-123")
if err != nil {
    // Handle error
}

// List tasks for a user
myTasks, err := taskService.ListTasks(ctx, "user-456")
if err != nil {
    // Handle error
}

// Approve a task plan
myTask, err = taskService.ApproveTaskPlan(ctx, "task-123", myApproveRequest)
if err != nil {
    // Handle error
}

// Update a task
myTask, err = taskService.UpdateTask(ctx, "task-123", "conversation-789", myTaskUpdates)
if err != nil {
    // Handle error
}

// Add a log entry to a task
err = taskService.AddTaskLog(ctx, "task-123", "Starting deployment", "info")
if err != nil {
    // Handle error
}
```

###### Temporal Workflow Execution

```go
// Create a Temporal client
temporalClient := agentsdk.NewTemporalClient(task.TemporalConfig{
    HostPort:                 "localhost:7233",
    Namespace:                "default",
    TaskQueue:                "example",
    WorkflowIDPrefix:         "example-",
    WorkflowExecutionTimeout: 10 * time.Minute,
    WorkflowRunTimeout:       5 * time.Minute,
    WorkflowTaskTimeout:      10 * time.Second,
})

// Register a Temporal workflow task
executor.RegisterTask("example_workflow", agentsdk.TemporalWorkflowTask(temporalClient, "ExampleWorkflow"))

// Execute the Temporal workflow task
result, err := executor.ExecuteSync(context.Background(), "example_workflow", map[string]interface{}{
    "input": "example input",
}, nil)
```

##### Task Options

You can configure task execution with the following options:

```go
options := &interfaces.TaskOptions{
    // Timeout specifies the maximum duration for task execution
    Timeout: &timeout,

    // RetryPolicy specifies the retry policy for the task
    RetryPolicy: &interfaces.RetryPolicy{
        MaxRetries:        3,
        InitialBackoff:    100 * time.Millisecond,
        MaxBackoff:        1 * time.Second,
        BackoffMultiplier: 2.0,
    },

    // Metadata contains additional information for the task execution
    Metadata: map[string]interface{}{
        "purpose": "example",
    },
}
```

##### Task Result

The task result contains the following information:

```go
type TaskResult struct {
    // Data contains the result data
    Data interface{}

    // Error contains any error that occurred during task execution
    Error error

    // Metadata contains additional information about the task execution
    Metadata map[string]interface{}
}
```

#### Task Package

The task package provides comprehensive task management functionality for agents. It includes models, interfaces, and services for creating, retrieving, and managing tasks.

##### Concepts

A task represents a unit of work that an agent needs to perform. Tasks have a lifecycle, starting from creation, through planning, execution, and finally completion. Each task can have multiple steps, which are executed sequentially.

##### Core Components

###### Task Model

The Task struct represents a task in the system:

```go
type Task struct {
	ID             string                 // Unique identifier
	Description    string                 // Task description
	Status         Status                 // Current status (pending, planning, awaiting_approval, executing, completed, failed)
	Title          string                 // Task title
	TaskKind       string                 // Type of task
	ConversationID string                 // Associated conversation ID
	Plan           *Plan                  // Execution plan
	Steps          []Step                 // Task steps
	CreatedAt      time.Time              // Creation timestamp
	UpdatedAt      time.Time              // Last update timestamp
	StartedAt      *time.Time             // When execution started
	CompletedAt    *time.Time             // When execution completed
	UserID         string                 // Owner user ID
	Logs           []LogEntry             // Activity logs
	Requirements   interface{}            // Task requirements
	Feedback       string                 // User feedback
	Metadata       map[string]interface{} // Additional metadata
}
```

###### Task Service

The `Service` interface defines methods for interacting with tasks:

```go
type Service interface {
	CreateTask(ctx context.Context, req CreateTaskRequest) (*Task, error)
	GetTask(ctx context.Context, taskID string) (*Task, error)
	ListTasks(ctx context.Context, filter TaskFilter) ([]*Task, error)
	ApproveTaskPlan(ctx context.Context, taskID string, req ApproveTaskPlanRequest) (*Task, error)
	UpdateTask(ctx context.Context, taskID string, updates []TaskUpdate) (*Task, error)
	AddTaskLog(ctx context.Context, taskID string, message string, level string) error
}
```

##### Task Adapter Pattern

The task package supports the adapter pattern to allow agents to work with their own domain-specific task models while leveraging the SDK's task management capabilities.

###### File Organization

The package is organized into the following key files:

- `models.go`: Contains all the data structures used in the task package
- `service.go`: Defines the core Service interface and provides both InMemoryTaskService and AgentTaskService implementations
- `adapter.go`: Implements the TaskAdapter interface and AdapterService for working with agent-specific models, including the default implementation

This compact organization helps keep related functionality together while still maintaining clean separation of concerns.

###### TaskAdapter Interface

The `TaskAdapter` is a generic interface that allows you to define custom conversion methods between SDK and agent-specific models:

```go
type TaskAdapter[AgentTask any, AgentCreateRequest any, AgentApprovalRequest any, AgentTaskUpdate any] interface {
	// ToSDK conversions (Agent -> SDK)
	ConvertCreateRequest(req AgentCreateRequest) CreateTaskRequest
	ConvertApproveRequest(req AgentApprovalRequest) ApproveTaskPlanRequest
	ConvertTaskUpdates(updates []AgentTaskUpdate) []TaskUpdate

	// FromSDK conversions (SDK -> Agent)
	ConvertTask(sdkTask *Task) AgentTask
	ConvertTasks(sdkTasks []*Task) []AgentTask
}
```

---
