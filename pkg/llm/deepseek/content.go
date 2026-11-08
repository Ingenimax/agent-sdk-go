package deepseek

import (
	"context"
	"encoding/json"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// imageTypes are the image MIME types DeepSeek's Chat Completions accepts.
var imageTypes = []string{"image/jpeg", "image/png", "image/gif", "image/webp"}

// contentPart is one entry of a Chat Completions content list.
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

// MarshalJSON sends Content as a plain string unless the message carries
// parts. Then content becomes a list with one entry per part, in order.
func (m Message) MarshalJSON() ([]byte, error) {
	type plain Message
	if len(m.Parts) == 0 {
		return json.Marshal(plain(m))
	}
	return json.Marshal(struct {
		plain
		Content []contentPart `json:"content"`
	}{plain(m), contentParts(m.Parts)})
}

// contentParts converts parts to Chat Completions content entries: text as
// text, and images as image_url entries holding a data URL or the image's
// URL. Empty text parts are left out. checkContent has already refused every
// other part.
func contentParts(parts []*interfaces.Part) []contentPart {
	out := make([]contentPart, 0, len(parts))
	for _, p := range parts {
		switch {
		case p.InlineData != nil:
			out = append(out, contentPart{Type: "image_url", ImageURL: &imageURL{URL: p.InlineData.DataURL()}})
		case p.FileData != nil:
			out = append(out, contentPart{Type: "image_url", ImageURL: &imageURL{URL: p.FileData.FileURI}})
		case p.Text != "":
			out = append(out, contentPart{Type: "text", Text: p.Text})
		}
	}
	return out
}

// checkContent refuses every part of the request that is not text or a
// JPEG, PNG, GIF, or WebP image, inline or by URL.
func checkContent(ctx context.Context, params *interfaces.GenerateOptions) error {
	return interfaces.CheckMedia(ctx, params, "deepseek", func(p *interfaces.Part) bool {
		return p.HasMIMEType(imageTypes...)
	})
}
