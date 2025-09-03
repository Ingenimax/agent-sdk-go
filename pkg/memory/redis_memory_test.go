package memory

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// MockLLM is a mock implementation of the LLM interface
type MockLLM struct {
	mock.Mock
}

func (m *MockLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	args := m.Called(ctx, prompt, options)
	return args.String(0), args.Error(1)
}

func (m *MockLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	args := m.Called(ctx, prompt, tools, options)
	return args.String(0), args.Error(1)
}

func (m *MockLLM) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockLLM) SupportsStreaming() bool {
	return false
}

func setupTestRedisClient(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	// Create a miniredis server
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to create miniredis: %v", err)
	}

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return client, mr
}

func TestRedisMemoryWithSummarization(t *testing.T) {
	// Setup
	client, mr := setupTestRedisClient(t)
	defer mr.Close()

	// Create mock LLM
	mockLLM := new(MockLLM)

	// Create Redis memory with summarization
	memory := NewRedisMemory(
		client,
		WithSummarization(mockLLM, 5, 2), // Summarize after 5 messages, keep 2 summaries
		WithTTL(1*time.Hour),
	)

	// Create context
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "test-org")
	ctx = WithConversationID(ctx, "test-conversation")

	// Test 1: Add messages below threshold (shouldn't trigger summarization)
	t.Run("BelowThreshold", func(t *testing.T) {
		// Clear memory first
		err := memory.Clear(ctx)
		assert.NoError(t, err)

		// Add 4 messages (below threshold of 5)
		for i := 0; i < 4; i++ {
			msg := interfaces.Message{
				Role:    "user",
				Content: "Test message " + string(rune('A'+i)),
			}
			err := memory.AddMessage(ctx, msg)
			assert.NoError(t, err)
		}

		// Get messages
		messages, err := memory.GetMessages(ctx)
		assert.NoError(t, err)
		assert.Len(t, messages, 4)

		// Verify no summaries were created
		for _, msg := range messages {
			assert.NotEqual(t, "system", msg.Role)
		}
	})

	// Test 2: Add messages to trigger summarization
	t.Run("TriggerSummarization", func(t *testing.T) {
		// Clear memory first
		err := memory.Clear(ctx)
		assert.NoError(t, err)

		// Setup mock to return a summary
		mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything).
			Return("This is a summary of the conversation.", nil).Once()

		// Add 6 messages (above threshold of 5)
		for i := 0; i < 6; i++ {
			msg := interfaces.Message{
				Role:    "user",
				Content: "Test message " + string(rune('A'+i)),
			}
			err := memory.AddMessage(ctx, msg)
			assert.NoError(t, err)
		}

		// Get messages
		messages, err := memory.GetMessages(ctx)
		assert.NoError(t, err)

		// Should have 1 summary + remaining messages
		summaryCount := 0
		for _, msg := range messages {
			if msg.Metadata != nil {
				if isSummary, ok := msg.Metadata["is_summary"].(bool); ok && isSummary {
					summaryCount++
					assert.Contains(t, msg.Content, "Previous conversation summary")
				}
			}
		}
		assert.Equal(t, 1, summaryCount)

		// Verify mock was called
		mockLLM.AssertCalled(t, "Generate", mock.Anything, mock.Anything, mock.Anything)
	})

	// Test 3: Multiple summarizations
	t.Run("MultipleSummarizations", func(t *testing.T) {
		// Clear memory first
		err := memory.Clear(ctx)
		assert.NoError(t, err)

		// Setup mock to return summaries
		mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything).
			Return("Summary of conversation.", nil)

		// Add many messages to trigger multiple summarizations
		for i := 0; i < 20; i++ {
			msg := interfaces.Message{
				Role:    "user",
				Content: "Test message " + string(rune('A'+i)),
			}
			err := memory.AddMessage(ctx, msg)
			assert.NoError(t, err)
		}

		// Get messages
		messages, err := memory.GetMessages(ctx)
		assert.NoError(t, err)

		// Count summaries
		summaryCount := 0
		for _, msg := range messages {
			if msg.Metadata != nil {
				if isSummary, ok := msg.Metadata["is_summary"].(bool); ok && isSummary {
					summaryCount++
				}
			}
		}

		// Should have at most 2 summaries (based on summaryCount setting)
		assert.LessOrEqual(t, summaryCount, 2)
	})

	// Test 4: Clear with summaries
	t.Run("ClearWithSummaries", func(t *testing.T) {
		// Setup mock
		mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything).
			Return("Summary of conversation.", nil).Maybe()

		// Add messages to create summaries
		for i := 0; i < 10; i++ {
			msg := interfaces.Message{
				Role:    "user",
				Content: "Test message for clear " + string(rune('A'+i)),
			}
			err := memory.AddMessage(ctx, msg)
			assert.NoError(t, err)
		}

		// Clear memory
		err := memory.Clear(ctx)
		assert.NoError(t, err)

		// Verify all messages and summaries are cleared
		messages, err := memory.GetMessages(ctx)
		assert.NoError(t, err)
		assert.Len(t, messages, 0)
	})
}

func TestRedisMemoryOptions(t *testing.T) {
	// Setup
	client, mr := setupTestRedisClient(t)
	defer mr.Close()

	t.Run("DefaultOptions", func(t *testing.T) {
		memory := NewRedisMemory(client)

		assert.Equal(t, 24*time.Hour, memory.ttl)
		assert.Equal(t, "agent:memory:", memory.keyPrefix)
		assert.False(t, memory.summarizationEnabled)
		assert.Equal(t, 50, memory.messageThreshold)
		assert.Equal(t, 5, memory.summaryCount)
	})

	t.Run("CustomOptions", func(t *testing.T) {
		mockLLM := new(MockLLM)
		memory := NewRedisMemory(
			client,
			WithTTL(2*time.Hour),
			WithKeyPrefix("custom:"),
			WithCompression(true),
			WithMaxMessageSize(2048),
			WithSummarization(mockLLM, 20, 3),
		)

		assert.Equal(t, 2*time.Hour, memory.ttl)
		assert.Equal(t, "custom:", memory.keyPrefix)
		assert.True(t, memory.compressionEnabled)
		assert.Equal(t, 2048, memory.maxMessageSize)
		assert.True(t, memory.summarizationEnabled)
		assert.Equal(t, 20, memory.messageThreshold)
		assert.Equal(t, 3, memory.summaryCount)
		assert.Equal(t, "custom:summary:", memory.summaryKeyPrefix)
	})
}
