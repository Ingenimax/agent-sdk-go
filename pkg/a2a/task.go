package a2a

import (
	"context"
	"fmt"
	"sync"

	"github.com/a2aproject/a2a-go/a2a"
)

// TaskStore manages A2A task persistence and retrieval.
type TaskStore interface {
	// Get retrieves a task by ID.
	Get(ctx context.Context, id a2a.TaskID) (*a2a.Task, error)
	// Save persists a task.
	Save(ctx context.Context, task *a2a.Task) error
	// List returns tasks matching the given filter.
	List(ctx context.Context, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error)
}

// InMemoryTaskStore is a simple in-memory task store for development and testing.
type InMemoryTaskStore struct {
	mu    sync.RWMutex
	tasks map[a2a.TaskID]*a2a.Task
}

// NewInMemoryTaskStore creates a new in-memory task store.
func NewInMemoryTaskStore() *InMemoryTaskStore {
	return &InMemoryTaskStore{
		tasks: make(map[a2a.TaskID]*a2a.Task),
	}
}

func (s *InMemoryTaskStore) Get(_ context.Context, id a2a.TaskID) (*a2a.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %s not found", id)
	}
	return task, nil
}

func (s *InMemoryTaskStore) Save(_ context.Context, task *a2a.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
	return nil
}

func (s *InMemoryTaskStore) List(_ context.Context, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*a2a.Task
	for _, task := range s.tasks {
		if req.ContextID != "" && task.ContextID != req.ContextID {
			continue
		}
		if req.Status != "" && task.Status.State != req.Status {
			continue
		}
		result = append(result, task)
	}

	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if len(result) > pageSize {
		result = result[:pageSize]
	}

	return &a2a.ListTasksResponse{
		Tasks:     result,
		TotalSize: len(result),
		PageSize:  pageSize,
	}, nil
}
