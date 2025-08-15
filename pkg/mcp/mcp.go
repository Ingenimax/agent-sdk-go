package mcp

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// MCPServer represents a connection to an MCP server
type MCPServer interface {
	// Initialize initializes the connection to the MCP server
	Initialize(ctx context.Context) error

	// ListTools lists the tools available on the MCP server
	ListTools(ctx context.Context) ([]interfaces.MCPTool, error)

	// CallTool calls a tool on the MCP server
	CallTool(ctx context.Context, name string, args interface{}) (*interfaces.MCPToolResponse, error)

	// Close closes the connection to the MCP server
	Close() error
}

// Tool represents a tool available on an MCP server
type Tool struct {
	Name        string
	Description string
	Schema      interface{}
}

// ToolResponse represents a response from a tool call
type ToolResponse struct {
	Content []*mcp.Content
	IsError bool
}

// MCPServerImpl is the implementation of interfaces.MCPServer
type MCPServerImpl struct {
	client *client.Client
}

// NewMCPServer creates a new MCPServer with the given client
func NewMCPServer(ctx context.Context, c *client.Client) (interfaces.MCPServer, error) {
	// Initialize the client
	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities: mcp.ClientCapabilities{
				Sampling: &struct{}{},
			},
			ClientInfo: mcp.Implementation{
				Name:    "agent-sdk-go",
				Version: "1.0.0",
			},
		},
	}

	_, err := c.Initialize(ctx, initReq)
	if err != nil {
		return nil, err
	}

	return &MCPServerImpl{
		client: c,
	}, nil
}

// Initialize initializes the connection to the MCP server
func (s *MCPServerImpl) Initialize(ctx context.Context) error {
	// Client is already initialized in NewMCPServer, so this is a no-op
	return nil
}

// ListTools lists the tools available on the MCP server
func (s *MCPServerImpl) ListTools(ctx context.Context) ([]interfaces.MCPTool, error) {
	resp, err := s.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}

	tools := make([]interfaces.MCPTool, 0, len(resp.Tools))
	for _, t := range resp.Tools {
		description := t.Description // Description is now string, not *string

		tools = append(tools, interfaces.MCPTool{
			Name:        t.Name,
			Description: description,
			Schema:      t.InputSchema,
		})
	}

	return tools, nil
}

// CallTool calls a tool on the MCP server
func (s *MCPServerImpl) CallTool(ctx context.Context, name string, args interface{}) (*interfaces.MCPToolResponse, error) {
	resp, err := s.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	})
	if err != nil {
		return nil, err
	}

	return &interfaces.MCPToolResponse{
		Content: resp.Content, // This is now interface{} which can hold []*mcp.Content
		IsError: false,        // The new client API handles errors differently
	}, nil
}

// Close closes the connection to the MCP server
func (s *MCPServerImpl) Close() error {
	return s.client.Close()
}

// StdioServerConfig holds configuration for a stdio MCP server
type StdioServerConfig struct {
	Command string
	Args    []string
	Env     []string
}

// NewStdioServer creates a new MCPServer that communicates over stdio
func NewStdioServer(ctx context.Context, config StdioServerConfig) (interfaces.MCPServer, error) {
	// Validate the command and arguments to mitigate command injection risks
	if config.Command == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}

	// Additional validation of command and arguments
	// Using LookPath to ensure the command exists in the system
	commandPath, err := exec.LookPath(config.Command)
	if err != nil {
		return nil, fmt.Errorf("invalid command %q: %v", config.Command, err)
	}

	// Create client using the new API
	c, err := client.NewStdioMCPClient(commandPath, config.Env, config.Args...)
	if err != nil {
		return nil, fmt.Errorf("failed to create stdio client: %v", err)
	}

	// Start the client
	err = c.Start(ctx)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("failed to start client: %v", err)
	}

	server, err := NewMCPServer(ctx, c)
	if err != nil {
		c.Close()
		return nil, err
	}

	return server, nil
}

// HTTPServerConfig holds configuration for an HTTP MCP server
type HTTPServerConfig struct {
	BaseURL string
	Path    string
	Token   string
}

// NewHTTPServer creates a new MCPServer that communicates over HTTP
func NewHTTPServer(ctx context.Context, config HTTPServerConfig) (interfaces.MCPServer, error) {
	baseURL := config.BaseURL + config.Path

	// Create SSE client using the new API
	var c *client.Client
	var err error

	if config.Token != "" {
		// Use SSE client with headers for authentication
		headers := map[string]string{
			"Authorization": "Bearer " + config.Token,
		}
		c, err = client.NewStreamableHttpClient(baseURL, transport.WithHTTPHeaders(headers))
	} else {
		c, err = client.NewStreamableHttpClient(baseURL)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %v", err)
	}

	// Start the client
	err = c.Start(ctx)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("failed to start client: %v", err)
	}

	server, err := NewMCPServer(ctx, c)
	if err != nil {
		c.Close()
		return nil, err
	}

	return server, nil
}
