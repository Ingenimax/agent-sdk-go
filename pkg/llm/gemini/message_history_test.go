package gemini

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestMessageHistoryBuilder_BuildContents(t *testing.T) {
	logger := logging.New()
	builder := newMessageHistoryBuilder(logger)

	tests := []struct {
		name     string
		prompt   string
		params   *interfaces.GenerateOptions
		expected int
	}{
		{
			name:     "no memory",
			prompt:   "Hello",
			params:   &interfaces.GenerateOptions{},
			expected: 1,
		},
		{
			name:   "no memory with content parts",
			prompt: "Hello",
			params: &interfaces.GenerateOptions{
				ContentParts: []interfaces.ContentPart{
					interfaces.TextPart("What is in this image?"),
					interfaces.ImageURLPart("data:image/png;base64,AA==", "auto"),
				},
			},
			expected: 1,
		},
		{
			name:   "with system message",
			prompt: "Hello",
			params: &interfaces.GenerateOptions{
				SystemMessage: "You are helpful",
			},
			expected: 1,
		},
		{
			name:   "with memory",
			prompt: "Continue",
			params: &interfaces.GenerateOptions{
				Memory: &mockMemory{
					messages: []interfaces.Message{
						{Role: interfaces.MessageRoleUser, Content: "Hi"},
						{Role: interfaces.MessageRoleAssistant, Content: "Hello!"},
						{Role: interfaces.MessageRoleUser, Content: "Continue"}, // Agent adds current prompt to memory by default
					},
				},
			},
			expected: 3, // 2 from memory + 1 current prompt
		},
		{
			name:   "with memory including system",
			prompt: "Continue",
			params: &interfaces.GenerateOptions{
				Memory: &mockMemory{
					messages: []interfaces.Message{
						{Role: interfaces.MessageRoleSystem, Content: "Old system"},
						{Role: interfaces.MessageRoleUser, Content: "Hi"},
						{Role: interfaces.MessageRoleAssistant, Content: "Hello!"},
						{Role: interfaces.MessageRoleUser, Content: "Continue"}, // Agent adds current prompt to memory by default
					},
				},
			},
			expected: 4, // 3 from memory + 1 current prompt
		},
		{
			name:   "with tool calls and results",
			prompt: "What's next?",
			params: &interfaces.GenerateOptions{
				Memory: &mockMemory{
					messages: []interfaces.Message{
						{Role: interfaces.MessageRoleUser, Content: "Get weather"},
						{Role: interfaces.MessageRoleAssistant, Content: "I'll check the weather", ToolCalls: []interfaces.ToolCall{
							{ID: "call_123", Name: "get_weather", Arguments: `{"location": "NYC"}`},
						}},
						{Role: interfaces.MessageRoleTool, Content: "Sunny, 72°F", ToolCallID: "call_123", Metadata: map[string]interface{}{
							"tool_name": "get_weather",
						}},
						{Role: interfaces.MessageRoleUser, Content: "What's next?"}, // Agent adds current prompt to memory by default
					},
				},
			},
			expected: 4, // 3 from memory + 1 current prompt
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contents := builder.buildContents(context.Background(), tt.prompt, tt.params)
			if len(contents) != tt.expected {
				t.Errorf("Expected %d contents, got %d", tt.expected, len(contents))
			}
		})
	}
}
