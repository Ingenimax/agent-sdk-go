package orchestration

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// recordingAgent returns an agent that records the prompt it received.
func recordingAgent(t *testing.T, name, reply string) (*agent.Agent, *testutil.FakeLLM) {
	t.Helper()
	llm := &testutil.FakeLLM{DefaultResponse: reply}
	a, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithName(name),
		agent.WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("NewAgent(%s) error = %v", name, err)
	}
	return a, llm
}

func TestSequentialRunsInOrderAndChainsResults(t *testing.T) {
	a1, llm1 := recordingAgent(t, "one", "first output")
	a2, llm2 := recordingAgent(t, "two", "second output")
	a3, llm3 := recordingAgent(t, "three", "third output")

	reg := registryWith(t, map[string]*agent.Agent{"a1": a1, "a2": a2, "a3": a3})

	result, err := Sequential(reg, "a1", "a2", "a3").Run(context.Background(), "start here")
	if err != nil {
		t.Fatalf("Sequential.Run() error = %v", err)
	}
	if result != "third output" {
		t.Errorf("result = %q, want the last agent's output", result)
	}

	// Each agent runs exactly once.
	for i, llm := range []*testutil.FakeLLM{llm1, llm2, llm3} {
		if got := llm.Calls(); got != 1 {
			t.Errorf("agent %d ran %d times, want 1", i+1, got)
		}
	}

	// The chain is real: each step sees the previous step's result.
	if prompts := llm2.Prompts(); len(prompts) == 0 || !strings.Contains(prompts[0], "first output") {
		t.Errorf("second agent did not receive the first agent's result; got %v", prompts)
	}
	if prompts := llm3.Prompts(); len(prompts) == 0 || !strings.Contains(prompts[0], "second output") {
		t.Errorf("third agent did not receive the second agent's result; got %v", prompts)
	}
}

func TestParallelFansOutAndCombines(t *testing.T) {
	var peak atomic.Int64
	var inFlight atomic.Int64
	release := make(chan struct{})

	reg := NewAgentRegistry()
	for i := 0; i < 3; i++ {
		llm := &testutil.FakeLLM{
			GenerateFunc: func(ctx context.Context, _ string, _ []interfaces.Tool) (string, error) {
				now := inFlight.Add(1)
				for {
					was := peak.Load()
					if now <= was || peak.CompareAndSwap(was, now) {
						break
					}
				}
				select {
				case <-release:
				case <-time.After(10 * time.Second):
				}
				inFlight.Add(-1)
				return "branch result", nil
			},
		}
		a, err := agent.NewAgent(agent.WithLLM(llm), agent.WithName(fmt.Sprintf("b%d", i)),
			agent.WithRequirePlanApproval(false))
		if err != nil {
			t.Fatalf("NewAgent() error = %v", err)
		}
		reg.Register(fmt.Sprintf("b%d", i), a)
	}

	combiner, combinerLLM := recordingAgent(t, "combiner", "combined")
	reg.Register("combiner", combiner)

	go func() {
		time.Sleep(300 * time.Millisecond)
		close(release)
	}()

	result, err := Parallel(reg, "combiner", "b0", "b1", "b2").Run(context.Background(), "assess this")
	if err != nil {
		t.Fatalf("Parallel.Run() error = %v", err)
	}
	if result != "combined" {
		t.Errorf("result = %q, want the combiner's output", result)
	}
	if got := peak.Load(); got < 2 {
		t.Errorf("peak concurrent branches = %d, want at least 2: Parallel must fan out", got)
	}
	if prompts := combinerLLM.Prompts(); len(prompts) == 0 || !strings.Contains(prompts[0], "branch result") {
		t.Errorf("combiner did not receive branch results; got %v", prompts)
	}
}

func TestLoopRunsTheRequestedNumberOfIterations(t *testing.T) {
	a, llm := recordingAgent(t, "refiner", "refined")
	reg := registryWith(t, map[string]*agent.Agent{"refiner": a})

	result, err := Loop(reg, "refiner", 3).Run(context.Background(), "draft")
	if err != nil {
		t.Fatalf("Loop.Run() error = %v", err)
	}
	if result != "refined" {
		t.Errorf("result = %q, want the final iteration's output", result)
	}
	if got := llm.Calls(); got != 3 {
		t.Errorf("agent ran %d times, want 3", got)
	}
	// Later iterations see the previous result.
	if prompts := llm.Prompts(); len(prompts) >= 2 && !strings.Contains(prompts[1], "refined") {
		t.Errorf("iteration 2 did not receive iteration 1's result; got %q", prompts[1])
	}
}

func TestLoopRejectsNonPositiveIterations(t *testing.T) {
	a, llm := recordingAgent(t, "refiner", "refined")
	reg := registryWith(t, map[string]*agent.Agent{"refiner": a})

	if _, err := Loop(reg, "refiner", 0).Run(context.Background(), "draft"); err != nil {
		t.Fatalf("Loop.Run() error = %v", err)
	}
	if got := llm.Calls(); got != 1 {
		t.Errorf("agent ran %d times, want 1: a non-positive count is clamped to one "+
			"iteration rather than producing an empty workflow", got)
	}
}

func TestGraphExpressesArbitraryShapes(t *testing.T) {
	a1, _ := recordingAgent(t, "one", "r1")
	a2, _ := recordingAgent(t, "two", "r2")
	a3, llm3 := recordingAgent(t, "three", "r3")
	reg := registryWith(t, map[string]*agent.Agent{"a1": a1, "a2": a2, "a3": a3})

	result, err := Graph(reg, "n3",
		GraphNode{ID: "n1", AgentID: "a1"},
		GraphNode{ID: "n2", AgentID: "a2"},
		GraphNode{ID: "n3", AgentID: "a3", DependsOn: []string{"n1", "n2"}},
	).Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("Graph.Run() error = %v", err)
	}
	if result != "r3" {
		t.Errorf("result = %q, want %q", result, "r3")
	}
	if prompts := llm3.Prompts(); len(prompts) == 0 ||
		!strings.Contains(prompts[0], "r1") || !strings.Contains(prompts[0], "r2") {
		t.Errorf("the join node did not receive both dependency results; got %v", prompts)
	}
}

func TestGraphRejectsCycles(t *testing.T) {
	a, _ := recordingAgent(t, "a", "r")
	reg := registryWith(t, map[string]*agent.Agent{"a": a})

	_, err := Graph(reg, "n1",
		GraphNode{ID: "n1", AgentID: "a", DependsOn: []string{"n2"}},
		GraphNode{ID: "n2", AgentID: "a", DependsOn: []string{"n1"}},
	).Run(context.Background(), "go")
	if err == nil {
		t.Fatal("Graph.Run() returned no error for a cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %v, want it to name the cycle", err)
	}
}

func TestPatternWithNoAgentsIsAnError(t *testing.T) {
	reg := NewAgentRegistry()
	if _, err := Sequential(reg).Run(context.Background(), "go"); err == nil {
		t.Error("Sequential with no agents should be an error, not a silent no-op")
	}
}
