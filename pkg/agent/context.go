package agent

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

// ContextKey is a type for context keys to avoid collisions
type ContextKey string

const (
	// SubAgentNameKey is the context key for sub-agent name
	SubAgentNameKey ContextKey = "sub_agent_name"

	// ParentAgentKey is the context key for parent agent
	ParentAgentKey ContextKey = "parent_agent"

	// RecursionDepthKey is the context key for recursion depth
	RecursionDepthKey ContextKey = "recursion_depth"

	// InvocationIDKey is the context key for invocation ID
	InvocationIDKey ContextKey = "invocation_id"

	// MaxRecursionDepth is the maximum allowed recursion depth
	MaxRecursionDepth = 5

	// DefaultSubAgentTimeout is the default timeout for sub-agent calls
	DefaultSubAgentTimeout = 30 * time.Second
)

// SubAgentContext contains context information for sub-agent invocations
type SubAgentContext struct {
	ParentAgent    string
	SubAgentName   string
	RecursionDepth int
	InvocationID   string
	StartTime      time.Time
}

// WithSubAgentContext adds sub-agent context to the context
func WithSubAgentContext(ctx context.Context, parentAgent, subAgentName string) context.Context {
	// Get current recursion depth
	depth := GetRecursionDepth(ctx)

	// Create sub-agent context
	subCtx := SubAgentContext{
		ParentAgent:    parentAgent,
		SubAgentName:   subAgentName,
		RecursionDepth: depth + 1,
		InvocationID:   generateInvocationID(),
		StartTime:      time.Now(),
	}

	// Add to context
	ctx = context.WithValue(ctx, SubAgentNameKey, subAgentName)
	ctx = context.WithValue(ctx, ParentAgentKey, parentAgent)
	ctx = context.WithValue(ctx, RecursionDepthKey, depth+1)
	ctx = context.WithValue(ctx, InvocationIDKey, subCtx.InvocationID)

	return ctx
}

// GetRecursionDepth retrieves the current recursion depth from context
func GetRecursionDepth(ctx context.Context) int {
	if depth, ok := ctx.Value(RecursionDepthKey).(int); ok {
		return depth
	}
	return 0
}

// GetSubAgentName retrieves the sub-agent name from context
func GetSubAgentName(ctx context.Context) string {
	if name, ok := ctx.Value(SubAgentNameKey).(string); ok {
		return name
	}
	return ""
}

// GetParentAgent retrieves the parent agent from context
func GetParentAgent(ctx context.Context) string {
	if parent, ok := ctx.Value(ParentAgentKey).(string); ok {
		return parent
	}
	return ""
}

// GetInvocationID retrieves the invocation ID from context
func GetInvocationID(ctx context.Context) string {
	if id, ok := ctx.Value(InvocationIDKey).(string); ok {
		return id
	}
	return ""
}

// IsSubAgentCall checks if the current context is a sub-agent call
func IsSubAgentCall(ctx context.Context) bool {
	return GetRecursionDepth(ctx) > 0
}

// ValidateRecursionDepth checks if the recursion depth is within limits
func ValidateRecursionDepth(ctx context.Context) error {
	depth := GetRecursionDepth(ctx)
	if depth > MaxRecursionDepth {
		return fmt.Errorf("maximum recursion depth %d exceeded (current: %d)", MaxRecursionDepth, depth)
	}
	return nil
}

// generateInvocationID generates a unique invocation ID
func generateInvocationID() string {
	// Use crypto/rand for secure random number generation
	randomNum, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		// Fallback to timestamp-only if crypto/rand fails
		return fmt.Sprintf("inv_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("inv_%d_%s", time.Now().UnixNano(), randomNum.String())
}

// WithTimeout adds a timeout to the context for sub-agent calls
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}
