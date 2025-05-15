package mcp

import (
	"context"
	"log"
	"os"
	"os/exec"

	mcplib "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport"
	"github.com/metoro-io/mcp-golang/transport/http"
	"github.com/metoro-io/mcp-golang/transport/stdio"

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
	Content []*mcplib.Content
	IsError bool
}

// MCPServerImpl is the implementation of interfaces.MCPServer
type MCPServerImpl struct {
	client *mcplib.Client
}

// NewMCPServer creates a new MCPServer with the given transport
func NewMCPServer(ctx context.Context, transport transport.Transport) (interfaces.MCPServer, error) {
	client := mcplib.NewClient(transport)
	_, err := client.Initialize(ctx)
	if err != nil {
		return nil, err
	}

	return &MCPServerImpl{
		client: client,
	}, nil
}

// Initialize initializes the connection to the MCP server
func (s *MCPServerImpl) Initialize(ctx context.Context) error {
	_, err := s.client.Initialize(ctx)
	return err
}

// ListTools lists the tools available on the MCP server
func (s *MCPServerImpl) ListTools(ctx context.Context) ([]interfaces.MCPTool, error) {
	resp, err := s.client.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}

	tools := make([]interfaces.MCPTool, 0, len(resp.Tools))
	for _, t := range resp.Tools {
		description := ""
		if t.Description != nil {
			description = *t.Description
		}

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
	resp, err := s.client.CallTool(ctx, name, args)
	if err != nil {
		return nil, err
	}

	return &interfaces.MCPToolResponse{
		Content: resp.Content,
		IsError: false, // MCP-golang doesn't have an IsError field, so we default to false
	}, nil
}

// Close closes the connection to the MCP server
func (s *MCPServerImpl) Close() error {
	// The mcp-golang client doesn't have a Close method yet
	// We'll implement this when it becomes available
	return nil
}

// StdioServerConfig holds configuration for a stdio MCP server
type StdioServerConfig struct {
	Command string
	Args    []string
	Env     []string
}

// NewStdioServer creates a new MCPServer that communicates over stdio
func NewStdioServer(ctx context.Context, config StdioServerConfig) (interfaces.MCPServer, error) {
	// Use the correct function from stdio package
	cmd := exec.Command(config.Command, config.Args...)
	if len(config.Env) > 0 {
		cmd.Env = append(os.Environ(), config.Env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to get stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	// defer cmd.Process.Kill()

	clientTransport := stdio.NewStdioServerTransportWithIO(stdout, stdin)

	server, err := NewMCPServer(ctx, clientTransport)
	if err != nil {
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
	transport := http.NewHTTPClientTransport(config.BaseURL + config.Path)
	if config.Token != "" {
		transport.WithHeader("Authorization", "Bearer "+config.Token)
	}

	server, err := NewMCPServer(ctx, transport)
	if err != nil {
		return nil, err
	}

	return server, nil
}
