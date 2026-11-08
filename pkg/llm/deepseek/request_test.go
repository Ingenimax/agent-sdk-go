package deepseek

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
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
)

// rawRequest is a chat request as the server reads it, with each message's
// content kept raw so a test can tell a string from a list.
type rawRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCalls  []ToolCall      `json:"tool_calls"`
		ToolCallID string          `json:"tool_call_id"`
		Name       string          `json:"name"`
	} `json:"messages"`
	Tools          []Tool               `json:"tools"`
	MaxTokens      int                  `json:"max_tokens"`
	Stop           []string             `json:"stop"`
	ResponseFormat *ResponseFormatParam `json:"response_format"`
}

// chatServer answers each request with the next response and records every
// request.
type chatServer struct {
	mu        sync.Mutex
	responses []ChatCompletionResponse
	requests  []rawRequest
}

func (s *chatServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req rawRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
	if len(s.responses) == 0 {
		http.Error(w, "no response left", http.StatusInternalServerError)
		return
	}
	resp := s.responses[0]
	if len(s.responses) > 1 {
		s.responses = s.responses[1:]
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func newChatServer(t *testing.T, responses ...ChatCompletionResponse) (*DeepSeekClient, *chatServer) {
	t.Helper()
	fake := &chatServer{responses: responses}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return NewClient("key", WithBaseURL(server.URL+"/")), fake
}

func answer(text string, in, out int) ChatCompletionResponse {
	return ChatCompletionResponse{
		Model:   DefaultModel,
		Choices: []Choice{{Message: Message{Role: "assistant", Content: text}, FinishReason: "stop"}},
		Usage:   Usage{PromptTokens: in, CompletionTokens: out, TotalTokens: in + out},
	}
}

func toolCallAnswer(calls ...ToolCall) ChatCompletionResponse {
	return ChatCompletionResponse{
		Model:   DefaultModel,
		Choices: []Choice{{Message: Message{Role: "assistant", ToolCalls: calls}, FinishReason: "tool_calls"}},
		Usage:   Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}
}

func call(id, name, args string) ToolCall {
	return ToolCall{ID: id, Type: "function", Function: FunctionCall{Name: name, Arguments: args}}
}

// recordingMemory stores added messages and can fail GetMessages after ok
// calls.
type recordingMemory struct {
	mu       sync.Mutex
	messages []interfaces.Message
	added    []interfaces.Message
	ok       int
	calls    int
}

func (m *recordingMemory) AddMessage(_ context.Context, msg interfaces.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.added = append(m.added, msg)
	return nil
}
func (m *recordingMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	m.calls++
	if m.ok >= 0 && m.calls > m.ok {
		return nil, errors.New("memory down")
	}
	return m.messages, nil
}
func (m *recordingMemory) Clear(context.Context) error { return nil }

// weatherTool returns a fixed result, or an error when err is set.
type weatherTool struct {
	err  error
	mu   sync.Mutex
	args []string
}

func (w *weatherTool) Name() string        { return "weather" }
func (w *weatherTool) Description() string { return "look up the weather" }
func (w *weatherTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"city":  {Type: "string", Description: "the city", Required: true, Enum: []interface{}{"Paris"}},
		"units": {Type: "array", Default: []string{"c"}, Items: &interfaces.ParameterSpec{Type: "string", Enum: []interface{}{"c", "f"}}},
	}
}
func (w *weatherTool) Run(ctx context.Context, input string) (string, error) {
	return w.Execute(ctx, input)
}
func (w *weatherTool) Execute(_ context.Context, args string) (string, error) {
	w.mu.Lock()
	w.args = append(w.args, args)
	w.mu.Unlock()
	if w.err != nil {
		return "", w.err
	}
	return "sunny", nil
}

func TestGenerateWithPartsSendsContentList(t *testing.T) {
	client, fake := newChatServer(t, answer("a cat", 1, 2))
	parts := interfaces.WithParts(interfaces.NewPartFromText("describe"), interfaces.NewPartFromBytes([]byte{1, 2}, "image/jpeg"))

	got, err := client.Generate(context.Background(), "ignored", parts)
	if err != nil || got != "a cat" {
		t.Fatalf("Generate() = %q, %v; want a cat", got, err)
	}
	messages := fake.requests[0].Messages
	if len(messages) != 1 || messages[0].Role != "user" {
		t.Fatalf("messages = %+v, want one user message", messages)
	}
	var content []contentPart
	if err := json.Unmarshal(messages[0].Content, &content); err != nil {
		t.Fatalf("content = %s, want a list", messages[0].Content)
	}
	if len(content) != 2 || content[0].Text != "describe" || content[1].ImageURL.URL != "data:image/jpeg;base64,AQI=" {
		t.Errorf("content = %s, want the text then the image", messages[0].Content)
	}
}

func TestGenerateWithMemorySendsHistoryParts(t *testing.T) {
	client, fake := newChatServer(t, answer("blue", 1, 1))
	mem := &recordingMemory{ok: -1, messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: "describe", Parts: []*interfaces.Part{
			interfaces.NewPartFromURI("https://example.com/a.webp", "image/webp"),
			interfaces.NewPartFromText("describe"),
		}},
		{Role: interfaces.MessageRoleAssistant, Content: "a cat"},
	}}
	system := func(o *interfaces.GenerateOptions) { o.SystemMessage = "be brief" }
	if _, err := client.Generate(context.Background(), "what color?", interfaces.WithMemory(mem), system); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	messages := fake.requests[0].Messages
	if len(messages) != 4 {
		t.Fatalf("got %d messages, want system, history user, history assistant, prompt", len(messages))
	}
	if messages[0].Role != "system" || string(messages[0].Content) != `"be brief"` {
		t.Errorf("message 0 = %s %s, want the system message", messages[0].Role, messages[0].Content)
	}
	var first []contentPart
	if err := json.Unmarshal(messages[1].Content, &first); err != nil || first[0].ImageURL.URL != "https://example.com/a.webp" {
		t.Errorf("message 1 = %s, want the image URL", messages[1].Content)
	}
	if string(messages[2].Content) != `"a cat"` || string(messages[3].Content) != `"what color?"` {
		t.Errorf("messages 2 and 3 = %s, %s; want plain strings", messages[2].Content, messages[3].Content)
	}
}

func TestGenerateFailsWhenMemoryFails(t *testing.T) {
	client := NewClient("key", WithBaseURL("http://127.0.0.1:0"))
	mem := &recordingMemory{ok: 0}
	if _, err := client.Generate(context.Background(), "x", interfaces.WithMemory(mem)); err == nil || !strings.Contains(err.Error(), "memory down") {
		t.Errorf("Generate() error = %v, want the memory error", err)
	}
	if _, err := client.GenerateWithTools(context.Background(), "x", nil, interfaces.WithMemory(mem)); err == nil || !strings.Contains(err.Error(), "memory down") {
		t.Errorf("GenerateWithTools() error = %v, want the memory error", err)
	}
}

func TestGenerateNoChoicesIsAnError(t *testing.T) {
	client, _ := newChatServer(t, ChatCompletionResponse{})
	if _, err := client.Generate(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "no response") {
		t.Errorf("Generate() error = %v, want a no response error", err)
	}
	if _, err := client.GenerateWithTools(context.Background(), "x", nil); err == nil || !strings.Contains(err.Error(), "no response") {
		t.Errorf("GenerateWithTools() error = %v, want a no response error", err)
	}
}

func TestGenerateUsesRetryExecutor(t *testing.T) {
	client, _ := newChatServer(t, answer("ok", 1, 1))
	WithRetry(retry.WithInitialInterval(time.Millisecond), retry.WithMaxAttempts(1))(client)
	if got, err := client.Generate(context.Background(), "x"); err != nil || got != "ok" {
		t.Errorf("Generate() = %q, %v; want ok", got, err)
	}
}

func TestDoRequestBadJSONIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()
	client := NewClient("key", WithBaseURL(server.URL), WithHTTPClient(server.Client()), WithLogger(logging.New()), WithModel("m"))
	if _, err := client.Generate(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "failed to parse response") {
		t.Errorf("Generate() error = %v, want a parse error", err)
	}
}

func TestGenerateWithToolsDetailedRunsToolLoop(t *testing.T) {
	client, fake := newChatServer(t,
		toolCallAnswer(call("c1", "weather", `{"city":"Paris"}`), call("c2", "missing", `{}`)),
		answer("sunny in Paris", 5, 6),
	)
	tool := &weatherTool{}
	mem := &recordingMemory{ok: -1, messages: []interfaces.Message{{Role: interfaces.MessageRoleUser, Content: "weather?"}}}
	system := func(o *interfaces.GenerateOptions) { o.SystemMessage = "be brief" }
	llmConfig := func(o *interfaces.GenerateOptions) {
		o.LLMConfig.MaxTokens = 50
		o.LLMConfig.StopSequences = []string{"END"}
	}

	resp, err := client.GenerateWithToolsDetailed(context.Background(), "weather?", []interfaces.Tool{tool}, interfaces.WithMemory(mem), system, llmConfig)
	if err != nil {
		t.Fatalf("GenerateWithToolsDetailed() error = %v", err)
	}
	if resp.Content != "sunny in Paris" || resp.Metadata["iterations"] != 2 {
		t.Errorf("response = %+v, want the answer after 2 iterations", resp)
	}
	if resp.Usage.InputTokens != 6 || resp.Usage.OutputTokens != 7 || resp.Usage.TotalTokens != 13 {
		t.Errorf("usage = %+v, want 6 in, 7 out, 13 total", resp.Usage)
	}
	if len(tool.args) != 1 || tool.args[0] != `{"city":"Paris"}` {
		t.Errorf("tool args = %v, want the city", tool.args)
	}

	first := fake.requests[0]
	if len(first.Tools) != 1 || first.Tools[0].Function.Name != "weather" || first.MaxTokens != 50 || first.Stop[0] != "END" {
		t.Errorf("first request = %+v, want the tool, max tokens, and stop", first)
	}
	second := fake.requests[1].Messages
	// system, memory user, prompt, assistant tool calls, two tool results
	if len(second) != 6 {
		t.Fatalf("second request has %d messages, want 6", len(second))
	}
	if second[3].Role != "assistant" || len(second[3].ToolCalls) != 2 {
		t.Errorf("message 3 = %+v, want the assistant tool calls", second[3])
	}
	if second[4].ToolCallID != "c1" || second[4].Name != "weather" || string(second[4].Content) != `"sunny"` {
		t.Errorf("message 4 = %+v, want the weather result", second[4])
	}
	if second[5].ToolCallID != "c2" || !strings.Contains(string(second[5].Content), "not found") {
		t.Errorf("message 5 = %+v, want the not found result", second[5])
	}

	if len(mem.added) != 3 {
		t.Fatalf("memory got %d messages, want the assistant call and two results", len(mem.added))
	}
	if mem.added[0].Role != interfaces.MessageRoleAssistant || len(mem.added[0].ToolCalls) != 2 {
		t.Errorf("memory message 0 = %+v, want the assistant tool calls", mem.added[0])
	}
	for _, m := range mem.added[1:] {
		if m.Role != interfaces.MessageRoleTool || m.Metadata["tool_name"] == "" {
			t.Errorf("memory message = %+v, want a named tool result", m)
		}
	}
}

func TestGenerateWithToolsSendsPartsWithoutMemory(t *testing.T) {
	client, fake := newChatServer(t, answer("ok", 1, 1))
	parts := interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "image/gif"), interfaces.NewPartFromText("what?"))
	if _, err := client.GenerateWithTools(context.Background(), "ignored", []interfaces.Tool{&weatherTool{}}, parts); err != nil {
		t.Fatalf("GenerateWithTools() error = %v", err)
	}
	var content []contentPart
	if err := json.Unmarshal(fake.requests[0].Messages[0].Content, &content); err != nil || len(content) != 2 {
		t.Errorf("content = %s, want the image and the text", fake.requests[0].Messages[0].Content)
	}
}

func TestGenerateWithToolsToolErrorIsReported(t *testing.T) {
	client, fake := newChatServer(t, toolCallAnswer(call("c1", "weather", `{}`)), answer("sorry", 1, 1))
	tool := &weatherTool{err: errors.New("offline")}
	if _, err := client.GenerateWithTools(context.Background(), "x", []interfaces.Tool{tool}); err != nil {
		t.Fatalf("GenerateWithTools() error = %v", err)
	}
	last := fake.requests[1].Messages[len(fake.requests[1].Messages)-1]
	if !strings.Contains(string(last.Content), "Error executing tool: offline") {
		t.Errorf("tool result = %s, want the tool error", last.Content)
	}
}

func TestGenerateWithToolsMaxIterationsMakesFinalCall(t *testing.T) {
	client, fake := newChatServer(t, toolCallAnswer(call("c1", "weather", `{}`)))
	maxOne := func(o *interfaces.GenerateOptions) {
		o.MaxIterations = 1
		o.LLMConfig.MaxTokens = 10
		o.LLMConfig.StopSequences = []string{"END"}
		o.ResponseFormat = &interfaces.ResponseFormat{Schema: interfaces.JSONSchema{"type": "object"}}
	}
	resp, err := client.GenerateWithToolsDetailed(context.Background(), "x", []interfaces.Tool{&weatherTool{}}, maxOne)
	if err != nil {
		t.Fatalf("GenerateWithToolsDetailed() error = %v", err)
	}
	if resp.Metadata["max_iterations"] != true || resp.Metadata["iterations"] != 1 {
		t.Errorf("metadata = %v, want max_iterations after 1 iteration", resp.Metadata)
	}
	if len(fake.requests) != 2 {
		t.Fatalf("made %d requests, want 2", len(fake.requests))
	}
	if fake.requests[0].ResponseFormat == nil || fake.requests[0].ResponseFormat.Type != "json_schema" {
		t.Errorf("first request response format = %+v, want json_schema on the last iteration", fake.requests[0].ResponseFormat)
	}
	if len(fake.requests[1].Tools) != 0 || fake.requests[1].MaxTokens != 10 {
		t.Errorf("final request = %+v, want no tools and the max tokens", fake.requests[1])
	}
}

func TestGenerateWithToolsFinalCallErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rawRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Tools) == 0 {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(toolCallAnswer(call("c1", "weather", `{}`)))
	}))
	defer server.Close()
	client := NewClient("key", WithBaseURL(server.URL))
	maxOne := func(o *interfaces.GenerateOptions) { o.MaxIterations = 1 }
	_, err := client.GenerateWithTools(context.Background(), "x", []interfaces.Tool{&weatherTool{}}, maxOne)
	if err == nil || !strings.Contains(err.Error(), "failed to make final request") {
		t.Errorf("GenerateWithTools() error = %v, want a final request error", err)
	}
}

func TestGenerateWithToolsAPIError(t *testing.T) {
	client := NewClient("key", WithBaseURL("http://127.0.0.1:0"))
	if _, err := client.GenerateWithTools(context.Background(), "x", nil); err == nil || !strings.Contains(err.Error(), "failed to generate text with tools") {
		t.Errorf("GenerateWithTools() error = %v, want a request error", err)
	}
}

func TestGenerateWithToolsStopsWhenContextIsDone(t *testing.T) {
	client := NewClient("key", WithBaseURL("http://127.0.0.1:0"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.GenerateWithTools(ctx, "x", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("GenerateWithTools() error = %v, want context.Canceled", err)
	}
}

func TestConvertToolsToDeepSeekFormat(t *testing.T) {
	tools := NewClient("key").convertToolsToDeepSeekFormat([]interfaces.Tool{&weatherTool{}})
	if len(tools) != 1 || tools[0].Type != "function" || tools[0].Function.Name != "weather" {
		t.Fatalf("tools = %+v, want the weather function", tools)
	}
	params := tools[0].Function.Parameters.(map[string]interface{})
	if required := params["required"].([]string); len(required) != 1 || required[0] != "city" {
		t.Errorf("required = %v, want [city]", params["required"])
	}
	props := params["properties"].(map[string]interface{})
	if city := props["city"].(map[string]interface{}); city["enum"] == nil {
		t.Errorf("city = %v, want an enum", city)
	}
	units := props["units"].(map[string]interface{})
	if units["default"] == nil || units["items"].(map[string]interface{})["enum"] == nil {
		t.Errorf("units = %v, want a default and an items enum", units)
	}
}

func TestContentPartsSkipsEmptyText(t *testing.T) {
	got := contentParts([]*interfaces.Part{interfaces.NewPartFromText(""), interfaces.NewPartFromText("hi")})
	if len(got) != 1 || got[0].Text != "hi" {
		t.Errorf("contentParts() = %+v, want only the non-empty text", got)
	}
}

func TestCheckContentAcceptsEveryImageType(t *testing.T) {
	for _, mime := range imageTypes {
		params := &interfaces.GenerateOptions{}
		interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, mime))(params)
		if err := checkContent(context.Background(), params); err != nil {
			t.Errorf("checkContent(%s) = %v", mime, err)
		}
	}
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(interfaces.NewPartFromURI("https://example.com/a.svg", "image/svg+xml"))(params)
	if err := checkContent(context.Background(), params); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("checkContent(svg) = %v, want ErrContentUnsupported", err)
	}
}

func TestBuildMessagesContinuesWhenMemoryFails(t *testing.T) {
	builder := newMessageHistoryBuilder(logging.New())
	parts := []*interfaces.Part{interfaces.NewPartFromBytes([]byte{1}, "image/png")}
	messages := builder.buildMessages(context.Background(), "hi", &recordingMemory{ok: 0}, parts)
	if len(messages) != 1 || messages[0].Content != "hi" || len(messages[0].Parts) != 0 {
		t.Errorf("messages = %+v, want only the prompt, without parts", messages)
	}
}

func TestConvertMemoryMessageKeepsPartsOnlyForUsers(t *testing.T) {
	builder := newMessageHistoryBuilder(logging.New())
	parts := []*interfaces.Part{interfaces.NewPartFromText("x")}
	if got := builder.convertMemoryMessage(interfaces.Message{Role: interfaces.MessageRoleUser, Parts: parts}); len(got.Parts) != 1 {
		t.Errorf("user message parts = %v, want the part", got.Parts)
	}
	if got := builder.convertMemoryMessage(interfaces.Message{Role: interfaces.MessageRoleAssistant, Content: "a", Parts: parts}); len(got.Parts) != 0 {
		t.Errorf("assistant message parts = %v, want none", got.Parts)
	}
	tool := builder.convertMemoryMessage(interfaces.Message{Role: interfaces.MessageRoleTool, ToolCallID: "c1", Content: "r"})
	if tool.ToolCallID != "c1" || tool.Name != "" {
		t.Errorf("tool message = %+v, want the call id and no name without metadata", tool)
	}
}
