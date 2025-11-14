package mcp

import (
	"fmt"
	"strings"
)

// MCPError represents a structured error from MCP operations
type MCPError struct {
	Operation   string            // The operation that failed (e.g., "ListTools", "CallTool")
	ServerName  string            // Name of the MCP server
	ServerType  string            // Type of server (stdio, http, etc.)
	ErrorType   MCPErrorType      // Category of error
	Cause       error             // The underlying error
	Retryable   bool              // Whether the error might succeed on retry
	Metadata    map[string]string // Additional context
}

// MCPErrorType categorizes different types of MCP errors
type MCPErrorType string

const (
	// Connection errors
	MCPErrorTypeConnection     MCPErrorType = "CONNECTION_ERROR"
	MCPErrorTypeTimeout        MCPErrorType = "TIMEOUT_ERROR"
	MCPErrorTypeAuthentication MCPErrorType = "AUTHENTICATION_ERROR"

	// Server errors
	MCPErrorTypeServerNotFound   MCPErrorType = "SERVER_NOT_FOUND"
	MCPErrorTypeServerStartup    MCPErrorType = "SERVER_STARTUP_ERROR"
	MCPErrorTypeServerCrash      MCPErrorType = "SERVER_CRASH"

	// Tool errors
	MCPErrorTypeToolNotFound     MCPErrorType = "TOOL_NOT_FOUND"
	MCPErrorTypeToolInvalidArgs  MCPErrorType = "TOOL_INVALID_ARGS"
	MCPErrorTypeToolExecution    MCPErrorType = "TOOL_EXECUTION_ERROR"

	// Protocol errors
	MCPErrorTypeProtocol         MCPErrorType = "PROTOCOL_ERROR"
	MCPErrorTypeSerialization    MCPErrorType = "SERIALIZATION_ERROR"

	// Configuration errors
	MCPErrorTypeConfiguration    MCPErrorType = "CONFIGURATION_ERROR"
	MCPErrorTypeValidation       MCPErrorType = "VALIDATION_ERROR"

	// Unknown errors
	MCPErrorTypeUnknown          MCPErrorType = "UNKNOWN_ERROR"
)

// Error implements the error interface
func (e *MCPError) Error() string {
	var parts []string

	if e.ServerName != "" {
		parts = append(parts, fmt.Sprintf("MCP server '%s'", e.ServerName))
	} else {
		parts = append(parts, "MCP operation")
	}

	if e.Operation != "" {
		parts = append(parts, fmt.Sprintf("operation '%s'", e.Operation))
	}

	parts = append(parts, "failed")

	if e.ErrorType != MCPErrorTypeUnknown {
		parts = append(parts, fmt.Sprintf("(%s)", e.ErrorType))
	}

	message := strings.Join(parts, " ")

	if e.Cause != nil {
		message += fmt.Sprintf(": %v", e.Cause)
	}

	return message
}

// Unwrap returns the underlying error
func (e *MCPError) Unwrap() error {
	return e.Cause
}

// IsRetryable returns whether this error might succeed on retry
func (e *MCPError) IsRetryable() bool {
	return e.Retryable
}

// WithMetadata adds metadata to the error
func (e *MCPError) WithMetadata(key, value string) *MCPError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]string)
	}
	e.Metadata[key] = value
	return e
}

// NewMCPError creates a new MCP error
func NewMCPError(operation, serverName, serverType string, errorType MCPErrorType, cause error) *MCPError {
	return &MCPError{
		Operation:  operation,
		ServerName: serverName,
		ServerType: serverType,
		ErrorType:  errorType,
		Cause:      cause,
		Retryable:  isRetryableErrorType(errorType),
		Metadata:   make(map[string]string),
	}
}

// NewConnectionError creates a connection-related error
func NewConnectionError(serverName, serverType string, cause error) *MCPError {
	return NewMCPError("Connect", serverName, serverType, MCPErrorTypeConnection, cause)
}

// NewTimeoutError creates a timeout error
func NewTimeoutError(operation, serverName, serverType string, cause error) *MCPError {
	return NewMCPError(operation, serverName, serverType, MCPErrorTypeTimeout, cause)
}

// NewToolError creates a tool-related error
func NewToolError(toolName, serverName, serverType string, errorType MCPErrorType, cause error) *MCPError {
	err := NewMCPError("CallTool", serverName, serverType, errorType, cause)
	_ = err.WithMetadata("tool_name", toolName)
	return err
}

// NewServerError creates a server-related error
func NewServerError(serverName, serverType string, errorType MCPErrorType, cause error) *MCPError {
	return NewMCPError("ServerOperation", serverName, serverType, errorType, cause)
}

// NewConfigurationError creates a configuration error
func NewConfigurationError(operation string, cause error) *MCPError {
	return NewMCPError(operation, "", "", MCPErrorTypeConfiguration, cause)
}

// isRetryableErrorType determines if an error type is retryable
func isRetryableErrorType(errorType MCPErrorType) bool {
	switch errorType {
	case MCPErrorTypeConnection,
		 MCPErrorTypeTimeout,
		 MCPErrorTypeServerStartup,
		 MCPErrorTypeServerCrash:
		return true
	case MCPErrorTypeAuthentication,
		 MCPErrorTypeServerNotFound,
		 MCPErrorTypeToolNotFound,
		 MCPErrorTypeToolInvalidArgs,
		 MCPErrorTypeProtocol,
		 MCPErrorTypeSerialization,
		 MCPErrorTypeConfiguration,
		 MCPErrorTypeValidation:
		return false
	default:
		return false
	}
}

// ClassifyError attempts to classify an error based on its message
func ClassifyError(err error, operation, serverName, serverType string) *MCPError {
	if err == nil {
		return nil
	}

	// If it's already an MCPError, return it
	if mcpErr, ok := err.(*MCPError); ok {
		return mcpErr
	}

	errMsg := strings.ToLower(err.Error())

	// Classify based on error message content
	var errorType MCPErrorType
	switch {
	case strings.Contains(errMsg, "connection refused") ||
		 strings.Contains(errMsg, "connection reset") ||
		 strings.Contains(errMsg, "no route to host") ||
		 strings.Contains(errMsg, "network unreachable"):
		errorType = MCPErrorTypeConnection

	case strings.Contains(errMsg, "timeout") ||
		 strings.Contains(errMsg, "deadline exceeded") ||
		 strings.Contains(errMsg, "context deadline exceeded"):
		errorType = MCPErrorTypeTimeout

	case strings.Contains(errMsg, "authentication") ||
		 strings.Contains(errMsg, "unauthorized") ||
		 strings.Contains(errMsg, "forbidden") ||
		 strings.Contains(errMsg, "invalid token"):
		errorType = MCPErrorTypeAuthentication

	case strings.Contains(errMsg, "not found") ||
		 strings.Contains(errMsg, "no such file"):
		if operation == "CallTool" {
			errorType = MCPErrorTypeToolNotFound
		} else {
			errorType = MCPErrorTypeServerNotFound
		}

	case strings.Contains(errMsg, "invalid argument") ||
		 strings.Contains(errMsg, "invalid parameter") ||
		 strings.Contains(errMsg, "bad request"):
		errorType = MCPErrorTypeToolInvalidArgs

	case strings.Contains(errMsg, "json") ||
		 strings.Contains(errMsg, "unmarshal") ||
		 strings.Contains(errMsg, "marshal") ||
		 strings.Contains(errMsg, "parse"):
		errorType = MCPErrorTypeSerialization

	case strings.Contains(errMsg, "protocol") ||
		 strings.Contains(errMsg, "invalid response") ||
		 strings.Contains(errMsg, "unexpected"):
		errorType = MCPErrorTypeProtocol

	case strings.Contains(errMsg, "config") ||
		 strings.Contains(errMsg, "validation"):
		errorType = MCPErrorTypeConfiguration

	case strings.Contains(errMsg, "server crashed") ||
		 strings.Contains(errMsg, "process exited") ||
		 strings.Contains(errMsg, "broken pipe"):
		errorType = MCPErrorTypeServerCrash

	default:
		errorType = MCPErrorTypeUnknown
	}

	return NewMCPError(operation, serverName, serverType, errorType, err)
}

// FormatUserFriendlyError creates a user-friendly error message
func FormatUserFriendlyError(err error) string {
	mcpErr, ok := err.(*MCPError)
	if !ok {
		return err.Error()
	}

	switch mcpErr.ErrorType {
	case MCPErrorTypeConnection:
		if mcpErr.ServerType == "stdio" {
			return fmt.Sprintf("Could not start MCP server '%s'. Please check that the command is installed and accessible.", mcpErr.ServerName)
		}
		return fmt.Sprintf("Could not connect to MCP server '%s'. Please check the server URL and network connectivity.", mcpErr.ServerName)

	case MCPErrorTypeTimeout:
		return fmt.Sprintf("MCP server '%s' took too long to respond. This might be a temporary issue - please try again.", mcpErr.ServerName)

	case MCPErrorTypeAuthentication:
		return fmt.Sprintf("Authentication failed for MCP server '%s'. Please check your credentials or API key.", mcpErr.ServerName)

	case MCPErrorTypeServerNotFound:
		return "MCP server command not found. Please ensure the server is installed correctly."

	case MCPErrorTypeToolNotFound:
		toolName := mcpErr.Metadata["tool_name"]
		if toolName != "" {
			return fmt.Sprintf("Tool '%s' is not available on MCP server '%s'. Try listing available tools first.", toolName, mcpErr.ServerName)
		}
		return fmt.Sprintf("Requested tool is not available on MCP server '%s'.", mcpErr.ServerName)

	case MCPErrorTypeToolInvalidArgs:
		return "Invalid arguments provided to MCP tool. Please check the tool's parameter requirements."

	case MCPErrorTypeConfiguration:
		return fmt.Sprintf("MCP configuration error: %v", mcpErr.Cause)

	default:
		return fmt.Sprintf("MCP server '%s' error: %v", mcpErr.ServerName, mcpErr.Cause)
	}
}