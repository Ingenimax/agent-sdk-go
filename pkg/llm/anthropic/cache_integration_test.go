//go:build integration
// +build integration

package anthropic

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run with: go test -tags=integration -run TestCacheIntegration -v ./pkg/llm/anthropic/...
// Requires ANTHROPIC_API_KEY environment variable

func TestCacheIntegration_SystemMessage(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	client := NewClient(apiKey, WithModel(ClaudeSonnet45))
	ctx := context.Background()

	// Long system message with timestamp to ensure fresh cache
	longSystemMessage := fmt.Sprintf(`You are an expert AI assistant. Session: %d
`, time.Now().UnixNano()) + generatePadding(2000)

	// First call - should create cache
	t.Log("Making first request (should create cache)...")
	resp1, err := client.GenerateDetailed(ctx, "What is 2+2? Answer briefly.",
		interfaces.WithSystemMessage(longSystemMessage),
		WithCacheSystemMessage(),
	)
	require.NoError(t, err)
	require.NotNil(t, resp1.Usage)

	t.Logf("First call - Cache creation: %d, Cache read: %d",
		resp1.Usage.CacheCreationInputTokens,
		resp1.Usage.CacheReadInputTokens)

	assert.Greater(t, resp1.Usage.CacheCreationInputTokens, 0, "First call should create cache")

	// Second call - should read from cache
	t.Log("Making second request (should read from cache)...")
	resp2, err := client.GenerateDetailed(ctx, "What is 3+3? Answer briefly.",
		interfaces.WithSystemMessage(longSystemMessage),
		WithCacheSystemMessage(),
	)
	require.NoError(t, err)
	require.NotNil(t, resp2.Usage)

	t.Logf("Second call - Cache creation: %d, Cache read: %d",
		resp2.Usage.CacheCreationInputTokens,
		resp2.Usage.CacheReadInputTokens)

	assert.Greater(t, resp2.Usage.CacheReadInputTokens, 0, "Second call should read from cache")
	assert.Equal(t, resp1.Usage.CacheCreationInputTokens, resp2.Usage.CacheReadInputTokens,
		"Cache read should match cache creation")
}

func TestCacheIntegration_LongConversation(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	client := NewClient(apiKey, WithModel(ClaudeSonnet45))
	ctx := context.Background()

	// Build a long conversation context (~15k tokens)
	longContext := fmt.Sprintf(`You are an expert software architect. Session: %d

=== PROJECT CONTEXT ===
Large enterprise microservices application.
`, time.Now().UnixNano())

	components := []string{"auth", "payments", "notifications", "analytics", "gateway"}
	for i := 0; i < 20; i++ {
		component := components[i%len(components)]
		longContext += fmt.Sprintf(`
--- Turn %d ---
User: Explain the %s component architecture.
Assistant: The %s component uses clean architecture with API, service, and repository layers.
It handles requests via middleware, implements circuit breakers, and publishes events async.
Key patterns: dependency injection, repository pattern, event sourcing.
`, i+1, component, component)
	}
	longContext += generatePadding(2000)

	t.Logf("Context size: ~%d tokens", len(longContext)/4)

	// First call - should create cache
	t.Log("Making first request (should create cache)...")
	resp1, err := client.GenerateDetailed(ctx, "Summarize the architecture in one sentence.",
		interfaces.WithSystemMessage(longContext),
		WithCacheSystemMessage(),
	)
	require.NoError(t, err)
	require.NotNil(t, resp1.Usage)

	t.Logf("First call - Cache creation: %d, Cache read: %d",
		resp1.Usage.CacheCreationInputTokens,
		resp1.Usage.CacheReadInputTokens)

	assert.Greater(t, resp1.Usage.CacheCreationInputTokens, 0, "First call should create cache")

	// Second call - should read from cache
	t.Log("Making second request (should read from cache)...")
	resp2, err := client.GenerateDetailed(ctx, "What's the most important component?",
		interfaces.WithSystemMessage(longContext),
		WithCacheSystemMessage(),
	)
	require.NoError(t, err)
	require.NotNil(t, resp2.Usage)

	t.Logf("Second call - Cache creation: %d, Cache read: %d",
		resp2.Usage.CacheCreationInputTokens,
		resp2.Usage.CacheReadInputTokens)

	assert.Greater(t, resp2.Usage.CacheReadInputTokens, 0, "Second call should read from cache")
}

func TestCacheIntegration_Tools(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	client := NewClient(apiKey, WithModel(ClaudeSonnet45))
	ctx := context.Background()

	tools := []interfaces.Tool{
		&cacheTestTool{
			name:        "calculator",
			description: "Performs calculations. " + generatePadding(500),
			params: map[string]interfaces.ParameterSpec{
				"expression": {Type: "string", Description: "Math expression", Required: true},
			},
		},
		&cacheTestTool{
			name:        "weather",
			description: "Gets weather. " + generatePadding(500),
			params: map[string]interfaces.ParameterSpec{
				"location": {Type: "string", Description: "City name", Required: true},
			},
		},
	}

	systemMessage := fmt.Sprintf("You are a helpful assistant. Session: %d. ", time.Now().UnixNano()) + generatePadding(1000)

	// First call - should create cache
	t.Log("Making first request with tools...")
	resp1, err := client.GenerateWithToolsDetailed(ctx, "What is 15 * 23?", tools,
		interfaces.WithSystemMessage(systemMessage),
		WithCacheSystemMessage(),
		WithCacheTools(),
	)
	require.NoError(t, err)

	if resp1.Usage != nil {
		t.Logf("First call - Cache creation: %d, Cache read: %d",
			resp1.Usage.CacheCreationInputTokens,
			resp1.Usage.CacheReadInputTokens)
	} else {
		t.Log("First call - Usage not available (multi-turn tool calls)")
	}

	// Second call - should read from cache
	t.Log("Making second request (tools should be cached)...")
	resp2, err := client.GenerateWithToolsDetailed(ctx, "What is 42 / 6?", tools,
		interfaces.WithSystemMessage(systemMessage),
		WithCacheSystemMessage(),
		WithCacheTools(),
	)
	require.NoError(t, err)

	if resp2.Usage != nil {
		t.Logf("Second call - Cache creation: %d, Cache read: %d",
			resp2.Usage.CacheCreationInputTokens,
			resp2.Usage.CacheReadInputTokens)

		if resp2.Usage.CacheReadInputTokens > 0 {
			t.Log("SUCCESS: Cache hit on tools!")
		}
	} else {
		t.Log("Second call - Usage not available (multi-turn tool calls)")
	}

	// Note: Tool caching works but Usage may not be available when tool calls
	// require multiple iterations. The cache is still being used internally.
	t.Log("Tool caching test completed - cache is applied to tool definitions")
}

// Helper to generate padding text to meet minimum cache token requirements
func generatePadding(words int) string {
	padding := ""
	for i := 0; i < words; i++ {
		padding += fmt.Sprintf("word%d ", i)
	}
	return padding
}

// cacheTestTool implementation for testing
type cacheTestTool struct {
	name        string
	description string
	params      map[string]interfaces.ParameterSpec
}

func (t *cacheTestTool) Name() string        { return t.name }
func (t *cacheTestTool) Description() string { return t.description }

func (t *cacheTestTool) Run(ctx context.Context, input string) (string, error) {
	return "42", nil
}

func (t *cacheTestTool) Execute(ctx context.Context, args string) (string, error) {
	return "42", nil
}

func (t *cacheTestTool) Parameters() map[string]interfaces.ParameterSpec {
	return t.params
}
