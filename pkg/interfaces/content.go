package interfaces

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Content is a multipart message for an agent: an ordered list of parts, such
// as text, an image, or a PDF. It follows the shape of Google ADK's
// genai.Content.
//
// Pass it to the agent's RunWithContent:
//
//	content := interfaces.NewContentFromParts([]*interfaces.Part{
//		interfaces.NewPartFromBytes(png, "image/png"),
//		interfaces.NewPartFromText("What is in this picture?"),
//	}, interfaces.MessageRoleUser)
//	answer, err := myAgent.RunWithContent(ctx, content)
type Content struct {
	// Role is the sender of the content. Run input must be from the user; an
	// empty role means user.
	Role MessageRole `json:"role,omitempty"`

	// Parts are the pieces of the content, in the order the model reads them.
	Parts []*Part `json:"parts,omitempty"`
}

// Part is one piece of a Content. Exactly one field is set.
//
// The MIME type of inline or file data says what the data is, for example
// "image/png" or "application/pdf", so new kinds of media need no new fields.
type Part struct {
	// Text is plain text.
	Text string `json:"text,omitempty"`

	// InlineData is data sent with the request.
	InlineData *Blob `json:"inlineData,omitempty"`

	// FileData is data the provider fetches from a URI.
	FileData *FileData `json:"fileData,omitempty"`
}

// Blob is raw data with its MIME type.
type Blob struct {
	MIMEType string `json:"mimeType"`
	Data     []byte `json:"data"`

	// DisplayName optionally names the data, such as a file name. Providers
	// that label files, such as OpenAI for PDFs, send it.
	DisplayName string `json:"displayName,omitempty"`
}

// FileData is a URI to data, with the data's MIME type.
type FileData struct {
	MIMEType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`

	// DisplayName optionally names the data, such as a file name.
	DisplayName string `json:"displayName,omitempty"`
}

// ErrContentUnsupported is returned by a provider, or by an agent path, that
// cannot send a part to the model. The part is refused instead of dropped.
var ErrContentUnsupported = errors.New("content part is not supported")

// NewContentFromText returns a content with one text part.
func NewContentFromText(text string, role MessageRole) *Content {
	return &Content{Role: role, Parts: []*Part{NewPartFromText(text)}}
}

// NewContentFromParts returns a content with the given parts.
func NewContentFromParts(parts []*Part, role MessageRole) *Content {
	return &Content{Role: role, Parts: parts}
}

// NewPartFromText returns a text part.
func NewPartFromText(text string) *Part {
	return &Part{Text: text}
}

// NewPartFromBytes returns a part that sends data inline, for example an
// image read from disk.
func NewPartFromBytes(data []byte, mimeType string) *Part {
	return &Part{InlineData: &Blob{MIMEType: mimeType, Data: data}}
}

// NewPartFromURI returns a part that points at data by URI, for example an
// image URL.
func NewPartFromURI(uri, mimeType string) *Part {
	return &Part{FileData: &FileData{MIMEType: mimeType, FileURI: uri}}
}

// Text returns the text parts of the content, joined by newlines.
func (c *Content) Text() string {
	return PartsText(c.Parts)
}

// Validate reports whether the part sets at most one field, and whether inline
// and file data carry a MIME type and their data or URI. A part with no field
// set is empty text.
func (p *Part) Validate() error {
	if p == nil {
		return errors.New("part is nil")
	}
	set := 0
	for _, ok := range []bool{p.Text != "", p.InlineData != nil, p.FileData != nil} {
		if ok {
			set++
		}
	}
	switch {
	case set > 1:
		return errors.New("part sets more than one of Text, InlineData, and FileData")
	case p.InlineData != nil && p.InlineData.MIMEType == "":
		return errors.New("inline data needs a MIME type")
	case p.InlineData != nil && len(p.InlineData.Data) == 0:
		return errors.New("inline data is empty")
	case p.FileData != nil && p.FileData.MIMEType == "":
		return errors.New("file data needs a MIME type")
	case p.FileData != nil && p.FileData.FileURI == "":
		return errors.New("file data needs a URI")
	}
	return nil
}

// IsText reports whether the part is text.
func (p *Part) IsText() bool {
	return p.InlineData == nil && p.FileData == nil
}

// MIMEType returns the MIME type of inline or file data, or "" for text.
func (p *Part) MIMEType() string {
	switch {
	case p.InlineData != nil:
		return p.InlineData.MIMEType
	case p.FileData != nil:
		return p.FileData.MIMEType
	}
	return ""
}

// IsImage reports whether the part is inline or file data with an image MIME
// type.
func (p *Part) IsImage() bool {
	return strings.HasPrefix(p.MIMEType(), "image/")
}

// HasMIMEType reports whether the part's MIME type is one of types.
func (p *Part) HasMIMEType(types ...string) bool {
	return slices.Contains(types, p.MIMEType())
}

// IsPDF reports whether the part is inline or file data with the PDF MIME
// type.
func (p *Part) IsPDF() bool {
	return p.MIMEType() == "application/pdf"
}

// Base64 returns the data as standard base64.
func (b *Blob) Base64() string {
	return base64.StdEncoding.EncodeToString(b.Data)
}

// DataURL returns the data as a data URL, such as "data:image/png;base64,...".
func (b *Blob) DataURL() string {
	return "data:" + b.MIMEType + ";base64," + b.Base64()
}

// ValidateParts validates every part and names the first invalid one.
func ValidateParts(parts []*Part) error {
	for i, p := range parts {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("part %d: %w", i, err)
		}
	}
	return nil
}

// PartsText returns the text parts, joined by newlines.
func PartsText(parts []*Part) string {
	var texts []string
	for _, p := range parts {
		if p.IsText() {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// HasMedia reports whether any part is not text.
func HasMedia(parts []*Part) bool {
	for _, p := range parts {
		if !p.IsText() {
			return true
		}
	}
	return false
}

// WithParts sets the current user turn as content parts. The parts replace
// the prompt text, so include the text as a text part.
//
// Providers read these only when no memory is set. With memory, the current
// turn comes from the last user message in memory, so the parts must be
// stored on that message (Message.Parts) instead. The agent does this for
// you.
func WithParts(parts ...*Part) GenerateOption {
	return func(options *GenerateOptions) {
		options.Parts = append(options.Parts, parts...)
	}
}

// PromptText returns the text of the current user turn, for a provider that
// sends text only: the text parts set with WithParts when there is no memory
// and parts are set, or prompt otherwise.
func PromptText(prompt string, options *GenerateOptions) string {
	if options.Memory == nil && len(options.Parts) > 0 {
		return PartsText(options.Parts)
	}
	return prompt
}

// RequestMedia returns every part of a request that is not text, in
// conversation order. With memory set, those are the parts of user messages
// in memory. Without memory, they are the parts set with WithParts.
func RequestMedia(ctx context.Context, options *GenerateOptions) ([]*Part, error) {
	parts := options.Parts
	if options.Memory != nil {
		messages, err := options.Memory.GetMessages(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to read memory: %w", err)
		}
		parts = nil
		for _, msg := range messages {
			if msg.Role == MessageRoleUser {
				parts = append(parts, msg.Parts...)
			}
		}
	}
	var media []*Part
	for _, p := range parts {
		if !p.IsText() {
			media = append(media, p)
		}
	}
	return media, nil
}

// CheckMedia returns ErrContentUnsupported, naming the provider and the MIME
// type, for the first non-text part of the request that supported rejects.
// Providers call it before building a request, so a part they cannot send is
// refused instead of dropped. A nil supported rejects every non-text part.
func CheckMedia(ctx context.Context, options *GenerateOptions, provider string, supported func(*Part) bool) error {
	media, err := RequestMedia(ctx, options)
	if err != nil {
		return err
	}
	for _, p := range media {
		if supported == nil || !supported(p) {
			kind := "file data"
			if p.InlineData != nil {
				kind = "inline data"
			}
			return fmt.Errorf("%s: %s with MIME type %q: %w", provider, kind, p.MIMEType(), ErrContentUnsupported)
		}
	}
	return nil
}
