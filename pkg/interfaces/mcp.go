package interfaces

import (
	"context"
	"io"
	"time"
)

// MCPServer represents a connection to an MCP server
type MCPServer interface {
	// Initialize initializes the connection to the MCP server
	Initialize(ctx context.Context) error

	// ListTools lists the tools available on the MCP server
	ListTools(ctx context.Context) ([]MCPTool, error)

	// CallTool calls a tool on the MCP server
	CallTool(ctx context.Context, name string, args interface{}) (*MCPToolResponse, error)

	// ListResources lists the resources available on the MCP server
	ListResources(ctx context.Context) ([]MCPResource, error)

	// GetResource retrieves a specific resource by URI
	GetResource(ctx context.Context, uri string) (*MCPResourceContent, error)

	// WatchResource watches for changes to a resource (if supported)
	WatchResource(ctx context.Context, uri string) (<-chan MCPResourceUpdate, error)

	// ListPrompts lists the prompts available on the MCP server
	ListPrompts(ctx context.Context) ([]MCPPrompt, error)

	// GetPrompt retrieves a specific prompt with variables
	GetPrompt(ctx context.Context, name string, variables map[string]interface{}) (*MCPPromptResult, error)

	// Sampling Methods (if supported by client)
	// CreateMessage requests the client to generate a completion using its LLM
	CreateMessage(ctx context.Context, request *MCPSamplingRequest) (*MCPSamplingResponse, error)

	// Metadata discovery methods
	// GetServerInfo returns the server metadata discovered during initialization
	GetServerInfo() (*MCPServerInfo, error)

	// GetCapabilities returns the server capabilities discovered during initialization
	GetCapabilities() (*MCPServerCapabilities, error)

	// Close closes the connection to the MCP server
	Close() error
}

// MCPTool represents a tool available on an MCP server
type MCPTool struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Schema       interface{}            `json:"inputSchema,omitempty"`
	OutputSchema interface{}            `json:"outputSchema,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// MCPToolResponse represents a response from a tool call
type MCPToolResponse struct {
	Content           interface{}            `json:"content,omitempty"`
	StructuredContent interface{}            `json:"structuredContent,omitempty"`
	IsError           bool                   `json:"isError,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

// MCPResourceType represents the type of MCP resource
type MCPResourceType string

const (
	MCPResourceTypeText   MCPResourceType = "text"
	MCPResourceTypeBinary MCPResourceType = "binary"
	MCPResourceTypeJSON   MCPResourceType = "json"
)

// MCPResource represents a resource available on an MCP server
type MCPResource struct {
	URI         string            `json:"uri"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	MimeType    string            `json:"mimeType,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// MCPResourceContent represents the content of a resource
type MCPResourceContent struct {
	URI      string            `json:"uri"`
	MimeType string            `json:"mimeType"`
	Text     string            `json:"text,omitempty"`
	Blob     []byte            `json:"blob,omitempty"`
	Reader   io.Reader         `json:"-"` // For streaming content
	Metadata map[string]string `json:"metadata,omitempty"`
}

// MCPResourceUpdate represents an update to a watched resource
type MCPResourceUpdate struct {
	URI       string                `json:"uri"`
	Type      MCPResourceUpdateType `json:"type"`
	Content   *MCPResourceContent   `json:"content,omitempty"`
	Timestamp time.Time             `json:"timestamp"`
	Error     error                 `json:"error,omitempty"`
}

// MCPResourceUpdateType represents the type of resource update
type MCPResourceUpdateType string

const (
	MCPResourceUpdateTypeChanged MCPResourceUpdateType = "changed"
	MCPResourceUpdateTypeDeleted MCPResourceUpdateType = "deleted"
	MCPResourceUpdateTypeError   MCPResourceUpdateType = "error"
)

// MCPPrompt represents a prompt template available on an MCP server
type MCPPrompt struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Arguments   []MCPPromptArgument    `json:"arguments,omitempty"`
	Schema      map[string]interface{} `json:"schema,omitempty"`
	Metadata    map[string]string      `json:"metadata,omitempty"`
}

// MCPPromptArgument represents an argument for a prompt template
type MCPPromptArgument struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Type        string      `json:"type,omitempty"`
	Default     interface{} `json:"default,omitempty"`
}

// MCPPromptResult represents the result of getting a prompt with variables
type MCPPromptResult struct {
	Prompt    string                 `json:"prompt"`
	Messages  []MCPPromptMessage     `json:"messages,omitempty"`
	Variables map[string]interface{} `json:"variables,omitempty"`
	Metadata  map[string]string      `json:"metadata,omitempty"`
}

// MCPPromptMessage represents a message in a prompt result
type MCPPromptMessage struct {
	Role    string                 `json:"role"`
	Content string                 `json:"content"`
	Name    string                 `json:"name,omitempty"`
	Meta    map[string]interface{} `json:"meta,omitempty"`
}

// MCPSamplingRequest represents a request for LLM sampling
type MCPSamplingRequest struct {
	Messages         []MCPMessage           `json:"messages"`
	ModelPreferences *MCPModelPreferences   `json:"modelPreferences,omitempty"`
	SystemPrompt     string                 `json:"systemPrompt,omitempty"`
	IncludeContext   string                 `json:"includeContext,omitempty"`
	Temperature      *float64               `json:"temperature,omitempty"`
	MaxTokens        *int                   `json:"maxTokens,omitempty"`
	StopSequences    []string               `json:"stopSequences,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// MCPSamplingResponse represents the response from LLM sampling
type MCPSamplingResponse struct {
	Role       string                 `json:"role"`
	Content    MCPContent             `json:"content"`
	Model      string                 `json:"model"`
	StopReason string                 `json:"stopReason,omitempty"`
	Usage      *MCPTokenUsage         `json:"usage,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// MCPMessage represents a message in the conversation
type MCPMessage struct {
	Role    string     `json:"role"`
	Content MCPContent `json:"content"`
	Name    string     `json:"name,omitempty"`
}

// MCPContent represents different types of content
type MCPContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"` // base64 encoded for images/audio
	MimeType string `json:"mimeType,omitempty"`
}

// MCPModelPreferences represents preferences for model selection
type MCPModelPreferences struct {
	// Model hints (optional suggestions)
	Hints []MCPModelHint `json:"hints,omitempty"`

	// Priority values (0.0 to 1.0)
	CostPriority         float64 `json:"costPriority,omitempty"`
	SpeedPriority        float64 `json:"speedPriority,omitempty"`
	IntelligencePriority float64 `json:"intelligencePriority,omitempty"`
}

// MCPModelHint represents a model hint
type MCPModelHint struct {
	Name string `json:"name"`
}

// MCPTokenUsage represents token usage information
type MCPTokenUsage struct {
	PromptTokens     int `json:"promptTokens,omitempty"`
	CompletionTokens int `json:"completionTokens,omitempty"`
	TotalTokens      int `json:"totalTokens,omitempty"`
}

// MCPServerInfo represents server metadata discovered during initialization
type MCPServerInfo struct {
	Name    string `json:"name"`              // Required: server identifier
	Title   string `json:"title,omitempty"`   // Optional: human-readable title
	Version string `json:"version,omitempty"` // Optional: server version
}

// MCPServerCapabilities represents server capabilities discovered during initialization
type MCPServerCapabilities struct {
	Tools     *MCPToolCapabilities     `json:"tools,omitempty"`
	Resources *MCPResourceCapabilities `json:"resources,omitempty"`
	Prompts   *MCPPromptCapabilities   `json:"prompts,omitempty"`
}

// MCPToolCapabilities represents tool-related capabilities
type MCPToolCapabilities struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// MCPResourceCapabilities represents resource-related capabilities
type MCPResourceCapabilities struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// MCPPromptCapabilities represents prompt-related capabilities
type MCPPromptCapabilities struct {
	ListChanged bool `json:"listChanged,omitempty"`
}
