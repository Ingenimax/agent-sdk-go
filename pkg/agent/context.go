package agent

import (
	"context"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
)

// Sub-agent context handling lives in pkg/tools, which owns the only recursion
// guard that actually runs (AgentTool.Execute consults it before invoking a
// sub-agent). Everything here delegates there.
//
// This package used to declare its own `type ContextKey string` with the same
// string values as the ones in pkg/tools. Go context keys compare by type AND
// value, so agent.ContextKey("recursion_depth") and
// tools.contextKey("recursion_depth") were two entirely separate keys backing
// two independent counters that could never observe one another. The copy here
// was reached only by a unit test and by examples/subagents/depth_validation,
// which meant the example demonstrated a recursion guard that did not protect
// production. Delegating leaves exactly one counter.
//
// pkg/agent already imports pkg/tools and the dependency cannot run the other
// way, so pkg/tools is the correct owner.

const (
	// MaxRecursionDepth is the maximum allowed sub-agent recursion depth.
	MaxRecursionDepth = tools.MaxRecursionDepth

	// DefaultSubAgentTimeout is the default timeout for sub-agent calls.
	DefaultSubAgentTimeout = 30 * time.Second
)

// SubAgentContext contains context information for sub-agent invocations.
type SubAgentContext struct {
	ParentAgent    string
	SubAgentName   string
	RecursionDepth int
	InvocationID   string
	StartTime      time.Time
}

// WithSubAgentContext adds sub-agent context to the context, incrementing the
// recursion depth that AgentTool.Execute enforces.
func WithSubAgentContext(ctx context.Context, parentAgent, subAgentName string) context.Context {
	return tools.WithSubAgentContext(ctx, parentAgent, subAgentName)
}

// GetRecursionDepth retrieves the current recursion depth from context.
func GetRecursionDepth(ctx context.Context) int {
	return tools.GetRecursionDepth(ctx)
}

// GetSubAgentName retrieves the sub-agent name from context.
func GetSubAgentName(ctx context.Context) string {
	return tools.GetSubAgentName(ctx)
}

// GetParentAgent retrieves the parent agent from context.
func GetParentAgent(ctx context.Context) string {
	return tools.GetParentAgent(ctx)
}

// GetInvocationID retrieves the invocation ID from context.
func GetInvocationID(ctx context.Context) string {
	return tools.GetInvocationID(ctx)
}

// IsSubAgentCall checks if the current context is a sub-agent call.
func IsSubAgentCall(ctx context.Context) bool {
	return tools.IsSubAgentCall(ctx)
}

// ValidateRecursionDepth checks if the recursion depth is within limits.
func ValidateRecursionDepth(ctx context.Context) error {
	return tools.ValidateRecursionDepth(ctx)
}

// WithTimeout adds a timeout to the context for sub-agent calls.
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout) // #nosec G118 - cancel func is returned to caller
}
