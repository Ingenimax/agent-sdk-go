package imagegen

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage"
)

// Tool implements image generation as a tool for agents
type Tool struct {
	generator     interfaces.ImageGenerator
	storage       storage.ImageStorage
	maxPromptLen  int
	defaultAspect string
	defaultFormat string
}

// Option represents an option for configuring the tool
type Option func(*Tool)

// WithMaxPromptLength sets maximum prompt length
func WithMaxPromptLength(maxLen int) Option {
	return func(t *Tool) {
		t.maxPromptLen = maxLen
	}
}

// WithDefaultAspectRatio sets the default aspect ratio
func WithDefaultAspectRatio(ratio string) Option {
	return func(t *Tool) {
		t.defaultAspect = ratio
	}
}

// WithDefaultFormat sets the default output format
func WithDefaultFormat(format string) Option {
	return func(t *Tool) {
		t.defaultFormat = format
	}
}

// New creates a new image generation tool
func New(generator interfaces.ImageGenerator, storage storage.ImageStorage, options ...Option) *Tool {
	tool := &Tool{
		generator:     generator,
		storage:       storage,
		maxPromptLen:  2000,
		defaultAspect: "1:1",
		defaultFormat: "png",
	}

	for _, opt := range options {
		opt(tool)
	}

	return tool
}

// Name returns the tool name
func (t *Tool) Name() string {
	return "generate_image"
}

// DisplayName returns a human-friendly name
func (t *Tool) DisplayName() string {
	return "Image Generator"
}

// Description returns what the tool does
func (t *Tool) Description() string {
	return "Generate images from text descriptions using AI. Provide a detailed prompt describing the image you want to create. Returns the URL of the generated image."
}

// Internal returns false as this is a user-visible tool
func (t *Tool) Internal() bool {
	return false
}

// Parameters returns the tool's parameter specifications
func (t *Tool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"prompt": {
			Type:        "string",
			Description: "A detailed text description of the image to generate. Be specific about style, composition, colors, lighting, and subject matter.",
			Required:    true,
		},
		"aspect_ratio": {
			Type:        "string",
			Description: "The aspect ratio of the output image",
			Required:    false,
			Default:     t.defaultAspect,
			Enum:        []interface{}{"1:1", "16:9", "9:16", "4:3", "3:4"},
		},
		"output_format": {
			Type:        "string",
			Description: "The output image format",
			Required:    false,
			Default:     t.defaultFormat,
			Enum:        []interface{}{"png", "jpeg"},
		},
	}
}

// Run executes the tool with the given input
func (t *Tool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute implements the tool execution
func (t *Tool) Execute(ctx context.Context, args string) (string, error) {
	// Parse arguments
	var params struct {
		Prompt       string `json:"prompt"`
		AspectRatio  string `json:"aspect_ratio,omitempty"`
		OutputFormat string `json:"output_format,omitempty"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Validate prompt
	if params.Prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	if len(params.Prompt) > t.maxPromptLen {
		return "", fmt.Errorf("prompt exceeds maximum length of %d characters", t.maxPromptLen)
	}

	// Set defaults
	if params.AspectRatio == "" {
		params.AspectRatio = t.defaultAspect
	}
	if params.OutputFormat == "" {
		params.OutputFormat = t.defaultFormat
	}

	// Build request
	request := interfaces.ImageGenerationRequest{
		Prompt: params.Prompt,
		Options: &interfaces.ImageGenerationOptions{
			NumberOfImages: 1,
			AspectRatio:    params.AspectRatio,
			OutputFormat:   params.OutputFormat,
		},
	}

	// Generate image
	response, err := t.generator.GenerateImage(ctx, request)
	if err != nil {
		return "", fmt.Errorf("image generation failed: %w", err)
	}

	if len(response.Images) == 0 {
		return "", fmt.Errorf("no images were generated")
	}

	// Store image if storage is configured
	var imageURL string
	if t.storage != nil {
		metadata := storage.StorageMetadata{
			Prompt:    params.Prompt,
			CreatedAt: time.Now(),
		}

		url, err := t.storage.Store(ctx, &response.Images[0], metadata)
		if err != nil {
			// Log warning but don't fail - return base64 instead
			return t.formatResultWithBase64(response, params.Prompt), nil
		}
		imageURL = url
		response.Images[0].URL = url
	}

	// Format result
	return t.formatResult(response, params.Prompt, imageURL), nil
}

// formatResult creates a human-readable result string with URL
func (t *Tool) formatResult(response *interfaces.ImageGenerationResponse, prompt, imageURL string) string {
	result := fmt.Sprintf("Successfully generated image for prompt: \"%s\"\n\n", truncateString(prompt, 100))

	if imageURL != "" {
		result += fmt.Sprintf("Image URL: %s\n", imageURL)
	}

	result += fmt.Sprintf("Format: %s\n", response.Images[0].MimeType)
	result += fmt.Sprintf("Size: %d bytes\n", len(response.Images[0].Data))

	if response.Usage != nil {
		result += fmt.Sprintf("\nTokens used: %d input, %d output\n",
			response.Usage.InputTokens, response.Usage.OutputTokens)
	}

	return result
}

// formatResultWithBase64 creates a result string with base64 data (fallback when storage fails)
func (t *Tool) formatResultWithBase64(response *interfaces.ImageGenerationResponse, prompt string) string {
	result := fmt.Sprintf("Successfully generated image for prompt: \"%s\"\n\n", truncateString(prompt, 100))
	result += fmt.Sprintf("Format: %s\n", response.Images[0].MimeType)
	result += fmt.Sprintf("Size: %d bytes\n", len(response.Images[0].Data))
	result += fmt.Sprintf("\nBase64 data (first 100 chars): %s...\n", truncateString(response.Images[0].Base64, 100))
	result += "\nNote: Image storage unavailable. Base64 data can be used directly."

	return result
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// GetImageReference returns an ImageReference for storing in memory
func GetImageReference(image *interfaces.GeneratedImage, prompt string) interfaces.ImageReference {
	return interfaces.ImageReference{
		URL:       image.URL,
		MimeType:  image.MimeType,
		Prompt:    prompt,
		CreatedAt: time.Now(),
	}
}
