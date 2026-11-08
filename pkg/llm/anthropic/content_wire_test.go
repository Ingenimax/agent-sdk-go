package anthropic

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

// entryPoints calls every public generate method with the options and
// returns each error by method name. Streams are drained.
func entryPoints(client *AnthropicClient, options ...interfaces.GenerateOption) map[string]error {
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

func drain(events <-chan interfaces.StreamEvent) {
	if events == nil {
		return
	}
	for range events {
	}
}

func TestEntryPointsRefuseUnsupportedContent(t *testing.T) {
	client := NewClient("test-key", WithModel(ClaudeSonnet4), WithBaseURL(unreachable))
	audio := interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "audio/wav"))

	for method, err := range entryPoints(client, audio) {
		if !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Errorf("%s() error = %v, want ErrContentUnsupported", method, err)
		}
	}
}

func TestEntryPointsRefuseURLsOnVertex(t *testing.T) {
	client := &AnthropicClient{
		Model:        ClaudeSonnet4,
		VertexConfig: &VertexConfig{Enabled: true},
		HTTPClient:   http.DefaultClient,
		logger:       logging.New(),
	}
	byURL := interfaces.WithParts(interfaces.NewPartFromURI("https://example.com/a.png", "image/png"))

	for method, err := range entryPoints(client, byURL) {
		if !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Errorf("%s() error = %v, want ErrContentUnsupported", method, err)
		}
	}
}

func TestEntryPointsRefuseMediaFromMemory(t *testing.T) {
	client := NewClient("test-key", WithModel(ClaudeSonnet4), WithBaseURL(unreachable))
	mem := &mockMemory{messages: []interfaces.Message{{
		Role:    interfaces.MessageRoleUser,
		Content: "listen",
		Parts:   []*interfaces.Part{interfaces.NewPartFromBytes([]byte{1}, "audio/wav")},
	}}}

	for method, err := range entryPoints(client, interfaces.WithMemory(mem)) {
		if !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Errorf("%s() error = %v, want ErrContentUnsupported", method, err)
		}
	}
}

// recorder is a fake Messages API. It records every request body and
// answers with JSON text, or with an SSE stream when the request asks to
// stream.
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
			writeSSE(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-20250514",
			"content":[{"type":"text","text":"done"}],"stop_reason":"end_turn",
			"usage":{"input_tokens":3,"output_tokens":1}}`))
	}))
	t.Cleanup(server.Close)
	return server
}

// first returns the first recorded request body.
func (rec *recorder) first(t *testing.T) map[string]interface{} {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.bodies) == 0 {
		t.Fatal("server received no request")
	}
	return rec.bodies[0]
}

func writeSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	events := []struct{ name, data string }{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[],"usage":{"input_tokens":3,"output_tokens":0}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"done"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}
	for _, e := range events {
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.name, e.data)
	}
}

// userBlocks returns the content blocks of the first message in a request
// body, failing when content is not a block list.
func userBlocks(t *testing.T, body map[string]interface{}) []map[string]interface{} {
	t.Helper()
	messages, _ := body["messages"].([]interface{})
	if len(messages) == 0 {
		t.Fatalf("request has no messages: %v", body)
	}
	msg, _ := messages[0].(map[string]interface{})
	list, ok := msg["content"].([]interface{})
	if !ok {
		t.Fatalf("message content = %v, want a block list", msg["content"])
	}
	blocks := make([]map[string]interface{}, 0, len(list))
	for _, b := range list {
		block, _ := b.(map[string]interface{})
		blocks = append(blocks, block)
	}
	return blocks
}

func imageAndText() interfaces.GenerateOption {
	return interfaces.WithParts(
		interfaces.NewPartFromBytes([]byte{1, 2}, "image/png"),
		interfaces.NewPartFromText("describe"),
	)
}

func assertImageThenText(t *testing.T, method string, blocks []map[string]interface{}) {
	t.Helper()
	if len(blocks) != 2 {
		t.Fatalf("%s: got %d blocks, want 2: %v", method, len(blocks), blocks)
	}
	source, _ := blocks[0]["source"].(map[string]interface{})
	if blocks[0]["type"] != "image" || source["type"] != "base64" || source["data"] != "AQI=" || source["media_type"] != "image/png" {
		t.Errorf("%s: block 0 = %v, want the inline image", method, blocks[0])
	}
	if blocks[1]["type"] != "text" || blocks[1]["text"] != "describe" {
		t.Errorf("%s: block 1 = %v, want the text", method, blocks[1])
	}
}

func TestEntryPointsSendPartsOnTheWire(t *testing.T) {
	ctx := context.Background()
	tools := []interfaces.Tool{wireTool{}}
	calls := map[string]func(*AnthropicClient) error{
		"Generate": func(c *AnthropicClient) error {
			_, err := c.Generate(ctx, "describe", imageAndText())
			return err
		},
		"GenerateWithTools": func(c *AnthropicClient) error {
			_, err := c.GenerateWithTools(ctx, "describe", tools, imageAndText())
			return err
		},
		"GenerateStream": func(c *AnthropicClient) error {
			events, err := c.GenerateStream(ctx, "describe", imageAndText())
			drain(events)
			return err
		},
		"GenerateWithToolsStream": func(c *AnthropicClient) error {
			events, err := c.GenerateWithToolsStream(ctx, "describe", tools, imageAndText())
			drain(events)
			return err
		},
	}

	for method, call := range calls {
		t.Run(method, func(t *testing.T) {
			rec := &recorder{}
			server := rec.serve(t)
			client := NewClient("test-key", WithModel(ClaudeSonnet4), WithBaseURL(server.URL))
			if err := call(client); err != nil {
				t.Fatalf("%s() error = %v", method, err)
			}
			assertImageThenText(t, method, userBlocks(t, rec.first(t)))
		})
	}
}

func TestGenerateWithStringContentWithoutParts(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", WithModel(ClaudeSonnet4), WithBaseURL(server.URL))

	got, err := client.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got != "done" {
		t.Errorf("Generate() = %q, want %q", got, "done")
	}
	messages, _ := rec.first(t)["messages"].([]interface{})
	msg, _ := messages[0].(map[string]interface{})
	if msg["content"] != "hello" {
		t.Errorf("content = %v, want the plain string", msg["content"])
	}
}

func TestGenerateCachesConversationWithParts(t *testing.T) {
	ctx := context.Background()
	calls := map[string]func(*AnthropicClient) error{
		"Generate": func(c *AnthropicClient) error {
			_, err := c.Generate(ctx, "describe", imageAndText(), WithCacheConversation())
			return err
		},
		"GenerateWithTools": func(c *AnthropicClient) error {
			_, err := c.GenerateWithTools(ctx, "describe", []interfaces.Tool{wireTool{}}, imageAndText(), WithCacheConversation(), WithCacheTools())
			return err
		},
	}

	for method, call := range calls {
		t.Run(method, func(t *testing.T) {
			rec := &recorder{}
			server := rec.serve(t)
			client := NewClient("test-key", WithModel(ClaudeSonnet4), WithBaseURL(server.URL))
			if err := call(client); err != nil {
				t.Fatalf("%s() error = %v", method, err)
			}
			blocks := userBlocks(t, rec.first(t))
			assertImageThenText(t, method, blocks)
			if _, ok := blocks[0]["cache_control"]; ok {
				t.Errorf("block 0 = %v, want no cache_control", blocks[0])
			}
			if _, ok := blocks[1]["cache_control"]; !ok {
				t.Errorf("block 1 = %v, want cache_control on the last block", blocks[1])
			}
		})
	}
}

func TestGenerateReadsPartsFromMemoryOnTheWire(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", WithModel(ClaudeSonnet4), WithBaseURL(server.URL))
	mem := &mockMemory{messages: []interfaces.Message{{
		Role:    interfaces.MessageRoleUser,
		Content: "describe",
		Parts: []*interfaces.Part{
			interfaces.NewPartFromURI("https://example.com/a.pdf", "application/pdf"),
			interfaces.NewPartFromText("describe"),
		},
	}}}

	if _, err := client.Generate(context.Background(), "describe", interfaces.WithMemory(mem)); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	blocks := userBlocks(t, rec.first(t))
	source, _ := blocks[0]["source"].(map[string]interface{})
	if blocks[0]["type"] != "document" || source["type"] != "url" || source["url"] != "https://example.com/a.pdf" {
		t.Errorf("block 0 = %v, want the PDF by URL", blocks[0])
	}
}

func TestGenerateSendsSystemAndStopSequences(t *testing.T) {
	rec := &recorder{}
	server := rec.serve(t)
	client := NewClient("test-key", WithModel(ClaudeSonnet4), WithBaseURL(server.URL))

	_, err := client.Generate(context.Background(), "hi",
		WithSystemMessage("be brief"),
		WithStopSequences([]string{"END"}),
	)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	body := rec.first(t)
	if body["system"] != "be brief" {
		t.Errorf("system = %v, want %q", body["system"], "be brief")
	}
	stops, _ := body["stop_sequences"].([]interface{})
	if len(stops) != 1 || stops[0] != "END" {
		t.Errorf("stop_sequences = %v, want [END]", body["stop_sequences"])
	}
	if body["model"] != ClaudeSonnet4 {
		t.Errorf("model = %v, want %q", body["model"], ClaudeSonnet4)
	}
}

func TestGenerateSendsAPIKeyAndVersionHeaders(t *testing.T) {
	var header http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"done"}]}`))
	}))
	defer server.Close()
	client := NewClient("secret", WithModel(ClaudeSonnet4), WithBaseURL(server.URL))

	if _, err := client.Generate(context.Background(), "hi"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got := header.Get("X-API-Key"); got != "secret" {
		t.Errorf("X-API-Key = %q, want %q", got, "secret")
	}
	if got := header.Get("Anthropic-Version"); got != "2023-06-01" {
		t.Errorf("Anthropic-Version = %q, want %q", got, "2023-06-01")
	}
}

func TestGenerateReportsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"bad image"}`, http.StatusBadRequest)
	}))
	defer server.Close()
	client := NewClient("test-key", WithModel(ClaudeSonnet4), WithBaseURL(server.URL))

	_, err := client.Generate(context.Background(), "hi", imageAndText())
	if err == nil || !strings.Contains(err.Error(), "status 400") || !strings.Contains(err.Error(), "bad image") {
		t.Errorf("Generate() error = %v, want the status and the body", err)
	}
}

func TestGenerateRefusesEmptyModel(t *testing.T) {
	client := NewClient("test-key", WithModel(""), WithBaseURL(unreachable))
	if _, err := client.Generate(context.Background(), "hi"); err == nil || !strings.Contains(err.Error(), "model not specified") {
		t.Errorf("Generate() error = %v, want model not specified", err)
	}
}

func TestCreateStreamingHTTPRequestSetsStreamAndHeaders(t *testing.T) {
	client := NewClient("secret", WithModel(ClaudeSonnet4), WithBaseURL("https://api.example.com"))
	req := &CompletionRequest{Model: ClaudeSonnet4, Messages: []Message{{
		Role:    "user",
		Content: "describe",
		Parts:   []*interfaces.Part{interfaces.NewPartFromBytes([]byte{1, 2}, "image/png")},
	}}}

	httpReq, err := client.createStreamingHTTPRequestWithCache(context.Background(), req, "/v1/messages",
		&interfaces.CacheConfig{CacheConversation: true})
	if err != nil {
		t.Fatalf("createStreamingHTTPRequestWithCache() error = %v", err)
	}
	if httpReq.URL.String() != "https://api.example.com/v1/messages" {
		t.Errorf("URL = %s", httpReq.URL)
	}
	if got := httpReq.Header.Get("Accept"); got != "text/event-stream" {
		t.Errorf("Accept = %q, want text/event-stream", got)
	}
	raw, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("body is not JSON: %s", raw)
	}
	if body["stream"] != true {
		t.Errorf("stream = %v, want true", body["stream"])
	}
	blocks := userBlocks(t, body)
	if len(blocks) != 1 || blocks[0]["type"] != "image" {
		t.Fatalf("blocks = %v, want one image", blocks)
	}
	if _, ok := blocks[0]["cache_control"]; !ok {
		t.Errorf("block = %v, want cache_control", blocks[0])
	}
}

func TestContentBlocksWithoutPartsIsOneTextBlock(t *testing.T) {
	blocks := Message{Role: "user", Content: "hi"}.contentBlocks()
	if len(blocks) != 1 || blocks[0].Type != "text" || blocks[0].Text != "hi" || blocks[0].Source != nil {
		t.Errorf("contentBlocks() = %+v, want one text block", blocks)
	}
}

func TestMessageWithEmptyTextPartKeepsTextBlock(t *testing.T) {
	raw, err := json.Marshal(Message{Role: "user", Parts: []*interfaces.Part{{}}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got wireMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v: %s", err, raw)
	}
	if len(got.Content) != 1 || got.Content[0].Type != "text" {
		t.Errorf("content = %s, want one text block", raw)
	}
}

func TestCheckContentAcceptsEveryImageType(t *testing.T) {
	for _, mime := range imageTypes {
		params := &interfaces.GenerateOptions{}
		interfaces.WithParts(
			interfaces.NewPartFromBytes([]byte{1}, mime),
			interfaces.NewPartFromURI("https://example.com/a", mime),
		)(params)
		if err := (&AnthropicClient{}).checkContent(context.Background(), params); err != nil {
			t.Errorf("checkContent(%s) = %v", mime, err)
		}
	}
}

func TestCheckContentRefusesPDFURLsOnBedrock(t *testing.T) {
	client := &AnthropicClient{BedrockConfig: &BedrockConfig{Enabled: true}}
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(interfaces.NewPartFromURI("https://example.com/a.pdf", "application/pdf"))(params)
	if err := client.checkContent(context.Background(), params); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("checkContent() = %v, want ErrContentUnsupported", err)
	}
}

func TestCheckContentAcceptsURLsWhenBackendsDisabled(t *testing.T) {
	client := &AnthropicClient{
		BedrockConfig: &BedrockConfig{Enabled: false},
		VertexConfig:  &VertexConfig{Enabled: false},
	}
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(interfaces.NewPartFromURI("https://example.com/a.png", "image/png"))(params)
	if err := client.checkContent(context.Background(), params); err != nil {
		t.Errorf("checkContent() = %v, want nil", err)
	}
}

type failingMemory struct{}

func (failingMemory) AddMessage(context.Context, interfaces.Message) error { return nil }
func (failingMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return nil, errors.New("memory is down")
}
func (failingMemory) Clear(context.Context) error { return nil }

func TestCheckContentReportsMemoryError(t *testing.T) {
	params := &interfaces.GenerateOptions{Memory: failingMemory{}}
	err := (&AnthropicClient{}).checkContent(context.Background(), params)
	if err == nil || !strings.Contains(err.Error(), "memory is down") {
		t.Errorf("checkContent() = %v, want the memory error", err)
	}
}

func TestBuildMessagesWithoutMemoryCarriesParts(t *testing.T) {
	parts := []*interfaces.Part{interfaces.NewPartFromBytes([]byte{1}, "image/png")}
	messages := newMessageHistoryBuilder(logging.New()).buildMessages(context.Background(), "describe",
		&interfaces.GenerateOptions{Parts: parts})
	if len(messages) != 1 || messages[0].Role != "user" || messages[0].Content != "describe" || len(messages[0].Parts) != 1 {
		t.Errorf("buildMessages() = %+v, want one user message with the part", messages)
	}
}

func TestBuildMessagesWithFailingMemoryIsEmpty(t *testing.T) {
	messages := newMessageHistoryBuilder(logging.New()).buildMessages(context.Background(), "hi",
		&interfaces.GenerateOptions{Memory: failingMemory{}})
	if len(messages) != 0 {
		t.Errorf("buildMessages() = %+v, want no messages", messages)
	}
}

func TestConvertMemoryMessage(t *testing.T) {
	builder := newMessageHistoryBuilder(logging.New())
	parts := []*interfaces.Part{interfaces.NewPartFromBytes([]byte{1}, "image/png")}

	tests := []struct {
		name    string
		msg     interfaces.Message
		want    *Message
		wantNil bool
	}{
		{
			name: "user keeps parts",
			msg:  interfaces.Message{Role: interfaces.MessageRoleUser, Content: "hi", Parts: parts},
			want: &Message{Role: "user", Content: "hi", Parts: parts},
		},
		{
			name: "assistant text",
			msg:  interfaces.Message{Role: interfaces.MessageRoleAssistant, Content: "hello"},
			want: &Message{Role: "assistant", Content: "hello"},
		},
		{
			name:    "empty assistant is dropped",
			msg:     interfaces.Message{Role: interfaces.MessageRoleAssistant},
			wantNil: true,
		},
		{
			name: "assistant tool call without text",
			msg: interfaces.Message{Role: interfaces.MessageRoleAssistant, ToolCalls: []interfaces.ToolCall{
				{ID: "c1", Name: "noop", Arguments: `{"a":1}`},
			}},
			want: &Message{Role: "assistant", Content: `{"id":"c1","input":{"a":1},"name":"noop","type":"tool_use"}`},
		},
		{
			name: "assistant tool call with bad arguments keeps the text",
			msg: interfaces.Message{Role: interfaces.MessageRoleAssistant, Content: "thinking", ToolCalls: []interfaces.ToolCall{
				{ID: "c1", Name: "noop", Arguments: `not json`},
			}},
			want: &Message{Role: "assistant", Content: "thinking"},
		},
		{
			name: "tool result becomes a user message",
			msg:  interfaces.Message{Role: interfaces.MessageRoleTool, Content: "42", ToolCallID: "c1"},
			want: &Message{Role: "user", Content: "Tool result for c1: 42"},
		},
		{
			name:    "tool result without an id is dropped",
			msg:     interfaces.Message{Role: interfaces.MessageRoleTool, Content: "42"},
			wantNil: true,
		},
		{
			name: "system becomes a user message",
			msg:  interfaces.Message{Role: interfaces.MessageRoleSystem, Content: "summary"},
			want: &Message{Role: "user", Content: "System: summary"},
		},
		{
			name:    "unknown role is dropped",
			msg:     interfaces.Message{Role: "other", Content: "x"},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := builder.convertMemoryMessage(tt.msg)
			if tt.wantNil {
				if got != nil {
					t.Errorf("convertMemoryMessage() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("convertMemoryMessage() = nil")
			}
			if got.Role != tt.want.Role || got.Content != tt.want.Content || len(got.Parts) != len(tt.want.Parts) {
				t.Errorf("convertMemoryMessage() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// With parts, the message sends its parts instead of Content, so the JSON
// instructions must arrive as a text part after the caller's parts.
func TestGenerateSendsResponseFormatWithParts(t *testing.T) {
	rec := &recorder{}
	client := NewClient("key", WithModel("claude-sonnet-4-20250514"), WithBaseURL(rec.serve(t).URL))

	parts := []*interfaces.Part{
		interfaces.NewPartFromBytes([]byte{1, 2}, "image/png"),
		interfaces.NewPartFromText("describe"),
	}
	format := interfaces.WithResponseFormat(interfaces.ResponseFormat{
		Type:   interfaces.ResponseFormatJSON,
		Name:   "answer",
		Schema: interfaces.JSONSchema{"type": "object"},
	})
	if _, err := client.Generate(context.Background(), "", interfaces.WithParts(parts...), format); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	blocks := userBlocks(t, rec.first(t))
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want the image, the text, and the instructions: %v", len(blocks), blocks)
	}
	text, _ := blocks[2]["text"].(string)
	if blocks[2]["type"] != "text" || !strings.Contains(text, "valid JSON object") {
		t.Errorf("block 2 = %v, want the JSON instructions", blocks[2])
	}
	if len(parts) != 2 {
		t.Errorf("caller's parts changed to %d entries, want 2", len(parts))
	}
}
