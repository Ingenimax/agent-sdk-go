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
//
// parts replace the prompt and are read only when memory is nil. With
// memory, each user message carries its own parts.
func (b *messageHistoryBuilder) buildMessages(ctx context.Context, prompt string, memory interfaces.Memory, parts []*interfaces.Part) []openai.ChatCompletionMessageParamUnion {
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
		messages = append(messages, userMessage(prompt, parts))
	}

	return messages
}

// convertMemoryMessage converts a memory message to Azure OpenAI format
func (b *messageHistoryBuilder) convertMemoryMessage(msg interfaces.Message) *openai.ChatCompletionMessageParamUnion {
	switch msg.Role {
	case interfaces.MessageRoleUser:
		userMsg := userMessage(msg.Content, msg.Parts)
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
		// Convert system messages from memory to Azure OpenAI system messages
		systemMsg := openai.SystemMessage(msg.Content)
		return &systemMsg
	}

	return nil
}

// userMessage builds a user message. A message with parts is sent as content
// parts, in their order; without parts it is the plain text. checkContent has
// already refused every part that is not text or an image.
func userMessage(text string, parts []*interfaces.Part) openai.ChatCompletionMessageParamUnion {
	if len(parts) == 0 {
		return openai.UserMessage(text)
	}
	content := make([]openai.ChatCompletionContentPartUnionParam, 0, len(parts))
	for _, p := range parts {
		switch {
		case p.InlineData != nil:
			content = append(content, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: p.InlineData.DataURL()}))
		case p.FileData != nil:
			content = append(content, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: p.FileData.FileURI}))
		default:
			content = append(content, openai.TextContentPart(p.Text))
		}
	}
	return openai.UserMessage(content)
}

// imageTypes are the image MIME types Azure OpenAI Chat Completions accepts.
var imageTypes = []string{"image/png", "image/jpeg", "image/webp", "image/gif"}

// checkContent refuses every part of the request that this client cannot
// send: anything other than text or a PNG, JPEG, WebP, or GIF image (inline
// or by URL). PDFs are refused: Azure documents file content parts only for
// its v1 API, and this client calls the dated deployments API.
func checkContent(ctx context.Context, params *interfaces.GenerateOptions) error {
	return interfaces.CheckMedia(ctx, params, "azureopenai", func(p *interfaces.Part) bool {
		return p.HasMIMEType(imageTypes...)
	})
}
