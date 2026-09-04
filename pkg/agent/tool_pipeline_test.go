package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// recordingDecorator wraps every tool and records the order in which decorators
// were applied, so tests can assert on composition rather than behaviour.
type recordingDecorator struct {
	label string
	log   *[]string
	mu    *sync.Mutex
}

func (d recordingDecorator) decorate(toolSet []interfaces.Tool) []interfaces.Tool {
	d.mu.Lock()
	*d.log = append(*d.log, d.label)
	d.mu.Unlock()

	out := make([]interfaces.Tool, len(toolSet))
	for i, t := range toolSet {
		out[i] = &labelledTool{inner: t, label: d.label}
	}
	return out
}

// labelledTool is a decorator that forwards optional interfaces via the shared
// helper and supports unwrapping.
type labelledTool struct {
	inner interfaces.Tool
	label string
}

func (t *labelledTool) Name() string        { return t.inner.Name() }
func (t *labelledTool) Description() string { return t.inner.Description() }

func (t *labelledTool) Parameters() map[string]interfaces.ParameterSpec {
	return t.inner.Parameters()
}

func (t *labelledTool) Run(ctx context.Context, input string) (string, error) {
	return t.inner.Run(ctx, input)
}

func (t *labelledTool) Execute(ctx context.Context, args string) (string, error) {
	return t.inner.Execute(ctx, args)
}

func (t *labelledTool) DisplayName() string {
	name, _ := ForwardOptionalToolInterfaces(t.inner)
	return name
}

func (t *labelledTool) Internal() bool {
	_, internal := ForwardOptionalToolInterfaces(t.inner)
	return internal
}

func (t *labelledTool) Unwrap() interfaces.Tool { return t.inner }

// namedTool is a leaf tool with an optional DisplayName, used to check that
// optional interfaces survive decoration.
type namedTool struct {
	name        string
	displayName string
	internal    bool
}

func (t *namedTool) Name() string        { return t.name }
func (t *namedTool) Description() string { return "leaf tool" }

func (t *namedTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{}
}

func (t *namedTool) Run(context.Context, string) (string, error)     { return "ran", nil }
func (t *namedTool) Execute(context.Context, string) (string, error) { return "ran", nil }
func (t *namedTool) DisplayName() string                             { return t.displayName }
func (t *namedTool) Internal() bool                                  { return t.internal }

// TestDecorateToolsAppliesPipelineInOrder asserts decorators run in the order
// they were registered, with the usage tracker innermost.
//
// The ordering is not cosmetic. The tool loop lives inside each provider, so
// this decorator slice is the only interposition point that reaches a tool call
// without editing all eight providers -- which is why several planned features
// all target these few lines. The tracker must be innermost so that a decorator
// which denies a call prevents it from being recorded as executed.
func TestDecorateToolsAppliesPipelineInOrder(t *testing.T) {
	var (
		mu  sync.Mutex
		log []string
	)

	first := recordingDecorator{label: "first", log: &log, mu: &mu}
	second := recordingDecorator{label: "second", log: &log, mu: &mu}

	agent, err := NewAgent(
		WithLLM(&concurrentStubLLM{}),
		WithName("pipeline-agent"),
		WithTools(&namedTool{name: "leaf", displayName: "Leaf"}),
		WithToolDecorator(first.decorate),
		WithToolDecorator(second.decorate),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	// NewAgent decorates once itself, to build the execution-plan executor.
	// Reset so this assertion covers exactly one decorateTools call.
	mu.Lock()
	log = nil
	mu.Unlock()

	decorated := agent.decorateTools([]interfaces.Tool{&namedTool{name: "leaf"}}, newUsageTracker(true))

	mu.Lock()
	got := append([]string(nil), log...)
	mu.Unlock()

	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Errorf("decorator order = %v, want [first second]", got)
	}

	if len(decorated) != 1 {
		t.Fatalf("decorateTools returned %d tools, want 1", len(decorated))
	}

	// Outermost decorator should be the last one registered.
	outer, ok := decorated[0].(*labelledTool)
	if !ok {
		t.Fatalf("outermost tool is %T, want *labelledTool", decorated[0])
	}
	if outer.label != "second" {
		t.Errorf("outermost decorator = %q, want %q", outer.label, "second")
	}
}

// TestDecorateToolsKeepsTrackerInnermost asserts the usage tracker is applied
// beneath the pipeline, so it sees a call only after every outer decorator has
// allowed it through.
func TestDecorateToolsKeepsTrackerInnermost(t *testing.T) {
	tracker := newUsageTracker(true)

	agent, err := NewAgent(
		WithLLM(&concurrentStubLLM{}),
		WithName("tracker-order-agent"),
		WithTools(&namedTool{name: "leaf"}),
		WithToolDecorator(func(toolSet []interfaces.Tool) []interfaces.Tool {
			out := make([]interfaces.Tool, len(toolSet))
			for i, tl := range toolSet {
				out[i] = &labelledTool{inner: tl, label: "outer"}
			}
			return out
		}),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	decorated := agent.decorateTools([]interfaces.Tool{&namedTool{name: "leaf"}}, tracker)

	// Unwrap one layer: beneath the pipeline decorator must sit the tracker.
	outer, ok := decorated[0].(*labelledTool)
	if !ok {
		t.Fatalf("outermost tool is %T, want *labelledTool", decorated[0])
	}
	if _, ok := outer.Unwrap().(*trackingTool); !ok {
		t.Errorf("beneath the pipeline decorator sits %T, want *trackingTool: "+
			"the tracker must be innermost so a denied call is not recorded as executed",
			outer.Unwrap())
	}
}

// TestUnwrapToolRecoversConcreteType asserts wrapping does not permanently
// erase the identity of the underlying tool. configureSubAgentTools relies on
// concrete type assertions and only works today because it runs before any
// decoration.
func TestUnwrapToolRecoversConcreteType(t *testing.T) {
	leaf := &namedTool{name: "leaf", displayName: "Leaf", internal: true}

	agent, err := NewAgent(
		WithLLM(&concurrentStubLLM{}),
		WithName("unwrap-agent"),
		WithTools(leaf),
		WithToolDecorator(func(toolSet []interfaces.Tool) []interfaces.Tool {
			out := make([]interfaces.Tool, len(toolSet))
			for i, tl := range toolSet {
				out[i] = &labelledTool{inner: tl, label: "outer"}
			}
			return out
		}),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	decorated := agent.decorateTools([]interfaces.Tool{leaf}, newUsageTracker(true))

	if _, ok := decorated[0].(*namedTool); ok {
		t.Fatal("test is not exercising decoration: the tool was not wrapped")
	}

	recovered := UnwrapTool(decorated[0])
	if recovered != interfaces.Tool(leaf) {
		t.Errorf("UnwrapTool() = %T, want the original *namedTool", recovered)
	}
}

// TestDecoratedToolsForwardOptionalInterfaces asserts DisplayName and Internal
// survive the full decorator stack. Each decorator used to hand-roll this
// forwarding, so any one of them forgetting silently changed how a tool is
// presented.
func TestDecoratedToolsForwardOptionalInterfaces(t *testing.T) {
	leaf := &namedTool{name: "leaf", displayName: "Human Readable", internal: true}

	agent, err := NewAgent(
		WithLLM(&concurrentStubLLM{}),
		WithName("forwarding-agent"),
		WithTools(leaf),
		WithToolDecorator(func(toolSet []interfaces.Tool) []interfaces.Tool {
			out := make([]interfaces.Tool, len(toolSet))
			for i, tl := range toolSet {
				out[i] = &labelledTool{inner: tl, label: "outer"}
			}
			return out
		}),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	decorated := agent.decorateTools([]interfaces.Tool{leaf}, newUsageTracker(true))

	named, ok := decorated[0].(interfaces.ToolWithDisplayName)
	if !ok {
		t.Fatal("decorated tool no longer implements ToolWithDisplayName")
	}
	if got := named.DisplayName(); got != "Human Readable" {
		t.Errorf("DisplayName() = %q, want %q", got, "Human Readable")
	}

	internal, ok := decorated[0].(interfaces.InternalTool)
	if !ok {
		t.Fatal("decorated tool no longer implements InternalTool")
	}
	if !internal.Internal() {
		t.Error("Internal() = false, want true: the flag was lost through decoration")
	}
}

// TestExecutionPlanExecutorSeesDecorators asserts the execution-plan path is
// decorated too.
//
// executionplan.NewExecutor captures the tool slice at construction and calls
// tool.Execute directly, bypassing the provider tool loop entirely. Since
// requirePlanApproval defaults to true, that is the SDK's default path, and it
// previously received raw undecorated tools -- so anything built on this seam
// would have silently not applied to most runs.
func TestExecutionPlanExecutorSeesDecorators(t *testing.T) {
	var decorated bool
	var mu sync.Mutex

	_, err := NewAgent(
		WithLLM(&concurrentStubLLM{}),
		WithName("plan-executor-agent"),
		WithTools(&namedTool{name: "leaf"}),
		WithToolDecorator(func(toolSet []interfaces.Tool) []interfaces.Tool {
			mu.Lock()
			decorated = true
			mu.Unlock()
			return toolSet
		}),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !decorated {
		t.Error("the execution-plan executor was built from undecorated tools; " +
			"since requirePlanApproval defaults to true this is the default path")
	}
}
