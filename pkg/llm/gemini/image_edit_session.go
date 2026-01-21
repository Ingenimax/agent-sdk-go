package gemini

import (
	"context"
	"encoding/base64"
	"fmt"

	"google.golang.org/genai"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// GeminiImageEditSession implements interfaces.ImageEditSession using Gemini chat API.
// It maintains a conversation context for iterative image creation and modification.
type GeminiImageEditSession struct {
	client *GeminiClient
	chat   *genai.Chat
	model  string
	logger logging.Logger
}

// SendMessage sends a text message and returns the response (text and/or image).
// The conversation context is automatically maintained by the underlying chat session.
func (s *GeminiImageEditSession) SendMessage(ctx context.Context, message string, options *interfaces.ImageEditOptions) (*interfaces.ImageEditResponse, error) {
	if message == "" {
		return nil, interfaces.ErrInvalidPrompt
	}

	s.logger.Debug(ctx, "Sending message to image edit session", map[string]interface{}{
		"model":        s.model,
		"message_len":  len(message),
		"has_options":  options != nil,
	})

	// Build message part
	part := genai.Part{Text: message}

	// Send message via chat
	result, err := s.chat.SendMessage(ctx, part)
	if err != nil {
		s.logger.Error(ctx, "Image edit session SendMessage failed", map[string]interface{}{
			"error": err.Error(),
			"model": s.model,
		})

		if isContentBlockedError(err) {
			return nil, fmt.Errorf("%w: %v", interfaces.ErrContentBlocked, err)
		}
		if isRateLimitError(err) {
			return nil, fmt.Errorf("%w: %v", interfaces.ErrRateLimitExceeded, err)
		}
		return nil, fmt.Errorf("image edit session send message failed: %w", err)
	}

	return s.parseResponse(result)
}

// SendMessageWithImage sends a message with an image reference for editing.
// This allows providing an external image for the model to modify.
func (s *GeminiImageEditSession) SendMessageWithImage(ctx context.Context, message string, image *interfaces.ImageData, options *interfaces.ImageEditOptions) (*interfaces.ImageEditResponse, error) {
	if message == "" && image == nil {
		return nil, interfaces.ErrInvalidPrompt
	}

	s.logger.Debug(ctx, "Sending message with image to edit session", map[string]interface{}{
		"model":       s.model,
		"message_len": len(message),
		"has_image":   image != nil,
		"has_options": options != nil,
	})

	// Build message parts
	var parts []genai.Part

	if message != "" {
		parts = append(parts, genai.Part{Text: message})
	}

	if image != nil {
		imageData := image.Data
		if imageData == nil && image.Base64 != "" {
			// Decode base64
			decoded, err := base64.StdEncoding.DecodeString(image.Base64)
			if err != nil {
				return nil, fmt.Errorf("failed to decode image base64: %w", err)
			}
			imageData = decoded
		}

		if len(imageData) > 0 {
			mimeType := image.MimeType
			if mimeType == "" {
				mimeType = "image/png"
			}
			parts = append(parts, genai.Part{
				InlineData: &genai.Blob{
					Data:     imageData,
					MIMEType: mimeType,
				},
			})
		}
	}

	// Send message via chat
	result, err := s.chat.SendMessage(ctx, parts...)
	if err != nil {
		s.logger.Error(ctx, "Image edit session SendMessageWithImage failed", map[string]interface{}{
			"error": err.Error(),
			"model": s.model,
		})

		if isContentBlockedError(err) {
			return nil, fmt.Errorf("%w: %v", interfaces.ErrContentBlocked, err)
		}
		if isRateLimitError(err) {
			return nil, fmt.Errorf("%w: %v", interfaces.ErrRateLimitExceeded, err)
		}
		return nil, fmt.Errorf("image edit session send message with image failed: %w", err)
	}

	return s.parseResponse(result)
}

// GetHistory returns the conversation history for this session.
func (s *GeminiImageEditSession) GetHistory() []interfaces.ImageEditTurn {
	genaiHistory := s.chat.History(true) // curated=true to get clean history

	turns := make([]interfaces.ImageEditTurn, 0, len(genaiHistory))
	for _, content := range genaiHistory {
		turn := interfaces.ImageEditTurn{
			Role:   content.Role,
			Images: make([]*interfaces.ImageData, 0),
		}

		// Extract text and images from parts
		for _, part := range content.Parts {
			if part.Text != "" {
				if turn.Message != "" {
					turn.Message += "\n"
				}
				turn.Message += part.Text
			}

			if part.InlineData != nil && part.InlineData.Data != nil {
				turn.Images = append(turn.Images, &interfaces.ImageData{
					Data:     part.InlineData.Data,
					Base64:   base64.StdEncoding.EncodeToString(part.InlineData.Data),
					MimeType: part.InlineData.MIMEType,
				})
			}
		}

		turns = append(turns, turn)
	}

	return turns
}

// Close closes the session and releases resources.
// Note: The genai Chat doesn't require explicit cleanup, but we implement
// this for consistency with the interface contract.
func (s *GeminiImageEditSession) Close() error {
	s.logger.Debug(context.Background(), "Closing image edit session", map[string]interface{}{
		"model": s.model,
	})
	// genai.Chat doesn't have a Close method, so this is a no-op
	// The chat will be garbage collected when no longer referenced
	return nil
}

// parseResponse extracts text and images from the API response.
func (s *GeminiImageEditSession) parseResponse(result *genai.GenerateContentResponse) (*interfaces.ImageEditResponse, error) {
	response := &interfaces.ImageEditResponse{
		Images:   make([]interfaces.GeneratedImage, 0),
		Metadata: make(map[string]interface{}),
	}

	if result == nil || len(result.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	for _, candidate := range result.Candidates {
		if candidate.Content == nil {
			continue
		}

		for _, part := range candidate.Content.Parts {
			// Extract text response
			if part.Text != "" {
				if response.Text != "" {
					response.Text += "\n"
				}
				response.Text += part.Text
			}

			// Extract image response
			if part.InlineData != nil && part.InlineData.Data != nil {
				mimeType := part.InlineData.MIMEType
				if mimeType == "" {
					mimeType = "image/png"
				}

				image := interfaces.GeneratedImage{
					Data:     part.InlineData.Data,
					Base64:   base64.StdEncoding.EncodeToString(part.InlineData.Data),
					MimeType: mimeType,
				}

				// Extract finish reason if available
				if candidate.FinishReason != "" {
					image.FinishReason = string(candidate.FinishReason)
				}

				response.Images = append(response.Images, image)
			}
		}
	}

	// Extract usage metadata
	if result.UsageMetadata != nil {
		response.Usage = &interfaces.ImageUsage{
			InputTokens:     int(result.UsageMetadata.PromptTokenCount),
			OutputTokens:    int(result.UsageMetadata.CandidatesTokenCount),
			ImagesGenerated: len(response.Images),
		}
	}

	// Store model info
	response.Metadata["model"] = s.model

	s.logger.Debug(context.Background(), "Parsed image edit response", map[string]interface{}{
		"text_len":     len(response.Text),
		"image_count":  len(response.Images),
		"has_usage":    response.Usage != nil,
	})

	return response, nil
}
