package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheControl_JSON(t *testing.T) {
	tests := []struct {
		name     string
		control  *CacheControl
		expected string
	}{
		{
			name:     "default cache control",
			control:  NewCacheControl(),
			expected: `{"type":"ephemeral"}`,
		},
		{
			name:     "cache control with 5m TTL",
			control:  NewCacheControlWithTTL("5m"),
			expected: `{"type":"ephemeral"}`, // 5m is default, no TTL field
		},
		{
			name:     "cache control with 1h TTL",
			control:  NewCacheControlWithTTL("1h"),
			expected: `{"type":"ephemeral","ttl":"1h"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.control)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(got))
		})
	}
}

func TestCacheRequestBuilder_HasCacheOptions(t *testing.T) {
	tests := []struct {
		name     string
		config   *interfaces.CacheConfig
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name:     "empty config",
			config:   &interfaces.CacheConfig{},
			expected: false,
		},
		{
			name:     "cache system message",
			config:   &interfaces.CacheConfig{CacheSystemMessage: true},
			expected: true,
		},
		{
			name:     "cache tools",
			config:   &interfaces.CacheConfig{CacheTools: true},
			expected: true,
		},
		{
			name:     "cache conversation",
			config:   &interfaces.CacheConfig{CacheConversation: true},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := newCacheRequestBuilder(tt.config)
			assert.Equal(t, tt.expected, builder.HasCacheOptions())
		})
	}
}

func TestCacheRequestBuilder_BuildSystemContent(t *testing.T) {
	tests := []struct {
		name           string
		config         *interfaces.CacheConfig
		system         string
		expectArray    bool
		expectCacheCtl bool
	}{
		{
			name:           "nil config returns string",
			config:         nil,
			system:         "You are helpful",
			expectArray:    false,
			expectCacheCtl: false,
		},
		{
			name:           "cache disabled returns string",
			config:         &interfaces.CacheConfig{CacheSystemMessage: false},
			system:         "You are helpful",
			expectArray:    false,
			expectCacheCtl: false,
		},
		{
			name:           "cache enabled returns array with cache_control",
			config:         &interfaces.CacheConfig{CacheSystemMessage: true},
			system:         "You are helpful",
			expectArray:    true,
			expectCacheCtl: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := newCacheRequestBuilder(tt.config)
			got, err := builder.BuildSystemContent(tt.system)
			require.NoError(t, err)

			// Check if it's an array (starts with [) or string (starts with ")
			gotStr := string(got)
			if tt.expectArray {
				assert.True(t, strings.HasPrefix(gotStr, "["), "expected array, got: %s", gotStr)
				assert.Contains(t, gotStr, "cache_control")
			} else {
				assert.True(t, strings.HasPrefix(gotStr, "\""), "expected string, got: %s", gotStr)
			}
		})
	}
}

func TestCacheRequestBuilder_BuildMessages(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
		{Role: "user", Content: "How are you?"},
	}

	tests := []struct {
		name              string
		config            *interfaces.CacheConfig
		expectCacheOnLast bool
	}{
		{
			name:              "no caching",
			config:            nil,
			expectCacheOnLast: false,
		},
		{
			name:              "cache conversation",
			config:            &interfaces.CacheConfig{CacheConversation: true},
			expectCacheOnLast: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := newCacheRequestBuilder(tt.config)
			got, err := builder.BuildMessages(messages)
			require.NoError(t, err)

			gotStr := string(got)
			if tt.expectCacheOnLast {
				// Last message should have cache_control
				assert.Contains(t, gotStr, "cache_control")
				// Should have content array for last message
				assert.Contains(t, gotStr, `"content":[`)
			} else {
				assert.NotContains(t, gotStr, "cache_control")
			}
		})
	}
}

func TestCacheRequestBuilder_BuildTools(t *testing.T) {
	tools := []Tool{
		{Name: "calculator", Description: "Does math", InputSchema: map[string]interface{}{"type": "object"}},
		{Name: "web_search", Description: "Searches web", InputSchema: map[string]interface{}{"type": "object"}},
	}

	tests := []struct {
		name              string
		config            *interfaces.CacheConfig
		expectCacheOnLast bool
	}{
		{
			name:              "no caching",
			config:            nil,
			expectCacheOnLast: false,
		},
		{
			name:              "cache tools",
			config:            &interfaces.CacheConfig{CacheTools: true},
			expectCacheOnLast: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := newCacheRequestBuilder(tt.config)
			got, err := builder.BuildTools(tools)
			require.NoError(t, err)

			gotStr := string(got)
			if tt.expectCacheOnLast {
				assert.Contains(t, gotStr, "cache_control")
			} else {
				assert.NotContains(t, gotStr, "cache_control")
			}
		})
	}
}

func TestCacheRequestBuilder_BuildCacheableRequest(t *testing.T) {
	req := &CompletionRequest{
		Model:       "claude-3-sonnet",
		MaxTokens:   1024,
		Temperature: 0.7,
		System:      "You are a helpful assistant",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi!"},
			{Role: "user", Content: "How are you?"},
		},
		Tools: []Tool{
			{Name: "search", Description: "Search", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	tests := []struct {
		name                string
		config              *interfaces.CacheConfig
		expectSystemCache   bool
		expectToolsCache    bool
		expectMessagesCache bool
	}{
		{
			name:                "cache all",
			config:              &interfaces.CacheConfig{CacheSystemMessage: true, CacheTools: true, CacheConversation: true},
			expectSystemCache:   true,
			expectToolsCache:    true,
			expectMessagesCache: true,
		},
		{
			name:                "cache system only",
			config:              &interfaces.CacheConfig{CacheSystemMessage: true},
			expectSystemCache:   true,
			expectToolsCache:    false,
			expectMessagesCache: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := newCacheRequestBuilder(tt.config)
			cacheableReq, err := builder.BuildCacheableRequest(req)
			require.NoError(t, err)

			// Check system
			systemStr := string(cacheableReq.System)
			if tt.expectSystemCache {
				assert.Contains(t, systemStr, "cache_control")
			} else {
				assert.NotContains(t, systemStr, "cache_control")
			}

			// Check tools
			toolsStr := string(cacheableReq.Tools)
			if tt.expectToolsCache {
				assert.Contains(t, toolsStr, "cache_control")
			} else {
				assert.NotContains(t, toolsStr, "cache_control")
			}

			// Check messages
			messagesStr := string(cacheableReq.Messages)
			if tt.expectMessagesCache {
				assert.Contains(t, messagesStr, "cache_control")
			} else {
				assert.NotContains(t, messagesStr, "cache_control")
			}
		})
	}
}

func TestUsageParsingWithCacheMetrics(t *testing.T) {
	responseJSON := `{
		"id": "msg_123",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "Hello"}],
		"model": "claude-3-sonnet",
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 100,
			"output_tokens": 50,
			"cache_creation_input_tokens": 80,
			"cache_read_input_tokens": 20
		}
	}`

	var resp CompletionResponse
	err := json.Unmarshal([]byte(responseJSON), &resp)
	require.NoError(t, err)

	assert.Equal(t, 100, resp.Usage.InputTokens)
	assert.Equal(t, 50, resp.Usage.OutputTokens)
	assert.Equal(t, 80, resp.Usage.CacheCreationInputTokens)
	assert.Equal(t, 20, resp.Usage.CacheReadInputTokens)
}

func TestCacheOptions(t *testing.T) {
	t.Run("WithCacheSystemMessage", func(t *testing.T) {
		opts := &interfaces.GenerateOptions{}
		WithCacheSystemMessage()(opts)

		require.NotNil(t, opts.CacheConfig)
		assert.True(t, opts.CacheConfig.CacheSystemMessage)
	})

	t.Run("WithCacheTools", func(t *testing.T) {
		opts := &interfaces.GenerateOptions{}
		WithCacheTools()(opts)

		require.NotNil(t, opts.CacheConfig)
		assert.True(t, opts.CacheConfig.CacheTools)
	})

	t.Run("WithCacheConversation", func(t *testing.T) {
		opts := &interfaces.GenerateOptions{}
		WithCacheConversation()(opts)

		require.NotNil(t, opts.CacheConfig)
		assert.True(t, opts.CacheConfig.CacheConversation)
	})

	t.Run("WithCacheTTL", func(t *testing.T) {
		opts := &interfaces.GenerateOptions{}
		WithCacheTTL("1h")(opts)

		require.NotNil(t, opts.CacheConfig)
		assert.Equal(t, "1h", opts.CacheConfig.CacheTTL)
	})

	t.Run("multiple options", func(t *testing.T) {
		opts := &interfaces.GenerateOptions{}
		WithCacheSystemMessage()(opts)
		WithCacheTools()(opts)
		WithCacheTTL("1h")(opts)

		require.NotNil(t, opts.CacheConfig)
		assert.True(t, opts.CacheConfig.CacheSystemMessage)
		assert.True(t, opts.CacheConfig.CacheTools)
		assert.Equal(t, "1h", opts.CacheConfig.CacheTTL)
	})
}
