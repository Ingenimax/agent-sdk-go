package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/go-redis/redis/v8"
)

// MemoryFactory provides factory functions to create memory instances from configuration
type MemoryFactory struct{}

// NewMemoryFactory creates a new memory factory
func NewMemoryFactory() *MemoryFactory {
	return &MemoryFactory{}
}

// CreateMemory creates a memory instance from configuration map
func (f *MemoryFactory) CreateMemory(config map[string]interface{}, llmClient interfaces.LLM) (interfaces.Memory, error) {
	if config == nil {
		return nil, fmt.Errorf("memory config cannot be nil")
	}

	memoryType, ok := config["type"].(string)
	if !ok {
		return nil, fmt.Errorf("memory type not specified or not a string")
	}

	switch memoryType {
	case "redis":
		return f.createRedisMemory(config, llmClient)
	case "buffer":
		return f.createBufferMemory(config, llmClient)
	case "vector":
		return f.createVectorMemory(config, llmClient)
	default:
		return nil, fmt.Errorf("unsupported memory type: %s", memoryType)
	}
}

// createRedisMemory creates a Redis memory instance from configuration
func (f *MemoryFactory) createRedisMemory(config map[string]interface{}, llmClient interfaces.LLM) (*RedisMemory, error) {
	// Extract Redis configuration
	address, ok := config["address"].(string)
	if !ok || address == "" {
		return nil, fmt.Errorf("redis address not specified or empty")
	}

	// Optional fields with defaults
	password, _ := config["password"].(string)
	db := 0
	if dbVal, ok := config["db"]; ok {
		switch v := dbVal.(type) {
		case int:
			db = v
		case float64:
			db = int(v)
		}
	}

	// TTL configuration
	ttlHours := 24 // default
	if ttlVal, ok := config["ttl_hours"]; ok {
		switch v := ttlVal.(type) {
		case int:
			ttlHours = v
		case float64:
			ttlHours = int(v)
		}
	}

	// Key prefix
	keyPrefix, ok := config["key_prefix"].(string)
	if !ok {
		keyPrefix = "agent:" // default prefix
	}

	// Max message size
	maxMessageSize := 1048576 // 1MB default
	if maxMsgVal, ok := config["max_message_size"]; ok {
		switch v := maxMsgVal.(type) {
		case int:
			maxMessageSize = v
		case float64:
			maxMessageSize = int(v)
		}
	}

	// Create Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       db,
	})

	// Test Redis connection
	ctx := context.Background()
	if _, err := redisClient.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis at %s: %w", address, err)
	}

	// Calculate TTL duration
	ttlDuration := time.Duration(ttlHours) * time.Hour

	// Create Redis memory with configuration options
	redisMemory := NewRedisMemory(
		redisClient,
		WithTTL(ttlDuration),
		WithKeyPrefix(keyPrefix),
		WithMaxMessageSize(maxMessageSize),
		WithRetryOptions(&RetryOptions{
			MaxRetries:    3,
			RetryInterval: 100 * time.Millisecond,
			BackoffFactor: 2.0,
		}),
	)

	// Add summarization if LLM is available
	if llmClient != nil {
		// Check for summarization config
		maxSummaries := 3
		summaryAfterMessages := 10

		if maxSum, ok := config["max_summaries"]; ok {
			switch v := maxSum.(type) {
			case int:
				maxSummaries = v
			case float64:
				maxSummaries = int(v)
			}
		}

		if sumAfter, ok := config["summary_after_messages"]; ok {
			switch v := sumAfter.(type) {
			case int:
				summaryAfterMessages = v
			case float64:
				summaryAfterMessages = int(v)
			}
		}

		redisMemory = NewRedisMemory(
			redisClient,
			WithTTL(ttlDuration),
			WithKeyPrefix(keyPrefix),
			WithMaxMessageSize(maxMessageSize),
			WithRetryOptions(&RetryOptions{
				MaxRetries:    3,
				RetryInterval: 100 * time.Millisecond,
				BackoffFactor: 2.0,
			}),
			WithSummarization(llmClient, maxSummaries, summaryAfterMessages),
		)
	}

	return redisMemory, nil
}

// createBufferMemory creates a buffer memory instance from configuration
func (f *MemoryFactory) createBufferMemory(config map[string]interface{}, llmClient interfaces.LLM) (interfaces.Memory, error) {
	// Buffer memory configuration
	bufferSize := 1000 // default
	if bufferSizeVal, ok := config["buffer_size"]; ok {
		switch v := bufferSizeVal.(type) {
		case int:
			bufferSize = v
		case float64:
			bufferSize = int(v)
		}
	}

	// Create buffer memory with options
	bufferOptions := []Option{WithMaxSize(bufferSize)}
	bufferMemory := NewConversationBuffer(bufferOptions...)

	// Add summarization if LLM is available and configured
	if llmClient != nil {
		if summaryEnabled, ok := config["enable_summarization"].(bool); ok && summaryEnabled {
			maxBufferSize := bufferSize
			summaryLength := 100 // default summary length

			if maxBuf, ok := config["max_buffer_size"]; ok {
				switch v := maxBuf.(type) {
				case int:
					maxBufferSize = v
				case float64:
					maxBufferSize = int(v)
				}
			}

			if sumLen, ok := config["summary_length"]; ok {
				switch v := sumLen.(type) {
				case int:
					summaryLength = v
				case float64:
					summaryLength = int(v)
				}
			}

			// Create summary options
			summaryOptions := []SummaryOption{
				WithMaxBufferSize(maxBufferSize),
				WithSummaryLength(summaryLength),
			}

			// Wrap with summary memory if enabled
			return NewConversationSummary(llmClient, summaryOptions...), nil
		}
	}

	return bufferMemory, nil
}

// createVectorMemory creates a vector memory instance from configuration
func (f *MemoryFactory) createVectorMemory(config map[string]interface{}, llmClient interfaces.LLM) (interfaces.Memory, error) {
	// Vector memory would require additional dependencies and configuration
	// This is a placeholder for future implementation
	return nil, fmt.Errorf("vector memory not yet implemented")
}

// NewMemoryFromConfig is a convenience function to create memory from config map
func NewMemoryFromConfig(config map[string]interface{}, llmClient interfaces.LLM) (interfaces.Memory, error) {
	factory := NewMemoryFactory()
	return factory.CreateMemory(config, llmClient)
}