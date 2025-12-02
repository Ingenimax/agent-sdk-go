# Google Custom Search Engine (CSE) MCP Example - YAML Configuration

This example demonstrates how to use the [mcp-google-cse](https://github.com/Richard-Weiss/mcp-google-cse) MCP server with agent-sdk-go using YAML configuration files for agent definition.

## Overview

This example is similar to the programmatic example in `examples/mcp/google-cse`, but uses a **declarative YAML configuration** approach for both agent behavior and MCP server setup. The agent's personality, goals, behavioral settings, and MCP server configuration are all defined in YAML with environment variable expansion support.

This approach is ideal for:
- Defining complete agent configuration in YAML files
- Using environment variable expansion for secrets (${API_KEY}, ${ENGINE_ID})
- Easy modification without code changes
- Version control of agent specifications
- Environment-specific configurations

## Prerequisites

1. **Python and uvx**: Install uv (includes uvx) for running the MCP server:
   ```bash
   pip install uv
   ```

2. **Google Custom Search API**:
   - Get a Google Custom Search API key from [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
   - Create a Custom Search Engine at [Google CSE](https://programmablesearchengine.google.com/)
   - Note your Engine ID

3. **OpenAI API Key**: Required for the LLM client

## Environment Variables

Set the following environment variables:

```bash
export OPENAI_API_KEY="your-openai-api-key"
export API_KEY="your-google-api-key"
export ENGINE_ID="your-search-engine-id"
```

Or create a `.env` file:

```bash
OPENAI_API_KEY=your-openai-api-key
API_KEY=your-google-api-key
ENGINE_ID=your-search-engine-id
```

## Configuration Files

### agent.yaml

The YAML configuration file defines the agent's behavior:

```yaml
web_search_agent:
  role: "Web Search Research Assistant"
  goal: "Find accurate and up-to-date information from the web"
  backstory: "Expert research assistant with Google Custom Search access"

  max_iterations: 5
  require_plan_approval: false

  llm_config:
    temperature: 0.7

  mcp:
    mcpServers:
      google_cse:
        command: "uvx"
        args: ["mcp-google-cse"]
        env:
          API_KEY: "${API_KEY}"
          ENGINE_ID: "${ENGINE_ID}"
```

**Key Configuration Options:**
- `role`: The agent's role/persona
- `goal`: What the agent aims to achieve
- `backstory`: Context about the agent's expertise
- `max_iterations`: Maximum number of tool-calling iterations
- `require_plan_approval`: Whether to require user approval before executing plans (set to `false` for autonomous operation)
- `llm_config.temperature`: Controls response creativity (0.0-1.0)
- `mcp.mcpServers`: MCP server configurations with environment variable expansion

## Running the Example

```bash
cd examples/mcp/google-cse-yaml
go run main.go
```

## How It Works

1. **Configuration Loading**: The agent configuration is loaded from `agent.yaml`
2. **Environment Variable Expansion**: `${API_KEY}` and `${ENGINE_ID}` are replaced with actual values
3. **Agent Creation**: The agent is created using `NewAgentFromConfig()` with the YAML configuration
4. **MCP Server Integration**: The mcp-google-cse server is initialized based on the YAML config
5. **Query Execution**: The agent processes the query and uses Google Custom Search as needed

## Advantages of YAML Configuration

1. **Declarative**: Define what the agent should do, not how
2. **Reusable**: Share configurations across different applications
3. **Version Control**: Track changes to agent behavior over time
4. **Environment Specific**: Use different configs for dev/staging/prod
5. **No Code Changes**: Modify agent behavior by editing YAML files

## Customization

### Modifying the Query

Edit the query in `main.go`:

```go
query := "Your custom question here"
```

### Adjusting Agent Behavior

Modify `agent.yaml` to change:
- Temperature for more/less creative responses
- Max iterations for longer/shorter reasoning chains
- Role and backstory for different agent personas

### Adding More MCP Servers

Add additional MCP servers in the `mcpServers` section:

```yaml
mcp:
  mcpServers:
    google_cse:
      command: "uvx"
      args: ["mcp-google-cse"]
      env:
        API_KEY: "${API_KEY}"
        ENGINE_ID: "${ENGINE_ID}"

    filesystem:
      command: "npx"
      args: ["@modelcontextprotocol/server-filesystem", "."]
```

## Quota Limits

The free quota for Google Custom Search is **100 searches per day**. Each tool call counts as one search.

## Comparison with Programmatic Example

| Feature | Programmatic (`google-cse`) | YAML (`google-cse-yaml`) |
|---------|---------------------------|------------------------|
| Configuration | In Go code | In YAML file |
| Flexibility | High (full programmatic control) | Medium (YAML structure) |
| Maintenance | Requires recompilation | Edit and restart |
| Best For | Dynamic configuration | Static/templated configs |
