package adapter

import (
	"context"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task"
)

// This file is maintained for backward compatibility
// New code should use core_adapter.go implementation with the core package

// TaskAdapter defines a generic interface for adapting SDK task models
// to agent-specific models and vice versa. This pattern helps separate
// the concerns of the SDK from agent-specific implementations.
//
// Implementing this interface allows agents to use their own domain models
// while still leveraging the SDK's task management capabilities.
type TaskAdapter[AgentTask any, AgentCreateRequest any, AgentApprovalRequest any, AgentTaskUpdate any] interface {
	// ToSDK conversions (Agent -> SDK)

	// ConvertCreateRequest converts an agent-specific create request to an SDK create request
	ConvertCreateRequest(req AgentCreateRequest) task.CreateTaskRequest

	// ConvertApproveRequest converts an agent-specific approve request to an SDK approve request
	ConvertApproveRequest(req AgentApprovalRequest) task.ApproveTaskPlanRequest

	// ConvertTaskUpdates converts agent-specific task updates to SDK task updates
	ConvertTaskUpdates(updates []AgentTaskUpdate) []task.TaskUpdate

	// FromSDK conversions (SDK -> Agent)

	// ConvertTask converts an SDK task to an agent-specific task
	ConvertTask(sdkTask *task.Task) AgentTask

	// ConvertTasks converts a slice of SDK tasks to a slice of agent-specific tasks
	ConvertTasks(sdkTasks []*task.Task) []AgentTask
}

// AdapterOptions contains optional parameters for creating a task adapter
type AdapterOptions struct {
	// Additional options can be added here as needed
	IncludeMetadata bool
	DefaultUserID   string
}

// AdapterOption is a function that configures AdapterOptions
type AdapterOption func(*AdapterOptions)

// WithMetadata configures the adapter to include SDK metadata in conversions
func WithMetadata(include bool) AdapterOption {
	return func(opts *AdapterOptions) {
		opts.IncludeMetadata = include
	}
}

// WithDefaultUserID sets a default user ID for task creation
func WithDefaultUserID(userID string) AdapterOption {
	return func(opts *AdapterOptions) {
		opts.DefaultUserID = userID
	}
}

// AdapterService provides a generic service for adapting between SDK tasks and agent-specific tasks.
// It wraps the SDK's task service and provides methods for working with agent-specific task models.
type AdapterService[AgentTask any, AgentCreateRequest any, AgentApprovalRequest any, AgentTaskUpdate any] struct {
	sdkService interfaces.TaskService
	adapter    TaskAdapter[AgentTask, AgentCreateRequest, AgentApprovalRequest, AgentTaskUpdate]
	logger     logging.Logger
}

// NewAdapterService creates a new adapter service for adapting between SDK and agent-specific task models.
// It provides a simple way for agents to work with their own task models while leveraging the SDK's task service.
func NewAdapterService[AgentTask any, AgentCreateRequest any, AgentApprovalRequest any, AgentTaskUpdate any](
	logger logging.Logger,
	sdkService interfaces.TaskService,
	adapter TaskAdapter[AgentTask, AgentCreateRequest, AgentApprovalRequest, AgentTaskUpdate],
) *AdapterService[AgentTask, AgentCreateRequest, AgentApprovalRequest, AgentTaskUpdate] {
	return &AdapterService[AgentTask, AgentCreateRequest, AgentApprovalRequest, AgentTaskUpdate]{
		sdkService: sdkService,
		adapter:    adapter,
		logger:     logger,
	}
}

// CreateTask creates a new task using the agent-specific task model.
func (s *AdapterService[AgentTask, AgentCreateRequest, AgentApprovalRequest, AgentTaskUpdate]) CreateTask(
	ctx context.Context,
	req AgentCreateRequest,
) (AgentTask, error) {
	s.logger.Debug(ctx, "Creating new task via adapter service", nil)

	// Convert the request to SDK format
	sdkReq := s.adapter.ConvertCreateRequest(req)

	// Create task using SDK service
	sdkTaskObj, err := s.sdkService.CreateTask(ctx, sdkReq)
	if err != nil {
		s.logger.Error(ctx, "Failed to create task via SDK", map[string]interface{}{
			"error": err.Error(),
		})
		return *new(AgentTask), err
	}

	// Type assertion
	sdkTask, ok := sdkTaskObj.(*task.Task)
	if !ok {
		return *new(AgentTask), fmt.Errorf("unexpected type returned from core service")
	}

	// Convert the SDK task back to agent-specific format
	task := s.adapter.ConvertTask(sdkTask)

	return task, nil
}

// GetTask retrieves a task by ID and returns it in the agent-specific format.
func (s *AdapterService[AgentTask, AgentCreateRequest, AgentApprovalRequest, AgentTaskUpdate]) GetTask(
	ctx context.Context,
	taskID string,
) (AgentTask, error) {
	// Get task using SDK service
	sdkTaskObj, err := s.sdkService.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Error(ctx, "Failed to get task via SDK", map[string]interface{}{
			"error":   err.Error(),
			"task_id": taskID,
		})
		return *new(AgentTask), err
	}

	// Type assertion
	sdkTask, ok := sdkTaskObj.(*task.Task)
	if !ok {
		return *new(AgentTask), fmt.Errorf("unexpected type returned from core service")
	}

	// Convert the SDK task back to agent-specific format
	task := s.adapter.ConvertTask(sdkTask)

	return task, nil
}

// ListTasks returns all tasks for a user in the agent-specific format.
func (s *AdapterService[AgentTask, AgentCreateRequest, AgentApprovalRequest, AgentTaskUpdate]) ListTasks(
	ctx context.Context,
	userID string,
) ([]AgentTask, error) {
	// Create filter for SDK service
	filter := task.TaskFilter{
		UserID: userID,
	}

	// List tasks using SDK service
	sdkTaskObjList, err := s.sdkService.ListTasks(ctx, filter)
	if err != nil {
		s.logger.Error(ctx, "Failed to list tasks via SDK", map[string]interface{}{
			"error":   err.Error(),
			"user_id": userID,
		})
		return nil, err
	}

	// Convert interface{} list to []*task.Task
	var sdkTasks []*task.Task
	for _, obj := range sdkTaskObjList {
		if sdkTask, ok := obj.(*task.Task); ok {
			sdkTasks = append(sdkTasks, sdkTask)
		}
	}

	// Convert the SDK tasks back to agent-specific format
	tasks := s.adapter.ConvertTasks(sdkTasks)

	return tasks, nil
}

// ApproveTaskPlan approves or rejects a task plan in the agent-specific format.
func (s *AdapterService[AgentTask, AgentCreateRequest, AgentApprovalRequest, AgentTaskUpdate]) ApproveTaskPlan(
	ctx context.Context,
	taskID string,
	req AgentApprovalRequest,
) (AgentTask, error) {
	// Convert the request to SDK format
	sdkReq := s.adapter.ConvertApproveRequest(req)

	// Approve or reject the task plan using SDK service
	sdkTaskObj, err := s.sdkService.ApproveTaskPlan(ctx, taskID, sdkReq)
	if err != nil {
		s.logger.Error(ctx, "Failed to approve/reject task plan via SDK", map[string]interface{}{
			"error":   err.Error(),
			"task_id": taskID,
		})
		return *new(AgentTask), err
	}

	// Type assertion
	sdkTask, ok := sdkTaskObj.(*task.Task)
	if !ok {
		return *new(AgentTask), fmt.Errorf("unexpected type returned from core service")
	}

	// Convert the SDK task back to agent-specific format
	task := s.adapter.ConvertTask(sdkTask)

	return task, nil
}

// UpdateTask updates an existing task in the agent-specific format.
func (s *AdapterService[AgentTask, AgentCreateRequest, AgentApprovalRequest, AgentTaskUpdate]) UpdateTask(
	ctx context.Context,
	taskID string,
	conversationID string,
	updates []AgentTaskUpdate,
) (AgentTask, error) {
	// Convert the updates to SDK format
	sdkUpdates := s.adapter.ConvertTaskUpdates(updates)

	// If a conversation ID is provided, first get the task to update it
	if conversationID != "" {
		sdkTaskObj, err := s.sdkService.GetTask(ctx, taskID)
		if err != nil {
			s.logger.Error(ctx, "Failed to get task for updating via SDK", map[string]interface{}{
				"error":   err.Error(),
				"task_id": taskID,
			})
			return *new(AgentTask), err
		}

		// Type assertion
		sdkTask, ok := sdkTaskObj.(*task.Task)
		if !ok {
			return *new(AgentTask), fmt.Errorf("unexpected type returned from core service")
		}

		// Update the conversation ID
		sdkTask.ConversationID = conversationID

		// TODO: Save the task with updated conversation ID
	}

	// Update the task using SDK service
	sdkTaskObj, err := s.sdkService.UpdateTask(ctx, taskID, sdkUpdates)
	if err != nil {
		s.logger.Error(ctx, "Failed to update task via SDK", map[string]interface{}{
			"error":   err.Error(),
			"task_id": taskID,
		})
		return *new(AgentTask), err
	}

	// Type assertion
	sdkTask, ok := sdkTaskObj.(*task.Task)
	if !ok {
		return *new(AgentTask), fmt.Errorf("unexpected type returned from core service")
	}

	// Convert the SDK task back to agent-specific format
	task := s.adapter.ConvertTask(sdkTask)

	return task, nil
}

// AddTaskLog adds a log entry to a task.
func (s *AdapterService[AgentTask, AgentCreateRequest, AgentApprovalRequest, AgentTaskUpdate]) AddTaskLog(
	ctx context.Context,
	taskID string,
	message string,
	level string,
) error {
	// Add log entry using SDK service
	err := s.sdkService.AddTaskLog(ctx, taskID, message, level)
	if err != nil {
		s.logger.Error(ctx, "Failed to add log entry via SDK", map[string]interface{}{
			"error":   err.Error(),
			"task_id": taskID,
		})
		return err
	}

	return nil
}
