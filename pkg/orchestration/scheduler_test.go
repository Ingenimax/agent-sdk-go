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

// newCountingAgent returns an agent whose LLM records how many times it ran,
// plus the counter.
func newCountingAgent(t *testing.T, name string, reply string) (*agent.Agent, *testutil.FakeLLM) {
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

func registryWith(t *testing.T, agents map[string]*agent.Agent) *AgentRegistry {
	t.Helper()
	reg := NewAgentRegistry()
	for id, a := range agents {
		reg.Register(id, a)
	}
	return reg
}

// runWithTimeout fails the test rather than hanging the suite, which is the
// only way a deadlock regression shows up as a readable failure.
func runWithTimeout(t *testing.T, o *CodeOrchestrator, wf *Workflow, d time.Duration) (string, error) {
	t.Helper()

	type outcome struct {
		result string
		err    error
	}
	ch := make(chan outcome, 1)

	go func() {
		result, err := o.ExecuteWorkflow(context.Background(), wf)
		ch <- outcome{result, err}
	}()

	select {
	case got := <-ch:
		return got.result, got.err
	case <-time.After(d):
		t.Fatalf("ExecuteWorkflow did not return within %s: the scheduler deadlocked", d)
		return "", nil
	}
}

// TestParallelTasksDoNotDeadlock guards the deadlock.
//
// executeTask used to end with an unbuffered `completionCh <- task.ID` and no
// select on ctx.Done(). When the monitor goroutine saw every task finished it
// called cancel() and returned -- so a second task finishing at the same moment
// blocked forever on that send, its deferred wg.Done() never ran, and
// wg.Wait() hung. Two independent tasks completing together was enough.
func TestParallelTasksDoNotDeadlock(t *testing.T) {
	a1, _ := newCountingAgent(t, "one", "result one")
	a2, _ := newCountingAgent(t, "two", "result two")
	a3, _ := newCountingAgent(t, "three", "result three")

	o := NewCodeOrchestrator(registryWith(t, map[string]*agent.Agent{
		"a1": a1, "a2": a2, "a3": a3,
	}))

	wf := NewWorkflow()
	// Three tasks with no dependencies all finish at once.
	wf.AddTask("t1", "a1", "go", nil)
	wf.AddTask("t2", "a2", "go", nil)
	wf.AddTask("t3", "a3", "go", []string{"t1", "t2"})
	wf.SetFinalTask("t3")

	result, err := runWithTimeout(t, o, wf, 20*time.Second)
	if err != nil {
		t.Fatalf("ExecuteWorkflow() error = %v", err)
	}
	if result != "result three" {
		t.Errorf("result = %q, want %q", result, "result three")
	}
}

// TestEachTaskExecutesExactlyOnce guards duplicate execution.
//
// The monitor launched any task still in TaskPending, but TaskRunning was set
// inside the spawned goroutine. Two completion signals arriving before that
// write both launched the same task -- running the agent twice and billing for
// it twice, the same defect class as the sub-agent double execution.
func TestEachTaskExecutesExactlyOnce(t *testing.T) {
	a1, llm1 := newCountingAgent(t, "one", "r1")
	a2, llm2 := newCountingAgent(t, "two", "r2")
	shared, sharedLLM := newCountingAgent(t, "shared", "shared result")

	o := NewCodeOrchestrator(registryWith(t, map[string]*agent.Agent{
		"a1": a1, "a2": a2, "shared": shared,
	}))

	wf := NewWorkflow()
	wf.AddTask("t1", "a1", "go", nil)
	wf.AddTask("t2", "a2", "go", nil)
	// Depends on both, so both completions can race to launch it.
	wf.AddTask("fan-in", "shared", "go", []string{"t1", "t2"})
	wf.SetFinalTask("fan-in")

	if _, err := runWithTimeout(t, o, wf, 20*time.Second); err != nil {
		t.Fatalf("ExecuteWorkflow() error = %v", err)
	}

	if got := sharedLLM.Calls(); got != 1 {
		t.Errorf("the fan-in task's agent ran %d times, want exactly 1: two "+
			"dependency completions must not both launch it", got)
	}
	if got := llm1.Calls(); got != 1 {
		t.Errorf("t1 ran %d times, want 1", got)
	}
	if got := llm2.Calls(); got != 1 {
		t.Errorf("t2 ran %d times, want 1", got)
	}
}

// TestConcurrentResultWritesAreRaceFree guards the data race.
//
// workflow.Results and workflow.Errors were plain maps written from N
// goroutines with no lock; only a separate mutex guarding an unrelated map was
// held. task.Status was likewise written by workers and read by the monitor.
// Meaningful only under -race, which CI runs.
func TestConcurrentResultWritesAreRaceFree(t *testing.T) {
	const fanOut = 12

	agents := map[string]*agent.Agent{}
	wf := NewWorkflow()
	var deps []string

	for i := 0; i < fanOut; i++ {
		id := fmt.Sprintf("a%d", i)
		a, _ := newCountingAgent(t, id, fmt.Sprintf("result %d", i))
		agents[id] = a
		taskID := fmt.Sprintf("t%d", i)
		wf.AddTask(taskID, id, "go", nil)
		deps = append(deps, taskID)
	}

	final, _ := newCountingAgent(t, "final", "final result")
	agents["final"] = final
	wf.AddTask("final", "final", "combine", deps)
	wf.SetFinalTask("final")

	o := NewCodeOrchestrator(registryWith(t, agents))

	result, err := runWithTimeout(t, o, wf, 30*time.Second)
	if err != nil {
		t.Fatalf("ExecuteWorkflow() error = %v", err)
	}
	if result != "final result" {
		t.Errorf("result = %q, want %q", result, "final result")
	}
	if len(wf.Results) != fanOut+1 {
		t.Errorf("collected %d results, want %d", len(wf.Results), fanOut+1)
	}
}

// TestDependencyCycleIsRejected asserts a cycle is reported rather than hanging.
// Every task in a cycle waits for a dependency that will never complete.
func TestDependencyCycleIsRejected(t *testing.T) {
	a, _ := newCountingAgent(t, "a", "r")
	o := NewCodeOrchestrator(registryWith(t, map[string]*agent.Agent{"a": a}))

	wf := NewWorkflow()
	wf.AddTask("t1", "a", "go", []string{"t2"})
	wf.AddTask("t2", "a", "go", []string{"t1"})

	_, err := runWithTimeout(t, o, wf, 20*time.Second)
	if err == nil {
		t.Fatal("ExecuteWorkflow() returned no error for a dependency cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %v, want it to name the cycle", err)
	}
}

// TestUnknownDependencyIsRejected asserts a reference to a task that does not
// exist is reported rather than waiting forever on it.
func TestUnknownDependencyIsRejected(t *testing.T) {
	a, _ := newCountingAgent(t, "a", "r")
	o := NewCodeOrchestrator(registryWith(t, map[string]*agent.Agent{"a": a}))

	wf := NewWorkflow()
	wf.AddTask("t1", "a", "go", []string{"does-not-exist"})

	_, err := runWithTimeout(t, o, wf, 20*time.Second)
	if err == nil {
		t.Fatal("ExecuteWorkflow() returned no error for an unknown dependency")
	}
	if !strings.Contains(err.Error(), "unknown task") {
		t.Errorf("error = %v, want it to name the unknown task", err)
	}
}

// TestFailedDependencySkipsDependents asserts a task whose dependency failed
// does not run: its input would be missing the result it was written to consume.
func TestFailedDependencySkipsDependents(t *testing.T) {
	failing := &testutil.FakeLLM{Err: fmt.Errorf("model unavailable")}
	failAgent, err := agent.NewAgent(
		agent.WithLLM(failing),
		agent.WithName("failing"),
		agent.WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	downstream, downstreamLLM := newCountingAgent(t, "downstream", "should not run")

	o := NewCodeOrchestrator(registryWith(t, map[string]*agent.Agent{
		"failing": failAgent, "downstream": downstream,
	}))

	wf := NewWorkflow()
	wf.AddTask("t1", "failing", "go", nil)
	wf.AddTask("t2", "downstream", "go", []string{"t1"})
	wf.SetFinalTask("t2")

	_, execErr := runWithTimeout(t, o, wf, 20*time.Second)
	if execErr == nil {
		t.Fatal("ExecuteWorkflow() returned no error when the final task's dependency failed")
	}
	if got := downstreamLLM.Calls(); got != 0 {
		t.Errorf("the dependent task ran %d times, want 0: its input would be "+
			"missing the dependency result it was written to consume", got)
	}
}

// TestIndependentTasksRunConcurrently asserts the scheduler still fans out
// rather than serialising. A correctness fix that removed parallelism would be
// its own regression, so this measures peak concurrent execution directly.
func TestIndependentTasksRunConcurrently(t *testing.T) {
	const fanOut = 4

	var inFlight atomic.Int64
	var peak atomic.Int64
	release := make(chan struct{})

	agents := map[string]*agent.Agent{}
	wf := NewWorkflow()

	for i := 0; i < fanOut; i++ {
		id := fmt.Sprintf("a%d", i)
		llm := &testutil.FakeLLM{
			GenerateFunc: func(ctx context.Context, _ string, _ []interfaces.Tool) (string, error) {
				now := inFlight.Add(1)
				for {
					was := peak.Load()
					if now <= was || peak.CompareAndSwap(was, now) {
						break
					}
				}
				// Hold until every task has had a chance to start.
				select {
				case <-release:
				case <-time.After(10 * time.Second):
				}
				inFlight.Add(-1)
				return "done", nil
			},
		}
		a, err := agent.NewAgent(
			agent.WithLLM(llm),
			agent.WithName(id),
			agent.WithRequirePlanApproval(false),
		)
		if err != nil {
			t.Fatalf("NewAgent() error = %v", err)
		}
		agents[id] = a
		wf.AddTask(fmt.Sprintf("t%d", i), id, "go", nil)
	}

	o := NewCodeOrchestrator(registryWith(t, agents))

	// Let the tasks proceed once they are all plausibly in flight.
	go func() {
		time.Sleep(300 * time.Millisecond)
		close(release)
	}()

	if _, err := runWithTimeout(t, o, wf, 30*time.Second); err != nil {
		t.Fatalf("ExecuteWorkflow() error = %v", err)
	}

	if got := peak.Load(); got < 2 {
		t.Errorf("peak concurrent tasks = %d, want at least 2: independent tasks "+
			"must still run in parallel", got)
	}
	if len(wf.Results) != fanOut {
		t.Errorf("collected %d results, want %d", len(wf.Results), fanOut)
	}
}
