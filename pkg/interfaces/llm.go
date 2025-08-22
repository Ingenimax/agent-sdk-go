package interfaces

import "context"

// LLM represents a large language model provider
type LLM interface {
	// Generate generates text based on the provided prompt
	Generate(ctx context.Context, prompt string, options ...GenerateOption) (string, error)

	// GenerateWithTools generates text and can use tools
	GenerateWithTools(ctx context.Context, prompt string, tools []Tool, options ...GenerateOption) (string, error)

	// Name returns the name of the LLM provider
	Name() string
}

// GenerateOption represents options for text generation
type GenerateOption func(options *GenerateOptions)

// GenerateOptions contains configuration for text generation
type GenerateOptions struct {
	LLMConfig      *LLMConfig      // LLM config for the generation
	OrgID          string          // For multi-tenancy
	SystemMessage  string          // System message for chat models
	ResponseFormat *ResponseFormat // Optional expected response format
	MaxIterations  int             // Maximum number of tool-calling iterations (0 = use default)
	Memory         Memory          // Optional memory for storing tool calls and results
}

type LLMConfig struct {
	Temperature      float64  // Temperature for the generation
	TopP             float64  // Top P for the generation
	FrequencyPenalty float64  // Frequency penalty for the generation
	PresencePenalty  float64  // Presence penalty for the generation
	StopSequences    []string // Stop sequences for the generation
	Reasoning        string   // Reasoning mode (none, minimal, comprehensive) to control explanation detail
}

// WithMaxIterations creates a GenerateOption to set the maximum number of tool-calling iterations
func WithMaxIterations(maxIterations int) GenerateOption {
	return func(options *GenerateOptions) {
		options.MaxIterations = maxIterations
	}
}

// WithMemory creates a GenerateOption to set the memory for storing tool calls and results
func WithMemory(memory Memory) GenerateOption {
	return func(options *GenerateOptions) {
		options.Memory = memory
	}
}

// WithSystemMessage creates a GenerateOption to set the system message
func WithSystemMessage(systemMessage string) GenerateOption {
	return func(options *GenerateOptions) {
		options.SystemMessage = systemMessage
	}
}

// WithTemperature creates a GenerateOption to set the temperature
func WithTemperature(temperature float64) GenerateOption {
	return func(options *GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &LLMConfig{}
		}
		options.LLMConfig.Temperature = temperature
	}
}

// WithTopP creates a GenerateOption to set the top_p
func WithTopP(topP float64) GenerateOption {
	return func(options *GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &LLMConfig{}
		}
		options.LLMConfig.TopP = topP
	}
}

// WithFrequencyPenalty creates a GenerateOption to set the frequency penalty
func WithFrequencyPenalty(frequencyPenalty float64) GenerateOption {
	return func(options *GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &LLMConfig{}
		}
		options.LLMConfig.FrequencyPenalty = frequencyPenalty
	}
}

// WithPresencePenalty creates a GenerateOption to set the presence penalty
func WithPresencePenalty(presencePenalty float64) GenerateOption {
	return func(options *GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &LLMConfig{}
		}
		options.LLMConfig.PresencePenalty = presencePenalty
	}
}

// WithStopSequences creates a GenerateOption to set the stop sequences
func WithStopSequences(stopSequences []string) GenerateOption {
	return func(options *GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &LLMConfig{}
		}
		options.LLMConfig.StopSequences = stopSequences
	}
}

// WithReasoning creates a GenerateOption to set the reasoning mode
// reasoning can be "none" (direct answers), "minimal" (brief explanations),
// or "comprehensive" (detailed step-by-step reasoning)
func WithReasoning(reasoning string) GenerateOption {
	return func(options *GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &LLMConfig{}
		}
		options.LLMConfig.Reasoning = reasoning
	}
}

// WithResponseFormat creates a GenerateOption to set the response format
func WithResponseFormat(format ResponseFormat) GenerateOption {
	return func(options *GenerateOptions) {
		options.ResponseFormat = &format
	}
}
