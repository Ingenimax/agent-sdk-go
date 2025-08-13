package interfaces

import (
	"context"
)

// Message represents a message in a conversation
type Message struct {
	// Role is the role of the message sender (e.g., "user", "assistant", "system", "tool")
	Role string

	// Content is the content of the message
	Content string

	// Metadata contains additional information about the message
	Metadata map[string]interface{}

	// ToolCallID is used for tool messages to reference the tool call
	ToolCallID string

	// ToolCalls contains tool call information for assistant messages
	ToolCalls []ToolCall
}

// ToolCall represents a tool call made by the assistant
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Memory represents a memory store for agent conversations
type Memory interface {
	// AddMessage adds a message to memory
	AddMessage(ctx context.Context, message Message) error

	// GetMessages retrieves messages from memory
	GetMessages(ctx context.Context, options ...GetMessagesOption) ([]Message, error)

	// Clear clears the memory
	Clear(ctx context.Context) error
}

// ToolMemory is an optional interface that memory implementations can implement
// to handle tool call storage more efficiently. If not implemented, tool calls
// will be stored as regular messages.
type ToolMemory interface {
	Memory

	// AddToolCall adds a tool call and its result to memory
	AddToolCall(ctx context.Context, toolCall ToolCall, result string) error

	// AddAssistantMessageWithToolCalls adds an assistant message containing tool calls
	AddAssistantMessageWithToolCalls(ctx context.Context, content string, toolCalls []ToolCall) error
}

// GetMessagesOptions contains options for retrieving messages
type GetMessagesOptions struct {
	// Limit is the maximum number of messages to retrieve
	Limit int

	// Roles filters messages by role
	Roles []string

	// Query is a search query for relevant messages
	Query string
}

// GetMessagesOption represents an option for retrieving messages
type GetMessagesOption func(*GetMessagesOptions)

// WithLimit sets the maximum number of messages to retrieve
func WithLimit(limit int) GetMessagesOption {
	return func(o *GetMessagesOptions) {
		o.Limit = limit
	}
}

// WithRoles filters messages by role
func WithRoles(roles ...string) GetMessagesOption {
	return func(o *GetMessagesOptions) {
		o.Roles = roles
	}
}

// WithQuery sets a search query for relevant messages
func WithQuery(query string) GetMessagesOption {
	return func(o *GetMessagesOptions) {
		o.Query = query
	}
}
