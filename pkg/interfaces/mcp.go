package interfaces

import (
	"context"
)

// MCPServer represents a connection to an MCP server
type MCPServer interface {
	// Initialize initializes the connection to the MCP server
	Initialize(ctx context.Context) error

	// ListTools lists the tools available on the MCP server
	ListTools(ctx context.Context) ([]MCPTool, error)

	// CallTool calls a tool on the MCP server
	CallTool(ctx context.Context, name string, args interface{}) (*MCPToolResponse, error)

	// Close closes the connection to the MCP server
	Close() error
}

// MCPTool represents a tool available on an MCP server
type MCPTool struct {
	Name        string
	Description string
	Schema      interface{}
}

// MCPToolResponse represents a response from a tool call
type MCPToolResponse struct {
	Content interface{}
	IsError bool
}
