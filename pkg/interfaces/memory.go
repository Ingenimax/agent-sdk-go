package interfaces

import (
	"context"
)

// MessageRole represents the role of a message sender
type MessageRole string

const (
	// MessageRoleUser represents a user message
	MessageRoleUser MessageRole = "user"
	// MessageRoleAssistant represents an assistant message
	MessageRoleAssistant MessageRole = "assistant"
	// MessageRoleSystem represents a system message
	MessageRoleSystem MessageRole = "system"
	// MessageRoleTool represents a tool response message
	MessageRoleTool MessageRole = "tool"
)

// Message represents a message in a conversation
type Message struct {
	// Role is the role of the message sender
	Role MessageRole

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
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Internal    bool   `json:"internal,omitempty"`
	Arguments   string `json:"arguments"`
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

// ConversationMemory extends Memory interface with conversation-level operations
type ConversationMemory interface {
	Memory

	// GetAllConversations returns all conversation IDs for current org
	GetAllConversations(ctx context.Context) ([]string, error)

	// GetConversationMessages gets all messages for a specific conversation in current org
	GetConversationMessages(ctx context.Context, conversationID string) ([]Message, error)

	// GetMemoryStatistics returns basic memory statistics for current org
	GetMemoryStatistics(ctx context.Context) (totalConversations, totalMessages int, err error)
}

// AdminConversationMemory extends ConversationMemory with cross-org operations
type AdminConversationMemory interface {
	ConversationMemory

	// GetAllConversationsAcrossOrgs returns all conversation IDs from all organizations
	GetAllConversationsAcrossOrgs() (map[string][]string, error) // map[orgID][]conversationID

	// GetConversationMessagesAcrossOrgs finds conversation in any org and returns messages
	GetConversationMessagesAcrossOrgs(conversationID string) ([]Message, string, error) // messages, orgID, error

	// GetMemoryStatisticsAcrossOrgs returns memory statistics across all organizations
	GetMemoryStatisticsAcrossOrgs() (totalConversations, totalMessages int, err error)
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
