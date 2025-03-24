package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task"
	"github.com/google/uuid"
)

// InMemoryTaskService implements the Service interface with an in-memory storage
type InMemoryTaskService struct {
	tasks         map[string]*task.Task
	mutex         sync.RWMutex
	logger        logging.Logger
	taskHistories map[string][]string
	planner       interfaces.TaskPlanner
	executor      interfaces.TaskExecutor
}

// NewInMemoryTaskService creates a new in-memory task service
func NewInMemoryTaskService(logger logging.Logger, planner interfaces.TaskPlanner, executor interfaces.TaskExecutor) *InMemoryTaskService {
	return &InMemoryTaskService{
		tasks:         make(map[string]*task.Task),
		taskHistories: make(map[string][]string),
		mutex:         sync.RWMutex{},
		logger:        logger,
		planner:       planner,
		executor:      executor,
	}
}

// CreateTask creates a new task
func (s *InMemoryTaskService) CreateTask(ctx context.Context, req task.CreateTaskRequest) (*task.Task, error) {
	taskID := uuid.New().String()
	s.logger.Info(ctx, "Creating new task", map[string]interface{}{
		"task_id": taskID,
	})

	newTask := &task.Task{
		ID:          taskID,
		Description: req.Description,
		Status:      task.StatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		UserID:      req.UserID,
		Logs:        []task.LogEntry{},
		Metadata:    req.Metadata,
	}

	// Add initial log entry
	logEntry := task.LogEntry{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		Message:   "Task created",
		Level:     "info",
		Timestamp: time.Now(),
	}
	newTask.Logs = append(newTask.Logs, logEntry)

	// Store the task
	s.mutex.Lock()
	s.tasks[taskID] = newTask
	s.taskHistories[taskID] = []string{req.Description}
	s.mutex.Unlock()

	s.logger.Info(ctx, "Task created successfully", map[string]interface{}{
		"task_id": taskID,
	})

	// Start planning in a goroutine if planner is available
	if s.planner != nil {
		go s.planTask(context.Background(), newTask)
	}

	return newTask, nil
}

// GetTask gets a task by ID
func (s *InMemoryTaskService) GetTask(ctx context.Context, taskID string) (*task.Task, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	s.logger.Info(ctx, "Getting task", map[string]interface{}{
		"task_id": taskID,
	})

	t, ok := s.tasks[taskID]
	if !ok {
		s.logger.Error(ctx, "Task not found", map[string]interface{}{
			"task_id": taskID,
		})
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	return t, nil
}

// ListTasks returns tasks filtered by the provided criteria
func (s *InMemoryTaskService) ListTasks(ctx context.Context, filter task.TaskFilter) ([]*task.Task, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	tasks := make([]*task.Task, 0)
	for _, t := range s.tasks {
		// Apply filters
		if filter.UserID != "" && t.UserID != filter.UserID {
			continue
		}

		if len(filter.Status) > 0 {
			statusMatch := false
			for _, status := range filter.Status {
				if t.Status == status {
					statusMatch = true
					break
				}
			}
			if !statusMatch {
				continue
			}
		}

		if filter.TaskKind != "" && t.TaskKind != filter.TaskKind {
			continue
		}

		if filter.CreatedAfter != nil && !t.CreatedAt.After(*filter.CreatedAfter) {
			continue
		}

		if filter.CreatedBefore != nil && !t.CreatedAt.Before(*filter.CreatedBefore) {
			continue
		}

		tasks = append(tasks, t)
	}

	return tasks, nil
}

// ApproveTaskPlan approves or rejects a task plan
func (s *InMemoryTaskService) ApproveTaskPlan(ctx context.Context, taskID string, req task.ApproveTaskPlanRequest) (*task.Task, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.logger.Info(ctx, "Approving/rejecting task plan", map[string]interface{}{
		"task_id":  taskID,
		"approved": req.Approved,
	})

	t, ok := s.tasks[taskID]
	if !ok {
		s.logger.Error(ctx, "Task not found", map[string]interface{}{
			"task_id": taskID,
		})
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	if t.Plan == nil {
		s.logger.Error(ctx, "Task has no plan", map[string]interface{}{
			"task_id": taskID,
		})
		return nil, fmt.Errorf("task has no plan: %s", taskID)
	}

	if t.Status != task.StatusApproval {
		s.logger.Error(ctx, "Task is not awaiting approval", map[string]interface{}{
			"task_id": taskID,
			"status":  t.Status,
		})
		return nil, fmt.Errorf("task is not awaiting approval: %s", taskID)
	}

	t.Plan.IsApproved = req.Approved
	approvedTime := time.Now()
	t.Plan.ApprovedAt = &approvedTime
	t.UpdatedAt = time.Now()
	t.Feedback = req.Feedback

	// Add log entry
	logEntry := task.LogEntry{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		Message:   fmt.Sprintf("Plan %s", map[bool]string{true: "approved", false: "rejected"}[req.Approved]),
		Level:     "info",
		Timestamp: time.Now(),
	}
	t.Logs = append(t.Logs, logEntry)

	// If approved, start executing
	if req.Approved {
		t.Status = task.StatusExecuting
		startTime := time.Now()
		t.StartedAt = &startTime

		// Schedule execution
		if s.executor != nil {
			go func() {
				if err := s.executor.ExecuteTask(context.Background(), t); err != nil {
					s.logger.Error(context.Background(), "Failed to execute task", map[string]interface{}{
						"task_id": t.ID,
						"error":   err.Error(),
					})

					// Update task status to failed
					s.mutex.Lock()
					t.Status = task.StatusFailed
					failedTime := time.Now()
					t.CompletedAt = &failedTime
					s.mutex.Unlock()
				}
			}()
		}
	} else {
		// If rejected, replan with feedback
		t.Status = task.StatusPlanning
		if s.planner != nil {
			go s.replanTask(context.Background(), t, req.Feedback)
		}
	}

	return t, nil
}

// UpdateTask updates an existing task with new steps or modifications
func (s *InMemoryTaskService) UpdateTask(ctx context.Context, taskID string, updates []task.TaskUpdate) (*task.Task, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.logger.Info(ctx, "Updating task", map[string]interface{}{
		"task_id": taskID,
	})

	t, ok := s.tasks[taskID]
	if !ok {
		s.logger.Error(ctx, "Task not found", map[string]interface{}{
			"task_id": taskID,
		})
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	// Process updates
	for _, update := range updates {
		switch update.Type {
		case "add_step":
			// Create new step
			step := task.Step{
				ID:          uuid.New().String(),
				Description: update.Description,
				Status:      task.StepStatusPending,
				Order:       len(t.Steps) + 1,
			}

			// Add to task steps
			t.Steps = append(t.Steps, step)

			// Add log entry
			logEntry := task.LogEntry{
				ID:        uuid.New().String(),
				TaskID:    taskID,
				Message:   fmt.Sprintf("Added step: %s", update.Description),
				Level:     "info",
				Timestamp: time.Now(),
			}
			t.Logs = append(t.Logs, logEntry)

		case "modify_step":
			// Find step
			var stepFound bool
			for i, step := range t.Steps {
				if step.ID == update.StepID {
					// Update step
					if update.Description != "" {
						t.Steps[i].Description = update.Description
					}
					if update.Status != "" {
						t.Steps[i].Status = task.Status(update.Status)
					}
					stepFound = true
					break
				}
			}

			if !stepFound {
				s.logger.Error(ctx, "Step not found", map[string]interface{}{
					"task_id": taskID,
					"step_id": update.StepID,
				})
				return nil, fmt.Errorf("step not found: %s", update.StepID)
			}

			// Add log entry
			logEntry := task.LogEntry{
				ID:        uuid.New().String(),
				TaskID:    taskID,
				StepID:    update.StepID,
				Message:   fmt.Sprintf("Modified step: %s", update.Description),
				Level:     "info",
				Timestamp: time.Now(),
			}
			t.Logs = append(t.Logs, logEntry)

		case "remove_step":
			// Find step
			var stepIndex = -1
			for i, step := range t.Steps {
				if step.ID == update.StepID {
					stepIndex = i
					break
				}
			}

			if stepIndex == -1 {
				s.logger.Error(ctx, "Step not found", map[string]interface{}{
					"task_id": taskID,
					"step_id": update.StepID,
				})
				return nil, fmt.Errorf("step not found: %s", update.StepID)
			}

			// Remove step
			t.Steps = append(t.Steps[:stepIndex], t.Steps[stepIndex+1:]...)

			// Add log entry
			logEntry := task.LogEntry{
				ID:        uuid.New().String(),
				TaskID:    taskID,
				StepID:    update.StepID,
				Message:   "Removed step",
				Level:     "info",
				Timestamp: time.Now(),
			}
			t.Logs = append(t.Logs, logEntry)

		case "update_status":
			// Update task status
			t.Status = task.Status(update.Status)

			// Add log entry
			logEntry := task.LogEntry{
				ID:        uuid.New().String(),
				TaskID:    taskID,
				Message:   fmt.Sprintf("Updated status to: %s", update.Status),
				Level:     "info",
				Timestamp: time.Now(),
			}
			t.Logs = append(t.Logs, logEntry)

		case "add_comment":
			// Add log entry
			logEntry := task.LogEntry{
				ID:        uuid.New().String(),
				TaskID:    taskID,
				Message:   update.Description,
				Level:     "info",
				Timestamp: time.Now(),
			}
			t.Logs = append(t.Logs, logEntry)
		}
	}

	// Update task
	t.UpdatedAt = time.Now()

	return t, nil
}

// AddTaskLog adds a log entry to a task
func (s *InMemoryTaskService) AddTaskLog(ctx context.Context, taskID string, message string, level string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.logger.Info(ctx, "Adding log entry to task", map[string]interface{}{
		"task_id": taskID,
	})

	t, ok := s.tasks[taskID]
	if !ok {
		s.logger.Error(ctx, "Task not found", map[string]interface{}{
			"task_id": taskID,
		})
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Add log entry
	logEntry := task.LogEntry{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		Message:   message,
		Level:     level,
		Timestamp: time.Now(),
	}
	t.Logs = append(t.Logs, logEntry)

	return nil
}

// planTask handles the planning of a task using the task planner
func (s *InMemoryTaskService) planTask(ctx context.Context, t *task.Task) {
	s.mutex.Lock()
	t.Status = task.StatusPlanning
	s.mutex.Unlock()

	s.logger.Info(ctx, "Planning task", map[string]interface{}{
		"task_id": t.ID,
	})

	// Add log entry
	s.AddTaskLog(ctx, t.ID, "Starting task planning", "info")

	// Call the planner to create a plan
	_, err := s.planner.CreatePlan(ctx, t)
	if err != nil {
		s.handlePlanningFailure(t, err)
		return
	}

	// Update task status to awaiting approval
	s.mutex.Lock()
	defer s.mutex.Unlock()

	t.Status = task.StatusApproval
	t.UpdatedAt = time.Now()

	// Add log entry
	logEntry := task.LogEntry{
		ID:        uuid.New().String(),
		TaskID:    t.ID,
		Message:   "Plan created, awaiting approval",
		Level:     "info",
		Timestamp: time.Now(),
	}
	t.Logs = append(t.Logs, logEntry)
}

// handlePlanningFailure handles a failure during task planning
func (s *InMemoryTaskService) handlePlanningFailure(t *task.Task, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	t.Status = task.StatusFailed
	t.UpdatedAt = time.Now()

	// Add log entry
	logEntry := task.LogEntry{
		ID:        uuid.New().String(),
		TaskID:    t.ID,
		Message:   fmt.Sprintf("Planning failed: %s", err.Error()),
		Level:     "error",
		Timestamp: time.Now(),
	}
	t.Logs = append(t.Logs, logEntry)
}

// replanTask handles the replanning of a task with feedback
func (s *InMemoryTaskService) replanTask(ctx context.Context, t *task.Task, feedback string) {
	s.mutex.Lock()
	t.Status = task.StatusPlanning
	s.mutex.Unlock()

	s.logger.Info(ctx, "Replanning task with feedback", map[string]interface{}{
		"task_id":  t.ID,
		"feedback": feedback,
	})

	// Add log entry
	s.AddTaskLog(ctx, t.ID, "Replanning task with feedback", "info")

	// Call the planner to create a new plan
	_, err := s.planner.CreatePlan(ctx, t)
	if err != nil {
		s.handlePlanningFailure(t, err)
		return
	}

	// Update task status to awaiting approval
	s.mutex.Lock()
	defer s.mutex.Unlock()

	t.Status = task.StatusApproval
	t.UpdatedAt = time.Now()

	// Add log entry
	logEntry := task.LogEntry{
		ID:        uuid.New().String(),
		TaskID:    t.ID,
		Message:   "Task has been replanned with feedback",
		Level:     "info",
		Timestamp: time.Now(),
	}
	t.Logs = append(t.Logs, logEntry)
}
