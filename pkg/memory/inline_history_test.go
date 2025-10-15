package memory

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestBuildInlineHistoryPrompt(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		history  []interfaces.Message
		expected string
	}{
		{
			name:     "no memory",
			prompt:   "Hello",
			history:  nil,
			expected: "User: Hello",
		},
		{
			name:     "empty memory",
			prompt:   "Hello",
			history:  []interfaces.Message{},
			expected: "Hello",
		},
		{
			name:   "system message first",
			prompt: "Hello",
			history: []interfaces.Message{
				{Role: interfaces.MessageRoleUser, Content: "Previous question"},
				{Role: interfaces.MessageRoleSystem, Content: "You are a helpful assistant"},
				{Role: interfaces.MessageRoleAssistant, Content: "Previous answer"},
			},
			expected: "System: You are a helpful assistant\nUser: Previous question\nAssistant: Previous answer\nUser: Hello",
		},
		{
			name:   "conversation with tools",
			prompt: "What's the status?",
			history: []interfaces.Message{
				{Role: interfaces.MessageRoleUser, Content: "Check the database"},
				{Role: interfaces.MessageRoleAssistant, Content: "I'll check the database for you"},
				{
					Role:       interfaces.MessageRoleTool,
					Content:    "Database is running",
					ToolCallID: "tool_123",
					Metadata:   map[string]interface{}{"tool_name": "db_check"},
				},
				{Role: interfaces.MessageRoleAssistant, Content: "The database is running normally"},
			},
			expected: "User: Check the database\nAssistant: I'll check the database for you\nTool db_check result: Database is running\nAssistant: The database is running normally\nUser: What's the status?",
		},
		{
			name:   "tool without metadata",
			prompt: "Continue",
			history: []interfaces.Message{
				{
					Role:       interfaces.MessageRoleTool,
					Content:    "Some result",
					ToolCallID: "tool_456",
				},
			},
			expected: "Tool unknown result: Some result\nUser: Continue",
		},
		{
			name:   "assistant message with empty content",
			prompt: "Next",
			history: []interfaces.Message{
				{Role: interfaces.MessageRoleAssistant, Content: ""},
				{Role: interfaces.MessageRoleUser, Content: "Previous"},
			},
			expected: "User: Previous\nUser: Next",
		},
		{
			name:   "basic conversation with clear role markers",
			prompt: "What can you help me with?",
			history: []interfaces.Message{
				{Role: interfaces.MessageRoleUser, Content: "Hello, how are you?"},
				{Role: interfaces.MessageRoleAssistant, Content: "I'm doing well, thank you!"},
			},
			expected: "User: Hello, how are you?\nAssistant: I'm doing well, thank you!\nUser: What can you help me with?",
		},
		{
			name:   "conversation with tool messages",
			prompt: "What's the cluster status?",
			history: []interfaces.Message{
				{Role: interfaces.MessageRoleUser, Content: "list which clusters I have available"},
				{Role: interfaces.MessageRoleAssistant, Content: `{"reasoning":["User is requesting a list of available clusters"]}`},
				{
					Role:       interfaces.MessageRoleTool,
					Content:    `{"query": "list all EKS clusters", "output": "eks-cluster-1"}`,
					ToolCallID: "tool_789",
					Metadata:   map[string]interface{}{"tool_name": "cluster_list"},
				},
				{Role: interfaces.MessageRoleAssistant, Content: "You have eks-cluster-1 available"},
			},
			expected: "User: list which clusters I have available\nAssistant: {\"reasoning\":[\"User is requesting a list of available clusters\"]}\nTool cluster_list result: {\"query\": \"list all EKS clusters\", \"output\": \"eks-cluster-1\"}\nAssistant: You have eks-cluster-1 available\nUser: What's the cluster status?",
		},
		{
			name:   "single message",
			prompt: "How are you?",
			history: []interfaces.Message{
				{Role: interfaces.MessageRoleUser, Content: "Hello"},
			},
			expected: "User: Hello\nUser: How are you?",
		},
		{
			name:   "system message included with conversation",
			prompt: "What should I do next?",
			history: []interfaces.Message{
				{Role: interfaces.MessageRoleSystem, Content: "You are a helpful assistant"},
				{Role: interfaces.MessageRoleUser, Content: "Hi"},
				{Role: interfaces.MessageRoleAssistant, Content: "Hello!"},
			},
			expected: "System: You are a helpful assistant\nUser: Hi\nAssistant: Hello!\nUser: What should I do next?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock memory, but pass nil for "no memory" test case
			var memory interfaces.Memory
			if tt.history != nil {
				memory = &mockMemory{messages: tt.history}
			}

			result := BuildInlineHistoryPrompt(context.Background(), tt.prompt, memory, nil)
			if result != tt.expected {
				t.Errorf("BuildInlineHistoryPrompt() mismatch\nGot:\n%s\n\nExpected:\n%s", result, tt.expected)
			}
		})
	}
}

// mockMemory is a simple in-memory implementation for testing
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
	m.messages = nil
	return nil
}
