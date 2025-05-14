package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
	"github.com/google/uuid"
)

// CoreMemoryService implements the interfaces.TaskService interface with in-memory storage
type CoreMemoryService struct {
	tasks   map[string]*core.Task
	logs    map[string][]*core.Log
	mutex   sync.RWMutex
	logger  logging.Logger
	planner interfaces.TaskPlanner
}

// NewCoreMemoryService creates a new in-memory service for core tasks
func NewCoreMemoryService(logger logging.Logger, planner interfaces.TaskPlanner) interfaces.TaskService {
	return &CoreMemoryService{
		tasks:   make(map[string]*core.Task),
		logs:    make(map[string][]*core.Log),
		mutex:   sync.RWMutex{},
		logger:  logger,
		planner: planner,
	}
}

// CreateTask creates a new task
func (s *CoreMemoryService) CreateTask(ctx context.Context, reqObj interface{}) (interface{}, error) {
	// Try to convert the request
	var req core.CreateTaskRequest
	var ok bool

	if req, ok = reqObj.(core.CreateTaskRequest); !ok {
		// Try to convert from map
		if reqMap, ok := reqObj.(map[string]interface{}); ok {
			req = core.CreateTaskRequest{
				Name:        reqMap["name"].(string),
				Description: reqMap["description"].(string),
				UserID:      reqMap["user_id"].(string),
			}

			// Handle optional fields
			if conv, ok := reqMap["conversation_id"].(string); ok {
				req.ConversationID = conv
			}
			if meta, ok := reqMap["metadata"].(map[string]interface{}); ok {
				req.Metadata = meta
			}
			if input, ok := reqMap["input"].(map[string]interface{}); ok {
				req.Input = input
			}
		} else {
			return nil, fmt.Errorf("invalid request type")
		}
	}

	taskID := uuid.New().String()
	now := time.Now()

	task := &core.Task{
		ID:             taskID,
		Name:           req.Name,
		Description:    req.Description,
		Status:         core.StatusPending,
		UserID:         req.UserID,
		ConversationID: req.ConversationID,
		Steps:          []*core.Step{},
		CreatedAt:      now,
		UpdatedAt:      now,
		Input:          req.Input,
		Metadata:       req.Metadata,
	}

	s.mutex.Lock()
	s.tasks[taskID] = task
	s.logs[taskID] = []*core.Log{}
	s.mutex.Unlock()

	s.logger.Info(ctx, "Created new core task", map[string]interface{}{
		"task_id": taskID,
	})

	return task, nil
}

// GetTask gets a task by ID
func (s *CoreMemoryService) GetTask(ctx context.Context, taskID string) (interface{}, error) {
	s.mutex.RLock()
	task, exists := s.tasks[taskID]
	s.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	return task, nil
}

// ListTasks returns tasks based on the filter
func (s *CoreMemoryService) ListTasks(ctx context.Context, filterObj interface{}) ([]interface{}, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Try to convert the filter
	var filter core.TaskFilter
	if f, ok := filterObj.(core.TaskFilter); ok {
		filter = f
	} else if fMap, ok := filterObj.(map[string]interface{}); ok {
		// Try to extract filter fields from map
		if userID, ok := fMap["user_id"].(string); ok {
			filter.UserID = userID
		}
		if convID, ok := fMap["conversation_id"].(string); ok {
			filter.ConversationID = convID
		}
		if status, ok := fMap["status"].(string); ok {
			filter.Status = core.Status(status)
		}
	}

	var results []interface{}

	for _, task := range s.tasks {
		// Apply filters
		if filter.UserID != "" && task.UserID != filter.UserID {
			continue
		}
		if filter.ConversationID != "" && task.ConversationID != filter.ConversationID {
			continue
		}
		if filter.Status != "" && task.Status != filter.Status {
			continue
		}
		if filter.FromDate != nil && task.CreatedAt.Before(*filter.FromDate) {
			continue
		}
		if filter.ToDate != nil && task.CreatedAt.After(*filter.ToDate) {
			continue
		}

		results = append(results, task)
	}

	// Apply limits and offset
	if filter.Limit > 0 && len(results) > filter.Limit {
		end := filter.Offset + filter.Limit
		if end > len(results) {
			end = len(results)
		}
		if filter.Offset < len(results) {
			results = results[filter.Offset:end]
		} else {
			results = []interface{}{}
		}
	}

	return results, nil
}

// ApproveTaskPlan approves or rejects a task plan
func (s *CoreMemoryService) ApproveTaskPlan(ctx context.Context, taskID string, reqObj interface{}) (interface{}, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	// Only tasks in planning or awaiting approval can be approved
	if task.Status != core.StatusPlanning && task.Status != core.StatusAwaitingApproval {
		return nil, fmt.Errorf("task is not in a state that can be approved: %s", task.Status)
	}

	// Try to convert the request
	var req core.ApproveTaskPlanRequest
	var ok bool

	if req, ok = reqObj.(core.ApproveTaskPlanRequest); !ok {
		// Try to convert from map
		if reqMap, ok := reqObj.(map[string]interface{}); ok {
			if approved, ok := reqMap["approved"].(bool); ok {
				req.Approved = approved
			}
			if feedback, ok := reqMap["feedback"].(string); ok {
				req.Feedback = feedback
			}
		} else {
			return nil, fmt.Errorf("invalid request type")
		}
	}

	// Update task based on approval
	if req.Approved {
		task.Status = core.StatusExecuting
		now := time.Now()
		task.UpdatedAt = now
	} else {
		task.Status = core.StatusPlanning // Back to planning
		task.UpdatedAt = time.Now()
	}

	s.logger.Info(ctx, "Task plan approval status updated", map[string]interface{}{
		"task_id":  taskID,
		"approved": req.Approved,
	})

	return task, nil
}

// UpdateTask updates a task
func (s *CoreMemoryService) UpdateTask(ctx context.Context, taskID string, updatesObj interface{}) (interface{}, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	task.UpdatedAt = time.Now()

	// Try to convert the updates
	var updates []core.TaskUpdate
	var ok bool

	if updates, ok = updatesObj.([]core.TaskUpdate); !ok {
		// Try to convert from array of maps
		if updatesArray, ok := updatesObj.([]interface{}); ok {
			for _, updateObj := range updatesArray {
				if updateMap, ok := updateObj.(map[string]interface{}); ok {
					update := core.TaskUpdate{}
					if field, ok := updateMap["field"].(string); ok {
						update.Field = field
					}
					if value, ok := updateMap["value"]; ok {
						update.Value = value
					}
					updates = append(updates, update)
				}
			}
		} else {
			return nil, fmt.Errorf("invalid updates type")
		}
	}

	for _, update := range updates {
		switch update.Field {
		case "status":
			if statusStr, ok := update.Value.(string); ok {
				task.Status = core.Status(statusStr)
				switch task.Status {
				case core.StatusExecuting:
					now := time.Now()
					task.UpdatedAt = now
				case core.StatusCompleted, core.StatusFailed:
					now := time.Now()
					task.CompletedAt = &now
					task.UpdatedAt = now
				}
			}
		case "add_step":
			if stepData, ok := update.Value.(map[string]interface{}); ok {
				step := &core.Step{
					ID:         uuid.New().String(),
					OrderIndex: len(task.Steps),
					Status:     core.StatusPending,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
					Output:     make(map[string]interface{}),
				}

				// Get fields from the step data
				if name, ok := stepData["name"].(string); ok {
					step.Name = name
				}
				if desc, ok := stepData["description"].(string); ok {
					step.Description = desc
				}
				if stepType, ok := stepData["type"].(string); ok {
					step.Type = stepType
				}

				task.Steps = append(task.Steps, step)
			}
		case "update_step":
			if stepData, ok := update.Value.(map[string]interface{}); ok {
				stepID, ok := stepData["id"].(string)
				if !ok {
					continue
				}

				for i, step := range task.Steps {
					if step.ID == stepID {
						if status, ok := stepData["status"].(string); ok {
							task.Steps[i].Status = core.Status(status)

							switch task.Steps[i].Status {
							case core.StatusExecuting:
								now := time.Now()
								task.Steps[i].UpdatedAt = now
							case core.StatusCompleted:
								now := time.Now()
								task.Steps[i].CompletedAt = &now
								task.Steps[i].UpdatedAt = now
							case core.StatusFailed:
								now := time.Now()
								task.Steps[i].FailedAt = &now
								task.Steps[i].UpdatedAt = now

								if errStr, ok := stepData["error"].(string); ok {
									task.Steps[i].Error = errStr
								}
							}
						}

						if output, ok := stepData["output"].(map[string]interface{}); ok {
							task.Steps[i].Output = output
						}

						break
					}
				}
			}
		case "plan":
			if planStr, ok := update.Value.(string); ok {
				task.Plan = planStr
			}
		}
	}

	s.logger.Info(ctx, "Task updated", map[string]interface{}{
		"task_id":         taskID,
		"update_count":    len(updates),
		"resulting_state": task.Status,
	})

	return task, nil
}

// AddTaskLog adds a log entry to a task
func (s *CoreMemoryService) AddTaskLog(ctx context.Context, taskID string, message string, level string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	logEntry := &core.Log{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		Message:   message,
		Level:     level,
		CreatedAt: time.Now(),
	}

	s.logs[taskID] = append(s.logs[taskID], logEntry)

	s.logger.Info(ctx, "Added log to task", map[string]interface{}{
		"task_id": taskID,
		"level":   level,
	})

	return nil
}

// GetTaskLogs returns all logs for a task
func (s *CoreMemoryService) GetTaskLogs(ctx context.Context, taskID string) ([]*core.Log, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	logs, exists := s.logs[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	return logs, nil
}
