package orchestration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func transferRegistry(t *testing.T) (*AgentRegistry, map[string]*testutil.FakeLLM) {
	t.Helper()
	reg := NewAgentRegistry()
	llms := map[string]*testutil.FakeLLM{}

	for _, name := range []string{"triage", "billing", "technical"} {
		llm := &testutil.FakeLLM{DefaultResponse: name + " answered"}
		a, err := agent.NewAgent(
			agent.WithLLM(llm),
			agent.WithName(name),
			agent.WithRequirePlanApproval(false),
		)
		if err != nil {
			t.Fatalf("NewAgent(%s) error = %v", name, err)
		}
		reg.Register(name, a)
		llms[name] = llm
	}
	return reg, llms
}

func descriptions() map[string]string {
	return map[string]string{
		"triage":    "Route a request to the right specialist",
		"billing":   "Answer invoicing and payment questions",
		"technical": "Diagnose product faults",
	}
}

// TestTransferTargetIsConstrainedToRealAgents is the concrete improvement over
// the regex handoff.
//
// The previous mechanism parsed [HANDOFF:name:reason] out of prose, so nothing
// stopped the model naming an agent that does not exist -- discovered only
// after the fact. An enum makes it unrepresentable.
func TestTransferTargetIsConstrainedToRealAgents(t *testing.T) {
	reg, _ := transferRegistry(t)
	tool := NewTransferTool(reg, descriptions())

	spec := tool.Parameters()["agent"]
	if len(spec.Enum) != 3 {
		t.Fatalf("agent enum has %d entries, want 3", len(spec.Enum))
	}

	got := map[string]bool{}
	for _, v := range spec.Enum {
		got[v.(string)] = true
	}
	for _, want := range []string{"triage", "billing", "technical"} {
		if !got[want] {
			t.Errorf("enum missing %q", want)
		}
	}
}

func TestDescriptionListsAgentsAndTheirPurpose(t *testing.T) {
	reg, _ := transferRegistry(t)
	tool := NewTransferTool(reg, descriptions())

	desc := tool.Description()
	if !strings.Contains(desc, "billing") || !strings.Contains(desc, "invoicing") {
		t.Errorf("description should carry each agent's purpose, since that is what "+
			"the model routes on; got:\n%s", desc)
	}
}

func TestExecuteRecordsTheRequestedTransfer(t *testing.T) {
	reg, _ := transferRegistry(t)
	tool := NewTransferTool(reg, descriptions())

	args, _ := json.Marshal(map[string]string{
		"agent":  "billing",
		"reason": "this is an invoice question",
		"query":  "why was I charged twice?",
	})
	out, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out, "billing") {
		t.Errorf("result = %q, want it to confirm the target", out)
	}

	req, ok := tool.Requested()
	if !ok {
		t.Fatal("no transfer was recorded")
	}
	if req.TargetAgentID != "billing" {
		t.Errorf("target = %q, want %q", req.TargetAgentID, "billing")
	}
	if req.Reason != "this is an invoice question" {
		t.Errorf("reason = %q, want it preserved for debugging", req.Reason)
	}
	if req.Query != "why was I charged twice?" {
		t.Errorf("query = %q", req.Query)
	}
}

// TestRequestedClearsOnRead stops a handle reused across turns from replaying a
// stale routing decision.
func TestRequestedClearsOnRead(t *testing.T) {
	reg, _ := transferRegistry(t)
	tool := NewTransferTool(reg, descriptions())

	args, _ := json.Marshal(map[string]string{"agent": "billing"})
	_, _ = tool.Execute(context.Background(), string(args))

	if _, ok := tool.Requested(); !ok {
		t.Fatal("the first read should return the transfer")
	}
	if _, ok := tool.Requested(); ok {
		t.Error("a second read returned a stale transfer; it must clear on read")
	}
}

func TestUnknownTargetIsRecoverable(t *testing.T) {
	reg, _ := transferRegistry(t)
	tool := NewTransferTool(reg, descriptions())

	args, _ := json.Marshal(map[string]string{"agent": "nonexistent"})
	out, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("an unknown target should not fail the run: %v", err)
	}
	if !strings.Contains(out, "billing") {
		t.Error("the response should list the real agents so the turn is recoverable")
	}
	if _, ok := tool.Requested(); ok {
		t.Error("an unknown target must not be recorded as a transfer")
	}
}

func TestRunWithTransfersFollowsTheChain(t *testing.T) {
	reg, llms := transferRegistry(t)
	tool := NewTransferTool(reg, descriptions())

	// triage transfers to billing on its first call.
	var transferred bool
	llms["triage"].GenerateFunc = func(context.Context, string, []interfaces.Tool) (string, error) {
		if !transferred {
			transferred = true
			args, _ := json.Marshal(map[string]string{
				"agent": "billing",
				"query": "why was I charged twice?",
			})
			_, _ = tool.Execute(context.Background(), string(args))
			return "handing off", nil
		}
		return "triage answered", nil
	}

	result, chain, err := RunWithTransfers(context.Background(), reg, tool,
		"triage", "I have a question", 3)
	if err != nil {
		t.Fatalf("RunWithTransfers() error = %v", err)
	}
	if result != "billing answered" {
		t.Errorf("result = %q, want the target agent's answer", result)
	}
	if len(chain) != 2 || chain[0] != "triage" || chain[1] != "billing" {
		t.Errorf("chain = %v, want [triage billing]", chain)
	}

	// The transferred query should reach the target.
	if prompts := llms["billing"].Prompts(); len(prompts) == 0 ||
		!strings.Contains(prompts[0], "charged twice") {
		t.Errorf("the target did not receive the transferred query; got %v", prompts)
	}
}

func TestNoTransferReturnsTheFirstAnswer(t *testing.T) {
	reg, _ := transferRegistry(t)
	tool := NewTransferTool(reg, descriptions())

	result, chain, err := RunWithTransfers(context.Background(), reg, tool,
		"triage", "simple question", 3)
	if err != nil {
		t.Fatalf("RunWithTransfers() error = %v", err)
	}
	if result != "triage answered" {
		t.Errorf("result = %q", result)
	}
	if len(chain) != 1 {
		t.Errorf("chain = %v, want just the starting agent", chain)
	}
}

// TestTransferLoopIsBounded guards the expensive failure: two agents handing a
// request back and forth, each pass costing a request.
func TestTransferLoopIsBounded(t *testing.T) {
	reg, llms := transferRegistry(t)
	tool := NewTransferTool(reg, descriptions())

	// triage and billing bounce the request between them forever.
	llms["triage"].GenerateFunc = func(context.Context, string, []interfaces.Tool) (string, error) {
		args, _ := json.Marshal(map[string]string{"agent": "billing"})
		_, _ = tool.Execute(context.Background(), string(args))
		return "to billing", nil
	}
	llms["billing"].GenerateFunc = func(context.Context, string, []interfaces.Tool) (string, error) {
		args, _ := json.Marshal(map[string]string{"agent": "triage"})
		_, _ = tool.Execute(context.Background(), string(args))
		return "to triage", nil
	}

	_, chain, err := RunWithTransfers(context.Background(), reg, tool,
		"triage", "bounce me", 10)
	if err == nil {
		t.Fatal("an unbounded transfer loop should be reported")
	}
	if len(chain) > 6 {
		t.Errorf("chain grew to %d hops before stopping: %v", len(chain), chain)
	}
}

func TestTransferLimitReturnsTheLastAnswer(t *testing.T) {
	reg, llms := transferRegistry(t)
	tool := NewTransferTool(reg, descriptions())

	llms["triage"].GenerateFunc = func(context.Context, string, []interfaces.Tool) (string, error) {
		args, _ := json.Marshal(map[string]string{"agent": "billing"})
		_, _ = tool.Execute(context.Background(), string(args))
		return "triage says hand off", nil
	}

	result, _, err := RunWithTransfers(context.Background(), reg, tool,
		"triage", "question", 0)
	if err == nil {
		t.Error("hitting the transfer limit should be reported")
	}
	if result != "triage says hand off" {
		t.Errorf("result = %q, want the last agent's answer rather than nothing: "+
			"a partial answer beats a failure", result)
	}
}

func TestRunWithTransfersRejectsUnknownStart(t *testing.T) {
	reg, _ := transferRegistry(t)
	tool := NewTransferTool(reg, descriptions())

	if _, _, err := RunWithTransfers(context.Background(), reg, tool,
		"nope", "question", 3); err == nil {
		t.Error("an unregistered starting agent should be an error")
	}
}
