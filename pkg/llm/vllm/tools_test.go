package vllm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// echoTool records its invocations and returns a fixed result.
type echoTool struct {
	calls atomic.Int64
	mu    sync.Mutex
	args  []string
}

func (t *echoTool) Name() string        { return "get_weather" }
func (t *echoTool) Description() string { return "look up the weather" }

func (t *echoTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"city": {Type: "string", Description: "the city", Required: true},
	}
}

func (t *echoTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *echoTool) Execute(_ context.Context, args string) (string, error) {
	t.calls.Add(1)
	t.mu.Lock()
	t.args = append(t.args, args)
	t.mu.Unlock()
	return "sunny, 21C", nil
}

func (t *echoTool) Args() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.args...)
}

// TestGenerateWithToolsExecutesToolsNatively guards the fix for a provider that
// could not call tools at all.
//
// GenerateWithTools used to stuff tool names and descriptions into the prompt
// and return whatever prose came back. Nothing parsed a tool call out of the
// reply and no tool was ever executed, so an agent configured with vLLM
// silently had no tools -- the same defect fixed for Ollama in #325.
func TestGenerateWithToolsExecutesToolsNatively(t *testing.T) {
	var requests atomic.Int64
	var sawTools atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)

		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		if len(req.Tools) > 0 {
			sawTools.Store(true)
		}

		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			// First turn: ask for the tool.
			_, _ = w.Write([]byte(`{
				"id":"c1","object":"chat.completion","model":"test",
				"choices":[{"index":0,"finish_reason":"tool_calls","message":{
					"role":"assistant","content":"",
					"tool_calls":[{"id":"call_1","type":"function","function":{
						"name":"get_weather","arguments":"{\"city\":\"Oslo\"}"}}]}}]}`))
			return
		}
		// Second turn: final answer.
		_, _ = w.Write([]byte(`{
			"id":"c2","object":"chat.completion","model":"test",
			"choices":[{"index":0,"finish_reason":"stop","message":{
				"role":"assistant","content":"It is sunny and 21C in Oslo."}}]}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithModel("test-model"))
	tool := &echoTool{}

	out, err := client.GenerateWithTools(context.Background(), "weather in Oslo?",
		[]interfaces.Tool{tool})
	if err != nil {
		t.Fatalf("GenerateWithTools() error = %v", err)
	}

	if !sawTools.Load() {
		t.Error("no tools were declared in the request: the client is not using " +
			"the native tools API")
	}
	if got := tool.calls.Load(); got != 1 {
		t.Errorf("tool executed %d times, want 1: a requested tool call must actually run", got)
	}
	if out != "It is sunny and 21C in Oslo." {
		t.Errorf("result = %q, want the model's final answer", out)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("made %d requests, want 2 (tool request, then final answer)", got)
	}
}

// TestToolResultsArePairedWithTheirCall asserts the tool-result message carries
// the originating call's id, and that the assistant turn requesting the call
// stays in the transcript. Both the OpenAI-compatible API and vLLM reject a
// tool result whose call is missing or unpaired.
func TestToolResultsArePairedWithTheirCall(t *testing.T) {
	var secondRequest ChatRequest
	var mu sync.Mutex
	var requests atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)

		var req ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if n == 2 {
			mu.Lock()
			secondRequest = req
			mu.Unlock()
		}

		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write([]byte(`{
				"id":"c1","choices":[{"index":0,"message":{
					"role":"assistant","content":"",
					"tool_calls":[{"id":"call_abc","type":"function","function":{
						"name":"get_weather","arguments":"{\"city\":\"Oslo\"}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"c2","choices":[{"index":0,"message":{
			"role":"assistant","content":"done"}}]}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithModel("test-model"))
	if _, err := client.GenerateWithTools(context.Background(), "go",
		[]interfaces.Tool{&echoTool{}}); err != nil {
		t.Fatalf("GenerateWithTools() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	var assistantWithCall, toolResult *ChatMessage
	for i := range secondRequest.Messages {
		m := &secondRequest.Messages[i]
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			assistantWithCall = m
		}
		if m.Role == "tool" {
			toolResult = m
		}
	}

	if assistantWithCall == nil {
		t.Fatal("the assistant turn requesting the tool call was not sent back; " +
			"the tool result then references a call the model cannot see")
	}
	if toolResult == nil {
		t.Fatal("no tool-result message was sent on the follow-up turn")
	}
	if toolResult.ToolCallID != "call_abc" {
		t.Errorf("tool_call_id = %q, want %q", toolResult.ToolCallID, "call_abc")
	}
	if toolResult.Content != "sunny, 21C" {
		t.Errorf("tool result content = %q, want the tool's output", toolResult.Content)
	}
}

// TestToolArgumentsReachTheTool asserts the model's JSON arguments are passed
// through to Execute unchanged.
func TestToolArgumentsReachTheTool(t *testing.T) {
	var requests atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write([]byte(`{"id":"c1","choices":[{"index":0,"message":{
				"role":"assistant","content":"",
				"tool_calls":[{"id":"call_1","type":"function","function":{
					"name":"get_weather","arguments":"{\"city\":\"Bergen\"}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"c2","choices":[{"index":0,"message":{
			"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithModel("test-model"))
	tool := &echoTool{}

	if _, err := client.GenerateWithTools(context.Background(), "go",
		[]interfaces.Tool{tool}); err != nil {
		t.Fatalf("GenerateWithTools() error = %v", err)
	}

	args := tool.Args()
	if len(args) != 1 {
		t.Fatalf("tool received %d invocations, want 1", len(args))
	}
	if args[0] != `{"city":"Bergen"}` {
		t.Errorf("tool received args %q, want the model's JSON unchanged", args[0])
	}
}

// TestUnknownToolIsReportedToTheModel asserts a call for a tool that is not
// available is reported back rather than aborting the run, so the model can
// recover or explain.
func TestUnknownToolIsReportedToTheModel(t *testing.T) {
	var requests atomic.Int64
	var secondRequest ChatRequest
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		var req ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if n == 2 {
			mu.Lock()
			secondRequest = req
			mu.Unlock()
		}

		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write([]byte(`{"id":"c1","choices":[{"index":0,"message":{
				"role":"assistant","content":"",
				"tool_calls":[{"id":"call_1","type":"function","function":{
					"name":"nonexistent_tool","arguments":"{}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"c2","choices":[{"index":0,"message":{
			"role":"assistant","content":"sorry"}}]}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithModel("test-model"))
	out, err := client.GenerateWithTools(context.Background(), "go",
		[]interfaces.Tool{&echoTool{}})
	if err != nil {
		t.Fatalf("GenerateWithTools() should not fail on an unknown tool: %v", err)
	}
	if out != "sorry" {
		t.Errorf("result = %q, want the model's follow-up answer", out)
	}

	mu.Lock()
	defer mu.Unlock()
	var sawError bool
	for _, m := range secondRequest.Messages {
		if m.Role == "tool" && m.Content != "" {
			sawError = true
		}
	}
	if !sawError {
		t.Error("the unknown-tool error was not reported back to the model")
	}
}
