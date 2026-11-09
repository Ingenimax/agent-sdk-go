package tracing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// fakeLLM is a non-streaming LLM with a fixed reply.
type fakeLLM struct {
	response  string
	err       error
	usage     *interfaces.TokenUsage
	toolCalls []ToolCall
}

func (f *fakeLLM) Generate(context.Context, string, ...interfaces.GenerateOption) (string, error) {
	return f.response, f.err
}

func (f *fakeLLM) GenerateWithTools(ctx context.Context, _ string, _ []interfaces.Tool, _ ...interfaces.GenerateOption) (string, error) {
	for _, tc := range f.toolCalls {
		AddToolCallToContext(ctx, tc)
	}
	return f.response, f.err
}

func (f *fakeLLM) GenerateDetailed(context.Context, string, ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &interfaces.LLMResponse{Content: f.response, Model: "resp-model", Usage: f.usage}, nil
}

func (f *fakeLLM) GenerateWithToolsDetailed(ctx context.Context, prompt string, _ []interfaces.Tool, opts ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return f.GenerateDetailed(ctx, prompt, opts...)
}

func (f *fakeLLM) Name() string            { return "fake" }
func (f *fakeLLM) SupportsStreaming() bool { return false }

// modelLLM adds GetModel to fakeLLM.
type modelLLM struct {
	fakeLLM
	model string
}

func (m *modelLLM) GetModel() string { return m.model }

// streamLLM is a streaming LLM that replays fixed events.
type streamLLM struct {
	fakeLLM
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

// namedTool is a minimal tool with a name.
type namedTool string

func (n namedTool) Name() string                                    { return string(n) }
func (n namedTool) Description() string                             { return "" }
func (n namedTool) Run(context.Context, string) (string, error)     { return "", nil }
func (n namedTool) Parameters() map[string]interfaces.ParameterSpec { return nil }
func (n namedTool) Execute(context.Context, string) (string, error) { return "", nil }

func TestTracedLLM_Generate(t *testing.T) {
	tests := []struct {
		name      string
		llm       interfaces.LLM
		wantModel string
		wantErr   bool
	}{
		{name: "model from GetModel", llm: &modelLLM{fakeLLM: fakeLLM{response: "out"}, model: "m1"}, wantModel: "m1"},
		{name: "empty model falls back to name", llm: &modelLLM{fakeLLM: fakeLLM{response: "out"}}, wantModel: "fake"},
		{name: "no GetModel", llm: &fakeLLM{response: "out"}, wantModel: "unknown"},
		{name: "error", llm: &fakeLLM{err: errors.New("down")}, wantModel: "unknown", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingTracer{}
			got, err := NewTracedLLM(tt.llm, rec).Generate(context.Background(), "prompt")
			span := rec.only(t)
			if span.name != "llm.generate" || !span.ended {
				t.Errorf("span = %q ended=%v", span.name, span.ended)
			}
			if span.attrs["model"] != tt.wantModel {
				t.Errorf("model = %v, want %q", span.attrs["model"], tt.wantModel)
			}
			if span.attrs["prompt.length"] != 6 || span.attrs["prompt.hash"] != hashString("prompt") {
				t.Errorf("prompt attrs = %v", span.attrs)
			}
			if tt.wantErr {
				if err == nil || len(span.errs) != 1 {
					t.Errorf("err = %v, recorded = %v", err, span.errs)
				}
				if _, ok := span.attrs["response.length"]; ok {
					t.Error("response attributes set on error")
				}
				return
			}
			if err != nil || got != "out" {
				t.Fatalf("Generate = %q, %v", got, err)
			}
			if span.attrs["response.length"] != 3 {
				t.Errorf("response.length = %v", span.attrs["response.length"])
			}
			if _, ok := span.attrs["prompt.content"]; ok {
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
			if _, err := NewTracedLLM(&fakeLLM{response: "secret answer"}, tr).Generate(context.Background(), "secret prompt"); err != nil {
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
		rec := &recordingTracer{}
		out, err := NewTracedLLM(&fakeLLM{response: "ok"}, rec).GenerateWithTools(context.Background(), "p", tools)
		if err != nil || out != "ok" {
			t.Fatalf("GenerateWithTools = %q, %v", out, err)
		}
		span := rec.only(t)
		if span.name != "llm.generate_with_tools" || span.attrs["tools"] != "a,b" || span.attrs["tools.count"] != 2 {
			t.Errorf("span %q attrs = %v", span.name, span.attrs)
		}
	})

	t.Run("error is recorded", func(t *testing.T) {
		rec := &recordingTracer{}
		_, err := NewTracedLLM(&fakeLLM{err: errors.New("nope")}, rec).GenerateWithTools(context.Background(), "p", nil)
		if err == nil || len(rec.only(t).errs) != 1 {
			t.Errorf("err = %v", err)
		}
	})

	tests := []struct {
		name           string
		includeContent bool
		llm            *fakeLLM
		useAdapter     bool
		wantOutput     string
		wantError      string
	}{
		{name: "redacted output", llm: &fakeLLM{response: "real"}, wantOutput: "<redacted>"},
		{name: "content included through adapter", includeContent: true, useAdapter: true, llm: &fakeLLM{response: "real"}, wantOutput: "real"},
		{name: "error with empty content", includeContent: true, llm: &fakeLLM{err: errors.New("broke")}, wantOutput: "<error>", wantError: "broke"},
	}
	for _, tt := range tests {
		t.Run("generation span: "+tt.name, func(t *testing.T) {
			tracer, exporter := newTestLangfuseTracer(t, tt.includeContent)
			var tr interfaces.Tracer = tracer
			if tt.useAdapter {
				tr = NewOTELTracerAdapter(tracer)
			}
			tt.llm.toolCalls = []ToolCall{{Name: "a", Arguments: "{}"}}
			_, _ = NewTracedLLM(tt.llm, tr).GenerateWithTools(context.Background(), "p", tools)

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
	plain := NewTracedLLM(&fakeLLM{}, noopTracer{}).(*TracedLLM)
	if plain.Name() != "fake" || plain.SupportsStreaming() {
		t.Errorf("Name/SupportsStreaming not delegated")
	}
	if plain.GetModel() != "fake" {
		t.Errorf("GetModel without model = %q, want provider name", plain.GetModel())
	}
	withModel := NewTracedLLM(&modelLLM{model: "m"}, noopTracer{}).(*TracedLLM)
	if withModel.GetModel() != "m" {
		t.Errorf("GetModel = %q", withModel.GetModel())
	}
	if !NewTracedLLM(&streamLLM{}, noopTracer{}).SupportsStreaming() {
		t.Error("SupportsStreaming not delegated")
	}
}

func TestTracedLLM_StreamRequiresStreamingLLM(t *testing.T) {
	traced := NewTracedLLM(&fakeLLM{}, noopTracer{}).(*TracedLLM)
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
		rec := &recordingTracer{}
		llm := &streamLLM{startErr: errors.New("refused")}
		_, err := NewTracedLLM(llm, rec).(*TracedLLM).GenerateWithToolsStream(context.Background(), "p", nil)
		if err == nil {
			t.Fatal("expected an error")
		}
		span := rec.only(t)
		if !span.ended || len(span.errs) != 1 {
			t.Errorf("ended=%v errs=%v", span.ended, span.errs)
		}
	})

	t.Run("proxies events and records tool names", func(t *testing.T) {
		rec := &recordingTracer{}
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
		span := rec.only(t)
		if !span.ended || span.attrs["tools"] != "x" || span.attrs["prompt.length"] != 1 {
			t.Errorf("ended=%v attrs=%v", span.ended, span.attrs)
		}
		if _, ok := span.attrs["duration_ms"]; !ok {
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
			rec := &recordingTracer{}
			resp, err := call(NewTracedLLM(&modelLLM{fakeLLM: fakeLLM{response: "done", usage: usage}, model: "m"}, rec).(*TracedLLM))
			if err != nil || resp.Content != "done" {
				t.Fatalf("resp = %+v, err = %v", resp, err)
			}
			span := rec.only(t)
			if span.name != spanName {
				t.Errorf("span name = %q", span.name)
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
				if span.attrs[k] != v {
					t.Errorf("%s = %v, want %v", k, span.attrs[k], v)
				}
			}
			if spanName == "llm.generate_with_tools_detailed" && span.attrs["tools.names"] != "t1,t2" {
				t.Errorf("tools.names = %v", span.attrs["tools.names"])
			}

			rec = &recordingTracer{}
			resp, err = call(NewTracedLLM(&fakeLLM{err: errors.New("fail")}, rec).(*TracedLLM))
			if err == nil || resp != nil {
				t.Errorf("expected error, got %+v", resp)
			}
			if len(rec.only(t).errs) != 1 {
				t.Error("error not recorded on span")
			}

			rec = &recordingTracer{}
			if _, err := call(NewTracedLLM(&fakeLLM{response: "x"}, rec).(*TracedLLM)); err != nil {
				t.Fatalf("no usage: %v", err)
			}
			if _, ok := rec.only(t).attrs["usage.input_tokens"]; ok {
				t.Error("usage attributes set without usage")
			}
		})
	}
}

func TestTracedLLM_GenerateDetailedIncludesContent(t *testing.T) {
	tracer, exporter := newTestLangfuseTracer(t, true)
	if _, err := NewTracedLLM(&fakeLLM{response: "r"}, tracer).GenerateDetailed(context.Background(), "p"); err != nil {
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
		rec := &recordingTracer{}
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

		if len(rec.spans) != 3 {
			t.Fatalf("spans = %d, want 3", len(rec.spans))
		}
		add, get, clr := rec.spans[0], rec.spans[1], rec.spans[2]
		if add.name != "memory.add_message" || add.attrs["message.role"] != "assistant" ||
			add.attrs["message.content_length"] != 5 || add.attrs["message.tool_calls_count"] != 1 {
			t.Errorf("add span %q attrs = %v", add.name, add.attrs)
		}
		if get.name != "memory.get_messages" || get.attrs["messages.count"] != 2 {
			t.Errorf("get span %q attrs = %v", get.name, get.attrs)
		}
		if clr.name != "memory.clear" {
			t.Errorf("clear span = %q", clr.name)
		}
		for _, s := range rec.spans {
			if !s.ended || len(s.errs) != 0 {
				t.Errorf("span %q ended=%v errs=%v", s.name, s.ended, s.errs)
			}
		}
	})

	t.Run("errors are recorded", func(t *testing.T) {
		rec := &recordingTracer{}
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
		for _, s := range rec.spans {
			if len(s.errs) != 1 {
				t.Errorf("span %q errs = %v", s.name, s.errs)
			}
		}
		if _, ok := rec.spans[1].attrs["messages.count"]; ok {
			t.Error("messages.count set on error")
		}
		if _, ok := rec.spans[0].attrs["message.tool_calls_count"]; ok {
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
