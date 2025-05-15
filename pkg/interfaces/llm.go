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
}

type LLMConfig struct {
	Temperature      float64  // Temperature for the generation
	TopP             float64  // Top P for the generation
	FrequencyPenalty float64  // Frequency penalty for the generation
	PresencePenalty  float64  // Presence penalty for the generation
	StopSequences    []string // Stop sequences for the generation
	Reasoning        string   // Reasoning mode (none, minimal, comprehensive) to control explanation detail
}
