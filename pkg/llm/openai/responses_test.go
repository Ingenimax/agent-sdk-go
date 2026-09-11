package openai

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestShouldUseResponsesAPI(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		reasoning string
		toolCount int
		want      bool
	}{
		{"reasoning model with reasoning and tools", "gpt-5-mini", "low", 2, true},
		{"reasoning model no reasoning effort", "gpt-5-mini", "", 2, false},
		{"reasoning model no tools", "gpt-5-mini", "high", 0, false},
		{"non-reasoning model with tools", "gpt-4o-mini", "low", 2, false},
		{"o3 reasoning with tools", "o3-mini", "medium", 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUseResponsesAPI(tt.model, tt.reasoning, tt.toolCount); got != tt.want {
				t.Errorf("shouldUseResponsesAPI(%q,%q,%d) = %v, want %v", tt.model, tt.reasoning, tt.toolCount, got, tt.want)
			}
		})
	}
}

func TestBuildResponseInput(t *testing.T) {
	c := &OpenAIClient{logger: logging.New()}

	tests := []struct {
		name     string
		prompt   string
		memory   interfaces.Memory
		expected int
	}{
		{
			name:     "no memory yields single user item",
			prompt:   "Hello",
			memory:   nil,
			expected: 1,
		},
		{
			name:   "user and assistant text preserved",
			prompt: "Continue",
			memory: &mockMemory{messages: []interfaces.Message{
				{Role: interfaces.MessageRoleUser, Content: "Hi"},
				{Role: interfaces.MessageRoleAssistant, Content: "Hello!"},
			}},
			expected: 2,
		},
		{
			name:   "system message preserved",
			prompt: "Continue",
			memory: &mockMemory{messages: []interfaces.Message{
				{Role: interfaces.MessageRoleSystem, Content: "System"},
				{Role: interfaces.MessageRoleUser, Content: "Hi"},
				{Role: interfaces.MessageRoleAssistant, Content: "Hello!"},
			}},
			expected: 3,
		},
		{
			name:   "tool role dropped, assistant text kept",
			prompt: "What's next?",
			memory: &mockMemory{messages: []interfaces.Message{
				{Role: interfaces.MessageRoleUser, Content: "Get weather"},
				{Role: interfaces.MessageRoleAssistant, Content: "I'll check", ToolCalls: []interfaces.ToolCall{
					{ID: "call_1", Name: "get_weather", Arguments: `{}`},
				}},
				{Role: interfaces.MessageRoleTool, Content: "Sunny", ToolCallID: "call_1"},
			}},
			expected: 2,
		},
		{
			name:   "empty assistant content dropped",
			prompt: "x",
			memory: &mockMemory{messages: []interfaces.Message{
				{Role: interfaces.MessageRoleUser, Content: "Hi"},
				{Role: interfaces.MessageRoleAssistant, Content: ""},
			}},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := c.buildResponseInput(context.Background(), tt.prompt, tt.memory)
			if len(items) != tt.expected {
				t.Errorf("expected %d input items, got %d", tt.expected, len(items))
			}
		})
	}
}
