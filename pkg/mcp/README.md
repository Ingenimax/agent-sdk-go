# MCP Integration for Agent SDK Go

This package provides integration with the [Model Context Protocol (MCP)](https://mcpgolang.com/) for the Agent SDK Go. It allows agents to connect to MCP servers and use their tools.

## Overview

The MCP integration allows agents to:

1. Connect to MCP servers using different transports (stdio, HTTP)
2. List and use tools provided by MCP servers
3. Convert MCP tools to agent tools

## Usage

### Creating an MCP Server Connection

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/mcp"
)

// Create an HTTP-based MCP server client
httpServer, err := mcp.NewHTTPServer(context.Background(), mcp.HTTPServerConfig{
    BaseURL: "http://localhost:8083/mcp",
})
if err != nil {
    log.Printf("Failed to initialize HTTP MCP server: %v", err)
}

// Create a stdio-based MCP server
stdioServer, err := mcp.NewStdioServer(context.Background(), mcp.StdioServerConfig{
    Command: "go",
    Args:    []string{"run", "./server-stdio/main.go"},
})
if err != nil {
    log.Printf("Failed to initialize STDIO MCP server: %v", err)
}
```

### Creating an Agent with MCP Servers

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/mcp"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// Create MCP servers
var mcpServers []interfaces.MCPServer
mcpServers = append(mcpServers, httpServer)
mcpServers = append(mcpServers, stdioServer)

// Create an LLM (e.g., OpenAI)
llm := openai.NewClient(apiKey, openai.WithModel("gpt-4o-mini"))

// Create the agent with MCP server support
myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithMCPServers(mcpServers),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithSystemPrompt("You are an AI assistant that can use tools from MCP servers."),
)
```

### Listing MCP Tools

```go
// List tools from an MCP server
tools, err := mcpServer.ListTools(context.Background())
if err != nil {
    log.Printf("Failed to list tools: %v", err)
    return
}

for _, tool := range tools {
    fmt.Printf("Tool: %s - %s\n", tool.Name, tool.Description)
}
```

## Transports

The MCP integration supports different transports for connecting to MCP servers:

- **stdio**: For local MCP servers that communicate over standard input/output
- **HTTP**: For remote MCP servers that communicate over HTTP

## Implementation Details

The MCP integration is built on top of the [mcp-go](https://github.com/mark3labs/mcp-go) library, which provides a Go implementation of the Model Context Protocol.

The integration consists of:

1. **MCPServer**: An interface for interacting with MCP servers
2. **MCPTool**: A struct that implements the `interfaces.Tool` interface for MCP tools
3. **Agent integration**: The agent can use MCP tools alongside its regular tools

## Example

See the [MCP examples](../../cmd/examples/mcp) for complete examples of using the MCP integration:

- **client**: A client that connects to both HTTP and stdio MCP servers and uses their tools
- **server-http**: An example HTTP MCP server implementation with tools for time and drink recommendations
- **server-stdio**: An example stdio MCP server implementation with a food recommendation tool

To run the examples:

1. Start the HTTP server: `go run cmd/examples/mcp/server-http/main.go`
2. Run the client, which will also start the stdio server: `go run cmd/examples/mcp/client/main.go`
