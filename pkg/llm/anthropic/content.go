package anthropic

import (
	"context"
	"encoding/json"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// ImageSource is the source of an Anthropic image or document block: inline
// base64 data or a URL the API fetches.
type ImageSource struct {
	Type      string `json:"type"` // "base64" or "url"
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// MarshalJSON sends Content as a plain string, the way the API has always
// received it here, unless the message carries parts. Then content becomes a
// list of blocks, one per part, in the parts' order.
func (m Message) MarshalJSON() ([]byte, error) {
	type plain struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if len(m.Parts) == 0 {
		return json.Marshal(plain{Role: m.Role, Content: m.Content})
	}
	return json.Marshal(CacheableMessage{Role: m.Role, Content: m.contentBlocks()})
}

// contentBlocks returns the message as content blocks: one text block for a
// message without parts, or one block per part: images as image blocks and
// PDFs as document blocks. checkContent has already refused every part that
// is not text, an image, or a PDF.
func (m Message) contentBlocks() []CacheableContent {
	if len(m.Parts) == 0 {
		return []CacheableContent{{Type: "text", Text: m.Content}}
	}
	blocks := make([]CacheableContent, 0, len(m.Parts))
	for _, p := range m.Parts {
		blockType := "image"
		if p.IsPDF() {
			blockType = "document"
		}
		switch {
		case p.InlineData != nil:
			blocks = append(blocks, CacheableContent{Type: blockType, Source: &ImageSource{
				Type:      "base64",
				MediaType: p.InlineData.MIMEType,
				Data:      p.InlineData.Base64(),
			}})
		case p.FileData != nil:
			blocks = append(blocks, CacheableContent{Type: blockType, Source: &ImageSource{Type: "url", URL: p.FileData.FileURI}})
		default:
			blocks = append(blocks, CacheableContent{Type: "text", Text: p.Text})
		}
	}
	return blocks
}

// imageTypes are the image MIME types the Messages API accepts.
var imageTypes = []string{"image/jpeg", "image/png", "image/gif", "image/webp"}

// checkContent refuses every part of the request that is not text, a JPEG,
// PNG, GIF, or WebP image, or a PDF. On Bedrock and Vertex AI it also
// refuses URL sources, because both take base64 sources only.
func (c *AnthropicClient) checkContent(ctx context.Context, params *interfaces.GenerateOptions) error {
	base64Only := (c.BedrockConfig != nil && c.BedrockConfig.Enabled) || (c.VertexConfig != nil && c.VertexConfig.Enabled)
	return interfaces.CheckMedia(ctx, params, "anthropic", func(p *interfaces.Part) bool {
		if base64Only && p.FileData != nil {
			return false
		}
		return p.HasMIMEType(imageTypes...) || p.IsPDF()
	})
}
