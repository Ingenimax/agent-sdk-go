package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// VectorStoreRetriever implements a memory that stores messages in a vector store
type VectorStoreRetriever struct {
	buffer      *ConversationBuffer
	vectorStore interfaces.VectorStore
	autoRecall  bool
	recallLimit int
	mu          sync.RWMutex
}

// RetrieverOption represents an option for configuring the vector store retriever
type RetrieverOption func(*VectorStoreRetriever)

// NewVectorStoreRetriever creates a new vector store retriever memory
func NewVectorStoreRetriever(vectorStore interfaces.VectorStore, options ...RetrieverOption) *VectorStoreRetriever {
	retriever := &VectorStoreRetriever{
		buffer:      NewConversationBuffer(),
		vectorStore: vectorStore,
		recallLimit: 5,
	}

	for _, option := range options {
		option(retriever)
	}

	return retriever
}

// AddMessage adds a message to the memory
func (v *VectorStoreRetriever) AddMessage(ctx context.Context, message interfaces.Message) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Add message to buffer
	if err := v.buffer.AddMessage(ctx, message); err != nil {
		return err
	}

	// Store message in vector store
	doc := interfaces.Document{
		ID:      fmt.Sprintf("%s-%d", message.Role, message.Metadata["timestamp"]),
		Content: message.Content,
		Metadata: map[string]interface{}{
			"role":      message.Role,
			"timestamp": message.Metadata["timestamp"],
		},
	}

	if err := v.vectorStore.Store(ctx, []interfaces.Document{doc}); err != nil {
		return fmt.Errorf("failed to store message in vector store: %w", err)
	}

	return nil
}

// GetMessages retrieves messages from the memory
func (v *VectorStoreRetriever) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Parse options
	opts := &interfaces.GetMessagesOptions{}
	for _, option := range options {
		option(opts)
	}

	// If no query is provided, fall back to the buffer -- optionally enriching it
	// with semantically recalled history first.
	//
	// This gate is why the vector store was effectively write-only. Every
	// provider builds its request with a bare, optionless GetMessages call, and
	// interfaces.WithQuery had no caller anywhere in the module, so opts.Query
	// was always "" and the search below was unreachable. Callers paid to embed
	// and store every message into a store nothing ever read back.
	if opts.Query == "" {
		recent, err := v.buffer.GetMessages(ctx, options...)
		if err != nil {
			return nil, err
		}
		if !v.autoRecall {
			return recent, nil
		}
		return v.withRecalledContext(ctx, recent), nil
	}

	// Search for relevant messages in vector store
	results, err := v.vectorStore.Search(ctx, opts.Query, opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search vector store: %w", err)
	}

	// Convert search results to messages
	var messages []interfaces.Message
	for _, result := range results {
		role, _ := result.Document.Metadata["role"].(string)
		timestamp, _ := result.Document.Metadata["timestamp"].(float64)

		messages = append(messages, interfaces.Message{
			Role:    interfaces.MessageRole(role),
			Content: result.Document.Content,
			Metadata: map[string]interface{}{
				"timestamp": timestamp,
				"score":     result.Score,
			},
		})
	}

	return messages, nil
}

// Clear clears the memory
func (v *VectorStoreRetriever) Clear(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Get conversation ID
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return err
	}

	// Clear buffer
	if err := v.buffer.Clear(ctx); err != nil {
		return err
	}

	// Delete messages from vector store
	// This would require a way to filter by conversation ID
	// For now, we'll just log a warning
	fmt.Printf("Warning: Messages for conversation %s not deleted from vector store\n", conversationID)

	return nil
}

// WithAutoRecall makes GetMessages search the vector store even when the caller
// supplies no explicit query, using the most recent user message as the query.
//
// It is opt-in. Enabling it changes what the model sees, and the safe default
// for an existing deployment is no change -- but note that without it the
// vector store is write-only, since no provider passes interfaces.WithQuery.
func WithAutoRecall() RetrieverOption {
	return func(v *VectorStoreRetriever) {
		v.autoRecall = true
	}
}

// WithRecallLimit sets how many prior messages auto-recall may surface.
// Defaults to 5.
func WithRecallLimit(n int) RetrieverOption {
	return func(v *VectorStoreRetriever) {
		if n > 0 {
			v.recallLimit = n
		}
	}
}

// withRecalledContext prepends semantically relevant history to the recent
// transcript as a single system message.
//
// It deliberately does NOT splice recalled messages into the transcript. The
// conversation carries tool_call / tool_result pairs that both the OpenAI and
// Anthropic APIs reject if separated or reordered, and a similarity search
// returns neither contiguous nor chronological results. Recalled content is
// therefore delivered as context ABOUT the conversation, leaving the real
// message sequence untouched.
//
// Recall failures are not fatal: the caller gets the unmodified transcript.
func (v *VectorStoreRetriever) withRecalledContext(ctx context.Context, recent []interfaces.Message) []interfaces.Message {
	query := lastUserContent(recent)
	if query == "" {
		return recent
	}

	results, err := v.vectorStore.Search(ctx, query, v.recallLimit)
	if err != nil || len(results) == 0 {
		return recent
	}

	// Skip anything already present in the recent window; repeating it wastes
	// context and reads as the model being told the same thing twice.
	present := make(map[string]struct{}, len(recent))
	for _, m := range recent {
		present[m.Content] = struct{}{}
	}

	var recalled []string
	for _, result := range results {
		content := result.Document.Content
		if content == "" {
			continue
		}
		if _, seen := present[content]; seen {
			continue
		}
		role, _ := result.Document.Metadata["role"].(string)
		if role == "" {
			role = "unknown"
		}
		recalled = append(recalled, fmt.Sprintf("- [%s] %s", role, content))
		present[content] = struct{}{}
	}

	if len(recalled) == 0 {
		return recent
	}

	context := interfaces.Message{
		Role: interfaces.MessageRoleSystem,
		Content: "Relevant excerpts from earlier in this conversation:\n" +
			strings.Join(recalled, "\n"),
		Metadata: map[string]interface{}{
			"recalled":     true,
			"recall_count": len(recalled),
		},
	}

	return append([]interfaces.Message{context}, recent...)
}

// lastUserContent returns the most recent user message's content, which is the
// natural recall query: it is what the agent is being asked right now.
func lastUserContent(messages []interfaces.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == interfaces.MessageRoleUser && messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
}
