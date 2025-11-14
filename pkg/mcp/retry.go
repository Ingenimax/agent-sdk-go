package mcp

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// RetryConfig configures retry behavior for MCP operations
type RetryConfig struct {
	MaxAttempts       int
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	BackoffMultiplier float64
	RetryableErrors   []string // Error message substrings that should trigger retry
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		RetryableErrors: []string{
			"connection refused",
			"connection reset",
			"timeout",
			"temporary failure",
			"server not ready",
			"initialization failed",
		},
	}
}

// RetryableServer wraps an MCP server with retry logic
type RetryableServer struct {
	server interfaces.MCPServer
	config *RetryConfig
	logger logging.Logger
}

// NewRetryableServer creates a new MCP server wrapper with retry capabilities
func NewRetryableServer(server interfaces.MCPServer, config *RetryConfig) interfaces.MCPServer {
	if config == nil {
		config = DefaultRetryConfig()
	}
	return &RetryableServer{
		server: server,
		config: config,
		logger: logging.New(),
	}
}

// Initialize initializes the connection with retry logic
func (r *RetryableServer) Initialize(ctx context.Context) error {
	return r.retryOperation(ctx, "Initialize", func() error {
		return r.server.Initialize(ctx)
	})
}

// ListTools lists tools with retry logic
func (r *RetryableServer) ListTools(ctx context.Context) ([]interfaces.MCPTool, error) {
	var result []interfaces.MCPTool
	err := r.retryOperation(ctx, "ListTools", func() error {
		tools, err := r.server.ListTools(ctx)
		if err != nil {
			return err
		}
		result = tools
		return nil
	})
	return result, err
}

// CallTool calls a tool with retry logic
func (r *RetryableServer) CallTool(ctx context.Context, name string, args interface{}) (*interfaces.MCPToolResponse, error) {
	var result *interfaces.MCPToolResponse
	err := r.retryOperation(ctx, fmt.Sprintf("CallTool(%s)", name), func() error {
		response, err := r.server.CallTool(ctx, name, args)
		if err != nil {
			return err
		}
		result = response
		return nil
	})
	return result, err
}

// ListResources lists resources with retry logic
func (r *RetryableServer) ListResources(ctx context.Context) ([]interfaces.MCPResource, error) {
	var result []interfaces.MCPResource
	err := r.retryOperation(ctx, "ListResources", func() error {
		resources, err := r.server.ListResources(ctx)
		if err != nil {
			return err
		}
		result = resources
		return nil
	})
	return result, err
}

// GetResource gets a resource with retry logic
func (r *RetryableServer) GetResource(ctx context.Context, uri string) (*interfaces.MCPResourceContent, error) {
	var result *interfaces.MCPResourceContent
	err := r.retryOperation(ctx, fmt.Sprintf("GetResource(%s)", uri), func() error {
		resource, err := r.server.GetResource(ctx, uri)
		if err != nil {
			return err
		}
		result = resource
		return nil
	})
	return result, err
}

// WatchResource watches a resource (no retry needed as it's a continuous operation)
func (r *RetryableServer) WatchResource(ctx context.Context, uri string) (<-chan interfaces.MCPResourceUpdate, error) {
	return r.server.WatchResource(ctx, uri)
}

// ListPrompts lists prompts with retry logic
func (r *RetryableServer) ListPrompts(ctx context.Context) ([]interfaces.MCPPrompt, error) {
	var result []interfaces.MCPPrompt
	err := r.retryOperation(ctx, "ListPrompts", func() error {
		prompts, err := r.server.ListPrompts(ctx)
		if err != nil {
			return err
		}
		result = prompts
		return nil
	})
	return result, err
}

// GetPrompt gets a prompt with retry logic
func (r *RetryableServer) GetPrompt(ctx context.Context, name string, variables map[string]interface{}) (*interfaces.MCPPromptResult, error) {
	var result *interfaces.MCPPromptResult
	err := r.retryOperation(ctx, fmt.Sprintf("GetPrompt(%s)", name), func() error {
		prompt, err := r.server.GetPrompt(ctx, name, variables)
		if err != nil {
			return err
		}
		result = prompt
		return nil
	})
	return result, err
}

// CreateMessage creates a message with retry logic
func (r *RetryableServer) CreateMessage(ctx context.Context, request *interfaces.MCPSamplingRequest) (*interfaces.MCPSamplingResponse, error) {
	var result *interfaces.MCPSamplingResponse
	err := r.retryOperation(ctx, "CreateMessage", func() error {
		response, err := r.server.CreateMessage(ctx, request)
		if err != nil {
			return err
		}
		result = response
		return nil
	})
	return result, err
}

// Close closes the connection (no retry needed)
func (r *RetryableServer) Close() error {
	return r.server.Close()
}

// retryOperation executes an operation with exponential backoff retry
func (r *RetryableServer) retryOperation(ctx context.Context, operationName string, operation func() error) error {
	delay := r.config.InitialDelay

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		// Execute the operation
		err := operation()

		// Success - return immediately
		if err == nil {
			if attempt > 1 {
				r.logger.Info(ctx, "Operation succeeded after retry", map[string]interface{}{
					"operation": operationName,
					"attempt":   attempt,
				})
			}
			return nil
		}

		// Check if we should retry this error
		if !r.shouldRetry(err) {
			r.logger.Debug(ctx, "Error is not retryable", map[string]interface{}{
				"operation": operationName,
				"error":     err.Error(),
			})
			return err
		}

		// Check if we've exhausted attempts
		if attempt >= r.config.MaxAttempts {
			r.logger.Error(ctx, "Operation failed after max attempts", map[string]interface{}{
				"operation":    operationName,
				"max_attempts": r.config.MaxAttempts,
				"error":        err.Error(),
			})
			return fmt.Errorf("%s failed after %d attempts: %w", operationName, r.config.MaxAttempts, err)
		}

		// Log retry attempt
		r.logger.Warn(ctx, "Operation failed, retrying", map[string]interface{}{
			"operation":      operationName,
			"attempt":        attempt,
			"max_attempts":   r.config.MaxAttempts,
			"retry_delay_ms": delay.Milliseconds(),
			"error":          err.Error(),
		})

		// Wait before retry
		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			// Context cancelled
			return fmt.Errorf("%s cancelled: %w", operationName, ctx.Err())
		}

		// Calculate next delay with exponential backoff
		delay = r.calculateBackoff(delay)
	}

	// This should never be reached due to the logic above
	return fmt.Errorf("%s failed after %d attempts", operationName, r.config.MaxAttempts)
}

// shouldRetry determines if an error is retryable
func (r *RetryableServer) shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	for _, retryableErr := range r.config.RetryableErrors {
		if containsIgnoreCase(errStr, retryableErr) {
			return true
		}
	}

	return false
}

// calculateBackoff calculates the next delay with exponential backoff
func (r *RetryableServer) calculateBackoff(currentDelay time.Duration) time.Duration {
	nextDelay := time.Duration(float64(currentDelay) * r.config.BackoffMultiplier)

	// Add jitter (±10%) to avoid thundering herd
	jitter := time.Duration(float64(nextDelay) * 0.1 * (2*randomFloat() - 1))
	nextDelay += jitter

	// Cap at max delay
	if nextDelay > r.config.MaxDelay {
		nextDelay = r.config.MaxDelay
	}

	return nextDelay
}

// containsIgnoreCase checks if a string contains a substring (case-insensitive)
func containsIgnoreCase(str, substr string) bool {
	return len(str) >= len(substr) &&
		   strings.Contains(strings.ToLower(str), strings.ToLower(substr))
}

// randomFloat returns a random float between 0 and 1
func randomFloat() float64 {
	// Simple pseudo-random using current time
	// For production, consider using math/rand with proper seeding
	return float64(time.Now().UnixNano()%1000) / 1000.0
}

// RetryWithExponentialBackoff is a utility function for retrying any operation
func RetryWithExponentialBackoff(
	ctx context.Context,
	operation func() error,
	config *RetryConfig,
) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	logger := logging.New()
	delay := config.InitialDelay

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		if attempt >= config.MaxAttempts {
			return fmt.Errorf("operation failed after %d attempts: %w", config.MaxAttempts, err)
		}

		logger.Debug(ctx, "Retrying operation", map[string]interface{}{
			"attempt":      attempt,
			"max_attempts": config.MaxAttempts,
			"delay_ms":     delay.Milliseconds(),
			"error":        err.Error(),
		})

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}

		// Exponential backoff with jitter
		delay = time.Duration(math.Min(
			float64(delay)*config.BackoffMultiplier,
			float64(config.MaxDelay),
		))
	}

	return fmt.Errorf("max retry attempts reached")
}