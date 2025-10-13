package anthropic

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestMessageHistoryBuilder_BuildMessages(t *testing.T) {
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
						{Role: interfaces.MessageRoleTool, Content: "Sunny, 72°F", ToolCallID: "call_123"},
						{Role: interfaces.MessageRoleUser, Content: "What's next?"}, // Agent adds current prompt to memory by default
					},
				},
			},
			expected: 4, // 3 from memory + 1 current prompt
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := builder.buildMessages(context.Background(), tt.prompt, tt.params)
			if len(messages) != tt.expected {
				t.Errorf("Expected %d messages, got %d", tt.expected, len(messages))
			}
		})
	}
}

// mockMemory is a simple mock implementation for testing
type mockMemory struct {
	messages []interfaces.Message
}

func (m *mockMemory) AddMessage(ctx context.Context, message interfaces.Message) error {
	m.messages = append(m.messages, message)
	return nil
}

func (m *mockMemory) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return m.messages, nil
}

func (m *mockMemory) Clear(ctx context.Context) error {
	m.messages = []interfaces.Message{}
	return nil
}
