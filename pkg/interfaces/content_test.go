package interfaces

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type stubMemory struct{ messages []Message }

func (m stubMemory) AddMessage(context.Context, Message) error { return nil }
func (m stubMemory) GetMessages(context.Context, ...GetMessagesOption) ([]Message, error) {
	return m.messages, nil
}
func (m stubMemory) Clear(context.Context) error { return nil }

func TestPartValidate(t *testing.T) {
	tests := []struct {
		name    string
		part    *Part
		wantErr bool
	}{
		{"text", NewPartFromText("hi"), false},
		{"empty text", NewPartFromText(""), false},
		{"bytes", NewPartFromBytes([]byte{1}, "image/png"), false},
		{"uri", NewPartFromURI("https://example.com/a.pdf", "application/pdf"), false},
		{"nil", nil, true},
		{"two fields", &Part{Text: "hi", InlineData: &Blob{MIMEType: "image/png", Data: []byte{1}}}, true},
		{"bytes without type", NewPartFromBytes([]byte{1}, ""), true},
		{"empty bytes", NewPartFromBytes(nil, "image/png"), true},
		{"uri without type", NewPartFromURI("https://example.com/a.png", ""), true},
		{"empty uri", NewPartFromURI("", "image/png"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.part.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContentText(t *testing.T) {
	content := NewContentFromParts([]*Part{
		NewPartFromText("a"),
		NewPartFromBytes([]byte{1}, "image/png"),
		NewPartFromText("b"),
	}, MessageRoleUser)
	if got := content.Text(); got != "a\nb" {
		t.Errorf("Text() = %q, want %q", got, "a\nb")
	}
	if !HasMedia(content.Parts) || HasMedia(NewContentFromText("a", MessageRoleUser).Parts) {
		t.Error("HasMedia() is wrong")
	}
}

func TestBlobDataURL(t *testing.T) {
	if got := NewPartFromBytes([]byte{1, 2}, "image/png").InlineData.DataURL(); got != "data:image/png;base64,AQI=" {
		t.Errorf("DataURL() = %q", got)
	}
}

func TestMessagePartsRoundTripJSON(t *testing.T) {
	msg := Message{Role: MessageRoleUser, Content: "hi", Parts: []*Part{NewPartFromBytes([]byte{1, 2}, "image/png"), NewPartFromText("hi")}}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got Message
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got.Parts) != 2 || got.Parts[0].InlineData == nil || len(got.Parts[0].InlineData.Data) != 2 || got.Parts[1].Text != "hi" {
		t.Errorf("round trip = %+v, want the parts back", got.Parts)
	}
}

func TestRequestMedia(t *testing.T) {
	image := NewPartFromURI("https://example.com/a.png", "image/png")

	opts := &GenerateOptions{}
	WithParts(image, NewPartFromText("x"))(opts)
	if got, _ := RequestMedia(context.Background(), opts); len(got) != 1 {
		t.Errorf("without memory: got %d media parts, want 1", len(got))
	}

	// With memory, only media on user messages count, and the option is
	// ignored because the current turn is already in memory.
	opts.Memory = stubMemory{messages: []Message{
		{Role: MessageRoleUser, Content: "a", Parts: []*Part{image}},
		{Role: MessageRoleAssistant, Content: "b", Parts: []*Part{image}},
		{Role: MessageRoleUser, Content: "c", Parts: []*Part{image, NewPartFromText("c"), image}},
	}}
	if got, _ := RequestMedia(context.Background(), opts); len(got) != 3 {
		t.Errorf("with memory: got %d media parts, want 3", len(got))
	}
}

func TestCheckMedia(t *testing.T) {
	opts := &GenerateOptions{}
	WithParts(NewPartFromText("x"))(opts)
	if err := CheckMedia(context.Background(), opts, "p", nil); err != nil {
		t.Errorf("CheckMedia() with text only = %v", err)
	}

	WithParts(NewPartFromBytes([]byte{1}, "audio/wav"))(opts)
	if err := CheckMedia(context.Background(), opts, "p", (*Part).IsImage); !errors.Is(err, ErrContentUnsupported) {
		t.Errorf("CheckMedia() with audio = %v, want ErrContentUnsupported", err)
	}
}

func TestPromptText(t *testing.T) {
	opts := &GenerateOptions{}
	if got := PromptText("prompt", opts); got != "prompt" {
		t.Errorf("PromptText() without parts = %q", got)
	}
	WithParts(NewPartFromText("from parts"))(opts)
	if got := PromptText("prompt", opts); got != "from parts" {
		t.Errorf("PromptText() with parts = %q", got)
	}
	opts.Memory = stubMemory{}
	if got := PromptText("prompt", opts); got != "prompt" {
		t.Errorf("PromptText() with memory = %q", got)
	}
}
