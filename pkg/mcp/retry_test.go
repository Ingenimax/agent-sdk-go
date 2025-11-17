package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock MCP Server for testing
type mockMCPServer struct {
	mock.Mock
}

func (m *mockMCPServer) Initialize(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockMCPServer) ListTools(ctx context.Context) ([]interfaces.MCPTool, error) {
	args := m.Called(ctx)
	if tools := args.Get(0); tools != nil {
		return tools.([]interfaces.MCPTool), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMCPServer) CallTool(ctx context.Context, name string, toolArgs interface{}) (*interfaces.MCPToolResponse, error) {
	args := m.Called(ctx, name, toolArgs)
	if resp := args.Get(0); resp != nil {
		return resp.(*interfaces.MCPToolResponse), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMCPServer) ListResources(ctx context.Context) ([]interfaces.MCPResource, error) {
	args := m.Called(ctx)
	if resources := args.Get(0); resources != nil {
		return resources.([]interfaces.MCPResource), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMCPServer) GetResource(ctx context.Context, uri string) (*interfaces.MCPResourceContent, error) {
	args := m.Called(ctx, uri)
	if content := args.Get(0); content != nil {
		return content.(*interfaces.MCPResourceContent), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMCPServer) WatchResource(ctx context.Context, uri string) (<-chan interfaces.MCPResourceUpdate, error) {
	args := m.Called(ctx, uri)
	if ch := args.Get(0); ch != nil {
		return ch.(<-chan interfaces.MCPResourceUpdate), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMCPServer) ListPrompts(ctx context.Context) ([]interfaces.MCPPrompt, error) {
	args := m.Called(ctx)
	if prompts := args.Get(0); prompts != nil {
		return prompts.([]interfaces.MCPPrompt), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMCPServer) GetPrompt(ctx context.Context, name string, variables map[string]interface{}) (*interfaces.MCPPromptResult, error) {
	args := m.Called(ctx, name, variables)
	if result := args.Get(0); result != nil {
		return result.(*interfaces.MCPPromptResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMCPServer) CreateMessage(ctx context.Context, request *interfaces.MCPSamplingRequest) (*interfaces.MCPSamplingResponse, error) {
	args := m.Called(ctx, request)
	if resp := args.Get(0); resp != nil {
		return resp.(*interfaces.MCPSamplingResponse), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMCPServer) GetServerInfo() (*interfaces.MCPServerInfo, error) {
	args := m.Called()
	if info := args.Get(0); info != nil {
		return info.(*interfaces.MCPServerInfo), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMCPServer) GetCapabilities() (*interfaces.MCPServerCapabilities, error) {
	args := m.Called()
	if caps := args.Get(0); caps != nil {
		return caps.(*interfaces.MCPServerCapabilities), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMCPServer) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 3, config.MaxAttempts)
	assert.Equal(t, 1*time.Second, config.InitialDelay)
	assert.Equal(t, 30*time.Second, config.MaxDelay)
	assert.Equal(t, 2.0, config.BackoffMultiplier)
	assert.Contains(t, config.RetryableErrors, "connection refused")
	assert.Contains(t, config.RetryableErrors, "connection reset")
	assert.Contains(t, config.RetryableErrors, "timeout")
}

func TestNewRetryableServer(t *testing.T) {
	mockServer := &mockMCPServer{}

	t.Run("with custom config", func(t *testing.T) {
		config := &RetryConfig{
			MaxAttempts:       5,
			InitialDelay:      2 * time.Second,
			MaxDelay:          60 * time.Second,
			BackoffMultiplier: 3.0,
		}

		server := NewRetryableServer(mockServer, config)
		retryable, ok := server.(*RetryableServer)

		assert.True(t, ok)
		assert.Equal(t, mockServer, retryable.server)
		assert.Equal(t, config, retryable.config)
		assert.NotNil(t, retryable.logger)
	})

	t.Run("with nil config uses default", func(t *testing.T) {
		server := NewRetryableServer(mockServer, nil)
		retryable, ok := server.(*RetryableServer)

		assert.True(t, ok)
		assert.NotNil(t, retryable.config)
		assert.Equal(t, 3, retryable.config.MaxAttempts)
	})
}

func TestRetryableServer_Initialize(t *testing.T) {
	ctx := context.Background()

	t.Run("success on first attempt", func(t *testing.T) {
		mockServer := &mockMCPServer{}
		mockServer.On("Initialize", ctx).Return(nil).Once()

		server := NewRetryableServer(mockServer, nil)
		err := server.Initialize(ctx)

		assert.NoError(t, err)
		mockServer.AssertNumberOfCalls(t, "Initialize", 1)
	})

	t.Run("success after retry", func(t *testing.T) {
		mockServer := &mockMCPServer{}
		config := &RetryConfig{
			MaxAttempts:       3,
			InitialDelay:      10 * time.Millisecond,
			MaxDelay:          100 * time.Millisecond,
			BackoffMultiplier: 2.0,
			RetryableErrors:   []string{"connection refused"},
		}

		mockServer.On("Initialize", ctx).Return(errors.New("connection refused")).Once()
		mockServer.On("Initialize", ctx).Return(nil).Once()

		server := NewRetryableServer(mockServer, config)
		err := server.Initialize(ctx)

		assert.NoError(t, err)
		mockServer.AssertNumberOfCalls(t, "Initialize", 2)
	})

	t.Run("failure after max attempts", func(t *testing.T) {
		mockServer := &mockMCPServer{}
		config := &RetryConfig{
			MaxAttempts:       2,
			InitialDelay:      10 * time.Millisecond,
			MaxDelay:          100 * time.Millisecond,
			BackoffMultiplier: 2.0,
			RetryableErrors:   []string{"timeout"},
		}

		mockServer.On("Initialize", ctx).Return(errors.New("timeout")).Times(2)

		server := NewRetryableServer(mockServer, config)
		err := server.Initialize(ctx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed after 2 attempts")
		mockServer.AssertNumberOfCalls(t, "Initialize", 2)
	})

	t.Run("non-retryable error", func(t *testing.T) {
		mockServer := &mockMCPServer{}
		config := &RetryConfig{
			MaxAttempts:     3,
			RetryableErrors: []string{"connection refused"},
		}

		mockServer.On("Initialize", ctx).Return(errors.New("authentication failed")).Once()

		server := NewRetryableServer(mockServer, config)
		err := server.Initialize(ctx)

		assert.Error(t, err)
		assert.Equal(t, "authentication failed", err.Error())
		mockServer.AssertNumberOfCalls(t, "Initialize", 1)
	})

	t.Run("context cancellation", func(t *testing.T) {
		mockServer := &mockMCPServer{}
		config := &RetryConfig{
			MaxAttempts:     3,
			InitialDelay:    100 * time.Millisecond,
			RetryableErrors: []string{"timeout"},
		}

		ctx, cancel := context.WithCancel(context.Background())

		mockServer.On("Initialize", ctx).Return(errors.New("timeout")).Once()

		// Cancel context after first failure
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		server := NewRetryableServer(mockServer, config)
		err := server.Initialize(ctx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cancelled")
		mockServer.AssertNumberOfCalls(t, "Initialize", 1)
	})
}

func TestRetryableServer_ListTools(t *testing.T) {
	ctx := context.Background()
	expectedTools := []interfaces.MCPTool{
		{Name: "tool1", Description: "Test tool 1"},
		{Name: "tool2", Description: "Test tool 2"},
	}

	t.Run("success with retry", func(t *testing.T) {
		mockServer := &mockMCPServer{}
		config := &RetryConfig{
			MaxAttempts:     3,
			InitialDelay:    10 * time.Millisecond,
			RetryableErrors: []string{"temporary failure"},
		}

		mockServer.On("ListTools", ctx).Return(nil, errors.New("temporary failure")).Once()
		mockServer.On("ListTools", ctx).Return(expectedTools, nil).Once()

		server := NewRetryableServer(mockServer, config)
		tools, err := server.ListTools(ctx)

		assert.NoError(t, err)
		assert.Equal(t, expectedTools, tools)
		mockServer.AssertNumberOfCalls(t, "ListTools", 2)
	})
}

func TestRetryableServer_CallTool(t *testing.T) {
	ctx := context.Background()
	toolName := "test-tool"
	toolArgs := map[string]interface{}{"param": "value"}
	expectedResponse := &interfaces.MCPToolResponse{Content: "result"}

	t.Run("success with retry", func(t *testing.T) {
		mockServer := &mockMCPServer{}
		config := &RetryConfig{
			MaxAttempts:     3,
			InitialDelay:    10 * time.Millisecond,
			RetryableErrors: []string{"server not ready"},
		}

		mockServer.On("CallTool", ctx, toolName, toolArgs).Return(nil, errors.New("server not ready")).Once()
		mockServer.On("CallTool", ctx, toolName, toolArgs).Return(expectedResponse, nil).Once()

		server := NewRetryableServer(mockServer, config)
		response, err := server.CallTool(ctx, toolName, toolArgs)

		assert.NoError(t, err)
		assert.Equal(t, expectedResponse, response)
		mockServer.AssertNumberOfCalls(t, "CallTool", 2)
	})
}

func TestRetryableServer_WatchResource(t *testing.T) {
	ctx := context.Background()
	uri := "test://resource"
	expectedChannel := make(chan interfaces.MCPResourceUpdate)

	t.Run("no retry for watch", func(t *testing.T) {
		mockServer := &mockMCPServer{}
		mockServer.On("WatchResource", ctx, uri).Return((<-chan interfaces.MCPResourceUpdate)(expectedChannel), nil).Once()

		server := NewRetryableServer(mockServer, nil)
		ch, err := server.WatchResource(ctx, uri)

		assert.NoError(t, err)
		assert.Equal(t, (<-chan interfaces.MCPResourceUpdate)(expectedChannel), ch)
		mockServer.AssertNumberOfCalls(t, "WatchResource", 1) // No retry for watch
	})
}

func TestRetryableServer_Close(t *testing.T) {
	t.Run("close without retry", func(t *testing.T) {
		mockServer := &mockMCPServer{}
		mockServer.On("Close").Return(nil).Once()

		server := NewRetryableServer(mockServer, nil)
		err := server.Close()

		assert.NoError(t, err)
		mockServer.AssertNumberOfCalls(t, "Close", 1) // No retry for close
	})
}

func TestRetryableServer_shouldRetry(t *testing.T) {
	config := &RetryConfig{
		RetryableErrors: []string{
			"connection refused",
			"timeout",
			"SERVER NOT READY", // Test case insensitive
		},
	}

	server := &RetryableServer{config: config}

	tests := []struct {
		name        string
		err         error
		shouldRetry bool
	}{
		{
			name:        "nil error",
			err:         nil,
			shouldRetry: false,
		},
		{
			name:        "retryable - connection refused",
			err:         errors.New("connection refused"),
			shouldRetry: true,
		},
		{
			name:        "retryable - timeout",
			err:         errors.New("request timeout"),
			shouldRetry: true,
		},
		{
			name:        "retryable - case insensitive",
			err:         errors.New("server not ready"),
			shouldRetry: true,
		},
		{
			name:        "non-retryable",
			err:         errors.New("authentication failed"),
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := server.shouldRetry(tt.err)
			assert.Equal(t, tt.shouldRetry, result)
		})
	}
}

func TestRetryableServer_calculateBackoff(t *testing.T) {
	config := &RetryConfig{
		BackoffMultiplier: 2.0,
		MaxDelay:          100 * time.Millisecond,
	}

	server := &RetryableServer{config: config}

	tests := []struct {
		name         string
		currentDelay time.Duration
		maxExpected  time.Duration
		minExpected  time.Duration
	}{
		{
			name:         "small delay",
			currentDelay: 10 * time.Millisecond,
			minExpected:  18 * time.Millisecond, // 20ms - 10% jitter
			maxExpected:  22 * time.Millisecond, // 20ms + 10% jitter
		},
		{
			name:         "approaching max",
			currentDelay: 60 * time.Millisecond,
			minExpected:  100 * time.Millisecond, // Capped at max
			maxExpected:  100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to test jitter
			for i := 0; i < 10; i++ {
				result := server.calculateBackoff(tt.currentDelay)
				assert.LessOrEqual(t, result, config.MaxDelay)

				// For values not at max, check jitter range
				if tt.currentDelay*2 < config.MaxDelay {
					assert.GreaterOrEqual(t, result, tt.minExpected)
					assert.LessOrEqual(t, result, tt.maxExpected)
				}
			}
		})
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		str      string
		substr   string
		expected bool
	}{
		{"Connection Refused", "connection refused", true},
		{"CONNECTION REFUSED", "connection refused", true},
		{"connection refused", "CONNECTION REFUSED", true},
		{"timeout error", "timeout", true},
		{"authentication failed", "timeout", false},
		{"", "test", false},
		{"test", "", true},
		{"test", "testing", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s contains %s", tt.str, tt.substr), func(t *testing.T) {
			result := containsIgnoreCase(tt.str, tt.substr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test for race condition in randomFloat as mentioned in PR review
func TestRandomFloat_RaceCondition(t *testing.T) {
	// Test concurrent access to randomFloat
	var wg sync.WaitGroup
	results := make([]float64, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = randomFloat()
		}(i)
	}

	wg.Wait()

	// Check all results are in valid range
	for _, r := range results {
		assert.GreaterOrEqual(t, r, 0.0)
		assert.LessOrEqual(t, r, 1.0)
	}

	// Note: Current implementation has race condition potential
	t.Log("Warning: randomFloat() implementation has potential race conditions - should use math/rand with proper seeding")
}


func TestRetryWithExponentialBackoff(t *testing.T) {
	ctx := context.Background()

	t.Run("success on first attempt", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return nil
		}

		err := RetryWithExponentialBackoff(ctx, operation, nil)

		assert.NoError(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("success after retry", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary error")
			}
			return nil
		}

		config := &RetryConfig{
			MaxAttempts:       5,
			InitialDelay:      10 * time.Millisecond,
			MaxDelay:          100 * time.Millisecond,
			BackoffMultiplier: 2.0,
		}

		err := RetryWithExponentialBackoff(ctx, operation, config)

		assert.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("failure after max attempts", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return errors.New("persistent error")
		}

		config := &RetryConfig{
			MaxAttempts:       3,
			InitialDelay:      10 * time.Millisecond,
			MaxDelay:          100 * time.Millisecond,
			BackoffMultiplier: 2.0,
		}

		err := RetryWithExponentialBackoff(ctx, operation, config)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed after 3 attempts")
		assert.Equal(t, 3, attempts)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0

		operation := func() error {
			attempts++
			if attempts == 1 {
				go func() {
					time.Sleep(20 * time.Millisecond)
					cancel()
				}()
			}
			return errors.New("error")
		}

		config := &RetryConfig{
			MaxAttempts:  5,
			InitialDelay: 50 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
		}

		err := RetryWithExponentialBackoff(ctx, operation, config)

		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
		// Context is canceled during the delay between attempts, so only 1 attempt is made
		assert.Equal(t, 1, attempts)
	})

	t.Run("with nil config uses default", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			if attempts < 2 {
				return errors.New("error")
			}
			return nil
		}

		err := RetryWithExponentialBackoff(ctx, operation, nil)

		assert.NoError(t, err)
		assert.Equal(t, 2, attempts)
	})
}

// Test complex retry scenarios
func TestRetryableServer_ComplexScenarios(t *testing.T) {
	ctx := context.Background()

	t.Run("mixed success and failure", func(t *testing.T) {
		mockServer := &mockMCPServer{}
		config := &RetryConfig{
			MaxAttempts:     5,
			InitialDelay:    10 * time.Millisecond,
			RetryableErrors: []string{"temporary"},
		}

		// Initialize succeeds
		mockServer.On("Initialize", ctx).Return(nil).Once()

		// ListTools fails twice then succeeds
		mockServer.On("ListTools", ctx).Return(nil, errors.New("temporary error")).Twice()
		mockServer.On("ListTools", ctx).Return([]interfaces.MCPTool{{Name: "tool1"}}, nil).Once()

		// CallTool fails with non-retryable error
		mockServer.On("CallTool", ctx, "tool1", mock.Anything).Return(nil, errors.New("permission denied")).Once()

		server := NewRetryableServer(mockServer, config)

		// Test sequence
		err := server.Initialize(ctx)
		assert.NoError(t, err)

		tools, err := server.ListTools(ctx)
		assert.NoError(t, err)
		assert.Len(t, tools, 1)

		_, err = server.CallTool(ctx, "tool1", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")

		mockServer.AssertExpectations(t)
	})
}

// Benchmarks
func BenchmarkRetryableServer_NoRetry(b *testing.B) {
	ctx := context.Background()
	mockServer := &mockMCPServer{}
	mockServer.On("Initialize", ctx).Return(nil)

	server := NewRetryableServer(mockServer, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := server.Initialize(ctx)
		if err != nil {
			assert.Fail(b, "Initialize failed", err)
		}
	}
}

func BenchmarkRetryableServer_WithRetry(b *testing.B) {
	ctx := context.Background()
	mockServer := &mockMCPServer{}
	config := &RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    1 * time.Millisecond,
		RetryableErrors: []string{"temporary"},
	}

	// Fail once then succeed
	mockServer.On("Initialize", ctx).Return(errors.New("temporary")).Once()
	mockServer.On("Initialize", ctx).Return(nil)

	server := NewRetryableServer(mockServer, config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := server.Initialize(ctx)
		if err != nil {
			assert.Fail(b, "Initialize failed", err)
		}
	}
}

func BenchmarkCalculateBackoff(b *testing.B) {
	server := &RetryableServer{
		config: &RetryConfig{
			BackoffMultiplier: 2.0,
			MaxDelay:          30 * time.Second,
		},
	}

	delays := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.calculateBackoff(delays[i%len(delays)])
	}
}

func BenchmarkRandomFloat(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		randomFloat()
	}
}
