package service

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task"
)

// stubRunner implements TaskRunner in the few lines the narrow interface
// requires. That it can be written at all is the point of this change.
type stubRunner struct{ executed []string }

func (r *stubRunner) ExecuteTask(_ context.Context, t *task.Task) error {
	r.executed = append(r.executed, t.ID)
	return nil
}

// TestServiceIsConstructibleWithARunner guards the fix.
//
// NewInMemoryTaskService previously took an interfaces.TaskExecutor: an
// eight-method interface that NO type in the module satisfied, making the
// richer of the two task services impossible to construct with anything at all.
// It now asks for the single method it actually uses.
func TestServiceIsConstructibleWithARunner(t *testing.T) {
	var runner TaskRunner = &stubRunner{}

	svc := NewInMemoryTaskService(logging.New(), nil, runner)
	if svc == nil {
		t.Fatal("NewInMemoryTaskService returned nil")
	}
}

// TestServiceAcceptsANilRunner pins the documented behaviour: a caller who
// wants to run approved tasks themselves should not have to supply a stub.
func TestServiceAcceptsANilRunner(t *testing.T) {
	svc := NewInMemoryTaskService(logging.New(), nil, nil)
	if svc == nil {
		t.Fatal("NewInMemoryTaskService returned nil for a nil runner")
	}
}

func TestCreateAndGetTask(t *testing.T) {
	svc := NewInMemoryTaskService(logging.New(), nil, &stubRunner{})
	ctx := context.Background()

	created, err := svc.CreateTask(ctx, task.CreateTaskRequest{
		Description: "summarise the incident",
		UserID:      "user1",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateTask produced no ID")
	}

	got, err := svc.GetTask(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if got.Description != "summarise the incident" {
		t.Errorf("Description = %q", got.Description)
	}
}

func TestGetUnknownTaskIsAnError(t *testing.T) {
	svc := NewInMemoryTaskService(logging.New(), nil, nil)
	if _, err := svc.GetTask(context.Background(), "nope"); err == nil {
		t.Error("GetTask on an unknown ID should be an error")
	}
}

func TestListTasksFiltersByUser(t *testing.T) {
	svc := NewInMemoryTaskService(logging.New(), nil, nil)
	ctx := context.Background()

	for _, user := range []string{"user1", "user1", "user2"} {
		if _, err := svc.CreateTask(ctx, task.CreateTaskRequest{
			Description: "work", UserID: user,
		}); err != nil {
			t.Fatalf("CreateTask() error = %v", err)
		}
	}

	got, err := svc.ListTasks(ctx, task.TaskFilter{UserID: "user1"})
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("ListTasks returned %d tasks for user1, want 2", len(got))
	}
}
