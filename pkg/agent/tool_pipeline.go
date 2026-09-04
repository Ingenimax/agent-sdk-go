package agent

import (
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// ToolDecorator transforms the tool set an agent is about to expose, typically
// by wrapping each tool to add behaviour around Execute and Run.
//
// A decorator must return a slice of the same length, preserving order, and
// should forward optional interfaces on the tools it wraps -- see
// ForwardOptionalToolInterfaces.
type ToolDecorator func([]interfaces.Tool) []interfaces.Tool

// ToolUnwrapper is implemented by tool decorators so callers can recover the
// tool underneath.
//
// Wrapping erases concrete type identity: after a tool is decorated, an
// assertion such as tool.(*tools.AgentTool) fails. configureSubAgentTools only
// works today because it inspects a.tools at construction, before any wrapping
// happens.
type ToolUnwrapper interface {
	Unwrap() interfaces.Tool
}

// UnwrapTool walks a chain of ToolUnwrapper decorators and returns the
// innermost tool.
func UnwrapTool(t interfaces.Tool) interfaces.Tool {
	for {
		unwrapper, ok := t.(ToolUnwrapper)
		if !ok {
			return t
		}
		inner := unwrapper.Unwrap()
		if inner == nil {
			return t
		}
		t = inner
	}
}

// decorateTools applies the agent's tool pipeline in order.
//
// This is the single composition point for per-tool interposition. The tool
// loop lives inside each provider, not in pkg/agent, so a []interfaces.Tool
// decorator is the only way to interpose on a tool call without editing every
// provider -- which makes these few lines the most contested seam in the
// codebase. Several planned features (usage tracking, context compaction,
// hooks, policy) all want it.
//
// Order is load-bearing and is fixed here rather than left to whoever composes
// last:
//
//	innermost -> outermost:  tracker, then any pipeline decorators in order
//
// The tracker sits innermost so it records executions that actually happen: a
// decorator that denies or short-circuits a call must do so before the tracker
// sees it, or usage accounting reports work that never ran.
func (a *Agent) decorateTools(toolSet []interfaces.Tool, tracker *usageTracker) []interfaces.Tool {
	decorated := wrapToolsWithTracker(toolSet, tracker)

	for _, decorate := range a.toolPipeline {
		if decorate == nil {
			continue
		}
		if next := decorate(decorated); next != nil {
			decorated = next
		}
	}

	return decorated
}

// ForwardOptionalToolInterfaces reports the optional-interface values a
// decorator should expose on behalf of the tool it wraps.
//
// interfaces.Tool is four methods, but tools may additionally implement
// ToolWithDisplayName and InternalTool, and those are discovered by type
// assertion. A decorator that does not forward them silently changes how the
// tool is presented. Each decorator previously hand-rolled this; doing it once
// means a newly added optional interface is handled in one place.
func ForwardOptionalToolInterfaces(inner interfaces.Tool) (displayName string, internal bool) {
	displayName = inner.Name()
	if d, ok := inner.(interfaces.ToolWithDisplayName); ok {
		displayName = d.DisplayName()
	}
	if i, ok := inner.(interfaces.InternalTool); ok {
		internal = i.Internal()
	}
	return displayName, internal
}

// WithToolDecorator appends a decorator to the agent's tool pipeline.
//
// Decorators run outside the usage tracker and in the order they are added, so
// a decorator that can deny or short-circuit a call should be added after any
// that only observe. See decorateTools for the full ordering rationale.
func WithToolDecorator(decorate ToolDecorator) Option {
	return func(a *Agent) {
		if decorate != nil {
			a.toolPipeline = append(a.toolPipeline, decorate)
		}
	}
}
