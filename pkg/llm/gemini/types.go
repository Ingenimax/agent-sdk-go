package gemini

import "fmt"

// ReasoningMode defines the reasoning approach for the model
type ReasoningMode string

const (
	ReasoningModeNone          ReasoningMode = "none"
	ReasoningModeMinimal       ReasoningMode = "minimal"
	ReasoningModeComprehensive ReasoningMode = "comprehensive"
)

// SafetyThreshold represents the safety filtering threshold
type SafetyThreshold string

const (
	SafetyThresholdUnspecified         SafetyThreshold = "HARM_BLOCK_THRESHOLD_UNSPECIFIED"
	SafetyThresholdBlockLowAndAbove    SafetyThreshold = "BLOCK_LOW_AND_ABOVE"
	SafetyThresholdBlockMediumAndAbove SafetyThreshold = "BLOCK_MEDIUM_AND_ABOVE"
	SafetyThresholdBlockOnlyHigh       SafetyThreshold = "BLOCK_ONLY_HIGH"
	SafetyThresholdBlockNone           SafetyThreshold = "BLOCK_NONE"
)

// HarmCategory represents the harm category for safety filtering
type HarmCategory string

const (
	HarmCategoryUnspecified      HarmCategory = "HARM_CATEGORY_UNSPECIFIED"
	HarmCategoryDerogatory       HarmCategory = "HARM_CATEGORY_DEROGATORY"
	HarmCategoryToxicity         HarmCategory = "HARM_CATEGORY_TOXICITY"
	HarmCategoryViolence         HarmCategory = "HARM_CATEGORY_VIOLENCE"
	HarmCategorySexual           HarmCategory = "HARM_CATEGORY_SEXUAL"
	HarmCategoryMedical          HarmCategory = "HARM_CATEGORY_MEDICAL"
	HarmCategoryDangerous        HarmCategory = "HARM_CATEGORY_DANGEROUS"
	HarmCategoryHarassment       HarmCategory = "HARM_CATEGORY_HARASSMENT"
	HarmCategoryHateSpeech       HarmCategory = "HARM_CATEGORY_HATE_SPEECH"
	HarmCategorySexuallyExplicit HarmCategory = "HARM_CATEGORY_SEXUALLY_EXPLICIT"
	HarmCategoryDangerousContent HarmCategory = "HARM_CATEGORY_DANGEROUS_CONTENT"
)

// SafetySetting represents a safety setting for content filtering
type SafetySetting struct {
	Category  HarmCategory    `json:"category"`
	Threshold SafetyThreshold `json:"threshold"`
}

// DefaultSafetySettings returns default safety settings
func DefaultSafetySettings() []SafetySetting {
	return []SafetySetting{
		{
			Category:  HarmCategoryHarassment,
			Threshold: SafetyThresholdBlockMediumAndAbove,
		},
		{
			Category:  HarmCategoryHateSpeech,
			Threshold: SafetyThresholdBlockMediumAndAbove,
		},
		{
			Category:  HarmCategorySexuallyExplicit,
			Threshold: SafetyThresholdBlockMediumAndAbove,
		},
		{
			Category:  HarmCategoryDangerousContent,
			Threshold: SafetyThresholdBlockMediumAndAbove,
		},
	}
}

// ThinkingConfig represents thinking/reasoning configuration for Gemini models
type ThinkingConfig struct {
	// Whether to include thinking content in responses
	IncludeThoughts bool
	// Maximum tokens allocated for thinking (nil for dynamic thinking)
	ThinkingBudget *int32
	// Thought signatures for context preservation across multi-turn conversations
	ThoughtSignatures [][]byte
}

// DefaultThinkingConfig returns default thinking configuration
func DefaultThinkingConfig() ThinkingConfig {
	return ThinkingConfig{
		IncludeThoughts:   false,
		ThinkingBudget:    nil, // Dynamic thinking by default
		ThoughtSignatures: nil,
	}
}

// ModelCapabilities represents the capabilities of different Gemini models
type ModelCapabilities struct {
	SupportsStreaming        bool
	SupportsToolCalling      bool
	SupportsVision           bool
	SupportsAudio            bool
	SupportsThinking         bool
	SupportsImageGeneration  bool // Whether the model can generate images
	MaxInputTokens           int
	MaxOutputTokens          int
	MaxThinkingTokens        *int32 // nil if thinking not supported
	SupportedMimeTypes       []string
	SupportedOutputFormats   []string // Output formats for image generation (e.g., "png", "jpeg")
}

// GetModelCapabilities returns the capabilities for a given model
func GetModelCapabilities(model string) ModelCapabilities {
	switch model {
	case ModelGemini25Pro:
		maxThinking := int32(32768) // 32K tokens for Pro
		return ModelCapabilities{
			SupportsStreaming:   true,
			SupportsToolCalling: true,
			SupportsVision:      true,
			SupportsAudio:       true,
			SupportsThinking:    true,
			MaxInputTokens:      2097152, // 2M tokens
			MaxOutputTokens:     8192,
			MaxThinkingTokens:   &maxThinking,
			SupportedMimeTypes: []string{
				"image/png", "image/jpeg", "image/webp", "image/heic", "image/heif",
				"audio/wav", "audio/mp3", "audio/aiff", "audio/aac", "audio/ogg", "audio/flac",
				"video/mp4", "video/mpeg", "video/mov", "video/avi", "video/flv", "video/mpv", "video/webm", "video/wmv", "video/3gpp",
				"text/plain", "text/html", "text/css", "text/javascript", "application/x-javascript", "text/x-typescript",
				"application/pdf",
			},
		}
	case ModelGemini25Flash:
		maxThinking := int32(24576) // 24K tokens for Flash
		return ModelCapabilities{
			SupportsStreaming:   true,
			SupportsToolCalling: true,
			SupportsVision:      true,
			SupportsAudio:       true,
			SupportsThinking:    true,
			MaxInputTokens:      1048576, // 1M tokens
			MaxOutputTokens:     8192,
			MaxThinkingTokens:   &maxThinking,
			SupportedMimeTypes: []string{
				"image/png", "image/jpeg", "image/webp", "image/heic", "image/heif",
				"audio/wav", "audio/mp3", "audio/aiff", "audio/aac", "audio/ogg", "audio/flac",
				"video/mp4", "video/mpeg", "video/mov", "video/avi", "video/flv", "video/mpv", "video/webm", "video/wmv", "video/3gpp",
				"text/plain", "text/html", "text/css", "text/javascript", "application/x-javascript", "text/x-typescript",
				"application/pdf",
			},
		}
	case ModelGemini25FlashLite:
		return ModelCapabilities{
			SupportsStreaming:   true,
			SupportsToolCalling: true,
			SupportsVision:      false,
			SupportsAudio:       false,
			SupportsThinking:    false, // Lite model doesn't support thinking
			MaxInputTokens:      32768,
			MaxOutputTokens:     8192,
			MaxThinkingTokens:   nil,
			SupportedMimeTypes: []string{
				"text/plain",
			},
		}
	case ModelGemini20Flash:
		return ModelCapabilities{
			SupportsStreaming:   true,
			SupportsToolCalling: true,
			SupportsVision:      true,
			SupportsAudio:       false,
			SupportsThinking:    false,   // 2.0 and 1.5 models don't support thinking
			MaxInputTokens:      1048576, // 1M tokens
			MaxOutputTokens:     8192,
			MaxThinkingTokens:   nil,
			SupportedMimeTypes: []string{
				"image/png", "image/jpeg", "image/webp", "image/heic", "image/heif",
				"video/mp4", "video/mpeg", "video/mov", "video/avi", "video/flv", "video/mpv", "video/webm", "video/wmv", "video/3gpp",
				"text/plain",
			},
		}
	case ModelGemini20FlashLite:
		return ModelCapabilities{
			SupportsStreaming:   true,
			SupportsToolCalling: true,
			SupportsVision:      false,
			SupportsAudio:       false,
			MaxInputTokens:      32768,
			MaxOutputTokens:     8192,
			MaxThinkingTokens:   nil,
			SupportedMimeTypes: []string{
				"text/plain",
			},
		}
	case ModelGemini15Pro:
		return ModelCapabilities{
			SupportsStreaming:   true,
			SupportsToolCalling: true,
			SupportsVision:      true,
			SupportsAudio:       false,
			SupportsThinking:    false,   // 2.0 and 1.5 models don't support thinking
			MaxInputTokens:      2097152, // 2M tokens
			MaxOutputTokens:     8192,
			MaxThinkingTokens:   nil,
			SupportedMimeTypes: []string{
				"image/png", "image/jpeg", "image/webp", "image/heic", "image/heif",
				"video/mp4", "video/mpeg", "video/mov", "video/avi", "video/flv", "video/mpv", "video/webm", "video/wmv", "video/3gpp",
				"text/plain", "text/html", "text/css", "text/javascript", "application/x-javascript", "text/x-typescript",
				"application/pdf",
			},
		}
	case ModelGemini15Flash:
		return ModelCapabilities{
			SupportsStreaming:   true,
			SupportsToolCalling: true,
			SupportsVision:      true,
			SupportsAudio:       false,
			SupportsThinking:    false,   // 2.0 and 1.5 models don't support thinking
			MaxInputTokens:      1048576, // 1M tokens
			MaxOutputTokens:     8192,
			MaxThinkingTokens:   nil,
			SupportedMimeTypes: []string{
				"image/png", "image/jpeg", "image/webp", "image/heic", "image/heif",
				"video/mp4", "video/mpeg", "video/mov", "video/avi", "video/flv", "video/mpv", "video/webm", "video/wmv", "video/3gpp",
				"text/plain", "text/html", "text/css", "text/javascript", "application/x-javascript", "text/x-typescript",
				"application/pdf",
			},
		}
	case ModelGemini15Flash8B:
		return ModelCapabilities{
			SupportsStreaming:   true,
			SupportsToolCalling: true,
			SupportsVision:      true,
			SupportsAudio:       false,
			SupportsThinking:    false,   // 2.0 and 1.5 models don't support thinking
			MaxInputTokens:      1048576, // 1M tokens
			MaxOutputTokens:     8192,
			MaxThinkingTokens:   nil,
			SupportedMimeTypes: []string{
				"image/png", "image/jpeg", "image/webp", "image/heic", "image/heif",
				"video/mp4", "video/mpeg", "video/mov", "video/avi", "video/flv", "video/mpv", "video/webm", "video/wmv", "video/3gpp",
				"text/plain",
			},
		}
	// Preview/Experimental models
	case ModelGeminiLive25FlashPreview, ModelGemini25FlashPreviewNativeAudio, ModelGemini25FlashExpNativeAudioThinking,
		ModelGemini25FlashPreviewTTS, ModelGemini25ProPreviewTTS, ModelGemini20FlashLive001:
		return ModelCapabilities{
			SupportsStreaming:   true,
			SupportsToolCalling: true,
			SupportsVision:      true,
			SupportsAudio:       true,
			MaxInputTokens:      1048576, // 1M tokens
			MaxOutputTokens:     8192,
			MaxThinkingTokens:   nil,
			SupportedMimeTypes: []string{
				"image/png", "image/jpeg", "image/webp", "image/heic", "image/heif",
				"audio/wav", "audio/mp3", "audio/aiff", "audio/aac", "audio/ogg", "audio/flac",
				"video/mp4", "video/mpeg", "video/mov", "video/avi", "video/flv", "video/mpv", "video/webm", "video/wmv", "video/3gpp",
				"text/plain",
			},
		}
	case ModelGemini20FlashPreviewImageGen:
		return ModelCapabilities{
			SupportsStreaming:        true,
			SupportsToolCalling:      true,
			SupportsVision:           true,
			SupportsAudio:            false,
			SupportsThinking:         false,   // 2.0 and 1.5 models don't support thinking
			SupportsImageGeneration:  true,    // Can generate images
			MaxInputTokens:           1048576, // 1M tokens
			MaxOutputTokens:          8192,
			MaxThinkingTokens:        nil,
			SupportedMimeTypes: []string{
				"image/png", "image/jpeg", "image/webp", "image/heic", "image/heif",
				"text/plain",
			},
			SupportedOutputFormats: []string{"png", "jpeg"},
		}
	case ModelGemini25FlashImage:
		return ModelCapabilities{
			SupportsStreaming:        true,
			SupportsToolCalling:      false,   // Image gen models typically don't support tools
			SupportsVision:           true,    // Can accept images as input for image-to-image
			SupportsAudio:            false,
			SupportsThinking:         false,
			SupportsImageGeneration:  true,    // Primary purpose: generate images
			MaxInputTokens:           32768,
			MaxOutputTokens:          8192,
			MaxThinkingTokens:        nil,
			SupportedMimeTypes: []string{
				"image/png", "image/jpeg", "image/webp",
				"text/plain",
			},
			SupportedOutputFormats: []string{"png", "jpeg"},
		}
	default:
		// Return default capabilities for unknown models
		return ModelCapabilities{
			SupportsStreaming:   true,
			SupportsToolCalling: true,
			SupportsVision:      false,
			SupportsAudio:       false,
			SupportsThinking:    false,
			MaxInputTokens:      32768,
			MaxOutputTokens:     2048,
			MaxThinkingTokens:   nil,
			SupportedMimeTypes: []string{
				"text/plain",
			},
		}
	}
}

// IsVisionModel returns true if the model supports vision capabilities
func IsVisionModel(model string) bool {
	capabilities := GetModelCapabilities(model)
	return capabilities.SupportsVision
}

// IsAudioModel returns true if the model supports audio capabilities
func IsAudioModel(model string) bool {
	capabilities := GetModelCapabilities(model)
	return capabilities.SupportsAudio
}

// SupportsToolCalling returns true if the model supports function/tool calling
func SupportsToolCalling(model string) bool {
	capabilities := GetModelCapabilities(model)
	return capabilities.SupportsToolCalling
}

// SupportsThinking returns true if the model supports thinking capabilities
func SupportsThinking(model string) bool {
	capabilities := GetModelCapabilities(model)
	return capabilities.SupportsThinking
}

// SupportsImageGeneration returns true if the model supports image generation
func SupportsImageGeneration(model string) bool {
	capabilities := GetModelCapabilities(model)
	return capabilities.SupportsImageGeneration
}

// GetSupportedOutputFormats returns the supported output formats for image generation
func GetSupportedOutputFormats(model string) []string {
	capabilities := GetModelCapabilities(model)
	return capabilities.SupportedOutputFormats
}

// GetMaxThinkingTokens returns the maximum thinking tokens for a model
func GetMaxThinkingTokens(model string) *int32 {
	capabilities := GetModelCapabilities(model)
	return capabilities.MaxThinkingTokens
}

// ValidateThinkingBudget validates if a thinking budget is within model limits
func ValidateThinkingBudget(model string, budget int32) error {
	if !SupportsThinking(model) {
		return fmt.Errorf("model %s does not support thinking", model)
	}

	maxTokens := GetMaxThinkingTokens(model)
	if maxTokens != nil && budget > *maxTokens {
		return fmt.Errorf("thinking budget %d exceeds maximum %d for model %s", budget, *maxTokens, model)
	}

	return nil
}
