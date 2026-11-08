package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

type contentMemory struct{ messages []interfaces.Message }

func (m contentMemory) AddMessage(context.Context, interfaces.Message) error { return nil }
func (m contentMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return m.messages, nil
}
func (m contentMemory) Clear(context.Context) error { return nil }

type wireMessage struct {
	Role    string `json:"role"`
	Content []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	} `json:"content"`
}

func TestMessageWithoutPartsKeepsStringContent(t *testing.T) {
	raw, err := json.Marshal(Message{Role: "user", Content: "hi"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(raw) != `{"role":"user","content":"hi"}` {
		t.Errorf("Marshal() = %s", raw)
	}
}

func TestMessageWithPartsSendsContentList(t *testing.T) {
	raw, err := json.Marshal(Message{Role: "user", Content: "describe", Parts: []*interfaces.Part{
		interfaces.NewPartFromText("describe"),
		interfaces.NewPartFromBytes([]byte{1, 2}, "image/png"),
		interfaces.NewPartFromURI("https://example.com/a.jpg", "image/jpeg"),
	}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got wireMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("content is not a list: %s", raw)
	}
	if got.Role != "user" || len(got.Content) != 3 {
		t.Fatalf("message = %s, want a user message with 3 parts", raw)
	}
	if c := got.Content[0]; c.Type != "text" || c.Text != "describe" {
		t.Errorf("part 0 = %s, want the text", raw)
	}
	if c := got.Content[1]; c.Type != "image_url" || c.ImageURL.URL != "data:image/png;base64,AQI=" {
		t.Errorf("part 1 = %s, want a data URL image", raw)
	}
	if c := got.Content[2]; c.Type != "image_url" || c.ImageURL.URL != "https://example.com/a.jpg" {
		t.Errorf("part 2 = %s, want a URL image", raw)
	}
}

func TestBuildMessagesSendsParts(t *testing.T) {
	builder := newMessageHistoryBuilder(logging.New())
	parts := []*interfaces.Part{interfaces.NewPartFromURI("https://example.com/a.png", "image/png")}

	withoutMemory := builder.buildMessages(context.Background(), "describe", nil, parts)
	if last := withoutMemory[len(withoutMemory)-1]; len(last.Parts) != 1 {
		t.Errorf("current message parts = %v, want the image", last.Parts)
	}

	mem := contentMemory{messages: []interfaces.Message{{Role: interfaces.MessageRoleUser, Content: "describe", Parts: parts}}}
	withMemory := builder.buildMessages(context.Background(), "describe", mem, parts)
	if len(withMemory[0].Parts) != 1 {
		t.Errorf("memory message parts = %v, want the image", withMemory[0].Parts)
	}
	if last := withMemory[len(withMemory)-1]; len(last.Parts) != 0 {
		t.Errorf("current message parts = %v, want none with memory", last.Parts)
	}
}

// Media DeepSeek cannot take must fail the call rather than be dropped from
// it. The base URL is unreachable: the check has to happen before any request
// is made.
func TestDeepSeekRefusesUnsupportedMedia(t *testing.T) {
	client := NewClient("key", WithBaseURL("http://127.0.0.1:0"))
	for _, part := range []*interfaces.Part{
		interfaces.NewPartFromBytes([]byte{1}, "application/pdf"),
		interfaces.NewPartFromBytes([]byte{1}, "image/bmp"),
	} {
		withPart := interfaces.WithParts(part)
		if _, err := client.Generate(context.Background(), "x", withPart); !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Errorf("Generate(%s) error = %v, want ErrContentUnsupported", part.MIMEType(), err)
		}
		if _, err := client.GenerateWithTools(context.Background(), "x", nil, withPart); !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Errorf("GenerateWithTools(%s) error = %v, want ErrContentUnsupported", part.MIMEType(), err)
		}
		if _, err := client.GenerateStream(context.Background(), "x", withPart); !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Errorf("GenerateStream(%s) error = %v, want ErrContentUnsupported", part.MIMEType(), err)
		}
		if _, err := client.GenerateWithToolsStream(context.Background(), "x", nil, withPart); !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Errorf("GenerateWithToolsStream(%s) error = %v, want ErrContentUnsupported", part.MIMEType(), err)
		}
	}
}
