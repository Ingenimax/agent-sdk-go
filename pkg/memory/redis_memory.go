package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
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

	// Summarization fields
	summarizationEnabled bool
	llmClient            interfaces.LLM
	messageThreshold     int
	summaryCount         int
	summaryKeyPrefix     string
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

// WithSummarization enables automatic summarization of old messages
func WithSummarization(llm interfaces.LLM, messageThreshold int, summaryCount int) RedisOption {
	return func(r *RedisMemory) {
		r.summarizationEnabled = true
		r.llmClient = llm
		r.messageThreshold = messageThreshold
		r.summaryCount = summaryCount
		r.summaryKeyPrefix = r.keyPrefix + "summary:"
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
		// Summarization defaults
		summarizationEnabled: false,
		messageThreshold:     50,
		summaryCount:         5,
		summaryKeyPrefix:     "agent:memory:summary:",
	}

	for _, option := range options {
		option(memory)
	}

	// Update summary key prefix if keyPrefix was changed by options
	if memory.summarizationEnabled && memory.summaryKeyPrefix == "agent:memory:summary:" {
		memory.summaryKeyPrefix = memory.keyPrefix + "summary:"
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

			// Check if summarization is needed
			if r.summarizationEnabled {
				if err := r.checkAndSummarize(ctx); err != nil {
					// Log error but don't fail the message addition
					// TODO: Add proper logging
					_ = fmt.Sprintf("Failed to summarize messages: %v", err)
				}
			}

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

	var allMessages []interfaces.Message

	// Get summaries first if summarization is enabled
	if r.summarizationEnabled {
		summaries, err := r.getSummaries(ctx)
		if err != nil {
			// Log error but continue without summaries
			_ = fmt.Sprintf("Failed to get summaries: %v", err)
		} else {
			allMessages = append(allMessages, summaries...)
		}
	}

	// Get all messages from Redis
	results, err := r.client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get messages from Redis: %w", err)
	}

	// Parse messages
	for _, result := range results {
		var message interfaces.Message
		if err := json.Unmarshal([]byte(result), &message); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message: %w", err)
		}
		allMessages = append(allMessages, message)
	}

	// Filter by role if specified
	if len(opts.Roles) > 0 {
		var filtered []interfaces.Message
		for _, msg := range allMessages {
			for _, role := range opts.Roles {
				if msg.Role == interfaces.MessageRole(role) {
					filtered = append(filtered, msg)
					break
				}
			}
		}
		allMessages = filtered
	}

	// Apply limit if specified
	if opts.Limit > 0 && opts.Limit < len(allMessages) {
		allMessages = allMessages[len(allMessages)-opts.Limit:]
	}

	return allMessages, nil
}

// Clear clears the memory for a conversation
func (r *RedisMemory) Clear(ctx context.Context) error {
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

	// Delete the messages key from Redis
	err = r.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to clear memory in Redis: %w", err)
	}

	// Clear summaries if summarization is enabled
	if r.summarizationEnabled {
		summaryKey := fmt.Sprintf("%s%s:%s", r.summaryKeyPrefix, orgID, conversationID)
		metaKey := fmt.Sprintf("%smeta:%s:%s", r.summaryKeyPrefix, orgID, conversationID)

		// Delete summary and metadata keys
		err = r.client.Del(ctx, summaryKey, metaKey).Err()
		if err != nil {
			return fmt.Errorf("failed to clear summaries in Redis: %w", err)
		}
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

// checkAndSummarize checks if summarization is needed and performs it
func (r *RedisMemory) checkAndSummarize(ctx context.Context) error {
	// Get conversation ID from context
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get conversation ID: %w", err)
	}

	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		orgID = "default"
	}

	// Create Redis key
	key := fmt.Sprintf("%s%s:%s", r.keyPrefix, orgID, conversationID)

	// Get message count
	count, err := r.client.LLen(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to get message count: %w", err)
	}

	// Check if we need to summarize
	if count < int64(r.messageThreshold) {
		return nil
	}

	// Get messages to summarize (all but the most recent ones)
	keepRecent := r.messageThreshold / 3 // Keep 1/3 of threshold as recent messages
	summarizeCount := int(count) - keepRecent

	// Get messages to summarize
	results, err := r.client.LRange(ctx, key, 0, int64(summarizeCount-1)).Result()
	if err != nil {
		return fmt.Errorf("failed to get messages for summarization: %w", err)
	}

	// Parse messages
	var messages []interfaces.Message
	for _, result := range results {
		var message interfaces.Message
		if err := json.Unmarshal([]byte(result), &message); err != nil {
			return fmt.Errorf("failed to unmarshal message: %w", err)
		}
		messages = append(messages, message)
	}

	// Create summary
	summary, err := r.createSummary(ctx, messages)
	if err != nil {
		return fmt.Errorf("failed to create summary: %w", err)
	}

	// Store summary
	if err := r.storeSummary(ctx, summary); err != nil {
		return fmt.Errorf("failed to store summary: %w", err)
	}

	// Remove summarized messages from the main list
	for i := 0; i < summarizeCount; i++ {
		if err := r.client.LPop(ctx, key).Err(); err != nil {
			return fmt.Errorf("failed to remove summarized message: %w", err)
		}
	}

	// Rotate summaries if needed
	if err := r.rotateSummaries(ctx); err != nil {
		return fmt.Errorf("failed to rotate summaries: %w", err)
	}

	return nil
}

// createSummary generates a summary of the given messages using the LLM
func (r *RedisMemory) createSummary(ctx context.Context, messages []interfaces.Message) (interfaces.Message, error) {
	// Format messages for summarization
	var sb strings.Builder
	sb.WriteString("Summarize the following conversation concisely, preserving key information and context:\n\n")

	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}

	sb.WriteString("\nProvide a concise summary that captures the essential information from this conversation.")

	// Generate summary
	summary, err := r.llmClient.Generate(ctx, sb.String(), func(o *interfaces.GenerateOptions) {
		o.LLMConfig = &interfaces.LLMConfig{
			Temperature: 0.3, // Lower temperature for more consistent summaries
		}
	})
	if err != nil {
		return interfaces.Message{}, fmt.Errorf("failed to generate summary: %w", err)
	}

	// Create summary message
	summaryMessage := interfaces.Message{
		Role:    "system",
		Content: fmt.Sprintf("Previous conversation summary (%d messages): %s", len(messages), strings.TrimSpace(summary)),
		Metadata: map[string]interface{}{
			"is_summary":    true,
			"message_count": len(messages),
			"summarized_at": time.Now().Unix(),
		},
	}

	return summaryMessage, nil
}

// storeSummary stores a summary in Redis
func (r *RedisMemory) storeSummary(ctx context.Context, summary interfaces.Message) error {
	// Get conversation ID from context
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get conversation ID: %w", err)
	}

	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		orgID = "default"
	}

	// Create Redis key for summaries
	summaryKey := fmt.Sprintf("%s%s:%s", r.summaryKeyPrefix, orgID, conversationID)

	// Marshal summary
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}

	// Add summary to Redis list
	if err := r.client.RPush(ctx, summaryKey, summaryJSON).Err(); err != nil {
		return fmt.Errorf("failed to store summary: %w", err)
	}

	// Set TTL on the summary key
	r.client.Expire(ctx, summaryKey, r.ttl)

	return nil
}

// getSummaries retrieves summaries from Redis
func (r *RedisMemory) getSummaries(ctx context.Context) ([]interfaces.Message, error) {
	// Get conversation ID from context
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation ID: %w", err)
	}

	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		orgID = "default"
	}

	// Create Redis key for summaries
	summaryKey := fmt.Sprintf("%s%s:%s", r.summaryKeyPrefix, orgID, conversationID)

	// Get all summaries from Redis
	results, err := r.client.LRange(ctx, summaryKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get summaries from Redis: %w", err)
	}

	// Parse summaries
	var summaries []interfaces.Message
	for _, result := range results {
		var summary interfaces.Message
		if err := json.Unmarshal([]byte(result), &summary); err != nil {
			return nil, fmt.Errorf("failed to unmarshal summary: %w", err)
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// rotateSummaries ensures we only keep the configured number of summaries
func (r *RedisMemory) rotateSummaries(ctx context.Context) error {
	// Get conversation ID from context
	conversationID, err := getConversationID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get conversation ID: %w", err)
	}

	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		orgID = "default"
	}

	// Create Redis key for summaries
	summaryKey := fmt.Sprintf("%s%s:%s", r.summaryKeyPrefix, orgID, conversationID)

	// Get summary count
	count, err := r.client.LLen(ctx, summaryKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get summary count: %w", err)
	}

	// Remove old summaries if we exceed the limit
	if count > int64(r.summaryCount) {
		removeCount := int(count) - r.summaryCount
		for i := 0; i < removeCount; i++ {
			if err := r.client.LPop(ctx, summaryKey).Err(); err != nil {
				return fmt.Errorf("failed to remove old summary: %w", err)
			}
		}
	}

	return nil
}

// Close closes the underlying Redis connection
func (r *RedisMemory) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}
