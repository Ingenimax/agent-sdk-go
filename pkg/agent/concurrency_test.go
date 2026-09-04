package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
)

// concurrentStubLLM is a minimal interfaces.LLM that is safe to call from many
// goroutines at once, so any race the detector reports comes from the Agent
// rather than from the test double.
type concurrentStubLLM struct{}

func (*concurrentStubLLM) Name() string            { return "concurrent-stub" }
func (*concurrentStubLLM) SupportsStreaming() bool { return false }

func (*concurrentStubLLM) Generate(context.Context, string, ...interfaces.GenerateOption) (string, error) {
	return "ok", nil
}

func (*concurrentStubLLM) GenerateWithTools(context.Context, string, []interfaces.Tool, ...interfaces.GenerateOption) (string, error) {
	return "ok", nil
}

func (*concurrentStubLLM) GenerateDetailed(context.Context, string, ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return &interfaces.LLMResponse{Content: "ok", Model: "concurrent-stub"}, nil
}

func (*concurrentStubLLM) GenerateWithToolsDetailed(context.Context, string, []interfaces.Tool, ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return &interfaces.LLMResponse{Content: "ok", Model: "concurrent-stub"}, nil
}

// concurrentStubTool exists only so that len(allTools) > 0, which together with
// the default requirePlanApproval=true selects the execution-plan path.
type concurrentStubTool struct{ name string }

func (t *concurrentStubTool) Name() string        { return t.name }
func (t *concurrentStubTool) Description() string { return "stub tool for concurrency tests" }

func (t *concurrentStubTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"query": {Type: "string", Description: "anything", Required: false},
	}
}

func (t *concurrentStubTool) Run(context.Context, string) (string, error) { return "stub", nil }

func (t *concurrentStubTool) Execute(context.Context, string) (string, error) { return "stub", nil }

// TestAgentRunIsSafeForConcurrentUse guards against reintroducing mutation of
// shared Agent state from inside the run path.
//
// runLocalWithTracking used to assign a.planGenerator on every run:
//
//	if (len(allTools) > 0) && a.requirePlanApproval {
//	    a.planGenerator = executionplan.NewGenerator(...)   // unsynchronised write
//	    return a.runWithExecutionPlan(ctx, input)
//	}
//
// requirePlanApproval defaults to true (WithRequirePlanApproval is opt-out), so
// this was the default configuration, not an edge case. Two concurrent Run
// calls raced on an interface field with no lock, and because sub-agents are
// shared *Agent pointers, a single parent fanning out to two sub-agent tool
// calls hit it without the caller ever writing concurrent code.
//
// The generator is now built per run and passed down, leaving a.planGenerator
// written once at construction and read-only thereafter. This test must be run
// with -race to be meaningful; CI does.
func TestAgentRunIsSafeForConcurrentUse(t *testing.T) {
	agent, err := NewAgent(
		WithLLM(&concurrentStubLLM{}),
		WithName("concurrency-agent"),
		WithTools(&concurrentStubTool{name: "stub_tool"}),
		// requirePlanApproval is deliberately left at its default (true) so the
		// execution-plan path -- the one that used to race -- is exercised.
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if !agent.requirePlanApproval {
		t.Fatal("expected requirePlanApproval to default to true; " +
			"this test only covers the racing path when it does")
	}

	const goroutines = 16

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			// The result is deliberately ignored: plan generation against a stub
			// LLM may or may not succeed, and the property under test is the
			// absence of a data race on shared Agent state, not the outcome.
			_, _ = agent.Run(context.Background(), fmt.Sprintf("request %d", n))
		}(i)
	}

	wg.Wait()
}

// TestAgentPlanGeneratorNotMutatedByRun asserts the specific invariant behind
// the fix: the construction-time generator is still the one on the Agent after
// runs have executed. If a future change reintroduces per-run assignment, this
// fails deterministically rather than depending on the race detector catching
// an interleaving.
func TestAgentPlanGeneratorNotMutatedByRun(t *testing.T) {
	agent, err := NewAgent(
		WithLLM(&concurrentStubLLM{}),
		WithName("plan-generator-agent"),
		WithTools(&concurrentStubTool{name: "stub_tool"}),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	before := agent.planGenerator
	if before == nil {
		t.Fatal("expected planGenerator to be set at construction")
	}

	for i := 0; i < 4; i++ {
		_, _ = agent.Run(context.Background(), fmt.Sprintf("request %d", i))
	}

	if agent.planGenerator != before {
		t.Error("Agent.planGenerator was reassigned by Run; it must be written " +
			"once at construction and read-only thereafter, or concurrent runs race on it")
	}
}

// TestRecursionDepthIsASingleCounter guards against reintroducing a second set
// of sub-agent context keys.
//
// pkg/agent used to declare `type ContextKey string` with the same string
// values as the unexported keys in pkg/tools. Go context keys compare by type
// AND value, so those were two distinct key sets backing two independent
// counters. Depth recorded through pkg/agent was invisible to the guard in
// pkg/tools that actually runs before a sub-agent is invoked, so the recursion
// limit could be exceeded without ever tripping.
func TestRecursionDepthIsASingleCounter(t *testing.T) {
	ctx := context.Background()

	if got := GetRecursionDepth(ctx); got != 0 {
		t.Fatalf("fresh context depth = %d, want 0", got)
	}

	// Record depth through the pkg/agent API...
	ctx = WithSubAgentContext(ctx, "parent", "child")

	// ...and observe it through the pkg/tools API, which owns the live guard.
	if got := tools.GetRecursionDepth(ctx); got != 1 {
		t.Errorf("depth recorded via agent.WithSubAgentContext is %d when read through "+
			"tools.GetRecursionDepth, want 1: the two packages are using different context keys, "+
			"so the recursion guard cannot see depth recorded by the agent package", got)
	}

	if !IsSubAgentCall(ctx) {
		t.Error("IsSubAgentCall should be true after WithSubAgentContext")
	}

	// And the reverse direction, so neither package can drift alone.
	ctx = tools.WithSubAgentContext(ctx, "child", "grandchild")
	if got := GetRecursionDepth(ctx); got != 2 {
		t.Errorf("depth recorded via tools.WithSubAgentContext is %d when read through "+
			"agent.GetRecursionDepth, want 2", got)
	}
}

// TestValidateRecursionDepthUsesTheSharedLimit asserts the depth check trips
// against the same counter the guard increments.
func TestValidateRecursionDepthUsesTheSharedLimit(t *testing.T) {
	ctx := context.Background()
	for i := 0; i <= MaxRecursionDepth; i++ {
		ctx = tools.WithSubAgentContext(ctx, "parent", "child")
	}

	if err := ValidateRecursionDepth(ctx); err == nil {
		t.Errorf("ValidateRecursionDepth returned nil at depth %d, want an error above %d",
			GetRecursionDepth(ctx), MaxRecursionDepth)
	}
}
