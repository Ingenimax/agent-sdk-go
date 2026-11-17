package agent

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// mediaBlindLLM refuses media the way the providers do: before it answers, it
// resolves its options and calls interfaces.CheckMedia, which reads every user
// message in memory. So, as with a real provider, a refused part left in
// memory fails every later call in the conversation.
type mediaBlindLLM struct {
	testutil.FakeLLM

	// refuseInStream reports the refusal as a stream error event instead of
	// returning it from GenerateStream.
	refuseInStream bool
}

func (m *mediaBlindLLM) refuse(ctx context.Context, options []interfaces.GenerateOption) error {
	resolved := &interfaces.GenerateOptions{}
	for _, option := range options {
		option(resolved)
	}
	return interfaces.CheckMedia(ctx, resolved, "mediablind", nil)
}

func (m *mediaBlindLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	if err := m.refuse(ctx, options); err != nil {
		return "", err
	}
	return m.FakeLLM.Generate(ctx, prompt)
}

func (m *mediaBlindLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	if err := m.refuse(ctx, options); err != nil {
		return "", err
	}
	return m.FakeLLM.GenerateWithTools(ctx, prompt, tools)
}

func (m *mediaBlindLLM) GenerateDetailed(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	if err := m.refuse(ctx, options); err != nil {
		return nil, err
	}
	return m.FakeLLM.GenerateDetailed(ctx, prompt)
}

func (m *mediaBlindLLM) GenerateWithToolsDetailed(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	if err := m.refuse(ctx, options); err != nil {
		return nil, err
	}
	return m.FakeLLM.GenerateWithToolsDetailed(ctx, prompt, tools)
}

func (m *mediaBlindLLM) GenerateStream(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	if err := m.refuse(ctx, options); err != nil {
		return m.streamRefusal(err)
	}
	return m.FakeLLM.GenerateStream(ctx, prompt)
}

func (m *mediaBlindLLM) GenerateWithToolsStream(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	if err := m.refuse(ctx, options); err != nil {
		return m.streamRefusal(err)
	}
	return m.FakeLLM.GenerateWithToolsStream(ctx, prompt, tools)
}

func (m *mediaBlindLLM) streamRefusal(err error) (<-chan interfaces.StreamEvent, error) {
	if !m.refuseInStream {
		return nil, err
	}
	events := make(chan interfaces.StreamEvent, 1)
	events <- interfaces.StreamEvent{Type: interfaces.StreamEventError, Error: err}
	close(events)
	return events, nil
}

// unwrappingMemory is a decorator that hides every optional capability of the
// memory it wraps except through interfaces.MemoryUnwrapper, as tracing and
// consolidation decorators do.
type unwrappingMemory struct {
	inner interfaces.Memory
}

func (m unwrappingMemory) AddMessage(ctx context.Context, message interfaces.Message) error {
	return m.inner.AddMessage(ctx, message)
}

func (m unwrappingMemory) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return m.inner.GetMessages(ctx, options...)
}

func (m unwrappingMemory) Clear(ctx context.Context) error { return m.inner.Clear(ctx) }

func (m unwrappingMemory) Unwrap() interfaces.Memory { return m.inner }

func pdfContent(text string) *interfaces.Content {
	return interfaces.NewContentFromParts([]*interfaces.Part{
		interfaces.NewPartFromBytes([]byte("%PDF-1.7"), "application/pdf"),
		interfaces.NewPartFromText(text),
	}, interfaces.MessageRoleUser)
}

type contentRunner func(context.Context, *Agent, *interfaces.Content) error

func runSync(ctx context.Context, a *Agent, content *interfaces.Content) error {
	_, err := a.RunWithContent(ctx, content)
	return err
}

func runStream(ctx context.Context, a *Agent, content *interfaces.Content) error {
	events, err := a.RunWithContentStream(ctx, content)
	if err != nil {
		return err
	}
	var first error
	for event := range events {
		if event.Error != nil && first == nil {
			first = event.Error
		}
	}
	return first
}

func storedMessages(t *testing.T, mem interfaces.Memory) []interfaces.Message {
	t.Helper()
	msgs, err := mem.GetMessages(preambleCtx())
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}
	return slices.Clone(msgs)
}

// TestRefusedContentIsNotReplayed guards a bug that broke a whole conversation
// after one refused attachment.
//
// beginRun stores the user message before the provider decides whether it can
// send the message's parts, and the providers build each request from memory.
// So when a provider refused a part with ErrContentUnsupported, the refused
// message stayed in the conversation and every later turn replayed it: even a
// text-only follow-up failed with ErrContentUnsupported (a 422 over the HTTP
// API), while other conversations kept working.
func TestRefusedContentIsNotReplayed(t *testing.T) {
	tests := []struct {
		name    string
		llm     *mediaBlindLLM
		run     contentRunner
		options []Option
		wrap    func(interfaces.Memory) interfaces.Memory
	}{
		{
			name: "sync",
			llm:  &mediaBlindLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: "ok"}},
			run:  runSync,
		},
		{
			name:    "sync with tools",
			llm:     &mediaBlindLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: "ok"}},
			run:     runSync,
			options: []Option{WithTools(&testutil.FakeTool{}), WithRequirePlanApproval(false)},
		},
		{
			name: "sync through a decorator",
			llm:  &mediaBlindLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: "ok"}},
			run:  runSync,
			wrap: func(m interfaces.Memory) interfaces.Memory { return unwrappingMemory{inner: m} },
		},
		{
			name: "stream",
			llm:  &mediaBlindLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: "ok", Streaming: true}},
			run:  runStream,
		},
		{
			name:    "stream with tools",
			llm:     &mediaBlindLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: "ok", Streaming: true}},
			run:     runStream,
			options: []Option{WithTools(&testutil.FakeTool{}), WithRequirePlanApproval(false)},
		},
		{
			name: "stream refused in an error event",
			llm:  &mediaBlindLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: "ok", Streaming: true}, refuseInStream: true},
			run:  runStream,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := memory.NewConversationBuffer()
			var mem interfaces.Memory = buffer
			if tt.wrap != nil {
				mem = tt.wrap(buffer)
			}
			options := append([]Option{WithLLM(tt.llm), WithName("blind"), WithMemory(mem)}, tt.options...)
			a, err := NewAgent(options...)
			if err != nil {
				t.Fatalf("NewAgent() error = %v", err)
			}
			ctx := preambleCtx()

			if err := tt.run(ctx, a, textContent("hello")); err != nil {
				t.Fatalf("first text run error = %v", err)
			}
			before := storedMessages(t, buffer)

			if err := tt.run(ctx, a, pdfContent("summarize this")); !errors.Is(err, interfaces.ErrContentUnsupported) {
				t.Fatalf("PDF run error = %v, want ErrContentUnsupported", err)
			}
			if after := storedMessages(t, buffer); !reflect.DeepEqual(after, before) {
				t.Errorf("memory after the refused run = %+v, want it unchanged: %+v", after, before)
			}

			if err := tt.run(ctx, a, textContent("what can you do?")); err != nil {
				t.Fatalf("text run after the refusal error = %v, want the conversation to keep working", err)
			}
			msgs := storedMessages(t, buffer)
			if len(msgs) < 2 || msgs[len(msgs)-2].Content != "what can you do?" {
				t.Errorf("memory = %+v, want the text follow-up stored", msgs)
			}
		})
	}
}

// A refused part in an earlier message is not this run's input, so the run
// must not remove its own message on that account.
func TestRefusedHistoryKeepsTextInput(t *testing.T) {
	buffer := memory.NewConversationBuffer()
	ctx := preambleCtx()
	poisoned := userMessage(pdfContent("from before"))
	if err := buffer.AddMessage(ctx, poisoned); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}

	llm := &mediaBlindLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: "ok"}}
	a, err := NewAgent(WithLLM(llm), WithName("blind"), WithMemory(buffer))
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if _, err := a.Run(ctx, "hello"); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Fatalf("Run() error = %v, want ErrContentUnsupported from the stored PDF", err)
	}
	msgs := storedMessages(t, buffer)
	if len(msgs) != 2 || msgs[1].Content != "hello" {
		t.Errorf("memory = %+v, want the earlier message and the text input both kept", msgs)
	}
}

// A memory that cannot remove messages keeps the old behavior: the run still
// returns the refusal, and nothing panics.
func TestRefusedContentWithoutRemover(t *testing.T) {
	buffer := memory.NewConversationBuffer()
	llm := &mediaBlindLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: "ok"}}
	a, err := NewAgent(WithLLM(llm), WithName("blind"), WithMemory(addOnlyMemory{buffer}))
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if _, err := a.RunWithContent(preambleCtx(), pdfContent("summarize this")); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Fatalf("RunWithContent() error = %v, want ErrContentUnsupported", err)
	}
}

// addOnlyMemory exposes only the three core Memory methods.
type addOnlyMemory struct {
	inner interfaces.Memory
}

func (m addOnlyMemory) AddMessage(ctx context.Context, message interfaces.Message) error {
	return m.inner.AddMessage(ctx, message)
}

func (m addOnlyMemory) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return m.inner.GetMessages(ctx, options...)
}

func (m addOnlyMemory) Clear(ctx context.Context) error { return m.inner.Clear(ctx) }
