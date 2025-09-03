package gemini

import (
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// WithTemperature creates a GenerateOption to set the temperature
func WithTemperature(temperature float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.Temperature = temperature
	}
}

// WithTopP creates a GenerateOption to set the top_p
func WithTopP(topP float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.TopP = topP
	}
}

// WithStopSequences creates a GenerateOption to set the stop sequences
func WithStopSequences(stopSequences []string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.StopSequences = stopSequences
	}
}

// WithSystemMessage creates a GenerateOption to set the system message
func WithSystemMessage(systemMessage string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.SystemMessage = systemMessage
	}
}

// WithResponseFormat creates a GenerateOption to set the response format
func WithResponseFormat(format interfaces.ResponseFormat) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.ResponseFormat = &format
	}
}

// WithReasoning creates a GenerateOption to set the reasoning mode
// reasoning can be "none" (direct answers), "minimal" (brief explanations),
// or "comprehensive" (detailed step-by-step reasoning)
func WithReasoning(reasoning string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.Reasoning = reasoning
	}
}

// Thinking-related client options (for configuring the GeminiClient)

// WithThinking creates a client Option to enable/disable thinking
func WithThinking(enabled bool) Option {
	return func(c *GeminiClient) {
		if c.thinkingConfig == nil {
			defaultConfig := DefaultThinkingConfig()
			c.thinkingConfig = &defaultConfig
		}
		c.thinkingConfig.IncludeThoughts = enabled
	}
}

// WithThinkingBudget creates a client Option to set thinking token budget
func WithThinkingBudget(budget int32) Option {
	return func(c *GeminiClient) {
		if c.thinkingConfig == nil {
			defaultConfig := DefaultThinkingConfig()
			c.thinkingConfig = &defaultConfig
		}
		c.thinkingConfig.ThinkingBudget = &budget
	}
}

// WithDynamicThinking creates a client Option to enable dynamic thinking (no fixed budget)
func WithDynamicThinking() Option {
	return func(c *GeminiClient) {
		if c.thinkingConfig == nil {
			defaultConfig := DefaultThinkingConfig()
			c.thinkingConfig = &defaultConfig
		}
		c.thinkingConfig.ThinkingBudget = nil // nil means dynamic
		c.thinkingConfig.IncludeThoughts = true
	}
}

// WithThoughtSignatures creates a client Option to set thought signatures for multi-turn context
func WithThoughtSignatures(signatures [][]byte) Option {
	return func(c *GeminiClient) {
		if c.thinkingConfig == nil {
			defaultConfig := DefaultThinkingConfig()
			c.thinkingConfig = &defaultConfig
		}
		c.thinkingConfig.ThoughtSignatures = signatures
	}
}

// WithThinkingConfig creates a client Option to set complete thinking configuration
func WithThinkingConfig(config ThinkingConfig) Option {
	return func(c *GeminiClient) {
		c.thinkingConfig = &config
	}
}
