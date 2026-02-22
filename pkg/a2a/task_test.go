package a2a

import (
	"context"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
)

func TestInMemoryTaskStore_SaveAndGet(t *testing.T) {
	store := NewInMemoryTaskStore()
	ctx := context.Background()

	task := &a2a.Task{
		ID:        a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Status: a2a.TaskStatus{
			State: a2a.TaskStateWorking,
		},
	}

	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("expected task ID %s, got %s", task.ID, got.ID)
	}
	if got.Status.State != a2a.TaskStateWorking {
		t.Errorf("expected state working, got %s", got.Status.State)
	}
}

func TestInMemoryTaskStore_GetNotFound(t *testing.T) {
	store := NewInMemoryTaskStore()
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestInMemoryTaskStore_List(t *testing.T) {
	store := NewInMemoryTaskStore()
	ctx := context.Background()

	contextID := a2a.NewContextID()

	task1 := &a2a.Task{
		ID:        a2a.NewTaskID(),
		ContextID: contextID,
		Status:    a2a.TaskStatus{State: a2a.TaskStateCompleted},
	}
	task2 := &a2a.Task{
		ID:        a2a.NewTaskID(),
		ContextID: contextID,
		Status:    a2a.TaskStatus{State: a2a.TaskStateWorking},
	}
	task3 := &a2a.Task{
		ID:        a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Status:    a2a.TaskStatus{State: a2a.TaskStateCompleted},
	}

	for _, task := range []*a2a.Task{task1, task2, task3} {
		if err := store.Save(ctx, task); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	// List by context ID
	resp, err := store.List(ctx, &a2a.ListTasksRequest{ContextID: contextID})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(resp.Tasks) != 2 {
		t.Errorf("expected 2 tasks for context, got %d", len(resp.Tasks))
	}

	// List by status
	resp, err = store.List(ctx, &a2a.ListTasksRequest{Status: a2a.TaskStateCompleted})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(resp.Tasks) != 2 {
		t.Errorf("expected 2 completed tasks, got %d", len(resp.Tasks))
	}

	// List all
	resp, err = store.List(ctx, &a2a.ListTasksRequest{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(resp.Tasks) != 3 {
		t.Errorf("expected 3 tasks total, got %d", len(resp.Tasks))
	}
}

func TestInMemoryTaskStore_ListPageSize(t *testing.T) {
	store := NewInMemoryTaskStore()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		task := &a2a.Task{
			ID:        a2a.NewTaskID(),
			ContextID: "ctx-1",
			Status:    a2a.TaskStatus{State: a2a.TaskStateCompleted},
		}
		if err := store.Save(ctx, task); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	resp, err := store.List(ctx, &a2a.ListTasksRequest{PageSize: 2})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(resp.Tasks) != 2 {
		t.Errorf("expected 2 tasks (page size), got %d", len(resp.Tasks))
	}
}
