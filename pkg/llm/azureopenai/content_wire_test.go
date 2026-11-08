package azureopenai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// unreachable is a base URL no request can reach. A test that uses it
// proves the call returned before sending anything.
const unreachable = "http://127.0.0.1:0"

// wireTool is a tool with no parameters, for the tool-calling entry points.
type wireTool struct{}

func (wireTool) Name() string        { return "noop" }
func (wireTool) Description() string { return "does nothing" }
func (wireTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{}
}
func (wireTool) Run(context.Context, string) (string, error)     { return "ok", nil }
func (wireTool) Execute(context.Context, string) (string, error) { return "ok", nil }

func drain(events <-chan interfaces.StreamEvent) {
	if events == nil {
		return
	}
	for range events {
	}
}

// entryPoints calls every public generate method with the options and
// returns each error by method name. Streams are drained.
func entryPoints(client *AzureOpenAIClient, options ...interfaces.GenerateOption) map[string]error {
	ctx := context.Background()
	tools := []interfaces.Tool{wireTool{}}
	errs := map[string]error{}

	_, errs["Generate"] = client.Generate(ctx, "hi", options...)
	_, errs["GenerateDetailed"] = client.GenerateDetailed(ctx, "hi", options...)
	_, errs["GenerateWithTools"] = client.GenerateWithTools(ctx, "hi", tools, options...)
	_, errs["GenerateWithToolsDetailed"] = client.GenerateWithToolsDetailed(ctx, "hi", tools, options...)

	stream, err := client.GenerateStream(ctx, "hi", options...)
	errs["GenerateStream"] = err
	drain(stream)

	toolStream, err := client.GenerateWithToolsStream(ctx, "hi", tools, options...)
	errs["GenerateWithToolsStream"] = err
	drain(toolStream)

	return errs
}

func TestEntryPointsRefuseUnsupportedContent(t *testing.T) {
	client := NewClient("test-key", unreachable, "gpt-4o")
	cases := map[string]interfaces.GenerateOption{
		"audio":      interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "audio/wav")),
		"inline PDF": interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "application/pdf")),
		"memory": interfaces.WithMemory(contentMemory{messages: []interfaces.Message{{
			Role:  interfaces.MessageRoleUser,
			Parts: []*interfaces.Part{interfaces.NewPartFromBytes([]byte{1}, "audio/wav")},
		}}}),
	}

	for name, option := range cases {
		for method, err := range entryPoints(client, option) {
			if !errors.Is(err, interfaces.ErrContentUnsupported) {
				t.Errorf("%s: %s() error = %v, want ErrContentUnsupported", name, method, err)
			}
		}
	}
}

// recorder is a fake Azure OpenAI deployment. It records every request path
// and body and answers with a text completion, as JSON or as an SSE stream
// when the request asks to stream.
type recorder struct {
	mu     sync.Mutex
	paths  []string
	bodies []map[string]interface{}
}

func (rec *recorder) serve(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("request body is not JSON: %s", raw)
		}
		rec.mu.Lock()
		rec.paths = append(rec.paths, r.URL.String())
		rec.bodies = append(rec.bodies, body)
		rec.mu.Unlock()

		if stream, _ := body["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: "+`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`+"\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func (rec *recorder) first(t *testing.T) map[string]interface{} {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.bodies) == 0 {
		t.Fatal("server received no request")
	}
	return rec.bodies[0]
}

func TestEntryPointsSendPartsOnTheWire(t *testing.T) {
	ctx := context.Background()
	tools := []interfaces.Tool{wireTool{}}
	parts := interfaces.WithParts(
		interfaces.NewPartFromURI("https://example.com/a.png", "image/png"),
		interfaces.NewPartFromBytes([]byte{1, 2}, "image/jpeg"),
		interfaces.NewPartFromText("describe"),
	)
	system := WithSystemMessage("be brief")
	calls := map[string]func(*AzureOpenAIClient) error{
		"Generate": func(c *AzureOpenAIClient) error {
			_, err := c.Generate(ctx, "describe", parts, system)
			return err
		},
		"GenerateWithTools": func(c *AzureOpenAIClient) error {
			_, err := c.GenerateWithTools(ctx, "describe", tools, parts, system)
			return err
		},
		"GenerateStream": func(c *AzureOpenAIClient) error {
			events, err := c.GenerateStream(ctx, "describe", parts, system)
			drain(events)
			return err
		},
		"GenerateWithToolsStream": func(c *AzureOpenAIClient) error {
			events, err := c.GenerateWithToolsStream(ctx, "describe", tools, parts, system)
			drain(events)
			return err
		},
	}

	for method, call := range calls {
		t.Run(method, func(t *testing.T) {
			rec := &recorder{}
			server := rec.serve(t)
			client := NewClient("test-key", server.URL, "gpt-4o")
			if err := call(client); err != nil {
				t.Fatalf("%s() error = %v", method, err)
			}
			body := rec.first(t)
			if !strings.HasPrefix(rec.paths[0], "/openai/deployments/gpt-4o/chat/completions?api-version=") {
				t.Errorf("path = %s, want the deployment chat completions path", rec.paths[0])
			}

			messages, _ := body["messages"].([]interface{})
			if len(messages) != 2 {
				t.Fatalf("messages = %v, want the system message and the user turn", messages)
			}
			if first, _ := messages[0].(map[string]interface{}); first["role"] != "system" || first["content"] != "be brief" {
				t.Errorf("message 0 = %v, want the system message", first)
			}
			user, _ := messages[1].(map[string]interface{})
			list, ok := user["content"].([]interface{})
			if !ok || len(list) != 3 {
				t.Fatalf("user content = %v, want three parts", user["content"])
			}
			urls := make([]interface{}, 0, 2)
			for _, p := range list[:2] {
				part, _ := p.(map[string]interface{})
				image, _ := part["image_url"].(map[string]interface{})
				if part["type"] != "image_url" {
					t.Errorf("part = %v, want an image_url part", part)
				}
				urls = append(urls, image["url"])
			}
			if urls[0] != "https://example.com/a.png" || urls[1] != "data:image/jpeg;base64,AQI=" {
				t.Errorf("image URLs = %v, want the URL then the data URL", urls)
			}
			if text, _ := list[2].(map[string]interface{}); text["type"] != "text" || text["text"] != "describe" {
				t.Errorf("part 2 = %v, want the text", text)
			}
		})
	}
}

func TestGenerateWithoutPartsSendsStringContent(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", server.URL, "gpt-4o")

	got, err := client.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got != "done" {
		t.Errorf("Generate() = %q, want %q", got, "done")
	}
	body := rec.first(t)
	if body["model"] != "gpt-4o" {
		t.Errorf("model = %v, want the deployment name", body["model"])
	}
	messages, _ := body["messages"].([]interface{})
	if msg, _ := messages[0].(map[string]interface{}); len(messages) != 1 || msg["content"] != "hello" {
		t.Errorf("messages = %v, want one plain string turn", messages)
	}
}

func TestGenerateIgnoresPartsWhenMemoryIsSet(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", server.URL, "gpt-4o")
	mem := contentMemory{messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: "from memory"},
	}}

	_, err := client.Generate(context.Background(), "prompt",
		interfaces.WithMemory(mem),
		interfaces.WithParts(interfaces.NewPartFromText("from parts")),
	)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	raw, _ := json.Marshal(rec.first(t)["messages"])
	if !strings.Contains(string(raw), "from memory") || strings.Contains(string(raw), "from parts") {
		t.Errorf("messages = %s, want the memory turn only", raw)
	}
}

func TestGenerateStreamDeliversText(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", server.URL, "gpt-4o")

	events, err := client.GenerateStream(context.Background(), "hi")
	if err != nil {
		t.Fatalf("GenerateStream() error = %v", err)
	}
	var text strings.Builder
	for e := range events {
		if e.Type == interfaces.StreamEventError {
			t.Fatalf("stream error: %v", e.Error)
		}
		if e.Type == interfaces.StreamEventContentDelta {
			text.WriteString(e.Content)
		}
	}
	if text.String() != "done" {
		t.Errorf("streamed text = %q, want %q", text.String(), "done")
	}
	if rec.first(t)["stream"] != true {
		t.Errorf("stream = %v, want true", rec.first(t)["stream"])
	}
}

func TestUserMessageSendsInlineImageAsDataURL(t *testing.T) {
	raw, err := json.Marshal(userMessage("", []*interfaces.Part{interfaces.NewPartFromBytes([]byte{1}, "image/webp")}))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(raw), `"url":"data:image/webp;base64,AQ=="`) {
		t.Errorf("message = %s, want the image as a data URL", raw)
	}
}

func TestCheckContentAcceptsEveryImageType(t *testing.T) {
	for _, mime := range imageTypes {
		params := &interfaces.GenerateOptions{}
		interfaces.WithParts(
			interfaces.NewPartFromBytes([]byte{1}, mime),
			interfaces.NewPartFromURI("https://example.com/a", mime),
		)(params)
		if err := checkContent(context.Background(), params); err != nil {
			t.Errorf("checkContent(%s) = %v", mime, err)
		}
	}
}

type failingMemory struct{}

func (failingMemory) AddMessage(context.Context, interfaces.Message) error { return nil }
func (failingMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return nil, errors.New("memory is down")
}
func (failingMemory) Clear(context.Context) error { return nil }

func TestCheckContentReportsMemoryError(t *testing.T) {
	err := checkContent(context.Background(), &interfaces.GenerateOptions{Memory: failingMemory{}})
	if err == nil || !strings.Contains(err.Error(), "memory is down") {
		t.Errorf("checkContent() = %v, want the memory error", err)
	}
}

func TestBuildMessagesWithFailingMemoryIsEmpty(t *testing.T) {
	messages := newMessageHistoryBuilder(logging.New()).buildMessages(context.Background(), "hi", failingMemory{}, nil)
	if len(messages) != 0 {
		t.Errorf("buildMessages() = %d messages, want none", len(messages))
	}
}

func TestConvertMemoryMessage(t *testing.T) {
	builder := newMessageHistoryBuilder(logging.New())
	tests := []struct {
		name string
		msg  interfaces.Message
		want string // substring of the JSON, or "" for a dropped message
	}{
		{"user with parts", interfaces.Message{Role: interfaces.MessageRoleUser, Parts: []*interfaces.Part{
			interfaces.NewPartFromURI("https://example.com/a.png", "image/png"),
		}}, `"url":"https://example.com/a.png"`},
		{"user text", interfaces.Message{Role: interfaces.MessageRoleUser, Content: "hi"}, `"content":"hi"`},
		{"assistant text", interfaces.Message{Role: interfaces.MessageRoleAssistant, Content: "hello"}, `"content":"hello"`},
		{"empty assistant", interfaces.Message{Role: interfaces.MessageRoleAssistant}, ""},
		{"assistant tool call", interfaces.Message{Role: interfaces.MessageRoleAssistant, ToolCalls: []interfaces.ToolCall{
			{ID: "c1", Name: "noop", Arguments: `{}`},
		}}, `"tool_calls":[{"id":"c1"`},
		{"tool result", interfaces.Message{Role: interfaces.MessageRoleTool, Content: "42", ToolCallID: "c1"}, `"tool_call_id":"c1"`},
		{"tool result without id", interfaces.Message{Role: interfaces.MessageRoleTool, Content: "42"}, ""},
		{"system", interfaces.Message{Role: interfaces.MessageRoleSystem, Content: "rules"}, `"role":"system"`},
		{"unknown role", interfaces.Message{Role: "other", Content: "x"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := builder.convertMemoryMessage(tt.msg)
			if tt.want == "" {
				if got != nil {
					t.Errorf("convertMemoryMessage() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("convertMemoryMessage() = nil")
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if !strings.Contains(string(raw), tt.want) {
				t.Errorf("message = %s, want it to contain %s", raw, tt.want)
			}
		})
	}
}
