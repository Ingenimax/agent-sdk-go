package deepseek

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// messageHistoryBuilder builds message history from memory
type messageHistoryBuilder struct {
	logger logging.Logger
}

// newMessageHistoryBuilder creates a new message history builder
func newMessageHistoryBuilder(logger logging.Logger) *messageHistoryBuilder {
	return &messageHistoryBuilder{
		logger: logger,
	}
}

// buildMessages builds messages from memory and current prompt
func (b *messageHistoryBuilder) buildMessages(ctx context.Context, prompt string, memory interfaces.Memory) []Message {
	messages := []Message{}

	// If memory is available, build from memory
	if memory != nil {
		memoryMessages, err := memory.GetMessages(ctx)
		if err != nil {
			b.logger.Error(ctx, "Failed to retrieve memory messages", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			b.logger.Debug(ctx, "Building messages from memory", map[string]interface{}{
				"memory_messages": len(memoryMessages),
			})

			for _, msg := range memoryMessages {
				convertedMsg := b.convertMemoryMessage(msg)
				messages = append(messages, convertedMsg)
			}
		}
	}

	// Add current prompt as user message
	messages = append(messages, Message{
		Role:    "user",
		Content: prompt,
	})

	return messages
}

// convertMemoryMessage converts a memory message to DeepSeek format
func (b *messageHistoryBuilder) convertMemoryMessage(msg interfaces.Message) Message {
	message := Message{
		Role:    string(msg.Role),
		Content: msg.Content,
	}

	// Handle tool calls in assistant messages
	if msg.Role == interfaces.MessageRoleAssistant && len(msg.ToolCalls) > 0 {
		toolCalls := make([]ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			toolCalls[i] = ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			}
		}
		message.ToolCalls = toolCalls
	}

	// Handle tool result messages
	if msg.Role == interfaces.MessageRoleTool {
		message.ToolCallID = msg.ToolCallID
		// Extract tool name from metadata if available
		if msg.Metadata != nil {
			if toolName, ok := msg.Metadata["tool_name"].(string); ok {
				message.Name = toolName
			}
		}
	}

	return message
}
