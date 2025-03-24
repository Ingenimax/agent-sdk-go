package interfaces

import (
	"context"
	"time"
)

//-----------------------------------------------------------------------------
// Task Execution Results and Options
//-----------------------------------------------------------------------------

// TaskResult represents the result of a task execution
type TaskResult struct {
	// Data contains the result data
	Data interface{}
	// Error contains any error that occurred during task execution
	Error error
	// Metadata contains additional information about the task execution
	Metadata map[string]interface{}
}

// TaskOptions represents options for task execution
type TaskOptions struct {
	// Timeout specifies the maximum duration for task execution
	Timeout *time.Duration
	// RetryPolicy specifies the retry policy for the task
	RetryPolicy *RetryPolicy
	// Metadata contains additional information for the task execution
	Metadata map[string]interface{}
}

// RetryPolicy defines how tasks should be retried
type RetryPolicy struct {
	// MaxRetries is the maximum number of retries
	MaxRetries int
	// InitialBackoff is the initial backoff duration
	InitialBackoff time.Duration
	// MaxBackoff is the maximum backoff duration
	MaxBackoff time.Duration
	// BackoffMultiplier is the multiplier for backoff duration after each retry
	BackoffMultiplier float64
}

//-----------------------------------------------------------------------------
// Task Service Interfaces
//-----------------------------------------------------------------------------

// TaskService defines the interface for task management
// This is a unified interface combining task and core.Service
type TaskService interface {
	// CreateTask creates a new task
	CreateTask(ctx context.Context, req interface{}) (interface{}, error)
	// GetTask gets a task by ID
	GetTask(ctx context.Context, taskID string) (interface{}, error)
	// ListTasks returns tasks filtered by the provided criteria
	ListTasks(ctx context.Context, filter interface{}) ([]interface{}, error)
	// ApproveTaskPlan approves or rejects a task plan
	ApproveTaskPlan(ctx context.Context, taskID string, req interface{}) (interface{}, error)
	// UpdateTask updates an existing task with new steps or modifications
	UpdateTask(ctx context.Context, taskID string, updates interface{}) (interface{}, error)
	// AddTaskLog adds a log entry to a task
	AddTaskLog(ctx context.Context, taskID string, message string, level string) error
}

// TaskPlanner defines the interface for planning a task
type TaskPlanner interface {
	// CreatePlan creates a plan for a task
	CreatePlan(ctx context.Context, task interface{}) (string, error)
}

// TaskExecutor is the interface for executing tasks
type TaskExecutor interface {
	// ExecuteSync executes a task synchronously
	ExecuteSync(ctx context.Context, taskName string, params interface{}, opts *TaskOptions) (*TaskResult, error)

	// ExecuteAsync executes a task asynchronously and returns a channel for the result
	ExecuteAsync(ctx context.Context, taskName string, params interface{}, opts *TaskOptions) (<-chan *TaskResult, error)

	// ExecuteWorkflow initiates a temporal workflow
	ExecuteWorkflow(ctx context.Context, workflowName string, params interface{}, opts *TaskOptions) (*TaskResult, error)

	// ExecuteWorkflowAsync initiates a temporal workflow asynchronously
	ExecuteWorkflowAsync(ctx context.Context, workflowName string, params interface{}, opts *TaskOptions) (<-chan *TaskResult, error)

	// CancelTask cancels a running task
	CancelTask(ctx context.Context, taskID string) error

	// GetTaskStatus gets the status of a task
	GetTaskStatus(ctx context.Context, taskID string) (string, error)

	// ExecuteStep executes a single step in a task's plan
	ExecuteStep(ctx context.Context, task interface{}, step interface{}) error

	// ExecuteTask executes all steps in a task's plan
	ExecuteTask(ctx context.Context, task interface{}) error
}

//-----------------------------------------------------------------------------
// Generic Task Service Interface
//-----------------------------------------------------------------------------

// AgentTaskServiceInterface is a generic interface for agent task services
type AgentTaskServiceInterface[AgentTask any, AgentCreateRequest any, AgentApproveRequest any, AgentTaskUpdate any] interface {
	// CreateTask creates a new task
	CreateTask(ctx context.Context, req AgentCreateRequest) (AgentTask, error)

	// GetTask gets a task by ID
	GetTask(ctx context.Context, taskID string) (AgentTask, error)

	// ListTasks returns all tasks for a user
	ListTasks(ctx context.Context, userID string) ([]AgentTask, error)

	// ApproveTaskPlan approves or rejects a task plan
	ApproveTaskPlan(ctx context.Context, taskID string, req AgentApproveRequest) (AgentTask, error)

	// UpdateTask updates an existing task with new steps or modifications
	UpdateTask(ctx context.Context, taskID string, conversationID string, updates []AgentTaskUpdate) (AgentTask, error)

	// AddTaskLog adds a log entry to a task
	AddTaskLog(ctx context.Context, taskID string, message string, level string) error
}
