package azureopenai

import (
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/openai/openai-go/v2"
)

// applyMaxTokens sets the output token cap on an OpenAI-compatible request.
//
// Uses max_completion_tokens rather than max_tokens: reasoning models reject
// the older field outright, while the newer one is accepted across current chat
// models, so one code path covers both.
//
// A zero or absent MaxTokens leaves the field unset so the model's own default
// applies. That is exactly what every caller got before #347, which is what
// makes adding this non-breaking.
func applyMaxTokens(req *openai.ChatCompletionNewParams, config *interfaces.LLMConfig) {
	if req == nil || config == nil || config.MaxTokens <= 0 {
		return
	}
	req.MaxCompletionTokens = openai.Int(int64(config.MaxTokens))
}

// applyMaxTokensValue is the resolver for the older llm.GenerateParams surface,
// which carries an int rather than an *interfaces.LLMConfig.
func applyMaxTokensValue(req *openai.ChatCompletionNewParams, maxTokens int) {
	if req == nil || maxTokens <= 0 {
		return
	}
	req.MaxCompletionTokens = openai.Int(int64(maxTokens))
}
