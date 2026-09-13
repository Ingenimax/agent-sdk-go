package agent

import (
	"context"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

type contextKey string

const usageTrackerKey contextKey = "usageTracker"

type usageTracker struct {
	totalUsage   *interfaces.TokenUsage
	execSummary  *interfaces.ExecutionSummary
	detailed     bool
	primaryModel string
	usageByModel map[string]*interfaces.TokenUsage
	mu           sync.Mutex
}

func newUsageTracker(detailed bool) *usageTracker {
	return &usageTracker{
		totalUsage: &interfaces.TokenUsage{},
		execSummary: &interfaces.ExecutionSummary{
			UsedTools:     []string{},
			UsedSubAgents: []string{},
			UsageByModel:  map[string]interfaces.TokenUsage{},
		},
		detailed:     detailed,
		usageByModel: map[string]*interfaces.TokenUsage{},
	}
}

func (ut *usageTracker) addLLMUsage(usage *interfaces.TokenUsage, model string) {
	if !ut.detailed || usage == nil {
		return
	}

	ut.mu.Lock()
	defer ut.mu.Unlock()

	ut.totalUsage.InputTokens += usage.InputTokens
	ut.totalUsage.OutputTokens += usage.OutputTokens
	ut.totalUsage.TotalTokens += usage.TotalTokens
	ut.totalUsage.ReasoningTokens += usage.ReasoningTokens
	ut.totalUsage.CacheCreationInputTokens += usage.CacheCreationInputTokens
	ut.totalUsage.CacheReadInputTokens += usage.CacheReadInputTokens
	ut.execSummary.LLMCalls++

	if ut.primaryModel == "" && model != "" {
		ut.primaryModel = model
	}

	if model == "" {
		model = "unknown"
	}

	modelUsage := ut.usageByModel[model]
	if modelUsage == nil {
		modelUsage = &interfaces.TokenUsage{}
		ut.usageByModel[model] = modelUsage
	}
	modelUsage.InputTokens += usage.InputTokens
	modelUsage.OutputTokens += usage.OutputTokens
	modelUsage.TotalTokens += usage.TotalTokens
	modelUsage.ReasoningTokens += usage.ReasoningTokens
	modelUsage.CacheCreationInputTokens += usage.CacheCreationInputTokens
	modelUsage.CacheReadInputTokens += usage.CacheReadInputTokens
}

func (ut *usageTracker) addToolCall(toolName string) {
	if !ut.detailed {
		return
	}

	ut.mu.Lock()
	defer ut.mu.Unlock()

	// ToolCalls counts invocations, so it is incremented for every call. The
	// early return used to sit above this line, inside the dedup loop, which
	// meant ToolCalls only advanced the first time a given tool was seen and
	// was therefore always exactly len(UsedTools) -- forty calls to one tool
	// reported 1.
	ut.execSummary.ToolCalls++

	// UsedTools is a distinct set, so it is appended to only once per tool.
	for _, used := range ut.execSummary.UsedTools {
		if used == toolName {
			return
		}
	}

	ut.execSummary.UsedTools = append(ut.execSummary.UsedTools, toolName)
}

func (ut *usageTracker) setExecutionTime(timeMs int64) {
	if !ut.detailed {
		return
	}

	ut.mu.Lock()
	defer ut.mu.Unlock()

	ut.execSummary.ExecutionTimeMs = timeMs
}

func (ut *usageTracker) getResults() (*interfaces.TokenUsage, *interfaces.ExecutionSummary, string) {
	if !ut.detailed {
		return nil, nil, ""
	}

	ut.mu.Lock()
	defer ut.mu.Unlock()

	ut.execSummary.UsageByModel = make(map[string]interfaces.TokenUsage, len(ut.usageByModel))
	for model, usage := range ut.usageByModel {
		ut.execSummary.UsageByModel[model] = *usage
	}

	return ut.totalUsage, ut.execSummary, ut.primaryModel
}

func withUsageTracker(ctx context.Context, tracker *usageTracker) context.Context {
	return context.WithValue(ctx, usageTrackerKey, tracker)
}

func getUsageTracker(ctx context.Context) *usageTracker {
	tracker, _ := ctx.Value(usageTrackerKey).(*usageTracker)
	return tracker
}
