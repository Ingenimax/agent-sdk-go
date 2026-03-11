package gemini

import (
	"encoding/base64"
	"path"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"google.golang.org/genai"
)

func hasAnyTextPart(parts []interfaces.ContentPart) bool {
	for _, p := range parts {
		if p.Type == "text" {
			return true
		}
	}
	return false
}

func parseDataURL(dataURL string) (mime string, data []byte, ok bool) {
	// data:<mime>;base64,<b64>
	if !strings.HasPrefix(dataURL, "data:") {
		return "", nil, false
	}
	withoutPrefix := strings.TrimPrefix(dataURL, "data:")
	meta, b64, found := strings.Cut(withoutPrefix, ",")
	if !found {
		return "", nil, false
	}
	if !strings.Contains(meta, ";base64") {
		return "", nil, false
	}
	mime = strings.TrimSuffix(meta, ";base64")
	if mime == "" {
		return "", nil, false
	}
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", nil, false
	}
	return mime, decoded, true
}

func guessImageMIMEFromURL(u string) string {
	ext := strings.ToLower(path.Ext(u))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return ""
	}
}

func buildGeminiPartsFromContentParts(prompt string, parts []interfaces.ContentPart) []*genai.Part {
	out := make([]*genai.Part, 0, len(parts)+1)

	// If prompt is provided and caller didn't include a text part, prepend it.
	if prompt != "" && !hasAnyTextPart(parts) {
		out = append(out, &genai.Part{Text: prompt})
	}

	for _, part := range parts {
		switch part.Type {
		case "text":
			if part.Text == "" {
				continue
			}
			out = append(out, &genai.Part{Text: part.Text})

		case "image_url":
			if part.ImageURL == nil || part.ImageURL.URL == "" {
				continue
			}

			if mime, data, ok := parseDataURL(part.ImageURL.URL); ok {
				out = append(out, &genai.Part{
					InlineData: &genai.Blob{
						MIMEType: mime,
						Data:     data,
					},
				})
				continue
			}

			// Otherwise treat as URI-based file input.
			out = append(out, &genai.Part{
				FileData: &genai.FileData{
					FileURI:  part.ImageURL.URL,
					MIMEType: guessImageMIMEFromURL(part.ImageURL.URL),
				},
			})

		case "image_file":
			// Gemini file IDs/URIs are not compatible with OpenAI file ids.
			// Keep this as a text hint rather than failing.
			out = append(out, &genai.Part{Text: "[image_file provided but not supported by Gemini provider]"})

		default:
			// Ignore unknown part types for forward-compatibility.
		}
	}

	// Fallback to prompt if nothing was built.
	if len(out) == 0 {
		return []*genai.Part{{Text: prompt}}
	}

	return out
}

