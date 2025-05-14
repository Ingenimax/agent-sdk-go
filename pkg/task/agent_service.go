package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
)

//-----------------------------------------------------------------------------
// Legacy Types for Backward Compatibility
//-----------------------------------------------------------------------------

// Status represents the current status of a task or step
type Status string

// Task statuses
const (
	StatusPending   Status = "pending"
	StatusPlanning  Status = "planning"
	StatusApproval  Status = "awaiting_approval"
	StatusExecuting Status = "executing"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Step statuses
const (
	StepStatusPending   Status = "pending"
	StepStatusExecuting Status = "executing"
	StepStatusCompleted Status = "completed"
	StepStatusFailed    Status = "failed"
)

// Task represents an infrastructure task to be executed
type Task struct {
	ID             string                 `json:"id"`
	Description    string                 `json:"description"`
	Status         Status                 `json:"status"`
	Title          string                 `json:"title,omitempty"`
	TaskKind       string                 `json:"task_kind,omitempty"`
	ConversationID string                 `json:"conversation_id,omitempty"`
	Plan           *Plan                  `json:"plan,omitempty"`
	Steps          []Step                 `json:"steps,omitempty"` // Direct access to steps
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	StartedAt      *time.Time             `json:"started_at,omitempty"`
	CompletedAt    *time.Time             `json:"completed_at,omitempty"`
	UserID         string                 `json:"user_id"`
	Logs           []LogEntry             `json:"logs,omitempty"`
	Requirements   interface{}            `json:"requirements,omitempty"` // JSON of TaskRequirements
	Feedback       string                 `json:"feedback,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"` // For extensibility
}

// Plan represents the execution plan for a task
type Plan struct {
	ID         string     `json:"id"`
	TaskID     string     `json:"task_id"`
	Steps      []Step     `json:"steps"`
	CreatedAt  time.Time  `json:"created_at"`
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	IsApproved bool       `json:"is_approved"`
}

// Step represents a single step in an execution plan
type Step struct {
	ID          string                 `json:"id"`
	PlanID      string                 `json:"plan_id"`
	Description string                 `json:"description"`
	Status      Status                 `json:"status"`
	Order       int                    `json:"order"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Output      string                 `json:"output,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// LogEntry represents a log entry for a task
type LogEntry struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	StepID    string    `json:"step_id,omitempty"` // Optional reference to a specific step
	Message   string    `json:"message"`
	Level     string    `json:"level"` // info, warning, error
	Timestamp time.Time `json:"timestamp"`
}

// CreateTaskRequest represents the request to create a new task
type CreateTaskRequest struct {
	Description string                 `json:"description"`
	UserID      string                 `json:"user_id"`
	Title       string                 `json:"title,omitempty"`
	TaskKind    string                 `json:"task_kind,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ApproveTaskPlanRequest represents the request to approve a task plan
type ApproveTaskPlanRequest struct {
	Approved bool   `json:"approved"`
	Feedback string `json:"feedback,omitempty"`
}

// TaskUpdate represents an update to a task
type TaskUpdate struct {
	Type        string `json:"type"` // add_step, modify_step, remove_step, add_comment, update_status
	StepID      string `json:"step_id,omitempty"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
}

// TaskFilter represents filters for querying tasks
type TaskFilter struct {
	UserID         string     `json:"user_id,omitempty"`
	ConversationID string     `json:"conversation_id,omitempty"`
	Status         []Status   `json:"status,omitempty"`
	TaskKind       string     `json:"task_kind,omitempty"`
	CreatedAfter   *time.Time `json:"created_after,omitempty"`
	CreatedBefore  *time.Time `json:"created_before,omitempty"`
}

//-----------------------------------------------------------------------------
// Agent Task Service
//-----------------------------------------------------------------------------

// AgentAdapterService is the interface used by the AgentTaskService
// This is defined here to avoid import cycles
type AgentAdapterService interface {
	CreateTask(ctx context.Context, req CreateTaskRequest) (*Task, error)
	GetTask(ctx context.Context, taskID string) (*Task, error)
	ListTasks(ctx context.Context, filter TaskFilter) ([]*Task, error)
	ApproveTaskPlan(ctx context.Context, taskID string, req ApproveTaskPlanRequest) (*Task, error)
	UpdateTask(ctx context.Context, taskID string, updates []TaskUpdate) (*Task, error)
	AddTaskLog(ctx context.Context, taskID string, message string, level string) error
}

// AgentTaskAdapter is the interface used by the AgentTaskService to convert requests
// This is defined here to avoid import cycles
type AgentTaskAdapter interface {
	CreateTaskRequestToCore(req CreateTaskRequest) core.CreateTaskRequest
	CoreTaskToTask(coreTask *core.Task) *Task
}

// AgentTaskService provides a complete implementation for the AgentTaskServiceInterface
// Agents can use this directly without additional wrapper.
type AgentTaskService struct {
	service        AgentAdapterService
	currentTask    *Task
	currentTaskMux sync.RWMutex
	logger         logging.Logger
}

// NewAgentTaskService creates a new TaskService for agents
func NewAgentTaskService(logger logging.Logger) (*AgentTaskService, error) {
	// Since we've removed the InMemoryTaskService, we'll use our CoreBridgeAdapter instead
	// Create a core planner
	corePlanner := &SimplePlannerCore{logger: logger}

	// Create a core memory service
	coreService := &SimpleMemoryService{
		tasks:   make(map[string]*core.Task),
		logs:    make(map[string][]*core.Log),
		mutex:   sync.RWMutex{},
		logger:  logger,
		planner: corePlanner,
	}

	// Create a bridge adapter
	bridgeAdapter := &SimpleBridgeAdapter{
		coreService: coreService,
		logger:      logger,
	}

	return &AgentTaskService{
		service:     bridgeAdapter,
		currentTask: nil,
		logger:      logger,
	}, nil
}

// NewAgentTaskServiceWithAdapter creates a new TaskService for agents using a custom service adapter
func NewAgentTaskServiceWithAdapter(logger logging.Logger, service AgentAdapterService) *AgentTaskService {
	return &AgentTaskService{
		service:     service,
		currentTask: nil,
		logger:      logger,
	}
}

// SimplePlanner implements TaskPlanner with minimal functionality
type SimplePlanner struct{}

// CreatePlan implements TaskPlanner.CreatePlan
func (p *SimplePlanner) CreatePlan(ctx context.Context, task *Task) (string, error) {
	return "Simple plan for " + task.Title, nil
}

// SimpleExecutor implements TaskExecutor with minimal functionality
type SimpleExecutor struct{}

// ExecuteStep implements TaskExecutor.ExecuteStep
func (e *SimpleExecutor) ExecuteStep(ctx context.Context, task *Task, step *Step) error {
	// Just mark the step as completed
	step.Status = StepStatusCompleted
	now := time.Now()
	step.CompletedAt = &now
	return nil
}

// ExecuteTask implements TaskExecutor.ExecuteTask
func (e *SimpleExecutor) ExecuteTask(ctx context.Context, task *Task) error {
	// Just mark all steps as completed
	for i := range task.Steps {
		task.Steps[i].Status = StepStatusCompleted
		now := time.Now()
		task.Steps[i].CompletedAt = &now
	}
	return nil
}

// CreateTask creates a new task with the given parameters
func (s *AgentTaskService) CreateTask(ctx context.Context, title, desc string, userID string, metadata map[string]interface{}) (*Task, error) {
	req := CreateTaskRequest{
		Title:       title,
		Description: desc,
		UserID:      userID,
		Metadata:    metadata,
	}

	task, err := s.service.CreateTask(ctx, req)
	if err != nil {
		return nil, err
	}

	s.currentTaskMux.Lock()
	s.currentTask = task
	s.currentTaskMux.Unlock()

	return task, nil
}

// CurrentTask returns the current task being worked on, if any
func (s *AgentTaskService) CurrentTask() *Task {
	s.currentTaskMux.RLock()
	defer s.currentTaskMux.RUnlock()
	return s.currentTask
}

// GetTask gets a task by ID
func (s *AgentTaskService) GetTask(ctx context.Context, taskID string) (*Task, error) {
	return s.service.GetTask(ctx, taskID)
}

// ListTasks returns all tasks for the current user
func (s *AgentTaskService) ListTasks(ctx context.Context, userID string) ([]*Task, error) {
	filter := TaskFilter{
		UserID: userID,
	}
	return s.service.ListTasks(ctx, filter)
}

// StartTask sets the current task to a specific task ID
func (s *AgentTaskService) StartTask(ctx context.Context, taskID string) (*Task, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	s.currentTaskMux.Lock()
	s.currentTask = task
	s.currentTaskMux.Unlock()

	return task, nil
}

// ResetCurrentTask clears the current task
func (s *AgentTaskService) ResetCurrentTask() {
	s.currentTaskMux.Lock()
	s.currentTask = nil
	s.currentTaskMux.Unlock()
}

// AddTaskStep adds a step to the current task
func (s *AgentTaskService) AddTaskStep(ctx context.Context, description string) error {
	s.currentTaskMux.RLock()
	currentTask := s.currentTask
	s.currentTaskMux.RUnlock()

	if currentTask == nil {
		return errors.New("no current task")
	}

	updates := []TaskUpdate{
		{
			Type:        "add_step",
			Description: description,
		},
	}

	task, err := s.service.UpdateTask(ctx, currentTask.ID, updates)
	if err != nil {
		return err
	}

	s.currentTaskMux.Lock()
	s.currentTask = task
	s.currentTaskMux.Unlock()

	return nil
}

// UpdateTaskStep updates a step in the current task
func (s *AgentTaskService) UpdateTaskStep(ctx context.Context, stepID string, status Status, output string, err error) error {
	s.currentTaskMux.RLock()
	currentTask := s.currentTask
	s.currentTaskMux.RUnlock()

	if currentTask == nil {
		return errors.New("no current task")
	}

	update := TaskUpdate{
		Type:   "modify_step",
		StepID: stepID,
		Status: string(status),
	}

	// For error and output information, we need to use separate updates
	// or modify the step directly through the API
	updates := []TaskUpdate{update}

	// Add error information if present
	if err != nil {
		errorUpdate := TaskUpdate{
			Type:        "add_log",
			StepID:      stepID,
			Description: "Error: " + err.Error(),
		}
		updates = append(updates, errorUpdate)
	}

	// Add output information if present
	if output != "" {
		// Use a log entry for the output
		outputUpdate := TaskUpdate{
			Type:        "add_log",
			StepID:      stepID,
			Description: "Output: " + output,
		}
		updates = append(updates, outputUpdate)
	}

	task, updateErr := s.service.UpdateTask(ctx, currentTask.ID, updates)
	if updateErr != nil {
		return updateErr
	}

	s.currentTaskMux.Lock()
	s.currentTask = task
	s.currentTaskMux.Unlock()

	return nil
}

// UpdateTaskStatus updates the status of the current task
func (s *AgentTaskService) UpdateTaskStatus(ctx context.Context, status Status) error {
	s.currentTaskMux.RLock()
	currentTask := s.currentTask
	s.currentTaskMux.RUnlock()

	if currentTask == nil {
		return errors.New("no current task")
	}

	update := TaskUpdate{
		Type:   "update_status",
		Status: string(status),
	}

	task, err := s.service.UpdateTask(ctx, currentTask.ID, []TaskUpdate{update})
	if err != nil {
		return err
	}

	s.currentTaskMux.Lock()
	s.currentTask = task
	s.currentTaskMux.Unlock()

	return nil
}

// LogTaskInfo adds an info log entry to the current task
func (s *AgentTaskService) LogTaskInfo(ctx context.Context, message string) error {
	s.currentTaskMux.RLock()
	currentTask := s.currentTask
	s.currentTaskMux.RUnlock()

	if currentTask == nil {
		return errors.New("no current task")
	}

	return s.service.AddTaskLog(ctx, currentTask.ID, message, "info")
}

// LogTaskError adds an error log entry to the current task
func (s *AgentTaskService) LogTaskError(ctx context.Context, message string) error {
	s.currentTaskMux.RLock()
	currentTask := s.currentTask
	s.currentTaskMux.RUnlock()

	if currentTask == nil {
		return errors.New("no current task")
	}

	return s.service.AddTaskLog(ctx, currentTask.ID, message, "error")
}

// LogTaskDebug adds a debug log entry to the current task
func (s *AgentTaskService) LogTaskDebug(ctx context.Context, message string) error {
	s.currentTaskMux.RLock()
	currentTask := s.currentTask
	s.currentTaskMux.RUnlock()

	if currentTask == nil {
		return errors.New("no current task")
	}

	return s.service.AddTaskLog(ctx, currentTask.ID, message, "debug")
}

// FormatTaskProgress formats a string with the task progress
func (s *AgentTaskService) FormatTaskProgress() string {
	s.currentTaskMux.RLock()
	defer s.currentTaskMux.RUnlock()

	if s.currentTask == nil {
		return "No active task"
	}

	task := s.currentTask
	result := fmt.Sprintf("Task: %s (Status: %s)\n", task.Title, task.Status)

	if task.Plan != nil && len(task.Steps) > 0 {
		result += "Progress:\n"
		for i, step := range task.Steps {
			statusEmoji := "⏱️"
			switch step.Status {
			case StatusCompleted:
				statusEmoji = "✅"
			case StatusFailed:
				statusEmoji = "❌"
			case StatusExecuting:
				statusEmoji = "⚙️"
			}
			result += fmt.Sprintf("  %d. %s %s\n", i+1, statusEmoji, step.Description)
		}
	}

	return result
}

// SimplePlannerCore implements interfaces.TaskPlanner
type SimplePlannerCore struct {
	logger logging.Logger
}

// CreatePlan creates a simple plan
func (p *SimplePlannerCore) CreatePlan(ctx context.Context, task interface{}) (string, error) {
	if coreTask, ok := task.(*core.Task); ok {
		return "Simple plan for " + coreTask.Name, nil
	}
	return "Simple plan", nil
}

// SimpleMemoryService implements interfaces.TaskService
type SimpleMemoryService struct {
	tasks   map[string]*core.Task
	logs    map[string][]*core.Log
	mutex   sync.RWMutex
	logger  logging.Logger
	planner interfaces.TaskPlanner
}

// CreateTask creates a new task
func (s *SimpleMemoryService) CreateTask(ctx context.Context, req interface{}) (interface{}, error) {
	coreReq, ok := req.(core.CreateTaskRequest)
	if !ok {
		return nil, fmt.Errorf("invalid request type")
	}

	task := &core.Task{
		ID:          "task-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:        coreReq.Name,
		Description: coreReq.Description,
		Status:      core.StatusPending,
		UserID:      coreReq.UserID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Steps:       []*core.Step{},
	}

	s.mutex.Lock()
	s.tasks[task.ID] = task
	s.mutex.Unlock()

	return task, nil
}

// GetTask gets a task by ID
func (s *SimpleMemoryService) GetTask(ctx context.Context, taskID string) (interface{}, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	return task, nil
}

// ListTasks returns tasks filtered by criteria
func (s *SimpleMemoryService) ListTasks(ctx context.Context, filter interface{}) ([]interface{}, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var tasks []interface{}

	// If we have a core.TaskFilter, apply it
	coreFilter, ok := filter.(core.TaskFilter)
	if ok {
		for _, task := range s.tasks {
			if coreFilter.UserID != "" && task.UserID != coreFilter.UserID {
				continue
			}
			tasks = append(tasks, task)
		}
	} else {
		// Otherwise, just return all tasks
		for _, task := range s.tasks {
			tasks = append(tasks, task)
		}
	}

	return tasks, nil
}

// ApproveTaskPlan approves or rejects a plan
func (s *SimpleMemoryService) ApproveTaskPlan(ctx context.Context, taskID string, req interface{}) (interface{}, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	// Try to convert the request
	coreReq, ok := req.(core.ApproveTaskPlanRequest)
	if !ok {
		return nil, fmt.Errorf("invalid request type")
	}

	if coreReq.Approved {
		task.Status = core.StatusExecuting
	}

	return task, nil
}

// UpdateTask updates a task
func (s *SimpleMemoryService) UpdateTask(ctx context.Context, taskID string, updates interface{}) (interface{}, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	// Try to convert the updates
	coreUpdates, ok := updates.([]core.TaskUpdate)
	if !ok {
		return nil, fmt.Errorf("invalid updates type")
	}

	for _, update := range coreUpdates {
		switch update.Field {
		case "add_step":
			if stepData, ok := update.Value.(map[string]interface{}); ok {
				step := &core.Step{
					ID:          "step-" + fmt.Sprintf("%d", time.Now().UnixNano()),
					Name:        "Step",
					Description: stepData["description"].(string),
					Status:      core.StatusPending,
					OrderIndex:  len(task.Steps),
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				task.Steps = append(task.Steps, step)
			}
		case "status":
			if status, ok := update.Value.(string); ok {
				task.Status = core.Status(status)
			}
		}
	}

	task.UpdatedAt = time.Now()
	return task, nil
}

// AddTaskLog adds a log to a task
func (s *SimpleMemoryService) AddTaskLog(ctx context.Context, taskID string, message string, level string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	log := &core.Log{
		ID:        "log-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		TaskID:    taskID,
		Message:   message,
		Level:     level,
		CreatedAt: time.Now(),
	}

	s.logs[taskID] = append(s.logs[taskID], log)
	return nil
}

// SimpleBridgeAdapter implements AgentAdapterService
type SimpleBridgeAdapter struct {
	coreService interfaces.TaskService
	logger      logging.Logger
}

// CreateTask creates a task using the core service
func (a *SimpleBridgeAdapter) CreateTask(ctx context.Context, req CreateTaskRequest) (*Task, error) {
	coreReq := core.CreateTaskRequest{
		Name:        req.Title,
		Description: req.Description,
		UserID:      req.UserID,
		Metadata:    req.Metadata,
	}

	coreTaskObj, err := a.coreService.CreateTask(ctx, coreReq)
	if err != nil {
		return nil, err
	}

	// Type assertion
	coreTask, ok := coreTaskObj.(*core.Task)
	if !ok {
		return nil, fmt.Errorf("unexpected type returned from core service")
	}

	return a.coreTaskToTask(coreTask), nil
}

// GetTask gets a task by ID
func (a *SimpleBridgeAdapter) GetTask(ctx context.Context, taskID string) (*Task, error) {
	coreTaskObj, err := a.coreService.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	// Type assertion
	coreTask, ok := coreTaskObj.(*core.Task)
	if !ok {
		return nil, fmt.Errorf("unexpected type returned from core service")
	}

	return a.coreTaskToTask(coreTask), nil
}

// ListTasks returns tasks based on filter
func (a *SimpleBridgeAdapter) ListTasks(ctx context.Context, filter TaskFilter) ([]*Task, error) {
	coreFilter := core.TaskFilter{
		UserID: filter.UserID,
	}

	coreTaskObjList, err := a.coreService.ListTasks(ctx, coreFilter)
	if err != nil {
		return nil, err
	}

	var tasks []*Task
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

// ApproveTaskPlan approves or rejects a plan
func (a *SimpleBridgeAdapter) ApproveTaskPlan(ctx context.Context, taskID string, req ApproveTaskPlanRequest) (*Task, error) {
	coreReq := core.ApproveTaskPlanRequest{
		Approved: req.Approved,
		Feedback: req.Feedback,
	}

	coreTaskObj, err := a.coreService.ApproveTaskPlan(ctx, taskID, coreReq)
	if err != nil {
		return nil, err
	}

	// Type assertion
	coreTask, ok := coreTaskObj.(*core.Task)
	if !ok {
		return nil, fmt.Errorf("unexpected type returned from core service")
	}

	return a.coreTaskToTask(coreTask), nil
}

// UpdateTask updates a task
func (a *SimpleBridgeAdapter) UpdateTask(ctx context.Context, taskID string, updates []TaskUpdate) (*Task, error) {
	var coreUpdates []core.TaskUpdate

	for _, update := range updates {
		switch update.Type {
		case "add_step":
			coreUpdates = append(coreUpdates, core.TaskUpdate{
				Field: "add_step",
				Value: map[string]interface{}{
					"description": update.Description,
				},
			})
		case "update_status":
			coreUpdates = append(coreUpdates, core.TaskUpdate{
				Field: "status",
				Value: update.Status,
			})
		}
	}

	coreTaskObj, err := a.coreService.UpdateTask(ctx, taskID, coreUpdates)
	if err != nil {
		return nil, err
	}

	// Type assertion
	coreTask, ok := coreTaskObj.(*core.Task)
	if !ok {
		return nil, fmt.Errorf("unexpected type returned from core service")
	}

	return a.coreTaskToTask(coreTask), nil
}

// AddTaskLog adds a log to a task
func (a *SimpleBridgeAdapter) AddTaskLog(ctx context.Context, taskID string, message string, level string) error {
	return a.coreService.AddTaskLog(ctx, taskID, message, level)
}

// coreTaskToTask converts a core.Task to a Task
func (a *SimpleBridgeAdapter) coreTaskToTask(coreTask *core.Task) *Task {
	task := &Task{
		ID:          coreTask.ID,
		Title:       coreTask.Name,
		Description: coreTask.Description,
		Status:      Status(coreTask.Status),
		UserID:      coreTask.UserID,
		CreatedAt:   coreTask.CreatedAt,
		UpdatedAt:   coreTask.UpdatedAt,
		CompletedAt: coreTask.CompletedAt,
		Metadata:    coreTask.Metadata,
	}

	// Convert steps
	for _, coreStep := range coreTask.Steps {
		step := Step{
			ID:          coreStep.ID,
			Description: coreStep.Description,
			Status:      Status(coreStep.Status),
			Order:       coreStep.OrderIndex,
			CompletedAt: coreStep.CompletedAt,
		}
		task.Steps = append(task.Steps, step)
	}

	return task
}
