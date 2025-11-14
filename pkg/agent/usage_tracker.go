package agent

import (
	"context"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

type contextKey string

const usageTrackerKey contextKey = "usageTracker"

type usageTracker struct {
	totalUsage  *interfaces.TokenUsage
	execSummary *interfaces.ExecutionSummary
	detailed    bool
	primaryModel string
	mu          sync.Mutex
}

func newUsageTracker(detailed bool) *usageTracker {
	return &usageTracker{
		totalUsage: &interfaces.TokenUsage{},
		execSummary: &interfaces.ExecutionSummary{
			UsedTools:     []string{},
			UsedSubAgents: []string{},
		},
		detailed: detailed,
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
	ut.execSummary.LLMCalls++

	if ut.primaryModel == "" && model != "" {
		ut.primaryModel = model
	}
}

func (ut *usageTracker) addToolCall(toolName string) {
	if !ut.detailed {
		return
	}

	ut.mu.Lock()
	defer ut.mu.Unlock()

	for _, used := range ut.execSummary.UsedTools {
		if used == toolName {
			return
		}
	}

	ut.execSummary.UsedTools = append(ut.execSummary.UsedTools, toolName)
	ut.execSummary.ToolCalls++
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

	return ut.totalUsage, ut.execSummary, ut.primaryModel
}

func withUsageTracker(ctx context.Context, tracker *usageTracker) context.Context {
	return context.WithValue(ctx, usageTrackerKey, tracker)
}

func getUsageTracker(ctx context.Context) *usageTracker {
	tracker, _ := ctx.Value(usageTrackerKey).(*usageTracker)
	return tracker
}