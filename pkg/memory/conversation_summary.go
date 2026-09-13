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
		llmClient:       llmClient,
		maxBufferSize:   10, // Default max buffer size
		summaryMessages: make(map[string]interfaces.Message),
		summaryParams:   make(map[string]interface{}),
	}

	for _, option := range options {
		option(summary)
	}

	if summary.maxBufferSize < 1 {
		summary.maxBufferSize = 1
	}

	// The inner buffer is built after the options are applied and sized to
	// maxBufferSize. It used to be constructed with a bare NewConversationBuffer(),
	// which hardcodes maxSize 100 and trims from the front on every add, so any
	// caller passing WithMaxBufferSize above 100 got a summarizer whose trigger
	// (len(messages) >= maxBufferSize) could never be reached. Summarization was
	// silently disabled with no error and no warning.
	summary.buffer = NewConversationBuffer(WithMaxSize(summary.maxBufferSize))

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
		// Fold any existing summary into the new one. Previously the new
		// summary simply replaced the old via a map assignment, and summarize()
		// only ever saw the current buffer -- so on every second and subsequent
		// summarization the entire earlier history was destroyed, silently and
		// irrecoverably, since the raw messages had already been cleared from
		// the only store. Passing the prior summary in makes the record compound
		// instead.
		prior := c.summaryMessages[conversationID].Content

		summary, err := c.summarize(ctx, prior, messages)
		if err != nil {
			return err
		}

		// Track how many raw messages this summary now stands for, across all
		// the rounds folded into it.
		covered := len(messages)
		if existing, ok := c.summaryMessages[conversationID]; ok {
			if previousCount, ok := existing.Metadata["count"].(int); ok {
				covered += previousCount
			}
		}

		// Store summary
		c.summaryMessages[conversationID] = interfaces.Message{
			Role:    "system",
			Content: summary,
			Metadata: map[string]interface{}{
				"is_summary": true,
				"count":      covered,
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

// summarize summarizes a list of messages.
//
// prior is the existing summary for this conversation, or "" if there is none.
// It is included in the prompt so that history already compressed into a
// summary survives the next round rather than being discarded.
func (c *ConversationSummary) summarize(ctx context.Context, prior string, messages []interfaces.Message) (string, error) {
	// Format messages for summarization
	var sb strings.Builder

	// Get configured summary length or use default
	summaryLength := 100
	if c.summaryParams != nil {
		if length, ok := c.summaryParams["summaryLength"].(int); ok {
			summaryLength = length
		}
	}

	fmt.Fprintf(&sb, "Summarize the following conversation in a concise summary (about %d words maximum):\n\n", summaryLength)

	if prior != "" {
		fmt.Fprintf(&sb, "Summary of the conversation so far:\n%s\n\nSubsequent messages:\n", prior)
	}

	for _, msg := range messages {
		content := msg.Content
		// Assistant turns that only carry tool calls have empty content;
		// rendering them as a bare role name loses what the agent actually did.
		if content == "" && len(msg.ToolCalls) > 0 {
			names := make([]string, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				names = append(names, tc.Name)
			}
			content = fmt.Sprintf("(called tools: %s)", strings.Join(names, ", "))
		}
		fmt.Fprintf(&sb, "%s: %s\n", msg.Role, content)
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
