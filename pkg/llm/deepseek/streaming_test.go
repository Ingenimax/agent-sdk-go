package deepseek

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// TestGenerateStream tests the basic streaming functionality
func TestGenerateStream(t *testing.T) {
	// Create mock server that returns SSE stream
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Send SSE stream
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Create test chunks
		chunks := []StreamChunk{
			{
				ID:      "chatcmpl-123",
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   "deepseek-chat",
				Choices: []StreamChoice{
					{
						Index: 0,
						Delta: StreamDelta{
							Role:    "assistant",
							Content: "Hello",
						},
					},
				},
			},
			{
				ID:      "chatcmpl-123",
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   "deepseek-chat",
				Choices: []StreamChoice{
					{
						Index: 0,
						Delta: StreamDelta{
							Content: " world",
						},
					},
				},
			},
			{
				ID:      "chatcmpl-123",
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   "deepseek-chat",
				Choices: []StreamChoice{
					{
						Index:        0,
						Delta:        StreamDelta{},
						FinishReason: "stop",
					},
				},
			},
		}

		// Write chunks as SSE
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("Expected http.ResponseWriter to be an http.Flusher")
		}

		for _, chunk := range chunks {
			data, err := json.Marshal(chunk)
			if err != nil {
				t.Fatalf("Failed to marshal chunk: %v", err)
			}

			if _, err := w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
				t.Fatalf("Failed to write chunk: %v", err)
			}
			flusher.Flush()
		}

		// Send done marker
		if _, err := w.Write([]byte("data: [DONE]\n\n")); err != nil {
			t.Fatalf("Failed to write done marker: %v", err)
		}
		flusher.Flush()
	}))
	defer server.Close()

	// Create client
	client := NewClient("test-key", WithBaseURL(server.URL))

	// Test streaming
	ctx := context.Background()
	eventChan, err := client.GenerateStream(ctx, "Hello")
	if err != nil {
		t.Fatalf("GenerateStream failed: %v", err)
	}

	// Collect events
	var events []interfaces.StreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Verify events
	if len(events) == 0 {
		t.Fatal("Expected at least one event")
	}

	// Check for message_start
	if events[0].Type != interfaces.StreamEventMessageStart {
		t.Errorf("Expected first event to be message_start, got %s", events[0].Type)
	}

	// Check for content deltas
	contentEvents := 0
	var fullContent strings.Builder
	for _, event := range events {
		if event.Type == interfaces.StreamEventContentDelta {
			contentEvents++
			fullContent.WriteString(event.Content)
		}
	}

	if contentEvents == 0 {
		t.Error("Expected at least one content delta event")
	}

	expectedContent := "Hello world"
	if fullContent.String() != expectedContent {
		t.Errorf("Expected content %q, got %q", expectedContent, fullContent.String())
	}

	// Check for message_stop
	lastEvent := events[len(events)-1]
	if lastEvent.Type != interfaces.StreamEventMessageStop {
		t.Errorf("Expected last event to be message_stop, got %s", lastEvent.Type)
	}
}

// TestGenerateStreamWithTools tests streaming with tool calls
func TestGenerateStreamWithTools(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("Expected http.ResponseWriter to be an http.Flusher")
		}

		// First iteration - tool call
		toolCallChunk := StreamChunk{
			ID:      "chatcmpl-123",
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   "deepseek-chat",
			Choices: []StreamChoice{
				{
					Index: 0,
					Delta: StreamDelta{
						ToolCalls: []ToolCall{
							{
								ID:   "call_123",
								Type: "function",
								Function: FunctionCall{
									Name:      "test_tool",
									Arguments: `{"input":"test"}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
		}

		data, err := json.Marshal(toolCallChunk)
		if err != nil {
			t.Fatalf("Failed to marshal chunk: %v", err)
		}

		if _, err := w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
			t.Fatalf("Failed to write chunk: %v", err)
		}
		flusher.Flush()

		// Send done marker
		if _, err := w.Write([]byte("data: [DONE]\n\n")); err != nil {
			t.Fatalf("Failed to write done marker: %v", err)
		}
		flusher.Flush()
	}))
	defer server.Close()

	// Create client
	client := NewClient("test-key", WithBaseURL(server.URL))

	// Create test tool
	testTool := &mockTool{
		name:        "test_tool",
		description: "Test tool",
		result:      "test result",
	}

	// Test streaming with tools
	ctx := context.Background()
	eventChan, err := client.GenerateWithToolsStream(ctx, "Test", []interfaces.Tool{testTool})
	if err != nil {
		t.Fatalf("GenerateWithToolsStream failed: %v", err)
	}

	// Collect events
	var events []interfaces.StreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Verify we got tool use event
	foundToolUse := false
	foundToolResult := false
	for _, event := range events {
		if event.Type == interfaces.StreamEventToolUse {
			foundToolUse = true
			if event.ToolCall == nil {
				t.Error("Expected ToolCall in tool_use event")
			} else if event.ToolCall.Name != "test_tool" {
				t.Errorf("Expected tool name test_tool, got %s", event.ToolCall.Name)
			}
		}
		if event.Type == interfaces.StreamEventToolResult {
			foundToolResult = true
		}
	}

	if !foundToolUse {
		t.Error("Expected to find tool_use event")
	}

	if !foundToolResult {
		t.Error("Expected to find tool_result event")
	}
}

// TestGenerateStreamError tests error handling in streaming
func TestGenerateStreamError(t *testing.T) {
	// Create mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(`{"error": {"message": "Internal server error"}}`)); err != nil {
			t.Fatalf("Failed to write error: %v", err)
		}
	}))
	defer server.Close()

	// Create client
	client := NewClient("test-key", WithBaseURL(server.URL))

	// Test streaming
	ctx := context.Background()
	eventChan, err := client.GenerateStream(ctx, "Test")
	if err != nil {
		t.Fatalf("GenerateStream failed: %v", err)
	}

	// Collect events
	var events []interfaces.StreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Verify we got an error event
	foundError := false
	for _, event := range events {
		if event.Type == interfaces.StreamEventError {
			foundError = true
			if event.Error == nil {
				t.Error("Expected error in error event")
			}
		}
	}

	if !foundError {
		t.Error("Expected to find error event")
	}
}

// mockTool implements interfaces.Tool for testing
type mockTool struct {
	name        string
	description string
	result      string
	executeErr  error
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"input": {
			Type:        "string",
			Description: "Test input",
			Required:    true,
		},
	}
}

func (m *mockTool) Run(ctx context.Context, input string) (string, error) {
	if m.executeErr != nil {
		return "", m.executeErr
	}
	return m.result, nil
}

func (m *mockTool) Execute(ctx context.Context, args string) (string, error) {
	if m.executeErr != nil {
		return "", m.executeErr
	}
	return m.result, nil
}
