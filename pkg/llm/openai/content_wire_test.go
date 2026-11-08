package openai

import (
	"context"
	"encoding/json"
	"errors"
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

func drain(events <-chan interfaces.StreamEvent) {
	if events == nil {
		return
	}
	for range events {
	}
}

// entryPoints calls every public generate method with the options and
// returns each error by method name. Streams are drained.
func entryPoints(client *OpenAIClient, options ...interfaces.GenerateOption) map[string]error {
	ctx := context.Background()
	tools := []interfaces.Tool{usageProbeTool{}}
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
	client := NewClient("test-key", WithModel("gpt-4o"), WithBaseURL(unreachable))
	cases := map[string]interfaces.GenerateOption{
		"audio":      interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "audio/wav")),
		"PDF by URL": interfaces.WithParts(interfaces.NewPartFromURI("https://example.com/a.pdf", "application/pdf")),
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

// recorder is a fake Chat Completions API. It records every request body
// and answers with a text completion, as JSON or as an SSE stream when the
// request asks to stream.
type recorder struct {
	mu     sync.Mutex
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
		rec.bodies = append(rec.bodies, body)
		rec.mu.Unlock()

		if stream, _ := body["stream"].(bool); stream {
			sse(w, `{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`)
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

// lastUserParts returns the content parts of the last message in a request
// body, failing when content is not a part list.
func lastUserParts(t *testing.T, body map[string]interface{}) []map[string]interface{} {
	t.Helper()
	messages, _ := body["messages"].([]interface{})
	if len(messages) == 0 {
		t.Fatalf("request has no messages: %v", body)
	}
	msg, _ := messages[len(messages)-1].(map[string]interface{})
	if msg["role"] != "user" {
		t.Fatalf("last message = %v, want the user turn", msg)
	}
	list, ok := msg["content"].([]interface{})
	if !ok {
		t.Fatalf("message content = %v, want a part list", msg["content"])
	}
	parts := make([]map[string]interface{}, 0, len(list))
	for _, p := range list {
		part, _ := p.(map[string]interface{})
		parts = append(parts, part)
	}
	return parts
}

func TestEntryPointsSendPartsOnTheWire(t *testing.T) {
	ctx := context.Background()
	tools := []interfaces.Tool{usageProbeTool{}}
	pdf := interfaces.NewPartFromBytes([]byte{1, 2}, "application/pdf")
	pdf.InlineData.DisplayName = "report.pdf"
	parts := interfaces.WithParts(
		interfaces.NewPartFromURI("https://example.com/a.png", "image/png"),
		pdf,
		interfaces.NewPartFromText("describe"),
	)
	calls := map[string]func(*OpenAIClient) error{
		"Generate": func(c *OpenAIClient) error {
			_, err := c.Generate(ctx, "describe", parts)
			return err
		},
		"GenerateWithTools": func(c *OpenAIClient) error {
			_, err := c.GenerateWithTools(ctx, "describe", tools, parts)
			return err
		},
		"GenerateStream": func(c *OpenAIClient) error {
			events, err := c.GenerateStream(ctx, "describe", parts)
			drain(events)
			return err
		},
		"GenerateWithToolsStream": func(c *OpenAIClient) error {
			events, err := c.GenerateWithToolsStream(ctx, "describe", tools, parts)
			drain(events)
			return err
		},
	}

	for method, call := range calls {
		t.Run(method, func(t *testing.T) {
			rec := &recorder{}
			server := rec.serve(t)
			client := NewClient("test-key", WithModel("gpt-4o"), WithBaseURL(server.URL))
			if err := call(client); err != nil {
				t.Fatalf("%s() error = %v", method, err)
			}
			got := lastUserParts(t, rec.first(t))
			if len(got) != 3 {
				t.Fatalf("got %d parts, want 3: %v", len(got), got)
			}
			image, _ := got[0]["image_url"].(map[string]interface{})
			if got[0]["type"] != "image_url" || image["url"] != "https://example.com/a.png" {
				t.Errorf("part 0 = %v, want the URL image", got[0])
			}
			file, _ := got[1]["file"].(map[string]interface{})
			if got[1]["type"] != "file" || file["file_data"] != "data:application/pdf;base64,AQI=" || file["filename"] != "report.pdf" {
				t.Errorf("part 1 = %v, want the PDF file", got[1])
			}
			if got[2]["type"] != "text" || got[2]["text"] != "describe" {
				t.Errorf("part 2 = %v, want the text", got[2])
			}
		})
	}
}

func TestSystemMessageComesBeforeTheUserTurn(t *testing.T) {
	ctx := context.Background()
	system := WithSystemMessage("be brief")
	calls := map[string]func(*OpenAIClient) error{
		"Generate": func(c *OpenAIClient) error {
			_, err := c.Generate(ctx, "hi", system)
			return err
		},
		"GenerateStream": func(c *OpenAIClient) error {
			events, err := c.GenerateStream(ctx, "hi", system)
			drain(events)
			return err
		},
		"GenerateWithToolsStream": func(c *OpenAIClient) error {
			events, err := c.GenerateWithToolsStream(ctx, "hi", []interfaces.Tool{usageProbeTool{}}, system)
			drain(events)
			return err
		},
	}

	for method, call := range calls {
		t.Run(method, func(t *testing.T) {
			rec := &recorder{}
			server := rec.serve(t)
			client := NewClient("test-key", WithModel("gpt-4o"), WithBaseURL(server.URL))
			if err := call(client); err != nil {
				t.Fatalf("%s() error = %v", method, err)
			}
			messages, _ := rec.first(t)["messages"].([]interface{})
			if len(messages) != 2 {
				t.Fatalf("messages = %v, want the system message and the user turn", messages)
			}
			first, _ := messages[0].(map[string]interface{})
			second, _ := messages[1].(map[string]interface{})
			if first["role"] != "system" || first["content"] != "be brief" || second["role"] != "user" {
				t.Errorf("messages = %v, want system then user", messages)
			}
		})
	}
}

func TestGenerateWithoutPartsSendsStringContent(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", WithModel("gpt-4o"), WithBaseURL(server.URL))

	got, err := client.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got != "done" {
		t.Errorf("Generate() = %q, want %q", got, "done")
	}
	messages, _ := rec.first(t)["messages"].([]interface{})
	if len(messages) != 1 {
		t.Fatalf("messages = %v, want one", messages)
	}
	if msg, _ := messages[0].(map[string]interface{}); msg["content"] != "hello" {
		t.Errorf("content = %v, want the plain string", msg["content"])
	}
}

func TestGenerateIgnoresPartsWhenMemoryIsSet(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", WithModel("gpt-4o"), WithBaseURL(server.URL))
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

func TestGenerateSendsStopAndPenaltiesWhenSet(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", WithModel("gpt-4o"), WithBaseURL(server.URL))

	_, err := client.Generate(context.Background(), "hi",
		WithStopSequences([]string{"END"}),
		WithFrequencyPenalty(0.5),
		WithPresencePenalty(0.25),
	)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	body := rec.first(t)
	if stop, _ := body["stop"].([]interface{}); len(stop) != 1 || stop[0] != "END" {
		t.Errorf("stop = %v, want [END]", body["stop"])
	}
	if body["frequency_penalty"] != 0.5 || body["presence_penalty"] != 0.25 {
		t.Errorf("penalties = %v, %v, want 0.5, 0.25", body["frequency_penalty"], body["presence_penalty"])
	}
}

func TestGenerateOmitsPenaltiesWhenUnset(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", WithModel("gpt-4o"), WithBaseURL(server.URL))

	if _, err := client.Generate(context.Background(), "hi"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	body := rec.first(t)
	for _, key := range []string{"frequency_penalty", "presence_penalty", "stop"} {
		if _, ok := body[key]; ok {
			t.Errorf("request sent %s = %v without the caller asking", key, body[key])
		}
	}
}

func TestGenerateForcesTemperatureOneForReasoningModels(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", WithModel("o3-mini"), WithBaseURL(server.URL))

	if _, err := client.Generate(context.Background(), "hi", WithTemperature(0.2), WithTopP(0.5)); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	body := rec.first(t)
	if body["temperature"] != 1.0 {
		t.Errorf("temperature = %v, want 1", body["temperature"])
	}
	if _, ok := body["top_p"]; ok {
		t.Errorf("top_p = %v, want it omitted for a reasoning model", body["top_p"])
	}
}

func TestGenerateStreamOmitsTemperatureForReasoningModels(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", WithModel("o3-mini"), WithBaseURL(server.URL))

	events, err := client.GenerateStream(context.Background(), "hi")
	if err != nil {
		t.Fatalf("GenerateStream() error = %v", err)
	}
	drain(events)
	if v, ok := rec.first(t)["temperature"]; ok {
		t.Errorf("temperature = %v, want it omitted for a reasoning model", v)
	}
}

func TestGenerateStreamSendsTemperatureAndStreams(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", WithModel("gpt-4o"), WithBaseURL(server.URL))

	events, err := client.GenerateStream(context.Background(), "hi", WithTemperature(0.3))
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
	body := rec.first(t)
	if body["temperature"] != 0.3 {
		t.Errorf("temperature = %v, want 0.3", body["temperature"])
	}
	if body["stream"] != true {
		t.Errorf("stream = %v, want true", body["stream"])
	}
}

func TestUserMessageSendsInlinePDFWithoutName(t *testing.T) {
	raw, err := json.Marshal(userMessage("", []*interfaces.Part{interfaces.NewPartFromBytes([]byte{1}, "application/pdf")}))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(raw), "filename") {
		t.Errorf("message = %s, want no filename when DisplayName is empty", raw)
	}
	if !strings.Contains(string(raw), `"file_data":"data:application/pdf;base64,AQ=="`) {
		t.Errorf("message = %s, want the PDF as a data URL", raw)
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
