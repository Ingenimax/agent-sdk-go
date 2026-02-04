package anthropic

import (
	"encoding/base64"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

type contentBlockParam struct {
	Type   string           `json:"type"`
	Text   string           `json:"text,omitempty"`
	Source *imageSourceParam `json:"source,omitempty"`
}

type imageSourceParam struct {
	// base64 | url
	Type string `json:"type"`
	// e.g. image/png, image/jpeg
	MediaType string `json:"media_type,omitempty"`
	// base64 string (no data: prefix)
	Data string `json:"data,omitempty"`
	// URL when Type == "url"
	URL string `json:"url,omitempty"`
}

func messageTextContent(msg Message) string {
	switch v := msg.Content.(type) {
	case string:
		return v
	case []contentBlockParam:
		var b strings.Builder
		for _, block := range v {
			if block.Type == "text" && block.Text != "" {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(block.Text)
			}
		}
		return b.String()
	case []any:
		// Best-effort: try to extract text blocks if request was built from generic maps.
		var b strings.Builder
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			t, _ := m["type"].(string)
			if t != "text" {
				continue
			}
			txt, _ := m["text"].(string)
			if txt == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(txt)
		}
		return b.String()
	default:
		return ""
	}
}

func hasAnyTextPart(parts []interfaces.ContentPart) bool {
	for _, p := range parts {
		if p.Type == "text" {
			return true
		}
	}
	return false
}

func parseDataURL(dataURL string) (mime string, b64 string, ok bool) {
	// data:<mime>;base64,<b64>
	if !strings.HasPrefix(dataURL, "data:") {
		return "", "", false
	}
	withoutPrefix := strings.TrimPrefix(dataURL, "data:")
	meta, data, found := strings.Cut(withoutPrefix, ",")
	if !found {
		return "", "", false
	}
	if !strings.Contains(meta, ";base64") {
		return "", "", false
	}
	mime = strings.TrimSuffix(meta, ";base64")
	if mime == "" {
		return "", "", false
	}
	b64 = data
	return mime, b64, true
}

func buildAnthropicContentFromParts(prompt string, parts []interfaces.ContentPart) any {
	blocks := make([]contentBlockParam, 0, len(parts)+1)

	// If prompt is provided and caller didn't include a text part, prepend it.
	if prompt != "" && !hasAnyTextPart(parts) {
		blocks = append(blocks, contentBlockParam{
			Type: "text",
			Text: prompt,
		})
	}

	for _, part := range parts {
		switch part.Type {
		case "text":
			if part.Text == "" {
				continue
			}
			blocks = append(blocks, contentBlockParam{
				Type: "text",
				Text: part.Text,
			})

		case "image_url":
			if part.ImageURL == nil || part.ImageURL.URL == "" {
				continue
			}

			// Prefer data URLs (base64) to avoid downloading.
			if mime, b64, ok := parseDataURL(part.ImageURL.URL); ok {
				// Validate base64 early to avoid sending obviously-bad payloads.
				if _, err := base64.StdEncoding.DecodeString(b64); err == nil {
					blocks = append(blocks, contentBlockParam{
						Type: "image",
						Source: &imageSourceParam{
							Type:      "base64",
							MediaType: mime,
							Data:      b64,
						},
					})
					continue
				}
			}

			// Fallback to URL source (may be rejected by some Anthropic endpoints/models).
			blocks = append(blocks, contentBlockParam{
				Type: "image",
				Source: &imageSourceParam{
					Type: "url",
					URL:  part.ImageURL.URL,
				},
			})

		case "image_file":
			// Anthropic message API doesn't accept OpenAI-style file IDs.
			// Keep this as a text hint rather than failing hard.
			if part.ImageFile != nil && part.ImageFile.FileID != "" {
				blocks = append(blocks, contentBlockParam{
					Type: "text",
					Text: "[image_file provided but not supported by Anthropic provider]",
				})
			}
		default:
			// Ignore unknown part types for forward-compatibility.
		}
	}

	// If nothing valid was built, fall back to plain text.
	if len(blocks) == 0 {
		return prompt
	}
	return blocks
}

