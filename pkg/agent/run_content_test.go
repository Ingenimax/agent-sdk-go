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

func imageContent(text string) *interfaces.Content {
	return interfaces.NewContentFromParts([]*interfaces.Part{
		interfaces.NewPartFromBytes([]byte{0x89, 'P', 'N', 'G'}, "image/png"),
		interfaces.NewPartFromText(text),
	}, interfaces.MessageRoleUser)
}

// partsRecordingLLM records the parts each call's options carry.
type partsRecordingLLM struct {
	testutil.FakeLLM
	seen [][]*interfaces.Part
}

func (m *partsRecordingLLM) record(options []interfaces.GenerateOption) {
	resolved := &interfaces.GenerateOptions{}
	for _, option := range options {
		option(resolved)
	}
	m.seen = append(m.seen, resolved.Parts)
}

func (m *partsRecordingLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	m.record(options)
	return m.FakeLLM.Generate(ctx, prompt)
}

func (m *partsRecordingLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	m.record(options)
	return m.FakeLLM.GenerateWithTools(ctx, prompt, tools)
}

func assertStoredParts(t *testing.T, mem interfaces.Memory) {
	t.Helper()
	msgs, err := mem.GetMessages(preambleCtx())
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}
	if len(msgs) == 0 || msgs[0].Role != interfaces.MessageRoleUser {
		t.Fatalf("memory = %+v, want the user message first", msgs)
	}
	if len(msgs[0].Parts) != 2 || msgs[0].Parts[0].MIMEType() != "image/png" {
		t.Errorf("stored parts = %+v, want the image then the text", msgs[0].Parts)
	}
	if msgs[0].Content != "what is this?" {
		t.Errorf("stored content = %q, want the text part", msgs[0].Content)
	}
}

func TestRunWithContentStoresPartsInMemory(t *testing.T) {
	mem := memory.NewConversationBuffer()
	a, err := NewAgent(WithLLM(testutil.NewFakeLLM()), WithName("vision"), WithMemory(mem))
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if _, err := a.RunWithContent(preambleCtx(), imageContent("what is this?")); err != nil {
		t.Fatalf("RunWithContent() error = %v", err)
	}
	assertStoredParts(t, mem)
}

func TestRunWithContentStreamStoresPartsInMemory(t *testing.T) {
	mem := memory.NewConversationBuffer()
	llm := &testutil.FakeLLM{DefaultResponse: "ok", Streaming: true}
	a, err := NewAgent(WithLLM(llm), WithName("vision"), WithMemory(mem))
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	events, err := a.RunWithContentStream(preambleCtx(), imageContent("what is this?"))
	if err != nil {
		t.Fatalf("RunWithContentStream() error = %v", err)
	}
	for event := range events {
		if event.Error != nil {
			t.Fatalf("stream error = %v", event.Error)
		}
	}
	assertStoredParts(t, mem)
}

func TestRunStoresTextWithoutParts(t *testing.T) {
	mem := memory.NewConversationBuffer()
	a, err := NewAgent(WithLLM(testutil.NewFakeLLM()), WithName("text"), WithMemory(mem))
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if _, err := a.Run(preambleCtx(), "hello"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	msgs, _ := mem.GetMessages(preambleCtx())
	if len(msgs) == 0 || msgs[0].Content != "hello" || msgs[0].Parts != nil {
		t.Errorf("memory = %+v, want a plain text user message", msgs)
	}
}

func TestRunWithContentWithoutMemoryPassesParts(t *testing.T) {
	llm := &partsRecordingLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: "ok"}}
	a, err := NewAgent(WithLLM(llm), WithName("vision"))
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if _, err := a.RunWithContent(preambleCtx(), imageContent("what is this?")); err != nil {
		t.Fatalf("RunWithContent() error = %v", err)
	}
	if len(llm.seen) != 1 || len(llm.seen[0]) != 2 {
		t.Fatalf("LLM saw parts %+v, want one call with both parts", llm.seen)
	}
}

func TestRunWithContentGuardsTextParts(t *testing.T) {
	mem := memory.NewConversationBuffer()
	a, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("vision"),
		WithMemory(mem),
		WithGuardrails(&redactingGuardrails{}),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	input := imageContent("my password is SECRET")
	if _, err := a.RunWithContent(preambleCtx(), input); err != nil {
		t.Fatalf("RunWithContent() error = %v", err)
	}
	msgs, _ := mem.GetMessages(preambleCtx())
	if len(msgs) == 0 || len(msgs[0].Parts) != 2 {
		t.Fatalf("memory = %+v, want the stored parts", msgs)
	}
	if strings.Contains(msgs[0].Parts[1].Text, "SECRET") || strings.Contains(msgs[0].Content, "SECRET") {
		t.Errorf("stored message = %+v, want the text guarded", msgs[0])
	}
	if input.Parts[1].Text != "my password is SECRET" {
		t.Errorf("caller's content changed to %q", input.Parts[1].Text)
	}
}

func TestRunWithContentRejectsInvalidInput(t *testing.T) {
	a, err := NewAgent(WithLLM(testutil.NewFakeLLM()), WithName("vision"))
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	notUser := imageContent("hi")
	notUser.Role = interfaces.MessageRoleAssistant
	if _, err := a.RunWithContent(preambleCtx(), notUser); err == nil {
		t.Error("RunWithContent() accepted assistant content as input")
	}

	noType := interfaces.NewContentFromParts([]*interfaces.Part{{InlineData: &interfaces.Blob{Data: []byte{1}}}}, "")
	if _, err := a.RunWithContent(preambleCtx(), noType); err == nil {
		t.Error("RunWithContent() accepted inline data without a MIME type")
	}

	if _, err := a.RunWithContentStream(preambleCtx(), nil); err == nil {
		t.Error("RunWithContentStream() accepted nil content")
	}
}

// A custom run function takes a string, so it would silently drop media.
func TestCustomRunFunctionsRefuseMedia(t *testing.T) {
	a, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithName("custom"),
		WithCustomRunFunction(func(_ context.Context, input string, _ *Agent) (string, error) { return input, nil }),
		WithCustomRunStreamFunction(func(context.Context, string, *Agent) (<-chan interfaces.AgentStreamEvent, error) {
			return nil, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if _, err := a.RunWithContent(preambleCtx(), imageContent("hi")); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("RunWithContent() error = %v, want ErrContentUnsupported", err)
	}
	if _, err := a.RunWithContentStream(preambleCtx(), imageContent("hi")); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("RunWithContentStream() error = %v, want ErrContentUnsupported", err)
	}
	if got, err := a.RunWithContent(preambleCtx(), interfaces.NewContentFromText("hi", "")); err != nil || got != "hi" {
		t.Errorf("RunWithContent() with text = %q, %v, want the text passed through", got, err)
	}
}
