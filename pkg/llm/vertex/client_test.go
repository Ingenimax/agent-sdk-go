package vertex

import (
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
)

func TestClientConfiguration(t *testing.T) {
	projectID := "test-project"

	tests := []struct {
		name     string
		options  []ClientOption
		expected struct {
			model         string
			location      string
			maxRetries    int
			retryDelay    time.Duration
			reasoningMode ReasoningMode
		}
	}{
		{
			name:    "default configuration",
			options: []ClientOption{},
			expected: struct {
				model         string
				location      string
				maxRetries    int
				retryDelay    time.Duration
				reasoningMode ReasoningMode
			}{
				model:         DefaultModel,
				location:      "us-central1",
				maxRetries:    3,
				retryDelay:    time.Second,
				reasoningMode: ReasoningModeNone,
			},
		},
		{
			name: "custom configuration",
			options: []ClientOption{
				WithModel(ModelGemini15Flash),
				WithLocation("us-west1"),
				WithMaxRetries(5),
				WithRetryDelay(2 * time.Second),
				WithReasoningMode(ReasoningModeComprehensive),
			},
			expected: struct {
				model         string
				location      string
				maxRetries    int
				retryDelay    time.Duration
				reasoningMode ReasoningMode
			}{
				model:         ModelGemini15Flash,
				location:      "us-west1",
				maxRetries:    5,
				retryDelay:    2 * time.Second,
				reasoningMode: ReasoningModeComprehensive,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create client configuration without actually connecting
			client := &Client{
				model:         DefaultModel,
				projectID:     projectID,
				location:      "us-central1",
				maxRetries:    3,
				retryDelay:    time.Second,
				reasoningMode: ReasoningModeNone,
			}

			// Apply options
			for _, opt := range tt.options {
				opt(client)
			}

			// Verify configuration
			if client.model != tt.expected.model {
				t.Errorf("expected model %s, got %s", tt.expected.model, client.model)
			}
			if client.location != tt.expected.location {
				t.Errorf("expected location %s, got %s", tt.expected.location, client.location)
			}
			if client.maxRetries != tt.expected.maxRetries {
				t.Errorf("expected maxRetries %d, got %d", tt.expected.maxRetries, client.maxRetries)
			}
			if client.retryDelay != tt.expected.retryDelay {
				t.Errorf("expected retryDelay %v, got %v", tt.expected.retryDelay, client.retryDelay)
			}
			if client.reasoningMode != tt.expected.reasoningMode {
				t.Errorf("expected reasoningMode %s, got %s", tt.expected.reasoningMode, client.reasoningMode)
			}
		})
	}
}

func TestClientName(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{ModelGemini15Pro, "vertex:gemini-1.5-pro"},
		{ModelGemini15Flash, "vertex:gemini-1.5-flash"},
		{ModelGemini20Flash, "vertex:gemini-2.0-flash-exp"},
		{ModelGeminiProVision, "vertex:gemini-pro-vision"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			client := &Client{model: tt.model}
			if got := client.Name(); got != tt.expected {
				t.Errorf("Name() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConvertMessages(t *testing.T) {
	client := &Client{}

	tests := []struct {
		name     string
		messages []llm.Message
		wantErr  bool
	}{
		{
			name: "valid user message",
			messages: []llm.Message{
				{Role: "user", Content: "Hello"},
			},
			wantErr: false,
		},
		{
			name: "valid assistant message",
			messages: []llm.Message{
				{Role: "assistant", Content: "Hi there"},
			},
			wantErr: false,
		},
		{
			name: "system message (should be skipped)",
			messages: []llm.Message{
				{Role: "system", Content: "You are a helpful assistant"},
			},
			wantErr: false,
		},
		{
			name: "mixed messages",
			messages: []llm.Message{
				{Role: "system", Content: "System prompt"},
				{Role: "user", Content: "User message"},
				{Role: "assistant", Content: "Assistant response"},
			},
			wantErr: false,
		},
		{
			name: "invalid role",
			messages: []llm.Message{
				{Role: "invalid", Content: "Invalid message"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts, err := client.convertMessages(tt.messages)

			if tt.wantErr {
				if err == nil {
					t.Errorf("convertMessages() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("convertMessages() unexpected error: %v", err)
				return
			}

			// Count expected parts (excluding system messages)
			expectedParts := 0
			for _, msg := range tt.messages {
				if msg.Role != "system" {
					expectedParts++
				}
			}

			if len(parts) != expectedParts {
				t.Errorf("convertMessages() expected %d parts, got %d", expectedParts, len(parts))
			}
		})
	}
}

func TestGetReasoningInstruction(t *testing.T) {
	tests := []struct {
		mode     ReasoningMode
		expected string
	}{
		{
			mode:     ReasoningModeNone,
			expected: "",
		},
		{
			mode:     ReasoningModeMinimal,
			expected: "Provide clear, direct responses with brief explanations when necessary.",
		},
		{
			mode:     ReasoningModeComprehensive,
			expected: "Think through problems step by step, showing your reasoning process and providing detailed explanations.",
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			client := &Client{reasoningMode: tt.mode}
			if got := client.getReasoningInstruction(); got != tt.expected {
				t.Errorf("getReasoningInstruction() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Note: Integration tests would require actual Google Cloud credentials and project setup
// These tests focus on unit testing the client configuration and message conversion logic
// For integration testing, create a separate test file with build tags or environment checks

func TestModelConstants(t *testing.T) {
	// Verify model constants are properly defined
	models := []string{
		ModelGemini15Pro,
		ModelGemini15Flash,
		ModelGemini20Flash,
		ModelGeminiProVision,
	}

	for _, model := range models {
		if model == "" {
			t.Errorf("Model constant is empty")
		}
	}

	// Verify default model is set
	if DefaultModel == "" {
		t.Errorf("DefaultModel is empty")
	}

	// Verify default model is one of the defined models
	found := false
	for _, model := range models {
		if model == DefaultModel {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DefaultModel %s is not in the list of defined models", DefaultModel)
	}
}

func TestReasoningModeConstants(t *testing.T) {
	// Verify reasoning mode constants are properly defined
	modes := []ReasoningMode{
		ReasoningModeNone,
		ReasoningModeMinimal,
		ReasoningModeComprehensive,
	}

	for _, mode := range modes {
		if string(mode) == "" {
			t.Errorf("ReasoningMode constant is empty")
		}
	}
}
