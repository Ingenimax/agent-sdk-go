package adapter

import (
	"context"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
)

// Service defines the interface for task management from the task package
// This is defined here to avoid import cycles
type Service interface {
	// CreateTask creates a new task
	CreateTask(ctx context.Context, req task.CreateTaskRequest) (*task.Task, error)
	// GetTask gets a task by ID
	GetTask(ctx context.Context, taskID string) (*task.Task, error)
	// ListTasks returns tasks filtered by the provided criteria
	ListTasks(ctx context.Context, filter task.TaskFilter) ([]*task.Task, error)
	// ApproveTaskPlan approves or rejects a task plan
	ApproveTaskPlan(ctx context.Context, taskID string, req task.ApproveTaskPlanRequest) (*task.Task, error)
	// UpdateTask updates an existing task with new steps or modifications
	UpdateTask(ctx context.Context, taskID string, updates []task.TaskUpdate) (*task.Task, error)
	// AddTaskLog adds a log entry to a task
	AddTaskLog(ctx context.Context, taskID string, message string, level string) error
}

// CoreBridgeAdapter provides a bridge between the old task.Service interface and the new interfaces.TaskService
// This allows migrating to the new core interfaces without breaking existing code
type CoreBridgeAdapter struct {
	coreService interfaces.TaskService
	logger      logging.Logger
}

// NewCoreBridgeAdapter creates a new bridge adapter
func NewCoreBridgeAdapter(coreService interfaces.TaskService, logger logging.Logger) Service {
	return &CoreBridgeAdapter{
		coreService: coreService,
		logger:      logger,
	}
}

// CreateTask creates a new task
func (a *CoreBridgeAdapter) CreateTask(ctx context.Context, req task.CreateTaskRequest) (*task.Task, error) {
	// Convert the request to core format
	coreReq := core.CreateTaskRequest{
		Name:           req.Title,
		Description:    req.Description,
		UserID:         req.UserID,
		ConversationID: "",
		Input:          nil,
		Metadata:       req.Metadata,
	}

	// Call the core service
	coreTaskObj, err := a.coreService.CreateTask(ctx, coreReq)
	if err != nil {
		return nil, err
	}

	// Type assertion
	coreTask, ok := coreTaskObj.(*core.Task)
	if !ok {
		return nil, fmt.Errorf("unexpected type returned from core service")
	}

	// Convert the response back to task format
	return a.coreTaskToTask(coreTask), nil
}

// GetTask gets a task by ID
func (a *CoreBridgeAdapter) GetTask(ctx context.Context, taskID string) (*task.Task, error) {
	// Call the core service
	coreTaskObj, err := a.coreService.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	// Type assertion
	coreTask, ok := coreTaskObj.(*core.Task)
	if !ok {
		return nil, fmt.Errorf("unexpected type returned from core service")
	}

	// Convert the response back to task format
	return a.coreTaskToTask(coreTask), nil
}

// ListTasks returns tasks filtered by the provided criteria
func (a *CoreBridgeAdapter) ListTasks(ctx context.Context, filter task.TaskFilter) ([]*task.Task, error) {
	// Convert the filter to core format
	coreFilter := core.TaskFilter{
		UserID:         filter.UserID,
		ConversationID: filter.ConversationID,
		Status:         a.convertStatusFilter(filter.Status),
		FromDate:       filter.CreatedAfter,
		ToDate:         filter.CreatedBefore,
	}

	// Call the core service
	coreTaskObjList, err := a.coreService.ListTasks(ctx, coreFilter)
	if err != nil {
		return nil, err
	}

	// Convert the response back to task format
	tasks := make([]*task.Task, 0, len(coreTaskObjList))
	for _, coreTaskObj := range coreTaskObjList {
		// Type assertion
		coreTask, ok := coreTaskObj.(*core.Task)
		if !ok {
			continue // Skip items that don't match the expected type
		}
		tasks = append(tasks, a.coreTaskToTask(coreTask))
	}

	return tasks, nil
}

// ApproveTaskPlan approves or rejects a task plan
func (a *CoreBridgeAdapter) ApproveTaskPlan(ctx context.Context, taskID string, req task.ApproveTaskPlanRequest) (*task.Task, error) {
	// Convert the request to core format
	coreReq := core.ApproveTaskPlanRequest{
		Approved: req.Approved,
		Feedback: req.Feedback,
	}

	// Call the core service
	coreTaskObj, err := a.coreService.ApproveTaskPlan(ctx, taskID, coreReq)
	if err != nil {
		return nil, err
	}

	// Type assertion
	coreTask, ok := coreTaskObj.(*core.Task)
	if !ok {
		return nil, fmt.Errorf("unexpected type returned from core service")
	}

	// Convert the response back to task format
	return a.coreTaskToTask(coreTask), nil
}

// UpdateTask updates an existing task with new steps or modifications
func (a *CoreBridgeAdapter) UpdateTask(ctx context.Context, taskID string, updates []task.TaskUpdate) (*task.Task, error) {
	// Convert the updates to core format
	coreUpdates := make([]core.TaskUpdate, len(updates))
	for i, update := range updates {
		coreUpdates[i] = a.taskUpdateToCoreUpdate(update)
	}

	// Call the core service
	coreTaskObj, err := a.coreService.UpdateTask(ctx, taskID, coreUpdates)
	if err != nil {
		return nil, err
	}

	// Type assertion
	coreTask, ok := coreTaskObj.(*core.Task)
	if !ok {
		return nil, fmt.Errorf("unexpected type returned from core service")
	}

	// Convert the response back to task format
	return a.coreTaskToTask(coreTask), nil
}

// AddTaskLog adds a log entry to a task
func (a *CoreBridgeAdapter) AddTaskLog(ctx context.Context, taskID string, message string, level string) error {
	// Call the core service
	return a.coreService.AddTaskLog(ctx, taskID, message, level)
}

// Helper methods for converting between core and task types

func (a *CoreBridgeAdapter) coreTaskToTask(coreTask *core.Task) *task.Task {
	if coreTask == nil {
		return nil
	}

	// Create a task.Task from a core.Task
	result := &task.Task{
		ID:             coreTask.ID,
		Description:    coreTask.Description,
		Status:         a.coreStatusToTaskStatus(coreTask.Status),
		Title:          coreTask.Name,
		ConversationID: coreTask.ConversationID,
		CreatedAt:      coreTask.CreatedAt,
		UpdatedAt:      coreTask.UpdatedAt,
		CompletedAt:    coreTask.CompletedAt,
		UserID:         coreTask.UserID,
		Metadata:       coreTask.Metadata,
	}

	// Convert steps
	if len(coreTask.Steps) > 0 {
		steps := make([]task.Step, len(coreTask.Steps))
		for i, coreStep := range coreTask.Steps {
			steps[i] = a.coreStepToTaskStep(coreStep)
		}
		result.Steps = steps
	}

	// Create a simple plan if plan string is available
	if coreTask.Plan != "" {
		result.Plan = &task.Plan{
			ID:         coreTask.ID + "_plan",
			TaskID:     coreTask.ID,
			CreatedAt:  coreTask.CreatedAt,
			IsApproved: coreTask.Status == core.StatusExecuting || coreTask.Status == core.StatusCompleted,
			Steps:      result.Steps,
		}
	}

	return result
}

func (a *CoreBridgeAdapter) coreStepToTaskStep(coreStep *core.Step) task.Step {
	var output string
	if coreStep.Output != nil {
		// Convert map to string representation
		if result, ok := coreStep.Output["result"]; ok {
			if str, ok := result.(string); ok {
				output = str
			}
		}
	}

	return task.Step{
		ID:          coreStep.ID,
		PlanID:      coreStep.ID + "_plan", // Placeholder
		Description: coreStep.Description,
		Status:      a.coreStatusToTaskStatus(coreStep.Status),
		Order:       coreStep.OrderIndex,
		StartedAt:   nil, // Not available directly
		CompletedAt: coreStep.CompletedAt,
		Error:       coreStep.Error,
		Output:      output,
	}
}

func (a *CoreBridgeAdapter) coreStatusToTaskStatus(coreStatus core.Status) task.Status {
	switch coreStatus {
	case core.StatusPending:
		return task.StatusPending
	case core.StatusPlanning:
		return task.StatusPlanning
	case core.StatusAwaitingApproval:
		return task.StatusApproval
	case core.StatusExecuting:
		return task.StatusExecuting
	case core.StatusCompleted:
		return task.StatusCompleted
	case core.StatusFailed:
		return task.StatusFailed
	default:
		return task.StatusPending
	}
}

func (a *CoreBridgeAdapter) taskStatusToCoreStatus(taskStatus task.Status) core.Status {
	switch taskStatus {
	case task.StatusPending:
		return core.StatusPending
	case task.StatusPlanning:
		return core.StatusPlanning
	case task.StatusApproval:
		return core.StatusAwaitingApproval
	case task.StatusExecuting:
		return core.StatusExecuting
	case task.StatusCompleted:
		return core.StatusCompleted
	case task.StatusFailed:
		return core.StatusFailed
	default:
		return core.StatusPending
	}
}

func (a *CoreBridgeAdapter) convertStatusFilter(taskStatuses []task.Status) core.Status {
	// This is a simplification since core.TaskFilter only accepts a single status
	if len(taskStatuses) > 0 {
		return a.taskStatusToCoreStatus(taskStatuses[0])
	}
	return ""
}

func (a *CoreBridgeAdapter) taskUpdateToCoreUpdate(update task.TaskUpdate) core.TaskUpdate {
	// Map task update types to core update fields
	switch update.Type {
	case "add_step":
		return core.TaskUpdate{
			Field: "add_step",
			Value: map[string]interface{}{
				"name":        "Step",
				"description": update.Description,
				"type":        "task",
			},
		}
	case "update_status":
		return core.TaskUpdate{
			Field: "status",
			Value: string(a.taskStatusToCoreStatus(task.Status(update.Status))),
		}
	case "modify_step":
		return core.TaskUpdate{
			Field: "update_step",
			Value: map[string]interface{}{
				"id":     update.StepID,
				"status": string(a.taskStatusToCoreStatus(task.Status(update.Status))),
			},
		}
	default:
		// Default simple mapping
		return core.TaskUpdate{
			Field: update.Type,
			Value: update.Description,
		}
	}
}
