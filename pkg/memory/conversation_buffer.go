package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// ConversationBuffer implements a simple in-memory conversation buffer
type ConversationBuffer struct {
	messages map[string][]interfaces.Message
	maxSize  int
	mu       sync.RWMutex
}

// Option represents an option for configuring the conversation buffer
type Option func(*ConversationBuffer)

// WithMaxSize sets the maximum number of messages to store
func WithMaxSize(size int) Option {
	return func(c *ConversationBuffer) {
		c.maxSize = size
	}
}

// NewConversationBuffer creates a new conversation buffer
func NewConversationBuffer(options ...Option) *ConversationBuffer {
	buffer := &ConversationBuffer{
		messages: make(map[string][]interfaces.Message),
		maxSize:  100, // Default max size
	}

	for _, option := range options {
		option(buffer)
	}

	return buffer
}

// AddMessage adds a message to the buffer
func (c *ConversationBuffer) AddMessage(ctx context.Context, message interfaces.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Get conversation ID from context
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return err
	}

	// Add message to buffer
	c.messages[conversationID] = append(c.messages[conversationID], message)

	// Trim buffer if it exceeds max size
	if c.maxSize > 0 && len(c.messages[conversationID]) > c.maxSize {
		c.messages[conversationID] = c.messages[conversationID][len(c.messages[conversationID])-c.maxSize:]
	}

	return nil
}

// GetMessages retrieves messages from the buffer
func (c *ConversationBuffer) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get conversation ID from context
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return nil, err
	}

	// Get messages for conversation
	messages, ok := c.messages[conversationID]
	if !ok {
		return []interfaces.Message{}, nil
	}

	// Apply options
	opts := &interfaces.GetMessagesOptions{}
	for _, option := range options {
		option(opts)
	}

	// Filter by role if specified
	if len(opts.Roles) > 0 {
		var filtered []interfaces.Message
		for _, msg := range messages {
			for _, role := range opts.Roles {
				if msg.Role == interfaces.MessageRole(role) {
					filtered = append(filtered, msg)
					break
				}
			}
		}
		messages = filtered
	}

	// Apply limit if specified
	if opts.Limit > 0 && opts.Limit < len(messages) {
		messages = messages[len(messages)-opts.Limit:]
	}

	return messages, nil
}

// Clear clears the buffer for a conversation
func (c *ConversationBuffer) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Get conversation ID from context
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return err
	}

	// Clear messages for conversation
	delete(c.messages, conversationID)

	return nil
}

// Helper function to get conversation ID from context
func getConversationID(ctx context.Context) (string, error) {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return "", fmt.Errorf("organization ID not found in context: %w", err)
	}

	// Get conversation ID from context
	conversationID, ok := GetConversationID(ctx)
	if !ok {
		return "", fmt.Errorf("conversation ID not found in context")
	}

	// Combine organization ID and conversation ID
	return fmt.Sprintf("%s:%s", orgID, conversationID), nil
}

// GetAllConversations returns all conversation IDs
func (c *ConversationBuffer) GetAllConversations(ctx context.Context) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get organization ID from context to filter conversations
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return nil, fmt.Errorf("organization ID not found in context: %w", err)
	}

	var conversations []string
	orgPrefix := orgID + ":"

	for conversationID := range c.messages {
		// Only return conversations for the current organization
		if strings.HasPrefix(conversationID, orgPrefix) {
			// Remove the org prefix to return clean conversation ID
			cleanID := strings.TrimPrefix(conversationID, orgPrefix)
			conversations = append(conversations, cleanID)
		}
	}

	return conversations, nil
}

// GetConversationMessages gets all messages for a specific conversation
func (c *ConversationBuffer) GetConversationMessages(ctx context.Context, conversationID string) ([]interfaces.Message, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return nil, fmt.Errorf("organization ID not found in context: %w", err)
	}

	// Build full conversation ID with org prefix
	fullConversationID := fmt.Sprintf("%s:%s", orgID, conversationID)

	messages, ok := c.messages[fullConversationID]
	if !ok {
		return []interfaces.Message{}, nil
	}

	return messages, nil
}

// GetMemoryStatistics returns basic memory statistics
func (c *ConversationBuffer) GetMemoryStatistics(ctx context.Context) (totalConversations, totalMessages int, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get organization ID from context to filter conversations
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("organization ID not found in context: %w", err)
	}

	orgPrefix := orgID + ":"
	totalConversations = 0
	totalMessages = 0

	for conversationID, messages := range c.messages {
		// Only count conversations for the current organization
		if strings.HasPrefix(conversationID, orgPrefix) {
			totalConversations++
			totalMessages += len(messages)
		}
	}

	return totalConversations, totalMessages, nil
}

// GetAllConversationsAcrossOrgs returns all conversation IDs from all organizations
func (c *ConversationBuffer) GetAllConversationsAcrossOrgs() (map[string][]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	orgConversations := make(map[string][]string)

	for fullConversationID := range c.messages {
		// Extract orgID and conversationID from the full ID (format: "orgID:conversationID")
		parts := strings.SplitN(fullConversationID, ":", 2)
		if len(parts) == 2 {
			orgID := parts[0]
			conversationID := parts[1]
			orgConversations[orgID] = append(orgConversations[orgID], conversationID)
		}
	}

	return orgConversations, nil
}

// GetConversationMessagesAcrossOrgs finds conversation in any org and returns messages
func (c *ConversationBuffer) GetConversationMessagesAcrossOrgs(conversationID string) ([]interfaces.Message, string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Search for conversation across all orgs
	for fullConversationID, messages := range c.messages {
		// Extract orgID and conversationID from the full ID (format: "orgID:conversationID")
		parts := strings.SplitN(fullConversationID, ":", 2)
		if len(parts) == 2 {
			orgID := parts[0]
			convID := parts[1]

			// Check if this is the conversation we're looking for
			if convID == conversationID {
				return messages, orgID, nil
			}
		}
	}

	// Conversation not found
	return []interfaces.Message{}, "", nil
}

// GetMemoryStatisticsAcrossOrgs returns memory statistics across all organizations
func (c *ConversationBuffer) GetMemoryStatisticsAcrossOrgs() (totalConversations, totalMessages int, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalConversations = len(c.messages)
	totalMessages = 0

	for _, messages := range c.messages {
		totalMessages += len(messages)
	}

	return totalConversations, totalMessages, nil
}
