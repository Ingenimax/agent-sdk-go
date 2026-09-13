// Package hooks runs user-supplied callbacks at points in an agent's lifecycle,
// and bundles related callbacks into named plugins.
//
// A Hook observes or intervenes at one point. A Plugin is a named bundle of
// hooks shipped together — logging, policy enforcement, metrics, redaction —
// which is what "packaged cross-cutting behavior" means in practice. ADK draws
// the same distinction between its per-agent callbacks and its Runner-level
// plugins.
//
// # Where hooks run
//
//	BeforeTool   before a tool executes; can rewrite arguments or deny the call
//	AfterTool    after a tool returns; can rewrite the result
//	OnToolError  when a tool fails; can substitute a result and suppress the error
//
// Tool hooks reach every LLM provider, because they attach to the tool set
// rather than to any provider's loop. See docs/tool-pipeline.md.
//
// # Denial
//
// A denied tool call returns a message to the model rather than an error. The
// providers convert a tool error into a tool-result message and keep going, so
// returning an error would not stop anything -- it would just look like a tool
// that failed. A denial the model can read is more useful and more honest.
package hooks

import (
	"context"
	"fmt"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// ToolCall describes a tool invocation a hook may inspect or alter.
type ToolCall struct {
	// Tool is the name of the tool being called.
	Tool string

	// Arguments is the raw JSON the model supplied.
	Arguments string

	// AgentName is the agent making the call, when known.
	AgentName string
}

// Decision is what a BeforeTool hook wants to happen.
type Decision int

const (
	// Allow lets the call proceed. This is the zero value, so a hook that
	// returns an empty Outcome allows by default rather than silently denying.
	Allow Decision = iota

	// Modify proceeds with rewritten arguments.
	Modify

	// Deny blocks the call and returns Message to the model instead.
	Deny
)

// Outcome is a BeforeTool hook's verdict.
//
// The zero value allows the call, which is deliberate: a hook that returns
// early, or one written before a new field existed, must not accidentally block
// every tool.
type Outcome struct {
	Decision Decision

	// Arguments replaces the call's arguments when Decision is Modify.
	Arguments string

	// Message is returned to the model when Decision is Deny.
	Message string
}

// BeforeToolFunc runs before a tool executes.
type BeforeToolFunc func(ctx context.Context, call ToolCall) (Outcome, error)

// AfterToolFunc runs after a tool returns and may rewrite the result.
type AfterToolFunc func(ctx context.Context, call ToolCall, result string) (string, error)

// OnToolErrorFunc runs when a tool fails. Returning handled=true substitutes
// the returned result and suppresses the error.
type OnToolErrorFunc func(ctx context.Context, call ToolCall, err error) (result string, handled bool)

// Plugin is a named bundle of hooks.
//
// Name is required: it appears in panic recovery and error messages, and
// "which plugin denied this?" is unanswerable without it.
type Plugin struct {
	Name        string
	BeforeTool  BeforeToolFunc
	AfterTool   AfterToolFunc
	OnToolError OnToolErrorFunc
}

// Registry holds the plugins an agent runs.
type Registry struct {
	mu      sync.RWMutex
	plugins []Plugin
}

// NewRegistry creates an empty registry.
func NewRegistry(plugins ...Plugin) *Registry {
	r := &Registry{}
	for _, p := range plugins {
		r.Register(p)
	}
	return r
}

// Register adds a plugin. Plugins run in registration order.
func (r *Registry) Register(p Plugin) {
	if p.Name == "" {
		p.Name = "unnamed"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = append(r.plugins, p)
}

// Names returns the registered plugin names, in order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.plugins))
	for _, p := range r.plugins {
		names = append(names, p.Name)
	}
	return names
}

func (r *Registry) snapshot() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Plugin(nil), r.plugins...)
}

// beforeTool runs every BeforeTool hook in order.
//
// The first plugin to deny wins and the rest are skipped: once a call is
// blocked, running further hooks against a call that will not happen is at best
// wasted work and at worst misleading in an audit log. Modifications accumulate,
// so a later hook sees the arguments an earlier one produced.
func (r *Registry) beforeTool(ctx context.Context, call ToolCall) (Outcome, error) {
	current := call

	for _, p := range r.snapshot() {
		if p.BeforeTool == nil {
			continue
		}

		outcome, err := r.runBefore(ctx, p, current)
		if err != nil {
			return Outcome{}, fmt.Errorf("plugin %q: %w", p.Name, err)
		}

		switch outcome.Decision {
		case Deny:
			message := outcome.Message
			if message == "" {
				message = fmt.Sprintf("This tool call was denied by %q.", p.Name)
			}
			return Outcome{Decision: Deny, Message: message}, nil
		case Modify:
			current.Arguments = outcome.Arguments
		}
	}

	if current.Arguments != call.Arguments {
		return Outcome{Decision: Modify, Arguments: current.Arguments}, nil
	}
	return Outcome{Decision: Allow}, nil
}

// runBefore isolates one hook so a panic inside it cannot take down the run.
//
// A logging plugin that dereferences a nil map should produce a failed tool
// call, not a dead agent with no explanation.
func (r *Registry) runBefore(ctx context.Context, p Plugin, call ToolCall) (outcome Outcome, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			outcome = Outcome{}
			err = fmt.Errorf("panic in BeforeTool: %v", recovered)
		}
	}()
	return p.BeforeTool(ctx, call)
}

// afterTool runs every AfterTool hook in order, threading the result through.
func (r *Registry) afterTool(ctx context.Context, call ToolCall, result string) string {
	current := result

	for _, p := range r.snapshot() {
		if p.AfterTool == nil {
			continue
		}
		next, err := r.runAfter(ctx, p, call, current)
		if err != nil {
			// An AfterTool hook is observational or cosmetic. Failing the tool
			// call because a metrics hook panicked would be a worse outcome
			// than carrying on with the unmodified result.
			continue
		}
		current = next
	}

	return current
}

func (r *Registry) runAfter(ctx context.Context, p Plugin, call ToolCall, result string) (out string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			out = result
			err = fmt.Errorf("panic in AfterTool: %v", recovered)
		}
	}()
	return p.AfterTool(ctx, call, result)
}

// onToolError offers each plugin the chance to handle a tool failure.
func (r *Registry) onToolError(ctx context.Context, call ToolCall, toolErr error) (string, bool) {
	for _, p := range r.snapshot() {
		if p.OnToolError == nil {
			continue
		}
		result, handled := r.runOnError(ctx, p, call, toolErr)
		if handled {
			return result, true
		}
	}
	return "", false
}

func (r *Registry) runOnError(ctx context.Context, p Plugin, call ToolCall, toolErr error) (result string, handled bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = ""
			handled = false
		}
	}()
	return p.OnToolError(ctx, call, toolErr)
}

// Decorate returns a tool decorator that applies this registry's hooks.
//
// Pass it to agent.WithToolDecorator. Hooks then reach every provider, because
// the decorator attaches to the tool set rather than to any provider's loop.
func (r *Registry) Decorate(agentName string) func([]interfaces.Tool) []interfaces.Tool {
	return func(toolSet []interfaces.Tool) []interfaces.Tool {
		if r == nil || len(toolSet) == 0 {
			return toolSet
		}
		out := make([]interfaces.Tool, len(toolSet))
		for i, tool := range toolSet {
			out[i] = &hookedTool{inner: tool, registry: r, agentName: agentName}
		}
		return out
	}
}

// hookedTool applies the registry's hooks around one tool.
type hookedTool struct {
	inner     interfaces.Tool
	registry  *Registry
	agentName string
}

func (t *hookedTool) Name() string        { return t.inner.Name() }
func (t *hookedTool) Description() string { return t.inner.Description() }

func (t *hookedTool) Parameters() map[string]interfaces.ParameterSpec {
	return t.inner.Parameters()
}

func (t *hookedTool) Run(ctx context.Context, input string) (string, error) {
	return t.execute(ctx, input, t.inner.Run)
}

func (t *hookedTool) Execute(ctx context.Context, args string) (string, error) {
	return t.execute(ctx, args, t.inner.Execute)
}

func (t *hookedTool) execute(ctx context.Context, args string, invoke func(context.Context, string) (string, error)) (string, error) {
	call := ToolCall{Tool: t.inner.Name(), Arguments: args, AgentName: t.agentName}

	outcome, err := t.registry.beforeTool(ctx, call)
	if err != nil {
		return "", err
	}

	switch outcome.Decision {
	case Deny:
		// Returned as a result, not an error: the providers turn a tool error
		// into a tool-result message and continue anyway, so an error would
		// merely look like a broken tool. A readable denial lets the model
		// adapt.
		return outcome.Message, nil
	case Modify:
		call.Arguments = outcome.Arguments
	}

	result, execErr := invoke(ctx, call.Arguments)
	if execErr != nil {
		if handled, ok := t.registry.onToolError(ctx, call, execErr); ok {
			return t.registry.afterTool(ctx, call, handled), nil
		}
		return "", execErr
	}

	return t.registry.afterTool(ctx, call, result), nil
}

// Unwrap returns the tool underneath, satisfying agent.ToolUnwrapper so
// wrapping does not permanently erase concrete type identity.
func (t *hookedTool) Unwrap() interfaces.Tool { return t.inner }

// DisplayName forwards the inner tool's display name.
func (t *hookedTool) DisplayName() string {
	if d, ok := t.inner.(interfaces.ToolWithDisplayName); ok {
		return d.DisplayName()
	}
	return t.inner.Name()
}

// Internal forwards the inner tool's internal flag.
func (t *hookedTool) Internal() bool {
	if i, ok := t.inner.(interfaces.InternalTool); ok {
		return i.Internal()
	}
	return false
}

var (
	_ interfaces.Tool                = (*hookedTool)(nil)
	_ interfaces.ToolWithDisplayName = (*hookedTool)(nil)
	_ interfaces.InternalTool        = (*hookedTool)(nil)
)
