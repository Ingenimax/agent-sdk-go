package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// VectorStoreRetriever implements a memory that stores messages in a vector store
type VectorStoreRetriever struct {
	buffer      *ConversationBuffer
	vectorStore interfaces.VectorStore
	mu          sync.RWMutex
}

// RetrieverOption represents an option for configuring the vector store retriever
type RetrieverOption func(*VectorStoreRetriever)

// NewVectorStoreRetriever creates a new vector store retriever memory
func NewVectorStoreRetriever(vectorStore interfaces.VectorStore, options ...RetrieverOption) *VectorStoreRetriever {
	retriever := &VectorStoreRetriever{
		buffer:      NewConversationBuffer(),
		vectorStore: vectorStore,
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

	// If no query is provided, return messages from buffer
	if opts.Query == "" {
		return v.buffer.GetMessages(ctx, options...)
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
