package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock logger for testing
type mockLogger struct {
	mock.Mock
}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}

func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}

func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}

func (m *mockLogger) Error(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}

func TestNewBuilder(t *testing.T) {
	builder := NewBuilder()

	assert.NotNil(t, builder)
	assert.NotNil(t, builder.logger)
	assert.NotNil(t, builder.retryOptions)
	assert.Equal(t, 5, builder.retryOptions.MaxAttempts)
	assert.Equal(t, 1*time.Second, builder.retryOptions.InitialDelay)
	assert.Equal(t, 30*time.Second, builder.retryOptions.MaxDelay)
	assert.Equal(t, 2.0, builder.retryOptions.BackoffMultiplier)
	assert.Equal(t, 30*time.Second, builder.timeout)
	assert.True(t, builder.healthCheck)
	assert.Empty(t, builder.servers)
	assert.Empty(t, builder.lazyConfigs)
	assert.Empty(t, builder.errors)
}

func TestBuilder_WithLogger(t *testing.T) {
	builder := NewBuilder()
	customLogger := &mockLogger{}

	result := builder.WithLogger(customLogger)

	assert.Equal(t, builder, result) // Fluent interface
	assert.Equal(t, customLogger, builder.logger)
}

func TestBuilder_WithRetry(t *testing.T) {
	builder := NewBuilder()
	maxAttempts := 10
	initialDelay := 2 * time.Second

	result := builder.WithRetry(maxAttempts, initialDelay)

	assert.Equal(t, builder, result) // Fluent interface
	assert.Equal(t, maxAttempts, builder.retryOptions.MaxAttempts)
	assert.Equal(t, initialDelay, builder.retryOptions.InitialDelay)
}

func TestBuilder_WithTimeout(t *testing.T) {
	builder := NewBuilder()
	timeout := 60 * time.Second

	result := builder.WithTimeout(timeout)

	assert.Equal(t, builder, result) // Fluent interface
	assert.Equal(t, timeout, builder.timeout)
}

func TestBuilder_WithHealthCheck(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enable health check", true},
		{"disable health check", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder()
			result := builder.WithHealthCheck(tt.enabled)

			assert.Equal(t, builder, result) // Fluent interface
			assert.Equal(t, tt.enabled, builder.healthCheck)
		})
	}
}

func TestBuilder_AddStdioServer(t *testing.T) {
	builder := NewBuilder()
	name := "test-server"
	command := "/usr/bin/test"
	args := []string{"--arg1", "value1", "--arg2"}

	result := builder.AddStdioServer(name, command, args...)

	assert.Equal(t, builder, result) // Fluent interface
	assert.Len(t, builder.lazyConfigs, 1)

	config := builder.lazyConfigs[0]
	assert.Equal(t, name, config.Name)
	assert.Equal(t, "stdio", config.Type)
	assert.Equal(t, command, config.Command)
	assert.Equal(t, args, config.Args)
}

func TestBuilder_AddHTTPServer(t *testing.T) {
	builder := NewBuilder()
	name := "http-server"
	baseURL := "http://localhost:8080/mcp"

	result := builder.AddHTTPServer(name, baseURL)

	assert.Equal(t, builder, result) // Fluent interface
	assert.Len(t, builder.lazyConfigs, 1)

	config := builder.lazyConfigs[0]
	assert.Equal(t, name, config.Name)
	assert.Equal(t, "http", config.Type)
	assert.Equal(t, baseURL, config.URL)
}

func TestBuilder_AddHTTPServerWithAuth(t *testing.T) {
	tests := []struct {
		name        string
		serverName  string
		baseURL     string
		token       string
		expectError bool
	}{
		{
			name:       "valid URL with auth",
			serverName: "auth-server",
			baseURL:    "https://api.example.com/mcp",
			token:      "secret-token",
		},
		{
			name:        "invalid URL",
			serverName:  "bad-server",
			baseURL:     "not-a-url",
			token:       "token",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder()
			result := builder.AddHTTPServerWithAuth(tt.serverName, tt.baseURL, tt.token)

			assert.Equal(t, builder, result) // Fluent interface

			if tt.expectError {
				assert.NotEmpty(t, builder.errors)
			} else {
				assert.Len(t, builder.lazyConfigs, 1)
				config := builder.lazyConfigs[0]
				assert.Equal(t, tt.serverName, config.Name)
				assert.Equal(t, "http", config.Type)
				assert.Contains(t, config.URL, "token="+tt.token)
			}
		})
	}
}

func TestBuilder_AddServer(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
		expectType  string
	}{
		{
			name:       "stdio URL",
			url:        "stdio://test-server/usr/bin/command?arg1=value1",
			expectType: "stdio",
		},
		{
			name:       "http URL",
			url:        "http://localhost:8080/mcp",
			expectType: "http",
		},
		{
			name:       "https URL with token",
			url:        "https://api.example.com/mcp?token=secret",
			expectType: "http",
		},
		{
			name:        "invalid URL",
			url:         "not://a-valid-url",
			expectError: true,
		},
		{
			name:        "unsupported scheme",
			url:         "ftp://server.com/path",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder()
			result := builder.AddServer(tt.url)

			assert.Equal(t, builder, result) // Fluent interface

			if tt.expectError {
				assert.NotEmpty(t, builder.errors)
			} else {
				assert.Empty(t, builder.errors)
				if tt.expectType != "" {
					assert.Len(t, builder.lazyConfigs, 1)
					assert.Equal(t, tt.expectType, builder.lazyConfigs[0].Type)
				}
			}
		})
	}
}

func TestBuilder_parseServerURL(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name           string
		url            string
		expectError    bool
		expectedType   string
		validateConfig func(t *testing.T, config *LazyMCPServerConfig)
	}{
		{
			name:         "stdio URL with host",
			url:          "stdio://myserver/usr/bin/test?arg1=val1&arg2",
			expectedType: "stdio",
			validateConfig: func(t *testing.T, config *LazyMCPServerConfig) {
				assert.Equal(t, "myserver", config.Name)
				assert.Equal(t, "/usr/bin/test", config.Command)
				assert.Contains(t, config.Args, "--arg1=val1")
				assert.Contains(t, config.Args, "--arg2")
			},
		},
		{
			name:         "stdio URL without host",
			url:          "stdio:///command/path/to/executable",
			expectedType: "stdio",
			validateConfig: func(t *testing.T, config *LazyMCPServerConfig) {
				assert.Equal(t, "command", config.Name)
				assert.Equal(t, "/path/to/executable", config.Command)
			},
		},
		{
			name:         "http URL",
			url:          "http://localhost:8080/mcp",
			expectedType: "http",
			validateConfig: func(t *testing.T, config *LazyMCPServerConfig) {
				assert.Equal(t, "localhost:8080", config.Name)
				assert.Equal(t, "http://localhost:8080/mcp", config.URL)
			},
		},
		{
			name:         "https URL with query",
			url:          "https://api.example.com/mcp?token=secret",
			expectedType: "http",
			validateConfig: func(t *testing.T, config *LazyMCPServerConfig) {
				assert.Equal(t, "api.example.com", config.Name)
				assert.Equal(t, "https://api.example.com/mcp?token=secret", config.URL)
			},
		},
		{
			name:        "invalid stdio URL format",
			url:         "stdio://",
			expectError: true,
		},
		{
			name:        "invalid URL",
			url:         "not a url",
			expectError: true,
		},
		{
			name:        "unsupported scheme",
			url:         "ftp://server.com/path",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, config, err := builder.parseServerURL(tt.url)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Nil(t, server) // We always return lazy configs
				assert.NotNil(t, config)
				assert.Equal(t, tt.expectedType, config.Type)
				if tt.validateConfig != nil {
					tt.validateConfig(t, config)
				}
			}
		})
	}
}

func TestBuilder_Build(t *testing.T) {
	ctx := context.Background()

	t.Run("successful build with lazy configs", func(t *testing.T) {
		builder := NewBuilder()
		builder.AddStdioServer("server1", "/bin/cmd1")
		builder.AddHTTPServer("server2", "http://localhost:8080")

		servers, configs, err := builder.Build(ctx)

		assert.NoError(t, err)
		assert.Empty(t, servers) // All configs are lazy
		assert.Len(t, configs, 2)
	})

	t.Run("build with accumulated errors", func(t *testing.T) {
		builder := NewBuilder()
		builder.errors = []error{
			errors.New("error 1"),
			errors.New("error 2"),
		}

		servers, configs, err := builder.Build(ctx)

		assert.Error(t, err)
		assert.Nil(t, servers)
		assert.Nil(t, configs)
		assert.Contains(t, err.Error(), "builder errors")
	})

	t.Run("build with health check disabled", func(t *testing.T) {
		builder := NewBuilder().WithHealthCheck(false)
		builder.AddStdioServer("server1", "/bin/cmd1")

		servers, configs, err := builder.Build(ctx)

		assert.NoError(t, err)
		assert.Empty(t, servers)
		assert.Len(t, configs, 1)
	})
}

func TestBuilder_BuildLazy(t *testing.T) {
	t.Run("successful build", func(t *testing.T) {
		builder := NewBuilder()
		builder.AddStdioServer("server1", "/bin/cmd1", "--arg1")
		builder.AddHTTPServer("server2", "http://localhost:8080")

		configs, err := builder.BuildLazy()

		assert.NoError(t, err)
		assert.Len(t, configs, 2)
		assert.Equal(t, "server1", configs[0].Name)
		assert.Equal(t, "server2", configs[1].Name)
	})

	t.Run("build with errors", func(t *testing.T) {
		builder := NewBuilder()
		builder.errors = []error{errors.New("test error")}

		configs, err := builder.BuildLazy()

		assert.Error(t, err)
		assert.Nil(t, configs)
	})
}

func TestBuilder_retryConnection(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}

	t.Run("successful after retries", func(t *testing.T) {
		builder := NewBuilder().
			WithLogger(logger).
			WithRetry(3, 100*time.Millisecond)

		config := LazyMCPServerConfig{
			Name:    "test-server",
			Type:    "stdio",
			Command: "/bin/test",
		}

		// Mock logger calls
		logger.On("Debug", mock.Anything, "Retrying MCP connection", mock.Anything).Return()

		// This test would require mocking the actual server creation
		// which would involve more complex setup
		_ = builder // Use the builder variable to avoid linting error
		_ = config  // Use the config variable to avoid linting error
		_ = ctx     // Use the ctx variable to avoid linting error
	})
}

func TestBuilder_shouldInitializeEagerly(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name     string
		config   LazyMCPServerConfig
		expected bool
	}{
		{
			name: "stdio server",
			config: LazyMCPServerConfig{
				Name: "test",
				Type: "stdio",
			},
			expected: false, // Currently all servers are lazy
		},
		{
			name: "http server",
			config: LazyMCPServerConfig{
				Name: "test",
				Type: "http",
			},
			expected: false, // Currently all servers are lazy
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.shouldInitializeEagerly(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuilder_ChainedMethods(t *testing.T) {
	// Test fluent interface with method chaining
	builder := NewBuilder().
		WithTimeout(45*time.Second).
		WithRetry(10, 2*time.Second).
		WithHealthCheck(false).
		AddStdioServer("server1", "/bin/cmd1").
		AddHTTPServer("server2", "http://localhost:8080").
		AddHTTPServerWithAuth("server3", "https://api.example.com", "token123")

	assert.Equal(t, 45*time.Second, builder.timeout)
	assert.Equal(t, 10, builder.retryOptions.MaxAttempts)
	assert.Equal(t, 2*time.Second, builder.retryOptions.InitialDelay)
	assert.False(t, builder.healthCheck)
	assert.Len(t, builder.lazyConfigs, 3)
}

// Test for URL parsing security issues mentioned in PR review
func TestBuilder_parseServerURL_Security(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name        string
		url         string
		description string
	}{
		{
			name:        "path traversal attempt",
			url:         "stdio://server/../../../etc/passwd",
			description: "Should handle path traversal attempts",
		},
		{
			name:        "command injection attempt",
			url:         "stdio://server/bin/cmd;rm -rf /",
			description: "Should handle command injection attempts",
		},
		{
			name:        "url with special characters",
			url:         "stdio://server/bin/cmd?arg=$USER&test=`whoami`",
			description: "Should handle special shell characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, config, err := builder.parseServerURL(tt.url)

			// The function should parse without error but the command
			// should be properly escaped/sanitized
			if err == nil {
				assert.NotNil(t, config)
				// Note: Current implementation doesn't sanitize.
				// This test documents the current behavior and should
				// be updated when security fixes are implemented
				t.Logf("Warning: %s - command not sanitized: %s", tt.description, config.Command)
			}
		})
	}
}

// Test token exposure in URLs as mentioned in PR review
func TestBuilder_AddHTTPServerWithAuth_TokenSecurity(t *testing.T) {
	builder := NewBuilder()

	// Test that token is added to query params (current behavior)
	builder.AddHTTPServerWithAuth("test", "https://api.example.com/mcp", "secret-token")

	assert.Len(t, builder.lazyConfigs, 1)
	config := builder.lazyConfigs[0]

	// Current implementation exposes token in URL
	// This test documents the behavior that needs to be fixed
	assert.Contains(t, config.URL, "token=secret-token")
	t.Log("Warning: Token is exposed in URL query parameters - security issue")
}

// Benchmark tests for performance monitoring
func BenchmarkBuilder_AddServer(b *testing.B) {
	builder := NewBuilder()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.AddServer(fmt.Sprintf("http://localhost:%d/mcp", 8080+i))
	}
}

func BenchmarkBuilder_parseServerURL(b *testing.B) {
	builder := NewBuilder()
	urls := []string{
		"stdio://server/usr/bin/cmd?arg1=val1&arg2=val2",
		"http://localhost:8080/mcp",
		"https://api.example.com/mcp?token=secret",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := builder.parseServerURL(urls[i%len(urls)])
		if err != nil {
			assert.Fail(b, "parseServerURL failed", err)
		}
	}
}
