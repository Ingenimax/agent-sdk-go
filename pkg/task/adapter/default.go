package adapter

import (
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task"
)

// DefaultTask is a default implementation of an agent task
type DefaultTask struct {
	ID             string                 `json:"id"`
	Description    string                 `json:"description"`
	Status         string                 `json:"status"`
	Title          string                 `json:"title,omitempty"`
	TaskKind       string                 `json:"task_kind,omitempty"`
	ConversationID string                 `json:"conversation_id,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	StartedAt      *time.Time             `json:"started_at,omitempty"`
	CompletedAt    *time.Time             `json:"completed_at,omitempty"`
	UserID         string                 `json:"user_id"`
	Steps          []DefaultStep          `json:"steps,omitempty"`
	Requirements   interface{}            `json:"requirements,omitempty"`
	Feedback       string                 `json:"feedback,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// DefaultStep is a default implementation of a task step
type DefaultStep struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	StartTime   *time.Time `json:"start_time,omitempty"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	Error       string     `json:"error,omitempty"`
	Output      string     `json:"output,omitempty"`
}

// DefaultCreateRequest is a default implementation of a task creation request
type DefaultCreateRequest struct {
	Description string `json:"description"`
	UserID      string `json:"user_id"`
	Title       string `json:"title,omitempty"`
	TaskKind    string `json:"task_kind,omitempty"`
}

// DefaultApproveRequest is a default implementation of a task approval request
type DefaultApproveRequest struct {
	Approved bool   `json:"approved"`
	Feedback string `json:"feedback,omitempty"`
}

// DefaultTaskUpdate is a default implementation of a task update
type DefaultTaskUpdate struct {
	Type        string `json:"type"`
	StepID      string `json:"step_id,omitempty"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
}

// DefaultTaskAdapter is a default implementation of the TaskAdapter interface
type DefaultTaskAdapter struct {
	logger logging.Logger
}

// NewDefaultTaskAdapter creates a new default task adapter
func NewDefaultTaskAdapter(logger logging.Logger) TaskAdapter[*DefaultTask, DefaultCreateRequest, DefaultApproveRequest, DefaultTaskUpdate] {
	return &DefaultTaskAdapter{
		logger: logger,
	}
}

// ConvertCreateRequest converts a default create request to an SDK create request
func (a *DefaultTaskAdapter) ConvertCreateRequest(req DefaultCreateRequest) task.CreateTaskRequest {
	return task.CreateTaskRequest{
		Description: req.Description,
		UserID:      req.UserID,
		Title:       req.Title,
		TaskKind:    req.TaskKind,
	}
}

// ConvertApproveRequest converts a default approve request to an SDK approve request
func (a *DefaultTaskAdapter) ConvertApproveRequest(req DefaultApproveRequest) task.ApproveTaskPlanRequest {
	return task.ApproveTaskPlanRequest{
		Approved: req.Approved,
		Feedback: req.Feedback,
	}
}

// ConvertTaskUpdates converts default task updates to SDK task updates
func (a *DefaultTaskAdapter) ConvertTaskUpdates(updates []DefaultTaskUpdate) []task.TaskUpdate {
	var sdkUpdates []task.TaskUpdate
	for _, update := range updates {
		sdkUpdates = append(sdkUpdates, task.TaskUpdate{
			Type:        update.Type,
			StepID:      update.StepID,
			Description: update.Description,
			Status:      update.Status,
		})
	}
	return sdkUpdates
}

// convertStepsToDefaultSteps converts SDK steps to default steps
func (a *DefaultTaskAdapter) convertStepsToDefaultSteps(sdkSteps []task.Step) []DefaultStep {
	var steps []DefaultStep
	for _, step := range sdkSteps {
		steps = append(steps, DefaultStep{
			ID:          step.ID,
			Description: step.Description,
			Status:      string(step.Status),
			StartTime:   step.StartedAt,
			EndTime:     step.CompletedAt,
			Error:       step.Error,
			Output:      step.Output,
		})
	}
	return steps
}

// ConvertTask converts an SDK task to a default task
func (a *DefaultTaskAdapter) ConvertTask(sdkTask *task.Task) *DefaultTask {
	if sdkTask == nil {
		return nil
	}

	// Convert steps if available
	var steps []DefaultStep
	if sdkTask.Plan != nil && len(sdkTask.Plan.Steps) > 0 {
		steps = a.convertStepsToDefaultSteps(sdkTask.Plan.Steps)
	} else if len(sdkTask.Steps) > 0 {
		steps = a.convertStepsToDefaultSteps(sdkTask.Steps)
	}

	return &DefaultTask{
		ID:             sdkTask.ID,
		Description:    sdkTask.Description,
		Status:         string(sdkTask.Status),
		Title:          sdkTask.Title,
		TaskKind:       sdkTask.TaskKind,
		ConversationID: sdkTask.ConversationID,
		CreatedAt:      sdkTask.CreatedAt,
		UpdatedAt:      sdkTask.UpdatedAt,
		StartedAt:      sdkTask.StartedAt,
		CompletedAt:    sdkTask.CompletedAt,
		UserID:         sdkTask.UserID,
		Steps:          steps,
		Requirements:   sdkTask.Requirements,
		Feedback:       sdkTask.Feedback,
		Metadata:       sdkTask.Metadata,
	}
}

// ConvertTasks converts SDK tasks to default tasks
func (a *DefaultTaskAdapter) ConvertTasks(sdkTasks []*task.Task) []*DefaultTask {
	var tasks []*DefaultTask
	for _, sdkTask := range sdkTasks {
		tasks = append(tasks, a.ConvertTask(sdkTask))
	}
	return tasks
}
