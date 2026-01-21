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

	// ErrSessionNotFound indicates the image editing session was not found
	ErrSessionNotFound = errors.New("image editing session not found")

	// ErrSessionExpired indicates the image editing session has expired
	ErrSessionExpired = errors.New("image editing session has expired")

	// ErrMultiTurnNotSupported indicates multi-turn image editing is not supported
	ErrMultiTurnNotSupported = errors.New("multi-turn image editing not supported by this model")
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

// =============================================================================
// Multi-Turn Image Editing Types
// =============================================================================

// ImageEditSession represents a multi-turn image editing session
// that maintains conversation context for iterative image creation and modification.
type ImageEditSession interface {
	// SendMessage sends a text message and returns the response (text and/or image)
	SendMessage(ctx context.Context, message string, options *ImageEditOptions) (*ImageEditResponse, error)

	// SendMessageWithImage sends a message with an image reference for editing
	SendMessageWithImage(ctx context.Context, message string, image *ImageData, options *ImageEditOptions) (*ImageEditResponse, error)

	// GetHistory returns the conversation history for this session
	GetHistory() []ImageEditTurn

	// Close closes the session and releases resources
	Close() error
}

// ImageEditOptions configures a single turn in the image editing session
type ImageEditOptions struct {
	// AspectRatio controls the output image dimensions
	// Supported values: "1:1", "2:3", "3:2", "16:9", "21:9"
	AspectRatio string

	// ImageSize controls the output image resolution
	// Supported values: "1K", "2K", "4K"
	ImageSize string
}

// ImageEditResponse represents the result of an image editing turn
type ImageEditResponse struct {
	// Text contains the text response from the model (if any)
	Text string

	// Images contains generated/edited images (if any)
	Images []GeneratedImage

	// Usage contains token/cost information if available
	Usage *ImageUsage

	// Metadata contains provider-specific information
	Metadata map[string]interface{}
}

// ImageEditTurn represents a single turn in the image editing conversation
type ImageEditTurn struct {
	// Role is either "user" or "model"
	Role string

	// Message is the text content of this turn
	Message string

	// Images contains any images in this turn
	Images []*ImageData
}

// MultiTurnImageEditor is an interface for LLM providers that support
// multi-turn conversational image editing sessions.
type MultiTurnImageEditor interface {
	// CreateImageEditSession creates a new multi-turn image editing session
	CreateImageEditSession(ctx context.Context, options *ImageEditSessionOptions) (ImageEditSession, error)

	// SupportsMultiTurnImageEditing returns true if this provider supports multi-turn editing
	SupportsMultiTurnImageEditing() bool
}

// ImageEditSessionOptions configures a new image editing session
type ImageEditSessionOptions struct {
	// Model specifies the model to use (e.g., "gemini-3-pro-image-preview")
	// If empty, the provider's default image editing model will be used
	Model string

	// SystemInstruction provides optional system-level guidance for the session
	SystemInstruction string

	// Tools lists optional tools to enable (e.g., "google_search")
	Tools []string
}

// DefaultImageEditOptions returns default options for image editing
func DefaultImageEditOptions() *ImageEditOptions {
	return &ImageEditOptions{
		AspectRatio: "1:1",
		ImageSize:   "1K",
	}
}
