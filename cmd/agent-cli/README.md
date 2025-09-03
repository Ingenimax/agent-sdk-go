# Agent CLI - Headless AI Agent Runner

A powerful command-line interface for the Agent SDK Go framework that provides headless access to AI agents with support for multiple LLM providers, tools, memory management, and enterprise features.

## Features

- 🤖 **Multiple LLM Providers**: OpenAI, Anthropic, Google Vertex AI, Ollama, vLLM
- 🛠️ **Rich Tool Ecosystem**: Web search, GitHub integration, MCP servers, and more
- 💬 **Interactive Chat Mode**: Real-time conversations with persistent memory
- 📝 **Task Execution**: Run predefined tasks from YAML configurations
- 🎨 **Auto-Configuration**: Generate agent configs from simple prompts
- 🔧 **Flexible Configuration**: JSON-based configuration with environment variables
- 📊 **Tracing & Monitoring**: Built-in observability with Langfuse integration
- 🏢 **Multi-tenancy**: Support for multiple organizations

## Installation

### Build from Source

```bash
# Clone the repository
git clone https://github.com/Ingenimax/agent-sdk-go
cd agent-sdk-go

# Build the CLI tool
make build-cli

# Install to system PATH (optional)
make install
```

### Quick Start

```bash
# Initialize configuration
./bin/agent-cli init

# Set your API key (example for OpenAI)
export OPENAI_API_KEY=your_api_key_here

# Run a simple query
./bin/agent-cli run "What's the weather in San Francisco?"

# Start interactive chat
./bin/agent-cli chat
```

## Commands

### `init` - Initialize Configuration

Set up the CLI with your preferred LLM provider and settings.

```bash
agent-cli init
```

Interactive setup will guide you through:
- LLM provider selection (OpenAI, Anthropic, Vertex AI, Ollama, vLLM)
- Model selection
- Default system prompt
- Basic configuration options

### `run` - Execute Single Prompt

Run an agent with a single prompt and get the response.

```bash
agent-cli run "Explain quantum computing in simple terms"
agent-cli run "Write a Python function to calculate fibonacci numbers"
```

### Direct Execution Mode

Execute prompts directly without subcommands, perfect for one-off tasks and automation.

```bash
# Simple direct execution
agent-cli --prompt "What is 2+2?"

# With MCP server configuration
agent-cli --prompt "List my EC2 instances" \
  --mcp-config ./aws_api_server.json \
  --allowedTools "suggest_aws_commands,call_aws" \
  --dangerously-skip-permissions

# With specific tool restrictions
agent-cli --prompt "Search for recent AI news" \
  --allowedTools "websearch"
```

**Direct Execution Options:**
- `--prompt <text>` - The prompt to execute (required)
- `--mcp-config <file>` - JSON file with MCP server configuration
- `--allowedTools <tool1,tool2>` - Comma-separated list of allowed tools
- `--dangerously-skip-permissions` - Skip permission checks (use with caution)

**Benefits:**
- **No Setup Required**: Works without running `init` first
- **Automation Friendly**: Perfect for scripts and CI/CD pipelines
- **Flexible Tool Control**: Specify exactly which tools can be used
- **MCP Integration**: Load MCP servers from external JSON configurations

### `chat` - Interactive Chat Mode

Start an interactive conversation with the agent.

```bash
agent-cli chat
```

Chat commands:
- `help` - Show available commands
- `clear` - Clear conversation history
- `config` - Show current configuration
- `exit`/`quit`/`bye` - Exit chat mode

### `task` - Execute Predefined Tasks

Run tasks defined in YAML configuration files.

```bash
# Execute a research task
agent-cli task --agent-config=agents.yaml --task-config=tasks.yaml --task=research_task --topic="Artificial Intelligence"

# Execute with custom variables
agent-cli task --agent-config=agents.yaml --task-config=tasks.yaml --task=analysis_task --var=dataset=sales_data --var=period=2024
```

### `generate` - Auto-Generate Configurations

Generate agent and task configurations from a system prompt.

```bash
# Generate configurations for a travel advisor
agent-cli generate --prompt="You are a travel advisor who helps users plan trips and find hidden gems" --output=./travel-configs

# Generate configurations for a code reviewer
agent-cli generate --prompt="You are a senior software engineer who reviews code for best practices and security" --output=./code-configs
```

### `config` - Manage Configuration

View and modify CLI configuration settings.

```bash
# Show current configuration
agent-cli config show

# Set configuration values
agent-cli config set provider anthropic
agent-cli config set model claude-3-5-sonnet-20241022
agent-cli config set system_prompt "You are a helpful coding assistant"

# Reset configuration
agent-cli config reset
```

### `list` - List Available Resources

Display information about available providers, models, and tools.

```bash
# List LLM providers
agent-cli list providers

# List popular models
agent-cli list models

# List available tools
agent-cli list tools

# Show current configuration
agent-cli list config
```

### `mcp` - Manage MCP Servers

Manage Model Context Protocol (MCP) servers for extended functionality.

```bash
# Add HTTP MCP server
agent-cli mcp add --type=http --url=http://localhost:8083/mcp --name=my-server

# Add stdio MCP server
agent-cli mcp add --type=stdio --command=python --args="-m,mcp_server" --name=python-server

# List configured MCP servers
agent-cli mcp list

# Test MCP server connection
agent-cli mcp test --name=my-server

# Remove MCP server
agent-cli mcp remove --name=my-server
```

**MCP Server Types:**
- **HTTP**: Connect to MCP servers via HTTP API
- **Stdio**: Connect to MCP servers via stdin/stdout communication

**Examples:**
```bash
# Add a time server (HTTP)
agent-cli mcp add --type=http --url=http://localhost:8083/mcp --name=time-server

# Add a file system server (stdio)
agent-cli mcp add --type=stdio --command=node --args="fs-server.js" --name=fs-server

# Add a Python-based server
agent-cli mcp add --type=stdio --command=python --args="-m,my_mcp_server" --name=python-tools
```

#### JSON Configuration Import/Export

You can also manage MCP servers using JSON configuration files, which is useful for sharing configurations or managing complex setups.

**JSON Configuration Format:**
```json
{
  "mcpServers": {
    "awslabs.aws-api-mcp-server": {
      "command": "docker",
      "args": [
        "run",
        "--rm",
        "--interactive",
        "--env",
        "AWS_REGION=us-west-2",
        "--env",
        "AWS_PROFILE=default",
        "--volume",
        "~/.aws:/app/.aws",
        "public.ecr.aws/awslabs-mcp/awslabs/aws-api-mcp-server:latest"
      ],
      "env": {
        "AWS_REGION": "us-west-2",
        "AWS_PROFILE": "default"
      }
    },
    "filesystem-server": {
      "command": "node",
      "args": ["fs-server.js"],
      "env": {
        "NODE_ENV": "production"
      }
    },
    "time-server": {
      "url": "http://localhost:8083/mcp",
      "env": {}
    }
  }
}
```

**Import/Export Commands:**
```bash
# Import MCP servers from JSON config
agent-cli mcp import --file=mcp-servers.json

# Export current MCP servers to JSON config
agent-cli mcp export --file=mcp-servers.json
```

**Features:**
- **Environment Variables**: Each server can specify environment variables in the `env` object
- **Duplicate Detection**: Import automatically skips servers that already exist
- **Type Detection**: Automatically detects HTTP vs stdio servers based on presence of `url` field
- **Preserve Configuration**: Export maintains all server settings including environment variables

#### kubectl-ai MCP Server Integration

The CLI includes built-in support for the [kubectl-ai MCP server](https://github.com/GoogleCloudPlatform/kubectl-ai), which provides Kubernetes management capabilities through natural language commands.

**Prerequisites:**
1. Install kubectl-ai: Follow instructions from [kubectl-ai repository](https://github.com/GoogleCloudPlatform/kubectl-ai)
2. Configure kubectl to access your Kubernetes cluster: `kubectl cluster-info`

**Adding kubectl-ai Servers:**
```bash
# Add basic kubectl-ai server
agent-cli mcp add --type=stdio --name=kubectl-ai --command=kubectl-ai --args="--mcp-server"

# Add enhanced kubectl-ai server with external tools
agent-cli mcp add --type=stdio --name=kubectl-ai-enhanced --command=kubectl-ai --args="--mcp-server,--external-tools"
```

**Available kubectl-ai Tools:**
- **`kubectl`**: Executes kubectl commands against your Kubernetes cluster
  - Translates natural language to kubectl commands
  - Examples: "List all pods" → `kubectl get pods`
  - Limitations: Interactive commands not supported
- **`bash`**: Executes general bash commands for shell operations

**Usage Examples:**
```bash
# Basic Kubernetes operations
agent-cli --prompt "List all pods in the default namespace" \
  --mcp-config ./kubectl_ai.json \
  --allowedTools "kubectl" \
  --dangerously-skip-permissions

# Use both kubectl and bash tools
agent-cli --prompt "Check if kubectl is working and list nodes" \
  --mcp-config ./kubectl_ai.json \
  --allowedTools "kubectl,bash" \
  --dangerously-skip-permissions

# Combined with AWS tools for EKS management
agent-cli --prompt "List my EKS clusters and show pods in production" \
  --mcp-config ./combined_servers.json \
  --allowedTools "suggest_aws_commands,call_aws,kubectl,bash" \
  --dangerously-skip-permissions
```

**Supported Kubernetes Operations:**
- **Cluster Management**: Get cluster info, node status, resource usage
- **Pod Operations**: List, describe, logs, exec into pods (non-interactive)
- **Deployment Management**: Create, update, scale deployments
- **Service Discovery**: List services, endpoints, ingress
- **Resource Monitoring**: Check resource usage, events, status
- **Troubleshooting**: Debug issues, analyze logs, check health

**kubectl-ai Server Modes:**
- **Basic Mode** (`--mcp-server`): Core Kubernetes operations with direct kubectl execution
- **Enhanced Mode** (`--mcp-server --external-tools`): All basic features plus tool discovery from other MCP servers

## Configuration

The CLI uses a JSON configuration file stored at `~/.agent-cli/config.json`.

### Example Configuration

```json
{
  "provider": "openai",
  "model": "gpt-4o-mini",
  "system_prompt": "You are a helpful AI assistant.",
  "temperature": 0.7,
  "max_iterations": 2,
  "org_id": "default-org",
  "conversation_id": "cli-session",
  "enable_tracing": false,
  "enable_memory": true,
  "enable_tools": true,
  "mcp_servers": [
    {
      "type": "http",
      "url": "http://localhost:8083/mcp"
    },
    {
      "type": "stdio",
      "command": "python",
      "args": ["-m", "mcp_server"]
    }
  ],
  "variables": {
    "default_topic": "technology",
    "output_format": "markdown"
  }
}
```

### Environment Variables

The CLI supports loading environment variables from a `.env` file in the current directory. This provides a convenient way to manage your API keys and configuration without setting them in your shell environment.

**Using .env file:**
1. Copy the example file: `cp env.example .env`
2. Edit `.env` with your actual API keys and settings
3. Run CLI commands - the `.env` file will be loaded automatically

The CLI respects the same environment variables as the main SDK:

#### LLM Provider Selection
```bash
# Set the default LLM provider (overrides config file)
export LLM_PROVIDER=openai          # or anthropic, vertex, ollama, vllm
```

#### LLM Providers
```bash
# OpenAI
export OPENAI_API_KEY=your_openai_key
export OPENAI_MODEL=gpt-4o-mini

# Anthropic
export ANTHROPIC_API_KEY=your_anthropic_key
export ANTHROPIC_MODEL=claude-3-5-sonnet-20241022

# Google Vertex AI
export GOOGLE_APPLICATION_CREDENTIALS=path/to/service-account.json
export GOOGLE_CLOUD_PROJECT=your-project-id

# Ollama (local)
export OLLAMA_BASE_URL=http://localhost:11434

# vLLM (local)
export VLLM_BASE_URL=http://localhost:8000
```

#### Tools
```bash
# Web Search
export GOOGLE_API_KEY=your_google_api_key
export GOOGLE_SEARCH_ENGINE_ID=your_search_engine_id

# GitHub
export GITHUB_TOKEN=your_github_token
```

#### Tracing
```bash
# Langfuse
export LANGFUSE_ENABLED=true
export LANGFUSE_SECRET_KEY=your_secret_key
export LANGFUSE_PUBLIC_KEY=your_public_key
export LANGFUSE_HOST=https://cloud.langfuse.com
```

## YAML Configuration Files

### Agent Configuration (`agents.yaml`)

Define agent personas with roles, goals, and capabilities.

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
              description:
                type: "string"
              source:
                type: "string"
        summary:
          type: "string"

analyst:
  role: >
    {topic} Data Analyst
  goal: >
    Analyze data and create insightful reports about {topic}
  backstory: >
    You're a meticulous analyst with expertise in {topic}. You excel at
    turning complex data into actionable insights and clear recommendations.
```

### Task Configuration (`tasks.yaml`)

Define specific tasks that agents can execute.

```yaml
research_task:
  description: >
    Conduct thorough research about {topic}.
    Focus on recent developments and provide credible sources.
  expected_output: >
    A structured report with key findings, sources, and summary
  agent: researcher
  output_file: "{topic}_research.json"

analysis_task:
  description: >
    Analyze the provided {dataset} for the {period} period.
    Identify trends, patterns, and actionable insights.
  expected_output: >
    A comprehensive analysis report with visualizations and recommendations
  agent: analyst
  output_file: "{dataset}_{period}_analysis.md"
```

## Examples

### Basic Usage

```bash
# Simple question
agent-cli run "What are the benefits of renewable energy?"

# Code generation
agent-cli run "Create a REST API in Go for a todo application"

# Data analysis request
agent-cli run "Analyze this CSV data and provide insights: [paste data]"
```

### Task Execution

```bash
# Research task with topic variable
agent-cli task \
  --agent-config=configs/agents.yaml \
  --task-config=configs/tasks.yaml \
  --task=research_task \
  --topic="Machine Learning"

# Analysis task with multiple variables
agent-cli task \
  --agent-config=configs/agents.yaml \
  --task-config=configs/tasks.yaml \
  --task=analysis_task \
  --var=dataset=sales_2024 \
  --var=period=Q1 \
  --var=format=pdf
```

### Configuration Generation

```bash
# Generate travel advisor configurations
agent-cli generate \
  --prompt="You are an expert travel advisor specializing in sustainable tourism and hidden gems" \
  --output=./travel-agent

# Generate code review configurations
agent-cli generate \
  --prompt="You are a senior software architect who reviews code for security, performance, and maintainability" \
  --output=./code-reviewer
```

### Interactive Chat

```bash
agent-cli chat
```

Example chat session:
```
🤖 You: Hello! I'm working on a Python project and need help with async programming.

🤖 Assistant: I'd be happy to help you with async programming in Python! Async programming allows you to write concurrent code that can handle multiple operations without blocking...

🤖 You: Can you show me an example of using asyncio with HTTP requests?

🤖 Assistant: Certainly! Here's a practical example using asyncio with aiohttp for making concurrent HTTP requests...
```

## Advanced Features

### MCP Server Integration

Connect to Model Context Protocol (MCP) servers for extended functionality.

```json
{
  "mcp_servers": [
    {
      "type": "http",
      "url": "http://localhost:8083/mcp"
    },
    {
      "type": "stdio",
      "command": "python",
      "args": ["-m", "my_mcp_server"]
    }
  ]
}
```

### Memory Management

The CLI maintains conversation history across interactions when memory is enabled.

```bash
# Memory is enabled by default
agent-cli config set enable_memory true

# Disable memory for stateless interactions
agent-cli config set enable_memory false
```

### Tool Integration

Enable various tools for enhanced agent capabilities:

- **Web Search**: Real-time information retrieval
- **GitHub**: Repository analysis and code operations
- **Calculator**: Mathematical computations
- **Custom MCP Tools**: Extensible tool ecosystem

### Tracing and Monitoring

Enable tracing to monitor agent performance and behavior.

```bash
# Enable Langfuse tracing
export LANGFUSE_ENABLED=true
export LANGFUSE_SECRET_KEY=your_secret_key
export LANGFUSE_PUBLIC_KEY=your_public_key

agent-cli config set enable_tracing true
```

## Troubleshooting

### Common Issues

1. **API Key Not Found**
   ```
   Error: OPENAI_API_KEY environment variable is required
   ```
   Solution: Set the appropriate API key for your chosen provider.

2. **Configuration Not Found**
   ```
   Error: Failed to read config file
   ```
   Solution: Run `agent-cli init` to create initial configuration.

3. **Tool Errors**
   ```
   Error: Failed to execute web search
   ```
   Solution: Ensure required API keys are set (GOOGLE_API_KEY, etc.).

4. **MCP Server Connection Failed**
   ```
   Warning: Failed to initialize MCP server
   ```
   Solution: Verify MCP server is running and accessible.

### Debug Mode

Enable verbose logging for troubleshooting:

```bash
export LOG_LEVEL=debug
agent-cli run "test query"
```

### Reset Configuration

If configuration becomes corrupted:

```bash
agent-cli config reset
agent-cli init
```

## Contributing

Contributions are welcome! Please see the main repository's [CONTRIBUTING.md](../../CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the MIT License - see the [LICENSE](../../LICENSE) file for details.

## Support

For support and questions:
- 📖 Documentation: [Agent SDK Docs](../../docs/)
- 🐛 Issues: [GitHub Issues](https://github.com/Ingenimax/agent-sdk-go/issues)
- 💬 Discussions: [GitHub Discussions](https://github.com/Ingenimax/agent-sdk-go/discussions)
