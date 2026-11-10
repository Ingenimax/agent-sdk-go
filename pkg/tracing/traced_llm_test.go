package tracing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// plainLLM hides the streaming methods of the LLM it wraps, so it is not an
// interfaces.StreamingLLM. testutil.FakeLLM always implements streaming.
type plainLLM struct{ interfaces.LLM }

// newLLM returns a non-streaming LLM named "fake" that answers with response
// or fails with err. Detailed responses report the model "resp-model".
func newLLM(response string, err error) plainLLM {
	return plainLLM{&testutil.FakeLLM{DefaultResponse: response, Err: err, ModelName: "resp-model"}}
}

// modelLLM adds GetModel to a non-streaming LLM.
type modelLLM struct {
	plainLLM
	model string
}

func (m modelLLM) GetModel() string { return m.model }

// streamLLM is a streaming LLM that replays fixed events.
type streamLLM struct {
	testutil.FakeLLM
	events   []interfaces.StreamEvent
	startErr error
}

func (s *streamLLM) SupportsStreaming() bool { return true }

func (s *streamLLM) GenerateStream(context.Context, string, ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	return s.replay()
}

func (s *streamLLM) GenerateWithToolsStream(context.Context, string, []interfaces.Tool, ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	return s.replay()
}

func (s *streamLLM) replay() (<-chan interfaces.StreamEvent, error) {
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

func namedTool(name string) interfaces.Tool { return &testutil.FakeTool{ToolName: name} }

func TestTracedLLM_Generate(t *testing.T) {
	tests := []struct {
		name      string
		llm       interfaces.LLM
		wantModel string
		wantErr   bool
	}{
		{name: "model from GetModel", llm: modelLLM{plainLLM: newLLM("out", nil), model: "m1"}, wantModel: "m1"},
		{name: "empty model falls back to name", llm: modelLLM{plainLLM: newLLM("out", nil)}, wantModel: "fake"},
		{name: "no GetModel", llm: newLLM("out", nil), wantModel: "unknown"},
		{name: "error", llm: newLLM("", errors.New("down")), wantModel: "unknown", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &testutil.FakeTracer{}
			got, err := NewTracedLLM(tt.llm, rec).Generate(context.Background(), "prompt")
			name, span := onlySpan(t, rec)
			if name != "llm.generate" || !span.Ended() {
				t.Errorf("span = %q ended=%v", name, span.Ended())
			}
			if attr(span, "model") != tt.wantModel {
				t.Errorf("model = %v, want %q", attr(span, "model"), tt.wantModel)
			}
			if attr(span, "prompt.length") != 6 || attr(span, "prompt.hash") != hashString("prompt") {
				t.Errorf("prompt attrs = %v, %v", attr(span, "prompt.length"), attr(span, "prompt.hash"))
			}
			if tt.wantErr {
				if err == nil || len(span.Errors()) != 1 {
					t.Errorf("err = %v, recorded = %v", err, span.Errors())
				}
				if hasAttr(span, "response.length") {
					t.Error("response attributes set on error")
				}
				return
			}
			if err != nil || got != "out" {
				t.Fatalf("Generate = %q, %v", got, err)
			}
			if attr(span, "response.length") != 3 {
				t.Errorf("response.length = %v", attr(span, "response.length"))
			}
			if hasAttr(span, "prompt.content") {
				t.Error("content included with a tracer that does not opt in")
			}
		})
	}
}

func TestTracedLLM_IncludesContentWhenEnabled(t *testing.T) {
	tracer, exporter := newTestLangfuseTracer(t, true)
	for name, tr := range map[string]interfaces.Tracer{"direct": tracer, "adapter": NewOTELTracerAdapter(tracer)} {
		t.Run(name, func(t *testing.T) {
			exporter.Reset()
			if _, err := NewTracedLLM(newLLM("secret answer", nil), tr).Generate(context.Background(), "secret prompt"); err != nil {
				t.Fatalf("Generate: %v", err)
			}
			assertAttrs(t, attrMap(spanByName(t, exporter, "llm.generate").Attributes), map[string]string{
				"prompt.content":   "secret prompt",
				"response.content": "secret answer",
			})
		})
	}
}

func TestTracedLLM_GenerateWithTools(t *testing.T) {
	tools := []interfaces.Tool{namedTool("a"), namedTool("b")}

	t.Run("records tool names", func(t *testing.T) {
		rec := &testutil.FakeTracer{}
		out, err := NewTracedLLM(newLLM("ok", nil), rec).GenerateWithTools(context.Background(), "p", tools)
		if err != nil || out != "ok" {
			t.Fatalf("GenerateWithTools = %q, %v", out, err)
		}
		name, span := onlySpan(t, rec)
		if name != "llm.generate_with_tools" || attr(span, "tools") != "a,b" || attr(span, "tools.count") != 2 {
			t.Errorf("span %q tools = %v count = %v", name, attr(span, "tools"), attr(span, "tools.count"))
		}
	})

	t.Run("error is recorded", func(t *testing.T) {
		rec := &testutil.FakeTracer{}
		_, err := NewTracedLLM(newLLM("", errors.New("nope")), rec).GenerateWithTools(context.Background(), "p", nil)
		_, span := onlySpan(t, rec)
		if err == nil || len(span.Errors()) != 1 {
			t.Errorf("err = %v", err)
		}
	})

	tests := []struct {
		name           string
		includeContent bool
		response       string
		err            error
		useAdapter     bool
		wantOutput     string
		wantError      string
	}{
		{name: "redacted output", response: "real", wantOutput: "<redacted>"},
		{name: "content included through adapter", includeContent: true, useAdapter: true, response: "real", wantOutput: "real"},
		{name: "error with empty content", includeContent: true, err: errors.New("broke"), wantOutput: "<error>", wantError: "broke"},
	}
	for _, tt := range tests {
		t.Run("generation span: "+tt.name, func(t *testing.T) {
			tracer, exporter := newTestLangfuseTracer(t, tt.includeContent)
			var tr interfaces.Tracer = tracer
			if tt.useAdapter {
				tr = NewOTELTracerAdapter(tracer)
			}
			// The LLM reports one tool call before it answers or fails.
			llm := plainLLM{&testutil.FakeLLM{GenerateFunc: func(ctx context.Context, _ string, _ []interfaces.Tool) (string, error) {
				AddToolCallToContext(ctx, ToolCall{Name: "a", Arguments: "{}"})
				return tt.response, tt.err
			}}}
			_, _ = NewTracedLLM(llm, tr).GenerateWithTools(context.Background(), "p", tools)

			gen := attrMap(spanByName(t, exporter, "llm.generation").Attributes)
			if !strings.HasPrefix(gen["gen_ai.completion.0.content"], tt.wantOutput) {
				t.Errorf("completion = %q, want %q", gen["gen_ai.completion.0.content"], tt.wantOutput)
			}
			assertAttrs(t, gen, map[string]string{
				"langfuse.observation.metadata.streaming": "false",
				"langfuse.observation.metadata.tools":     "2",
			})
			if gen["langfuse.observation.metadata.error"] != tt.wantError {
				t.Errorf("error metadata = %q, want %q", gen["langfuse.observation.metadata.error"], tt.wantError)
			}
		})
	}
}

func TestTracedLLM_PassThrough(t *testing.T) {
	plain := NewTracedLLM(newLLM("", nil), noopTracer{}).(*TracedLLM)
	if plain.Name() != "fake" || plain.SupportsStreaming() {
		t.Errorf("Name/SupportsStreaming not delegated")
	}
	if plain.GetModel() != "fake" {
		t.Errorf("GetModel without model = %q, want provider name", plain.GetModel())
	}
	withModel := NewTracedLLM(modelLLM{plainLLM: newLLM("", nil), model: "m"}, noopTracer{}).(*TracedLLM)
	if withModel.GetModel() != "m" {
		t.Errorf("GetModel = %q", withModel.GetModel())
	}
	if !NewTracedLLM(&streamLLM{}, noopTracer{}).SupportsStreaming() {
		t.Error("SupportsStreaming not delegated")
	}
}

func TestTracedLLM_StreamRequiresStreamingLLM(t *testing.T) {
	traced := NewTracedLLM(newLLM("", nil), noopTracer{}).(*TracedLLM)
	if _, err := traced.GenerateStream(context.Background(), "p"); err == nil {
		t.Error("GenerateStream: expected an error")
	}
	if _, err := traced.GenerateWithToolsStream(context.Background(), "p", nil); err == nil {
		t.Error("GenerateWithToolsStream: expected an error")
	}
}

func TestTracedLLM_GenerateStream(t *testing.T) {
	tracer, exporter := newTestLangfuseTracer(t, true)
	llm := &streamLLM{events: []interfaces.StreamEvent{{Type: interfaces.StreamEventContentDelta, Content: "hi"}}}

	ch, err := NewTracedLLM(llm, tracer).(*TracedLLM).GenerateStream(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("GenerateStream: %v", err)
	}
	var got []string
	for e := range ch {
		got = append(got, e.Content)
	}
	if strings.Join(got, "") != "hi" {
		t.Errorf("events = %v", got)
	}
	assertAttrs(t, attrMap(spanByName(t, exporter, "llm.generate_stream").Attributes), map[string]string{
		"streaming":      "true",
		"model":          "unknown",
		"prompt.content": "prompt",
	})
}

func TestTracedLLM_GenerateWithToolsStream(t *testing.T) {
	t.Run("start error ends the span", func(t *testing.T) {
		rec := &testutil.FakeTracer{}
		llm := &streamLLM{startErr: errors.New("refused")}
		_, err := NewTracedLLM(llm, rec).(*TracedLLM).GenerateWithToolsStream(context.Background(), "p", nil)
		if err == nil {
			t.Fatal("expected an error")
		}
		_, span := onlySpan(t, rec)
		if !span.Ended() || len(span.Errors()) != 1 {
			t.Errorf("ended=%v errs=%v", span.Ended(), span.Errors())
		}
	})

	t.Run("proxies events and records tool names", func(t *testing.T) {
		rec := &testutil.FakeTracer{}
		llm := &streamLLM{events: []interfaces.StreamEvent{
			{Type: interfaces.StreamEventContentDelta, Content: "a"},
			{Type: interfaces.StreamEventContentDelta, Content: "b"},
		}}
		ch, err := NewTracedLLM(llm, rec).(*TracedLLM).GenerateWithToolsStream(context.Background(), "p", []interfaces.Tool{namedTool("x")})
		if err != nil {
			t.Fatalf("GenerateWithToolsStream: %v", err)
		}
		var got string
		for e := range ch {
			got += e.Content
		}
		if got != "ab" {
			t.Errorf("content = %q", got)
		}
		_, span := onlySpan(t, rec)
		if !span.Ended() || attr(span, "tools") != "x" || attr(span, "prompt.length") != 1 {
			t.Errorf("ended=%v tools=%v prompt.length=%v", span.Ended(), attr(span, "tools"), attr(span, "prompt.length"))
		}
		if !hasAttr(span, "duration_ms") {
			t.Error("duration_ms not set after the stream finished")
		}
	})
}

func TestTracedLLM_Detailed(t *testing.T) {
	usage := &interfaces.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, ReasoningTokens: 2}
	tools := []interfaces.Tool{namedTool("t1"), namedTool("t2")}

	calls := map[string]func(*TracedLLM) (*interfaces.LLMResponse, error){
		"llm.generate_detailed": func(m *TracedLLM) (*interfaces.LLMResponse, error) {
			return m.GenerateDetailed(context.Background(), "p")
		},
		"llm.generate_with_tools_detailed": func(m *TracedLLM) (*interfaces.LLMResponse, error) {
			return m.GenerateWithToolsDetailed(context.Background(), "p", tools)
		},
	}
	for spanName, call := range calls {
		t.Run(spanName, func(t *testing.T) {
			rec := &testutil.FakeTracer{}
			withUsage := plainLLM{&testutil.FakeLLM{DefaultResponse: "done", Usage: usage, ModelName: "resp-model"}}
			resp, err := call(NewTracedLLM(modelLLM{plainLLM: withUsage, model: "m"}, rec).(*TracedLLM))
			if err != nil || resp.Content != "done" {
				t.Fatalf("resp = %+v, err = %v", resp, err)
			}
			name, span := onlySpan(t, rec)
			if name != spanName {
				t.Errorf("span name = %q", name)
			}
			want := map[string]any{
				"model":                  "m",
				"response.length":        4,
				"response.model":         "resp-model",
				"usage.input_tokens":     10,
				"usage.output_tokens":    5,
				"usage.total_tokens":     15,
				"usage.reasoning_tokens": 2,
			}
			for k, v := range want {
				if attr(span, k) != v {
					t.Errorf("%s = %v, want %v", k, attr(span, k), v)
				}
			}
			if spanName == "llm.generate_with_tools_detailed" && attr(span, "tools.names") != "t1,t2" {
				t.Errorf("tools.names = %v", attr(span, "tools.names"))
			}

			rec = &testutil.FakeTracer{}
			resp, err = call(NewTracedLLM(newLLM("", errors.New("fail")), rec).(*TracedLLM))
			if err == nil || resp != nil {
				t.Errorf("expected error, got %+v", resp)
			}
			if _, span := onlySpan(t, rec); len(span.Errors()) != 1 {
				t.Error("error not recorded on span")
			}

			rec = &testutil.FakeTracer{}
			if _, err := call(NewTracedLLM(newLLM("x", nil), rec).(*TracedLLM)); err != nil {
				t.Fatalf("no usage: %v", err)
			}
			if _, span := onlySpan(t, rec); hasAttr(span, "usage.input_tokens") {
				t.Error("usage attributes set without usage")
			}
		})
	}
}

func TestTracedLLM_GenerateDetailedIncludesContent(t *testing.T) {
	tracer, exporter := newTestLangfuseTracer(t, true)
	if _, err := NewTracedLLM(newLLM("r", nil), tracer).GenerateDetailed(context.Background(), "p"); err != nil {
		t.Fatalf("GenerateDetailed: %v", err)
	}
	assertAttrs(t, attrMap(spanByName(t, exporter, "llm.generate_detailed").Attributes), map[string]string{
		"prompt.content":   "p",
		"response.content": "r",
	})
}

// failingMemory returns err from every call.
type failingMemory struct {
	err      error
	messages []interfaces.Message
}

func (f *failingMemory) AddMessage(context.Context, interfaces.Message) error { return f.err }
func (f *failingMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return f.messages, f.err
}
func (f *failingMemory) Clear(context.Context) error { return f.err }

func TestTracedMemory_Operations(t *testing.T) {
	msg := interfaces.Message{Role: interfaces.MessageRoleAssistant, Content: "hello", ToolCalls: []interfaces.ToolCall{{ID: "1"}}}

	t.Run("success", func(t *testing.T) {
		rec := &testutil.FakeTracer{}
		mem := NewTracedMemory(&failingMemory{messages: []interfaces.Message{msg, msg}}, rec)
		ctx := context.Background()

		if err := mem.AddMessage(ctx, msg); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
		got, err := mem.GetMessages(ctx)
		if err != nil || len(got) != 2 {
			t.Fatalf("GetMessages = %v, %v", got, err)
		}
		if err := mem.Clear(ctx); err != nil {
			t.Fatalf("Clear: %v", err)
		}

		names, spans := rec.SpanNames(), rec.Spans()
		if strings.Join(names, ",") != "memory.add_message,memory.get_messages,memory.clear" {
			t.Fatalf("spans = %v", names)
		}
		add, get := spans[0], spans[1]
		if attr(add, "message.role") != "assistant" || attr(add, "message.content_length") != 5 || attr(add, "message.tool_calls_count") != 1 {
			t.Errorf("add span role=%v length=%v tool calls=%v",
				attr(add, "message.role"), attr(add, "message.content_length"), attr(add, "message.tool_calls_count"))
		}
		if attr(get, "messages.count") != 2 {
			t.Errorf("messages.count = %v", attr(get, "messages.count"))
		}
		for i, s := range spans {
			if !s.Ended() || len(s.Errors()) != 0 {
				t.Errorf("span %q ended=%v errs=%v", names[i], s.Ended(), s.Errors())
			}
		}
	})

	t.Run("errors are recorded", func(t *testing.T) {
		rec := &testutil.FakeTracer{}
		boom := errors.New("boom")
		mem := NewTracedMemory(&failingMemory{err: boom}, rec)
		ctx := context.Background()

		if err := mem.AddMessage(ctx, interfaces.Message{}); !errors.Is(err, boom) {
			t.Errorf("AddMessage = %v", err)
		}
		if _, err := mem.GetMessages(ctx); !errors.Is(err, boom) {
			t.Errorf("GetMessages = %v", err)
		}
		if err := mem.Clear(ctx); !errors.Is(err, boom) {
			t.Errorf("Clear = %v", err)
		}
		names, spans := rec.SpanNames(), rec.Spans()
		if len(spans) != 3 {
			t.Fatalf("spans = %v", names)
		}
		for i, s := range spans {
			if len(s.Errors()) != 1 {
				t.Errorf("span %q errs = %v", names[i], s.Errors())
			}
		}
		if hasAttr(spans[1], "messages.count") {
			t.Error("messages.count set on error")
		}
		if hasAttr(spans[0], "message.tool_calls_count") {
			t.Error("tool_calls_count set for a message without tool calls")
		}
	})
}

func TestHashString(t *testing.T) {
	if hashString("") != "" {
		t.Error("empty input should hash to empty string")
	}
	// SHA-256 of "abc".
	if got := hashString("abc"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("hashString(abc) = %q", got)
	}
}
