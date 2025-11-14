package tools

import (
	"context"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// MockStreamingAgent implements StreamingSubAgent for testing
type MockStreamingAgent struct {
	name        string
	description string
	streamFunc  func(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error)
}

func (m *MockStreamingAgent) Run(ctx context.Context, input string) (string, error) {
	return "mock non-streaming result", nil
}

func (m *MockStreamingAgent) RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error) {
	if m.streamFunc != nil {
		return m.streamFunc(ctx, input)
	}

	// Default implementation: send some mock events
	eventChan := make(chan interfaces.AgentStreamEvent, 10)
	go func() {
		defer close(eventChan)

		// Send thinking event
		eventChan <- interfaces.AgentStreamEvent{
			Type:         interfaces.AgentEventThinking,
			ThinkingStep: "Processing request",
			Timestamp:    time.Now(),
		}

		// Send content event
		eventChan <- interfaces.AgentStreamEvent{
			Type:      interfaces.AgentEventContent,
			Content:   "Mock streaming result",
			Timestamp: time.Now(),
		}

		// Send complete event
		eventChan <- interfaces.AgentStreamEvent{
			Type:      interfaces.AgentEventComplete,
			Timestamp: time.Now(),
		}
	}()

	return eventChan, nil
}

func (m *MockStreamingAgent) GetName() string {
	return m.name
}

func (m *MockStreamingAgent) GetDescription() string {
	return m.description
}

// TestAgentToolStreamingCapability tests that AgentTool correctly detects streaming support
func TestAgentToolStreamingCapability(t *testing.T) {
	mockAgent := &MockStreamingAgent{
		name:        "TestAgent",
		description: "Test agent for streaming",
	}

	tool := NewAgentTool(mockAgent)

	// Create a parent event channel
	parentChan := make(chan interfaces.AgentStreamEvent, 10)
	defer close(parentChan)

	// Create context with event channel
	ctx := WithStreamEventChan(context.Background(), parentChan)

	// Execute the tool
	result, err := tool.Run(ctx, "test input")
	if err != nil {
		t.Fatalf("Tool execution failed: %v", err)
	}

	if result == "" {
		t.Error("Expected non-empty result")
	}

	// Verify that events were forwarded to parent channel
	eventsReceived := 0
	timeout := time.After(2 * time.Second)

	for {
		select {
		case event, ok := <-parentChan:
			if !ok {
				// Channel closed, stop reading
				goto done
			}
			eventsReceived++

			// Verify sub-agent metadata was added
			if event.Metadata == nil {
				t.Error("Event metadata should not be nil")
			} else {
				if subAgent, ok := event.Metadata["sub_agent"].(string); !ok || subAgent != "TestAgent" {
					t.Errorf("Expected sub_agent metadata to be 'TestAgent', got: %v", event.Metadata["sub_agent"])
				}
			}

		case <-timeout:
			goto done
		default:
			// No more events immediately available
			if eventsReceived > 0 {
				goto done
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

done:
	if eventsReceived == 0 {
		t.Error("Expected to receive forwarded events from sub-agent, but got none")
	} else {
		t.Logf("Successfully received %d forwarded events from sub-agent", eventsReceived)
	}
}

// TestAgentToolWithoutStreamContext tests fallback to non-streaming when context has no event channel
func TestAgentToolWithoutStreamContext(t *testing.T) {
	mockAgent := &MockStreamingAgent{
		name:        "TestAgent",
		description: "Test agent for streaming",
	}

	tool := NewAgentTool(mockAgent)

	// Create context WITHOUT event channel
	ctx := context.Background()

	// Execute the tool
	result, err := tool.Run(ctx, "test input")
	if err != nil {
		t.Fatalf("Tool execution failed: %v", err)
	}

	// Should get the non-streaming result
	if result != "mock non-streaming result" {
		t.Errorf("Expected non-streaming result, got: %s", result)
	}
}

// TestStreamEventChanContext tests the context helper functions
func TestStreamEventChanContext(t *testing.T) {
	ctx := context.Background()

	// Create an event channel
	eventChan := make(chan interfaces.AgentStreamEvent, 10)
	defer close(eventChan)

	// Add to context
	ctx = WithStreamEventChan(ctx, eventChan)

	// Retrieve from context
	retrieved, ok := ctx.Value(streamEventChan).(chan<- interfaces.AgentStreamEvent)
	if !ok {
		t.Fatal("Failed to retrieve event channel from context")
	}

	// Verify it's the same channel
	if retrieved == nil {
		t.Error("Retrieved channel should not be nil")
	}

	// Test that we can send to it
	select {
	case retrieved <- interfaces.AgentStreamEvent{
		Type:      interfaces.AgentEventContent,
		Content:   "test",
		Timestamp: time.Now(),
	}:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("Failed to send event to retrieved channel")
	}

	// Verify we can receive it
	select {
	case event := <-eventChan:
		if event.Content != "test" {
			t.Errorf("Expected content 'test', got: %s", event.Content)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Failed to receive event from channel")
	}
}

// TestAgentToolMetadataInjection tests that sub-agent metadata is properly injected into events
func TestAgentToolMetadataInjection(t *testing.T) {
	mockAgent := &MockStreamingAgent{
		name:        "MathAgent",
		description: "Math specialist",
	}

	tool := NewAgentTool(mockAgent)
	parentChan := make(chan interfaces.AgentStreamEvent, 10)
	defer close(parentChan)

	ctx := WithStreamEventChan(context.Background(), parentChan)

	// Execute tool in goroutine
	go func() {
		_, _ = tool.Run(ctx, "calculate 2+2")
	}()

	// Collect events
	timeout := time.After(2 * time.Second)
	var events []interfaces.AgentStreamEvent

	for {
		select {
		case event, ok := <-parentChan:
			if !ok {
				goto verify
			}
			events = append(events, event)
			if event.Type == interfaces.AgentEventComplete {
				goto verify
			}
		case <-timeout:
			goto verify
		}
	}

verify:
	if len(events) == 0 {
		t.Fatal("No events received")
	}

	// Verify all events have correct metadata
	for i, event := range events {
		if event.Metadata == nil {
			t.Errorf("Event %d has nil metadata", i)
			continue
		}

		subAgent, ok := event.Metadata["sub_agent"].(string)
		if !ok {
			t.Errorf("Event %d missing sub_agent metadata", i)
			continue
		}

		if subAgent != "MathAgent" {
			t.Errorf("Event %d has wrong sub_agent: expected 'MathAgent', got '%s'", i, subAgent)
		}

		toolName, ok := event.Metadata["sub_agent_tool"].(string)
		if !ok {
			t.Errorf("Event %d missing sub_agent_tool metadata", i)
			continue
		}

		expectedToolName := "MathAgent_agent"
		if toolName != expectedToolName {
			t.Errorf("Event %d has wrong tool name: expected '%s', got '%s'", i, expectedToolName, toolName)
		}
	}
}

// TestInterfaceImplementation verifies that types correctly implement the interfaces
func TestInterfaceImplementation(t *testing.T) {
	// Verify MockStreamingAgent implements StreamingSubAgent
	var _ StreamingSubAgent = (*MockStreamingAgent)(nil)

	// Verify MockStreamingAgent implements SubAgent
	var _ SubAgent = (*MockStreamingAgent)(nil)
}
