package ollama

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
// then fails.
type failingMemory struct {
	ok       int
	calls    int
	messages []interfaces.Message
}

func (m *failingMemory) AddMessage(context.Context, interfaces.Message) error { return nil }
func (m *failingMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	m.calls++
	if m.calls > m.ok {
		return nil, errors.New("memory down")
	}
	return m.messages, nil
}
func (m *failingMemory) Clear(context.Context) error { return nil }

// erroringTool always fails.
type erroringTool struct{}

func (erroringTool) Name() string        { return "broken" }
func (erroringTool) Description() string { return "always fails" }
func (erroringTool) Parameters() map[string]interfaces.ParameterSpec {
	return nil
}
func (erroringTool) Run(context.Context, string) (string, error) { return "", errors.New("boom") }
func (erroringTool) Execute(context.Context, string) (string, error) {
	return "", errors.New("boom")
}

// replayServer answers chat requests with the next response and records each
// request body as raw JSON.
type replayServer struct {
	mu        sync.Mutex
	responses []string
	bodies    []map[string]any
	paths     []string
}

func (s *replayServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bodies = append(s.bodies, body)
	s.paths = append(s.paths, r.URL.Path)
	if len(s.responses) == 0 {
		http.Error(w, "no response left", http.StatusInternalServerError)
		return
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	_, _ = w.Write([]byte(resp))
}

func newReplayClient(t *testing.T, responses ...string) (*OllamaClient, *replayServer) {
	t.Helper()
	fake := &replayServer{responses: responses}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return NewClient(WithBaseURL(server.URL), WithModel("llava")), fake
}

func TestRequestImagesReturnsMemoryError(t *testing.T) {
	// The first read passes CheckMedia; the second, for the images, fails.
	mem := &failingMemory{ok: 1}
	if _, err := requestImages(context.Background(), &interfaces.GenerateOptions{Memory: mem}); err == nil || !strings.Contains(err.Error(), "memory down") {
		t.Errorf("requestImages() error = %v, want the memory error", err)
	}
}

func TestRequestImagesFromMemoryUserMessages(t *testing.T) {
	mem := &failingMemory{ok: 10, messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Parts: []*interfaces.Part{interfaces.NewPartFromBytes([]byte{1}, "image/png"), interfaces.NewPartFromText("a")}},
		{Role: interfaces.MessageRoleAssistant, Parts: []*interfaces.Part{interfaces.NewPartFromBytes([]byte{9}, "image/png")}},
		{Role: interfaces.MessageRoleUser, Parts: []*interfaces.Part{interfaces.NewPartFromBytes([]byte{2}, "image/jpeg")}},
	}}
	images, err := requestImages(context.Background(), &interfaces.GenerateOptions{Memory: mem})
	if err != nil {
		t.Fatalf("requestImages() error = %v", err)
	}
	if len(images) != 2 || images[0] != "AQ==" || images[1] != "Ag==" {
		t.Errorf("images = %v, want the two user images in order", images)
	}
}

func TestRequestImagesRefusesNonImageBytes(t *testing.T) {
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "application/pdf"))(params)
	if _, err := requestImages(context.Background(), params); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("requestImages() error = %v, want ErrContentUnsupported", err)
	}
}

func TestRequestImagesEmptyWithoutMedia(t *testing.T) {
	images, err := requestImages(context.Background(), &interfaces.GenerateOptions{})
	if err != nil || images == nil || len(images) != 0 {
		t.Errorf("requestImages() = %#v, %v; want an empty list", images, err)
	}
}

func TestGenerateReturnsMemoryError(t *testing.T) {
	client := NewClient(WithBaseURL("http://127.0.0.1:0"))
	// The first read builds the prompt; the second, in CheckMedia, fails.
	if _, err := client.Generate(context.Background(), "x", interfaces.WithMemory(&failingMemory{ok: 1})); err == nil || !strings.Contains(err.Error(), "memory down") {
		t.Errorf("Generate() error = %v, want the memory error", err)
	}
}

func TestGenerateWithToolsRefusesUnsupportedMedia(t *testing.T) {
	client := NewClient(WithBaseURL("http://127.0.0.1:0"))
	tools := []interfaces.Tool{&mockTool{name: "t"}}
	withURL := interfaces.WithParts(interfaces.NewPartFromURI("https://example.com/a.png", "image/png"))
	if _, err := client.GenerateWithTools(context.Background(), "x", tools, withURL); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("GenerateWithTools() error = %v, want ErrContentUnsupported", err)
	}
	if _, err := client.GenerateWithTools(context.Background(), "x", tools, interfaces.WithMemory(&failingMemory{})); err == nil || !strings.Contains(err.Error(), "memory down") {
		t.Errorf("GenerateWithTools() error = %v, want the memory error", err)
	}
}

func TestGenerateRequestBody(t *testing.T) {
	client, fake := newReplayClient(t, `{"response":"done","done":true}`)
	opts := []interfaces.GenerateOption{
		WithTemperature(0.2),
		WithTopP(0.9),
		WithStopSequences([]string{"END"}),
		WithSystemMessage("be brief"),
		func(o *interfaces.GenerateOptions) { o.LLMConfig.MaxTokens = 64 },
	}
	got, err := client.Generate(context.Background(), "hello", opts...)
	if err != nil || got != "done" {
		t.Fatalf("Generate() = %q, %v; want done", got, err)
	}
	body := fake.bodies[0]
	if fake.paths[0] != "/api/generate" || body["model"] != "llava" || body["stream"] != false || body["system"] != "be brief" {
		t.Errorf("body = %v, want model llava, no stream, the system message at /api/generate", body)
	}
	if body["prompt"] != "User: hello" {
		t.Errorf("prompt = %v, want %q", body["prompt"], "User: hello")
	}
	if _, ok := body["images"]; ok {
		t.Errorf("body = %v, want no images field", body)
	}
	options := body["options"].(map[string]any)
	if options["temperature"] != 0.2 || options["top_p"] != 0.9 || options["num_predict"] != float64(64) {
		t.Errorf("options = %v, want temperature, top_p, and num_predict", options)
	}
	if stop := options["stop"].([]any); len(stop) != 1 || stop[0] != "END" {
		t.Errorf("stop = %v, want [END]", options["stop"])
	}
}

func TestGenerateJSONFormatAddsSchema(t *testing.T) {
	client, fake := newReplayClient(t, `{"response":"{}","done":true}`)
	format := WithResponseFormat(interfaces.ResponseFormat{
		Type:   interfaces.ResponseFormatJSON,
		Name:   "answer",
		Schema: interfaces.JSONSchema{"type": "object"},
	})
	if _, err := client.Generate(context.Background(), "hello", format); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	body := fake.bodies[0]
	prompt := body["prompt"].(string)
	if body["format"] != "json" || !strings.Contains(prompt, "Schema Name: answer") || !strings.Contains(prompt, `{"type":"object"}`) {
		t.Errorf("body = %v, want json format and the schema in the prompt", body)
	}
	if !strings.HasPrefix(prompt, "User: hello") {
		t.Errorf("prompt = %q, want it to start with the user prompt", prompt)
	}
}

func TestGenerateJSONFormatBadSchemaIsAnError(t *testing.T) {
	client := NewClient(WithBaseURL("http://127.0.0.1:0"))
	format := WithResponseFormat(interfaces.ResponseFormat{
		Type:   interfaces.ResponseFormatJSON,
		Schema: interfaces.JSONSchema{"bad": make(chan int)},
	})
	if _, err := client.Generate(context.Background(), "hello", format); err == nil || !strings.Contains(err.Error(), "marshal JSON schema") {
		t.Errorf("Generate() error = %v, want a schema error", err)
	}
}

func TestGenerateSendsImagesFromMemory(t *testing.T) {
	client, fake := newReplayClient(t, `{"response":"a cat","done":true}`)
	mem := &failingMemory{ok: 10, messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: "describe", Parts: []*interfaces.Part{interfaces.NewPartFromBytes([]byte{1, 2}, "image/png")}},
	}}
	if _, err := client.Generate(context.Background(), "describe", interfaces.WithMemory(mem)); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	images := fake.bodies[0]["images"].([]any)
	if len(images) != 1 || images[0] != "AQI=" {
		t.Errorf("images = %v, want the image from memory", images)
	}
}

func TestGenerateBadResponseIsAnError(t *testing.T) {
	client, _ := newReplayClient(t, `not json`)
	if _, err := client.Generate(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "unmarshal response") {
		t.Errorf("Generate() error = %v, want an unmarshal error", err)
	}
}

func TestGenerateAPIErrorStatus(t *testing.T) {
	client, _ := newReplayClient(t)
	if _, err := client.Generate(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Errorf("Generate() error = %v, want a status error", err)
	}
}

func TestGenerateDetailedAndWithToolsDetailed(t *testing.T) {
	client, _ := newReplayClient(t, `{"response":"one","done":true}`, `{"message":{"role":"assistant","content":"two"},"done":true}`)
	resp, err := client.GenerateDetailed(context.Background(), "x")
	if err != nil || resp.Content != "one" || resp.Model != "llava" || resp.Metadata["provider"] != "ollama" {
		t.Errorf("GenerateDetailed() = %+v, %v; want one from llava", resp, err)
	}
	resp, err = client.GenerateWithToolsDetailed(context.Background(), "x", []interfaces.Tool{&mockTool{name: "t"}})
	if err != nil || resp.Content != "two" || resp.Metadata["tools_used"] != true {
		t.Errorf("GenerateWithToolsDetailed() = %+v, %v; want two with tools_used", resp, err)
	}
}

func TestDetailedMethodsReturnErrors(t *testing.T) {
	client := NewClient(WithBaseURL("http://127.0.0.1:0"))
	if _, err := client.GenerateDetailed(context.Background(), "x"); err == nil {
		t.Error("GenerateDetailed() error = nil, want a request error")
	}
	if _, err := client.GenerateWithToolsDetailed(context.Background(), "x", []interfaces.Tool{&mockTool{name: "t"}}); err == nil {
		t.Error("GenerateWithToolsDetailed() error = nil, want a request error")
	}
}

func TestGenerateWithToolsWithoutToolsUsesGenerate(t *testing.T) {
	client, fake := newReplayClient(t, `{"response":"plain","done":true}`)
	got, err := client.GenerateWithTools(context.Background(), "x", nil)
	if err != nil || got != "plain" || fake.paths[0] != "/api/generate" {
		t.Errorf("GenerateWithTools(no tools) = %q, %v at %v; want plain from /api/generate", got, err, fake.paths)
	}
}

func TestGenerateWithToolsRequestBody(t *testing.T) {
	client, fake := newReplayClient(t, `{"message":{"role":"assistant","content":"ok"},"done":true}`)
	tool := &mockTool{name: "lookup", description: "look up"}
	parts := interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1, 2}, "image/png"), interfaces.NewPartFromText("what is it?"))
	opts := []interfaces.GenerateOption{parts, WithSystemMessage("be brief"), WithTopP(0.5), func(o *interfaces.GenerateOptions) { o.LLMConfig.MaxTokens = 32 }}

	if _, err := client.GenerateWithTools(context.Background(), "ignored", []interfaces.Tool{tool}, opts...); err != nil {
		t.Fatalf("GenerateWithTools() error = %v", err)
	}
	body := fake.bodies[0]
	if fake.paths[0] != "/api/chat" {
		t.Errorf("path = %s, want /api/chat", fake.paths[0])
	}
	messages := body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %v, want system and user", messages)
	}
	system := messages[0].(map[string]any)
	user := messages[1].(map[string]any)
	if system["role"] != "system" || system["content"] != "be brief" {
		t.Errorf("message 0 = %v, want the system message", system)
	}
	if user["role"] != "user" || user["content"] != "User: what is it?" {
		t.Errorf("message 1 = %v, want the text part as the prompt", user)
	}
	if images := user["images"].([]any); len(images) != 1 || images[0] != "AQI=" {
		t.Errorf("images = %v, want the image", user["images"])
	}

	tools := body["tools"].([]any)
	fn := tools[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "lookup" || fn["description"] != "look up" {
		t.Errorf("function = %v, want lookup", fn)
	}
	params := fn["parameters"].(map[string]any)
	if params["type"] != "object" || params["required"].([]any)[0] != "input" {
		t.Errorf("parameters = %v, want an object requiring input", params)
	}
	options := body["options"].(map[string]any)
	if options["top_p"] != 0.5 || options["num_predict"] != float64(32) {
		t.Errorf("options = %v, want top_p and num_predict", options)
	}
}

func TestGenerateWithToolsReportsToolError(t *testing.T) {
	client, fake := newReplayClient(t,
		`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"broken","arguments":{}}}]},"done":true}`,
		`{"message":{"role":"assistant","content":"failed"},"done":true}`,
	)
	got, err := client.GenerateWithTools(context.Background(), "x", []interfaces.Tool{erroringTool{}})
	if err != nil || got != "failed" {
		t.Fatalf("GenerateWithTools() = %q, %v; want failed", got, err)
	}
	messages := fake.bodies[1]["messages"].([]any)
	last := messages[len(messages)-1].(map[string]any)
	if last["role"] != "tool" || last["content"] != "error: boom" {
		t.Errorf("last message = %v, want the tool error", last)
	}
}

func TestGenerateWithToolsBadResponseAndAPIError(t *testing.T) {
	client, _ := newReplayClient(t, `not json`)
	tools := []interfaces.Tool{&mockTool{name: "t"}}
	if _, err := client.GenerateWithTools(context.Background(), "x", tools); err == nil || !strings.Contains(err.Error(), "unmarshal tool-chat response") {
		t.Errorf("GenerateWithTools() error = %v, want an unmarshal error", err)
	}
	if _, err := client.GenerateWithTools(context.Background(), "x", tools); err == nil || !strings.Contains(err.Error(), "failed to chat with tools") {
		t.Errorf("GenerateWithTools() error = %v, want a request error", err)
	}
}

func TestToolParametersToJSONSchema(t *testing.T) {
	schema := toolParametersToJSONSchema(map[string]interfaces.ParameterSpec{
		"tags": {
			Type:        "array",
			Description: "tags",
			Default:     []string{"a"},
			Enum:        []interface{}{"a", "b"},
			Items:       &interfaces.ParameterSpec{Type: "string", Description: "a tag", Enum: []interface{}{"a"}},
		},
		"bare": {},
	})
	if _, ok := schema["required"]; ok {
		t.Errorf("schema = %v, want no required list", schema)
	}
	props := schema["properties"].(map[string]interface{})
	tags := props["tags"].(map[string]interface{})
	for _, key := range []string{"type", "description", "default", "enum", "items"} {
		if _, ok := tags[key]; !ok {
			t.Errorf("tags = %v, want key %q", tags, key)
		}
	}
	items := tags["items"].(map[string]interface{})
	if items["type"] != "string" || items["description"] != "a tag" || items["enum"] == nil {
		t.Errorf("items = %v, want type, description, and enum", items)
	}
	if bare := props["bare"].(map[string]interface{}); len(bare) != 0 {
		t.Errorf("bare = %v, want an empty field", bare)
	}
}

func TestChatBadResponseIsAnError(t *testing.T) {
	client, _ := newReplayClient(t, `not json`)
	if _, err := client.Chat(context.Background(), nil, &llm.GenerateParams{}); err == nil || !strings.Contains(err.Error(), "unmarshal chat response") {
		t.Errorf("Chat() error = %v, want an unmarshal error", err)
	}
	if _, err := client.Chat(context.Background(), nil, &llm.GenerateParams{}); err == nil || !strings.Contains(err.Error(), "failed to chat") {
		t.Errorf("Chat() error = %v, want a request error", err)
	}
}

func TestListModelsErrors(t *testing.T) {
	client, _ := newReplayClient(t, `not json`)
	if _, err := client.ListModels(context.Background()); err == nil || !strings.Contains(err.Error(), "unmarshal models") {
		t.Errorf("ListModels() error = %v, want an unmarshal error", err)
	}
	if _, err := client.ListModels(context.Background()); err == nil || !strings.Contains(err.Error(), "failed to list models") {
		t.Errorf("ListModels() error = %v, want a request error", err)
	}
	if err := client.PullModel(context.Background(), "m"); err == nil || !strings.Contains(err.Error(), "failed to pull model m") {
		t.Errorf("PullModel() error = %v, want a request error", err)
	}
}

func TestMakeRequestWithRetry(t *testing.T) {
	client, _ := newReplayClient(t, `{"response":"ok","done":true}`)
	WithRetry(retry.WithInitialInterval(time.Millisecond), retry.WithMaxAttempts(1))(client)
	WithHTTPClient(&http.Client{Timeout: time.Second})(client)
	if got, err := client.Generate(context.Background(), "x"); err != nil || got != "ok" {
		t.Errorf("Generate() = %q, %v; want ok", got, err)
	}
}

func TestGetModel(t *testing.T) {
	if got := NewClient(WithModel("m")).GetModel(); got != "m" {
		t.Errorf("GetModel() = %q, want m", got)
	}
	if NewClient().SupportsStreaming() {
		t.Error("SupportsStreaming() = true, want false")
	}
}
