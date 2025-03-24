package core

import (
	"time"
)

// Status represents the current status of a task or step
type Status string

const (
	// StatusPending indicates that a task or step is waiting to be executed
	StatusPending Status = "pending"
	// StatusPlanning indicates that a task is in the planning phase
	StatusPlanning Status = "planning"
	// StatusAwaitingApproval indicates that a task plan is waiting for approval
	StatusAwaitingApproval Status = "awaiting_approval"
	// StatusExecuting indicates that a task or step is currently being executed
	StatusExecuting Status = "executing"
	// StatusCompleted indicates that a task or step has been successfully completed
	StatusCompleted Status = "completed"
	// StatusFailed indicates that a task or step has failed
	StatusFailed Status = "failed"
	// StatusCancelled indicates that a task has been cancelled
	StatusCancelled Status = "cancelled"
)

// Task represents a task to be executed
type Task struct {
	// ID is the unique identifier for the task
	ID string `json:"id"`
	// Name is a human-readable name for the task
	Name string `json:"name"`
	// Description is a human-readable description of the task
	Description string `json:"description"`
	// Status is the current status of the task
	Status Status `json:"status"`
	// Steps are the individual steps that make up the task
	Steps []*Step `json:"steps"`
	// UserID is the ID of the user who created the task
	UserID string `json:"user_id"`
	// Plan is the plan for executing the task
	Plan string `json:"plan,omitempty"`
	// CreatedAt is the time when the task was created
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the time when the task was last updated
	UpdatedAt time.Time `json:"updated_at"`
	// CompletedAt is the time when the task was completed, if applicable
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// FailedAt is the time when the task failed, if applicable
	FailedAt *time.Time `json:"failed_at,omitempty"`
	// ConversationID is the ID of the conversation associated with the task
	ConversationID string `json:"conversation_id,omitempty"`
	// Input is the input provided for the task
	Input map[string]interface{} `json:"input,omitempty"`
	// Output is the output produced by the task
	Output map[string]interface{} `json:"output,omitempty"`
	// Metadata contains additional data about the task
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Step represents a single step in a task
type Step struct {
	// ID is the unique identifier for the step
	ID string `json:"id"`
	// Name is a human-readable name for the step
	Name string `json:"name"`
	// Description is a human-readable description of the step
	Description string `json:"description"`
	// Status is the current status of the step
	Status Status `json:"status"`
	// Type is the type of step (e.g., "chat", "execute_code", etc.)
	Type string `json:"type"`
	// Context is additional context for the step
	Context map[string]interface{} `json:"context,omitempty"`
	// CreatedAt is the time when the step was created
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the time when the step was last updated
	UpdatedAt time.Time `json:"updated_at"`
	// CompletedAt is the time when the step was completed, if applicable
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// FailedAt is the time when the step failed, if applicable
	FailedAt *time.Time `json:"failed_at,omitempty"`
	// Error is the error message if the step failed
	Error string `json:"error,omitempty"`
	// Output is the output produced by the step
	Output map[string]interface{} `json:"output,omitempty"`
	// OrderIndex is the order of the step in the task
	OrderIndex int `json:"order_index"`
}

// Log represents a log entry for a task
type Log struct {
	// ID is the unique identifier for the log entry
	ID string `json:"id"`
	// TaskID is the ID of the task this log entry is associated with
	TaskID string `json:"task_id"`
	// Message is the log message
	Message string `json:"message"`
	// Level is the log level (e.g., "info", "error", etc.)
	Level string `json:"level"`
	// CreatedAt is the time when the log entry was created
	CreatedAt time.Time `json:"created_at"`
}

// CreateTaskRequest is the request to create a new task
type CreateTaskRequest struct {
	// Name is a human-readable name for the task
	Name string `json:"name"`
	// Description is a human-readable description of the task
	Description string `json:"description"`
	// UserID is the ID of the user who is creating the task
	UserID string `json:"user_id"`
	// ConversationID is the ID of the conversation associated with the task
	ConversationID string `json:"conversation_id,omitempty"`
	// Input is the input provided for the task
	Input map[string]interface{} `json:"input,omitempty"`
	// Metadata contains additional data about the task
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ApproveTaskPlanRequest is the request to approve or reject a task plan
type ApproveTaskPlanRequest struct {
	// Approved indicates whether the plan was approved or rejected
	Approved bool `json:"approved"`
	// Feedback is optional feedback on the plan
	Feedback string `json:"feedback,omitempty"`
}

// TaskUpdate represents an update to a task
type TaskUpdate struct {
	// Field is the field to update
	Field string `json:"field"`
	// Value is the new value for the field
	Value interface{} `json:"value"`
}

// TaskFilter defines criteria for filtering tasks
type TaskFilter struct {
	// UserID filters tasks by user ID
	UserID string `json:"user_id,omitempty"`
	// Status filters tasks by status
	Status Status `json:"status,omitempty"`
	// ConversationID filters tasks by conversation ID
	ConversationID string `json:"conversation_id,omitempty"`
	// FromDate filters tasks created on or after this date
	FromDate *time.Time `json:"from_date,omitempty"`
	// ToDate filters tasks created on or before this date
	ToDate *time.Time `json:"to_date,omitempty"`
	// Limit limits the number of tasks returned
	Limit int `json:"limit,omitempty"`
	// Offset specifies the offset for pagination
	Offset int `json:"offset,omitempty"`
}
