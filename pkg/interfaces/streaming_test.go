package interfaces

import (
	"testing"
	"time"
)

func TestStreamEvent(t *testing.T) {
	// Test StreamEvent creation
	event := StreamEvent{
		Type:      StreamEventContentDelta,
		Content:   "test content",
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{"test": "value"},
	}

	if event.Type != StreamEventContentDelta {
		t.Errorf("Expected event type %s, got %s", StreamEventContentDelta, event.Type)
	}

	if event.Content != "test content" {
		t.Errorf("Expected content 'test content', got '%s'", event.Content)
	}

	if event.Metadata["test"] != "value" {
		t.Errorf("Expected metadata test=value, got %v", event.Metadata["test"])
	}
}

func TestAgentStreamEvent(t *testing.T) {
	// Test AgentStreamEvent creation
	event := AgentStreamEvent{
		Type:         AgentEventToolCall,
		Content:      "test content",
		ThinkingStep: "thinking step",
		Timestamp:    time.Now(),
		ToolCall: &ToolCallEvent{
			ID:     "test-id",
			Name:   "test-tool",
			Status: "executing",
		},
	}

	if event.Type != AgentEventToolCall {
		t.Errorf("Expected event type %s, got %s", AgentEventToolCall, event.Type)
	}

	if event.ToolCall == nil {
		t.Error("Expected tool call to be set")
	}

	if event.ToolCall.ID != "test-id" {
		t.Errorf("Expected tool call ID 'test-id', got '%s'", event.ToolCall.ID)
	}
}

func TestStreamConfig(t *testing.T) {
	// Test default stream config
	config := DefaultStreamConfig()

	if config.BufferSize != 100 {
		t.Errorf("Expected default buffer size 100, got %d", config.BufferSize)
	}

	if !config.IncludeThinking {
		t.Error("Expected IncludeThinking to be true by default")
	}

	if !config.IncludeToolProgress {
		t.Error("Expected IncludeToolProgress to be true by default")
	}

	// Test custom stream config
	customConfig := StreamConfig{
		BufferSize:          200,
		IncludeThinking:     false,
		IncludeToolProgress: false,
	}

	if customConfig.BufferSize != 200 {
		t.Errorf("Expected custom buffer size 200, got %d", customConfig.BufferSize)
	}

	if customConfig.IncludeThinking {
		t.Error("Expected IncludeThinking to be false")
	}

	if customConfig.IncludeToolProgress {
		t.Error("Expected IncludeToolProgress to be false")
	}
}

func TestStreamEventTypes(t *testing.T) {
	// Test all stream event types are defined
	eventTypes := []StreamEventType{
		StreamEventMessageStart,
		StreamEventContentDelta,
		StreamEventContentComplete,
		StreamEventMessageStop,
		StreamEventError,
		StreamEventToolUse,
		StreamEventToolResult,
		StreamEventThinking,
	}

	for _, eventType := range eventTypes {
		if string(eventType) == "" {
			t.Errorf("Event type %v is empty", eventType)
		}
	}
}

func TestAgentEventTypes(t *testing.T) {
	// Test all agent event types are defined
	eventTypes := []AgentEventType{
		AgentEventContent,
		AgentEventThinking,
		AgentEventToolCall,
		AgentEventToolResult,
		AgentEventError,
		AgentEventComplete,
	}

	for _, eventType := range eventTypes {
		if string(eventType) == "" {
			t.Errorf("Agent event type %v is empty", eventType)
		}
	}
}

func TestToolCallEvent(t *testing.T) {
	// Test ToolCallEvent
	toolCall := ToolCallEvent{
		ID:        "test-id-123",
		Name:      "calculator",
		Arguments: `{"operation": "add", "a": 1, "b": 2}`,
		Result:    "3",
		Status:    "completed",
	}

	if toolCall.ID != "test-id-123" {
		t.Errorf("Expected tool call ID 'test-id-123', got '%s'", toolCall.ID)
	}

	if toolCall.Name != "calculator" {
		t.Errorf("Expected tool name 'calculator', got '%s'", toolCall.Name)
	}

	if toolCall.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", toolCall.Status)
	}
}