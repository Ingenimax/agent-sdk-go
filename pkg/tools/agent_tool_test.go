package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// MockSubAgent is a mock implementation of the SubAgent interface
type MockSubAgent struct {
	name          string
	description   string
	runFunc       func(ctx context.Context, input string) (string, error)
	runStreamFunc func(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error)
}

func (m *MockSubAgent) Run(ctx context.Context, input string) (string, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, input)
	}
	return "mock response: " + input, nil
}

func (m *MockSubAgent) RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error) {
	if m.runStreamFunc != nil {
		return m.runStreamFunc(ctx, input)
	}

	// Default implementation: create a simple streaming response
	eventChan := make(chan interfaces.AgentStreamEvent, 1)
	go func() {
		defer close(eventChan)
		result, err := m.Run(ctx, input)
		if err != nil {
			eventChan <- interfaces.AgentStreamEvent{
				Type:      interfaces.AgentEventError,
				Error:     err,
				Timestamp: time.Now(),
			}
		} else {
			eventChan <- interfaces.AgentStreamEvent{
				Type:      interfaces.AgentEventContent,
				Content:   result,
				Timestamp: time.Now(),
			}
			eventChan <- interfaces.AgentStreamEvent{
				Type:      interfaces.AgentEventComplete,
				Timestamp: time.Now(),
			}
		}
	}()
	return eventChan, nil
}

func (m *MockSubAgent) GetName() string {
	return m.name
}

func (m *MockSubAgent) GetDescription() string {
	return m.description
}

func TestNewAgentTool(t *testing.T) {
	mockAgent := &MockSubAgent{
		name:        "TestAgent",
		description: "A test agent for unit testing",
	}

	tool := NewAgentTool(mockAgent)

	// Verify tool name
	expectedName := "TestAgent_agent"
	if tool.Name() != expectedName {
		t.Errorf("Expected tool name %s, got %s", expectedName, tool.Name())
	}

	// Verify description
	if tool.Description() != "A test agent for unit testing" {
		t.Errorf("Expected description from agent, got %s", tool.Description())
	}

	// Verify parameters
	params := tool.Parameters()
	if _, ok := params["query"]; !ok {
		t.Error("Expected 'query' parameter in tool parameters")
	}
	if _, ok := params["context"]; !ok {
		t.Error("Expected 'context' parameter in tool parameters")
	}
}

func TestAgentToolRun(t *testing.T) {
	mockAgent := &MockSubAgent{
		name:        "TestAgent",
		description: "Test agent",
		runFunc: func(ctx context.Context, input string) (string, error) {
			// Check if context values are set
			if agentName := ctx.Value(subAgentNameKey); agentName != "TestAgent" {
				t.Errorf("Expected sub_agent_name context value to be TestAgent, got %v", agentName)
			}
			return "Processed: " + input, nil
		},
	}

	tool := NewAgentTool(mockAgent)
	ctx := context.Background()

	result, err := tool.Run(ctx, "test query")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result != "Processed: test query" {
		t.Errorf("Expected 'Processed: test query', got %s", result)
	}
}

func TestAgentToolExecute(t *testing.T) {
	mockAgent := &MockSubAgent{
		name:        "TestAgent",
		description: "Test agent",
		runFunc: func(ctx context.Context, input string) (string, error) {
			// Check if context value from params is set
			if val := ctx.Value(contextKey("test_key")); val != nil {
				if strVal, ok := val.(string); ok && strVal == "test_value" {
					return "Context received: " + input, nil
				}
			}
			return "No context: " + input, nil
		},
	}

	tool := NewAgentTool(mockAgent)
	ctx := context.Background()

	// Test with query only
	args := map[string]interface{}{
		"query": "test query",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(ctx, string(argsJSON))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result != "No context: test query" {
		t.Errorf("Expected 'No context: test query', got %s", result)
	}

	// Test with query and context
	args = map[string]interface{}{
		"query": "test query with context",
		"context": map[string]interface{}{
			"test_key": "test_value",
		},
	}
	argsJSON, _ = json.Marshal(args)

	result, err = tool.Execute(ctx, string(argsJSON))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result != "Context received: test query with context" {
		t.Errorf("Expected 'Context received: test query with context', got %s", result)
	}
}

func TestAgentToolTimeout(t *testing.T) {
	mockAgent := &MockSubAgent{
		name:        "SlowAgent",
		description: "A slow agent",
		runFunc: func(ctx context.Context, input string) (string, error) {
			// Simulate slow operation
			select {
			case <-time.After(2 * time.Second):
				return "completed", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	}

	tool := NewAgentTool(mockAgent).WithTimeout(100 * time.Millisecond)
	ctx := context.Background()

	_, err := tool.Run(ctx, "test")
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("Expected context deadline exceeded error, got %v", err)
	}
}

func TestRecursionDepthLimit(t *testing.T) {
	mockAgent := &MockSubAgent{
		name:        "RecursiveAgent",
		description: "A recursive agent",
	}

	tool := NewAgentTool(mockAgent)

	// Create a context with max recursion depth exceeded (6 > 5)
	ctx := context.Background()
	// Add 6 levels of sub-agent context to exceed the limit
	for i := 0; i < 6; i++ {
		ctx = withSubAgentContext(ctx, "parent", "child")
	}

	_, err := tool.Run(ctx, "test")
	if err == nil {
		t.Error("Expected recursion depth error, got nil")
		return
	}

	if !strings.Contains(err.Error(), "maximum recursion depth") || !strings.Contains(err.Error(), "exceeded") {
		t.Errorf("Expected maximum recursion depth exceeded error, got %v", err)
	}
}

func TestAgentToolWithEmptyQuery(t *testing.T) {
	mockAgent := &MockSubAgent{
		name:        "TestAgent",
		description: "Test agent",
	}

	tool := NewAgentTool(mockAgent)
	ctx := context.Background()

	// Test Execute with empty query
	args := map[string]interface{}{
		"query": "",
	}
	argsJSON, _ := json.Marshal(args)

	_, err := tool.Execute(ctx, string(argsJSON))
	if err == nil {
		t.Error("Expected error for empty query, got nil")
	}

	if !strings.Contains(err.Error(), "query parameter is required") {
		t.Errorf("Expected 'query parameter is required' error, got %v", err)
	}
}

func TestAgentToolDescription(t *testing.T) {
	// Test with description
	mockAgent := &MockSubAgent{
		name:        "TestAgent",
		description: "Custom description",
	}

	tool := NewAgentTool(mockAgent)
	if tool.Description() != "Custom description" {
		t.Errorf("Expected 'Custom description', got %s", tool.Description())
	}

	// Test without description (fallback)
	mockAgent2 := &MockSubAgent{
		name:        "TestAgent2",
		description: "",
	}

	tool2 := NewAgentTool(mockAgent2)
	expectedDesc := "Delegate task to TestAgent2 agent for specialized handling"
	if tool2.Description() != expectedDesc {
		t.Errorf("Expected '%s', got %s", expectedDesc, tool2.Description())
	}

	// Test SetDescription
	tool2.SetDescription("Updated description")
	if tool2.Description() != "Updated description" {
		t.Errorf("Expected 'Updated description', got %s", tool2.Description())
	}
}
