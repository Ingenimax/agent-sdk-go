package gemini

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestThinkingOptionsOnEmptyClient(t *testing.T) {
	budget := int32(512)
	tests := []struct {
		name string
		opt  Option
		want ThinkingConfig
	}{
		{"thinking", WithThinking(true), ThinkingConfig{IncludeThoughts: true}},
		{"budget", WithThinkingBudget(512), ThinkingConfig{ThinkingBudget: &budget}},
		{"level", WithThinkingLevel(ThinkingLevelHigh), ThinkingConfig{ThinkingLevel: ThinkingLevelHigh}},
		{"dynamic", WithDynamicThinking(), ThinkingConfig{IncludeThoughts: true}},
		{"signatures", WithThoughtSignatures([][]byte{[]byte("sig")}), ThinkingConfig{ThoughtSignatures: [][]byte{[]byte("sig")}}},
		{"full config", WithThinkingConfig(ThinkingConfig{IncludeThoughts: true, ThinkingLevel: ThinkingLevelLow}), ThinkingConfig{IncludeThoughts: true, ThinkingLevel: ThinkingLevelLow}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A client built without NewClient has no thinking config yet.
			c := &GeminiClient{}
			tt.opt(c)
			require.NotNil(t, c.thinkingConfig)
			assert.Equal(t, tt.want, *c.thinkingConfig)
		})
	}
}

func TestDynamicThinkingClearsBudget(t *testing.T) {
	c := &GeminiClient{}
	WithThinkingBudget(100)(c)
	WithDynamicThinking()(c)
	assert.Nil(t, c.thinkingConfig.ThinkingBudget)
	assert.True(t, c.thinkingConfig.IncludeThoughts)
}

func TestNewClientStoresOptions(t *testing.T) {
	client, err := NewClient(t.Context(),
		WithAPIKey("k"),
		WithMaxOutputTokens(256),
		WithLocation("europe-west1"),
		WithRetry(),
		WithThoughtSignatures([][]byte{[]byte("s")}),
	)
	require.NoError(t, err)
	require.NotNil(t, client.maxOutputTokens)
	assert.Equal(t, int32(256), *client.maxOutputTokens)
	assert.Equal(t, "europe-west1", client.location)
	assert.NotNil(t, client.retryExecutor)
	assert.Equal(t, [][]byte{[]byte("s")}, client.thinkingConfig.ThoughtSignatures)
}

func TestNewClientVertexCredentialErrors(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		wantErr string
	}{
		{
			name:    "missing credentials file",
			opts:    []Option{WithCredentialsFile(filepath.Join(t.TempDir(), "missing.json"))},
			wantErr: "failed to load credentials from file",
		},
		{
			name:    "invalid credentials JSON",
			opts:    []Option{WithCredentialsJSON([]byte("not json"))},
			wantErr: "failed to load credentials from JSON",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := append([]Option{WithBackend(genai.BackendVertexAI), WithProjectID("p")}, tt.opts...)
			client, err := NewClient(t.Context(), opts...)
			require.Error(t, err)
			assert.Nil(t, client)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
