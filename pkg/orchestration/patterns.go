package orchestration

import (
	"context"
	"fmt"
)

// Sequential builds a workflow that runs agents one after another, feeding each
// agent's result into the next.
//
// These three patterns were always expressible with AddTask and dependency
// lists, but every caller had to hand-roll the wiring -- and the sequential
// case in particular is easy to get subtly wrong, since "sequential" is encoded
// as each task depending on the previous one rather than declared.
//
//	result, err := orchestration.Sequential(reg, "research", "draft", "edit").
//	    Run(ctx, "write about tide pools")
//
// The result of the final agent is the result of the workflow.
func Sequential(registry *AgentRegistry, agentIDs ...string) *Pattern {
	wf := NewWorkflow()

	var previous []string
	for i, agentID := range agentIDs {
		taskID := fmt.Sprintf("step-%d", i)
		wf.AddTask(taskID, agentID, "", previous)
		previous = []string{taskID}
	}

	if len(agentIDs) > 0 {
		wf.SetFinalTask(fmt.Sprintf("step-%d", len(agentIDs)-1))
	}

	return &Pattern{registry: registry, workflow: wf, kind: "sequential"}
}

// Parallel builds a workflow that runs every agent concurrently on the same
// input, then combines their results with a final agent.
//
// combinerID receives each branch's output appended to its input, in the order
// the branches were declared.
//
//	result, err := orchestration.Parallel(reg, "summarize",
//	    "legal-review", "security-review", "cost-review").
//	    Run(ctx, "assess this proposal")
func Parallel(registry *AgentRegistry, combinerID string, agentIDs ...string) *Pattern {
	wf := NewWorkflow()

	branches := make([]string, 0, len(agentIDs))
	for i, agentID := range agentIDs {
		taskID := fmt.Sprintf("branch-%d", i)
		wf.AddTask(taskID, agentID, "", nil)
		branches = append(branches, taskID)
	}

	wf.AddTask("combine", combinerID, "", branches)
	wf.SetFinalTask("combine")

	return &Pattern{registry: registry, workflow: wf, kind: "parallel"}
}

// Loop builds a workflow that runs one agent repeatedly, feeding each
// iteration's result into the next.
//
// iterations must be at least 1. Unlike ADK's LoopAgent there is no
// model-driven escape hatch yet: the count is fixed, which is why the parameter
// is required rather than defaulted -- an accidental unbounded loop against a
// paid API is not a failure mode worth offering.
func Loop(registry *AgentRegistry, agentID string, iterations int) *Pattern {
	if iterations < 1 {
		iterations = 1
	}

	wf := NewWorkflow()

	var previous []string
	for i := 0; i < iterations; i++ {
		taskID := fmt.Sprintf("iteration-%d", i)
		wf.AddTask(taskID, agentID, "", previous)
		previous = []string{taskID}
	}
	wf.SetFinalTask(fmt.Sprintf("iteration-%d", iterations-1))

	return &Pattern{registry: registry, workflow: wf, kind: "loop"}
}

// Graph builds a workflow from an explicit dependency graph, for shapes the
// three named patterns do not cover.
//
// Each node names the agent to run and the node IDs it depends on. Cycles and
// references to unknown nodes are rejected when the workflow runs.
func Graph(registry *AgentRegistry, finalNodeID string, nodes ...GraphNode) *Pattern {
	wf := NewWorkflow()
	for _, node := range nodes {
		wf.AddTask(node.ID, node.AgentID, node.Input, node.DependsOn)
	}
	wf.SetFinalTask(finalNodeID)

	return &Pattern{registry: registry, workflow: wf, kind: "graph"}
}

// GraphNode is one agent invocation in a Graph workflow.
type GraphNode struct {
	// ID identifies this node; other nodes reference it in DependsOn.
	ID string

	// AgentID is the registered agent to run.
	AgentID string

	// Input is this node's own input, before dependency results are appended.
	Input string

	// DependsOn lists node IDs that must complete first.
	DependsOn []string
}

// Pattern is a composed multi-agent workflow, ready to run.
type Pattern struct {
	registry *AgentRegistry
	workflow *Workflow
	kind     string
	input    string
}

// Run executes the pattern. input is supplied to every task that has no input
// of its own, which for Sequential and Loop means the first step and for
// Parallel means every branch.
func (p *Pattern) Run(ctx context.Context, input string) (string, error) {
	if p.registry == nil {
		return "", fmt.Errorf("%s pattern has no agent registry", p.kind)
	}
	if len(p.workflow.Tasks) == 0 {
		return "", fmt.Errorf("%s pattern has no agents", p.kind)
	}

	for _, task := range p.workflow.Tasks {
		if task.Input == "" {
			task.Input = input
		}
	}
	p.input = input

	return NewCodeOrchestrator(p.registry).ExecuteWorkflow(ctx, p.workflow)
}

// Workflow exposes the underlying workflow, so a pattern can be inspected or
// extended before it runs.
func (p *Pattern) Workflow() *Workflow { return p.workflow }
