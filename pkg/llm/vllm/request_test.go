package vllm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
)

// failingMemory returns its messages for the first ok GetMessages calls,
// then fails. It records added messages.
type failingMemory struct {
	ok       int
	calls    int
	messages []interfaces.Message
	mu       sync.Mutex
	added    []interfaces.Message
}

func (m *failingMemory) AddMessage(_ context.Context, msg interfaces.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.added = append(m.added, msg)
	return nil
}
func (m *failingMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	m.calls++
	if m.calls > m.ok {
		return nil, errors.New("memory down")
	}
	return m.messages, nil
}
func (m *failingMemory) Clear(context.Context) error { return nil }

// failTool always fails.
type failTool struct{}

func (failTool) Name() string                                    { return "broken" }
func (failTool) Description() string                             { return "always fails" }
func (failTool) Parameters() map[string]interfaces.ParameterSpec { return nil }
func (failTool) Run(context.Context, string) (string, error)     { return "", errors.New("boom") }
func (failTool) Execute(context.Context, string) (string, error) { return "", errors.New("boom") }

// replayServer answers with the next canned body and records each request
// path and JSON body.
type replayServer struct {
	mu        sync.Mutex
	responses []string
	paths     []string
	bodies    []map[string]any
}

func (s *replayServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paths = append(s.paths, r.Method+" "+r.URL.Path)
	s.bodies = append(s.bodies, body)
	if len(s.responses) == 0 {
		http.Error(w, "no response left", http.StatusInternalServerError)
		return
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	_, _ = w.Write([]byte(resp))
}

func newReplayClient(t *testing.T, responses ...string) (*VLLMClient, *replayServer) {
	t.Helper()
	fake := &replayServer{responses: responses}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return NewClient(WithBaseURL(server.URL), WithModel("m")), fake
}

const (
	completionOK = `{"choices":[{"text":"done"}]}`
	chatOK       = `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`
)

func toolCallsReply(content string, calls ...string) string {
	return `{"choices":[{"message":{"role":"assistant","content":"` + content + `","tool_calls":[` + strings.Join(calls, ",") + `]}}]}`
}

func toolCall(id, name, args string) string {
	raw, _ := json.Marshal(ToolCall{ID: id, Type: "function", Function: ToolCallFunction{Name: name, Arguments: args}})
	return string(raw)
}

func TestGenerateReturnsMediaMemoryError(t *testing.T) {
	client := NewClient(WithBaseURL("http://127.0.0.1:0"))
	// The first read passes checkContent; the second, for the media, fails.
	if _, err := client.Generate(context.Background(), "x", interfaces.WithMemory(&failingMemory{ok: 1})); err == nil || !strings.Contains(err.Error(), "memory down") {
		t.Errorf("Generate() error = %v, want the memory error", err)
	}
	if _, err := client.Generate(context.Background(), "x", interfaces.WithMemory(&failingMemory{})); err == nil || !strings.Contains(err.Error(), "memory down") {
		t.Errorf("Generate() error = %v, want the memory error", err)
	}
}

func TestGenerateWithToolsReturnsHistoryMemoryError(t *testing.T) {
	client := NewClient(WithBaseURL("http://127.0.0.1:0"))
	tools := []interfaces.Tool{&echoTool{}}
	// The first read passes checkContent; the second, for the history, fails.
	if _, err := client.GenerateWithTools(context.Background(), "x", tools, interfaces.WithMemory(&failingMemory{ok: 1})); err == nil || !strings.Contains(err.Error(), "failed to read memory") {
		t.Errorf("GenerateWithTools() error = %v, want the memory error", err)
	}
}

func TestChatMessagesReturnsMemoryError(t *testing.T) {
	_, err := chatMessages(context.Background(), "x", &interfaces.GenerateOptions{Memory: &failingMemory{}})
	if err == nil || !strings.Contains(err.Error(), "failed to read memory: memory down") {
		t.Errorf("chatMessages() error = %v, want the memory error", err)
	}
}

func TestChatMessagesWithSystemMessage(t *testing.T) {
	params := &interfaces.GenerateOptions{SystemMessage: "be brief"}
	interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "image/png"), interfaces.NewPartFromText("what?"))(params)
	messages, err := chatMessages(context.Background(), "ignored", params)
	if err != nil {
		t.Fatalf("chatMessages() error = %v", err)
	}
	if len(messages) != 2 || messages[0].Role != "system" || messages[0].Content != "be brief" {
		t.Fatalf("messages = %+v, want the system message first", messages)
	}
	if messages[1].Content != "what?" || len(messages[1].Parts) != 2 {
		t.Errorf("message 1 = %+v, want the text part and the parts", messages[1])
	}
}

func TestChatMessagesFromMemoryLeavesOutToolTurns(t *testing.T) {
	mem := &failingMemory{ok: 1, messages: []interfaces.Message{
		{Role: interfaces.MessageRoleSystem, Content: "old system"},
		{Role: interfaces.MessageRoleUser, Content: "weather?"},
		{Role: interfaces.MessageRoleAssistant, Content: "calling", ToolCalls: []interfaces.ToolCall{{ID: "1", Name: "w"}}},
		{Role: interfaces.MessageRoleTool, Content: "sunny", ToolCallID: "1"},
		{Role: interfaces.MessageRoleAssistant, Content: ""},
		{Role: interfaces.MessageRoleAssistant, Content: "it is sunny"},
	}}
	messages, err := chatMessages(context.Background(), "ignored", &interfaces.GenerateOptions{Memory: mem})
	if err != nil {
		t.Fatalf("chatMessages() error = %v", err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Content != "it is sunny" {
		t.Errorf("messages = %+v, want the user message and the text reply", messages)
	}
}

func TestContentPartsSkipsEmptyText(t *testing.T) {
	got := contentParts([]*interfaces.Part{interfaces.NewPartFromText(""), interfaces.NewPartFromText("hi")})
	if len(got) != 1 || got[0].Type != "text" || got[0].Text != "hi" {
		t.Errorf("contentParts() = %+v, want only the non-empty text", got)
	}
}

func TestChatMessageMarshalKeepsToolFields(t *testing.T) {
	raw, err := json.Marshal(ChatMessage{Role: "tool", Content: "r", ToolCallID: "c1"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(raw) != `{"role":"tool","content":"r","tool_call_id":"c1"}` {
		t.Errorf("Marshal() = %s", raw)
	}
}

func TestCheckContentAcceptsAnyImage(t *testing.T) {
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "image/bmp"), interfaces.NewPartFromURI("https://example.com/a.tiff", "image/tiff"))(params)
	if err := checkContent(context.Background(), params); err != nil {
		t.Errorf("checkContent() = %v, want nil for any image type", err)
	}
}

func TestGenerateTextUsesCompletionsEndpoint(t *testing.T) {
	client, fake := newReplayClient(t, completionOK)
	opts := []interfaces.GenerateOption{
		WithTemperature(0.3),
		WithTopP(0.8),
		WithStopSequences([]string{"END"}),
		func(o *interfaces.GenerateOptions) { o.LLMConfig.MaxTokens = 16 },
		interfaces.WithParts(interfaces.NewPartFromText("from parts")),
	}
	got, err := client.Generate(context.Background(), "ignored", opts...)
	if err != nil || got != "done" {
		t.Fatalf("Generate() = %q, %v; want done", got, err)
	}
	if fake.paths[0] != "POST /v1/completions" {
		t.Errorf("path = %s, want POST /v1/completions", fake.paths[0])
	}
	body := fake.bodies[0]
	if body["prompt"] != "User: from parts" || body["model"] != "m" || body["max_tokens"] != float64(16) {
		t.Errorf("body = %v, want the text part as the prompt, the model, and max tokens", body)
	}
	if body["temperature"] != 0.3 || body["top_p"] != 0.8 || body["stop"].([]any)[0] != "END" {
		t.Errorf("body = %v, want temperature, top_p, and stop", body)
	}
}

func TestGenerateTextWithMemoryInlinesHistory(t *testing.T) {
	client, fake := newReplayClient(t, completionOK)
	mem := &failingMemory{ok: 10, messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: "hi"},
		{Role: interfaces.MessageRoleAssistant, Content: "hello"},
	}}
	if _, err := client.Generate(context.Background(), "next", interfaces.WithMemory(mem)); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	prompt := fake.bodies[0]["prompt"].(string)
	if !strings.Contains(prompt, "User: hi") || !strings.Contains(prompt, "Assistant: hello") {
		t.Errorf("prompt = %q, want the inlined history", prompt)
	}
}

func TestGenerateJSONFormatAddsSchema(t *testing.T) {
	client, fake := newReplayClient(t, completionOK)
	format := WithResponseFormat(interfaces.ResponseFormat{Type: interfaces.ResponseFormatJSON, Name: "answer", Schema: interfaces.JSONSchema{"type": "object"}})
	if _, err := client.Generate(context.Background(), "hello", format); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	prompt := fake.bodies[0]["prompt"].(string)
	if !strings.HasPrefix(prompt, "hello\n") || !strings.Contains(prompt, "Schema Name: answer") || !strings.Contains(prompt, `{"type":"object"}`) {
		t.Errorf("prompt = %q, want the prompt and the schema", prompt)
	}
}

func TestGenerateJSONFormatBadSchemaIsAnError(t *testing.T) {
	client := NewClient(WithBaseURL("http://127.0.0.1:0"))
	format := WithResponseFormat(interfaces.ResponseFormat{Type: interfaces.ResponseFormatJSON, Schema: interfaces.JSONSchema{"bad": make(chan int)}})
	if _, err := client.Generate(context.Background(), "hello", format); err == nil || !strings.Contains(err.Error(), "marshal JSON schema") {
		t.Errorf("Generate() error = %v, want a schema error", err)
	}
}

func TestGenerateCompletionErrors(t *testing.T) {
	client, _ := newReplayClient(t, `{"choices":[]}`, `not json`)
	if _, err := client.Generate(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Errorf("Generate() error = %v, want a no choices error", err)
	}
	if _, err := client.Generate(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "unmarshal response") {
		t.Errorf("Generate() error = %v, want an unmarshal error", err)
	}
	if _, err := client.Generate(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Errorf("Generate() error = %v, want a status error", err)
	}
}

func TestGenerateWithToolsWithoutToolsUsesCompletions(t *testing.T) {
	client, fake := newReplayClient(t, completionOK)
	if got, err := client.GenerateWithTools(context.Background(), "x", nil); err != nil || got != "done" || fake.paths[0] != "POST /v1/completions" {
		t.Errorf("GenerateWithTools(no tools) = %q, %v at %v; want done from completions", got, err, fake.paths)
	}
}

func TestChatToolLoopStoresExchangesInMemory(t *testing.T) {
	client, fake := newReplayClient(t,
		toolCallsReply("checking", toolCall("", "get_weather", `{"city":"Paris"}`), toolCall("c2", "broken", `{}`), toolCall("c3", "missing", `{}`)),
		chatOK,
	)
	mem := &failingMemory{ok: 10, messages: []interfaces.Message{{Role: interfaces.MessageRoleUser, Content: "weather?"}}}
	tools := []interfaces.Tool{&echoTool{}, failTool{}}
	maxTokens := func(o *interfaces.GenerateOptions) { o.LLMConfig.MaxTokens = 8 }

	got, err := client.GenerateWithTools(context.Background(), "weather?", tools, interfaces.WithMemory(mem), maxTokens)
	if err != nil || got != "ok" {
		t.Fatalf("GenerateWithTools() = %q, %v; want ok", got, err)
	}
	if fake.bodies[0]["max_tokens"] != float64(8) {
		t.Errorf("max_tokens = %v, want 8", fake.bodies[0]["max_tokens"])
	}

	messages := fake.bodies[1]["messages"].([]any)
	// user, assistant with calls, three tool results
	if len(messages) != 5 {
		t.Fatalf("second request has %d messages, want 5", len(messages))
	}
	results := make([]map[string]any, 0, 3)
	for _, m := range messages[2:] {
		results = append(results, m.(map[string]any))
	}
	if results[0]["tool_call_id"] != "vllm:get_weather:0" || results[0]["content"] != "sunny, 21C" {
		t.Errorf("result 0 = %v, want a synthesized id and the tool output", results[0])
	}
	if results[1]["tool_call_id"] != "c2" || results[1]["content"] != "Error: boom" {
		t.Errorf("result 1 = %v, want the tool error", results[1])
	}
	if results[2]["tool_call_id"] != "c3" || !strings.Contains(results[2]["content"].(string), "not available") {
		t.Errorf("result 2 = %v, want the missing tool message", results[2])
	}

	if len(mem.added) != 4 {
		t.Fatalf("memory got %d messages, want the assistant call and three results", len(mem.added))
	}
	if mem.added[0].Role != interfaces.MessageRoleAssistant || len(mem.added[0].ToolCalls) != 3 || mem.added[0].Content != "checking" {
		t.Errorf("memory message 0 = %+v, want the assistant tool calls", mem.added[0])
	}
	if mem.added[1].Role != interfaces.MessageRoleTool || mem.added[1].ToolCallID != "vllm:get_weather:0" {
		t.Errorf("memory message 1 = %+v, want the first tool result", mem.added[1])
	}
}

func TestChatOutOfIterationsReturnsLastAssistantText(t *testing.T) {
	client, _ := newReplayClient(t, toolCallsReply("still working", toolCall("c1", "get_weather", `{}`)))
	maxOne := func(o *interfaces.GenerateOptions) { o.MaxIterations = 1 }
	got, err := client.GenerateWithTools(context.Background(), "x", []interfaces.Tool{&echoTool{}}, maxOne)
	if err != nil || got != "still working" {
		t.Errorf("GenerateWithTools() = %q, %v; want the last assistant text", got, err)
	}
}

func TestChatOutOfIterationsWithoutTextIsAnError(t *testing.T) {
	client, _ := newReplayClient(t, toolCallsReply("", toolCall("c1", "get_weather", `{}`)))
	maxOne := func(o *interfaces.GenerateOptions) { o.MaxIterations = 1 }
	_, err := client.GenerateWithTools(context.Background(), "x", []interfaces.Tool{&echoTool{}}, maxOne)
	if err == nil || !strings.Contains(err.Error(), "exhausted 1 iterations") {
		t.Errorf("GenerateWithTools() error = %v, want an exhausted error", err)
	}
}

func TestChatErrors(t *testing.T) {
	client, _ := newReplayClient(t, `{"choices":[]}`, `not json`)
	tools := []interfaces.Tool{&echoTool{}}
	if _, err := client.GenerateWithTools(context.Background(), "x", tools); err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Errorf("GenerateWithTools() error = %v, want a no choices error", err)
	}
	if _, err := client.GenerateWithTools(context.Background(), "x", tools); err == nil || !strings.Contains(err.Error(), "unmarshal tool-chat response") {
		t.Errorf("GenerateWithTools() error = %v, want an unmarshal error", err)
	}
	if _, err := client.GenerateWithTools(context.Background(), "x", tools); err == nil || !strings.Contains(err.Error(), "failed to chat with tools") {
		t.Errorf("GenerateWithTools() error = %v, want a request error", err)
	}
}

func TestChatStopsWhenContextIsDone(t *testing.T) {
	client, fake := newReplayClient(t, chatOK)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.GenerateWithTools(ctx, "x", []interfaces.Tool{&echoTool{}}); !errors.Is(err, context.Canceled) {
		t.Errorf("GenerateWithTools() error = %v, want context.Canceled", err)
	}
	if len(fake.paths) != 0 {
		t.Errorf("made %d requests, want none", len(fake.paths))
	}
}

func TestToolParametersSchema(t *testing.T) {
	tool := &schemaTool{params: map[string]interfaces.ParameterSpec{
		"tags": {Type: "array", Description: "tags", Default: []string{"a"}, Enum: []interface{}{"a"}, Required: true,
			Items: &interfaces.ParameterSpec{Type: "string", Enum: []interface{}{"a", "b"}}},
	}}
	schema := toolParametersSchema(tool)
	if required := schema["required"].([]string); len(required) != 1 || required[0] != "tags" {
		t.Errorf("required = %v, want [tags]", schema["required"])
	}
	tags := schema["properties"].(map[string]interface{})["tags"].(map[string]interface{})
	if tags["default"] == nil || tags["enum"] == nil || tags["items"].(map[string]interface{})["enum"] == nil {
		t.Errorf("tags = %v, want a default, an enum, and an items enum", tags)
	}

	empty := toolParametersSchema(&schemaTool{})
	if _, ok := empty["required"]; ok {
		t.Errorf("schema = %v, want no required list", empty)
	}
}

type schemaTool struct {
	params map[string]interfaces.ParameterSpec
}

func (s *schemaTool) Name() string                                    { return "schema" }
func (s *schemaTool) Description() string                             { return "schema" }
func (s *schemaTool) Parameters() map[string]interfaces.ParameterSpec { return s.params }
func (s *schemaTool) Run(context.Context, string) (string, error)     { return "", nil }
func (s *schemaTool) Execute(context.Context, string) (string, error) { return "", nil }

func TestChatSendsConvertedMessages(t *testing.T) {
	client, fake := newReplayClient(t, chatOK)
	params := &llm.GenerateParams{Temperature: 0.1, TopP: 0.2, StopSequences: []string{"END"}, MaxTokens: 5}
	got, err := client.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, params)
	if err != nil || got != "ok" {
		t.Fatalf("Chat() = %q, %v; want ok", got, err)
	}
	body := fake.bodies[0]
	if fake.paths[0] != "POST /v1/chat/completions" || body["max_tokens"] != float64(5) || body["temperature"] != 0.1 {
		t.Errorf("body = %v at %s, want the chat endpoint with the params", body, fake.paths[0])
	}
	messages := body["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["content"] != "hi" {
		t.Errorf("messages = %v, want the user message", messages)
	}
}

func TestChatNilParamsUsesDefaults(t *testing.T) {
	client, _ := newReplayClient(t, chatOK)
	if got, err := client.Chat(context.Background(), nil, nil); err != nil || got != "ok" {
		t.Errorf("Chat(nil params) = %q, %v; want ok", got, err)
	}
}

func TestChatMethodErrors(t *testing.T) {
	client, _ := newReplayClient(t, `{"choices":[]}`, `not json`)
	if _, err := client.Chat(context.Background(), nil, nil); err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Errorf("Chat() error = %v, want a no choices error", err)
	}
	if _, err := client.Chat(context.Background(), nil, nil); err == nil || !strings.Contains(err.Error(), "unmarshal chat response") {
		t.Errorf("Chat() error = %v, want an unmarshal error", err)
	}
	if _, err := client.Chat(context.Background(), nil, nil); err == nil || !strings.Contains(err.Error(), "failed to chat") {
		t.Errorf("Chat() error = %v, want a request error", err)
	}
}

func TestListModels(t *testing.T) {
	client, fake := newReplayClient(t, `{"object":"list","data":[{"id":"a"},{"id":"b"}]}`, `not json`)
	models, err := client.ListModels(context.Background())
	if err != nil || len(models) != 2 || models[0] != "a" || models[1] != "b" {
		t.Errorf("ListModels() = %v, %v; want [a b]", models, err)
	}
	if fake.paths[0] != "GET /v1/models" {
		t.Errorf("path = %s, want GET /v1/models", fake.paths[0])
	}
	if _, err := client.ListModels(context.Background()); err == nil || !strings.Contains(err.Error(), "unmarshal models") {
		t.Errorf("ListModels() error = %v, want an unmarshal error", err)
	}
	if _, err := client.ListModels(context.Background()); err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Errorf("ListModels() error = %v, want a status error", err)
	}
}

func TestListModelsUnreachable(t *testing.T) {
	client := NewClient(WithBaseURL("http://127.0.0.1:0"))
	if _, err := client.ListModels(context.Background()); err == nil || !strings.Contains(err.Error(), "failed to execute request") {
		t.Errorf("ListModels() error = %v, want a request error", err)
	}
}

func TestRequestsWithRetry(t *testing.T) {
	client, _ := newReplayClient(t, completionOK, `{"data":[{"id":"a"}]}`)
	WithRetry(retry.WithInitialInterval(time.Millisecond), retry.WithMaxAttempts(1))(client)
	if got, err := client.Generate(context.Background(), "x"); err != nil || got != "done" {
		t.Errorf("Generate() = %q, %v; want done", got, err)
	}
	if models, err := client.ListModels(context.Background()); err != nil || len(models) != 1 {
		t.Errorf("ListModels() = %v, %v; want [a]", models, err)
	}
}

func TestDetailedMethods(t *testing.T) {
	client, _ := newReplayClient(t, completionOK, chatOK)
	resp, err := client.GenerateDetailed(context.Background(), "x")
	if err != nil || resp.Content != "done" || resp.Model != "m" || resp.Metadata["provider"] != "vllm" {
		t.Errorf("GenerateDetailed() = %+v, %v; want done from m", resp, err)
	}
	resp, err = client.GenerateWithToolsDetailed(context.Background(), "x", []interfaces.Tool{&echoTool{}})
	if err != nil || resp.Content != "ok" || resp.Metadata["tools_used"] != true {
		t.Errorf("GenerateWithToolsDetailed() = %+v, %v; want ok with tools_used", resp, err)
	}
	if _, err := client.GenerateDetailed(context.Background(), "x"); err == nil {
		t.Error("GenerateDetailed() error = nil, want a status error")
	}
	if _, err := client.GenerateWithToolsDetailed(context.Background(), "x", []interfaces.Tool{&echoTool{}}); err == nil {
		t.Error("GenerateWithToolsDetailed() error = nil, want a status error")
	}
}

func TestGetModelAndStreaming(t *testing.T) {
	client := NewClient(WithModel("x"))
	if client.GetModel() != "x" || client.SupportsStreaming() {
		t.Errorf("GetModel() = %q, SupportsStreaming() = %v; want x and false", client.GetModel(), client.SupportsStreaming())
	}
}
