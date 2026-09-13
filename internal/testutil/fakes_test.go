package testutil

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestFakeLLM_ZeroValueIsUsable(t *testing.T) {
	llm := &FakeLLM{}

	got, err := llm.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got != "" {
		t.Errorf("Generate() = %q, want the empty DefaultResponse", got)
	}
	if llm.Calls() != 1 {
		t.Errorf("Calls() = %d, want 1", llm.Calls())
	}
	if name := llm.Name(); name != "fake" {
		t.Errorf("Name() = %q, want %q", name, "fake")
	}
}

func TestFakeLLM_ResponsesAreConsumedInOrder(t *testing.T) {
	llm := &FakeLLM{
		Responses:       []string{"first", "second"},
		DefaultResponse: "fallback",
	}
	ctx := context.Background()

	for _, want := range []string{"first", "second", "fallback", "fallback"} {
		got, err := llm.Generate(ctx, "prompt")
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if got != want {
			t.Errorf("Generate() = %q, want %q", got, want)
		}
	}
}

func TestFakeLLM_RecordsPromptsAndTools(t *testing.T) {
	llm := &FakeLLM{DefaultResponse: "ok"}
	tool := &FakeTool{ToolName: "search"}

	if _, err := llm.GenerateWithTools(context.Background(), "find things", []interfaces.Tool{tool}); err != nil {
		t.Fatalf("GenerateWithTools() error = %v", err)
	}

	prompts := llm.Prompts()
	if len(prompts) != 1 || prompts[0] != "find things" {
		t.Errorf("Prompts() = %v, want [find things]", prompts)
	}

	names := llm.ToolNamesAt(0)
	if len(names) != 1 || names[0] != "search" {
		t.Errorf("ToolNamesAt(0) = %v, want [search]", names)
	}
}

func TestFakeLLM_InvokeToolsDrivesTheToolSet(t *testing.T) {
	llm := &FakeLLM{DefaultResponse: "done", InvokeTools: true}
	tool := &FakeTool{ToolName: "search", Result: "results"}

	if _, err := llm.GenerateWithTools(context.Background(), "go", []interfaces.Tool{tool}); err != nil {
		t.Fatalf("GenerateWithTools() error = %v", err)
	}

	if got := tool.CallCount(); got != 1 {
		t.Errorf("tool call count = %d, want 1", got)
	}
}

func TestFakeLLM_ErrPropagates(t *testing.T) {
	sentinel := errors.New("upstream is down")
	llm := &FakeLLM{Err: sentinel}

	if _, err := llm.Generate(context.Background(), "x"); !errors.Is(err, sentinel) {
		t.Errorf("Generate() error = %v, want %v", err, sentinel)
	}
	if _, err := llm.GenerateDetailed(context.Background(), "x"); !errors.Is(err, sentinel) {
		t.Errorf("GenerateDetailed() error = %v, want %v", err, sentinel)
	}
}

func TestFakeLLM_DetailedCarriesUsageAndModel(t *testing.T) {
	llm := &FakeLLM{
		ModelName:       "test-model",
		DefaultResponse: "hi",
		Usage:           &interfaces.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}

	resp, err := llm.GenerateDetailed(context.Background(), "x")
	if err != nil {
		t.Fatalf("GenerateDetailed() error = %v", err)
	}
	if resp.Model != "test-model" {
		t.Errorf("Model = %q, want %q", resp.Model, "test-model")
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 15 {
		t.Errorf("Usage = %+v, want TotalTokens 15", resp.Usage)
	}
}

// TestFakeLLM_SafeForConcurrentUse is the property that made this package worth
// writing: none of the dozen hand-rolled mocks it replaces were safe to call
// from multiple goroutines, so a concurrency test using one could not tell a
// race in the code under test from a race in its own double.
func TestFakeLLM_SafeForConcurrentUse(t *testing.T) {
	llm := &FakeLLM{DefaultResponse: "ok"}
	tool := &FakeTool{ToolName: "t", Result: "r"}
	ctx := context.Background()

	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = llm.Generate(ctx, "p")
			_, _ = llm.GenerateWithTools(ctx, "p", []interfaces.Tool{tool})
			_, _ = tool.Execute(ctx, "{}")
			_ = llm.Prompts()
			_ = tool.CallCount()
		}()
	}

	wg.Wait()

	if got := llm.Calls(); got != goroutines*2 {
		t.Errorf("Calls() = %d, want %d", got, goroutines*2)
	}
}

func TestFakeTool_ForwardsOptionalInterfaces(t *testing.T) {
	tool := &FakeTool{ToolName: "raw", Display: "Pretty Name", IsInternal: true}

	var asTool interfaces.Tool = tool

	named, ok := asTool.(interfaces.ToolWithDisplayName)
	if !ok {
		t.Fatal("FakeTool does not implement ToolWithDisplayName")
	}
	if got := named.DisplayName(); got != "Pretty Name" {
		t.Errorf("DisplayName() = %q, want %q", got, "Pretty Name")
	}

	internal, ok := asTool.(interfaces.InternalTool)
	if !ok {
		t.Fatal("FakeTool does not implement InternalTool")
	}
	if !internal.Internal() {
		t.Error("Internal() = false, want true")
	}
}

func TestFakeMemory_RoundTrips(t *testing.T) {
	mem := &FakeMemory{}
	ctx := context.Background()

	if err := mem.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "hi"}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}

	msgs, err := mem.GetMessages(ctx)
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "hi" {
		t.Errorf("GetMessages() = %v, want one message %q", msgs, "hi")
	}

	if err := mem.Clear(ctx); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if msgs, _ := mem.GetMessages(ctx); len(msgs) != 0 {
		t.Errorf("after Clear, GetMessages() = %v, want empty", msgs)
	}
}

func TestFakeConversationMemory_ReportsCapability(t *testing.T) {
	mem := &FakeConversationMemory{Conversations: []string{"a", "b"}}

	convMem, ok := interfaces.AsConversationMemory(mem)
	if !ok {
		t.Fatal("FakeConversationMemory does not satisfy ConversationMemory")
	}

	got, err := convMem.GetAllConversations(context.Background())
	if err != nil {
		t.Fatalf("GetAllConversations() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("GetAllConversations() = %v, want 2 entries", got)
	}
}

func TestFakeTracer_RecordsSpans(t *testing.T) {
	tracer := &FakeTracer{}

	_, span := tracer.StartSpan(context.Background(), "llm.generate")
	span.SetAttribute("model", "test-model")
	span.RecordError(errors.New("boom"))
	span.End()

	if names := tracer.SpanNames(); len(names) != 1 || names[0] != "llm.generate" {
		t.Errorf("SpanNames() = %v, want [llm.generate]", names)
	}

	fake, ok := span.(*FakeSpan)
	if !ok {
		t.Fatalf("span is %T, want *FakeSpan", span)
	}
	if v, ok := fake.Attribute("model"); !ok || v != "test-model" {
		t.Errorf("Attribute(model) = %v, %v; want test-model, true", v, ok)
	}
	if len(fake.Errors()) != 1 {
		t.Errorf("Errors() = %v, want one error", fake.Errors())
	}
	if !fake.Ended() {
		t.Error("Ended() = false, want true")
	}
}
