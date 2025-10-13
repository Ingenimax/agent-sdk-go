package azureopenai

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/openai/openai-go/v2"
)

// messageHistoryBuilder builds Azure OpenAI-compatible message history from memory and current prompt
type messageHistoryBuilder struct {
	logger logging.Logger
}

// newMessageHistoryBuilder creates a new message history builder
func newMessageHistoryBuilder(logger logging.Logger) *messageHistoryBuilder {
	return &messageHistoryBuilder{
		logger: logger,
	}
}

// buildMessages constructs Azure OpenAI messages from memory and current prompt
// Returns messages ready for Azure OpenAI API calls, preserving chronological order
func (b *messageHistoryBuilder) buildMessages(ctx context.Context, prompt string, memory interfaces.Memory) []openai.ChatCompletionMessageParamUnion {
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Add memory messages
	if memory != nil {
		memoryMessages, err := memory.GetMessages(ctx)
		if err != nil {
			b.logger.Error(ctx, "Failed to retrieve memory messages", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			// Convert memory messages to Azure OpenAI format, preserving chronological order
			for _, msg := range memoryMessages {
				openaiMsg := b.convertMemoryMessage(msg)
				if openaiMsg != nil {
					messages = append(messages, *openaiMsg)
				}
			}
		}
	} else {
		// Only append current user message when memory is nil
		messages = append(messages, openai.UserMessage(prompt))
	}

	return messages
}

// convertMemoryMessage converts a memory message to Azure OpenAI format
func (b *messageHistoryBuilder) convertMemoryMessage(msg interfaces.Message) *openai.ChatCompletionMessageParamUnion {
	switch msg.Role {
	case interfaces.MessageRoleUser:
		userMsg := openai.UserMessage(msg.Content)
		return &userMsg

	case interfaces.MessageRoleAssistant:
		// For Azure OpenAI, treat assistant messages with tool calls as regular assistant messages
		// The tool results will be added separately as tool messages
		if msg.Content != "" {
			assistantMsg := openai.AssistantMessage(msg.Content)
			return &assistantMsg
		}

	case interfaces.MessageRoleTool:
		if msg.ToolCallID != "" {
			toolMsg := openai.ToolMessage(msg.Content, msg.ToolCallID)
			return &toolMsg
		}

	case interfaces.MessageRoleSystem:
		// Convert system messages from memory to Azure OpenAI system messages
		systemMsg := openai.SystemMessage(msg.Content)
		return &systemMsg
	}

	return nil
}
