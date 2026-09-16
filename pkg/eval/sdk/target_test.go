package sdk

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	coreeval "github.com/Ingenimax/agent-sdk-go/pkg/eval"
	"github.com/Ingenimax/agent-sdk-go/pkg/hooks"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestOptionsCaptureAttemptAndModifiedExecution(t *testing.T) {
	recorder := coreeval.NewRecorder(coreeval.RecorderOptions{})
	registry := hooks.NewRegistry(hooks.Plugin{
		Name: "normalize-city",
		BeforeTool: func(context.Context, hooks.ToolCall) (hooks.Outcome, error) {
			return hooks.Outcome{Decision: hooks.Modify, Arguments: `{"city":"Madrid"}`}, nil
		},
		AfterTool: func(_ context.Context, _ hooks.ToolCall, result string) (string, error) {
			return result + "C", nil
		},
	})
	tool := &testutil.FakeTool{ToolName: "weather", Result: "22"}
	llm := &testutil.FakeLLM{InvokeTools: true, DefaultResponse: "done"}
	options := []agent.Option{
		agent.WithLLM(llm),
		agent.WithName("root"),
		agent.WithTools(tool),
		agent.WithRequirePlanApproval(false),
		agent.WithDisableFinalSummary(true),
	}
	options = append(options, Options(recorder, coreeval.RootAgentPath, registry.Decorate("root"))...)
	subject, err := agent.NewAgent(options...)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	if _, err := subject.RunDetailed(context.Background(), "weather"); err != nil {
		t.Fatalf("RunDetailed: %v", err)
	}

	trace := recorder.Snapshot()
	if len(trace.Spans) != 2 {
		t.Fatalf("got %d spans: %#v", len(trace.Spans), trace.Spans)
	}
	attempt, execution := trace.Spans[0], trace.Spans[1]
	if attempt.Layer != coreeval.ToolLayerAttempt || attempt.Arguments != `{}` || attempt.Result != "22C" {
		t.Fatalf("attempt span = %#v", attempt)
	}
	if execution.Layer != coreeval.ToolLayerExecution || execution.Arguments != `{"city":"Madrid"}` || execution.Result != "22" {
		t.Fatalf("execution span = %#v", execution)
	}
	if execution.ParentID != attempt.ID {
		t.Fatalf("execution parent = %q, want %q", execution.ParentID, attempt.ID)
	}
	if calls := tool.Calls(); len(calls) != 1 || calls[0] != `{"city":"Madrid"}` {
		t.Fatalf("tool calls = %#v", calls)
	}
}

func TestOptionsMarkDeniedAttemptWithoutExecution(t *testing.T) {
	recorder := coreeval.NewRecorder(coreeval.RecorderOptions{})
	registry := hooks.NewRegistry(hooks.Plugin{
		Name: "deny",
		BeforeTool: func(context.Context, hooks.ToolCall) (hooks.Outcome, error) {
			return hooks.Outcome{Decision: hooks.Deny, Message: "denied"}, nil
		},
	})
	tool := &testutil.FakeTool{ToolName: "deploy", Result: "deployed"}
	llm := &testutil.FakeLLM{InvokeTools: true, DefaultResponse: "done"}
	options := []agent.Option{
		agent.WithLLM(llm),
		agent.WithName("root"),
		agent.WithTools(tool),
		agent.WithRequirePlanApproval(false),
		agent.WithDisableFinalSummary(true),
	}
	options = append(options, Options(recorder, coreeval.RootAgentPath, registry.Decorate("root"))...)
	subject, err := agent.NewAgent(options...)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	response, err := subject.RunDetailed(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("RunDetailed: %v", err)
	}
	if tool.CallCount() != 0 || response.ExecutionSummary.ToolCalls != 0 {
		t.Fatalf("denied tool executed: calls=%d summary=%d", tool.CallCount(), response.ExecutionSummary.ToolCalls)
	}
	trace := recorder.Snapshot()
	if len(trace.Spans) != 1 || trace.Spans[0].Status != coreeval.ToolSpanShortCircuited {
		t.Fatalf("trace = %#v", trace)
	}
}

func TestDecoratorRecordsAndRepanics(t *testing.T) {
	recorder := coreeval.NewRecorder(coreeval.RecorderOptions{})
	tool := &testutil.FakeTool{
		ToolName: "panic",
		ExecuteFunc: func(context.Context, string) (string, error) {
			panic("boom")
		},
	}
	wrapped := Decorator(recorder, coreeval.RootAgentPath, coreeval.ToolLayerExecution)([]interfaces.Tool{tool})[0]
	recovered := func() (value any) {
		defer func() { value = recover() }()
		_, _ = wrapped.Execute(context.Background(), `{}`)
		return nil
	}()
	if recovered != "boom" {
		t.Fatalf("recovered = %#v", recovered)
	}
	trace := recorder.Snapshot()
	if len(trace.Spans) != 1 || trace.Spans[0].Status != coreeval.ToolSpanPanicked || trace.Spans[0].Error != "boom" {
		t.Fatalf("trace = %#v", trace)
	}
}

func TestDecoratorForwardsToolContractAndRecordsRunErrors(t *testing.T) {
	recorder := coreeval.NewRecorder(coreeval.RecorderOptions{})
	parameters := map[string]interfaces.ParameterSpec{
		"city": {Type: "string", Required: true},
	}
	toolErr := errors.New("weather unavailable")
	tool := &testutil.FakeTool{
		ToolName:   "weather",
		Desc:       "fetch weather",
		Params:     parameters,
		Display:    "Weather",
		IsInternal: true,
		Err:        toolErr,
	}
	wrapped := Decorator(recorder, "", coreeval.ToolLayerExecution)([]interfaces.Tool{tool})[0]
	if wrapped.Name() != tool.Name() || wrapped.Description() != tool.Description() {
		t.Fatalf("metadata was not forwarded: %q, %q", wrapped.Name(), wrapped.Description())
	}
	if !reflect.DeepEqual(wrapped.Parameters(), parameters) {
		t.Fatalf("parameters = %#v", wrapped.Parameters())
	}
	if named := wrapped.(interfaces.ToolWithDisplayName).DisplayName(); named != "Weather" {
		t.Fatalf("DisplayName = %q", named)
	}
	if !wrapped.(interfaces.InternalTool).Internal() {
		t.Fatal("Internal = false")
	}
	if unwrapped := wrapped.(agent.ToolUnwrapper).Unwrap(); unwrapped != tool {
		t.Fatalf("Unwrap = %#v", unwrapped)
	}
	if _, err := wrapped.Run(context.Background(), `{"city":"Madrid"}`); !errors.Is(err, toolErr) {
		t.Fatalf("Run error = %v", err)
	}
	trace := recorder.Snapshot()
	if len(trace.Spans) != 1 || trace.Spans[0].Status != coreeval.ToolSpanError || trace.Spans[0].Method != "Run" {
		t.Fatalf("trace = %#v", trace)
	}
	if trace.Spans[0].AgentPath != coreeval.RootAgentPath {
		t.Fatalf("agent path = %q", trace.Spans[0].AgentPath)
	}

	plain := &plainTool{}
	plainWrapped := Decorator(recorder, "child", coreeval.ToolLayerAttempt)([]interfaces.Tool{plain})[0]
	if plainWrapped.(interfaces.ToolWithDisplayName).DisplayName() != plain.Name() {
		t.Fatal("display-name fallback did not use Name")
	}
	if plainWrapped.(interfaces.InternalTool).Internal() {
		t.Fatal("plain tool was marked internal")
	}
}

func TestDecoratorNoOpAndNewTargetCapabilities(t *testing.T) {
	tool := &testutil.FakeTool{ToolName: "tool"}
	toolSet := []interfaces.Tool{tool}
	decorated := Decorator(nil, "", coreeval.ToolLayerExecution)(toolSet)
	if len(decorated) != 1 || decorated[0] != tool {
		t.Fatalf("nil-recorder decoration changed tools: %#v", decorated)
	}
	if got := Decorator(coreeval.NewRecorder(coreeval.RecorderOptions{}), "", coreeval.ToolLayerExecution)(nil); got != nil {
		t.Fatalf("empty decoration = %#v", got)
	}

	closed := false
	closeTarget := func(context.Context) error {
		closed = true
		return nil
	}
	subject, err := agent.NewAgent(agent.WithLLM(&testutil.FakeLLM{DefaultResponse: "done"}))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	target := NewTarget(subject, closeTarget)
	if target.Subject != subject || target.Close == nil {
		t.Fatalf("target = %#v", target)
	}
	if target.Capabilities.ToolCapture != coreeval.CoverageComplete ||
		target.Capabilities.Usage != coreeval.CoverageComplete ||
		target.Capabilities.Output != coreeval.CoverageComplete {
		t.Fatalf("capabilities = %#v", target.Capabilities)
	}
	if err := target.Close(context.Background()); err != nil || !closed {
		t.Fatalf("close: closed=%v err=%v", closed, err)
	}
}

type plainTool struct{}

func (*plainTool) Name() string                                    { return "plain" }
func (*plainTool) Description() string                             { return "plain tool" }
func (*plainTool) Parameters() map[string]interfaces.ParameterSpec { return nil }
func (*plainTool) Run(context.Context, string) (string, error)     { return "run", nil }
func (*plainTool) Execute(context.Context, string) (string, error) { return "execute", nil }
