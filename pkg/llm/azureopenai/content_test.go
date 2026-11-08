package azureopenai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

func TestUserMessageSendsPartsInOrder(t *testing.T) {
	msg := userMessage("ignored", []*interfaces.Part{
		interfaces.NewPartFromText("first"),
		interfaces.NewPartFromURI("https://example.com/a.png", "image/png"),
		interfaces.NewPartFromBytes([]byte{1, 2}, "image/png"),
	})
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got struct {
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL struct {
				URL string `json:"url"`
			} `json:"image_url"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("content is not a list of parts: %s", raw)
	}
	if len(got.Content) != 3 {
		t.Fatalf("got %d parts, want 3: %s", len(got.Content), raw)
	}
	if got.Content[0].Type != "text" || got.Content[0].Text != "first" {
		t.Errorf("part 0 = %+v, want the text", got.Content[0])
	}
	if got.Content[1].Type != "image_url" || got.Content[1].ImageURL.URL != "https://example.com/a.png" {
		t.Errorf("part 1 = %+v, want the URL image", got.Content[1])
	}
	if got.Content[2].ImageURL.URL != "data:image/png;base64,AQI=" {
		t.Errorf("part 2 URL = %q, want a data URL", got.Content[2].ImageURL.URL)
	}
}

func TestUserMessageWithoutPartsStaysAString(t *testing.T) {
	raw, err := json.Marshal(userMessage("hi", nil))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(raw), `"content":"hi"`) {
		t.Errorf("message = %s, want string content", raw)
	}
}

func TestBuildMessagesReadsPartsFromMemory(t *testing.T) {
	mem := contentMemory{messages: []interfaces.Message{{
		Role:    interfaces.MessageRoleUser,
		Content: "what is this?",
		Parts: []*interfaces.Part{
			interfaces.NewPartFromURI("https://example.com/a.png", "image/png"),
			interfaces.NewPartFromText("what is this?"),
		},
	}}}
	messages := newMessageHistoryBuilder(logging.New()).buildMessages(context.Background(), "what is this?", mem, nil)
	raw, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(raw), "https://example.com/a.png") {
		t.Errorf("request = %s, want the image from memory", raw)
	}
}

func TestCheckContentRefusesUnsupportedMedia(t *testing.T) {
	image := &interfaces.GenerateOptions{}
	interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "image/png"))(image)
	if err := checkContent(context.Background(), image); err != nil {
		t.Errorf("checkContent() with an image = %v", err)
	}

	for _, mime := range []string{"audio/wav", "image/bmp", "image/svg+xml"} {
		params := &interfaces.GenerateOptions{}
		interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, mime))(params)
		if err := checkContent(context.Background(), params); !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Errorf("checkContent(%s) = %v, want ErrContentUnsupported", mime, err)
		}
	}
}
