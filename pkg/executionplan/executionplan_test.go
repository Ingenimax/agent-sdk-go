package executionplan

import (
	"strings"
	"testing"
	"time"
)

func TestNewExecutionPlan(t *testing.T) {
	description := "Test plan"
	steps := []ExecutionStep{
		{
			ToolName:    "test_tool",
			Description: "Test step",
			Input:       "test input",
			Parameters: map[string]interface{}{
				"param1": "value1",
			},
		},
	}

	plan := NewExecutionPlan(description, steps)

	if plan.Description != description {
		t.Errorf("Expected description %s, got %s", description, plan.Description)
	}

	if len(plan.Steps) != len(steps) {
		t.Errorf("Expected %d steps, got %d", len(steps), len(plan.Steps))
	}

	if plan.UserApproved {
		t.Errorf("Expected UserApproved to be false, got true")
	}

	if plan.TaskID == "" {
		t.Errorf("Expected TaskID to be non-empty")
	}

	if plan.Status != StatusDraft {
		t.Errorf("Expected Status to be %s, got %s", StatusDraft, plan.Status)
	}

	if time.Since(plan.CreatedAt) > time.Minute {
		t.Errorf("CreatedAt time is too old: %v", plan.CreatedAt)
	}

	if time.Since(plan.UpdatedAt) > time.Minute {
		t.Errorf("UpdatedAt time is too old: %v", plan.UpdatedAt)
	}
}

func TestFormatExecutionPlan(t *testing.T) {
	plan := &ExecutionPlan{
		Description: "Test plan",
		TaskID:      "test-id",
		Status:      StatusDraft,
		Steps: []ExecutionStep{
			{
				ToolName:    "test_tool",
				Description: "Test step",
				Input:       "test input",
				Parameters: map[string]interface{}{
					"param1": "value1",
				},
			},
		},
	}

	formatted := FormatExecutionPlan(plan)

	// Basic checks for expected content
	expectedStrings := []string{
		"# Execution Plan: Test plan",
		"Task ID: test-id",
		"Status: draft",
		"## Step 1: Test step",
		"Tool: test_tool",
		"Input: test input",
		"Parameters:",
		"param1: value1",
	}

	for _, expected := range expectedStrings {
		if !contains(formatted, expected) {
			t.Errorf("Expected formatted plan to contain '%s', but it didn't", expected)
		}
	}
}

func TestParseExecutionPlanFromResponse(t *testing.T) {
	response := `
Some text before JSON
{
  "description": "Test plan",
  "steps": [
    {
      "toolName": "test_tool",
      "description": "Test step",
      "input": "test input",
      "parameters": {
        "param1": "value1"
      }
    }
  ]
}
Some text after JSON
`

	plan, err := ParseExecutionPlanFromResponse(response)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if plan.Description != "Test plan" {
		t.Errorf("Expected description 'Test plan', got '%s'", plan.Description)
	}

	if len(plan.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(plan.Steps))
	}

	step := plan.Steps[0]
	if step.ToolName != "test_tool" {
		t.Errorf("Expected tool name 'test_tool', got '%s'", step.ToolName)
	}

	if step.Description != "Test step" {
		t.Errorf("Expected step description 'Test step', got '%s'", step.Description)
	}

	if step.Input != "test input" {
		t.Errorf("Expected step input 'test input', got '%s'", step.Input)
	}

	paramValue, exists := step.Parameters["param1"]
	if !exists {
		t.Errorf("Expected parameter 'param1' to exist")
	} else if paramValue != "value1" {
		t.Errorf("Expected parameter 'param1' to have value 'value1', got '%v'", paramValue)
	}
}

func TestParseExecutionPlanFromResponse_InvalidJSON(t *testing.T) {
	response := `This is not valid JSON`

	_, err := ParseExecutionPlanFromResponse(response)
	if err == nil {
		t.Fatalf("Expected error for invalid JSON, got nil")
	}
}

// Helper function to check if a string contains another string
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
