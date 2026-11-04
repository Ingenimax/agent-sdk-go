package anthropic

import (
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// TestResolveMaxTokens pins the two properties that make #347 safe to land:
// an unset MaxTokens reproduces the old behaviour exactly, and an explicit one
// is still floored above the reasoning budget so the request stays valid.
func TestResolveMaxTokens(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config *interfaces.LLMConfig
		want   int
	}{
		{"nil config keeps the old default", nil, 2048},
		{"empty config keeps the old default", &interfaces.LLMConfig{}, 2048},
		{"explicit value is honoured", &interfaces.LLMConfig{MaxTokens: 8192}, 8192},
		{
			name:   "reasoning budget drives the default, as before",
			config: &interfaces.LLMConfig{EnableReasoning: true, ReasoningBudget: 10000},
			want:   14000,
		},
		{
			name:   "explicit value above the reasoning floor wins",
			config: &interfaces.LLMConfig{EnableReasoning: true, ReasoningBudget: 2000, MaxTokens: 30000},
			want:   30000,
		},
		{
			// Anthropic rejects max_tokens <= budget_tokens outright, so
			// honouring 1024 here would produce a guaranteed API error.
			name:   "explicit value below the reasoning floor is raised",
			config: &interfaces.LLMConfig{EnableReasoning: true, ReasoningBudget: 10000, MaxTokens: 1024},
			want:   14000,
		},
		{
			name:   "reasoning enabled without a budget uses the plain default",
			config: &interfaces.LLMConfig{EnableReasoning: true},
			want:   2048,
		},
		{
			name:   "budget without reasoning enabled does not raise the floor",
			config: &interfaces.LLMConfig{ReasoningBudget: 10000, MaxTokens: 512},
			want:   512,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveMaxTokens(tc.config); got != tc.want {
				t.Errorf("resolveMaxTokens = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMaxTokensOrDefault(t *testing.T) {
	if got := maxTokensOrDefault(0); got != defaultMaxTokens {
		t.Errorf("maxTokensOrDefault(0) = %d, want %d", got, defaultMaxTokens)
	}
	if got := maxTokensOrDefault(-5); got != defaultMaxTokens {
		t.Errorf("maxTokensOrDefault(-5) = %d, want %d", got, defaultMaxTokens)
	}
	if got := maxTokensOrDefault(777); got != 777 {
		t.Errorf("maxTokensOrDefault(777) = %d, want 777", got)
	}
}
