package openai

import (
	"context"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/openai/openai-go/v2"
)

// usageAccumulator collects token usage across the multiple API calls a
// single GenerateWithTools invocation makes (one per tool-loop iteration
// plus the final summary). Lets GenerateWithToolsDetailed return a
// total that reflects every underlying chat completion, not just the
// last one (#276).
type usageAccumulator struct {
	mu      sync.Mutex
	total   interfaces.TokenUsage
	model   string
	touched bool
}

func (u *usageAccumulator) add(input, output, total, reasoning int, model string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.total.InputTokens += input
	u.total.OutputTokens += output
	u.total.TotalTokens += total
	u.total.ReasoningTokens += reasoning
	if u.model == "" {
		u.model = model
	}
	u.touched = true
}

func (u *usageAccumulator) snapshot() (*interfaces.TokenUsage, string, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.touched {
		return nil, "", false
	}
	t := u.total
	return &t, u.model, true
}

type usageCtxKey struct{}

func withUsageAccumulator(ctx context.Context, acc *usageAccumulator) context.Context {
	return context.WithValue(ctx, usageCtxKey{}, acc)
}

func getUsageAccumulator(ctx context.Context) *usageAccumulator {
	acc, _ := ctx.Value(usageCtxKey{}).(*usageAccumulator)
	return acc
}

// recordStreamUsage pushes a stream chunk's usage into acc and, when present,
// the context-scoped accumulator.
//
// OpenAI reports usage on a final chunk that carries no choices, and only when
// stream_options.include_usage is set on the request. Both call sites in
// GenerateWithToolsStream set it unconditionally for that reason (#327).
func (c *OpenAIClient) recordStreamUsage(ctx context.Context, acc *usageAccumulator, usage openai.CompletionUsage) {
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		return
	}
	input := int(usage.PromptTokens)
	output := int(usage.CompletionTokens)
	total := int(usage.TotalTokens)
	reasoning := int(usage.CompletionTokensDetails.ReasoningTokens)

	acc.add(input, output, total, reasoning, c.Model)
	if ctxAcc := getUsageAccumulator(ctx); ctxAcc != nil {
		ctxAcc.add(input, output, total, reasoning, c.Model)
	}
}

// messageStopWithUsage builds the terminal stream event, carrying aggregated
// token usage when any was observed.
//
// GenerateWithToolsStream returns only an event channel, so metadata on this
// event is the single channel through which the agent's usage tracker can learn
// what the tool loop actually spent.
func messageStopWithUsage(acc *usageAccumulator, fallbackModel string) interfaces.StreamEvent {
	event := interfaces.StreamEvent{
		Type:      interfaces.StreamEventMessageStop,
		Timestamp: time.Now(),
	}
	usage, model, ok := acc.snapshot()
	if !ok {
		return event
	}
	if model == "" {
		model = fallbackModel
	}
	event.Metadata = map[string]interface{}{
		interfaces.MetadataKeyUsage: usage,
		interfaces.MetadataKeyModel: model,
	}
	return event
}
