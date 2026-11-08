package vllm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

type contentMemory struct{ messages []interfaces.Message }

func (m contentMemory) AddMessage(context.Context, interfaces.Message) error { return nil }
func (m contentMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return m.messages, nil
}
func (m contentMemory) Clear(context.Context) error { return nil }

type wireRequest struct {
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

type wirePart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

// chatServer answers every chat request with "ok" and records the path and
// the last request body.
func chatServer(t *testing.T, path *string, got *wireRequest) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(ChatResponse{Choices: []struct {
			Index        int         `json:"index"`
			Message      ChatMessage `json:"message"`
			FinishReason string      `json:"finish_reason"`
		}{{Message: ChatMessage{Role: "assistant", Content: "ok"}}}})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestChatMessageWithoutPartsKeepsStringContent(t *testing.T) {
	raw, err := json.Marshal(ChatMessage{Role: "user", Content: "hi"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(raw) != `{"role":"user","content":"hi"}` {
		t.Errorf("Marshal() = %s", raw)
	}
}

func TestGenerateSendsImagesToChatEndpoint(t *testing.T) {
	var path string
	var got wireRequest
	client := NewClient(WithBaseURL(chatServer(t, &path, &got).URL))

	parts := interfaces.WithParts(
		interfaces.NewPartFromBytes([]byte{1, 2}, "image/png"),
		interfaces.NewPartFromText("describe"),
	)
	answer, err := client.Generate(context.Background(), "ignored", parts)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if answer != "ok" || path != "/v1/chat/completions" {
		t.Fatalf("answer %q from %s, want ok from /v1/chat/completions", answer, path)
	}

	var content []wirePart
	if err := json.Unmarshal(got.Messages[len(got.Messages)-1].Content, &content); err != nil {
		t.Fatalf("content is not a list: %s", got.Messages[len(got.Messages)-1].Content)
	}
	if len(content) != 2 || content[0].ImageURL.URL != "data:image/png;base64,AQI=" || content[1].Text != "describe" {
		t.Errorf("content = %+v, want the image then the text", content)
	}
}

func TestGenerateWithToolsReadsCurrentPartsFromMemory(t *testing.T) {
	var path string
	var got wireRequest
	client := NewClient(WithBaseURL(chatServer(t, &path, &got).URL))

	mem := contentMemory{messages: []interfaces.Message{{
		Role:    interfaces.MessageRoleUser,
		Content: "describe",
		Parts: []*interfaces.Part{
			interfaces.NewPartFromURI("https://example.com/a.png", "image/png"),
			interfaces.NewPartFromText("describe"),
		},
	}}}
	tools := []interfaces.Tool{&testutil.FakeTool{}}
	if _, err := client.GenerateWithTools(context.Background(), "describe", tools, interfaces.WithMemory(mem)); err != nil {
		t.Fatalf("GenerateWithTools() error = %v", err)
	}

	var content []wirePart
	if err := json.Unmarshal(got.Messages[len(got.Messages)-1].Content, &content); err != nil {
		t.Fatalf("content is not a list: %s", got.Messages[len(got.Messages)-1].Content)
	}
	if len(content) != 2 || content[0].ImageURL.URL != "https://example.com/a.png" {
		t.Errorf("content = %+v, want the image from memory", content)
	}
}

// Media that is not an image must fail the call rather than be dropped from
// it. The base URL is unreachable: the check has to happen before any request
// is made.
func TestVLLMRefusesNonImageMedia(t *testing.T) {
	client := NewClient(WithBaseURL("http://127.0.0.1:0"))
	withPDF := interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "application/pdf"))

	if _, err := client.Generate(context.Background(), "x", withPDF); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("Generate() error = %v, want ErrContentUnsupported", err)
	}
	tool := []interfaces.Tool{&testutil.FakeTool{}}
	if _, err := client.GenerateWithTools(context.Background(), "x", tool, withPDF); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("GenerateWithTools() error = %v, want ErrContentUnsupported", err)
	}
}

// An image from an earlier turn must reach the model too, so the request goes
// to the chat endpoint with the history, not to the text-only completions
// endpoint.
func TestGenerateSendsHistoryWithEarlierImages(t *testing.T) {
	var path string
	var got wireRequest
	client := NewClient(WithBaseURL(chatServer(t, &path, &got).URL))

	mem := contentMemory{messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: "describe", Parts: []*interfaces.Part{
			interfaces.NewPartFromURI("https://example.com/a.png", "image/png"),
			interfaces.NewPartFromText("describe"),
		}},
		{Role: interfaces.MessageRoleAssistant, Content: "a cat"},
		{Role: interfaces.MessageRoleUser, Content: "what color is it?"},
	}}
	if _, err := client.Generate(context.Background(), "what color is it?", interfaces.WithMemory(mem)); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if path != "/v1/chat/completions" || len(got.Messages) != 3 {
		t.Fatalf("sent %d messages to %s, want the 3 history messages to the chat endpoint", len(got.Messages), path)
	}
	var first []wirePart
	if err := json.Unmarshal(got.Messages[0].Content, &first); err != nil || first[0].ImageURL.URL != "https://example.com/a.png" {
		t.Errorf("first message = %s, want the earlier image", got.Messages[0].Content)
	}
	if string(got.Messages[2].Content) != `"what color is it?"` {
		t.Errorf("last message = %s, want the current text", got.Messages[2].Content)
	}
}

func TestGenerateRefusesResponseFormatWithImages(t *testing.T) {
	client := NewClient(WithBaseURL("http://127.0.0.1:0"))
	format := func(o *interfaces.GenerateOptions) {
		o.ResponseFormat = &interfaces.ResponseFormat{Type: interfaces.ResponseFormatJSON}
	}
	withImage := interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "image/png"))
	if _, err := client.Generate(context.Background(), "x", withImage, format); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("Generate() error = %v, want ErrContentUnsupported", err)
	}
}
