package interfaces

// ContentPart represents a single content part in a multimodal message.
//
// This is designed to be flexible and map cleanly to major provider formats
// (e.g. OpenAI chat content[]).
//
// Supported (MVP):
// - type="text"
// - type="image_url"
// - type="image_file"
//
// Audio/video parts can be added later without breaking callers.
type ContentPart struct {
	// Type indicates the content part type.
	// Valid values (MVP): "text" | "image_url" | "image_file"
	Type string `json:"type"`

	// Text is used when Type == "text".
	Text string `json:"text,omitempty"`

	// ImageURL is used when Type == "image_url".
	ImageURL *ImageURL `json:"image_url,omitempty"`

	// ImageFile is used when Type == "image_file".
	ImageFile *ImageFile `json:"image_file,omitempty"`
}

// ImageURL represents an image URL content part.
type ImageURL struct {
	URL string `json:"url"`
	// Detail is optional: "low" | "high" | "auto"
	Detail string `json:"detail,omitempty"`
}

// ImageFile represents an image file content part (provider-specific file id).
type ImageFile struct {
	FileID string `json:"file_id"`
}

// TextPart is a helper to construct a text content part.
func TextPart(text string) ContentPart {
	return ContentPart{
		Type: "text",
		Text: text,
	}
}

// ImageURLPart is a helper to construct an image_url content part.
func ImageURLPart(url string, detail string) ContentPart {
	part := ContentPart{
		Type: "image_url",
		ImageURL: &ImageURL{
			URL: url,
		},
	}
	if detail != "" {
		part.ImageURL.Detail = detail
	}
	return part
}

// ImageFilePart is a helper to construct an image_file content part.
func ImageFilePart(fileID string) ContentPart {
	return ContentPart{
		Type: "image_file",
		ImageFile: &ImageFile{
			FileID: fileID,
		},
	}
}

