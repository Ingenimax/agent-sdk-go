package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBoom = errors.New("boom")

// fakePlanner is an interfaces.TaskPlanner that returns a fixed result and
// counts its calls.
type fakePlanner struct {
	err   error
	calls chan struct{}
}

func newFakePlanner(err error) *fakePlanner {
	return &fakePlanner{err: err, calls: make(chan struct{}, 10)}
}

func (p *fakePlanner) CreatePlan(context.Context, interface{}) (string, error) {
	p.calls <- struct{}{}
	return "plan", p.err
}

// signalRunner is a TaskRunner that reports each task it runs on a channel.
type signalRunner struct {
	err error
	ran chan string
}

func (r *signalRunner) ExecuteTask(_ context.Context, t *task.Task) error {
	r.ran <- t.ID
	return r.err
}

// snapshot copies the task under the service lock so tests can read it while
// background goroutines may still write to it.
func snapshot(s *InMemoryTaskService, id string) task.Task {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	t := *s.tasks[id]
	t.Logs = append([]task.LogEntry(nil), t.Logs...)
	t.Steps = append([]task.Step(nil), t.Steps...)
	return t
}

func waitForStatus(t *testing.T, s *InMemoryTaskService, id string, want task.Status) task.Task {
	t.Helper()
	require.Eventually(t, func() bool { return snapshot(s, id).Status == want }, 2*time.Second, time.Millisecond)
	return snapshot(s, id)
}

func logMessages(t task.Task) []string {
	msgs := make([]string, len(t.Logs))
	for i, l := range t.Logs {
		msgs[i] = l.Message
	}
	return msgs
}

// awaitingApproval creates a task and puts it in the awaiting approval state
// with a plan attached.
func awaitingApproval(t *testing.T, s *InMemoryTaskService) string {
	t.Helper()
	created, err := s.CreateTask(context.Background(), task.CreateTaskRequest{Description: "d", UserID: "u"})
	require.NoError(t, err)
	if s.planner != nil {
		waitForStatus(t, s, created.ID, task.StatusApproval)
	}
	s.mutex.Lock()
	created.Status = task.StatusApproval
	created.Plan = &task.Plan{ID: "p"}
	s.mutex.Unlock()
	return created.ID
}

func TestCreateTaskInitialState(t *testing.T) {
	s := NewInMemoryTaskService(logging.New(), nil, nil)
	meta := map[string]interface{}{"k": "v"}

	created, err := s.CreateTask(context.Background(), task.CreateTaskRequest{Description: "d", UserID: "u", Metadata: meta})
	require.NoError(t, err)
	assert.Equal(t, task.StatusPending, created.Status)
	assert.Equal(t, "u", created.UserID)
	assert.Equal(t, meta, created.Metadata)
	assert.Equal(t, []string{"Task created"}, logMessages(*created))
	assert.Equal(t, []string{"d"}, s.taskHistories[created.ID])
}

func TestCreateTaskPlansInBackground(t *testing.T) {
	planner := newFakePlanner(nil)
	s := NewInMemoryTaskService(logging.New(), planner, nil)

	created, err := s.CreateTask(context.Background(), task.CreateTaskRequest{Description: "d"})
	require.NoError(t, err)

	got := waitForStatus(t, s, created.ID, task.StatusApproval)
	assert.Equal(t, []string{"Task created", "Starting task planning", "Plan created, awaiting approval"}, logMessages(got))
	assert.Len(t, planner.calls, 1)
}

func TestCreateTaskPlanningFailure(t *testing.T) {
	s := NewInMemoryTaskService(logging.New(), newFakePlanner(errBoom), nil)

	created, err := s.CreateTask(context.Background(), task.CreateTaskRequest{Description: "d"})
	require.NoError(t, err)

	got := waitForStatus(t, s, created.ID, task.StatusFailed)
	last := got.Logs[len(got.Logs)-1]
	assert.Equal(t, "Planning failed: boom", last.Message)
	assert.Equal(t, "error", last.Level)
}

func TestListTasksFilters(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryTaskService(logging.New(), nil, nil)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	add := func(id, user string, status task.Status, kind string, offset time.Duration) {
		s.tasks[id] = &task.Task{ID: id, UserID: user, Status: status, TaskKind: kind, CreatedAt: base.Add(offset)}
	}
	add("a", "u1", task.StatusPending, "deploy", 0)
	add("b", "u1", task.StatusCompleted, "build", time.Hour)
	add("c", "u2", task.StatusFailed, "deploy", 2*time.Hour)

	after := base.Add(30 * time.Minute)
	before := base.Add(90 * time.Minute)

	tests := []struct {
		name   string
		filter task.TaskFilter
		want   []string
	}{
		{name: "no filter", filter: task.TaskFilter{}, want: []string{"a", "b", "c"}},
		{name: "user", filter: task.TaskFilter{UserID: "u1"}, want: []string{"a", "b"}},
		{name: "status", filter: task.TaskFilter{Status: []task.Status{task.StatusCompleted, task.StatusFailed}}, want: []string{"b", "c"}},
		{name: "kind", filter: task.TaskFilter{TaskKind: "deploy"}, want: []string{"a", "c"}},
		{name: "created after", filter: task.TaskFilter{CreatedAfter: &after}, want: []string{"b", "c"}},
		{name: "created before", filter: task.TaskFilter{CreatedBefore: &before}, want: []string{"a", "b"}},
		{name: "window", filter: task.TaskFilter{CreatedAfter: &after, CreatedBefore: &before}, want: []string{"b"}},
		{name: "no match", filter: task.TaskFilter{UserID: "nobody"}, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ListTasks(ctx, tt.filter)
			require.NoError(t, err)
			ids := make([]string, len(got))
			for i, tk := range got {
				ids[i] = tk.ID
			}
			assert.ElementsMatch(t, tt.want, ids)
		})
	}
}

func TestApproveTaskPlanPreconditions(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryTaskService(logging.New(), nil, nil)

	_, err := s.ApproveTaskPlan(ctx, "missing", task.ApproveTaskPlanRequest{Approved: true})
	assert.EqualError(t, err, "task not found: missing")

	created, err := s.CreateTask(ctx, task.CreateTaskRequest{Description: "d"})
	require.NoError(t, err)
	_, err = s.ApproveTaskPlan(ctx, created.ID, task.ApproveTaskPlanRequest{Approved: true})
	assert.EqualError(t, err, "task has no plan: "+created.ID)

	created.Plan = &task.Plan{}
	_, err = s.ApproveTaskPlan(ctx, created.ID, task.ApproveTaskPlanRequest{Approved: true})
	assert.EqualError(t, err, "task is not awaiting approval: "+created.ID)
}

func TestApproveTaskPlanWithoutExecutor(t *testing.T) {
	s := NewInMemoryTaskService(logging.New(), nil, nil)
	id := awaitingApproval(t, s)

	got, err := s.ApproveTaskPlan(context.Background(), id, task.ApproveTaskPlanRequest{Approved: true, Feedback: "lgtm"})
	require.NoError(t, err)
	assert.Equal(t, task.StatusExecuting, got.Status)
	assert.True(t, got.Plan.IsApproved)
	assert.NotNil(t, got.Plan.ApprovedAt)
	assert.NotNil(t, got.StartedAt)
	assert.Equal(t, "lgtm", got.Feedback)
	assert.Equal(t, "Plan approved", got.Logs[len(got.Logs)-1].Message)
}

func TestApproveTaskPlanRunsExecutor(t *testing.T) {
	runner := &signalRunner{ran: make(chan string, 1)}
	s := NewInMemoryTaskService(logging.New(), nil, runner)
	id := awaitingApproval(t, s)

	_, err := s.ApproveTaskPlan(context.Background(), id, task.ApproveTaskPlanRequest{Approved: true})
	require.NoError(t, err)

	select {
	case ran := <-runner.ran:
		assert.Equal(t, id, ran)
	case <-time.After(2 * time.Second):
		t.Fatal("executor was not called")
	}
	assert.Equal(t, task.StatusExecuting, snapshot(s, id).Status)
}

func TestApproveTaskPlanExecutorFailure(t *testing.T) {
	runner := &signalRunner{err: errBoom, ran: make(chan string, 1)}
	s := NewInMemoryTaskService(logging.New(), nil, runner)
	id := awaitingApproval(t, s)

	_, err := s.ApproveTaskPlan(context.Background(), id, task.ApproveTaskPlanRequest{Approved: true})
	require.NoError(t, err)

	got := waitForStatus(t, s, id, task.StatusFailed)
	assert.NotNil(t, got.CompletedAt)
}

func TestApproveTaskPlanRejectWithoutPlanner(t *testing.T) {
	s := NewInMemoryTaskService(logging.New(), nil, nil)
	id := awaitingApproval(t, s)

	got, err := s.ApproveTaskPlan(context.Background(), id, task.ApproveTaskPlanRequest{Approved: false, Feedback: "redo"})
	require.NoError(t, err)
	assert.Equal(t, task.StatusPlanning, got.Status)
	assert.False(t, got.Plan.IsApproved)
	assert.Equal(t, "redo", got.Feedback)
	assert.Equal(t, "Plan rejected", got.Logs[len(got.Logs)-1].Message)
}

func TestApproveTaskPlanRejectReplans(t *testing.T) {
	planner := newFakePlanner(nil)
	s := NewInMemoryTaskService(logging.New(), planner, nil)
	id := awaitingApproval(t, s)
	<-planner.calls // initial planning

	_, err := s.ApproveTaskPlan(context.Background(), id, task.ApproveTaskPlanRequest{Approved: false, Feedback: "redo"})
	require.NoError(t, err)

	select {
	case <-planner.calls:
	case <-time.After(2 * time.Second):
		t.Fatal("planner was not called again")
	}
	require.Eventually(t, func() bool {
		logs := logMessages(snapshot(s, id))
		return logs[len(logs)-1] == "Task has been replanned with feedback"
	}, 2*time.Second, time.Millisecond)

	got := snapshot(s, id)
	assert.Equal(t, task.StatusApproval, got.Status)
	assert.Contains(t, logMessages(got), "Replanning task with feedback")
}

func TestApproveTaskPlanReplanFailure(t *testing.T) {
	planner := newFakePlanner(nil)
	s := NewInMemoryTaskService(logging.New(), planner, nil)
	id := awaitingApproval(t, s)
	<-planner.calls

	planner.err = errBoom
	_, err := s.ApproveTaskPlan(context.Background(), id, task.ApproveTaskPlanRequest{Approved: false})
	require.NoError(t, err)

	got := waitForStatus(t, s, id, task.StatusFailed)
	assert.Equal(t, "Planning failed: boom", got.Logs[len(got.Logs)-1].Message)
}

func TestUpdateTask(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryTaskService(logging.New(), nil, nil)

	_, err := s.UpdateTask(ctx, "missing", nil)
	assert.EqualError(t, err, "task not found: missing")

	created, err := s.CreateTask(ctx, task.CreateTaskRequest{Description: "d"})
	require.NoError(t, err)
	id := created.ID

	got, err := s.UpdateTask(ctx, id, []task.TaskUpdate{
		{Type: "add_step", Description: "one"},
		{Type: "add_step", Description: "two"},
		{Type: "add_step", Description: "three"},
	})
	require.NoError(t, err)
	require.Len(t, got.Steps, 3)
	assert.Equal(t, 1, got.Steps[0].Order)
	assert.Equal(t, 3, got.Steps[2].Order)
	assert.Equal(t, task.StepStatusPending, got.Steps[0].Status)
	first, second, third := got.Steps[0].ID, got.Steps[1].ID, got.Steps[2].ID

	got, err = s.UpdateTask(ctx, id, []task.TaskUpdate{
		{Type: "modify_step", StepID: first, Description: "uno", Status: "completed"},
		{Type: "modify_step", StepID: third},
		{Type: "remove_step", StepID: second},
		{Type: "update_status", Status: "executing"},
		{Type: "add_comment", Description: "a note"},
		{Type: "unknown"},
	})
	require.NoError(t, err)
	require.Len(t, got.Steps, 2)
	assert.Equal(t, task.Step{ID: first, Description: "uno", Status: task.StepStatusCompleted, Order: 1}, got.Steps[0])
	assert.Equal(t, "three", got.Steps[1].Description, "empty fields leave the step unchanged")
	assert.Equal(t, task.StepStatusPending, got.Steps[1].Status)
	assert.Equal(t, task.StatusExecuting, got.Status)

	logs := logMessages(*got)
	assert.Equal(t, []string{
		"Task created",
		"Added step: one", "Added step: two", "Added step: three",
		"Modified step: uno", "Modified step: ",
		"Removed step",
		"Updated status to: executing",
		"a note",
	}, logs)
	assert.Equal(t, second, got.Logs[6].StepID)

	_, err = s.UpdateTask(ctx, id, []task.TaskUpdate{{Type: "modify_step", StepID: "nope"}})
	assert.EqualError(t, err, "step not found: nope")
	_, err = s.UpdateTask(ctx, id, []task.TaskUpdate{{Type: "remove_step", StepID: "nope"}})
	assert.EqualError(t, err, "step not found: nope")
}

func TestAddTaskLog(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryTaskService(logging.New(), nil, nil)

	assert.EqualError(t, s.AddTaskLog(ctx, "missing", "m", "info"), "task not found: missing")

	created, err := s.CreateTask(ctx, task.CreateTaskRequest{Description: "d"})
	require.NoError(t, err)
	require.NoError(t, s.AddTaskLog(ctx, created.ID, "careful", "warning"))

	last := created.Logs[len(created.Logs)-1]
	assert.Equal(t, "careful", last.Message)
	assert.Equal(t, "warning", last.Level)
	assert.Equal(t, created.ID, last.TaskID)
	assert.NotEmpty(t, last.ID)
}
