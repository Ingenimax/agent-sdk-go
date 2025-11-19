package mcp

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *MCPError
		expected string
	}{
		{
			name: "full error with all fields",
			err: &MCPError{
				Operation:  "CallTool",
				ServerName: "test-server",
				ServerType: "http",
				ErrorType:  MCPErrorTypeConnection,
				Cause:      errors.New("connection refused"),
			},
			expected: "MCP server 'test-server' operation 'CallTool' failed (CONNECTION_ERROR): connection refused",
		},
		{
			name: "error without server name",
			err: &MCPError{
				Operation: "ListTools",
				ErrorType: MCPErrorTypeTimeout,
				Cause:     errors.New("timeout"),
			},
			expected: "MCP operation operation 'ListTools' failed (TIMEOUT_ERROR): timeout",
		},
		{
			name: "error without operation",
			err: &MCPError{
				ServerName: "test-server",
				ErrorType:  MCPErrorTypeAuthentication,
				Cause:      errors.New("invalid token"),
			},
			expected: "MCP server 'test-server' failed (AUTHENTICATION_ERROR): invalid token",
		},
		{
			name: "error without cause",
			err: &MCPError{
				ServerName: "test-server",
				Operation:  "Connect",
				ErrorType:  MCPErrorTypeConnection,
			},
			expected: "MCP server 'test-server' operation 'Connect' failed (CONNECTION_ERROR)",
		},
		{
			name: "error with unknown type",
			err: &MCPError{
				ServerName: "test-server",
				Operation:  "DoSomething",
				ErrorType:  MCPErrorTypeUnknown,
				Cause:      errors.New("something went wrong"),
			},
			expected: "MCP server 'test-server' operation 'DoSomething' failed: something went wrong",
		},
		{
			name: "minimal error",
			err: &MCPError{
				ErrorType: MCPErrorTypeUnknown,
			},
			expected: "MCP operation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestMCPError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &MCPError{
		Cause: cause,
	}

	assert.Equal(t, cause, err.Unwrap())
	assert.True(t, errors.Is(err, cause))
}

func TestMCPError_IsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       *MCPError
		retryable bool
	}{
		{
			name:      "connection error is retryable",
			err:       &MCPError{ErrorType: MCPErrorTypeConnection, Retryable: true},
			retryable: true,
		},
		{
			name:      "timeout error is retryable",
			err:       &MCPError{ErrorType: MCPErrorTypeTimeout, Retryable: true},
			retryable: true,
		},
		{
			name:      "authentication error is not retryable",
			err:       &MCPError{ErrorType: MCPErrorTypeAuthentication, Retryable: false},
			retryable: false,
		},
		{
			name:      "validation error is not retryable",
			err:       &MCPError{ErrorType: MCPErrorTypeValidation, Retryable: false},
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.retryable, tt.err.IsRetryable())
		})
	}
}

func TestMCPError_WithMetadata(t *testing.T) {
	err := &MCPError{
		ServerName: "test-server",
		ErrorType:  MCPErrorTypeToolNotFound,
	}

	// Add metadata to nil map
	err = err.WithMetadata("tool_name", "test-tool")
	assert.NotNil(t, err.Metadata)
	assert.Equal(t, "test-tool", err.Metadata["tool_name"])

	// Add more metadata
	err = err.WithMetadata("attempt", "3")
	assert.Equal(t, "test-tool", err.Metadata["tool_name"])
	assert.Equal(t, "3", err.Metadata["attempt"])

	// Verify fluent interface
	err2 := &MCPError{}
	result := err2.WithMetadata("key1", "value1").WithMetadata("key2", "value2")
	assert.Equal(t, err2, result)
	assert.Equal(t, "value1", err2.Metadata["key1"])
	assert.Equal(t, "value2", err2.Metadata["key2"])
}

func TestNewMCPError(t *testing.T) {
	cause := errors.New("test error")
	err := NewMCPError("CallTool", "server1", "http", MCPErrorTypeConnection, cause)

	assert.Equal(t, "CallTool", err.Operation)
	assert.Equal(t, "server1", err.ServerName)
	assert.Equal(t, "http", err.ServerType)
	assert.Equal(t, MCPErrorTypeConnection, err.ErrorType)
	assert.Equal(t, cause, err.Cause)
	assert.True(t, err.Retryable)
	assert.NotNil(t, err.Metadata)
}

func TestNewConnectionError(t *testing.T) {
	cause := errors.New("connection refused")
	err := NewConnectionError("server1", "stdio", cause)

	assert.Equal(t, "Connect", err.Operation)
	assert.Equal(t, "server1", err.ServerName)
	assert.Equal(t, "stdio", err.ServerType)
	assert.Equal(t, MCPErrorTypeConnection, err.ErrorType)
	assert.Equal(t, cause, err.Cause)
	assert.True(t, err.Retryable)
}

func TestNewTimeoutError(t *testing.T) {
	cause := errors.New("timeout exceeded")
	err := NewTimeoutError("ListTools", "server1", "http", cause)

	assert.Equal(t, "ListTools", err.Operation)
	assert.Equal(t, "server1", err.ServerName)
	assert.Equal(t, "http", err.ServerType)
	assert.Equal(t, MCPErrorTypeTimeout, err.ErrorType)
	assert.Equal(t, cause, err.Cause)
	assert.True(t, err.Retryable)
}

func TestNewToolError(t *testing.T) {
	cause := errors.New("tool not found")
	err := NewToolError("my-tool", "server1", "http", MCPErrorTypeToolNotFound, cause)

	assert.Equal(t, "CallTool", err.Operation)
	assert.Equal(t, "server1", err.ServerName)
	assert.Equal(t, "http", err.ServerType)
	assert.Equal(t, MCPErrorTypeToolNotFound, err.ErrorType)
	assert.Equal(t, cause, err.Cause)
	assert.Equal(t, "my-tool", err.Metadata["tool_name"])
	assert.False(t, err.Retryable)
}

func TestNewServerError(t *testing.T) {
	cause := errors.New("server startup failed")
	err := NewServerError("server1", "stdio", MCPErrorTypeServerStartup, cause)

	assert.Equal(t, "ServerOperation", err.Operation)
	assert.Equal(t, "server1", err.ServerName)
	assert.Equal(t, "stdio", err.ServerType)
	assert.Equal(t, MCPErrorTypeServerStartup, err.ErrorType)
	assert.Equal(t, cause, err.Cause)
	assert.True(t, err.Retryable)
}

func TestNewConfigurationError(t *testing.T) {
	cause := errors.New("invalid configuration")
	err := NewConfigurationError("LoadConfig", cause)

	assert.Equal(t, "LoadConfig", err.Operation)
	assert.Equal(t, "", err.ServerName)
	assert.Equal(t, "", err.ServerType)
	assert.Equal(t, MCPErrorTypeConfiguration, err.ErrorType)
	assert.Equal(t, cause, err.Cause)
	assert.False(t, err.Retryable)
}

func TestIsRetryableErrorType(t *testing.T) {
	tests := []struct {
		errorType MCPErrorType
		retryable bool
	}{
		// Retryable errors
		{MCPErrorTypeConnection, true},
		{MCPErrorTypeTimeout, true},
		{MCPErrorTypeServerStartup, true},
		{MCPErrorTypeServerCrash, true},

		// Non-retryable errors
		{MCPErrorTypeAuthentication, false},
		{MCPErrorTypeServerNotFound, false},
		{MCPErrorTypeToolNotFound, false},
		{MCPErrorTypeToolInvalidArgs, false},
		{MCPErrorTypeProtocol, false},
		{MCPErrorTypeSerialization, false},
		{MCPErrorTypeConfiguration, false},
		{MCPErrorTypeValidation, false},
		{MCPErrorTypeUnknown, false},
		{"CUSTOM_TYPE", false}, // Unknown type defaults to false
	}

	for _, tt := range tests {
		t.Run(string(tt.errorType), func(t *testing.T) {
			assert.Equal(t, tt.retryable, isRetryableErrorType(tt.errorType))
		})
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		operation     string
		serverName    string
		serverType    string
		expectedType  MCPErrorType
		expectedRetry bool
	}{
		// Nil error
		{
			name: "nil error",
			err:  nil,
		},

		// Already MCPError
		{
			name: "already MCPError",
			err: &MCPError{
				ErrorType: MCPErrorTypeAuthentication,
				Retryable: false,
			},
			expectedType:  MCPErrorTypeAuthentication,
			expectedRetry: false,
		},

		// Connection errors
		{
			name:          "connection refused",
			err:           errors.New("connection refused"),
			operation:     "Connect",
			serverName:    "test",
			serverType:    "http",
			expectedType:  MCPErrorTypeConnection,
			expectedRetry: true,
		},
		{
			name:          "connection reset",
			err:           errors.New("connection reset by peer"),
			expectedType:  MCPErrorTypeConnection,
			expectedRetry: true,
		},
		{
			name:          "no route to host",
			err:           errors.New("no route to host"),
			expectedType:  MCPErrorTypeConnection,
			expectedRetry: true,
		},
		{
			name:          "network unreachable",
			err:           errors.New("network unreachable"),
			expectedType:  MCPErrorTypeConnection,
			expectedRetry: true,
		},

		// Timeout errors
		{
			name:          "timeout",
			err:           errors.New("operation timeout"),
			expectedType:  MCPErrorTypeTimeout,
			expectedRetry: true,
		},
		{
			name:          "deadline exceeded",
			err:           errors.New("deadline exceeded"),
			expectedType:  MCPErrorTypeTimeout,
			expectedRetry: true,
		},
		{
			name:          "context deadline exceeded",
			err:           errors.New("context deadline exceeded"),
			expectedType:  MCPErrorTypeTimeout,
			expectedRetry: true,
		},

		// Authentication errors
		{
			name:          "authentication failed",
			err:           errors.New("authentication failed"),
			expectedType:  MCPErrorTypeAuthentication,
			expectedRetry: false,
		},
		{
			name:          "unauthorized",
			err:           errors.New("401 unauthorized"),
			expectedType:  MCPErrorTypeAuthentication,
			expectedRetry: false,
		},
		{
			name:          "forbidden",
			err:           errors.New("403 forbidden"),
			expectedType:  MCPErrorTypeAuthentication,
			expectedRetry: false,
		},
		{
			name:          "invalid token",
			err:           errors.New("invalid token provided"),
			expectedType:  MCPErrorTypeAuthentication,
			expectedRetry: false,
		},

		// Not found errors - tool context
		{
			name:          "tool not found",
			err:           errors.New("tool not found"),
			operation:     "CallTool",
			expectedType:  MCPErrorTypeToolNotFound,
			expectedRetry: false,
		},

		// Not found errors - server context
		{
			name:          "server not found",
			err:           errors.New("server not found"),
			operation:     "StartServer",
			expectedType:  MCPErrorTypeServerNotFound,
			expectedRetry: false,
		},
		{
			name:          "no such file",
			err:           errors.New("no such file or directory"),
			expectedType:  MCPErrorTypeServerNotFound,
			expectedRetry: false,
		},

		// Invalid arguments
		{
			name:          "invalid argument",
			err:           errors.New("invalid argument provided"),
			expectedType:  MCPErrorTypeToolInvalidArgs,
			expectedRetry: false,
		},
		{
			name:          "invalid parameter",
			err:           errors.New("invalid parameter: name"),
			expectedType:  MCPErrorTypeToolInvalidArgs,
			expectedRetry: false,
		},
		{
			name:          "bad request",
			err:           errors.New("400 bad request"),
			expectedType:  MCPErrorTypeToolInvalidArgs,
			expectedRetry: false,
		},

		// Serialization errors
		{
			name:          "json error",
			err:           errors.New("json unmarshal error"),
			expectedType:  MCPErrorTypeSerialization,
			expectedRetry: false,
		},
		{
			name:          "unmarshal error",
			err:           errors.New("cannot unmarshal string into int"),
			expectedType:  MCPErrorTypeSerialization,
			expectedRetry: false,
		},
		{
			name:          "marshal error",
			err:           errors.New("failed to marshal response"),
			expectedType:  MCPErrorTypeSerialization,
			expectedRetry: false,
		},
		{
			name:          "parse error",
			err:           errors.New("failed to parse JSON"),
			expectedType:  MCPErrorTypeSerialization,
			expectedRetry: false,
		},

		// Protocol errors
		{
			name:          "protocol error",
			err:           errors.New("protocol version mismatch"),
			expectedType:  MCPErrorTypeProtocol,
			expectedRetry: false,
		},
		{
			name:          "invalid response",
			err:           errors.New("invalid response format"),
			expectedType:  MCPErrorTypeProtocol,
			expectedRetry: false,
		},
		{
			name:          "unexpected response",
			err:           errors.New("unexpected server response"),
			expectedType:  MCPErrorTypeProtocol,
			expectedRetry: false,
		},

		// Configuration errors
		{
			name:          "config error",
			err:           errors.New("invalid config file"),
			expectedType:  MCPErrorTypeConfiguration,
			expectedRetry: false,
		},
		{
			name:          "validation error",
			err:           errors.New("validation failed: missing required field"),
			expectedType:  MCPErrorTypeConfiguration,
			expectedRetry: false,
		},

		// Server crash errors
		{
			name:          "server crashed",
			err:           errors.New("server crashed unexpectedly"),
			expectedType:  MCPErrorTypeServerCrash,
			expectedRetry: true,
		},
		{
			name:          "process exited",
			err:           errors.New("process exited with code 1"),
			expectedType:  MCPErrorTypeServerCrash,
			expectedRetry: true,
		},
		{
			name:          "broken pipe",
			err:           errors.New("broken pipe"),
			expectedType:  MCPErrorTypeServerCrash,
			expectedRetry: true,
		},

		// Unknown errors
		{
			name:          "unknown error",
			err:           errors.New("something completely unexpected"),
			expectedType:  MCPErrorTypeUnknown,
			expectedRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err, tt.operation, tt.serverName, tt.serverType)

			if tt.err == nil {
				assert.Nil(t, result)
				return
			}

			// If already MCPError, should return the same
			if mcpErr, ok := tt.err.(*MCPError); ok {
				assert.Equal(t, mcpErr, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedType, result.ErrorType)
			assert.Equal(t, tt.expectedRetry, result.Retryable)
			assert.Equal(t, tt.operation, result.Operation)
			assert.Equal(t, tt.serverName, result.ServerName)
			assert.Equal(t, tt.serverType, result.ServerType)
			assert.Equal(t, tt.err, result.Cause)
		})
	}
}

func TestFormatUserFriendlyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "non-MCP error",
			err:      errors.New("generic error"),
			expected: "generic error",
		},
		{
			name: "connection error - stdio",
			err: &MCPError{
				ServerName: "test-server",
				ServerType: "stdio",
				ErrorType:  MCPErrorTypeConnection,
			},
			expected: "Could not start MCP server 'test-server'. Please check that the command is installed and accessible.",
		},
		{
			name: "connection error - http",
			err: &MCPError{
				ServerName: "api-server",
				ServerType: "http",
				ErrorType:  MCPErrorTypeConnection,
			},
			expected: "Could not connect to MCP server 'api-server'. Please check the server URL and network connectivity.",
		},
		{
			name: "timeout error",
			err: &MCPError{
				ServerName: "slow-server",
				ErrorType:  MCPErrorTypeTimeout,
			},
			expected: "MCP server 'slow-server' took too long to respond. This might be a temporary issue - please try again.",
		},
		{
			name: "authentication error",
			err: &MCPError{
				ServerName: "secure-server",
				ErrorType:  MCPErrorTypeAuthentication,
			},
			expected: "Authentication failed for MCP server 'secure-server'. Please check your credentials or API key.",
		},
		{
			name: "server not found",
			err: &MCPError{
				ErrorType: MCPErrorTypeServerNotFound,
			},
			expected: "MCP server command not found. Please ensure the server is installed correctly.",
		},
		{
			name: "tool not found with name",
			err: &MCPError{
				ServerName: "tool-server",
				ErrorType:  MCPErrorTypeToolNotFound,
				Metadata:   map[string]string{"tool_name": "my-tool"},
			},
			expected: "Tool 'my-tool' is not available on MCP server 'tool-server'. Try listing available tools first.",
		},
		{
			name: "tool not found without name",
			err: &MCPError{
				ServerName: "tool-server",
				ErrorType:  MCPErrorTypeToolNotFound,
				Metadata:   map[string]string{},
			},
			expected: "Requested tool is not available on MCP server 'tool-server'.",
		},
		{
			name: "invalid arguments",
			err: &MCPError{
				ErrorType: MCPErrorTypeToolInvalidArgs,
			},
			expected: "Invalid arguments provided to MCP tool. Please check the tool's parameter requirements.",
		},
		{
			name: "configuration error",
			err: &MCPError{
				ErrorType: MCPErrorTypeConfiguration,
				Cause:     errors.New("missing API key"),
			},
			expected: "MCP configuration error: missing API key",
		},
		{
			name: "unknown error",
			err: &MCPError{
				ServerName: "mystery-server",
				ErrorType:  MCPErrorTypeUnknown,
				Cause:      errors.New("something went wrong"),
			},
			expected: "MCP server 'mystery-server' error: something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatUserFriendlyError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test error string generation complexity mentioned in PR review
func TestMCPError_Error_Complexity(t *testing.T) {
	// Test that error string generation is efficient
	err := &MCPError{
		Operation:  "VeryLongOperationName",
		ServerName: strings.Repeat("server", 100), // Very long server name
		ServerType: "http",
		ErrorType:  MCPErrorTypeConnection,
		Cause:      errors.New(strings.Repeat("error ", 100)), // Very long error message
	}

	// Should handle long strings efficiently
	result := err.Error()
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "VeryLongOperationName")

	// Benchmark string concatenation
	b := &testing.B{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}

// Test for error classification complexity mentioned in PR review
func TestClassifyError_Complexity(t *testing.T) {
	// Create a complex error message that matches multiple patterns
	complexErr := errors.New("connection timeout: authentication failed, json parse error, server not found")

	result := ClassifyError(complexErr, "TestOp", "test-server", "http")

	// Should prioritize first match (connection)
	assert.Equal(t, MCPErrorTypeConnection, result.ErrorType)

	// Test performance with many classifications
	b := &testing.B{}
	errors := []error{
		errors.New("connection refused"),
		errors.New("timeout exceeded"),
		errors.New("authentication failed"),
		errors.New("json unmarshal error"),
		errors.New("server crashed"),
		errors.New("unknown error"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ClassifyError(errors[i%len(errors)], "Op", "server", "type")
	}
}

// Test concurrent access to metadata
func TestMCPError_WithMetadata_Concurrent(t *testing.T) {
	err := &MCPError{
		ServerName: "test-server",
		ErrorType:  MCPErrorTypeToolNotFound,
	}

	// WithMetadata is now thread-safe with mutex protection
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			_ = err.WithMetadata(fmt.Sprintf("key%d", idx), fmt.Sprintf("value%d", idx))
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Thread-safe operation should complete without race conditions
	assert.NotNil(t, err.Metadata)
	assert.Equal(t, 10, len(err.Metadata)) // All 10 metadata items should be added
}

// Benchmark error creation
func BenchmarkNewMCPError(b *testing.B) {
	cause := errors.New("test error")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewMCPError("CallTool", "server", "http", MCPErrorTypeConnection, cause)
	}
}

// Benchmark error classification
func BenchmarkClassifyError(b *testing.B) {
	errors := []error{
		errors.New("connection refused"),
		errors.New("timeout exceeded"),
		errors.New("authentication failed"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ClassifyError(errors[i%len(errors)], "Op", "server", "type")
	}
}

// Benchmark error formatting
func BenchmarkFormatUserFriendlyError(b *testing.B) {
	err := &MCPError{
		ServerName: "test-server",
		ErrorType:  MCPErrorTypeConnection,
		ServerType: "http",
		Metadata:   map[string]string{"tool_name": "test-tool"},
		Cause:      errors.New("connection refused"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FormatUserFriendlyError(err)
	}
}
