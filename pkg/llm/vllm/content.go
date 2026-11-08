package vllm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// contentPart is one entry of a Chat Completions content list.
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

// MarshalJSON sends Content as a plain string unless the message carries
// parts. Then content becomes a list with one entry per part, in order.
func (m ChatMessage) MarshalJSON() ([]byte, error) {
	type plain ChatMessage
	if len(m.Parts) == 0 {
		return json.Marshal(plain(m))
	}
	return json.Marshal(struct {
		plain
		Content []contentPart `json:"content"`
	}{plain(m), contentParts(m.Parts)})
}

// contentParts converts parts to Chat Completions content entries: text as
// text, and images as image_url entries holding a data URL or the image's
// URL. Empty text parts are left out. checkContent has already refused every
// other part.
func contentParts(parts []*interfaces.Part) []contentPart {
	out := make([]contentPart, 0, len(parts))
	for _, p := range parts {
		switch {
		case p.InlineData != nil:
			out = append(out, contentPart{Type: "image_url", ImageURL: &imageURL{URL: p.InlineData.DataURL()}})
		case p.FileData != nil:
			out = append(out, contentPart{Type: "image_url", ImageURL: &imageURL{URL: p.FileData.FileURI}})
		case p.Text != "":
			out = append(out, contentPart{Type: "text", Text: p.Text})
		}
	}
	return out
}

// chatMessages returns the opening messages of a chat request: the system
// message, then the conversation. Without memory, the conversation is the
// prompt, with the parts set by WithParts. With memory, it is the user
// messages, with their parts, and the assistant text replies in memory,
// where the agent has already stored the current turn. Tool calls and tool
// results in memory are left out, so the request has no tool result without
// its call.
func chatMessages(ctx context.Context, prompt string, params *interfaces.GenerateOptions) ([]ChatMessage, error) {
	var messages []ChatMessage
	if params.SystemMessage != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: params.SystemMessage})
	}
	if params.Memory == nil {
		return append(messages, ChatMessage{Role: "user", Content: interfaces.PromptText(prompt, params), Parts: params.Parts}), nil
	}
	history, err := params.Memory.GetMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read memory: %w", err)
	}
	for _, m := range history {
		switch {
		case m.Role == interfaces.MessageRoleUser:
			messages = append(messages, ChatMessage{Role: "user", Content: m.Content, Parts: m.Parts})
		case m.Role == interfaces.MessageRoleAssistant && len(m.ToolCalls) == 0 && m.Content != "":
			messages = append(messages, ChatMessage{Role: "assistant", Content: m.Content})
		}
	}
	return messages, nil
}

// checkContent refuses every part of the request that is not text or an
// image. vLLM's chat endpoint takes images inline as a data URL or by URL;
// the served model must accept images too.
func checkContent(ctx context.Context, params *interfaces.GenerateOptions) error {
	return interfaces.CheckMedia(ctx, params, "vllm", (*interfaces.Part).IsImage)
}
