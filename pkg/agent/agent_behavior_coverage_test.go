package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/executionplan"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
)

// applyGenerateOptions folds options into a GenerateOptions value.
func applyGenerateOptions(options []interfaces.GenerateOption) interfaces.GenerateOptions {
	var o interfaces.GenerateOptions
	for _, opt := range options {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// scriptedStreamLLM is a FakeLLM whose streams replay a fixed list of events.
// It records the options of the last stream call.
type scriptedStreamLLM struct {
	*testutil.FakeLLM
	events   []interfaces.StreamEvent
	startErr error

	mu      sync.Mutex
	options interfaces.GenerateOptions
	tools   []string
}

func newScriptedStreamLLM(events ...interfaces.StreamEvent) *scriptedStreamLLM {
	return &scriptedStreamLLM{FakeLLM: &testutil.FakeLLM{Streaming: true}, events: events}
}

func (s *scriptedStreamLLM) GenerateStream(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	return s.GenerateWithToolsStream(ctx, prompt, nil, options...)
}

func (s *scriptedStreamLLM) GenerateWithToolsStream(_ context.Context, _ string, toolSet []interfaces.Tool, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	s.mu.Lock()
	s.options = applyGenerateOptions(options)
	s.tools = toolNames(toolSet)
	s.mu.Unlock()
	if s.startErr != nil {
		return nil, s.startErr
	}
	ch := make(chan interfaces.StreamEvent, len(s.events))
	for _, e := range s.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func (s *scriptedStreamLLM) lastOptions() interfaces.GenerateOptions {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.options
}

// fakeDataStore is an interfaces.DataStore used only for identity checks.
type fakeDataStore struct{ interfaces.DataStore }

// fakeGraphStore is an interfaces.GraphRAGStore used only for identity checks.
type fakeGraphStore struct{ interfaces.GraphRAGStore }

func TestOptionsReachTheLLM(t *testing.T) {
	var got interfaces.GenerateOptions
	llm := &mockLLM{generateFunc: func(_ context.Context, _ string, options ...interfaces.GenerateOption) (string, error) {
		got = applyGenerateOptions(options)
		return "ok", nil
	}}
	mem := &testutil.FakeMemory{}
	format := interfaces.ResponseFormat{Type: interfaces.ResponseFormatJSON, Name: "Out"}

	a, err := NewAgent(
		WithLLM(llm),
		WithMemory(mem),
		WithSystemPrompt("be terse"),
		WithResponseFormat(format),
		WithLLMConfig(interfaces.LLMConfig{Temperature: 0.3}),
		WithCacheConfig(interfaces.CacheConfig{CacheTools: true}),
		WithMaxIterations(9),
		WithDisableFinalSummary(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}

	if got.SystemMessage != "be terse" || got.MaxIterations != 9 || !got.DisableFinalSummary {
		t.Errorf("scalar options = %+v", got)
	}
	if got.ResponseFormat == nil || got.ResponseFormat.Name != "Out" {
		t.Errorf("response format = %+v", got.ResponseFormat)
	}
	if got.LLMConfig == nil || got.LLMConfig.Temperature != 0.3 {
		t.Errorf("llm config = %+v", got.LLMConfig)
	}
	if got.CacheConfig == nil || !got.CacheConfig.CacheTools {
		t.Errorf("cache config = %+v", got.CacheConfig)
	}
	if got.Memory != mem {
		t.Errorf("memory = %v", got.Memory)
	}
}

func TestSimpleOptionsAndAccessors(t *testing.T) {
	ds := &fakeDataStore{}
	lazy := []LazyMCPConfig{{Name: "x"}}
	stream := &interfaces.StreamConfig{BufferSize: 4}

	a := &Agent{}
	for _, opt := range []Option{WithDataStore(ds), WithLazyMCPConfigs(lazy), WithStreamConfig(stream)} {
		opt(a)
	}
	if a.GetDataStore() != ds || len(a.lazyMCPConfigs) != 1 || a.streamConfig != stream {
		t.Fatalf("options not applied: %+v", a)
	}
	other := &fakeDataStore{}
	a.SetDataStore(other)
	if a.GetDataStore() != other {
		t.Error("SetDataStore did not replace the datastore")
	}

	if got := GetInvocationID(context.Background()); got != "" {
		t.Errorf("GetInvocationID() = %q", got)
	}
	ctx, cancel := WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Error("WithTimeout did not set a deadline")
	}
}

func TestGetCapabilities(t *testing.T) {
	tests := []struct {
		agent *Agent
		want  string
	}{
		{&Agent{description: "does things", systemPrompt: "p"}, "does things"},
		{&Agent{systemPrompt: "be kind"}, "Agent with system prompt: be kind"},
		{&Agent{}, "A general-purpose AI agent"},
	}
	for _, tc := range tests {
		if got := tc.agent.GetCapabilities(); got != tc.want {
			t.Errorf("GetCapabilities() = %q, want %q", got, tc.want)
		}
	}
}

func TestConversationAccessors(t *testing.T) {
	ctx := context.Background()
	conv := &testutil.FakeConversationMemory{Conversations: []string{"c1", "c2"}}
	if err := conv.AddMessage(ctx, interfaces.Message{Role: "user", Content: "hi"}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		memory    interfaces.Memory
		wantConvs []string
		wantMsgs  int
	}{
		{"no memory", nil, []string{}, 0},
		{"plain memory", &testutil.FakeMemory{}, []string{}, 0},
		{"conversation memory", conv, []string{"c1", "c2"}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{memory: tc.memory}
			convs, err := a.GetAllConversations(ctx)
			if err != nil || !slices.Equal(convs, tc.wantConvs) {
				t.Errorf("GetAllConversations() = %v, %v", convs, err)
			}
			msgs, err := a.GetConversationMessages(ctx, "c1")
			if err != nil || len(msgs) != tc.wantMsgs {
				t.Errorf("GetConversationMessages() = %v, %v", msgs, err)
			}
			total, count, err := a.GetMemoryStatistics(ctx)
			if err != nil || total != len(tc.wantConvs) || count != tc.wantMsgs {
				t.Errorf("GetMemoryStatistics() = %d, %d, %v", total, count, err)
			}
		})
	}
}

func TestWithGraphRAG(t *testing.T) {
	store := &fakeGraphStore{}

	withLLM, err := NewAgent(WithLLM(testutil.NewFakeLLM()), WithGraphRAG(store))
	if err != nil {
		t.Fatal(err)
	}
	if !withLLM.HasGraphRAG() || withLLM.GetGraphRAGStore() != store {
		t.Error("store not registered")
	}
	want := []string{"graphrag_search", "graphrag_add_entity", "graphrag_add_relationship", "graphrag_get_context", "graphrag_extract"}
	if got := toolNames(withLLM.GetTools()); !slices.Equal(got, want) {
		t.Errorf("tools = %v", got)
	}

	// Without an LLM the extract tool is left out.
	noLLM := &Agent{}
	WithGraphRAG(store)(noLLM)
	if got := toolNames(noLLM.tools); !slices.Equal(got, want[:4]) {
		t.Errorf("tools without llm = %v", got)
	}

	none := &Agent{}
	WithGraphRAG(nil)(none)
	if none.HasGraphRAG() || len(none.tools) != 0 {
		t.Error("nil store must be a no-op")
	}
}

func TestTrackingToolForwards(t *testing.T) {
	inner := &testutil.FakeTool{ToolName: "inner", Desc: "inner desc", Result: "r", Params: map[string]interfaces.ParameterSpec{"q": {Type: "string"}}}
	tracker := newUsageTracker(true)
	tt := &trackingTool{inner: inner, tracker: tracker}

	if tt.Description() != "inner desc" || len(tt.Parameters()) != 1 {
		t.Errorf("description/parameters = %q / %v", tt.Description(), tt.Parameters())
	}
	got, err := tt.Run(context.Background(), "in")
	if err != nil || got != "r" {
		t.Fatalf("Run() = %q, %v", got, err)
	}
	_, summary, _ := tracker.getResults()
	if summary.ToolCalls != 1 || !slices.Equal(summary.UsedTools, []string{"inner"}) {
		t.Errorf("summary = %+v", summary)
	}

	// A nil tracker only delegates.
	if out, err := (&trackingTool{inner: inner}).Run(context.Background(), "x"); err != nil || out != "r" {
		t.Errorf("Run() without tracker = %q, %v", out, err)
	}
}

func TestDeduplicateToolsSkipsNilAndUnnamed(t *testing.T) {
	a := &testutil.FakeTool{ToolName: "a"}
	got := deduplicateTools([]interfaces.Tool{nil, &mockTool{name: ""}, a, &testutil.FakeTool{ToolName: "a"}})
	if len(got) != 1 || got[0] != a {
		t.Errorf("deduplicateTools() = %v", toolNames(got))
	}
}

// planAgent returns an agent whose LLM answers every prompt with a one-step
// plan that calls the "stub" tool.
func planAgent(t *testing.T, mem interfaces.Memory, stub *testutil.FakeTool) (*Agent, *testutil.FakeLLM) {
	t.Helper()
	llm := &testutil.FakeLLM{DefaultResponse: streamPlanJSON}
	opts := []Option{WithLLM(llm), WithTools(stub), WithName("planner")}
	if mem != nil {
		opts = append(opts, WithMemory(mem))
	}
	a, err := NewAgent(opts...)
	if err != nil {
		t.Fatal(err)
	}
	return a, llm
}

func TestPlanActions(t *testing.T) {
	ctx := context.Background()
	stub := &testutil.FakeTool{ToolName: "stub", Result: "stub ran"}
	mem := &testutil.FakeMemory{}
	a, _ := planAgent(t, mem, stub)

	plan, err := a.GenerateExecutionPlan(ctx, "look around")
	if err != nil {
		t.Fatal(err)
	}
	a.planStore.StorePlan(plan)
	if got, ok := a.GetTaskByID(plan.TaskID); !ok || got != plan {
		t.Fatalf("GetTaskByID() = %v, %v", got, ok)
	}
	if len(a.ListTasks()) != 1 {
		t.Errorf("ListTasks() = %v", a.ListTasks())
	}

	status, err := a.handlePlanAction(ctx, plan.TaskID, "status", "")
	if err != nil || !strings.Contains(status, "Current plan status: pending_approval") || !strings.Contains(status, plan.TaskID) {
		t.Errorf("status = %q, %v", status, err)
	}

	modified, err := a.handlePlanAction(ctx, plan.TaskID, "modify", "be faster")
	if err != nil || !strings.Contains(modified, "I've updated the execution plan") {
		t.Errorf("modify = %q, %v", modified, err)
	}

	current, _ := a.GetTaskByID(plan.TaskID)
	approved, err := a.handlePlanAction(ctx, plan.TaskID, "approve", "")
	if err != nil || !strings.Contains(approved, "stub ran") {
		t.Fatalf("approve = %q, %v", approved, err)
	}
	if stub.CallCount() != 1 || current.Status != executionplan.StatusCompleted || !current.UserApproved {
		t.Errorf("after approve: calls = %d, status = %s", stub.CallCount(), current.Status)
	}

	cancelled, err := a.handlePlanAction(ctx, plan.TaskID, "cancel", "")
	if err != nil || !strings.Contains(cancelled, "Plan cancelled") || current.Status != executionplan.StatusCancelled {
		t.Errorf("cancel = %q, %v, status %s", cancelled, err, current.Status)
	}

	if _, err := a.handlePlanAction(ctx, plan.TaskID, "dance", ""); err == nil || !strings.Contains(err.Error(), "unknown plan action") {
		t.Errorf("unknown action error = %v", err)
	}
	if _, err := a.handlePlanAction(ctx, "missing", "status", ""); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing plan error = %v", err)
	}

	// Approval and modification requests are written to memory as user turns.
	msgs, _ := mem.GetMessages(ctx)
	var userTurns []string
	for _, m := range msgs {
		if m.Role == interfaces.MessageRoleUser {
			userTurns = append(userTurns, m.Content)
		}
	}
	if !slices.Contains(userTurns, "I'd like to modify the plan: be faster") ||
		!slices.Contains(userTurns, "I approve the plan. Please proceed with execution.") {
		t.Errorf("user turns = %v", userTurns)
	}
}

func TestPlanActionFailures(t *testing.T) {
	ctx := context.Background()
	failing := &addFailingMemory{err: errors.New("disk full")}

	a, _ := planAgent(t, failing, &testutil.FakeTool{ToolName: "stub"})
	plan, err := a.GenerateExecutionPlan(ctx, "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ApproveExecutionPlan(ctx, plan); err == nil || !strings.Contains(err.Error(), "failed to add approval to memory") {
		t.Errorf("approve memory error = %v", err)
	}
	if _, err := a.modifyPlan(ctx, plan, "x"); err == nil || !strings.Contains(err.Error(), "failed to add modification request") {
		t.Errorf("modify memory error = %v", err)
	}

	b, llm := planAgent(t, nil, &testutil.FakeTool{ToolName: "stub", Err: errors.New("tool broke")})
	plan, err = b.GenerateExecutionPlan(ctx, "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.ApproveExecutionPlan(ctx, plan); err == nil || !strings.Contains(err.Error(), "failed to execute plan") {
		t.Errorf("approve tool error = %v", err)
	}
	llm.Err = errors.New("llm down")
	if _, err := b.modifyPlan(ctx, plan, "x"); err == nil || !strings.Contains(err.Error(), "failed to modify plan") {
		t.Errorf("modify llm error = %v", err)
	}
	if _, err := b.ModifyExecutionPlan(ctx, plan, "x"); err == nil {
		t.Error("expected ModifyExecutionPlan to surface the llm error")
	}
	if _, err := b.Run(ctx, "plan this"); err == nil || !strings.Contains(err.Error(), "failed to generate execution plan") {
		t.Errorf("run plan error = %v", err)
	}
}

func TestRoleQuestions(t *testing.T) {
	ctx := context.Background()

	llm := &testutil.FakeLLM{DefaultResponse: "I am Helper."}
	a, err := NewAgent(WithLLM(llm), WithName("Helper"), WithSystemPrompt("You help with Go."))
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Run(ctx, "Who are you?")
	if err != nil || got != "I am Helper." {
		t.Fatalf("Run() = %q, %v", got, err)
	}
	if prompt := llm.Prompts()[0]; !strings.Contains(prompt, `You are named "Helper"`) || !strings.Contains(prompt, "You help with Go.") {
		t.Errorf("role prompt = %q", prompt)
	}

	// Plain questions do not take the role path.
	if a.isAskingAboutRole("what is 2+2") {
		t.Error("isAskingAboutRole matched an unrelated question")
	}

	// LLM errors fall back to a canned answer, with and without a name.
	failing := &testutil.FakeLLM{Err: errors.New("down")}
	tests := []struct {
		agent *Agent
		want  string
	}{
		{&Agent{llm: failing, systemPrompt: "p", name: "Bot"}, "I'm Bot, an AI assistant"},
		{&Agent{llm: failing, systemPrompt: "p"}, "I'm an AI assistant based on the role"},
		{&Agent{}, "designed to help you with various tasks"},
	}
	for _, tc := range tests {
		if got := tc.agent.generateRoleResponse(ctx); !strings.Contains(got, tc.want) {
			t.Errorf("generateRoleResponse() = %q, want it to contain %q", got, tc.want)
		}
	}
}

func TestExecuteTaskFromConfig(t *testing.T) {
	ctx := context.Background()
	llm := &testutil.FakeLLM{GenerateFunc: func(_ context.Context, prompt string, _ []interfaces.Tool) (string, error) {
		return "result for " + prompt, nil
	}}
	a, err := NewAgent(WithLLM(llm))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	tasks := TaskConfigs{
		"write":  {Description: "Write about {topic}", OutputFile: filepath.Join(dir, "{topic}.md")},
		"say":    {Description: "Say {topic}"},
		"broken": {Description: "x", OutputFile: filepath.Join(dir, "missing", "out.md")},
	}
	vars := map[string]string{"topic": "go"}

	got, err := a.ExecuteTaskFromConfig(ctx, "write", tasks, vars)
	if err != nil || got != "result for Write about go" {
		t.Fatalf("ExecuteTaskFromConfig() = %q, %v", got, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "go.md"))
	if err != nil || string(data) != got {
		t.Errorf("output file = %q, %v", data, err)
	}

	if got, err := a.ExecuteTaskFromConfig(ctx, "say", tasks, vars); err != nil || got != "result for Say go" {
		t.Errorf("no output file = %q, %v", got, err)
	}
	if got, err := a.ExecuteTaskFromConfig(ctx, "broken", tasks, vars); err == nil || got == "" {
		t.Errorf("write failure should return the result and an error: %q, %v", got, err)
	}
	if _, err := a.ExecuteTaskFromConfig(ctx, "ghost", tasks, vars); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing task error = %v", err)
	}

	llm.GenerateFunc = nil
	llm.Err = errors.New("down")
	if _, err := a.ExecuteTaskFromConfig(ctx, "say", tasks, vars); err == nil || !strings.Contains(err.Error(), "failed to execute task say") {
		t.Errorf("run error = %v", err)
	}
}

func TestRunLLMErrors(t *testing.T) {
	ctx := context.Background()
	for _, withTools := range []bool{false, true} {
		opts := []Option{WithLLM(&testutil.FakeLLM{Err: errors.New("down")}), WithRequirePlanApproval(false)}
		if withTools {
			opts = append(opts, WithTools(&testutil.FakeTool{}))
		}
		a, err := NewAgent(opts...)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.Run(ctx, "x"); err == nil || !strings.Contains(err.Error(), "failed to generate response") {
			t.Errorf("tools=%v Run() error = %v", withTools, err)
		}
		if _, err := a.RunDetailed(ctx, "x"); err == nil || !strings.Contains(err.Error(), "failed to generate response") {
			t.Errorf("tools=%v RunDetailed() error = %v", withTools, err)
		}
	}
}

func TestRunWithAuthDetailedLocal(t *testing.T) {
	a, err := NewAgent(WithLLM(&testutil.FakeLLM{DefaultResponse: "local"}))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := a.RunWithAuthDetailed(context.Background(), "x", "token")
	if err != nil || resp.Content != "local" || resp.Metadata["auth_enabled"] != true {
		t.Fatalf("RunWithAuthDetailed() = %+v, %v", resp, err)
	}

	failing, err := NewAgent(WithLLM(&testutil.FakeLLM{Err: errors.New("down")}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failing.RunWithAuthDetailed(context.Background(), "x", "token"); err == nil {
		t.Error("expected an error from the failing llm")
	}
}

func TestNewAgentWithAutoConfigWithoutLLM(t *testing.T) {
	if _, err := NewAgentWithAutoConfig(context.Background()); err == nil {
		t.Error("expected an error without an LLM")
	}
}

func TestSubAgentValidationErrors(t *testing.T) {
	llm := testutil.NewFakeLLM()

	if id := (&Agent{}).getUniqueID(); !strings.HasPrefix(id, "0x") {
		t.Errorf("unnamed id = %q", id)
	}
	parent := &Agent{subAgents: []*Agent{{name: "kid"}}}
	if _, ok := parent.GetSubAgent("ghost"); ok {
		t.Error("GetSubAgent found a missing sub-agent")
	}

	// A sub-agent without an LLM fails component validation.
	if _, err := NewAgent(WithLLM(llm), WithName("p"), WithAgents(&Agent{name: "nollm"})); err == nil || !strings.Contains(err.Error(), "missing required LLM") {
		t.Errorf("missing llm error = %v", err)
	}
	// A sub-agent without a name is rejected.
	if _, err := NewAgent(WithLLM(llm), WithName("p"), WithAgents(&Agent{llm: llm})); err == nil || !strings.Contains(err.Error(), "missing a name") {
		t.Errorf("missing name error = %v", err)
	}

	// A chain deeper than five levels is rejected.
	leaf := &Agent{name: "l0", llm: llm}
	for i := 1; i <= 6; i++ {
		leaf = &Agent{name: "l" + string(rune('0'+i)), llm: llm, subAgents: []*Agent{leaf}}
	}
	if _, err := NewAgent(WithLLM(llm), WithName("root"), WithAgents(leaf)); err == nil || !strings.Contains(err.Error(), "exceeds maximum allowed depth") {
		t.Errorf("depth error = %v", err)
	}
}

func TestConfigureSubAgentToolsSetsTracer(t *testing.T) {
	sub, err := NewAgent(WithLLM(testutil.NewFakeLLM()), WithName("kid"))
	if err != nil {
		t.Fatal(err)
	}
	tracer := &testutil.FakeTracer{}
	a, err := NewAgent(WithLLM(testutil.NewFakeLLM()), WithName("p"), WithTracer(tracer), WithAgents(sub))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.GetTools()[0].(*tools.AgentTool); !ok {
		t.Fatalf("tool type = %T", a.GetTools()[0])
	}
	if a.GetTracer() != tracer {
		t.Error("tracer not set")
	}
}

func TestConvertLLMEventToAgentEvent(t *testing.T) {
	a := &Agent{}
	toolSet := []interfaces.Tool{&testutil.FakeTool{ToolName: "calc", Display: "Calculator", IsInternal: true}}
	call := &interfaces.ToolCall{ID: "1", Name: "calc", Arguments: "{}"}
	boom := errors.New("boom")

	tests := []struct {
		name  string
		in    interfaces.StreamEvent
		check func(interfaces.AgentStreamEvent) bool
	}{
		{"message start", interfaces.StreamEvent{Type: interfaces.StreamEventMessageStart, Content: "a"},
			func(e interfaces.AgentStreamEvent) bool {
				return e.Type == interfaces.AgentEventContent && e.Content == "a"
			}},
		{"content complete", interfaces.StreamEvent{Type: interfaces.StreamEventContentComplete, Content: "b"},
			func(e interfaces.AgentStreamEvent) bool {
				return e.Type == interfaces.AgentEventContent && e.Content == "b"
			}},
		{"thinking", interfaces.StreamEvent{Type: interfaces.StreamEventThinking, Content: "hmm"},
			func(e interfaces.AgentStreamEvent) bool {
				return e.Type == interfaces.AgentEventThinking && e.ThinkingStep == "hmm"
			}},
		{"tool use", interfaces.StreamEvent{Type: interfaces.StreamEventToolUse, ToolCall: call},
			func(e interfaces.AgentStreamEvent) bool {
				return e.Type == interfaces.AgentEventToolCall && e.ToolCall.DisplayName == "Calculator" && e.ToolCall.Internal && e.ToolCall.Status == "received"
			}},
		{"tool use without call", interfaces.StreamEvent{Type: interfaces.StreamEventToolUse},
			func(e interfaces.AgentStreamEvent) bool {
				return e.Type == interfaces.AgentEventToolCall && e.ToolCall == nil
			}},
		{"tool result", interfaces.StreamEvent{Type: interfaces.StreamEventToolResult, ToolCall: call, Content: "42"},
			func(e interfaces.AgentStreamEvent) bool {
				return e.Type == interfaces.AgentEventToolResult && e.ToolCall.Result == "42" && e.ToolCall.Status == "completed"
			}},
		{"unknown tool keeps its name", interfaces.StreamEvent{Type: interfaces.StreamEventToolResult, ToolCall: &interfaces.ToolCall{Name: "other"}},
			func(e interfaces.AgentStreamEvent) bool {
				return e.ToolCall.DisplayName == "other" && !e.ToolCall.Internal
			}},
		{"error", interfaces.StreamEvent{Type: interfaces.StreamEventError, Error: boom},
			func(e interfaces.AgentStreamEvent) bool {
				return e.Type == interfaces.AgentEventError && errors.Is(e.Error, boom)
			}},
		{"message stop", interfaces.StreamEvent{Type: interfaces.StreamEventMessageStop, Content: "c"},
			func(e interfaces.AgentStreamEvent) bool {
				return e.Type == interfaces.AgentEventContent && e.Content == "c"
			}},
		{"unknown type", interfaces.StreamEvent{Type: "mystery", Content: "d"},
			func(e interfaces.AgentStreamEvent) bool {
				return e.Type == interfaces.AgentEventContent && e.Content == "d"
			}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := a.convertLLMEventToAgentEvent(tc.in, toolSet); !tc.check(got) {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestStreamWithToolEventsPersistsToMemory(t *testing.T) {
	call := &interfaces.ToolCall{ID: "call-1", Name: "calc", Arguments: `{"x":1}`}
	llm := newScriptedStreamLLM(
		interfaces.StreamEvent{Type: interfaces.StreamEventContentDelta, Content: "let me check "},
		interfaces.StreamEvent{Type: interfaces.StreamEventToolUse, ToolCall: call},
		interfaces.StreamEvent{Type: interfaces.StreamEventToolResult, ToolCall: call, Content: "2"},
		interfaces.StreamEvent{Type: interfaces.StreamEventError, Error: errors.New("partial failure")},
	)
	mem := &testutil.FakeMemory{}
	tracer := &testutil.FakeTracer{}
	a, err := NewAgent(
		WithLLM(llm),
		WithMemory(mem),
		WithTracer(tracer),
		WithTools(&testutil.FakeTool{ToolName: "calc"}),
		WithRequirePlanApproval(false),
		WithSystemPrompt("sys"),
		WithLLMConfig(interfaces.LLMConfig{TopP: 0.5}),
		WithResponseFormat(interfaces.ResponseFormat{Name: "R"}),
		WithStreamConfig(&interfaces.StreamConfig{BufferSize: 3}),
		WithCacheConfig(interfaces.CacheConfig{CacheSystemMessage: true}),
	)
	if err != nil {
		t.Fatal(err)
	}

	events, err := a.RunStreamWithAuth(context.Background(), "add", "ignored-token")
	if err != nil {
		t.Fatal(err)
	}
	var types []interfaces.AgentEventType
	var complete interfaces.AgentStreamEvent
	for e := range events {
		types = append(types, e.Type)
		if e.Type == interfaces.AgentEventComplete {
			complete = e
		}
	}
	want := []interfaces.AgentEventType{
		interfaces.AgentEventContent, interfaces.AgentEventToolCall, interfaces.AgentEventToolResult,
		interfaces.AgentEventError, interfaces.AgentEventComplete, interfaces.AgentEventError,
	}
	if !slices.Equal(types, want) {
		t.Errorf("event types = %v, want %v", types, want)
	}
	if complete.Metadata["had_error"] != true {
		t.Errorf("completion metadata = %+v", complete.Metadata)
	}

	opts := llm.lastOptions()
	if opts.SystemMessage != "sys" || opts.LLMConfig.TopP != 0.5 || opts.ResponseFormat.Name != "R" ||
		opts.StreamConfig.BufferSize != 3 || !opts.CacheConfig.CacheSystemMessage || opts.Memory != mem || opts.MaxIterations != 2 {
		t.Errorf("stream options = %+v", opts)
	}

	msgs, _ := mem.GetMessages(context.Background())
	if len(msgs) != 3 {
		t.Fatalf("memory = %+v", msgs)
	}
	if msgs[1].Role != "assistant" || len(msgs[1].ToolCalls) != 1 || msgs[1].Content != "let me check " {
		t.Errorf("assistant message = %+v", msgs[1])
	}
	if msgs[2].Role != "tool" || msgs[2].Content != "2" || msgs[2].ToolCallID != "call-1" || msgs[2].Metadata["tool_name"] != "calc" {
		t.Errorf("tool message = %+v", msgs[2])
	}
	if names := tracer.SpanNames(); !slices.Contains(names, "agent.RunStream") {
		t.Errorf("spans = %v", names)
	}
}

func TestStreamFailurePaths(t *testing.T) {
	ctx := context.Background()

	t.Run("llm without streaming", func(t *testing.T) {
		a, err := NewAgent(WithLLM(&mockLLM{}))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.RunStream(ctx, "x"); err == nil || !strings.Contains(err.Error(), "does not support streaming") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("stream fails to start", func(t *testing.T) {
		llm := newScriptedStreamLLM()
		llm.startErr = errors.New("no socket")
		a, err := NewAgent(WithLLM(llm))
		if err != nil {
			t.Fatal(err)
		}
		events, err := a.RunStream(ctx, "x")
		if err != nil {
			t.Fatal(err)
		}
		if _, errs := drainStream(t, events); len(errs) != 1 || !strings.Contains(errs[0].Error(), "failed to start LLM streaming") {
			t.Errorf("errors = %v", errs)
		}
	})

	t.Run("memory write fails before the run", func(t *testing.T) {
		a, err := NewAgent(WithLLM(newScriptedStreamLLM()), WithMemory(&addFailingMemory{err: errors.New("disk full")}))
		if err != nil {
			t.Fatal(err)
		}
		events, err := a.RunStream(ctx, "x")
		if err != nil {
			t.Fatal(err)
		}
		if _, errs := drainStream(t, events); len(errs) != 1 || !strings.Contains(errs[0].Error(), "disk full") {
			t.Errorf("errors = %v", errs)
		}
	})

	t.Run("plan generation fails", func(t *testing.T) {
		llm := newScriptedStreamLLM()
		llm.Err = errors.New("planner down")
		a, err := NewAgent(WithLLM(llm), WithTools(&testutil.FakeTool{}))
		if err != nil {
			t.Fatal(err)
		}
		events, err := a.RunStream(ctx, "x")
		if err != nil {
			t.Fatal(err)
		}
		if _, errs := drainStream(t, events); len(errs) != 1 || !strings.Contains(errs[0].Error(), "failed to generate execution plan") {
			t.Errorf("errors = %v", errs)
		}
	})

	t.Run("output guardrail blocks persistence", func(t *testing.T) {
		mem := &testutil.FakeMemory{}
		llm := newScriptedStreamLLM(interfaces.StreamEvent{Type: interfaces.StreamEventContentDelta, Content: "raw"})
		a, err := NewAgent(WithLLM(llm), WithMemory(mem), WithGuardrails(&blockingOutputGuardrails{err: errors.New("blocked")}))
		if err != nil {
			t.Fatal(err)
		}
		events, err := a.RunStream(ctx, "x")
		if err != nil {
			t.Fatal(err)
		}
		if content, errs := drainStream(t, events); content != "raw" || len(errs) != 0 {
			t.Errorf("stream = %q, %v", content, errs)
		}
		msgs, _ := mem.GetMessages(ctx)
		if len(msgs) != 1 || msgs[0].Role != interfaces.MessageRoleUser {
			t.Errorf("memory = %+v", msgs)
		}
	})
}

func TestStreamRoleQuestion(t *testing.T) {
	llm := newScriptedStreamLLM()
	llm.DefaultResponse = "I am the streamer."
	a, err := NewAgent(WithLLM(llm), WithSystemPrompt("You stream."))
	if err != nil {
		t.Fatal(err)
	}
	events, err := a.RunStream(context.Background(), "What can you do?")
	if err != nil {
		t.Fatal(err)
	}
	if content, errs := drainStream(t, events); content != "I am the streamer." || len(errs) != 0 {
		t.Errorf("stream = %q, %v", content, errs)
	}

	// An output guardrail error on the role answer is reported as an event.
	b, err := NewAgent(WithLLM(llm), WithSystemPrompt("You stream."), WithGuardrails(&blockingOutputGuardrails{err: errors.New("blocked")}))
	if err != nil {
		t.Fatal(err)
	}
	events, err = b.RunStream(context.Background(), "who are you")
	if err != nil {
		t.Fatal(err)
	}
	if _, errs := drainStream(t, events); len(errs) != 1 || !strings.Contains(errs[0].Error(), "blocked") {
		t.Errorf("errors = %v", errs)
	}
}

func TestSendEventRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sendEvent(ctx, make(chan interfaces.AgentStreamEvent), interfaces.AgentStreamEvent{}) {
		t.Error("sendEvent reported success on a cancelled context with no reader")
	}
	ch := make(chan interfaces.AgentStreamEvent, 1)
	if !sendEvent(context.Background(), ch, interfaces.AgentStreamEvent{Content: "x"}) || (<-ch).Content != "x" {
		t.Error("sendEvent did not deliver on a buffered channel")
	}
}
