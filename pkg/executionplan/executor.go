package executionplan

import (
	"context"
	"fmt"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Executor handles execution of execution plans
type Executor struct {
	tools map[string]interfaces.Tool
}

// NewExecutor creates a new execution plan executor
func NewExecutor(tools []interfaces.Tool) *Executor {
	toolMap := make(map[string]interfaces.Tool)
	for _, tool := range tools {
		toolMap[tool.Name()] = tool
	}

	return &Executor{
		tools: toolMap,
	}
}

// ExecutePlan executes an approved execution plan
func (e *Executor) ExecutePlan(ctx context.Context, plan *ExecutionPlan) (string, error) {
	if !plan.UserApproved {
		return "", fmt.Errorf("execution plan has not been approved by the user")
	}

	// Update status to executing
	plan.Status = StatusExecuting

	// Execute each step in the plan
	results := make([]string, 0, len(plan.Steps))
	for i, step := range plan.Steps {
		// Get the tool
		tool, ok := e.tools[step.ToolName]
		if !ok {
			plan.Status = StatusFailed
			return "", fmt.Errorf("unknown tool: %s", step.ToolName)
		}

		fmt.Println("step.Input", step.Input)
		// Execute the tool
		result, err := tool.Execute(ctx, step.Input)
		if err != nil {
			plan.Status = StatusFailed
			return "", fmt.Errorf("failed to execute step %d: %w", i+1, err)
		}

		// Add the result to the list of results
		results = append(results, fmt.Sprintf("Step %d (%s): %s", i+1, step.Description, result))
	}

	// Update status to completed
	plan.Status = StatusCompleted

	// Format the results
	return fmt.Sprintf("Execution plan completed successfully!\n\n%s", strings.Join(results, "\n\n")), nil
}

// CancelPlan cancels an execution plan
func (e *Executor) CancelPlan(plan *ExecutionPlan) {
	plan.Status = StatusCancelled
}

// GetPlanStatus returns the status of an execution plan
func (e *Executor) GetPlanStatus(plan *ExecutionPlan) ExecutionPlanStatus {
	return plan.Status
}
