package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

const streamPlanJSON = `{"description": "look", "steps": [{"toolName": "stub", "description": "use the stub", "input": "x"}]}`

// addFailingMemory is a memory whose writes fail.
type addFailingMemory struct {
	testutil.FakeMemory
	err error
}

func (m *addFailingMemory) AddMessage(context.Context, interfaces.Message) error { return m.err }

func newContentAgent(t *testing.T, options ...Option) *Agent {
	t.Helper()
	a, err := NewAgent(append([]Option{WithLLM(testutil.NewFakeLLM()), WithName("content")}, options...)...)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	return a
}

func drainStream(t *testing.T, events <-chan interfaces.AgentStreamEvent) (string, []error) {
	t.Helper()
	var content strings.Builder
	var errs []error
	for event := range events {
		if event.Error != nil {
			errs = append(errs, event.Error)
		}
		content.WriteString(event.Content)
	}
	return content.String(), errs
}

func TestCheckRunInput(t *testing.T) {
	tests := []struct {
		name    string
		content *interfaces.Content
		wantErr string
	}{
		{"nil", nil, "content is nil"},
		{"empty role", interfaces.NewContentFromText("hi", ""), ""},
		{"user role", interfaces.NewContentFromText("hi", interfaces.MessageRoleUser), ""},
		{"no parts", &interfaces.Content{}, ""},
		{"assistant role", interfaces.NewContentFromText("hi", interfaces.MessageRoleAssistant), `got role "assistant"`},
		{"system role", interfaces.NewContentFromText("hi", interfaces.MessageRoleSystem), `got role "system"`},
		{"invalid part", interfaces.NewContentFromParts([]*interfaces.Part{nil}, ""), "invalid run input: part 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkRunInput(tt.content)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("checkRunInput() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("checkRunInput() = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestTextOnly(t *testing.T) {
	tests := []struct {
		name        string
		content     *interfaces.Content
		want        string
		unsupported bool
		wantErr     bool
	}{
		{"text", interfaces.NewContentFromParts([]*interfaces.Part{interfaces.NewPartFromText("a"), interfaces.NewPartFromText("b")}, ""), "a\nb", false, false},
		{"media", imageContent("hi"), "", true, true},
		{"nil", nil, "", false, true},
		{"wrong role", interfaces.NewContentFromText("hi", interfaces.MessageRoleAssistant), "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := textOnly(tt.content, "the path")
			if (err != nil) != tt.wantErr {
				t.Fatalf("textOnly() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("textOnly() = %q, want %q", got, tt.want)
			}
			if errors.Is(err, interfaces.ErrContentUnsupported) != tt.unsupported {
				t.Errorf("textOnly() error = %v, want ErrContentUnsupported %v", err, tt.unsupported)
			}
			if tt.unsupported && !strings.Contains(err.Error(), "the path takes text only") {
				t.Errorf("textOnly() error = %v, want it to name the path", err)
			}
		})
	}
}

func TestUserMessage(t *testing.T) {
	text := userMessage(interfaces.NewContentFromText("hi", ""))
	if text.Role != interfaces.MessageRoleUser || text.Content != "hi" || text.Parts != nil {
		t.Errorf("userMessage(text) = %+v, want a plain user message", text)
	}

	content := imageContent("look")
	media := userMessage(content)
	if media.Content != "look" || len(media.Parts) != 2 || media.Parts[0] != content.Parts[0] {
		t.Errorf("userMessage(media) = %+v, want the text and the parts", media)
	}
}

func TestPartOptions(t *testing.T) {
	if got := partOptions(interfaces.NewContentFromText("hi", "")); got != nil {
		t.Errorf("partOptions(text) = %d options, want nil", len(got))
	}

	content := imageContent("look")
	got := partOptions(content)
	if len(got) != 1 {
		t.Fatalf("partOptions(media) = %d options, want 1", len(got))
	}
	resolved := &interfaces.GenerateOptions{}
	got[0](resolved)
	if len(resolved.Parts) != 2 || resolved.Parts[0] != content.Parts[0] || resolved.Parts[1] != content.Parts[1] {
		t.Errorf("partOptions(media) set parts %+v, want all content parts", resolved.Parts)
	}
}

func TestMediaParts(t *testing.T) {
	image := interfaces.NewPartFromBytes([]byte{1}, "image/png")
	pdf := interfaces.NewPartFromURI("https://example.com/a.pdf", "application/pdf")
	tests := []struct {
		name  string
		parts []*interfaces.Part
		want  []*interfaces.Part
	}{
		{"none", nil, nil},
		{"text only", []*interfaces.Part{interfaces.NewPartFromText("a")}, nil},
		{"mixed", []*interfaces.Part{interfaces.NewPartFromText("a"), image, interfaces.NewPartFromText("b"), pdf}, []*interfaces.Part{image, pdf}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mediaParts(interfaces.NewContentFromParts(tt.parts, ""))
			if len(got) != len(tt.want) {
				t.Fatalf("mediaParts() = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("mediaParts()[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGuardContent(t *testing.T) {
	content := interfaces.NewContentFromParts([]*interfaces.Part{
		interfaces.NewPartFromBytes([]byte{1}, "image/png"),
		interfaces.NewPartFromText("SECRET one"),
		interfaces.NewPartFromText("two"),
	}, "")

	t.Run("no guardrails", func(t *testing.T) {
		got, err := newContentAgent(t).guardContent(context.Background(), content)
		if err != nil {
			t.Fatalf("guardContent() error = %v", err)
		}
		if got.Role != interfaces.MessageRoleUser {
			t.Errorf("Role = %q, want user", got.Role)
		}
		for i := range content.Parts {
			if got.Parts[i] != content.Parts[i] {
				t.Errorf("part %d was replaced without guardrails", i)
			}
		}
	})

	t.Run("redacts text parts only", func(t *testing.T) {
		got, err := newContentAgent(t, WithGuardrails(&redactingGuardrails{})).guardContent(context.Background(), content)
		if err != nil {
			t.Fatalf("guardContent() error = %v", err)
		}
		if got.Parts[0] != content.Parts[0] {
			t.Error("media part was replaced, want it kept as given")
		}
		if got.Parts[1].Text != "[REDACTED] one" || got.Parts[2].Text != "two" {
			t.Errorf("text parts = %q, %q, want the guarded text", got.Parts[1].Text, got.Parts[2].Text)
		}
		if content.Parts[1].Text != "SECRET one" {
			t.Errorf("caller's part changed to %q", content.Parts[1].Text)
		}
	})

	t.Run("guardrail error", func(t *testing.T) {
		sentinel := errors.New("blocked")
		_, err := newContentAgent(t, WithGuardrails(&redactingGuardrails{inputErr: sentinel})).guardContent(context.Background(), content)
		if !errors.Is(err, sentinel) {
			t.Errorf("guardContent() error = %v, want it to wrap the guardrail error", err)
		}
	})
}

func TestBeginRunRejectsInvalidInput(t *testing.T) {
	mem := memory.NewConversationBuffer()
	a := newContentAgent(t, WithMemory(mem))
	if _, _, err := a.beginRun(preambleCtx(), nil); err == nil {
		t.Fatal("beginRun() accepted nil content")
	}
	if msgs, _ := mem.GetMessages(preambleCtx()); len(msgs) != 0 {
		t.Errorf("memory holds %d messages after invalid input, want 0", len(msgs))
	}
}

func TestBeginRunMemoryWriteFailure(t *testing.T) {
	sentinel := errors.New("disk full")
	a := newContentAgent(t, WithMemory(&addFailingMemory{err: sentinel}))
	_, got, err := a.beginRun(preambleCtx(), textContent("hi"))
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "failed to add user message to memory") {
		t.Errorf("beginRun() error = %v, want the wrapped memory error", err)
	}
	if got != nil {
		t.Errorf("beginRun() content = %+v, want nil on error", got)
	}
}

func TestRunWithContentMemoryWriteFailure(t *testing.T) {
	sentinel := errors.New("disk full")
	llm := &testutil.FakeLLM{DefaultResponse: "ok", Streaming: true}
	a := newContentAgent(t, WithLLM(llm), WithMemory(&addFailingMemory{err: sentinel}))

	if _, err := a.RunWithContent(preambleCtx(), imageContent("hi")); !errors.Is(err, sentinel) {
		t.Errorf("RunWithContent() error = %v, want the memory error", err)
	}

	events, err := a.RunWithContentStream(preambleCtx(), imageContent("hi"))
	if err != nil {
		t.Fatalf("RunWithContentStream() error = %v", err)
	}
	if _, errs := drainStream(t, events); len(errs) != 1 || !errors.Is(errs[0], sentinel) {
		t.Errorf("stream errors = %v, want one memory error event", errs)
	}
	if llm.Calls() != 0 {
		t.Errorf("LLM called %d times after the memory write failed, want 0", llm.Calls())
	}
}

func TestRunWithContentDetailed(t *testing.T) {
	a := newContentAgent(t, WithLLM(&testutil.FakeLLM{DefaultResponse: "a cat"}))

	got, err := a.RunWithContentDetailed(preambleCtx(), imageContent("what is this?"))
	if err != nil {
		t.Fatalf("RunWithContentDetailed() error = %v", err)
	}
	if got == nil || got.Content != "a cat" {
		t.Errorf("RunWithContentDetailed() = %+v, want content %q", got, "a cat")
	}

	if _, err := a.RunWithContentDetailed(preambleCtx(), nil); err == nil {
		t.Error("RunWithContentDetailed() accepted nil content")
	}
}

func TestCustomRunFunctionRejectsInvalidContent(t *testing.T) {
	called := false
	a := newContentAgent(t, WithCustomRunFunction(func(context.Context, string, *Agent) (string, error) {
		called = true
		return "", nil
	}))
	_, err := a.RunWithContent(preambleCtx(), nil)
	if err == nil || errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("RunWithContent(nil) error = %v, want an input error", err)
	}
	if called {
		t.Error("custom run function was called with invalid input")
	}
}

// A remote agent sends a string to another service, so it would silently
// drop media. Both paths refuse before any network call.
func TestRemoteAgentRefusesMedia(t *testing.T) {
	a := newContentAgent(t)
	a.isRemote = true

	if _, err := a.RunWithContent(preambleCtx(), imageContent("hi")); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("RunWithContent() error = %v, want ErrContentUnsupported", err)
	}
	if _, err := a.RunWithContentStream(preambleCtx(), imageContent("hi")); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("RunWithContentStream() error = %v, want ErrContentUnsupported", err)
	}
	if _, err := a.RunWithContentStream(preambleCtx(), nil); err == nil {
		t.Error("RunWithContentStream() accepted nil content")
	}
}

func TestCustomStreamFunctionGetsText(t *testing.T) {
	var got string
	a := newContentAgent(t, WithCustomRunStreamFunction(func(_ context.Context, input string, _ *Agent) (<-chan interfaces.AgentStreamEvent, error) {
		got = input
		ch := make(chan interfaces.AgentStreamEvent)
		close(ch)
		return ch, nil
	}))

	if _, err := a.RunStream(preambleCtx(), "hello"); err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	if got != "hello" {
		t.Errorf("custom stream function got %q, want %q", got, "hello")
	}
}

func TestRunWithAuthLocal(t *testing.T) {
	a := newContentAgent(t, WithLLM(&testutil.FakeLLM{DefaultResponse: "local answer"}))

	got, err := a.RunWithAuth(preambleCtx(), "hi", "token")
	if err != nil {
		t.Fatalf("RunWithAuth() error = %v", err)
	}
	if got != "local answer" {
		t.Errorf("RunWithAuth() = %q, want %q", got, "local answer")
	}
}

func TestRunWithContentStreamWithTracer(t *testing.T) {
	tracer := &testutil.FakeTracer{}
	llm := &testutil.FakeLLM{DefaultResponse: "traced", Streaming: true}
	a := newContentAgent(t, WithLLM(llm), WithTracer(tracer))

	events, err := a.RunWithContentStream(preambleCtx(), imageContent("what is this?"))
	if err != nil {
		t.Fatalf("RunWithContentStream() error = %v", err)
	}
	if _, errs := drainStream(t, events); len(errs) != 0 {
		t.Fatalf("stream errors = %v", errs)
	}

	names := tracer.SpanNames()
	if len(names) == 0 || names[0] != "agent.RunStream" {
		t.Fatalf("span names = %v, want agent.RunStream first", names)
	}
	if !tracer.Spans()[0].Ended() {
		t.Error("the agent.RunStream span was not ended")
	}
}

// With tools and plan approval, the streaming path falls back to the
// execution plan, and the planner must see the input's media.
func TestRunWithContentStreamPlanSeesMedia(t *testing.T) {
	llm := &partsRecordingLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: streamPlanJSON, Streaming: true}}
	a := newContentAgent(t, WithLLM(llm), WithTools(&testutil.FakeTool{ToolName: "stub"}))

	content := imageContent("what is this?")
	events, err := a.RunWithContentStream(preambleCtx(), content)
	if err != nil {
		t.Fatalf("RunWithContentStream() error = %v", err)
	}
	text, errs := drainStream(t, events)
	if len(errs) != 0 {
		t.Fatalf("stream errors = %v", errs)
	}
	if !strings.Contains(text, "execution plan") {
		t.Errorf("stream content = %q, want the plan for approval", text)
	}
	if len(llm.seen) != 1 {
		t.Fatalf("planner called %d times, want 1", len(llm.seen))
	}
	if parts := llm.seen[0]; len(parts) != 2 || parts[0] != content.Parts[0] || !parts[1].IsText() {
		t.Errorf("planner parts = %+v, want the image then the planning prompt", parts)
	}
}

func TestRunWithContentPlanSeesMedia(t *testing.T) {
	llm := &partsRecordingLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: streamPlanJSON}}
	a := newContentAgent(t, WithLLM(llm), WithTools(&testutil.FakeTool{ToolName: "stub"}))

	content := imageContent("what is this?")
	got, err := a.RunWithContent(preambleCtx(), content)
	if err != nil {
		t.Fatalf("RunWithContent() error = %v", err)
	}
	if !strings.Contains(got, "execution plan") {
		t.Errorf("RunWithContent() = %q, want the plan for approval", got)
	}
	if len(llm.seen) != 1 || len(llm.seen[0]) != 2 || llm.seen[0][0] != content.Parts[0] {
		t.Errorf("planner parts = %+v, want the image then the planning prompt", llm.seen)
	}
}

func TestRunWithContentStreamPlanError(t *testing.T) {
	llm := &testutil.FakeLLM{DefaultResponse: "no plan here", Streaming: true}
	a := newContentAgent(t, WithLLM(llm), WithTools(&testutil.FakeTool{ToolName: "stub"}))

	events, err := a.RunWithContentStream(preambleCtx(), imageContent("what is this?"))
	if err != nil {
		t.Fatalf("RunWithContentStream() error = %v", err)
	}
	if _, errs := drainStream(t, events); len(errs) != 1 || !strings.Contains(errs[0].Error(), "failed to generate execution plan") {
		t.Errorf("stream errors = %v, want one plan error event", errs)
	}
}

func TestFinishRunMemoryWriteFailure(t *testing.T) {
	sentinel := errors.New("disk full")
	a := newContentAgent(t, WithMemory(&addFailingMemory{err: sentinel}))
	if _, err := a.finishRun(preambleCtx(), "answer"); !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "failed to add agent message to memory") {
		t.Errorf("finishRun() error = %v, want the wrapped memory error", err)
	}
}
