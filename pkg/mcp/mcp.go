package mcp

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// MCPServerImpl is the implementation of interfaces.MCPServer using the official SDK
type MCPServerImpl struct {
	session      *mcp.ClientSession
	logger       logging.Logger
	serverInfo   *interfaces.MCPServerInfo
	capabilities *interfaces.MCPServerCapabilities
}

const TraceParentAttribute = "traceparent"

func tracingMiddleware(h mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (result mcp.Result, err error) {
		// Add tracing information to the request metadata for tools/call method
		if method == "tools/call" {
			spanCtx := trace.SpanContextFromContext(ctx)
			if !spanCtx.IsValid() {
				// No tracing context available
				return h(ctx, method, req)
			}
			propagator := propagation.TraceContext{}
			headers := make(http.Header)
			propagator.Inject(ctx, propagation.HeaderCarrier(headers))
			traceparentValue := headers.Get(TraceParentAttribute)
			if rp, ok := req.GetParams().(mcp.RequestParams); ok {
				if rp.GetMeta() == nil {
					rp.SetMeta(map[string]any{
						TraceParentAttribute: traceparentValue,
					})
				} else {
					rp.GetMeta()[TraceParentAttribute] = traceparentValue
				}
			}
		}
		return h(ctx, method, req)
	}
}

// convertMCPCapabilities converts mcp.ServerCapabilities to interfaces.MCPServerCapabilities
func convertMCPCapabilities(caps *mcp.ServerCapabilities) *interfaces.MCPServerCapabilities {
	if caps == nil {
		return nil
	}

	result := &interfaces.MCPServerCapabilities{}

	if caps.Tools != nil {
		result.Tools = &interfaces.MCPToolCapabilities{
			ListChanged: caps.Tools.ListChanged,
		}
	}

	if caps.Resources != nil {
		result.Resources = &interfaces.MCPResourceCapabilities{
			Subscribe:   caps.Resources.Subscribe,
			ListChanged: caps.Resources.ListChanged,
		}
	}

	if caps.Prompts != nil {
		result.Prompts = &interfaces.MCPPromptCapabilities{
			ListChanged: caps.Prompts.ListChanged,
		}
	}

	return result
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

	// Add tracing middleware to the client
	client.AddSendingMiddleware(tracingMiddleware)
	// Connect to the server using the transport
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		// govulncheck:ignore GO-2025-4155 - err.Error() used for logging only, not exploitable
		logger.Error(ctx, "Failed to connect to MCP server", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, err
	}

	// Get initialization result immediately after connection
	initResult := session.InitializeResult()

	var serverInfo *interfaces.MCPServerInfo
	var capabilities *interfaces.MCPServerCapabilities

	if initResult != nil {
		// Extract server info (standard MCP fields)
		if initResult.ServerInfo != nil {
			serverInfo = &interfaces.MCPServerInfo{
				Name:    initResult.ServerInfo.Name,    // Always present
				Title:   initResult.ServerInfo.Title,   // Optional
				Version: initResult.ServerInfo.Version, // Optional
			}

			logger.Info(ctx, "Discovered MCP server metadata", map[string]interface{}{
				"server_name":    serverInfo.Name,
				"server_title":   serverInfo.Title,
				"server_version": serverInfo.Version,
			})
		}

		// Extract capabilities
		if initResult.Capabilities != nil {
			capabilities = convertMCPCapabilities(initResult.Capabilities)
		}
	}

	logger.Debug(ctx, "MCP server connection established with metadata", map[string]interface{}{
		"has_server_info":  serverInfo != nil,
		"has_capabilities": capabilities != nil,
	})

	return &MCPServerImpl{
		session:      session,
		logger:       logger,
		serverInfo:   serverInfo,
		capabilities: capabilities,
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
		mcpErr := ClassifyError(err, "ListTools", "server", "unknown")
		// govulncheck:ignore GO-2025-4155 - err.Error() used for logging only, not exploitable
		s.logger.Error(ctx, "Failed to list MCP tools", map[string]interface{}{
			"error":      err.Error(),
			"error_type": mcpErr.ErrorType,
			"retryable":  mcpErr.Retryable,
		})
		return nil, mcpErr
	}

	tools := make([]interfaces.MCPTool, 0, len(resp.Tools))
	for _, t := range resp.Tools {
		tool := interfaces.MCPTool{
			Name:        t.Name,
			Description: t.Description,
			Schema:      t.InputSchema,
			Metadata:    make(map[string]interface{}),
		}

		// Add output schema if available (this might not be in the current Go SDK yet)
		// This is forward-compatible for when the Go SDK adds output schema support
		if t.Annotations != nil {
			// For now, just store the annotations as metadata
			// The ToolAnnotations type might have different fields than expected
			tool.Metadata["annotations"] = "present"
		}

		// Placeholder for future output schema support
		// tool.OutputSchema = t.OutputSchema // This field doesn't exist in current Go SDK yet

		tools = append(tools, tool)
	}

	s.logger.Info(ctx, "Successfully listed MCP tools", map[string]interface{}{
		"tool_count": len(tools),
	})

	return tools, nil
}

// ListResources lists the resources available on the MCP server
func (s *MCPServerImpl) ListResources(ctx context.Context) ([]interfaces.MCPResource, error) {
	s.logger.Debug(ctx, "Listing MCP resources", nil)

	resp, err := s.session.ListResources(ctx, &mcp.ListResourcesParams{})
	if err != nil {
		mcpErr := ClassifyError(err, "ListResources", "server", "unknown")
		// govulncheck:ignore GO-2025-4155 - err.Error() used for logging only, not exploitable
		s.logger.Error(ctx, "Failed to list MCP resources", map[string]interface{}{
			"error":      err.Error(),
			"error_type": mcpErr.ErrorType,
			"retryable":  mcpErr.Retryable,
		})
		return nil, mcpErr
	}

	resources := make([]interfaces.MCPResource, 0, len(resp.Resources))
	for _, r := range resp.Resources {
		resource := interfaces.MCPResource{
			URI:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MimeType:    r.MIMEType,
			Metadata:    make(map[string]string),
		}

		// Convert annotations to metadata if present
		if r.Annotations != nil {
			if len(r.Annotations.Audience) > 0 {
				// Convert audience roles to comma-separated string
				var audienceStrs []string
				for _, role := range r.Annotations.Audience {
					audienceStrs = append(audienceStrs, string(role))
				}
				resource.Metadata["audience"] = strings.Join(audienceStrs, ",")
			}
			if r.Annotations.LastModified != "" {
				resource.Metadata["lastModified"] = r.Annotations.LastModified
			}
			if r.Annotations.Priority > 0 {
				resource.Metadata["priority"] = fmt.Sprintf("%.2f", r.Annotations.Priority)
			}
		}

		resources = append(resources, resource)
	}

	s.logger.Info(ctx, "Successfully listed MCP resources", map[string]interface{}{
		"resource_count": len(resources),
	})

	return resources, nil
}

// GetResource retrieves a specific resource by URI
func (s *MCPServerImpl) GetResource(ctx context.Context, uri string) (*interfaces.MCPResourceContent, error) {
	s.logger.Debug(ctx, "Getting MCP resource", map[string]interface{}{
		"uri": uri,
	})

	resp, err := s.session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: uri,
	})
	if err != nil {
		mcpErr := ClassifyError(err, "GetResource", "server", "unknown")
		mcpErr = mcpErr.WithMetadata("uri", uri)
		// govulncheck:ignore GO-2025-4155 - err.Error() used for logging only, not exploitable
		s.logger.Error(ctx, "Failed to get MCP resource", map[string]interface{}{
			"uri":        uri,
			"error":      err.Error(),
			"error_type": mcpErr.ErrorType,
			"retryable":  mcpErr.Retryable,
		})
		return nil, mcpErr
	}

	content := &interfaces.MCPResourceContent{
		URI:      uri,
		Metadata: make(map[string]string),
	}

	// Process contents
	if len(resp.Contents) > 0 {
		firstContent := resp.Contents[0]

		// ResourceContents is a struct with Text and Blob fields
		if firstContent.Text != "" {
			content.Text = firstContent.Text
			content.MimeType = firstContent.MIMEType
			if content.MimeType == "" {
				content.MimeType = "text/plain"
			}
		} else if len(firstContent.Blob) > 0 {
			content.Blob = firstContent.Blob
			content.MimeType = firstContent.MIMEType
			if content.MimeType == "" {
				content.MimeType = "application/octet-stream"
			}
		}
	}

	s.logger.Debug(ctx, "Successfully retrieved MCP resource", map[string]interface{}{
		"uri":       uri,
		"mime_type": content.MimeType,
		"size":      len(content.Text) + len(content.Blob),
	})

	return content, nil
}

// WatchResource watches for changes to a resource (if supported)
func (s *MCPServerImpl) WatchResource(ctx context.Context, uri string) (<-chan interfaces.MCPResourceUpdate, error) {
	s.logger.Debug(ctx, "Setting up resource watch", map[string]interface{}{
		"uri": uri,
	})

	// Create update channel
	updates := make(chan interfaces.MCPResourceUpdate, 10)

	// For now, we'll implement basic polling since the Go SDK doesn't have built-in watching
	// In a real implementation, this would use server-sent events or websockets
	go func() {
		defer close(updates)

		ticker := time.NewTicker(5 * time.Second) // Poll every 5 seconds
		defer ticker.Stop()

		var lastContent *interfaces.MCPResourceContent

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				content, err := s.GetResource(ctx, uri)
				if err != nil {
					updates <- interfaces.MCPResourceUpdate{
						URI:       uri,
						Type:      interfaces.MCPResourceUpdateTypeError,
						Timestamp: time.Now(),
						Error:     err,
					}
					continue
				}

				// Check if content has changed
				if lastContent == nil ||
					content.Text != lastContent.Text ||
					!bytes.Equal(content.Blob, lastContent.Blob) {

					updates <- interfaces.MCPResourceUpdate{
						URI:       uri,
						Type:      interfaces.MCPResourceUpdateTypeChanged,
						Content:   content,
						Timestamp: time.Now(),
					}
					lastContent = content
				}
			}
		}
	}()

	return updates, nil
}

// ListPrompts lists the prompts available on the MCP server
func (s *MCPServerImpl) ListPrompts(ctx context.Context) ([]interfaces.MCPPrompt, error) {
	s.logger.Debug(ctx, "Listing MCP prompts", nil)

	resp, err := s.session.ListPrompts(ctx, &mcp.ListPromptsParams{})
	if err != nil {
		mcpErr := ClassifyError(err, "ListPrompts", "server", "unknown")
		// govulncheck:ignore GO-2025-4155 - err.Error() used for logging only, not exploitable
		s.logger.Error(ctx, "Failed to list MCP prompts", map[string]interface{}{
			"error":      err.Error(),
			"error_type": mcpErr.ErrorType,
			"retryable":  mcpErr.Retryable,
		})
		return nil, mcpErr
	}

	prompts := make([]interfaces.MCPPrompt, 0, len(resp.Prompts))
	for _, p := range resp.Prompts {
		prompt := interfaces.MCPPrompt{
			Name:        p.Name,
			Description: p.Description,
			Arguments:   make([]interfaces.MCPPromptArgument, 0, len(p.Arguments)),
			Metadata:    make(map[string]string),
		}

		// Convert arguments
		for _, arg := range p.Arguments {
			prompt.Arguments = append(prompt.Arguments, interfaces.MCPPromptArgument{
				Name:        arg.Name,
				Description: arg.Description,
				Required:    arg.Required,
			})
		}

		prompts = append(prompts, prompt)
	}

	s.logger.Info(ctx, "Successfully listed MCP prompts", map[string]interface{}{
		"prompt_count": len(prompts),
	})

	return prompts, nil
}

// GetPrompt retrieves a specific prompt with variables
func (s *MCPServerImpl) GetPrompt(ctx context.Context, name string, variables map[string]interface{}) (*interfaces.MCPPromptResult, error) {
	s.logger.Debug(ctx, "Getting MCP prompt", map[string]interface{}{
		"name":      name,
		"variables": variables,
	})

	// Convert variables from map[string]interface{} to map[string]string
	args := make(map[string]string)
	for k, v := range variables {
		if str, ok := v.(string); ok {
			args[k] = str
		} else {
			args[k] = fmt.Sprintf("%v", v)
		}
	}

	resp, err := s.session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		mcpErr := ClassifyError(err, "GetPrompt", "server", "unknown")
		_ = mcpErr.WithMetadata("prompt_name", name)
		// govulncheck:ignore GO-2025-4155 - err.Error() used for logging only, not exploitable
		s.logger.Error(ctx, "Failed to get MCP prompt", map[string]interface{}{
			"name":       name,
			"error":      err.Error(),
			"error_type": mcpErr.ErrorType,
			"retryable":  mcpErr.Retryable,
		})
		return nil, mcpErr
	}

	result := &interfaces.MCPPromptResult{
		Variables: variables,
		Messages:  make([]interfaces.MCPPromptMessage, 0, len(resp.Messages)),
		Metadata:  make(map[string]string),
	}

	// Convert messages
	for _, msg := range resp.Messages {
		message := interfaces.MCPPromptMessage{
			Role: string(msg.Role),
		}

		// Extract content from message content
		if msg.Content != nil {
			if textContent, ok := msg.Content.(*mcp.TextContent); ok {
				message.Content = textContent.Text
			} else {
				// Handle other content types
				message.Content = fmt.Sprintf("%v", msg.Content)
			}
		}

		result.Messages = append(result.Messages, message)
	}

	// If there's only one message, also set it as the prompt field for backwards compatibility
	if len(result.Messages) == 1 {
		result.Prompt = result.Messages[0].Content
	}

	s.logger.Debug(ctx, "Successfully retrieved MCP prompt", map[string]interface{}{
		"name":          name,
		"message_count": len(result.Messages),
	})

	return result, nil
}

// CreateMessage requests the client to generate a completion using its LLM
func (s *MCPServerImpl) CreateMessage(ctx context.Context, request *interfaces.MCPSamplingRequest) (*interfaces.MCPSamplingResponse, error) {
	s.logger.Debug(ctx, "Creating message via sampling", map[string]interface{}{
		"message_count": len(request.Messages),
		"system_prompt": request.SystemPrompt != "",
		"max_tokens":    request.MaxTokens,
	})

	// Convert our request format to the Go SDK format
	samplingRequest := &mcp.CreateMessageParams{
		Messages: make([]*mcp.SamplingMessage, 0, len(request.Messages)),
	}

	// Convert messages
	for _, msg := range request.Messages {
		samplingMsg := &mcp.SamplingMessage{
			Role: mcp.Role(msg.Role),
		}

		// Convert content based on type
		switch msg.Content.Type {
		case "text":
			samplingMsg.Content = &mcp.TextContent{
				Text: msg.Content.Text,
			}
		case "image":
			// Convert base64 string to byte slice
			var imageData []byte
			if msg.Content.Data != "" {
				// For now, assume it's already base64 decoded bytes
				// In production, you might need to decode from base64
				imageData = []byte(msg.Content.Data)
			}
			samplingMsg.Content = &mcp.ImageContent{
				Data:     imageData,
				MIMEType: msg.Content.MimeType,
			}
		default:
			// Default to text content
			samplingMsg.Content = &mcp.TextContent{
				Text: msg.Content.Text,
			}
		}

		samplingRequest.Messages = append(samplingRequest.Messages, samplingMsg)
	}

	// Add system prompt if provided
	if request.SystemPrompt != "" {
		samplingRequest.SystemPrompt = request.SystemPrompt
	}

	// Add model preferences if provided
	if request.ModelPreferences != nil {
		samplingRequest.ModelPreferences = &mcp.ModelPreferences{
			CostPriority:         request.ModelPreferences.CostPriority,
			SpeedPriority:        request.ModelPreferences.SpeedPriority,
			IntelligencePriority: request.ModelPreferences.IntelligencePriority,
		}

		// Convert model hints
		if len(request.ModelPreferences.Hints) > 0 {
			hints := make([]*mcp.ModelHint, 0, len(request.ModelPreferences.Hints))
			for _, hint := range request.ModelPreferences.Hints {
				hints = append(hints, &mcp.ModelHint{
					Name: hint.Name,
				})
			}
			samplingRequest.ModelPreferences.Hints = hints
		}
	}

	// Add optional parameters
	if request.MaxTokens != nil {
		maxTokens := int64(*request.MaxTokens)
		samplingRequest.MaxTokens = maxTokens
	}
	if request.Temperature != nil {
		samplingRequest.Temperature = *request.Temperature
	}
	if len(request.StopSequences) > 0 {
		samplingRequest.StopSequences = request.StopSequences
	}
	if request.IncludeContext != "" {
		samplingRequest.IncludeContext = request.IncludeContext
	}

	// For now, implement a placeholder since the Go SDK might not have sampling yet
	// This is a forward-compatible implementation for when sampling is available
	s.logger.Warn(ctx, "MCP Sampling feature not yet implemented in Go SDK", map[string]interface{}{
		"message_count": len(request.Messages),
		"system_prompt": request.SystemPrompt != "",
	})

	return nil, fmt.Errorf("MCP Sampling is not yet available in the Go SDK - this is a placeholder implementation for the 2025 specification")
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

	s.logger.Debug(ctx, "Calling session.CallTool", map[string]interface{}{
		"tool_name": name,
		"params":    params,
	})

	resp, err := s.session.CallTool(ctx, params)
	if err != nil {
		mcpErr := ClassifyError(err, "CallTool", "server", "unknown")
		_ = mcpErr.WithMetadata("tool_name", name)
		// govulncheck:ignore GO-2025-4155 - err.Error() used for logging only, not exploitable
		s.logger.Error(ctx, "Failed to call MCP tool", map[string]interface{}{
			"tool_name":  name,
			"error":      err.Error(),
			"error_type": mcpErr.ErrorType,
			"retryable":  mcpErr.Retryable,
		})
		return nil, mcpErr
	}

	s.logger.Debug(ctx, "Received response from session.CallTool", map[string]interface{}{
		"tool_name": name,
		"is_error":  resp.IsError,
		"content":   resp.Content,
		"meta":      resp.Meta,
	})

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

	response := &interfaces.MCPToolResponse{
		Content:  resp.Content,
		IsError:  resp.IsError,
		Metadata: make(map[string]interface{}),
	}

	// Check for structured content (this is forward-compatible)
	// The current Go SDK might not have this field yet, but this prepares us for when it does
	if resp.Meta != nil {
		response.Metadata = resp.Meta
	}

	// Try to extract structured content from metadata if present
	// This is a placeholder for when the Go SDK adds proper structured content support
	if metadata, ok := resp.Meta["structuredContent"]; ok {
		response.StructuredContent = metadata
	}

	return response, nil
}

// GetServerInfo returns the server metadata discovered during initialization
func (s *MCPServerImpl) GetServerInfo() (*interfaces.MCPServerInfo, error) {
	return s.serverInfo, nil
}

// GetCapabilities returns the server capabilities discovered during initialization
func (s *MCPServerImpl) GetCapabilities() (*interfaces.MCPServerCapabilities, error) {
	return s.capabilities, nil
}

// Close closes the connection to the MCP server
func (s *MCPServerImpl) Close() error {
	s.logger.Debug(context.Background(), "Closing MCP server connection", nil)
	err := s.session.Close()
	if err != nil {
		// govulncheck:ignore GO-2025-4155 - err.Error() used for logging only, not exploitable
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
	Logger  logging.Logger
}

// NewStdioServer creates a new MCPServer that communicates over stdio using the official SDK
func NewStdioServer(ctx context.Context, config StdioServerConfig) (interfaces.MCPServer, error) {
	return NewStdioServerWithRetry(ctx, config, nil)
}

// NewStdioServerWithRetry creates a new MCPServer with retry logic
func NewStdioServerWithRetry(ctx context.Context, config StdioServerConfig, retryConfig *RetryConfig) (interfaces.MCPServer, error) {
	// Create logger if not configured
	logger := config.Logger
	if logger == nil {
		logger = logging.New()
	}
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

	// Additional security validation - ensure command path is absolute and exists
	if !filepath.IsAbs(commandPath) {
		return nil, fmt.Errorf("command path must be absolute for security: %q", commandPath)
	}

	// Check if the file exists and is executable
	if info, err := os.Stat(commandPath); err != nil {
		return nil, fmt.Errorf("command not accessible: %v", err)
	} else if info.IsDir() {
		return nil, fmt.Errorf("command path is a directory, not executable: %q", commandPath)
	}

	// Create the command with context
	// #nosec G204 -- commandPath is validated above with LookPath and security checks
	cmd := exec.CommandContext(ctx, commandPath, config.Args...)
	if len(config.Env) > 0 {
		cmd.Env = append(os.Environ(), config.Env...)
	}

	// Create the command transport using the official SDK
	transport := &mcp.CommandTransport{Command: cmd}

	// Create a new client with basic implementation info
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "agent-sdk-go",
		Version: "0.0.0",
	}, nil)

	// Add tracing middleware to the client
	client.AddSendingMiddleware(tracingMiddleware)
	// Connect to the server using the transport
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		mcpErr := ClassifyError(err, "Connect", "stdio-server", "stdio")
		// govulncheck:ignore GO-2025-4155 - err.Error() used for logging only, not exploitable
		logger.Error(ctx, "Failed to connect to MCP server", map[string]interface{}{
			"error":      err.Error(),
			"error_type": mcpErr.ErrorType,
			"retryable":  mcpErr.Retryable,
			"command":    config.Command,
		})
		return nil, mcpErr
	}

	// Get initialization result immediately after connection
	initResult := session.InitializeResult()

	var serverInfo *interfaces.MCPServerInfo
	var capabilities *interfaces.MCPServerCapabilities

	if initResult != nil {
		// Extract server info (standard MCP fields)
		if initResult.ServerInfo != nil {
			serverInfo = &interfaces.MCPServerInfo{
				Name:    initResult.ServerInfo.Name,    // Always present
				Title:   initResult.ServerInfo.Title,   // Optional
				Version: initResult.ServerInfo.Version, // Optional
			}

			logger.Info(ctx, "Discovered MCP server metadata", map[string]interface{}{
				"server_name":    serverInfo.Name,
				"server_title":   serverInfo.Title,
				"server_version": serverInfo.Version,
			})
		}

		// Extract capabilities
		if initResult.Capabilities != nil {
			capabilities = convertMCPCapabilities(initResult.Capabilities)
		}
	}

	logger.Debug(ctx, "MCP server connection established with metadata", map[string]interface{}{
		"has_server_info":  serverInfo != nil,
		"has_capabilities": capabilities != nil,
	})

	server := &MCPServerImpl{
		session:      session,
		logger:       logger,
		serverInfo:   serverInfo,
		capabilities: capabilities,
	}

	// Wrap with retry logic if configured
	if retryConfig != nil {
		return NewRetryableServer(server, retryConfig), nil
	}

	return server, nil
}

// HTTPServerConfig holds configuration for an HTTP MCP server
type HTTPServerConfig struct {
	BaseURL      string
	Path         string
	Token        string
	ProtocolType ServerProtocolType
	Logger       logging.Logger

	ResourceIndicator string `json:"resource_indicator,omitempty"`
}

// ServerProtocolType defines the protocol type for the MCP server communication
// Supported types are "streamable" and "sse"
type ServerProtocolType string

const (
	StreamableHTTP ServerProtocolType = "streamable"
	SSE            ServerProtocolType = "sse"
)

// Wrap an http.RoundTripper to add the Authorization header
type customRoundTripper struct {
	delegate http.RoundTripper
	token    string
}

// RoundTrip implements the http.RoundTripper interface
func (rt *customRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+rt.token)
	return rt.delegate.RoundTrip(req)
}

// customHTTPClient creates an HTTP client that adds the Authorization header to each request
func customHTTPClient(token string) *http.Client {
	return &http.Client{
		Transport: &customRoundTripper{
			delegate: http.DefaultTransport,
			token:    token,
		},
	}
}

// NewHTTPServer creates a new MCPServer that communicates over HTTP using the official SDK
func NewHTTPServer(ctx context.Context, config HTTPServerConfig) (interfaces.MCPServer, error) {
	return NewHTTPServerWithRetry(ctx, config, nil)
}

// NewHTTPServerWithRetry creates a new HTTP MCPServer with retry logic
func NewHTTPServerWithRetry(ctx context.Context, config HTTPServerConfig, retryConfig *RetryConfig) (interfaces.MCPServer, error) {
	// Create logger if not configured
	logger := config.Logger
	if logger == nil {
		logger = logging.New()
	}

	// Create a new client with basic implementation info
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "agent-sdk-go",
		Version: "0.0.0",
	}, nil)

	// Add tracing middleware to the client
	client.AddSendingMiddleware(tracingMiddleware)

	httpClient := http.DefaultClient

	// Handle token-based authentication
	if config.Token != "" {
		// Fallback to legacy token-based authentication
		httpClient = customHTTPClient(config.Token)
	}

	var transport mcp.Transport
	switch config.ProtocolType {
	case SSE:
		// Create SSE client transport for HTTP communication
		// It is legacy but still supported by some MCP servers
		transport = &mcp.SSEClientTransport{
			Endpoint:   config.BaseURL,
			HTTPClient: httpClient,
		}
	case StreamableHTTP:
		// Create StreamableHTTP client transport for HTTP communication
		transport = &mcp.StreamableClientTransport{
			Endpoint:   config.BaseURL,
			HTTPClient: httpClient,
		}
	default:
		// Default to SSE if type is not recognized
		logger.Warn(ctx, "Server protocol type is not set, defaulting to SSE", map[string]interface{}{})
		transport = &mcp.SSEClientTransport{
			Endpoint:   config.BaseURL,
			HTTPClient: httpClient,
		}
	}

	// Connect to the server using the transport
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		mcpErr := ClassifyError(err, "Connect", "http-server", "http")
		return nil, mcpErr
	}

	// Get initialization result immediately after connection
	initResult := session.InitializeResult()

	var serverInfo *interfaces.MCPServerInfo
	var capabilities *interfaces.MCPServerCapabilities

	if initResult != nil {
		// Extract server info (standard MCP fields)
		if initResult.ServerInfo != nil {
			serverInfo = &interfaces.MCPServerInfo{
				Name:    initResult.ServerInfo.Name,    // Always present
				Title:   initResult.ServerInfo.Title,   // Optional
				Version: initResult.ServerInfo.Version, // Optional
			}

			logger.Info(ctx, "Discovered MCP server metadata", map[string]interface{}{
				"server_name":    serverInfo.Name,
				"server_title":   serverInfo.Title,
				"server_version": serverInfo.Version,
			})
		}

		// Extract capabilities
		if initResult.Capabilities != nil {
			capabilities = convertMCPCapabilities(initResult.Capabilities)
		}
	}

	logger.Debug(ctx, "HTTP MCP server connection established with metadata", map[string]interface{}{
		"base_url":         config.BaseURL,
		"has_server_info":  serverInfo != nil,
		"has_capabilities": capabilities != nil,
	})

	server := &MCPServerImpl{
		session:      session,
		logger:       logger,
		serverInfo:   serverInfo,
		capabilities: capabilities,
	}

	// Wrap with retry logic if configured
	if retryConfig != nil {
		return NewRetryableServer(server, retryConfig), nil
	}

	return server, nil
}
