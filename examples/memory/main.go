package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func main() {
	// Create a logger
	logger := logging.New()

	cfg := config.Get()
	// Create context with organization ID and conversation ID
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "example-org")
	ctx = memory.WithConversationID(ctx, "conversation-123")

	// Example 1: Conversation Buffer Memory
	logger.Info(ctx, "=== Conversation Buffer Memory ===", nil)
	bufferMemory := memory.NewConversationBuffer(
		memory.WithMaxSize(5),
	)
	testMemory(ctx, bufferMemory, logger)

	// Example 2: Conversation Summary Memory
	logger.Info(ctx, "\n=== Conversation Summary Memory ===", nil)

	llmClient := openai.NewClient(cfg.LLM.OpenAI.APIKey,
		openai.WithModel(cfg.LLM.OpenAI.Model),
		openai.WithLogger(logger),
	)

	summaryMemory := memory.NewConversationSummary(
		llmClient,
		memory.WithMaxBufferSize(3),
	)
	testMemory(ctx, summaryMemory, logger)

	// Example 3: Vector Store Retriever Memory
	logger.Info(ctx, "\n=== Vector Store Retriever Memory ===", nil)
	vectorStore, err := setupVectorStore(logger)
	if err != nil {
		logger.Info(ctx, "Skipping vector store example", map[string]interface{}{"error": err.Error()})
	} else {
		retrieverMemory := memory.NewVectorStoreRetriever(vectorStore)
		testMemory(ctx, retrieverMemory, logger)
	}

	// Example 4: Redis Memory
	logger.Info(ctx, "\n=== Redis Memory ===", nil)
	redisClient, err := setupRedisClient()
	if err != nil {
		logger.Info(ctx, "Skipping Redis example", map[string]interface{}{"error": err.Error()})
	} else {
		redisMemory := memory.NewRedisMemory(
			redisClient,
			memory.WithTTL(1*time.Hour),
		)
		testMemory(ctx, redisMemory, logger)

		// Example 5: Redis Memory with Summarization
		logger.Info(ctx, "\n=== Redis Memory with Summarization ===", nil)

		// Create a new conversation context for summarization example
		summaryCtx := memory.WithConversationID(ctx, "conversation-summarization-demo")

		// Create Redis memory with summarization enabled
		redisMemoryWithSummary := memory.NewRedisMemory(
			redisClient,
			memory.WithTTL(1*time.Hour),
			memory.WithSummarization(llmClient, 10, 3), // Summarize after 10 messages, keep 3 summaries
		)

		testMemoryWithSummarization(summaryCtx, redisMemoryWithSummary, logger)

		// Close Redis client
		if err := redisClient.Close(); err != nil {
			logger.Error(ctx, "Error closing Redis client", map[string]interface{}{"error": err.Error()})
		}
	}
}

func testMemory(ctx context.Context, mem interfaces.Memory, logger logging.Logger) {
	// Add messages
	messages := []interfaces.Message{
		{
			Role:    "system",
			Content: "You are a helpful assistant.",
			Metadata: map[string]interface{}{
				"timestamp": time.Now().UnixNano(),
			},
		},
		{
			Role:    "user",
			Content: "Hello, how are you?",
			Metadata: map[string]interface{}{
				"timestamp": time.Now().UnixNano(),
			},
		},
		{
			Role:    "assistant",
			Content: "I'm doing well, thank you for asking! How can I help you today?",
			Metadata: map[string]interface{}{
				"timestamp": time.Now().UnixNano(),
			},
		},
		{
			Role:    "user",
			Content: "Tell me about the weather.",
			Metadata: map[string]interface{}{
				"timestamp": time.Now().UnixNano(),
			},
		},
	}

	for _, msg := range messages {
		if err := mem.AddMessage(ctx, msg); err != nil {
			logger.Error(ctx, "Failed to add message", map[string]interface{}{"error": err.Error()})
			return
		}
	}

	// Get all messages
	allMessages, err := mem.GetMessages(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to get messages", map[string]interface{}{"error": err.Error()})
		return
	}

	logger.Info(ctx, "All messages:", nil)
	for i, msg := range allMessages {
		logger.Info(ctx, fmt.Sprintf("%d. %s: %s", i+1, msg.Role, msg.Content), nil)
	}

	// Get user messages only
	userMessages, err := mem.GetMessages(ctx, interfaces.WithRoles("user"))
	if err != nil {
		logger.Error(ctx, "Failed to get user messages", map[string]interface{}{"error": err.Error()})
		return
	}

	logger.Info(ctx, "User messages only:", nil)
	for i, msg := range userMessages {
		logger.Info(ctx, fmt.Sprintf("%d. %s: %s", i+1, msg.Role, msg.Content), nil)
	}

	// Get last 2 messages
	lastMessages, err := mem.GetMessages(ctx, interfaces.WithLimit(2))
	if err != nil {
		logger.Error(ctx, "Failed to get last messages", map[string]interface{}{"error": err.Error()})
		return
	}

	logger.Info(ctx, "Last 2 messages:", nil)
	for i, msg := range lastMessages {
		logger.Info(ctx, fmt.Sprintf("%d. %s: %s", i+1, msg.Role, msg.Content), nil)
	}

	// Clear memory
	if err := mem.Clear(ctx); err != nil {
		logger.Error(ctx, "Failed to clear memory", map[string]interface{}{"error": err.Error()})
		return
	}

	// Verify memory is cleared
	clearedMessages, err := mem.GetMessages(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to get messages after clearing", map[string]interface{}{"error": err.Error()})
		return
	}

	logger.Info(ctx, "After clearing:", nil)
	if len(clearedMessages) == 0 {
		logger.Info(ctx, "Memory cleared successfully", nil)
	} else {
		logger.Info(ctx, fmt.Sprintf("Memory not cleared, %d messages remaining", len(clearedMessages)), nil)
	}
}

func setupVectorStore(logger logging.Logger) (interfaces.VectorStore, error) {
	// Check if we have the necessary environment variables
	// This is a placeholder - in a real application, you would
	// configure and return a real vector store

	// Log that we're using a placeholder implementation
	logger.Info(context.Background(), "Vector store setup is a placeholder implementation", nil)

	// For example, to use a simple in-memory vector store:
	// return vectorstore.NewInMemory(), nil

	return nil, fmt.Errorf("vector store setup not implemented - skipping example")
}

func setupRedisClient() (*redis.Client, error) {
	// Check if Redis is available
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379" // Default Redis address
	}

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	// Test connection
	_, err := client.Ping(context.Background()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis at %s: %w", redisAddr, err)
	}

	return client, nil
}

func testMemoryWithSummarization(ctx context.Context, mem interfaces.Memory, logger logging.Logger) {
	logger.Info(ctx, "Testing Redis memory with automatic summarization", nil)

	// Clear any existing messages first
	if err := mem.Clear(ctx); err != nil {
		logger.Error(ctx, "Failed to clear memory", map[string]interface{}{"error": err.Error()})
		return
	}

	// Add many messages to trigger summarization
	for i := 0; i < 15; i++ {
		userMsg := interfaces.Message{
			Role:    "user",
			Content: fmt.Sprintf("Question %d: Tell me something interesting about topic %d", i+1, i+1),
			Metadata: map[string]interface{}{
				"timestamp": time.Now().UnixNano(),
				"index":     i,
			},
		}

		assistantMsg := interfaces.Message{
			Role:    "assistant",
			Content: fmt.Sprintf("Response %d: Here's an interesting fact about topic %d - it's quite fascinating!", i+1, i+1),
			Metadata: map[string]interface{}{
				"timestamp": time.Now().UnixNano(),
				"index":     i,
			},
		}

		// Add user message
		if err := mem.AddMessage(ctx, userMsg); err != nil {
			logger.Error(ctx, "Failed to add user message", map[string]interface{}{
				"error": err.Error(),
				"index": i,
			})
			return
		}

		// Add assistant message
		if err := mem.AddMessage(ctx, assistantMsg); err != nil {
			logger.Error(ctx, "Failed to add assistant message", map[string]interface{}{
				"error": err.Error(),
				"index": i,
			})
			return
		}

		// Log progress
		if (i+1)%5 == 0 {
			logger.Info(ctx, fmt.Sprintf("Added %d message pairs", i+1), nil)
		}
	}

	// Get all messages (should include summaries + recent messages)
	allMessages, err := mem.GetMessages(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to get messages", map[string]interface{}{"error": err.Error()})
		return
	}

	logger.Info(ctx, fmt.Sprintf("Total messages retrieved: %d", len(allMessages)), nil)

	// Check for summaries
	summaryCount := 0
	regularCount := 0
	for _, msg := range allMessages {
		if msg.Metadata != nil {
			if isSummary, ok := msg.Metadata["is_summary"].(bool); ok && isSummary {
				summaryCount++
				logger.Info(ctx, fmt.Sprintf("Summary found: %s", msg.Content), nil)
			} else {
				regularCount++
			}
		} else {
			regularCount++
		}
	}

	logger.Info(ctx, fmt.Sprintf("Summaries: %d, Regular messages: %d", summaryCount, regularCount), nil)

	// Show the last few messages to verify they're recent
	logger.Info(ctx, "Last 3 messages:", nil)
	start := len(allMessages) - 3
	if start < 0 {
		start = 0
	}
	for i := start; i < len(allMessages); i++ {
		msg := allMessages[i]
		logger.Info(ctx, fmt.Sprintf("%d. %s: %s", i+1, msg.Role, msg.Content), nil)
	}
}
