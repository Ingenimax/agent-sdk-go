package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestParseSSELine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected *AnthropicSSEEvent
		wantErr  bool
	}{
		{
			name: "Valid data line",
			line: `data: {"type": "message_start", "data": {"id": "msg_123"}}`,
			expected: &AnthropicSSEEvent{
				Type: "message_start",
				Data: json.RawMessage(`{"id": "msg_123"}`),
			},
			wantErr: false,
		},
		{
			name: "Event line",
			line: "event: ping",
			expected: &AnthropicSSEEvent{
				Type: "ping",
			},
			wantErr: false,
		},
		{
			name:     "Empty line",
			line:     "",
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "Comment line",
			line:     ": this is a comment",
			expected: nil,
			wantErr:  false,
		},
		{
			name: "Done signal",
			line: "data: [DONE]",
			expected: &AnthropicSSEEvent{
				Type: "done",
			},
			wantErr: false,
		},
		{
			name:    "Invalid JSON",
			line:    "data: {invalid json}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseSSELine(tt.line)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if (result == nil) != (tt.expected == nil) {
				t.Errorf("Expected result nil=%v, got nil=%v", tt.expected == nil, result == nil)
				return
			}

			if result != nil && tt.expected != nil {
				if result.Type != tt.expected.Type {
					t.Errorf("Expected type %s, got %s", tt.expected.Type, result.Type)
				}

				// Only compare Data if both are non-nil
				if len(tt.expected.Data) > 0 {
					var expectedData, resultData map[string]interface{}
					if err := json.Unmarshal(tt.expected.Data, &expectedData); err != nil {
						t.Errorf("Failed to unmarshal expected data: %v", err)
					}
					if err := json.Unmarshal(result.Data, &resultData); err != nil {
						t.Errorf("Failed to unmarshal result data: %v", err)
					}
					// Basic check for ID field
					if expectedData["id"] != resultData["id"] {
						t.Errorf("Expected data mismatch")
					}
				}
			}
		})
	}
}

func TestConvertAnthropicEventToStreamEvent(t *testing.T) {
	client := &AnthropicClient{}

	tests := []struct {
		name           string
		anthropicEvent *AnthropicSSEEvent
		expectedType   interfaces.StreamEventType
		expectError    bool
	}{
		{
			name: "Message start event",
			anthropicEvent: &AnthropicSSEEvent{
				Type: "message_start",
				Data: json.RawMessage(`{"type": "message_start", "message": {"id": "msg_123", "role": "assistant", "content": [], "model": "claude-3-sonnet-20240229", "usage": {"input_tokens": 10, "output_tokens": 0}}}`),
			},
			expectedType: interfaces.StreamEventMessageStart,
			expectError:  false,
		},
		{
			name: "Content block delta",
			anthropicEvent: &AnthropicSSEEvent{
				Type: "content_block_delta",
				Data: json.RawMessage(`{"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}`),
			},
			expectedType: interfaces.StreamEventContentDelta,
			expectError:  false,
		},
		{
			name: "Message stop event",
			anthropicEvent: &AnthropicSSEEvent{
				Type: "message_stop",
				Data: json.RawMessage(`{"type": "message_stop"}`),
			},
			expectedType: interfaces.StreamEventMessageStop,
			expectError:  false,
		},
		{
			name: "Error event",
			anthropicEvent: &AnthropicSSEEvent{
				Type: "error",
				Data: json.RawMessage(`{"type": "error", "error": {"type": "invalid_request_error", "message": "Invalid request"}}`),
			},
			expectedType: interfaces.StreamEventError,
			expectError:  false,
		},
		{
			name: "Ping event (should be ignored)",
			anthropicEvent: &AnthropicSSEEvent{
				Type: "ping",
			},
			expectedType: interfaces.StreamEventType(""),
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thinkingBlocks := make(map[int]bool)
			toolBlocks := make(map[int]struct {
				ID        string
				Name      string
				InputJSON strings.Builder
			})
			result, err := client.convertAnthropicEventToStreamEvent(tt.anthropicEvent, thinkingBlocks, toolBlocks)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// For ping events, result should be nil
			if tt.anthropicEvent.Type == "ping" {
				if result != nil {
					t.Error("Expected nil result for ping event")
				}
				return
			}

			if result == nil {
				t.Error("Expected non-nil result")
				return
			}

			if result.Type != tt.expectedType {
				t.Errorf("Expected event type %s, got %s", tt.expectedType, result.Type)
			}

			// Verify timestamp is set
			if result.Timestamp.IsZero() {
				t.Error("Expected timestamp to be set")
			}

			// Verify metadata is initialized
			if result.Metadata == nil {
				t.Error("Expected metadata to be initialized")
			}
		})
	}
}

func TestMessageStartData(t *testing.T) {
	var msgStart MessageStartData
	// The actual Anthropic SSE format nests message data inside a "message" field
	err := json.Unmarshal([]byte(`{"type": "message_start", "message": {"id": "msg_123", "role": "assistant", "content": [], "model": "claude-3-sonnet-20240229", "usage": {"input_tokens": 10, "output_tokens": 0}}}`), &msgStart)
	if err != nil {
		t.Fatalf("Failed to unmarshal MessageStartData: %v", err)
	}

	if msgStart.Message.ID != "msg_123" {
		t.Errorf("Expected ID 'msg_123', got '%s'", msgStart.Message.ID)
	}

	if msgStart.Message.Model != "claude-3-sonnet-20240229" {
		t.Errorf("Expected model 'claude-3-sonnet-20240229', got '%s'", msgStart.Message.Model)
	}

	if msgStart.Message.Role != "assistant" {
		t.Errorf("Expected role 'assistant', got '%s'", msgStart.Message.Role)
	}

	if msgStart.Message.Usage.InputTokens != 10 {
		t.Errorf("Expected input_tokens 10, got %d", msgStart.Message.Usage.InputTokens)
	}
}

func TestContentBlockDeltaData(t *testing.T) {
	var blockDelta ContentBlockDeltaData
	err := json.Unmarshal([]byte(`{"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello world"}}`), &blockDelta)
	if err != nil {
		t.Fatalf("Failed to unmarshal ContentBlockDeltaData: %v", err)
	}

	if blockDelta.Index != 0 {
		t.Errorf("Expected index 0, got %d", blockDelta.Index)
	}

	if blockDelta.Delta.Text != "Hello world" {
		t.Errorf("Expected text 'Hello world', got '%s'", blockDelta.Delta.Text)
	}

	if blockDelta.Delta.Type != "text_delta" {
		t.Errorf("Expected delta type 'text_delta', got '%s'", blockDelta.Delta.Type)
	}
}

func TestGenerateWithMemory(t *testing.T) {
	// Test memory message ordering with a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body to validate messages
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Verify messages array
		messages, ok := reqBody["messages"].([]interface{})
		if !ok {
			t.Fatalf("Expected messages array in request")
		}

		// We expect 4 messages: system->user, user, assistant, current user
		if len(messages) != 4 {
			t.Errorf("Expected 4 messages in request, got %d", len(messages))
			// Debug: print the messages
			for i, msg := range messages {
				t.Logf("Message %d: %+v", i, msg)
			}
		}

		// Send mock response
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"content": []map[string]interface{}{
				{"text": "test response", "type": "text"},
			},
			"id":    "msg_123",
			"model": "claude-3-sonnet-20240229",
			"role":  "assistant",
			"type":  "message",
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client with test server
	client := NewClient(
		"test-key",
		WithBaseURL(server.URL),
		WithLogger(logging.New()),
	)

	memory := &mockMemory{
		messages: []interfaces.Message{
			{Role: interfaces.MessageRoleSystem, Content: "You are helpful"},
			{Role: interfaces.MessageRoleUser, Content: "Hi"},
			{Role: interfaces.MessageRoleAssistant, Content: "Hello!"},
			{Role: interfaces.MessageRoleUser, Content: "How are you?"},
		},
	}

	// Test Generate with memory
	_, err := client.Generate(context.Background(), "How are you?",
		interfaces.WithMemory(memory))

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
}
