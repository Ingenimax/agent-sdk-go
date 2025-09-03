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

	// SupportsStreaming returns true if this LLM supports streaming
	SupportsStreaming() bool
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
	StreamConfig   *StreamConfig   // Optional streaming configuration
}

type LLMConfig struct {
	Temperature      float64  // Temperature for the generation
	TopP             float64  // Top P for the generation
	FrequencyPenalty float64  // Frequency penalty for the generation
	PresencePenalty  float64  // Presence penalty for the generation
	StopSequences    []string // Stop sequences for the generation
	Reasoning        string   // Reasoning mode (none, minimal, comprehensive) to control explanation detail
	EnableReasoning  bool     // Enable native reasoning tokens (Anthropic thinking/OpenAI o1)
	ReasoningBudget  int      // Optional token budget for reasoning (Anthropic only)
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

// WithStreamConfig creates a GenerateOption to set the streaming configuration
func WithStreamConfig(config StreamConfig) GenerateOption {
	return func(options *GenerateOptions) {
		options.StreamConfig = &config
	}
}

// WithReasoning creates a GenerateOption to enable native reasoning tokens
func WithReasoning(enabled bool, budget ...int) GenerateOption {
	return func(options *GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &LLMConfig{}
		}
		options.LLMConfig.EnableReasoning = enabled
		if len(budget) > 0 {
			options.LLMConfig.ReasoningBudget = budget[0]
		}
	}
}
