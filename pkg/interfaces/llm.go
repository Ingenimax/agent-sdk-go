package interfaces

import "context"

// LLM represents a large language model provider
type LLM interface {
	// Generate generates text based on the provided prompt
	Generate(ctx context.Context, prompt string, options ...GenerateOption) (string, error)

	// GenerateWithTools generates text and can use tools
	GenerateWithTools(ctx context.Context, prompt string, tools []Tool, options ...GenerateOption) (string, error)

	// GenerateDetailed generates text and returns detailed response information including token usage
	GenerateDetailed(ctx context.Context, prompt string, options ...GenerateOption) (*LLMResponse, error)

	// GenerateWithToolsDetailed generates text with tools and returns detailed response information including token usage
	GenerateWithToolsDetailed(ctx context.Context, prompt string, tools []Tool, options ...GenerateOption) (*LLMResponse, error)

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
	Reasoning        string   // Reasoning mode (minimal, low, medium, high) to control reasoning effort
	EnableReasoning  bool     // Enable native reasoning tokens (Anthropic thinking/OpenAI o1)
	ReasoningBudget  int      // Optional token budget for reasoning (Anthropic only), minimum 1024
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
		options.LLMConfig.EnableReasoning = enabled
		if len(budget) > 0 {
			options.LLMConfig.ReasoningBudget = budget[0]
		}
	}
}

// LLMResponse represents the detailed response from an LLM generation request
type LLMResponse struct {
	// Content is the generated text response
	Content string

	// Usage contains token usage information (nil if not available)
	Usage *TokenUsage

	// Model indicates which model was used for generation
	Model string

	// StopReason indicates why the generation stopped (optional)
	StopReason string

	// Metadata contains provider-specific additional information
	Metadata map[string]interface{}
}

// TokenUsage represents token usage information for an LLM request
type TokenUsage struct {
	// InputTokens is the number of tokens in the input/prompt
	InputTokens int

	// OutputTokens is the number of tokens in the generated response
	OutputTokens int

	// TotalTokens is the total number of tokens used (input + output)
	TotalTokens int

	// ReasoningTokens is the number of tokens used for reasoning (optional, for models that support it)
	ReasoningTokens int
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

// WithResponseFormat creates a GenerateOption to set the response format
func WithResponseFormat(format ResponseFormat) GenerateOption {
	return func(options *GenerateOptions) {
		options.ResponseFormat = &format
	}
}

// CacheConfig contains configuration for prompt caching (Anthropic)
type CacheConfig struct {
	// CacheSystemMessage enables caching of the system message
	CacheSystemMessage bool

	// CacheTools enables caching of tool definitions
	CacheTools bool

	// CacheConversation enables caching of conversation messages
	CacheConversation bool

	// CacheTTL specifies the cache time-to-live (e.g., "5m", "1h")
	// If empty, uses provider default
	CacheTTL string
}
