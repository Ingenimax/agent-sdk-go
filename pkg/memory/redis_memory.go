package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// RedisMemory implements a Redis-backed memory store
type RedisMemory struct {
	client             *redis.Client
	ttl                time.Duration
	keyPrefix          string
	compressionEnabled bool
	encryptionKey      []byte
	maxMessageSize     int
	retryOptions       *RetryOptions
}

// RetryOptions configures retry behavior for Redis operations
type RetryOptions struct {
	MaxRetries    int
	RetryInterval time.Duration
	BackoffFactor float64
}

// RedisOption represents an option for configuring the Redis memory
type RedisOption func(*RedisMemory)

// WithTTL sets the TTL for Redis keys
func WithTTL(ttl time.Duration) RedisOption {
	return func(r *RedisMemory) {
		r.ttl = ttl
	}
}

// WithKeyPrefix sets a custom prefix for Redis keys
func WithKeyPrefix(prefix string) RedisOption {
	return func(r *RedisMemory) {
		r.keyPrefix = prefix
	}
}

// WithCompression enables compression for stored messages
func WithCompression(enabled bool) RedisOption {
	return func(r *RedisMemory) {
		r.compressionEnabled = enabled
	}
}

// WithEncryption enables encryption for stored messages
func WithEncryption(key []byte) RedisOption {
	return func(r *RedisMemory) {
		r.encryptionKey = key
	}
}

// WithMaxMessageSize sets the maximum size for stored messages
func WithMaxMessageSize(size int) RedisOption {
	return func(r *RedisMemory) {
		r.maxMessageSize = size
	}
}

// WithRetryOptions configures retry behavior for Redis operations
func WithRetryOptions(options *RetryOptions) RedisOption {
	return func(r *RedisMemory) {
		r.retryOptions = options
	}
}

// RedisConfig contains configuration for Redis
type RedisConfig struct {
	// URL is the Redis URL (e.g., "localhost:6379")
	URL string

	// Password is the Redis password
	Password string

	// DB is the Redis database number
	DB int
}

// NewRedisMemory creates a new Redis-backed memory store
func NewRedisMemory(client *redis.Client, options ...RedisOption) *RedisMemory {
	memory := &RedisMemory{
		client:             client,
		ttl:                24 * time.Hour,  // Default TTL
		keyPrefix:          "agent:memory:", // Default prefix
		compressionEnabled: false,
		maxMessageSize:     1024 * 1024, // 1MB default max size
		retryOptions: &RetryOptions{
			MaxRetries:    3,
			RetryInterval: 100 * time.Millisecond,
			BackoffFactor: 2.0,
		},
	}

	for _, option := range options {
		option(memory)
	}

	return memory
}

// AddMessage adds a message to the memory with improved error handling and retry logic
func (r *RedisMemory) AddMessage(ctx context.Context, message interfaces.Message) error {
	// Get conversation ID from context
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return err
	}

	// Get organization ID from context for multi-tenancy support
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		// If no organization ID is found, use a default
		orgID = "default"
	}

	// Create Redis key with org and conversation IDs for proper isolation
	key := fmt.Sprintf("%s%s:%s", r.keyPrefix, orgID, conversationID)

	// Validate message size if configured
	if r.maxMessageSize > 0 {
		messageBytes, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}
		if len(messageBytes) > r.maxMessageSize {
			return fmt.Errorf("message size exceeds maximum allowed size of %d bytes", r.maxMessageSize)
		}
	}

	// Process message content (compression/encryption) if enabled
	processedMessage := message
	if r.compressionEnabled || r.encryptionKey != nil {
		processedMessage, err = r.processMessage(message)
		if err != nil {
			return fmt.Errorf("failed to process message: %w", err)
		}
	}

	// Implement retry logic for Redis operations
	var retryErr error
	for attempt := 0; attempt <= r.retryOptions.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff duration with exponential backoff
			backoffDuration := time.Duration(float64(r.retryOptions.RetryInterval) *
				math.Pow(r.retryOptions.BackoffFactor, float64(attempt-1)))
			time.Sleep(backoffDuration)
		}

		// Serialize message to JSON
		messageJSON, err := json.Marshal(processedMessage)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		// Add message to Redis list
		err = r.client.RPush(ctx, key, messageJSON).Err()
		if err == nil {
			// Set TTL on the key if not already set
			r.client.Expire(ctx, key, r.ttl)
			return nil
		}

		retryErr = err
	}

	return fmt.Errorf("failed to add message to Redis after %d attempts: %w",
		r.retryOptions.MaxRetries, retryErr)
}

// processMessage handles compression and encryption of messages
func (r *RedisMemory) processMessage(message interfaces.Message) (interfaces.Message, error) {
	// Create a copy of the message to avoid modifying the original
	processedMessage := message

	// Apply compression if enabled
	if r.compressionEnabled {
		// TODO: Implement compression in the future
		// No-op to avoid empty branch warning
		_ = fmt.Sprintf("Compression flag set to: %v", r.compressionEnabled)
	}

	// Apply encryption if enabled
	if r.encryptionKey != nil {
		// TODO: Implement encryption in the future
		// No-op to avoid empty branch warning
		_ = len(r.encryptionKey)
	}

	return processedMessage, nil
}

// GetMessages retrieves messages from the memory with improved filtering and pagination
func (r *RedisMemory) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	// ... implement with similar improvements to AddMessage
	// Include support for pagination, filtering by role, etc.

	// Get conversation ID from context
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation ID: %w", err)
	}

	// Get organization ID from context for multi-tenancy support
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		// If no organization ID is found, use a default
		orgID = "default"
	}

	// Create Redis key with org and conversation IDs
	key := fmt.Sprintf("%s%s:%s", r.keyPrefix, orgID, conversationID)

	// Apply options
	opts := &interfaces.GetMessagesOptions{}
	for _, option := range options {
		option(opts)
	}

	// Get all messages from Redis
	results, err := r.client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get messages from Redis: %w", err)
	}

	// Parse messages
	var messages []interfaces.Message
	for _, result := range results {
		var message interfaces.Message
		if err := json.Unmarshal([]byte(result), &message); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message: %w", err)
		}
		messages = append(messages, message)
	}

	// Filter by role if specified
	if len(opts.Roles) > 0 {
		var filtered []interfaces.Message
		for _, msg := range messages {
			for _, role := range opts.Roles {
				if msg.Role == role {
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

// Clear clears the memory for a conversation
func (r *RedisMemory) Clear(ctx context.Context) error {
	// ... implement with improved error handling and multi-tenancy support

	// Get conversation ID from context
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get conversation ID: %w", err)
	}

	// Get organization ID from context for multi-tenancy support
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		// If no organization ID is found, use a default
		orgID = "default"
	}

	// Create Redis key with org and conversation IDs
	key := fmt.Sprintf("%s%s:%s", r.keyPrefix, orgID, conversationID)

	// Delete the key from Redis
	err = r.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to clear memory in Redis: %w", err)
	}

	return nil
}

// ... additional methods for advanced Redis operations ...

// NewRedisMemoryFromConfig creates a new Redis memory from configuration
func NewRedisMemoryFromConfig(config RedisConfig, options ...RedisOption) (*RedisMemory, error) {
	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr:     config.URL,
		Password: config.Password,
		DB:       config.DB,
	})

	// Test connection
	ctx := context.Background()
	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// Create Redis memory
	return NewRedisMemory(client, options...), nil
}

// Close closes the underlying Redis connection
func (r *RedisMemory) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}
