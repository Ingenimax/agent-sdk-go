package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// ConversationSummary implements a memory that summarizes old messages
type ConversationSummary struct {
	buffer          *ConversationBuffer
	llmClient       interfaces.LLM
	maxBufferSize   int
	summaryMessages map[string]interfaces.Message
	summaryParams   map[string]interface{}
	mu              sync.RWMutex
}

// SummaryOption represents an option for configuring the conversation summary
type SummaryOption func(*ConversationSummary)

// WithMaxBufferSize sets the maximum number of messages before summarizing
func WithMaxBufferSize(size int) SummaryOption {
	return func(c *ConversationSummary) {
		c.maxBufferSize = size
	}
}

// WithSummaryLength sets the maximum word count target for summaries
func WithSummaryLength(wordCount int) SummaryOption {
	return func(c *ConversationSummary) {
		// Store the word count in a new field
		if c.summaryParams == nil {
			c.summaryParams = make(map[string]interface{})
		}
		c.summaryParams["summaryLength"] = wordCount
	}
}

// NewConversationSummary creates a new conversation summary memory
func NewConversationSummary(llmClient interfaces.LLM, options ...SummaryOption) *ConversationSummary {
	summary := &ConversationSummary{
		buffer:          NewConversationBuffer(),
		llmClient:       llmClient,
		maxBufferSize:   10, // Default max buffer size
		summaryMessages: make(map[string]interfaces.Message),
		summaryParams:   make(map[string]interface{}),
	}

	for _, option := range options {
		option(summary)
	}

	return summary
}

// AddMessage adds a message to the memory
func (c *ConversationSummary) AddMessage(ctx context.Context, message interfaces.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Add message to buffer
	if err := c.buffer.AddMessage(ctx, message); err != nil {
		return err
	}

	// Get conversation ID
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return err
	}

	// Check if we need to summarize
	messages, err := c.buffer.GetMessages(ctx)
	if err != nil {
		return err
	}

	if len(messages) >= c.maxBufferSize {
		// Summarize messages
		summary, err := c.summarize(ctx, messages)
		if err != nil {
			return err
		}

		// Store summary
		c.summaryMessages[conversationID] = interfaces.Message{
			Role:    "system",
			Content: summary,
			Metadata: map[string]interface{}{
				"is_summary": true,
				"count":      len(messages),
			},
		}

		// Clear buffer
		if err := c.buffer.Clear(ctx); err != nil {
			return err
		}
	}

	return nil
}

// GetMessages retrieves messages from the memory
func (c *ConversationSummary) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get conversation ID
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return nil, err
	}

	// Get current messages from buffer
	messages, err := c.buffer.GetMessages(ctx, options...)
	if err != nil {
		return nil, err
	}

	// Check if we have a summary
	summary, ok := c.summaryMessages[conversationID]
	if !ok {
		return messages, nil
	}

	// Combine summary and current messages
	result := []interfaces.Message{summary}
	result = append(result, messages...)

	return result, nil
}

// Clear clears the memory
func (c *ConversationSummary) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Get conversation ID
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return err
	}

	// Clear buffer
	if err := c.buffer.Clear(ctx); err != nil {
		return err
	}

	// Clear summary
	delete(c.summaryMessages, conversationID)

	return nil
}

// summarize summarizes a list of messages
func (c *ConversationSummary) summarize(ctx context.Context, messages []interfaces.Message) (string, error) {
	// Format messages for summarization
	var sb strings.Builder

	// Get configured summary length or use default
	summaryLength := 100
	if c.summaryParams != nil {
		if length, ok := c.summaryParams["summaryLength"].(int); ok {
			summaryLength = length
		}
	}

	sb.WriteString(fmt.Sprintf("Summarize the following conversation in a concise summary (about %d words maximum):\n\n", summaryLength))
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}
	sb.WriteString("\nSummary:")

	// Generate summary with default options instead of nil
	summary, err := c.llmClient.Generate(ctx, sb.String(), func(o *interfaces.GenerateOptions) {
		o.LLMConfig.Temperature = 0.7
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	return strings.TrimSpace(summary), nil
}
