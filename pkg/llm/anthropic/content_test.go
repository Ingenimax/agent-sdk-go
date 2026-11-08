package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

type wireBlock struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	Source *struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
		URL       string `json:"url"`
	} `json:"source"`
	CacheControl *json.RawMessage `json:"cache_control"`
}

type wireMessage struct {
	Role    string      `json:"role"`
	Content []wireBlock `json:"content"`
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

func TestMessageWithPartsSendsBlocksInOrder(t *testing.T) {
	raw, err := json.Marshal(Message{Role: "user", Content: "describe", Parts: []*interfaces.Part{
		interfaces.NewPartFromText("describe"),
		interfaces.NewPartFromBytes([]byte{1, 2}, "image/png"),
		interfaces.NewPartFromURI("https://example.com/a.png", "image/png"),
	}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got wireMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("content is not a block list: %s", raw)
	}
	if len(got.Content) != 3 {
		t.Fatalf("got %d blocks, want 3: %s", len(got.Content), raw)
	}
	if b := got.Content[0]; b.Type != "text" || b.Text != "describe" {
		t.Errorf("block 0 = %s, want the text", raw)
	}
	if b := got.Content[1]; b.Type != "image" || b.Source.Type != "base64" || b.Source.MediaType != "image/png" || b.Source.Data != "AQI=" {
		t.Errorf("block 1 = %s, want an inline image", raw)
	}
	if b := got.Content[2]; b.Type != "image" || b.Source.Type != "url" || b.Source.URL != "https://example.com/a.png" {
		t.Errorf("block 2 = %s, want a URL image", raw)
	}
}

// The cache builder rewrites the last message itself, so it must keep the
// parts rather than rebuilding the message from its text alone.
func TestCachedLastMessageKeepsParts(t *testing.T) {
	builder := newCacheRequestBuilder(&interfaces.CacheConfig{CacheConversation: true})
	raw, err := builder.BuildMessages([]Message{{
		Role:    "user",
		Content: "describe",
		Parts: []*interfaces.Part{
			interfaces.NewPartFromURI("https://example.com/a.png", "image/png"),
			interfaces.NewPartFromText("describe"),
		},
	}})
	if err != nil {
		t.Fatalf("BuildMessages() error = %v", err)
	}
	var got []wireMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v: %s", err, raw)
	}
	blocks := got[0].Content
	if len(blocks) != 2 || blocks[0].Type != "image" {
		t.Fatalf("blocks = %s, want the image then the text", raw)
	}
	if blocks[0].CacheControl != nil || blocks[1].CacheControl == nil {
		t.Errorf("blocks = %s, want cache_control on the last block only", raw)
	}
}

func TestCheckContentRefusesUnsupportedMedia(t *testing.T) {
	for _, mime := range []string{"audio/wav", "image/bmp", "image/svg+xml"} {
		params := &interfaces.GenerateOptions{}
		interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, mime))(params)
		if err := (&AnthropicClient{}).checkContent(context.Background(), params); !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Errorf("checkContent(%s) = %v, want ErrContentUnsupported", mime, err)
		}
	}
}

func TestCheckContentRefusesURLsOnBedrockAndVertex(t *testing.T) {
	clients := map[string]*AnthropicClient{
		"bedrock": {BedrockConfig: &BedrockConfig{Enabled: true}},
		"vertex":  {VertexConfig: &VertexConfig{Enabled: true}},
	}
	for name, client := range clients {
		byURL := &interfaces.GenerateOptions{}
		interfaces.WithParts(interfaces.NewPartFromURI("https://example.com/a.png", "image/png"))(byURL)
		if err := client.checkContent(context.Background(), byURL); !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Errorf("%s: checkContent() with a URL = %v, want ErrContentUnsupported", name, err)
		}

		inline := &interfaces.GenerateOptions{}
		interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "image/png"))(inline)
		if err := client.checkContent(context.Background(), inline); err != nil {
			t.Errorf("%s: checkContent() with inline data = %v", name, err)
		}
	}
}
