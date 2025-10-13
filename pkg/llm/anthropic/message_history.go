package anthropic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// messageHistoryBuilder builds Anthropic-compatible message history from memory and current prompt
type messageHistoryBuilder struct {
	logger logging.Logger
}

// newMessageHistoryBuilder creates a new message history builder
func newMessageHistoryBuilder(logger logging.Logger) *messageHistoryBuilder {
	return &messageHistoryBuilder{
		logger: logger,
	}
}

// buildMessages constructs Anthropic messages from memory and current prompt
// Returns messages ready for Anthropic API calls, preserving chronological order
func (b *messageHistoryBuilder) buildMessages(ctx context.Context, prompt string, params *interfaces.GenerateOptions) []Message {
	messages := []Message{}

	// Add memory messages
	if params.Memory != nil {
		memoryMessages, err := params.Memory.GetMessages(ctx)
		if err != nil {
			b.logger.Error(ctx, "Failed to retrieve memory messages", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			// Convert memory messages to Anthropic format, preserving chronological order
			for _, msg := range memoryMessages {
				anthropicMsg := b.convertMemoryMessage(msg)
				if anthropicMsg != nil {
					messages = append(messages, *anthropicMsg)
				}
			}
		}
	} else {
		// Only append current user message when memory is nil
		messages = append(messages, Message{
			Role:    "user",
			Content: prompt,
		})
	}

	return messages
}

// convertMemoryMessage converts a memory message to Anthropic format
func (b *messageHistoryBuilder) convertMemoryMessage(msg interfaces.Message) *Message {
	switch msg.Role {
	case interfaces.MessageRoleUser:
		return &Message{
			Role:    "user",
			Content: msg.Content,
		}

	case interfaces.MessageRoleAssistant:
		if len(msg.ToolCalls) > 0 {
			// Assistant message with tool calls
			var contentParts []string

			// Add text content if present
			if msg.Content != "" {
				contentParts = append(contentParts, msg.Content)
			}

			// Add tool use information
			for _, toolCall := range msg.ToolCalls {
				// Parse arguments from JSON string
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(toolCall.Arguments), &args); err != nil {
					b.logger.Warn(context.Background(), "Failed to parse tool call arguments", map[string]interface{}{
						"error":     err.Error(),
						"arguments": toolCall.Arguments,
					})
					continue
				}

				// Create tool use content
				toolUseContent := map[string]interface{}{
					"type":  "tool_use",
					"id":    toolCall.ID,
					"name":  toolCall.Name,
					"input": args,
				}

				// Convert to JSON string for content
				if toolUseJSON, err := json.Marshal(toolUseContent); err == nil {
					contentParts = append(contentParts, string(toolUseJSON))
				}
			}

			// Join all content parts
			content := ""
			if len(contentParts) > 0 {
				content = contentParts[0]
				for i := 1; i < len(contentParts); i++ {
					content += "\n" + contentParts[i]
				}
			}

			return &Message{
				Role:    "assistant",
				Content: content,
			}
		} else if msg.Content != "" {
			// Regular assistant message
			return &Message{
				Role:    "assistant",
				Content: msg.Content,
			}
		}

	case interfaces.MessageRoleTool:
		// Tool messages in Anthropic are handled as tool results
		if msg.ToolCallID != "" {
			return &Message{
				Role:    "user",
				Content: fmt.Sprintf("Tool result for %s: %s", msg.ToolCallID, msg.Content),
			}
		}

	case interfaces.MessageRoleSystem:
		return &Message{
			Role:    "user", // System instruction is handled separately, other system (like summarized) are passed as user messages
			Content: fmt.Sprintf("System: %s", msg.Content),
		}
	}

	return nil
}
