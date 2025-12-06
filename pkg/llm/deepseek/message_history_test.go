package deepseek

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func TestBuildMessagesWithoutMemory(t *testing.T) {
	builder := newMessageHistoryBuilder(logging.New())
	ctx := context.Background()

	messages := builder.buildMessages(ctx, "test prompt", nil)

	if len(messages) != 1 {
		t.Errorf("len(messages) = %v, want 1", len(messages))
	}

	if messages[0].Role != "user" {
		t.Errorf("Role = %v, want user", messages[0].Role)
	}

	if messages[0].Content != "test prompt" {
		t.Errorf("Content = %v, want 'test prompt'", messages[0].Content)
	}
}

func TestBuildMessagesWithMemory(t *testing.T) {
	builder := newMessageHistoryBuilder(logging.New())
	ctx := context.Background()
	ctx = memory.WithConversationID(ctx, "test-conv")
	ctx = multitenancy.WithOrgID(ctx, "test-org")

	// Create memory with messages
	mem := memory.NewConversationBuffer()
	if err := mem.AddMessage(ctx, interfaces.Message{
		Role:    interfaces.MessageRoleUser,
		Content: "Previous question",
	}); err != nil {
		t.Fatalf("Failed to add message: %v", err)
	}
	if err := mem.AddMessage(ctx, interfaces.Message{
		Role:    interfaces.MessageRoleAssistant,
		Content: "Previous answer",
	}); err != nil {
		t.Fatalf("Failed to add message: %v", err)
	}

	messages := builder.buildMessages(ctx, "new prompt", mem)

	// Should have 3 messages: 2 from memory + 1 new prompt
	if len(messages) != 3 {
		t.Fatalf("len(messages) = %v, want 3", len(messages))
	}

	// Verify first message from memory
	if messages[0].Role != "user" {
		t.Errorf("messages[0].Role = %v, want user", messages[0].Role)
	}

	if messages[0].Content != "Previous question" {
		t.Errorf("messages[0].Content = %v, want 'Previous question'", messages[0].Content)
	}

	// Verify second message from memory
	if messages[1].Role != "assistant" {
		t.Errorf("messages[1].Role = %v, want assistant", messages[1].Role)
	}

	if messages[1].Content != "Previous answer" {
		t.Errorf("messages[1].Content = %v, want 'Previous answer'", messages[1].Content)
	}

	// Verify new prompt
	if messages[2].Role != "user" {
		t.Errorf("messages[2].Role = %v, want user", messages[2].Role)
	}

	if messages[2].Content != "new prompt" {
		t.Errorf("messages[2].Content = %v, want 'new prompt'", messages[2].Content)
	}
}

func TestConvertMemoryMessageSimple(t *testing.T) {
	builder := newMessageHistoryBuilder(logging.New())

	tests := []struct {
		name        string
		input       interfaces.Message
		wantRole    string
		wantContent string
	}{
		{
			name: "user message",
			input: interfaces.Message{
				Role:    interfaces.MessageRoleUser,
				Content: "user content",
			},
			wantRole:    "user",
			wantContent: "user content",
		},
		{
			name: "assistant message",
			input: interfaces.Message{
				Role:    interfaces.MessageRoleAssistant,
				Content: "assistant content",
			},
			wantRole:    "assistant",
			wantContent: "assistant content",
		},
		{
			name: "system message",
			input: interfaces.Message{
				Role:    interfaces.MessageRoleSystem,
				Content: "system content",
			},
			wantRole:    "system",
			wantContent: "system content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.convertMemoryMessage(tt.input)

			if result.Role != tt.wantRole {
				t.Errorf("Role = %v, want %v", result.Role, tt.wantRole)
			}

			if result.Content != tt.wantContent {
				t.Errorf("Content = %v, want %v", result.Content, tt.wantContent)
			}
		})
	}
}

func TestConvertMemoryMessageWithToolCalls(t *testing.T) {
	builder := newMessageHistoryBuilder(logging.New())

	input := interfaces.Message{
		Role:    interfaces.MessageRoleAssistant,
		Content: "Let me call a tool",
		ToolCalls: []interfaces.ToolCall{
			{
				ID:        "call-1",
				Name:      "weather",
				Arguments: `{"location":"NYC"}`,
			},
			{
				ID:        "call-2",
				Name:      "calculator",
				Arguments: `{"expression":"2+2"}`,
			},
		},
	}

	result := builder.convertMemoryMessage(input)

	if result.Role != "assistant" {
		t.Errorf("Role = %v, want assistant", result.Role)
	}

	if result.Content != "Let me call a tool" {
		t.Errorf("Content = %v, want 'Let me call a tool'", result.Content)
	}

	if len(result.ToolCalls) != 2 {
		t.Fatalf("len(ToolCalls) = %v, want 2", len(result.ToolCalls))
	}

	// Verify first tool call
	if result.ToolCalls[0].ID != "call-1" {
		t.Errorf("ToolCalls[0].ID = %v, want call-1", result.ToolCalls[0].ID)
	}

	if result.ToolCalls[0].Type != "function" {
		t.Errorf("ToolCalls[0].Type = %v, want function", result.ToolCalls[0].Type)
	}

	if result.ToolCalls[0].Function.Name != "weather" {
		t.Errorf("ToolCalls[0].Function.Name = %v, want weather", result.ToolCalls[0].Function.Name)
	}

	if result.ToolCalls[0].Function.Arguments != `{"location":"NYC"}` {
		t.Errorf("ToolCalls[0].Function.Arguments = %v, want {\"location\":\"NYC\"}", result.ToolCalls[0].Function.Arguments)
	}

	// Verify second tool call
	if result.ToolCalls[1].ID != "call-2" {
		t.Errorf("ToolCalls[1].ID = %v, want call-2", result.ToolCalls[1].ID)
	}

	if result.ToolCalls[1].Function.Name != "calculator" {
		t.Errorf("ToolCalls[1].Function.Name = %v, want calculator", result.ToolCalls[1].Function.Name)
	}
}

func TestConvertMemoryMessageToolResult(t *testing.T) {
	builder := newMessageHistoryBuilder(logging.New())

	input := interfaces.Message{
		Role:       interfaces.MessageRoleTool,
		Content:    "Tool result content",
		ToolCallID: "call-123",
		Metadata: map[string]interface{}{
			"tool_name": "weather",
		},
	}

	result := builder.convertMemoryMessage(input)

	if result.Role != "tool" {
		t.Errorf("Role = %v, want tool", result.Role)
	}

	if result.Content != "Tool result content" {
		t.Errorf("Content = %v, want 'Tool result content'", result.Content)
	}

	if result.ToolCallID != "call-123" {
		t.Errorf("ToolCallID = %v, want call-123", result.ToolCallID)
	}

	if result.Name != "weather" {
		t.Errorf("Name = %v, want weather", result.Name)
	}
}

func TestBuildMessagesWithComplexConversation(t *testing.T) {
	builder := newMessageHistoryBuilder(logging.New())
	ctx := context.Background()
	ctx = memory.WithConversationID(ctx, "test-conv")
	ctx = multitenancy.WithOrgID(ctx, "test-org")

	// Create memory with a complex conversation including tool calls
	mem := memory.NewConversationBuffer()

	// User asks a question
	if err := mem.AddMessage(ctx, interfaces.Message{
		Role:    interfaces.MessageRoleUser,
		Content: "What's the weather in NYC?",
	}); err != nil {
		t.Fatalf("Failed to add message: %v", err)
	}

	// Assistant calls tool
	if err := mem.AddMessage(ctx, interfaces.Message{
		Role:    interfaces.MessageRoleAssistant,
		Content: "Let me check the weather for you",
		ToolCalls: []interfaces.ToolCall{
			{
				ID:        "call-1",
				Name:      "get_weather",
				Arguments: `{"location":"NYC"}`,
			},
		},
	}); err != nil {
		t.Fatalf("Failed to add message: %v", err)
	}

	// Tool result
	if err := mem.AddMessage(ctx, interfaces.Message{
		Role:       interfaces.MessageRoleTool,
		Content:    "The weather in NYC is sunny, 75F",
		ToolCallID: "call-1",
		Metadata: map[string]interface{}{
			"tool_name": "get_weather",
		},
	}); err != nil {
		t.Fatalf("Failed to add message: %v", err)
	}

	// Assistant responds
	if err := mem.AddMessage(ctx, interfaces.Message{
		Role:    interfaces.MessageRoleAssistant,
		Content: "The weather in NYC is sunny with a temperature of 75F",
	}); err != nil {
		t.Fatalf("Failed to add message: %v", err)
	}

	messages := builder.buildMessages(ctx, "What about Boston?", mem)

	// Should have 5 messages: 4 from memory + 1 new prompt
	if len(messages) != 5 {
		t.Fatalf("len(messages) = %v, want 5", len(messages))
	}

	// Verify user message
	if messages[0].Role != "user" || messages[0].Content != "What's the weather in NYC?" {
		t.Error("First message doesn't match expected user message")
	}

	// Verify assistant with tool call
	if messages[1].Role != "assistant" {
		t.Error("Second message should be assistant")
	}

	if len(messages[1].ToolCalls) != 1 {
		t.Error("Second message should have one tool call")
	}

	// Verify tool result
	if messages[2].Role != "tool" {
		t.Error("Third message should be tool")
	}

	if messages[2].ToolCallID != "call-1" {
		t.Error("Third message should have correct tool call ID")
	}

	// Verify assistant response
	if messages[3].Role != "assistant" {
		t.Error("Fourth message should be assistant")
	}

	// Verify new prompt
	if messages[4].Role != "user" || messages[4].Content != "What about Boston?" {
		t.Error("Fifth message doesn't match expected new prompt")
	}
}
