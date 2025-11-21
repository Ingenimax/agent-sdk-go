# LLM YAML Configuration Guide

## Overview

This guide explains how to configure LLM providers declaratively using YAML configuration files in the Agent SDK. This feature allows you to specify LLM provider settings in your agent configuration files while maintaining security best practices for sensitive credentials.

## Table of Contents

- [Quick Start](#quick-start)
- [Configuration Schema](#configuration-schema)
- [Supported Providers](#supported-providers)
- [Security Best Practices](#security-best-practices)
- [Advanced Configuration](#advanced-configuration)
- [Migration Guide](#migration-guide)
- [Troubleshooting](#troubleshooting)

## Quick Start

### Basic Example

Add an `llm_provider` section to your agent YAML configuration:

```yaml
# agents.yaml
my_agent:
  role: "AI Assistant"
  goal: "Help users with their questions"
  backstory: "You are a knowledgeable assistant"

  # LLM Provider Configuration
  llm_provider:
    provider: "${LLM_PROVIDER:-anthropic}"
    model: "${ANTHROPIC_MODEL:-claude-sonnet-4-20250514}"
    config:
      temperature: 0.7
      max_tokens: 4096
      api_key: "${ANTHROPIC_API_KEY}"  # Always use env vars for credentials
```

### Usage in Code

```go
package main

import (
    "log"
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
)

func main() {
    // Load agent configuration from YAML
    configs, err := agent.LoadAgentConfigsFromFile("agents.yaml")
    if err != nil {
        log.Fatal(err)
    }

    // Create agent with YAML config - LLM will be auto-created
    myAgent, err := agent.NewAgentFromConfig(
        "my_agent",
        configs,
        nil, // environment variables
    )
    if err != nil {
        log.Fatal(err)
    }

    // Agent is ready with configured LLM
    response, err := myAgent.Run("What is the weather today?")
    if err != nil {
        log.Fatal(err)
    }
    log.Println(response)
}
```

## Configuration Schema

### LLMProviderYAML Structure

```yaml
llm_provider:
  provider: string       # Required: Provider name
  model: string         # Optional: Model identifier
  config:               # Optional: Provider-specific configuration
    <key>: <value>      # Key-value pairs for provider settings
```

### Field Descriptions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `provider` | string | Yes | LLM provider name (`anthropic`, `openai`, `gemini`, `azure_openai`, `ollama`, `vllm`) |
| `model` | string | No | Model identifier (defaults to provider's default model) |
| `config` | map | No | Provider-specific configuration options |

## Supported Providers

### Anthropic

```yaml
llm_provider:
  provider: "anthropic"
  model: "${ANTHROPIC_MODEL:-claude-3-5-sonnet-latest}"
  config:
    api_key: "${ANTHROPIC_API_KEY}"           # Required
    temperature: 0.7                           # Optional (0.0-1.0)
    max_tokens: 4096                          # Optional
    top_p: 0.95                               # Optional
    top_k: 40                                 # Optional

    # Vertex AI Configuration (optional)
    vertex_ai_project: "${VERTEX_AI_PROJECT}"
    vertex_ai_region: "${VERTEX_AI_REGION}"   # Use 'region' for global endpoints
    google_application_credentials: "${VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT}"

    # Advanced options
    enable_streaming: true
    enable_reasoning: true
    reasoning_budget: 10000
```

#### Real-World Example: StarOps DeepOps Agent

Here's a complete production example from the StarOps DeepOps Agent:

```yaml
starops_deepops_agent:
  role: "Platform Engineering Expert"
  goal: "Orchestrate complex infrastructure tasks with deep operational expertise"

  # LLM Provider Configuration - Anthropic with Vertex AI
  llm_provider:
    provider: "${LLM_PROVIDER:-anthropic}"
    model: "${ANTHROPIC_MODEL:-claude-sonnet-4-5@20250929}"
    config:
      api_key: "${ANTHROPIC_API_KEY}"
      vertex_ai_region: "${VERTEX_AI_REGION}"
      vertex_ai_project: "${VERTEX_AI_PROJECT}"
      google_application_credentials: "${VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT}"

  backstory: |
    You are the DeepOps agent - a platform engineering expert that thinks and acts like a seasoned infrastructure engineer.

  # Behavioral settings
  max_iterations: 30
  require_plan_approval: false

  # Complex configuration objects
  stream_config:
    buffer_size: 150
    include_tool_progress: true
    include_intermediate_messages: true

  llm_config:
    temperature: 0.7
    top_p: 0.95
    enable_reasoning: true
    reasoning: "comprehensive"
    reasoning_budget: 25000

  # Memory configuration for complex analysis
  memory:
    type: "redis"
    config:
      address: "${REDIS_ADDRESS}"
      password: "${REDIS_PASSWORD}"
      db: 0
      ttl_hours: 48
      key_prefix: "starops-deepops:"
      max_message_size: 2097152  # 2MB for larger infrastructure analysis data
```

This configuration was successfully migrated from hardcoded Go configuration to YAML, providing:

1. **Centralized Configuration**: All settings in one place
2. **Environment Flexibility**: Easy switching between dev/prod environments
3. **Vertex AI Integration**: Production-ready Anthropic via Google Cloud
4. **Advanced Features**: Reasoning, streaming, and comprehensive memory management

### OpenAI

```yaml
llm_provider:
  provider: "openai"
  model: "${OPENAI_MODEL:-gpt-4-turbo-preview}"
  config:
    api_key: "${OPENAI_API_KEY}"              # Required
    organization: "${OPENAI_ORG_ID}"          # Optional
    base_url: "${OPENAI_BASE_URL}"            # Optional (for custom endpoints)
    temperature: 0.7                           # Optional (0.0-2.0)
    max_tokens: 4096                          # Optional
    top_p: 1.0                                # Optional
    frequency_penalty: 0.0                    # Optional (-2.0-2.0)
    presence_penalty: 0.0                     # Optional (-2.0-2.0)

    # Advanced options
    enable_streaming: true
    timeout_seconds: 30
```

### Google Gemini

```yaml
llm_provider:
  provider: "gemini"
  model: "${GEMINI_MODEL:-gemini-1.5-pro}"
  config:
    api_key: "${GEMINI_API_KEY}"              # Required (or use ADC)
    project: "${GOOGLE_CLOUD_PROJECT}"        # Optional (for Vertex AI)
    location: "${GOOGLE_CLOUD_LOCATION}"      # Optional (for Vertex AI)
    temperature: 0.7                           # Optional (0.0-1.0)
    max_output_tokens: 4096                   # Optional
    top_p: 0.95                               # Optional
    top_k: 40                                 # Optional

    # Safety settings
    harm_block_threshold: "BLOCK_MEDIUM_AND_ABOVE"

    # Advanced options
    enable_streaming: true
    use_vertex_ai: false                      # Use Vertex AI instead of Gemini API
```

### Azure OpenAI

```yaml
llm_provider:
  provider: "azure_openai"
  model: "${AZURE_OPENAI_DEPLOYMENT}"
  config:
    api_key: "${AZURE_OPENAI_API_KEY}"        # Required
    endpoint: "${AZURE_OPENAI_ENDPOINT}"      # Required
    api_version: "${AZURE_API_VERSION:-2024-02-01}"  # Required
    deployment: "${AZURE_OPENAI_DEPLOYMENT}"  # Required
    temperature: 0.7                           # Optional
    max_tokens: 4096                          # Optional
    top_p: 1.0                                # Optional

    # Advanced options
    enable_streaming: true
    timeout_seconds: 60
```

### Ollama (Local)

```yaml
llm_provider:
  provider: "ollama"
  model: "${OLLAMA_MODEL:-llama3}"
  config:
    base_url: "${OLLAMA_BASE_URL:-http://localhost:11434}"
    temperature: 0.7
    max_tokens: 4096

    # Ollama-specific options
    num_ctx: 4096                             # Context window size
    num_gpu: 1                                # Number of GPUs to use
    num_thread: 8                             # Number of CPU threads
```

### vLLM (Self-hosted)

```yaml
llm_provider:
  provider: "vllm"
  model: "${VLLM_MODEL}"
  config:
    base_url: "${VLLM_BASE_URL}"              # Required
    api_key: "${VLLM_API_KEY}"                # Optional
    temperature: 0.7
    max_tokens: 4096
    top_p: 0.95

    # vLLM-specific options
    best_of: 1
    use_beam_search: false
    n: 1
```

## Security Best Practices

### 1. Never Hardcode Credentials

❌ **Wrong:**
```yaml
llm_provider:
  provider: "openai"
  config:
    api_key: "sk-abc123def456..."  # NEVER DO THIS
```

✅ **Correct:**
```yaml
llm_provider:
  provider: "openai"
  config:
    api_key: "${OPENAI_API_KEY}"   # Use environment variable
```

### 2. Use Environment Variables

Set environment variables before running your application:

```bash
# Using export
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."

# Using .env file
cat > .env << EOF
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
EOF

# Run your application
./myapp
```

### 3. Validate Configuration

The SDK automatically validates:
- Required fields are present
- API keys are not hardcoded
- Provider names are valid
- Model names follow expected patterns

### 4. Use Default Values

Leverage defaults with environment variable fallbacks:

```yaml
llm_provider:
  provider: "anthropic"
  model: "${ANTHROPIC_MODEL:-claude-3-5-sonnet-latest}"  # Fallback to default
  config:
    temperature: "${LLM_TEMPERATURE:-0.7}"                # Configurable with default
```

## Advanced Configuration

### Sub-Agent LLM Configuration

Sub-agents can have different LLM providers:

```yaml
lead_agent:
  role: "Lead Coordinator"
  goal: "Coordinate tasks"
  backstory: "Expert coordinator"

  llm_provider:
    provider: "anthropic"
    model: "claude-3-5-sonnet-latest"
    config:
      api_key: "${ANTHROPIC_API_KEY}"
      temperature: 0.3  # Lower for coordination

  sub_agents:
    creative_agent:
      role: "Creative Writer"
      goal: "Generate creative content"
      backstory: "Creative specialist"

      llm_provider:
        provider: "openai"
        model: "gpt-4-turbo-preview"
        config:
          api_key: "${OPENAI_API_KEY}"
          temperature: 0.9  # Higher for creativity

    analysis_agent:
      role: "Data Analyst"
      goal: "Analyze data"
      backstory: "Data specialist"

      # Inherits parent's LLM provider if not specified
```

### Environment-Specific Configuration

Use different configurations per environment:

```yaml
# production.yaml
llm_provider:
  provider: "anthropic"
  model: "claude-3-5-sonnet-latest"
  config:
    api_key: "${ANTHROPIC_API_KEY}"
    temperature: 0.3
    max_tokens: 4096
    enable_reasoning: true

# development.yaml
llm_provider:
  provider: "ollama"
  model: "llama3"
  config:
    base_url: "http://localhost:11434"
    temperature: 0.7
    max_tokens: 2048
```

### Combining with Programmatic Configuration

YAML configuration can be overridden programmatically:

```go
// Load YAML config
configs, _ := agent.LoadAgentConfigsFromFile("agents.yaml")

// Create custom LLM client
customLLM := openai.NewClient(apiKey,
    openai.WithCustomEndpoint("https://my-proxy.com"),
    openai.WithRetry(3),
)

// Override YAML LLM configuration
myAgent, _ := agent.NewAgentFromConfig(
    "my_agent",
    configs,
    nil,
    agent.WithLLM(customLLM),  // This overrides YAML config
)
```

## Migration Guide

### From Environment Variables Only

**Before:**
```go
// Old approach - manual LLM creation
llmClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
agent := agent.NewAgent(
    agent.WithLLM(llmClient),
    // ... other options
)
```

**After:**
```yaml
# agents.yaml
my_agent:
  llm_provider:
    provider: "openai"
    config:
      api_key: "${OPENAI_API_KEY}"
```

```go
// New approach - automatic LLM creation
configs, _ := agent.LoadAgentConfigsFromFile("agents.yaml")
agent, _ := agent.NewAgentFromConfig("my_agent", configs, nil)
```

### From Code-Based Configuration

**Before:**
```go
llm := anthropic.NewClient(apiKey,
    anthropic.WithModel("claude-3-5-sonnet-latest"),
    anthropic.WithTemperature(0.7),
    anthropic.WithMaxTokens(4096),
)
```

**After:**
```yaml
llm_provider:
  provider: "anthropic"
  model: "claude-3-5-sonnet-latest"
  config:
    api_key: "${ANTHROPIC_API_KEY}"
    temperature: 0.7
    max_tokens: 4096
```

### Complete Migration Example: StarOps DeepOps Agent

This section shows a real-world migration from hardcoded Go configuration to YAML-based configuration.

#### Step 1: Analyze Existing Hardcoded Configuration

**Before (internal/tracing/tracing.go):**
```go
// Hardcoded LLM creation functions
func createAnthropicClient(cfg *config.Config) (interfaces.LLM, error) {
    apiKey := cfg.Anthropic.APIKey
    var options []anthropic.Option

    if cfg.Anthropic.Model != "" {
        options = append(options, anthropic.WithModel(cfg.Anthropic.Model))
    }

    if cfg.Anthropic.VertexAIRegion != "" && cfg.Anthropic.VertexAIProject != "" {
        options = append(options, anthropic.WithVertexAI(cfg.Anthropic.VertexAIRegion, cfg.Anthropic.VertexAIProject))
    }

    return anthropic.NewClient(apiKey, options...), nil
}

func CreateAgentOptions(cfg *config.Config, llmClient interfaces.LLM, ...) []agent.Option {
    return []agent.Option{
        agent.WithName("StarOps-DeepOps-Agent"),
        agent.WithLLM(llmClient),  // Hardcoded LLM injection
        agent.WithMaxIterations(25), // Hardcoded value
        // ... more hardcoded settings
    }
}
```

#### Step 2: Create YAML Configuration

**After (config/agents.yaml):**
```yaml
starops_deepops_agent:
  role: "Platform Engineering Expert"
  goal: "Orchestrate complex infrastructure tasks with deep operational expertise"

  # LLM Provider Configuration - replaces hardcoded createAnthropicClient
  llm_provider:
    provider: "${LLM_PROVIDER:-anthropic}"
    model: "${ANTHROPIC_MODEL:-claude-sonnet-4-5@20250929}"
    config:
      api_key: "${ANTHROPIC_API_KEY}"
      vertex_ai_region: "${VERTEX_AI_REGION}"
      vertex_ai_project: "${VERTEX_AI_PROJECT}"
      google_application_credentials: "${VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT}"

  # Agent Configuration - replaces hardcoded CreateAgentOptions
  max_iterations: 30              # Was hardcoded as 25
  require_plan_approval: false

  stream_config:
    buffer_size: 150              # Was hardcoded as 100
    include_tool_progress: true
    include_intermediate_messages: true

  llm_config:
    temperature: 0.7
    top_p: 0.95
    enable_reasoning: true
    reasoning: "comprehensive"
    reasoning_budget: 25000

  memory:
    type: "redis"
    config:
      address: "${REDIS_ADDRESS}"
      password: "${REDIS_PASSWORD}"
      db: 0
      ttl_hours: 48
      key_prefix: "starops-deepops:"
      max_message_size: 2097152
```

#### Step 3: Update Application Code

**Before (cmd/serve.go):**
```go
func serve(cmd *cobra.Command, args []string) error {
    cfg, err := config.LoadConfig()

    // Create LLM and agent with hardcoded configuration
    llm, agentTracer, err := tracing.InitializeLLMAndAgentTracing(cfg)
    appTools, err := server.SetupTools(cfg, llm)

    // Hardcoded agent options
    agentOptions := server.CreateAgentOptions(cfg, llm, systemPrompt, appTools, redisMemory, agentTracer, llmConfig)
    deepOpsAgent, err := server.CreateAgent(agentOptions)

    // ... rest of setup
}
```

**After (cmd/serve.go):**
```go
func serve(cmd *cobra.Command, args []string) error {
    cfg, err := config.LoadConfig()

    // Initialize agent tracing only (LLM handled by YAML)
    agentTracer, err := tracing.InitializeAgentTracing(cfg)

    // Use dummy LLM client for tool setup
    dummyLLMClient, err := tracing.CreateDummyLLMClient()
    appTools, err := server.SetupTools(cfg, dummyLLMClient)

    // Create agent using YAML configuration
    deepOpsAgent, err := server.CreateAgentFromYAML(cfg, appTools, agentTracer, configPath)

    // ... rest of setup
}
```

#### Step 4: Create YAML-Based Agent Creation Function

**New (internal/server/agent.go):**
```go
func CreateAgentFromYAML(cfg *config.Config, tools []interfaces.Tool, agentTracer interfaces.Tracer, configPath string) (*agent.Agent, error) {
    // Load system prompt from file
    systemPrompt, err := LoadSystemPrompt()
    if err != nil {
        return nil, fmt.Errorf("failed to load system prompt: %w", err)
    }

    // Use configPath if provided, otherwise use default
    yamlConfigPath := configPath
    if yamlConfigPath == "" {
        yamlConfigPath = "config/agents.yaml"
    }

    // Load YAML configuration
    yamlConfig, err := agent.LoadAgentConfigsFromFile(yamlConfigPath)
    if err != nil {
        return nil, fmt.Errorf("failed to load YAML agent configuration: %w", err)
    }

    agentConfig, exists := yamlConfig["starops_deepops_agent"]
    if !exists {
        return nil, fmt.Errorf("starops_deepops_agent configuration not found")
    }

    // Create agent with YAML config
    agentConfigs := map[string]agent.AgentConfig{
        "starops_deepops_agent": agentConfig,
    }

    var options []agent.Option
    options = append(options, agent.WithSystemPrompt(systemPrompt))

    if len(tools) > 0 {
        options = append(options, agent.WithTools(tools...))
    }

    if agentTracer != nil {
        options = append(options, agent.WithTracer(agentTracer))
    }

    return agent.NewAgentFromConfig(
        "starops_deepops_agent",
        agentConfigs,
        nil, // environment variables loaded automatically
        options...,
    )
}
```

#### Step 5: Deprecate Hardcoded Functions

**Updated (internal/tracing/tracing.go):**
```go
// Deprecated hardcoded functions - replaced with YAML configuration
func InitializeLLMWithTracing(cfg *config.Config) (interfaces.LLM, error) {
    return nil, fmt.Errorf("InitializeLLMWithTracing is deprecated - use YAML-based LLM configuration instead")
}

func createAnthropicClient(cfg *config.Config) (interfaces.LLM, error) {
    return nil, fmt.Errorf("createAnthropicClient is deprecated - use YAML-based LLM configuration instead")
}

// Keep only necessary utility functions
func CreateDummyLLMClient() (interfaces.LLM, error) {
    return anthropic.NewClient("dummy-key"), nil
}

func InitializeAgentTracing(cfg *config.Config) (interfaces.Tracer, error) {
    // Agent tracing logic remains - only LLM creation moved to YAML
    // ... implementation
}
```

#### Results

This migration achieved:

✅ **200+ lines of hardcoded configuration removed**
✅ **All agent settings now configurable via YAML**
✅ **Environment-specific configuration support**
✅ **Vertex AI integration maintained and simplified**
✅ **Clean separation between tool setup and agent creation**
✅ **Backward compatibility maintained during transition**

The agent now starts with:
```bash
# Development
LLM_PROVIDER=anthropic ANTHROPIC_API_KEY=test-key ./starops-deepops-agent serve

# Production (using Vertex AI)
VERTEX_AI_REGION=global VERTEX_AI_PROJECT=production ./starops-deepops-agent serve
```

## Troubleshooting

### Common Issues

#### 1. API Key Not Found

**Error:** `API key not found for provider`

**Solution:** Ensure environment variable is set:
```bash
echo $OPENAI_API_KEY  # Should output your key
```

#### 2. Invalid Provider Name

**Error:** `Unsupported LLM provider: <name>`

**Solution:** Use one of the supported provider names:
- `anthropic`
- `openai`
- `gemini`
- `azure_openai`
- `ollama`
- `vllm`

#### 3. Model Not Available

**Error:** `Model <model> not available for provider`

**Solution:** Check provider documentation for valid model names:
- Anthropic: `claude-3-5-sonnet-latest`, `claude-3-opus-latest`
- OpenAI: `gpt-4-turbo-preview`, `gpt-4`, `gpt-3.5-turbo`
- Gemini: `gemini-1.5-pro`, `gemini-1.5-flash`

#### 4. Configuration Validation Failed

**Error:** `Invalid configuration for provider`

**Solution:** Check required fields for your provider:
- All providers need `api_key` (except Ollama with local setup)
- Azure OpenAI needs `endpoint`, `api_version`, and `deployment`
- Vertex AI needs `project` and `location`

### Debug Logging

Enable debug logging to troubleshoot LLM configuration:

```yaml
runtime:
  log_level: "debug"

llm_provider:
  provider: "anthropic"
  config:
    api_key: "${ANTHROPIC_API_KEY}"
    # Debug flags
    log_requests: true
    log_responses: true
```

### Validation Tool

Use the CLI tool to validate your configuration:

```bash
# Validate YAML configuration
agent-cli validate agents.yaml

# Test LLM connection
agent-cli test-llm agents.yaml my_agent
```

## Best Practices Summary

1. **Always use environment variables for credentials**
2. **Set appropriate temperature based on use case** (0.0-0.3 for factual, 0.7-1.0 for creative)
3. **Configure timeouts for production environments**
4. **Use model-specific features when available** (e.g., reasoning for Anthropic)
5. **Test configuration in development before production**
6. **Monitor token usage and costs**
7. **Implement retry logic for production**
8. **Use local models (Ollama) for development when possible**

## Related Documentation

- [Agent Configuration Guide](agent.md)
- [YAML Configuration Enhancement Plan](yaml-configuration-enhancement-plan.md)
- [Environment Variables](environment_variables.md)
- [LLM Providers](llm.md)
- [Security Best Practices](../README.md#security)