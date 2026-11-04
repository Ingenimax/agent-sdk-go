package anthropic

import "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"

const (
	// defaultMaxTokens is Anthropic's long-standing default in this client.
	// Anthropic requires max_tokens on every request, so there has to be one.
	defaultMaxTokens = 2048

	// reasoningResponseBuffer is headroom added above the reasoning budget for
	// the visible answer. Anthropic rejects a request whose max_tokens does not
	// exceed budget_tokens, so the budget alone is not a usable limit.
	reasoningResponseBuffer = 4000
)

// resolveMaxTokens picks the output token limit for a request.
//
// Order matters. An explicit MaxTokens wins over the default, but is still
// floored above the reasoning budget: a caller who sets 1024 alongside a 8192
// reasoning budget would otherwise produce a request Anthropic rejects
// outright, which is a worse outcome than quietly honouring the floor.
//
// Zero MaxTokens reproduces the previous behaviour exactly, which is what keeps
// this change non-breaking for every existing caller (#347).
func resolveMaxTokens(config *interfaces.LLMConfig) int {
	floor := 0
	if config != nil && config.EnableReasoning && config.ReasoningBudget > 0 {
		floor = config.ReasoningBudget + reasoningResponseBuffer
	}

	maxTokens := defaultMaxTokens
	if floor > 0 {
		maxTokens = floor
	}
	if config != nil && config.MaxTokens > 0 {
		maxTokens = config.MaxTokens
	}
	if maxTokens < floor {
		maxTokens = floor
	}
	return maxTokens
}

// maxTokensOrDefault is the resolver for the older llm.GenerateParams surface,
// which carries no reasoning fields.
func maxTokensOrDefault(maxTokens int) int {
	if maxTokens > 0 {
		return maxTokens
	}
	return defaultMaxTokens
}
