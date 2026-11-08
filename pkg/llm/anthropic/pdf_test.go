package anthropic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestMessageSendsPDFsAsDocumentBlocks(t *testing.T) {
	raw, err := json.Marshal(Message{Role: "user", Content: "summarize", Parts: []*interfaces.Part{
		interfaces.NewPartFromBytes([]byte{1, 2}, "application/pdf"),
		interfaces.NewPartFromURI("https://example.com/a.pdf", "application/pdf"),
		interfaces.NewPartFromText("summarize"),
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
	if b := got.Content[0]; b.Type != "document" || b.Source.Type != "base64" || b.Source.MediaType != "application/pdf" || b.Source.Data != "AQI=" {
		t.Errorf("block 0 = %s, want an inline PDF document", raw)
	}
	if b := got.Content[1]; b.Type != "document" || b.Source.Type != "url" || b.Source.URL != "https://example.com/a.pdf" {
		t.Errorf("block 1 = %s, want a URL PDF document", raw)
	}
}

func TestCheckContentAcceptsPDFs(t *testing.T) {
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(
		interfaces.NewPartFromBytes([]byte{1}, "application/pdf"),
		interfaces.NewPartFromURI("https://example.com/a.pdf", "application/pdf"),
	)(params)
	if err := (&AnthropicClient{}).checkContent(context.Background(), params); err != nil {
		t.Errorf("checkContent() with PDFs = %v", err)
	}
}
