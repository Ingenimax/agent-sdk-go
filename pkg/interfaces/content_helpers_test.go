package interfaces

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// failingMemory is a memory whose reads fail.
type failingMemory struct{ err error }

func (m failingMemory) AddMessage(context.Context, Message) error { return nil }
func (m failingMemory) GetMessages(context.Context, ...GetMessagesOption) ([]Message, error) {
	return nil, m.err
}
func (m failingMemory) Clear(context.Context) error { return nil }

func TestNewContentAndPartConstructors(t *testing.T) {
	content := NewContentFromText("hi", MessageRoleUser)
	if content.Role != MessageRoleUser || len(content.Parts) != 1 || content.Parts[0].Text != "hi" {
		t.Errorf("NewContentFromText() = %+v, want one user text part", content)
	}

	uri := NewPartFromURI("gs://bucket/a.pdf", "application/pdf")
	if uri.FileData == nil || uri.FileData.FileURI != "gs://bucket/a.pdf" || uri.FileData.MIMEType != "application/pdf" {
		t.Errorf("NewPartFromURI() = %+v, want the URI and MIME type", uri)
	}
	if uri.InlineData != nil || uri.Text != "" {
		t.Errorf("NewPartFromURI() set more than FileData: %+v", uri)
	}
}

func TestPartValidateEmptyPart(t *testing.T) {
	if err := (&Part{}).Validate(); err != nil {
		t.Errorf("Validate() on an empty part = %v, want nil (empty text)", err)
	}
	both := &Part{
		InlineData: &Blob{MIMEType: "image/png", Data: []byte{1}},
		FileData:   &FileData{MIMEType: "image/png", FileURI: "https://example.com/a.png"},
	}
	if err := both.Validate(); err == nil {
		t.Error("Validate() accepted a part with both InlineData and FileData")
	}
}

func TestPartMIMETypeAndKind(t *testing.T) {
	tests := []struct {
		name    string
		part    *Part
		mime    string
		isText  bool
		isImage bool
		isPDF   bool
	}{
		{"text", NewPartFromText("hi"), "", true, false, false},
		{"empty part", &Part{}, "", true, false, false},
		{"inline image", NewPartFromBytes([]byte{1}, "image/png"), "image/png", false, true, false},
		{"file image", NewPartFromURI("https://example.com/a.jpg", "image/jpeg"), "image/jpeg", false, true, false},
		{"inline pdf", NewPartFromBytes([]byte{1}, "application/pdf"), "application/pdf", false, false, true},
		{"file pdf", NewPartFromURI("https://example.com/a.pdf", "application/pdf"), "application/pdf", false, false, true},
		{"audio", NewPartFromBytes([]byte{1}, "audio/wav"), "audio/wav", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.part.MIMEType(); got != tt.mime {
				t.Errorf("MIMEType() = %q, want %q", got, tt.mime)
			}
			if got := tt.part.IsText(); got != tt.isText {
				t.Errorf("IsText() = %v, want %v", got, tt.isText)
			}
			if got := tt.part.IsImage(); got != tt.isImage {
				t.Errorf("IsImage() = %v, want %v", got, tt.isImage)
			}
			if got := tt.part.IsPDF(); got != tt.isPDF {
				t.Errorf("IsPDF() = %v, want %v", got, tt.isPDF)
			}
		})
	}
}

func TestPartHasMIMEType(t *testing.T) {
	png := NewPartFromBytes([]byte{1}, "image/png")
	tests := []struct {
		name  string
		part  *Part
		types []string
		want  bool
	}{
		{"match", png, []string{"image/jpeg", "image/png"}, true},
		{"no match", png, []string{"image/jpeg"}, false},
		{"no types", png, nil, false},
		{"file data match", NewPartFromURI("https://example.com/a.pdf", "application/pdf"), []string{"application/pdf"}, true},
		{"text against empty type", NewPartFromText("hi"), []string{""}, true},
		{"text against image", NewPartFromText("hi"), []string{"image/png"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.part.HasMIMEType(tt.types...); got != tt.want {
				t.Errorf("HasMIMEType(%v) = %v, want %v", tt.types, got, tt.want)
			}
		})
	}
}

func TestBlobBase64(t *testing.T) {
	tests := []struct {
		name    string
		blob    *Blob
		b64     string
		dataURL string
	}{
		{"bytes", &Blob{MIMEType: "image/png", Data: []byte("hello")}, "aGVsbG8=", "data:image/png;base64,aGVsbG8="},
		{"empty", &Blob{MIMEType: "application/pdf"}, "", "data:application/pdf;base64,"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.blob.Base64(); got != tt.b64 {
				t.Errorf("Base64() = %q, want %q", got, tt.b64)
			}
			if got := tt.blob.DataURL(); got != tt.dataURL {
				t.Errorf("DataURL() = %q, want %q", got, tt.dataURL)
			}
		})
	}
}

func TestValidateParts(t *testing.T) {
	tests := []struct {
		name    string
		parts   []*Part
		wantErr string
	}{
		{"nil slice", nil, ""},
		{"all valid", []*Part{NewPartFromText("a"), NewPartFromBytes([]byte{1}, "image/png")}, ""},
		{"first invalid", []*Part{nil, NewPartFromText("a")}, "part 0"},
		{"second invalid", []*Part{NewPartFromText("a"), NewPartFromURI("", "image/png")}, "part 1"},
		{"names first of two invalid", []*Part{NewPartFromText("a"), nil, NewPartFromBytes(nil, "image/png")}, "part 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParts(tt.parts)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateParts() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.HasPrefix(err.Error(), tt.wantErr+":") {
				t.Errorf("ValidateParts() = %v, want an error starting with %q", err, tt.wantErr)
			}
		})
	}
}

func TestPartsTextAndHasMedia(t *testing.T) {
	image := NewPartFromBytes([]byte{1}, "image/png")
	tests := []struct {
		name     string
		parts    []*Part
		text     string
		hasMedia bool
	}{
		{"nil", nil, "", false},
		{"one text", []*Part{NewPartFromText("a")}, "a", false},
		{"media only", []*Part{image}, "", true},
		{"mixed", []*Part{NewPartFromText("a"), image, NewPartFromText("b")}, "a\nb", true},
		{"file data", []*Part{NewPartFromURI("https://example.com/a.pdf", "application/pdf")}, "", true},
		{"empty text part kept", []*Part{NewPartFromText(""), NewPartFromText("b")}, "\nb", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PartsText(tt.parts); got != tt.text {
				t.Errorf("PartsText() = %q, want %q", got, tt.text)
			}
			if got := HasMedia(tt.parts); got != tt.hasMedia {
				t.Errorf("HasMedia() = %v, want %v", got, tt.hasMedia)
			}
		})
	}
}

func TestWithPartsAppends(t *testing.T) {
	a, b, c := NewPartFromText("a"), NewPartFromText("b"), NewPartFromText("c")
	opts := &GenerateOptions{}
	WithParts(a)(opts)
	WithParts(b, c)(opts)
	WithParts()(opts)
	if len(opts.Parts) != 3 || opts.Parts[0] != a || opts.Parts[1] != b || opts.Parts[2] != c {
		t.Errorf("Parts = %+v, want a, b, c in order", opts.Parts)
	}
}

func TestPromptTextMemoryWithoutParts(t *testing.T) {
	opts := &GenerateOptions{Memory: stubMemory{}}
	if got := PromptText("prompt", opts); got != "prompt" {
		t.Errorf("PromptText() = %q, want the prompt", got)
	}
	opts = &GenerateOptions{}
	WithParts(NewPartFromBytes([]byte{1}, "image/png"))(opts)
	if got := PromptText("prompt", opts); got != "" {
		t.Errorf("PromptText() with only media parts = %q, want empty", got)
	}
}

func TestRequestMediaKeepsOrderAndIdentity(t *testing.T) {
	first := NewPartFromURI("https://example.com/1.png", "image/png")
	second := NewPartFromBytes([]byte{2}, "application/pdf")
	opts := &GenerateOptions{Memory: stubMemory{messages: []Message{
		{Role: MessageRoleUser, Parts: []*Part{first}},
		{Role: MessageRoleSystem, Parts: []*Part{NewPartFromBytes([]byte{9}, "image/png")}},
		{Role: MessageRoleUser, Parts: []*Part{NewPartFromText("x"), second}},
	}}}
	WithParts(NewPartFromBytes([]byte{3}, "image/gif"))(opts)

	got, err := RequestMedia(context.Background(), opts)
	if err != nil {
		t.Fatalf("RequestMedia() error = %v", err)
	}
	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Errorf("RequestMedia() = %+v, want the two user media parts in order", got)
	}
}

func TestRequestMediaNone(t *testing.T) {
	got, err := RequestMedia(context.Background(), &GenerateOptions{})
	if err != nil || got != nil {
		t.Errorf("RequestMedia() = %v, %v, want nil, nil", got, err)
	}
}

func TestRequestMediaMemoryError(t *testing.T) {
	sentinel := errors.New("redis down")
	opts := &GenerateOptions{Memory: failingMemory{err: sentinel}}

	if _, err := RequestMedia(context.Background(), opts); !errors.Is(err, sentinel) {
		t.Errorf("RequestMedia() error = %v, want it to wrap the memory error", err)
	}
	err := CheckMedia(context.Background(), opts, "p", nil)
	if !errors.Is(err, sentinel) || errors.Is(err, ErrContentUnsupported) {
		t.Errorf("CheckMedia() error = %v, want the memory error only", err)
	}
}

func TestCheckMediaTable(t *testing.T) {
	inlineAudio := NewPartFromBytes([]byte{1}, "audio/wav")
	fileImage := NewPartFromURI("https://example.com/a.png", "image/png")
	tests := []struct {
		name      string
		parts     []*Part
		supported func(*Part) bool
		wantErr   string
	}{
		{"no parts, nil supported", nil, nil, ""},
		{"text only, nil supported", []*Part{NewPartFromText("x")}, nil, ""},
		{"inline media, nil supported", []*Part{inlineAudio}, nil, `p: inline data with MIME type "audio/wav"`},
		{"file media, nil supported", []*Part{fileImage}, nil, `p: file data with MIME type "image/png"`},
		{"all supported", []*Part{fileImage, NewPartFromText("x")}, (*Part).IsImage, ""},
		{"first rejected named", []*Part{fileImage, inlineAudio}, (*Part).IsImage, `"audio/wav"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &GenerateOptions{}
			WithParts(tt.parts...)(opts)
			err := CheckMedia(context.Background(), opts, "p", tt.supported)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("CheckMedia() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, ErrContentUnsupported) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("CheckMedia() = %v, want ErrContentUnsupported containing %q", err, tt.wantErr)
			}
		})
	}
}
