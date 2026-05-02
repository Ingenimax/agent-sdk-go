package openai

import (
	"context"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
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
