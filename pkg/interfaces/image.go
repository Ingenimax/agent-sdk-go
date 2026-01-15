package interfaces

import (
	"context"
	"errors"
	"time"
)

// Image generation errors
var (
	// ErrImageGenerationNotSupported indicates the model doesn't support image generation
	ErrImageGenerationNotSupported = errors.New("image generation not supported by this model")

	// ErrContentBlocked indicates the content was blocked by safety filters
	ErrContentBlocked = errors.New("content blocked by safety filters")

	// ErrRateLimitExceeded indicates rate limiting triggered
	ErrRateLimitExceeded = errors.New("rate limit exceeded")

	// ErrInvalidPrompt indicates an invalid or empty prompt
	ErrInvalidPrompt = errors.New("invalid or empty prompt")

	// ErrStorageUploadFailed indicates failed to upload image to storage
	ErrStorageUploadFailed = errors.New("failed to upload image to storage")
)

// ImageGenerator represents an LLM that can generate images
type ImageGenerator interface {
	// GenerateImage generates one or more images from a text prompt
	GenerateImage(ctx context.Context, request ImageGenerationRequest) (*ImageGenerationResponse, error)

	// SupportsImageGeneration returns true if this LLM supports image generation
	SupportsImageGeneration() bool

	// SupportedImageFormats returns the output formats supported (e.g., "png", "jpeg")
	SupportedImageFormats() []string
}

// ImageGenerationRequest represents a request to generate an image
type ImageGenerationRequest struct {
	// Prompt is the text description of the image to generate (required)
	Prompt string

	// ReferenceImage is an optional input image for image-to-image generation
	ReferenceImage *ImageData

	// Options contains generation configuration
	Options *ImageGenerationOptions
}

// ImageGenerationOptions configures image generation behavior
type ImageGenerationOptions struct {
	// NumberOfImages specifies how many images to generate (default: 1)
	NumberOfImages int

	// AspectRatio controls the image dimensions (e.g., "1:1", "16:9", "9:16", "4:3", "3:4")
	AspectRatio string

	// OutputFormat specifies the desired output format ("png", "jpeg")
	OutputFormat string

	// SafetyFilterLevel controls content filtering ("none", "low", "medium", "high")
	SafetyFilterLevel string
}

// ImageGenerationResponse represents the result of image generation
type ImageGenerationResponse struct {
	// Images contains the generated images
	Images []GeneratedImage

	// Usage contains token/cost information if available
	Usage *ImageUsage

	// Metadata contains provider-specific information
	Metadata map[string]interface{}
}

// GeneratedImage represents a single generated image
type GeneratedImage struct {
	// Data contains the raw image bytes
	Data []byte

	// Base64 contains the base64-encoded image data
	Base64 string

	// MimeType is the MIME type of the image (e.g., "image/png", "image/jpeg")
	MimeType string

	// URL is the storage URL (populated after upload to storage)
	URL string

	// RevisedPrompt is the prompt actually used by the model (may differ from input)
	RevisedPrompt string

	// FinishReason indicates why generation stopped
	FinishReason string
}

// ImageData represents input image data for image-to-image generation
type ImageData struct {
	// Data contains raw image bytes
	Data []byte

	// Base64 contains base64-encoded image data
	Base64 string

	// MimeType is the MIME type (e.g., "image/jpeg", "image/png")
	MimeType string

	// URL is a URL to fetch the image from
	URL string
}

// ImageUsage represents usage/cost information for image generation
type ImageUsage struct {
	// InputTokens used for the prompt
	InputTokens int

	// OutputTokens used for generation
	OutputTokens int

	// ImagesGenerated is the number of images produced
	ImagesGenerated int
}

// ImageReference represents a reference to a generated image stored in memory
type ImageReference struct {
	// URL is the storage URL of the generated image
	URL string

	// MimeType is the image MIME type
	MimeType string

	// Prompt is the original prompt used to generate the image
	Prompt string

	// CreatedAt is the timestamp when the image was generated
	CreatedAt time.Time
}

// ImageGenerationOption represents options for image generation
type ImageGenerationOption func(*ImageGenerationOptions)

// WithNumberOfImages sets how many images to generate
func WithNumberOfImages(n int) ImageGenerationOption {
	return func(opts *ImageGenerationOptions) {
		opts.NumberOfImages = n
	}
}

// WithAspectRatio sets the aspect ratio
func WithAspectRatio(ratio string) ImageGenerationOption {
	return func(opts *ImageGenerationOptions) {
		opts.AspectRatio = ratio
	}
}

// WithOutputFormat sets the output image format
func WithOutputFormat(format string) ImageGenerationOption {
	return func(opts *ImageGenerationOptions) {
		opts.OutputFormat = format
	}
}

// WithSafetyFilter sets the safety filter level
func WithSafetyFilter(level string) ImageGenerationOption {
	return func(opts *ImageGenerationOptions) {
		opts.SafetyFilterLevel = level
	}
}

// ApplyImageGenerationOptions applies a list of options to ImageGenerationOptions
func ApplyImageGenerationOptions(opts *ImageGenerationOptions, options ...ImageGenerationOption) {
	for _, opt := range options {
		opt(opts)
	}
}

// DefaultImageGenerationOptions returns the default options for image generation
func DefaultImageGenerationOptions() *ImageGenerationOptions {
	return &ImageGenerationOptions{
		NumberOfImages:    1,
		AspectRatio:       "1:1",
		OutputFormat:      "png",
		SafetyFilterLevel: "medium",
	}
}
