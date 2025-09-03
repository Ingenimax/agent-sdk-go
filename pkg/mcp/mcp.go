package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// MCPServerImpl is the implementation of interfaces.MCPServer using the official SDK
type MCPServerImpl struct {
	session *mcp.ClientSession
	logger  logging.Logger
}

// NewMCPServer creates a new MCPServer with the given transport using the official SDK
func NewMCPServer(ctx context.Context, transport mcp.Transport) (interfaces.MCPServer, error) {
	// Create logger
	logger := logging.New()

	// Create a new client with basic implementation info
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "agent-sdk-go",
		Version: "0.0.0",
	}, nil)

	// Connect to the server using the transport
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		logger.Error(ctx, "Failed to connect to MCP server", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, err
	}

	logger.Debug(ctx, "MCP server connection established", nil)

	return &MCPServerImpl{
		session: session,
		logger:  logger,
	}, nil
}

// Initialize initializes the connection to the MCP server
func (s *MCPServerImpl) Initialize(ctx context.Context) error {
	// Session is already initialized in NewMCPServer, so this is a no-op
	return nil
}

// ListTools lists the tools available on the MCP server
func (s *MCPServerImpl) ListTools(ctx context.Context) ([]interfaces.MCPTool, error) {
	s.logger.Debug(ctx, "Listing MCP tools", nil)

	resp, err := s.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		s.logger.Error(ctx, "Failed to list MCP tools", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, err
	}

	tools := make([]interfaces.MCPTool, 0, len(resp.Tools))
	for _, t := range resp.Tools {
		tools = append(tools, interfaces.MCPTool{
			Name:        t.Name,
			Description: t.Description,
			Schema:      t.InputSchema,
		})
	}

	s.logger.Info(ctx, "Successfully listed MCP tools", map[string]interface{}{
		"tool_count": len(tools),
	})

	return tools, nil
}

// CallTool calls a tool on the MCP server
func (s *MCPServerImpl) CallTool(ctx context.Context, name string, args interface{}) (*interfaces.MCPToolResponse, error) {
	s.logger.Debug(ctx, "Calling MCP tool", map[string]interface{}{
		"tool_name": name,
		"args":      args,
	})

	params := &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	}

	resp, err := s.session.CallTool(ctx, params)
	if err != nil {
		s.logger.Error(ctx, "Failed to call MCP tool", map[string]interface{}{
			"tool_name": name,
			"error":     err.Error(),
		})
		return nil, err
	}

	if resp.IsError {
		s.logger.Warn(ctx, "MCP tool returned error", map[string]interface{}{
			"tool_name": name,
			"content":   resp.Content,
		})
	} else {
		s.logger.Debug(ctx, "MCP tool executed successfully", map[string]interface{}{
			"tool_name": name,
		})
	}

	return &interfaces.MCPToolResponse{
		Content: resp.Content,
		IsError: resp.IsError,
	}, nil
}

// Close closes the connection to the MCP server
func (s *MCPServerImpl) Close() error {
	s.logger.Debug(context.Background(), "Closing MCP server connection", nil)
	err := s.session.Close()
	if err != nil {
		s.logger.Error(context.Background(), "Failed to close MCP server connection", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		s.logger.Debug(context.Background(), "MCP server connection closed successfully", nil)
	}
	return err
}

// StdioServerConfig holds configuration for a stdio MCP server
type StdioServerConfig struct {
	Command string
	Args    []string
	Env     []string
}

// NewStdioServer creates a new MCPServer that communicates over stdio using the official SDK
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

	// Create the command with context
	cmd := exec.CommandContext(ctx, commandPath, config.Args...)
	if len(config.Env) > 0 {
		cmd.Env = append(os.Environ(), config.Env...)
	}

	// Create the command transport using the official SDK
	transport := &mcp.CommandTransport{Command: cmd}

	// Create the MCP server using the transport
	return NewMCPServer(ctx, transport)
}

// HTTPServerConfig holds configuration for an HTTP MCP server
type HTTPServerConfig struct {
	BaseURL string
	Path    string
	Token   string
}

// NewHTTPServer creates a new MCPServer that communicates over HTTP using the official SDK
func NewHTTPServer(ctx context.Context, config HTTPServerConfig) (interfaces.MCPServer, error) {
	// Create logger
	logger := logging.New()

	// Create a new client with basic implementation info
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "agent-sdk-go",
		Version: "0.0.0",
	}, nil)

	// Create SSE client transport for HTTP communication
	transport := &mcp.SSEClientTransport{
		Endpoint: config.BaseURL,
	}

	// Connect to the server using the transport
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		logger.Error(ctx, "Failed to connect to HTTP MCP server", map[string]interface{}{
			"error":    err.Error(),
			"base_url": config.BaseURL,
		})
		return nil, err
	}

	logger.Debug(ctx, "HTTP MCP server connection established", map[string]interface{}{
		"base_url": config.BaseURL,
	})

	return &MCPServerImpl{
		session: session,
		logger:  logger,
	}, nil
}
