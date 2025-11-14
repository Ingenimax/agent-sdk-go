# Model Context Protocol (MCP) Guide

This guide explains how to use Model Context Protocol (MCP) with the agent-sdk-go to connect your AI agents to external tools and data sources.

## Table of Contents

1. [What is MCP?](#what-is-mcp)
2. [Quick Start](#quick-start)
3. [Configuration Methods](#configuration-methods)
4. [Common Use Cases](#common-use-cases)
5. [Error Handling](#error-handling)
6. [Performance Considerations](#performance-considerations)
7. [Security Best Practices](#security-best-practices)
8. [Troubleshooting](#troubleshooting)

## What is MCP?

Model Context Protocol (MCP) is an open standard that enables AI agents to securely connect to external tools and data sources. It provides a standardized way to:

- Access external APIs and services
- Execute tools and commands
- Retrieve contextual information
- Maintain secure connections with proper authentication

### Benefits

- **Standardized**: One protocol for all tool integrations
- **Secure**: Built-in authentication and permission controls
- **Extensible**: Easy to add new tools and services
- **Reliable**: Automatic retry and error handling

## Quick Start

### 1. Simple URL-Based Setup

```go
package main

import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
    // Create an agent with MCP servers using simple URLs
    myAgent, err := agent.NewAgent(
        agent.WithLLM(openai.NewClient("your-api-key")),
        agent.WithMCPURLs(
            "stdio://filesystem-server/usr/local/bin/mcp-filesystem",
            "http://localhost:8080/mcp",
            "https://api.example.com/mcp?token=your-token",
        ),
    )
    if err != nil {
        panic(err)
    }

    // Use the agent
    response, err := myAgent.Run(context.Background(), "List files in the current directory")
    if err != nil {
        panic(err)
    }

    println(response)
}
```

### 2. Using Presets

```go
// Use predefined configurations for common services
myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMCPPresets("filesystem", "github", "postgres"),
)
```

### 3. Using Builder Pattern

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/mcp"

// Create MCP configuration using builder
builder := mcp.NewBuilder().
    WithRetry(3, time.Second).
    WithTimeout(30*time.Second).
    AddStdioServer("filesystem", "/usr/local/bin/mcp-filesystem").
    AddHTTPServerWithAuth("api-server", "https://api.example.com/mcp", "your-token").
    AddPreset("github")

servers, lazyConfigs, err := builder.Build(context.Background())
if err != nil {
    panic(err)
}

myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMCPServers(servers),
    agent.WithLazyMCPConfigs(lazyConfigs),
)
```

## Configuration Methods

### 1. URL-Based Configuration

The simplest way to configure MCP servers is using URL strings:

#### Stdio Servers
```
stdio://server-name/path/to/executable?arg1=value1&arg2=value2
```

Example:
```go
"stdio://filesystem/usr/local/bin/mcp-filesystem?root=/home/user"
```

#### HTTP Servers
```
http://host:port/path
https://host:port/path?token=your-token
```

Examples:
```go
"http://localhost:8080/mcp"
"https://api.example.com/mcp?token=abc123"
```

#### Preset Servers
```
mcp://preset-name
```

Example:
```go
"mcp://github"  // Uses the predefined GitHub preset
```

### 2. Available Presets

The SDK includes presets for common MCP servers:

| Preset | Description | Required Environment Variables |
|--------|-------------|------------------------------|
| `filesystem` | File system operations | None |
| `github` | GitHub API operations | `GITHUB_TOKEN` |
| `git` | Git repository operations | None |
| `postgres` | PostgreSQL database | `DATABASE_URL` |
| `slack` | Slack integration | `SLACK_BOT_TOKEN`, `SLACK_TEAM_ID` |
| `gdrive` | Google Drive operations | `GOOGLE_CREDENTIALS` |
| `puppeteer` | Web automation | None |
| `memory` | Knowledge management | None |
| `fetch` | HTTP requests | None |
| `brave-search` | Brave Search API | `BRAVE_API_KEY` |
| `time` | Date/time operations | None |
| `aws` | AWS operations | `AWS_REGION`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` |

#### List Available Presets

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/mcp"

presets := mcp.ListPresets()
for _, preset := range presets {
    info, err := mcp.GetPresetInfo(preset)
    if err == nil {
        fmt.Printf("%s: %s\n", preset, info)
    }
}
```

### 3. Advanced Configuration

For more control, use the builder pattern:

```go
builder := mcp.NewBuilder()

// Configure retry behavior
builder.WithRetry(5, 2*time.Second)

// Set connection timeout
builder.WithTimeout(60*time.Second)

// Disable health checks for faster startup
builder.WithHealthCheck(false)

// Add servers with different methods
builder.
    AddStdioServer("custom-tool", "/path/to/tool", "--arg1", "value1").
    AddHTTPServer("api-server", "https://api.example.com/mcp").
    AddPreset("github")

// Build configurations
lazyConfigs, err := builder.BuildLazy()
```

## Common Use Cases

### 1. File System Operations

```go
myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMCPPresets("filesystem"),
)

// Agent can now read, write, and manipulate files
response, err := myAgent.Run(ctx, "Create a new file called hello.txt with content 'Hello World'")
```

### 2. Database Queries

```go
// Set DATABASE_URL environment variable first
os.Setenv("DATABASE_URL", "postgresql://user:pass@localhost/db")

myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMCPPresets("postgres"),
)

// Agent can now query the database
response, err := myAgent.Run(ctx, "Show me all users from the users table")
```

### 3. GitHub Operations

```go
// Set GITHUB_TOKEN environment variable first
os.Setenv("GITHUB_TOKEN", "your-github-token")

myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMCPPresets("github"),
)

// Agent can now interact with GitHub
response, err := myAgent.Run(ctx, "List all repositories for the user 'octocat'")
```

### 4. Web Automation

```go
myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMCPPresets("puppeteer"),
)

// Agent can now automate web browsers
response, err := myAgent.Run(ctx, "Take a screenshot of https://example.com")
```

### 5. Multi-Service Integration

```go
myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMCPPresets("filesystem", "github", "slack"),
    agent.WithMCPURLs(
        "http://localhost:8080/custom-api",
        "stdio://custom-tool/path/to/tool",
    ),
)

// Agent now has access to multiple services
response, err := myAgent.Run(ctx, "Read the README.md file, update it based on recent commits, and post a summary to Slack")
```

## Error Handling

The SDK provides structured error handling with detailed error classification:

### Error Types

- `CONNECTION_ERROR`: Network connectivity issues
- `TIMEOUT_ERROR`: Operation timeouts
- `AUTHENTICATION_ERROR`: Authentication failures
- `TOOL_NOT_FOUND`: Requested tool doesn't exist
- `TOOL_INVALID_ARGS`: Invalid tool arguments
- `SERVER_STARTUP_ERROR`: Server failed to start
- `CONFIGURATION_ERROR`: Invalid configuration

### Handling Errors

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/mcp"

response, err := myAgent.Run(ctx, "some query")
if err != nil {
    if mcpErr, ok := err.(*mcp.MCPError); ok {
        fmt.Printf("MCP Error Type: %s\n", mcpErr.ErrorType)
        fmt.Printf("Is Retryable: %t\n", mcpErr.IsRetryable())
        fmt.Printf("User-friendly message: %s\n", mcp.FormatUserFriendlyError(err))

        // Handle specific error types
        switch mcpErr.ErrorType {
        case mcp.MCPErrorTypeConnection:
            fmt.Println("Check your network connection")
        case mcp.MCPErrorTypeAuthentication:
            fmt.Println("Check your API keys and credentials")
        case mcp.MCPErrorTypeToolNotFound:
            fmt.Println("The requested tool is not available")
        }
    } else {
        fmt.Printf("Other error: %v\n", err)
    }
}
```

### Retry Configuration

```go
// Configure automatic retry with exponential backoff
builder := mcp.NewBuilder().
    WithRetry(5, time.Second).  // 5 attempts, starting with 1 second delay
    WithTimeout(30*time.Second)

// Retry is automatic for retryable errors
```

## Performance Considerations

### 1. Lazy Initialization

MCP servers are initialized on-demand by default to improve startup performance:

```go
// Servers are only started when first used
myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMCPURLs("stdio://slow-server/path/to/server"),
)
// Server startup happens when first tool is called
```

### 2. Connection Pooling

For HTTP-based MCP servers, connections are reused automatically:

```go
// Multiple calls to the same HTTP server reuse connections
myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMCPURLs("https://api.example.com/mcp"),
)
```

### 3. Health Checks

Disable health checks for faster startup:

```go
builder := mcp.NewBuilder().
    WithHealthCheck(false).  // Skip initial health checks
    AddPreset("github")
```

### 4. Timeouts

Configure appropriate timeouts for your use case:

```go
builder := mcp.NewBuilder().
    WithTimeout(60*time.Second).  // Longer timeout for slow servers
    AddStdioServer("slow-server", "/path/to/slow/server")
```

## Security Best Practices

### 1. Environment Variables

Store sensitive information in environment variables:

```bash
export GITHUB_TOKEN="your-token-here"
export DATABASE_URL="postgresql://user:pass@localhost/db"
export API_KEY="your-api-key"
```

### 2. Token Management

Use secure token storage and rotation:

```go
// Good: Token from environment
token := os.Getenv("API_TOKEN")
if token == "" {
    log.Fatal("API_TOKEN environment variable is required")
}

// Good: Pass token securely
builder.AddHTTPServerWithAuth("api", "https://api.example.com/mcp", token)
```

### 3. Input Validation

The SDK automatically validates server configurations and sanitizes inputs to prevent injection attacks.

### 4. Network Security

- Use HTTPS for remote MCP servers
- Validate TLS certificates
- Use firewall rules to restrict access

```go
// Always use HTTPS for remote servers
builder.AddHTTPServer("secure-api", "https://api.example.com/mcp")  // Good
```

### 5. Principle of Least Privilege

Only enable MCP servers and tools that your agent actually needs:

```go
// Good: Only include necessary servers
myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMCPPresets("filesystem"),  // Only filesystem access
)

// Avoid: Adding unnecessary servers
myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMCPPresets("filesystem", "github", "aws", "postgres"),  // Too broad
)
```

## Troubleshooting

### Common Issues

#### 1. Command Not Found (Stdio Servers)

**Error**: `server not found` or `command not found`

**Solutions**:
- Ensure the MCP server command is installed
- Check that the command is in your PATH
- Use absolute paths in configurations

```go
// Use absolute path
builder.AddStdioServer("tool", "/usr/local/bin/mcp-tool")
```

#### 2. Connection Refused (HTTP Servers)

**Error**: `connection refused`

**Solutions**:
- Verify the server is running
- Check the URL and port
- Verify firewall settings

```bash
# Test the connection manually
curl -v http://localhost:8080/mcp
```

#### 3. Authentication Failures

**Error**: `authentication failed` or `unauthorized`

**Solutions**:
- Verify API keys and tokens
- Check environment variables
- Ensure tokens have required permissions

```bash
# Check environment variables
echo $GITHUB_TOKEN
echo $API_KEY
```

#### 4. Timeout Issues

**Error**: `timeout` or `deadline exceeded`

**Solutions**:
- Increase timeout values
- Check network connectivity
- Verify server performance

```go
// Increase timeout
builder := mcp.NewBuilder().
    WithTimeout(60*time.Second).
    WithRetry(3, 5*time.Second)
```

#### 5. Tool Not Found

**Error**: `tool not found`

**Solutions**:
- List available tools first
- Check tool names and spelling
- Verify server is properly configured

```go
// Debug: List available tools
servers, _, err := builder.Build(ctx)
if err == nil && len(servers) > 0 {
    tools, err := servers[0].ListTools(ctx)
    if err == nil {
        for _, tool := range tools {
            fmt.Printf("Available tool: %s - %s\n", tool.Name, tool.Description)
        }
    }
}
```

### Debug Mode

Enable detailed logging for troubleshooting:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/logging"

// Create logger with debug level
logger := logging.New().WithLevel("debug")

// Use in MCP configuration
config := mcp.StdioServerConfig{
    Command: "/path/to/server",
    Logger:  logger,
}
```

### Getting Help

1. Check the error message classification for specific guidance
2. Use the user-friendly error formatting
3. Enable debug logging to see detailed information
4. Test MCP servers independently before integrating
5. Check the official MCP documentation at https://modelcontextprotocol.io

## Next Steps

- Explore [quickstart examples](../examples/quickstart/)
- Learn about [advanced patterns](../examples/advanced/)
- Check out [community MCP servers](https://github.com/modelcontextprotocol)
- Read the [MCP specification](https://spec.modelcontextprotocol.io/)