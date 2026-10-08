package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/openai/openai-go/v2"
)

func chunkUsage(prompt, completion, total int64) openai.CompletionUsage {
	return openai.CompletionUsage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
	}
}

// usageProbeTool is a tool the streaming loop can actually execute, so the
// loop takes its tool-calling branch rather than returning on the first chunk.
type usageProbeTool struct{}

func (usageProbeTool) Name() string        { return "probe" }
func (usageProbeTool) Description() string { return "probe tool" }
func (usageProbeTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{}
}
func (usageProbeTool) Run(context.Context, string) (string, error)     { return "probe result", nil }
func (usageProbeTool) Execute(context.Context, string) (string, error) { return "probe result", nil }

func sse(w http.ResponseWriter, chunks ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	for _, c := range chunks {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", c)
		if flusher != nil {
			flusher.Flush()
		}
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// TestGenerateWithToolsStreamAggregatesUsage pins the fix for #327: the
// streaming tool loop must sum token usage over every underlying completion and
// publish the total on the terminal event. Before the fix the stream requested
// no usage at all on these calls and reported nothing.
func TestGenerateWithToolsStreamAggregatesUsage(t *testing.T) {
	var calls atomic.Int32
	var sawIncludeUsage atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"include_usage":true`) {
			sawIncludeUsage.Add(1)
		}

		switch calls.Add(1) {
		case 1:
			// Tool call, plus 100 tokens on the usage-only trailing chunk.
			sse(w,
				`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"probe","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
				`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[],"usage":{"prompt_tokens":70,"completion_tokens":30,"total_tokens":100}}`,
			)
		default:
			// Plain answer, plus 11 tokens.
			sse(w,
				`{"id":"c2","object":"chat.completion.chunk","created":2,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`,
				`{"id":"c2","object":"chat.completion.chunk","created":2,"model":"gpt-4o","choices":[],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`,
			)
		}
	}))
	defer server.Close()

	client := NewClient("test-key", WithModel("gpt-4o"), WithBaseURL(server.URL))

	events, err := client.GenerateWithToolsStream(
		context.Background(), "use the probe", []interfaces.Tool{usageProbeTool{}},
	)
	if err != nil {
		t.Fatalf("GenerateWithToolsStream: %v", err)
	}

	var stop *interfaces.StreamEvent
	for event := range events {
		if event.Type == interfaces.StreamEventError {
			t.Fatalf("stream error: %v", event.Error)
		}
		if event.Type == interfaces.StreamEventMessageStop {
			e := event
			stop = &e
		}
	}

	if stop == nil {
		t.Fatal("stream produced no message_stop event")
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("server saw %d completion calls, want at least 2 (tool loop + synthesis)", got)
	}
	if got := sawIncludeUsage.Load(); got != calls.Load() {
		t.Errorf("include_usage requested on %d of %d calls, want all of them", got, calls.Load())
	}

	usage, ok := stop.Metadata[interfaces.MetadataKeyUsage].(*interfaces.TokenUsage)
	if !ok || usage == nil {
		t.Fatalf("message_stop carried no *TokenUsage under %q; metadata=%v",
			interfaces.MetadataKeyUsage, stop.Metadata)
	}

	// The point of the fix: a total spanning every call, not just the last.
	wantTotal := 100 + 11*int(calls.Load()-1)
	if usage.TotalTokens != wantTotal {
		t.Errorf("TotalTokens = %d, want %d (usage summed across %d calls)",
			usage.TotalTokens, wantTotal, calls.Load())
	}
	if usage.TotalTokens <= 100 {
		t.Errorf("TotalTokens = %d, want more than the %d of a single call",
			usage.TotalTokens, 100)
	}
	if model, _ := stop.Metadata[interfaces.MetadataKeyModel].(string); model != "gpt-4o" {
		t.Errorf("model metadata = %q, want %q", model, "gpt-4o")
	}
}

// TestRecordStreamUsageIgnoresEmptyChunks keeps content-only chunks, which carry
// a zero Usage, from marking the accumulator as touched.
func TestRecordStreamUsageIgnoresEmptyChunks(t *testing.T) {
	client := NewClient("k", WithModel("gpt-4o"))
	acc := &usageAccumulator{}

	client.recordStreamUsage(context.Background(), acc, chunkUsage(0, 0, 0))
	if _, _, ok := acc.snapshot(); ok {
		t.Error("accumulator was touched by a zero-usage chunk")
	}

	client.recordStreamUsage(context.Background(), acc, chunkUsage(5, 2, 7))
	usage, model, ok := acc.snapshot()
	if !ok {
		t.Fatal("accumulator not touched by a real usage chunk")
	}
	if usage.TotalTokens != 7 || usage.InputTokens != 5 || usage.OutputTokens != 2 {
		t.Errorf("usage = %+v, want input=5 output=2 total=7", *usage)
	}
	if model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", model)
	}
}

// TestMessageStopWithUsageOmitsMetadataWhenUnused keeps the terminal event
// clean when the provider reported nothing, rather than publishing a zero total
// that a consumer would read as "0 tokens spent".
func TestMessageStopWithUsageOmitsMetadataWhenUnused(t *testing.T) {
	event := messageStopWithUsage(&usageAccumulator{}, "gpt-4o")
	if event.Type != interfaces.StreamEventMessageStop {
		t.Errorf("type = %q, want message_stop", event.Type)
	}
	if _, present := event.Metadata[interfaces.MetadataKeyUsage]; present {
		t.Error("untouched accumulator still published usage metadata")
	}

	acc := &usageAccumulator{}
	acc.add(1, 2, 3, 0, "")
	event = messageStopWithUsage(acc, "fallback-model")
	if model, _ := event.Metadata[interfaces.MetadataKeyModel].(string); model != "fallback-model" {
		t.Errorf("model = %q, want the fallback when the accumulator has none", model)
	}
}
