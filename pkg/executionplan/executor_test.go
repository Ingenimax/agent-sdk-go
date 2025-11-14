package executionplan

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// mockTool implements interfaces.Tool for testing
type mockTool struct {
	name           string
	description    string
	lastExecuteArg string
	executeErr     error
	executeResult  string
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) Run(ctx context.Context, input string) (string, error) {
	return m.executeResult, m.executeErr
}

func (m *mockTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"query": {
			Type:        "string",
			Description: "Test query parameter",
			Required:    true,
		},
	}
}

func (m *mockTool) Execute(ctx context.Context, args string) (string, error) {
	m.lastExecuteArg = args
	return m.executeResult, m.executeErr
}

func TestExecutePlan_WithParameters(t *testing.T) {
	// Create mock tool
	mockTool := &mockTool{
		name:          "test_tool",
		description:   "A test tool",
		executeResult: "success",
	}

	// Create executor
	executor := NewExecutor([]interfaces.Tool{mockTool})

	// Create execution plan with parameters
	plan := &ExecutionPlan{
		Description:  "Test plan with parameters",
		UserApproved: true,
		Steps: []ExecutionStep{
			{
				ToolName:    "test_tool",
				Description: "Test step with parameters",
				Input:       "plain string input", // This should be ignored
				Parameters: map[string]interface{}{
					"query": "test query",
					"count": 5,
				},
			},
		},
	}

	// Execute plan
	ctx := context.Background()
	result, err := executor.ExecutePlan(ctx, plan)

	// Verify no error
	if err != nil {
		t.Fatalf("ExecutePlan failed: %v", err)
	}

	// Verify result
	if result == "" {
		t.Error("Expected non-empty result")
	}

	// Verify the tool received JSON with parameters
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(mockTool.lastExecuteArg), &params); err != nil {
		t.Fatalf("Tool did not receive valid JSON: %v, received: %s", err, mockTool.lastExecuteArg)
	}

	// Check specific parameters
	if params["query"] != "test query" {
		t.Errorf("Expected query='test query', got %v", params["query"])
	}
	if params["count"] != float64(5) {
		t.Errorf("Expected count=5, got %v", params["count"])
	}
}

func TestExecutePlan_FallbackToInput(t *testing.T) {
	// Create mock tool
	mockTool := &mockTool{
		name:          "test_tool",
		description:   "A test tool",
		executeResult: "success",
	}

	// Create executor
	executor := NewExecutor([]interfaces.Tool{mockTool})

	// Create execution plan with only Input (no Parameters)
	jsonInput := `{"query": "fallback query"}`
	plan := &ExecutionPlan{
		Description:  "Test plan with input fallback",
		UserApproved: true,
		Steps: []ExecutionStep{
			{
				ToolName:    "test_tool",
				Description: "Test step with input only",
				Input:       jsonInput,
				Parameters:  nil, // No parameters
			},
		},
	}

	// Execute plan
	ctx := context.Background()
	_, err := executor.ExecutePlan(ctx, plan)

	// Verify no error
	if err != nil {
		t.Fatalf("ExecutePlan failed: %v", err)
	}

	// Verify the tool received the input as-is
	if mockTool.lastExecuteArg != jsonInput {
		t.Errorf("Expected tool to receive '%s', got '%s'", jsonInput, mockTool.lastExecuteArg)
	}
}

func TestExecutePlan_EmptyInputAndParameters(t *testing.T) {
	// Create mock tool
	mockTool := &mockTool{
		name:          "test_tool",
		description:   "A test tool",
		executeResult: "success",
	}

	// Create executor
	executor := NewExecutor([]interfaces.Tool{mockTool})

	// Create execution plan with neither Input nor Parameters
	plan := &ExecutionPlan{
		Description:  "Test plan with empty input",
		UserApproved: true,
		Steps: []ExecutionStep{
			{
				ToolName:    "test_tool",
				Description: "Test step with no input",
				Input:       "",
				Parameters:  nil,
			},
		},
	}

	// Execute plan
	ctx := context.Background()
	_, err := executor.ExecutePlan(ctx, plan)

	// Verify no error
	if err != nil {
		t.Fatalf("ExecutePlan failed: %v", err)
	}

	// Verify the tool received empty JSON object
	if mockTool.lastExecuteArg != "{}" {
		t.Errorf("Expected tool to receive '{}', got '%s'", mockTool.lastExecuteArg)
	}
}

func TestExecutePlan_NotApproved(t *testing.T) {
	executor := NewExecutor([]interfaces.Tool{})

	plan := &ExecutionPlan{
		Description:  "Test plan",
		UserApproved: false, // Not approved
		Steps:        []ExecutionStep{},
	}

	ctx := context.Background()
	_, err := executor.ExecutePlan(ctx, plan)

	if err == nil {
		t.Error("Expected error for non-approved plan")
	}

	if err.Error() != "execution plan has not been approved by the user" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestExecutePlan_UnknownTool(t *testing.T) {
	executor := NewExecutor([]interfaces.Tool{})

	plan := &ExecutionPlan{
		Description:  "Test plan",
		UserApproved: true,
		Steps: []ExecutionStep{
			{
				ToolName:    "unknown_tool",
				Description: "Test step",
			},
		},
	}

	ctx := context.Background()
	_, err := executor.ExecutePlan(ctx, plan)

	if err == nil {
		t.Error("Expected error for unknown tool")
	}

	if plan.Status != StatusFailed {
		t.Errorf("Expected plan status to be Failed, got %v", plan.Status)
	}
}

func TestExecutePlan_MarshalError(t *testing.T) {
	// Create mock tool
	mockTool := &mockTool{
		name:          "test_tool",
		description:   "A test tool",
		executeResult: "success",
	}

	executor := NewExecutor([]interfaces.Tool{mockTool})

	// Create a parameter that cannot be marshaled to JSON
	unmarshalable := make(chan int)

	plan := &ExecutionPlan{
		Description:  "Test plan",
		UserApproved: true,
		Steps: []ExecutionStep{
			{
				ToolName:    "test_tool",
				Description: "Test step",
				Parameters: map[string]interface{}{
					"channel": unmarshalable, // Channels cannot be marshaled to JSON
				},
			},
		},
	}

	ctx := context.Background()
	_, err := executor.ExecutePlan(ctx, plan)

	if err == nil {
		t.Error("Expected error for unmarshalable parameters")
	}

	expectedError := "failed to marshal parameters for step 1"
	if err != nil && !testContains(err.Error(), expectedError) {
		t.Errorf("Expected error containing '%s', got: %v", expectedError, err)
	}

	if plan.Status != StatusFailed {
		t.Errorf("Expected plan status to be Failed, got %v", plan.Status)
	}
}

func TestExecutePlan_ToolExecuteError(t *testing.T) {
	// Create mock tool that returns an error
	mockTool := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		executeErr:  fmt.Errorf("tool execution failed"),
	}

	executor := NewExecutor([]interfaces.Tool{mockTool})

	plan := &ExecutionPlan{
		Description:  "Test plan",
		UserApproved: true,
		Steps: []ExecutionStep{
			{
				ToolName:    "test_tool",
				Description: "Test step",
				Parameters: map[string]interface{}{
					"query": "test",
				},
			},
		},
	}

	ctx := context.Background()
	_, err := executor.ExecutePlan(ctx, plan)

	if err == nil {
		t.Error("Expected error from tool execution")
	}

	if plan.Status != StatusFailed {
		t.Errorf("Expected plan status to be Failed, got %v", plan.Status)
	}
}

// Helper function to check if a string contains a substring
func testContains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || len(s) > len(substr) && testContains(s[1:], substr)
}
