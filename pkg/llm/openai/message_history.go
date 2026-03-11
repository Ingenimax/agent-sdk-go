package openai

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/openai/openai-go/v2"
)

// messageHistoryBuilder builds OpenAI-compatible message history from memory and current prompt
type messageHistoryBuilder struct {
	logger logging.Logger
}

// newMessageHistoryBuilder creates a new message history builder
func newMessageHistoryBuilder(logger logging.Logger) *messageHistoryBuilder {
	return &messageHistoryBuilder{
		logger: logger,
	}
}

// buildMessages constructs OpenAI messages from memory and current prompt
// Returns messages ready for OpenAI API calls, preserving chronological order
func (b *messageHistoryBuilder) buildMessages(ctx context.Context, prompt string, memory interfaces.Memory, contentParts []interfaces.ContentPart) []openai.ChatCompletionMessageParamUnion {
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Add memory messages
	if memory != nil {
		memoryMessages, err := memory.GetMessages(ctx)
		if err != nil {
			b.logger.Error(ctx, "Failed to retrieve memory messages", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			// Convert memory messages to OpenAI format, preserving chronological order
			for _, msg := range memoryMessages {
				openaiMsg := b.convertMemoryMessage(msg)
				if openaiMsg != nil {
					messages = append(messages, *openaiMsg)
				}
			}
		}
	} else {
		// Only append current user message when memory is nil
		if len(contentParts) > 0 {
			messages = append(messages, b.buildMultimodalUserMessage(prompt, contentParts))
		} else {
			messages = append(messages, openai.UserMessage(prompt))
		}
	}

	return messages
}

// convertMemoryMessage converts a memory message to OpenAI format
func (b *messageHistoryBuilder) convertMemoryMessage(msg interfaces.Message) *openai.ChatCompletionMessageParamUnion {
	switch msg.Role {
	case interfaces.MessageRoleUser:
		if len(msg.ContentParts) > 0 {
			param := b.buildMultimodalUserMessage(msg.Content, msg.ContentParts)
			return &param
		}
		userMsg := openai.UserMessage(msg.Content)
		return &userMsg

	case interfaces.MessageRoleAssistant:
		if len(msg.ToolCalls) > 0 {
			// Assistant message with tool calls
			var toolCalls []openai.ChatCompletionMessageToolCallUnion

			for _, toolCall := range msg.ToolCalls {
				toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnion{
					ID:   toolCall.ID,
					Type: "function",
					Function: openai.ChatCompletionMessageFunctionToolCallFunction{
						Name:      toolCall.Name,
						Arguments: toolCall.Arguments,
					},
				})
			}

			// Create assistant message with tool calls
			assistantMsg := openai.ChatCompletionMessage{
				Role:      "assistant",
				Content:   msg.Content,
				ToolCalls: toolCalls,
			}
			param := assistantMsg.ToParam()
			return &param
		} else if msg.Content != "" {
			// Regular assistant message
			assistantMsg := openai.AssistantMessage(msg.Content)
			return &assistantMsg
		}

	case interfaces.MessageRoleTool:
		if msg.ToolCallID != "" {
			toolMsg := openai.ToolMessage(msg.Content, msg.ToolCallID)
			return &toolMsg
		}

	case interfaces.MessageRoleSystem:
		// Convert system messages from memory to OpenAI system messages
		systemMsg := openai.SystemMessage(msg.Content)
		return &systemMsg
	}

	return nil
}

func (b *messageHistoryBuilder) buildMultimodalUserMessage(prompt string, parts []interfaces.ContentPart) openai.ChatCompletionMessageParamUnion {
	contentItems := make([]openai.ChatCompletionContentPartUnionParam, 0, len(parts)+1)

	// If prompt is provided and caller didn't include a text part, prepend it.
	if prompt != "" {
		hasText := false
		for _, p := range parts {
			if p.Type == "text" {
				hasText = true
				break
			}
		}
		if !hasText {
			contentItems = append(contentItems, openai.TextContentPart(prompt))
		}
	}

	for _, part := range parts {
		switch part.Type {
		case "text":
			if part.Text == "" {
				continue
			}
			contentItems = append(contentItems, openai.TextContentPart(part.Text))
		case "image_url":
			if part.ImageURL == nil || part.ImageURL.URL == "" {
				continue
			}
			imageURL := openai.ChatCompletionContentPartImageImageURLParam{
				URL: part.ImageURL.URL,
			}
			if part.ImageURL.Detail != "" {
				imageURL.Detail = part.ImageURL.Detail
			}
			contentItems = append(contentItems, openai.ImageContentPart(imageURL))
		case "image_file":
			if part.ImageFile == nil || part.ImageFile.FileID == "" {
				continue
			}
			// openai-go chat completions uses content type "file" for file_id inputs.
			// We map SDK-level "image_file" to the OpenAI "file" content part.
			contentItems = append(contentItems, openai.FileContentPart(openai.ChatCompletionContentPartFileFileParam{
				FileID: openai.String(part.ImageFile.FileID),
			}))
		default:
			// Ignore unknown part types for forward-compatibility.
		}
	}

	if len(contentItems) == 0 {
		// Fallback to plain text if nothing valid was provided.
		return openai.UserMessage(prompt)
	}

	return openai.ChatCompletionMessageParamUnion{
		OfUser: &openai.ChatCompletionUserMessageParam{
			Content: openai.ChatCompletionUserMessageParamContentUnion{
				OfArrayOfContentParts: contentItems,
			},
		},
	}
}
