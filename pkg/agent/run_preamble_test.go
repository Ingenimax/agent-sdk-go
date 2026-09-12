package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// redactingGuardrails stands in for a real input guardrail: it rewrites the
// input, which is exactly the behaviour that was being discarded.
type redactingGuardrails struct {
	inputErr error
}

func (g *redactingGuardrails) ProcessInput(_ context.Context, input string) (string, error) {
	if g.inputErr != nil {
		return "", g.inputErr
	}
	return strings.ReplaceAll(input, "SECRET", "[REDACTED]"), nil
}

func (g *redactingGuardrails) ProcessOutput(_ context.Context, output string) (string, error) {
	return strings.ReplaceAll(output, "SECRET", "[REDACTED]"), nil
}

func preambleCtx() context.Context {
	ctx := multitenancy.WithOrgID(context.Background(), "org1")
	return memory.WithConversationID(ctx, "conv1")
}

// TestBeginRunPersistsGuardedInput guards a bug that silently disabled input
// guardrails.
//
// Both run paths used to write the RAW input to memory and only afterwards call
// guardrails.ProcessInput, assigning the result to a local variable. The
// providers build their request from memory, not from that variable --
// pkg/llm/openai/message_history.go appends the prompt argument only when memory
// is nil. So with memory configured, which is the normal case, the model was
// shown the unguarded input and ProcessInput had no effect on the request at
// all. For a guardrail whose job is stripping secrets, that is the entire
// feature failing quietly.
func TestBeginRunPersistsGuardedInput(t *testing.T) {
	mem := memory.NewConversationBuffer()

	agent, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("guardrails-agent"),
		WithMemory(mem),
		WithGuardrails(&redactingGuardrails{}),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctx := preambleCtx()

	ctx, guarded, err := agent.beginRun(ctx, "my password is SECRET")
	if err != nil {
		t.Fatalf("beginRun() error = %v", err)
	}

	if strings.Contains(guarded, "SECRET") {
		t.Errorf("beginRun returned unguarded input %q", guarded)
	}

	msgs, err := mem.GetMessages(ctx)
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("memory holds %d messages, want 1", len(msgs))
	}

	if strings.Contains(msgs[0].Content, "SECRET") {
		t.Errorf("memory holds the UNGUARDED input %q: guardrails ran after the memory "+
			"write, so the model is served the raw text and ProcessInput is a no-op",
			msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "[REDACTED]") {
		t.Errorf("memory holds %q, want the redacted form", msgs[0].Content)
	}
}

// TestBeginRunDoesNotPersistOnGuardrailFailure asserts a rejected input is not
// written to memory. Persisting it would leave the transcript containing text
// the guardrail refused.
func TestBeginRunDoesNotPersistOnGuardrailFailure(t *testing.T) {
	mem := memory.NewConversationBuffer()
	sentinel := errors.New("blocked by policy")

	agent, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("blocking-agent"),
		WithMemory(mem),
		WithGuardrails(&redactingGuardrails{inputErr: sentinel}),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctx := preambleCtx()

	if _, _, err := agent.beginRun(ctx, "anything"); err == nil {
		t.Fatal("beginRun() returned no error when guardrails rejected the input")
	}

	msgs, err := mem.GetMessages(ctx)
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("memory holds %d messages after a rejected input, want 0", len(msgs))
	}
}

// TestBeginRunWithoutGuardrailsPersistsInput pins the no-guardrails path, which
// is the common configuration.
func TestBeginRunWithoutGuardrailsPersistsInput(t *testing.T) {
	mem := memory.NewConversationBuffer()

	agent, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("plain-agent"),
		WithMemory(mem),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctx := preambleCtx()

	ctx, guarded, err := agent.beginRun(ctx, "hello")
	if err != nil {
		t.Fatalf("beginRun() error = %v", err)
	}
	if guarded != "hello" {
		t.Errorf("beginRun() = %q, want %q", guarded, "hello")
	}

	msgs, _ := mem.GetMessages(ctx)
	if len(msgs) != 1 || msgs[0].Content != "hello" {
		t.Errorf("memory = %v, want one user message %q", msgs, "hello")
	}
	if len(msgs) == 1 && msgs[0].Role != interfaces.MessageRoleUser {
		t.Errorf("message role = %q, want %q", msgs[0].Role, interfaces.MessageRoleUser)
	}
}

// TestApplyRunIdentityStampsOrg asserts the agent's configured org reaches the
// context, which memory scoping requires.
func TestApplyRunIdentityStampsOrg(t *testing.T) {
	agent, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("org-agent"),
		WithOrgID("acme"),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctx := agent.applyRunIdentity(context.Background())

	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		t.Fatalf("GetOrgID() error = %v", err)
	}
	if orgID != "acme" {
		t.Errorf("org ID = %q, want %q", orgID, "acme")
	}
}

// TestAssembleToolsIsSharedByBothRunPaths asserts the tool set does not depend
// on which run path is taken.
//
// The streaming path used to build its own list and had drifted: it never added
// lazy MCP tools, so an agent configured with them saw a different tool set
// depending on whether it was streamed.
func TestAssembleToolsIsSharedByBothRunPaths(t *testing.T) {
	agent, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("tools-agent"),
		WithTools(
			&testutil.FakeTool{ToolName: "alpha"},
			&testutil.FakeTool{ToolName: "beta"},
		),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	got := agent.assembleTools(context.Background())
	if len(got) != 2 {
		t.Fatalf("assembleTools() returned %d tools, want 2", len(got))
	}

	names := map[string]bool{}
	for _, tl := range got {
		names[tl.Name()] = true
	}
	for _, want := range []string{"alpha", "beta"} {
		if !names[want] {
			t.Errorf("assembleTools() missing %q; got %v", want, names)
		}
	}
}

// TestAssembleToolsDeduplicates pins the deduplication contract. Providers such
// as Anthropic reject requests carrying duplicate tool names.
func TestAssembleToolsDeduplicates(t *testing.T) {
	agent, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("dedup-agent"),
		WithTools(
			&testutil.FakeTool{ToolName: "same"},
			&testutil.FakeTool{ToolName: "same"},
		),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	got := agent.assembleTools(context.Background())
	seen := map[string]int{}
	for _, tl := range got {
		seen[tl.Name()]++
	}
	if seen["same"] > 1 {
		t.Errorf("assembleTools() returned %d tools named %q, want at most 1", seen["same"], "same")
	}
}

// TestOutputGuardrailsRunOnTheDefaultPath guards the fix for output guardrails
// that almost never ran.
//
// ProcessOutput used to have exactly one call site, inside
// runWithoutExecutionPlanWithToolsTracked. Five terminal paths can return a
// response and only that one reached it. Because requirePlanApproval defaults
// to true, an agent configured with tools takes runWithExecutionPlan by
// default -- so on the SDK's DEFAULT configuration, output guardrails did not
// run at all.
func TestOutputGuardrailsRunOnTheDefaultPath(t *testing.T) {
	mem := memory.NewConversationBuffer()

	agent, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("default-path-agent"),
		WithMemory(mem),
		WithGuardrails(&redactingGuardrails{}),
		WithTools(&testutil.FakeTool{ToolName: "stub"}),
		// requirePlanApproval left at its default (true), which is what selects
		// the execution-plan path.
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if !agent.requirePlanApproval {
		t.Fatal("expected requirePlanApproval to default true; this test only " +
			"covers the previously-unguarded path when it does")
	}

	out, err := agent.finishRun(preambleCtx(), "the value is SECRET")
	if err != nil {
		t.Fatalf("finishRun() error = %v", err)
	}

	if strings.Contains(out, "SECRET") {
		t.Errorf("finishRun returned unguarded text %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("finishRun returned %q, want the redacted form", out)
	}
}

// TestFinishRunPersistsGuardedOutput asserts the transcript holds what the
// guardrail approved, so the next turn replays guarded text rather than raw
// model output.
func TestFinishRunPersistsGuardedOutput(t *testing.T) {
	mem := memory.NewConversationBuffer()

	agent, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("persist-guarded-agent"),
		WithMemory(mem),
		WithGuardrails(&redactingGuardrails{}),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctx := preambleCtx()
	if _, err := agent.finishRun(ctx, "the value is SECRET"); err != nil {
		t.Fatalf("finishRun() error = %v", err)
	}

	msgs, err := mem.GetMessages(ctx)
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("memory holds %d messages, want 1", len(msgs))
	}
	if strings.Contains(msgs[0].Content, "SECRET") {
		t.Errorf("memory holds UNGUARDED output %q: the next turn would replay it "+
			"to the model", msgs[0].Content)
	}
	if msgs[0].Role != interfaces.MessageRoleAssistant {
		t.Errorf("role = %q, want %q", msgs[0].Role, interfaces.MessageRoleAssistant)
	}
}

// TestGuardOutputRejectionIsNotPersisted asserts a rejected response does not
// reach the transcript.
func TestGuardOutputRejectionIsNotPersisted(t *testing.T) {
	mem := memory.NewConversationBuffer()
	sentinel := errors.New("output blocked by policy")

	agent, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("reject-output-agent"),
		WithMemory(mem),
		WithGuardrails(&blockingOutputGuardrails{err: sentinel}),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctx := preambleCtx()
	if _, err := agent.finishRun(ctx, "anything"); err == nil {
		t.Fatal("finishRun() returned no error when output guardrails rejected the response")
	}

	msgs, _ := mem.GetMessages(ctx)
	if len(msgs) != 0 {
		t.Errorf("memory holds %d messages after a rejected response, want 0", len(msgs))
	}
}

// blockingOutputGuardrails rejects on output only.
type blockingOutputGuardrails struct{ err error }

func (g *blockingOutputGuardrails) ProcessInput(_ context.Context, in string) (string, error) {
	return in, nil
}

func (g *blockingOutputGuardrails) ProcessOutput(context.Context, string) (string, error) {
	return "", g.err
}

// TestWithAgentsIsIdempotent guards a defect where the option accumulated tools
// while replacing sub-agents.
//
// It assigned a.subAgents (replacing) but appended to a.tools (accumulating),
// so calling it twice left one sub-agent and two tools -- the second wrapping an
// agent no longer registered. The model could then call a tool the parent did
// not know about.
func TestWithAgentsIsIdempotent(t *testing.T) {
	child, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("child"),
		WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("NewAgent(child) error = %v", err)
	}

	replacement, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("replacement"),
		WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("NewAgent(replacement) error = %v", err)
	}

	parent, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("parent"),
		WithRequirePlanApproval(false),
		WithAgents(child),
		WithAgents(replacement), // applied twice
	)
	if err != nil {
		t.Fatalf("NewAgent(parent) error = %v", err)
	}

	if len(parent.subAgents) != 1 {
		t.Errorf("subAgents = %d, want 1", len(parent.subAgents))
	}

	var agentTools []string
	for _, tool := range parent.tools {
		if strings.HasSuffix(tool.Name(), "_agent") {
			agentTools = append(agentTools, tool.Name())
		}
	}
	if len(agentTools) != 1 {
		t.Errorf("sub-agent tools = %v, want exactly one: a replaced sub-agent must "+
			"not leave its tool behind, or the model can call an agent the parent "+
			"no longer knows about", agentTools)
	}
	if len(agentTools) == 1 && agentTools[0] != "replacement_agent" {
		t.Errorf("surviving tool = %q, want the replacement", agentTools[0])
	}
}

// TestRuntimeTimeoutBoundsTheRun guards a config setting that did nothing.
//
// a.timeout was assigned from `runtime.timeout` in YAML and read nowhere in the
// module, so configuring it had no effect at all.
func TestRuntimeTimeoutBoundsTheRun(t *testing.T) {
	agent, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("timeout-agent"),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	agent.timeout = 50 * time.Millisecond

	ctx, cancel := agent.applyRunTimeout(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("no deadline was applied; a configured runtime timeout must bound the run")
	}
	if time.Until(deadline) > time.Second {
		t.Errorf("deadline is %s away, want roughly the configured 50ms", time.Until(deadline))
	}
}

// TestRuntimeTimeoutDoesNotExtendATighterDeadline asserts a config default
// cannot loosen a bound the caller deliberately set.
func TestRuntimeTimeoutDoesNotExtendATighterDeadline(t *testing.T) {
	agent, err := NewAgent(WithLLM(testutil.NewFakeLLM()), WithName("timeout-agent"))
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	agent.timeout = time.Hour

	caller, cancelCaller := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelCaller()

	ctx, cancel := agent.applyRunTimeout(caller)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("the caller's deadline was lost")
	}
	if time.Until(deadline) > time.Minute {
		t.Error("the agent's longer timeout replaced the caller's tighter deadline; " +
			"whichever bound is tighter should win")
	}
}

func TestNoRuntimeTimeoutLeavesTheContextAlone(t *testing.T) {
	agent, err := NewAgent(WithLLM(testutil.NewFakeLLM()), WithName("no-timeout"))
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctx, cancel := agent.applyRunTimeout(context.Background())
	defer cancel()

	if _, ok := ctx.Deadline(); ok {
		t.Error("a deadline was applied when none was configured")
	}
}
