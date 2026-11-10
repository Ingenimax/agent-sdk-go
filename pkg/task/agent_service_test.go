package task

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAdapterService records calls and returns configured results.
type fakeAdapterService struct {
	task        *Task
	tasks       []*Task
	err         error
	logErr      error
	updates     [][]TaskUpdate
	logs        []string
	lastFilter  TaskFilter
	lastTaskID  string
	lastCreated CreateTaskRequest
}

func (f *fakeAdapterService) CreateTask(_ context.Context, req CreateTaskRequest) (*Task, error) {
	f.lastCreated = req
	return f.task, f.err
}

func (f *fakeAdapterService) GetTask(_ context.Context, taskID string) (*Task, error) {
	f.lastTaskID = taskID
	return f.task, f.err
}

func (f *fakeAdapterService) ListTasks(_ context.Context, filter TaskFilter) ([]*Task, error) {
	f.lastFilter = filter
	return f.tasks, f.err
}

func (f *fakeAdapterService) ApproveTaskPlan(_ context.Context, _ string, _ ApproveTaskPlanRequest) (*Task, error) {
	return f.task, f.err
}

func (f *fakeAdapterService) UpdateTask(_ context.Context, taskID string, updates []TaskUpdate) (*Task, error) {
	f.lastTaskID = taskID
	f.updates = append(f.updates, updates)
	return f.task, f.err
}

func (f *fakeAdapterService) AddTaskLog(_ context.Context, taskID, message, level string) error {
	f.lastTaskID = taskID
	f.logs = append(f.logs, level+":"+message)
	return f.logErr
}

// fakeCoreService is a configurable interfaces.TaskService.
type fakeCoreService struct {
	obj     interface{}
	list    []interface{}
	err     error
	updates interface{}
}

func (f *fakeCoreService) CreateTask(context.Context, interface{}) (interface{}, error) {
	return f.obj, f.err
}

func (f *fakeCoreService) GetTask(context.Context, string) (interface{}, error) {
	return f.obj, f.err
}

func (f *fakeCoreService) ListTasks(context.Context, interface{}) ([]interface{}, error) {
	return f.list, f.err
}

func (f *fakeCoreService) ApproveTaskPlan(context.Context, string, interface{}) (interface{}, error) {
	return f.obj, f.err
}

func (f *fakeCoreService) UpdateTask(_ context.Context, _ string, updates interface{}) (interface{}, error) {
	f.updates = updates
	return f.obj, f.err
}

func (f *fakeCoreService) AddTaskLog(context.Context, string, string, string) error {
	return f.err
}

func newService(t *testing.T) *AgentTaskService {
	t.Helper()
	svc, err := NewAgentTaskService(logging.New())
	require.NoError(t, err)
	return svc
}

func TestAgentTaskServiceLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	assert.Nil(t, svc.CurrentTask())

	created, err := svc.CreateTask(ctx, "Deploy", "deploy the app", "user1", map[string]interface{}{"k": "v"})
	require.NoError(t, err)
	assert.Equal(t, "Deploy", created.Title)
	assert.Equal(t, "deploy the app", created.Description)
	assert.Equal(t, "user1", created.UserID)
	assert.Equal(t, StatusPending, created.Status)
	assert.Same(t, created, svc.CurrentTask())

	got, err := svc.GetTask(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)

	require.NoError(t, svc.AddTaskStep(ctx, "build image"))
	require.NoError(t, svc.AddTaskStep(ctx, "push image"))
	current := svc.CurrentTask()
	require.Len(t, current.Steps, 2)
	assert.Equal(t, "build image", current.Steps[0].Description)
	assert.Equal(t, 0, current.Steps[0].Order)
	assert.Equal(t, "push image", current.Steps[1].Description)
	assert.Equal(t, 1, current.Steps[1].Order)
	assert.Equal(t, StatusPending, current.Steps[0].Status)

	require.NoError(t, svc.UpdateTaskStatus(ctx, StatusExecuting))
	assert.Equal(t, StatusExecuting, svc.CurrentTask().Status)

	require.NoError(t, svc.UpdateTaskStep(ctx, current.Steps[0].ID, StatusCompleted, "done", errors.New("boom")))

	require.NoError(t, svc.LogTaskInfo(ctx, "info"))
	require.NoError(t, svc.LogTaskError(ctx, "error"))
	require.NoError(t, svc.LogTaskDebug(ctx, "debug"))

	svc.ResetCurrentTask()
	assert.Nil(t, svc.CurrentTask())

	started, err := svc.StartTask(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, started.ID)
	assert.Equal(t, created.ID, svc.CurrentTask().ID)

	_, err = svc.StartTask(ctx, "missing")
	require.Error(t, err)
	assert.Equal(t, created.ID, svc.CurrentTask().ID, "a failed start keeps the current task")
}

func TestAgentTaskServiceListTasksFiltersByUser(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	for _, user := range []string{"alice", "alice", "bob"} {
		_, err := svc.CreateTask(ctx, "t", "d", user, nil)
		require.NoError(t, err)
	}

	alice, err := svc.ListTasks(ctx, "alice")
	require.NoError(t, err)
	assert.Len(t, alice, 2)
	for _, task := range alice {
		assert.Equal(t, "alice", task.UserID)
	}

	all, err := svc.ListTasks(ctx, "")
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestAgentTaskServiceRequiresCurrentTask(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	calls := map[string]func() error{
		"AddTaskStep":      func() error { return svc.AddTaskStep(ctx, "x") },
		"UpdateTaskStep":   func() error { return svc.UpdateTaskStep(ctx, "s", StatusCompleted, "", nil) },
		"UpdateTaskStatus": func() error { return svc.UpdateTaskStatus(ctx, StatusCompleted) },
		"LogTaskInfo":      func() error { return svc.LogTaskInfo(ctx, "x") },
		"LogTaskError":     func() error { return svc.LogTaskError(ctx, "x") },
		"LogTaskDebug":     func() error { return svc.LogTaskDebug(ctx, "x") },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			assert.EqualError(t, call(), "no current task")
		})
	}
}

func TestAgentTaskServiceWithAdapterForwardsCalls(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAdapterService{task: &Task{ID: "t1"}}
	svc := NewAgentTaskServiceWithAdapter(logging.New(), fake)

	_, err := svc.CreateTask(ctx, "title", "desc", "u1", map[string]interface{}{"a": 1})
	require.NoError(t, err)
	assert.Equal(t, CreateTaskRequest{Title: "title", Description: "desc", UserID: "u1", Metadata: map[string]interface{}{"a": 1}}, fake.lastCreated)

	_, err = svc.ListTasks(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, TaskFilter{UserID: "u1"}, fake.lastFilter)

	require.NoError(t, svc.UpdateTaskStep(ctx, "s1", StatusFailed, "out", errors.New("bad")))
	require.NoError(t, svc.UpdateTaskStep(ctx, "s2", StatusCompleted, "", nil))
	require.Len(t, fake.updates, 2)
	assert.Equal(t, []TaskUpdate{
		{Type: "modify_step", StepID: "s1", Status: "failed"},
		{Type: "add_log", StepID: "s1", Description: "Error: bad"},
		{Type: "add_log", StepID: "s1", Description: "Output: out"},
	}, fake.updates[0])
	assert.Equal(t, []TaskUpdate{{Type: "modify_step", StepID: "s2", Status: "completed"}}, fake.updates[1])

	require.NoError(t, svc.LogTaskInfo(ctx, "i"))
	require.NoError(t, svc.LogTaskError(ctx, "e"))
	require.NoError(t, svc.LogTaskDebug(ctx, "d"))
	assert.Equal(t, []string{"info:i", "error:e", "debug:d"}, fake.logs)
	assert.Equal(t, "t1", fake.lastTaskID)
}

func TestAgentTaskServiceWithAdapterPropagatesErrors(t *testing.T) {
	ctx := context.Background()
	errBoom := errors.New("boom")
	fake := &fakeAdapterService{task: &Task{ID: "t1"}}
	svc := NewAgentTaskServiceWithAdapter(logging.New(), fake)

	_, err := svc.CreateTask(ctx, "t", "d", "u", nil)
	require.NoError(t, err)
	current := svc.CurrentTask()

	fake.err = errBoom
	fake.logErr = errBoom

	_, err = svc.CreateTask(ctx, "t", "d", "u", nil)
	assert.ErrorIs(t, err, errBoom)
	assert.ErrorIs(t, svc.AddTaskStep(ctx, "x"), errBoom)
	assert.ErrorIs(t, svc.UpdateTaskStep(ctx, "s", StatusCompleted, "", nil), errBoom)
	assert.ErrorIs(t, svc.UpdateTaskStatus(ctx, StatusCompleted), errBoom)
	assert.ErrorIs(t, svc.LogTaskInfo(ctx, "x"), errBoom)
	_, err = svc.StartTask(ctx, "t1")
	assert.ErrorIs(t, err, errBoom)

	assert.Same(t, current, svc.CurrentTask(), "failed calls must not replace the current task")
}

func TestFormatTaskProgress(t *testing.T) {
	svc := NewAgentTaskServiceWithAdapter(logging.New(), &fakeAdapterService{})
	assert.Equal(t, "No active task", svc.FormatTaskProgress())

	// Without a plan, only the header line is printed.
	svc.currentTask = &Task{Title: "T", Status: StatusExecuting, Steps: []Step{{Description: "a"}}}
	assert.Equal(t, "Task: T (Status: executing)\n", svc.FormatTaskProgress())

	svc.currentTask = &Task{
		Title:  "T",
		Status: StatusExecuting,
		Plan:   &Plan{},
		Steps: []Step{
			{Description: "pending step", Status: StatusPending},
			{Description: "done step", Status: StatusCompleted},
			{Description: "failed step", Status: StatusFailed},
			{Description: "running step", Status: StatusExecuting},
		},
	}
	out := svc.FormatTaskProgress()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	require.Len(t, lines, 6)
	assert.Equal(t, "Task: T (Status: executing)", lines[0])
	assert.Equal(t, "Progress:", lines[1])

	// Each status gets its own marker between the index and the description.
	markers := map[string]bool{}
	for i, line := range lines[2:] {
		desc := svc.currentTask.Steps[i].Description
		prefix := "  " + string(rune('1'+i)) + ". "
		require.True(t, strings.HasPrefix(line, prefix), line)
		require.True(t, strings.HasSuffix(line, " "+desc), line)
		marker := strings.TrimSuffix(strings.TrimPrefix(line, prefix), " "+desc)
		assert.NotEmpty(t, marker)
		markers[marker] = true
	}
	assert.Len(t, markers, 4)
}

func TestSimplePlannerAndExecutor(t *testing.T) {
	ctx := context.Background()

	plan, err := (&SimplePlanner{}).CreatePlan(ctx, &Task{Title: "X"})
	require.NoError(t, err)
	assert.Equal(t, "Simple plan for X", plan)

	exec := &SimpleExecutor{}
	step := &Step{Status: StepStatusPending}
	require.NoError(t, exec.ExecuteStep(ctx, nil, step))
	assert.Equal(t, StepStatusCompleted, step.Status)
	assert.NotNil(t, step.CompletedAt)

	task := &Task{Steps: []Step{{Status: StepStatusPending}, {Status: StepStatusFailed}}}
	require.NoError(t, exec.ExecuteTask(ctx, task))
	for _, s := range task.Steps {
		assert.Equal(t, StepStatusCompleted, s.Status)
		assert.NotNil(t, s.CompletedAt)
	}
}

func TestSimplePlannerCore(t *testing.T) {
	p := &SimplePlannerCore{}
	plan, err := p.CreatePlan(context.Background(), &core.Task{Name: "N"})
	require.NoError(t, err)
	assert.Equal(t, "Simple plan for N", plan)

	plan, err = p.CreatePlan(context.Background(), "not a task")
	require.NoError(t, err)
	assert.Equal(t, "Simple plan", plan)
}

func newMemoryService() *SimpleMemoryService {
	return &SimpleMemoryService{
		tasks:  make(map[string]*core.Task),
		logs:   make(map[string][]*core.Log),
		logger: logging.New(),
	}
}

func TestSimpleMemoryService(t *testing.T) {
	ctx := context.Background()
	s := newMemoryService()

	_, err := s.CreateTask(ctx, "bad")
	assert.EqualError(t, err, "invalid request type")

	obj, err := s.CreateTask(ctx, core.CreateTaskRequest{Name: "n", Description: "d", UserID: "u1"})
	require.NoError(t, err)
	created := obj.(*core.Task)
	assert.Equal(t, core.StatusPending, created.Status)
	_, err = s.CreateTask(ctx, core.CreateTaskRequest{UserID: "u2"})
	require.NoError(t, err)

	_, err = s.GetTask(ctx, "missing")
	assert.EqualError(t, err, "task not found: missing")

	list, err := s.ListTasks(ctx, core.TaskFilter{UserID: "u1"})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	list, err = s.ListTasks(ctx, "no filter")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	t.Run("approve", func(t *testing.T) {
		_, err := s.ApproveTaskPlan(ctx, "missing", core.ApproveTaskPlanRequest{})
		assert.Error(t, err)
		_, err = s.ApproveTaskPlan(ctx, created.ID, "bad")
		assert.EqualError(t, err, "invalid request type")

		obj, err := s.ApproveTaskPlan(ctx, created.ID, core.ApproveTaskPlanRequest{Approved: false})
		require.NoError(t, err)
		assert.Equal(t, core.StatusPending, obj.(*core.Task).Status)

		obj, err = s.ApproveTaskPlan(ctx, created.ID, core.ApproveTaskPlanRequest{Approved: true})
		require.NoError(t, err)
		assert.Equal(t, core.StatusExecuting, obj.(*core.Task).Status)
	})

	t.Run("update", func(t *testing.T) {
		_, err := s.UpdateTask(ctx, "missing", []core.TaskUpdate{})
		assert.Error(t, err)
		_, err = s.UpdateTask(ctx, created.ID, "bad")
		assert.EqualError(t, err, "invalid updates type")

		obj, err := s.UpdateTask(ctx, created.ID, []core.TaskUpdate{
			{Field: "add_step", Value: map[string]interface{}{"description": "s1"}},
			{Field: "add_step", Value: "ignored"},
			{Field: "status", Value: "completed"},
			{Field: "status", Value: 42},
			{Field: "unknown", Value: "x"},
		})
		require.NoError(t, err)
		task := obj.(*core.Task)
		require.Len(t, task.Steps, 1)
		assert.Equal(t, "s1", task.Steps[0].Description)
		assert.Equal(t, core.StatusCompleted, task.Status)
	})

	t.Run("logs", func(t *testing.T) {
		assert.Error(t, s.AddTaskLog(ctx, "missing", "m", "info"))
		require.NoError(t, s.AddTaskLog(ctx, created.ID, "m", "info"))
		require.Len(t, s.logs[created.ID], 1)
		assert.Equal(t, "m", s.logs[created.ID][0].Message)
		assert.Equal(t, "info", s.logs[created.ID][0].Level)
	})
}

func TestSimpleBridgeAdapterConvertsTasks(t *testing.T) {
	ctx := context.Background()
	coreTask := &core.Task{
		ID:       "c1",
		Name:     "name",
		Status:   core.StatusExecuting,
		UserID:   "u",
		Metadata: map[string]interface{}{"m": 1},
		Steps:    []*core.Step{{ID: "s1", Description: "d", Status: core.StatusCompleted, OrderIndex: 3}},
	}
	fake := &fakeCoreService{obj: coreTask, list: []interface{}{coreTask, "skip me"}}
	a := &SimpleBridgeAdapter{coreService: fake, logger: logging.New()}

	got, err := a.GetTask(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "name", got.Title)
	assert.Equal(t, StatusExecuting, got.Status)
	assert.Equal(t, map[string]interface{}{"m": 1}, got.Metadata)
	require.Len(t, got.Steps, 1)
	assert.Equal(t, Step{ID: "s1", Description: "d", Status: StatusCompleted, Order: 3}, got.Steps[0])

	list, err := a.ListTasks(ctx, TaskFilter{})
	require.NoError(t, err)
	assert.Len(t, list, 1, "non-task items are skipped")

	got, err = a.ApproveTaskPlan(ctx, "c1", ApproveTaskPlanRequest{Approved: true})
	require.NoError(t, err)
	assert.Equal(t, "c1", got.ID)

	_, err = a.UpdateTask(ctx, "c1", []TaskUpdate{
		{Type: "add_step", Description: "x"},
		{Type: "update_status", Status: "completed"},
		{Type: "modify_step", StepID: "s1"},
	})
	require.NoError(t, err)
	updates, ok := fake.updates.([]core.TaskUpdate)
	require.True(t, ok)
	assert.Contains(t, updates, core.TaskUpdate{Field: "add_step", Value: map[string]interface{}{"description": "x"}})
	assert.Contains(t, updates, core.TaskUpdate{Field: "status", Value: "completed"})
}

func TestSimpleBridgeAdapterErrors(t *testing.T) {
	ctx := context.Background()
	errBoom := errors.New("boom")

	tests := []struct {
		name string
		core *fakeCoreService
		want string
	}{
		{name: "core error", core: &fakeCoreService{err: errBoom}, want: "boom"},
		{name: "wrong type", core: &fakeCoreService{obj: "not a task"}, want: "unexpected type returned from core service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &SimpleBridgeAdapter{coreService: tt.core, logger: logging.New()}

			_, err := a.CreateTask(ctx, CreateTaskRequest{})
			assert.EqualError(t, err, tt.want)
			_, err = a.GetTask(ctx, "x")
			assert.EqualError(t, err, tt.want)
			_, err = a.ApproveTaskPlan(ctx, "x", ApproveTaskPlanRequest{})
			assert.EqualError(t, err, tt.want)
			_, err = a.UpdateTask(ctx, "x", nil)
			assert.EqualError(t, err, tt.want)
		})
	}

	a := &SimpleBridgeAdapter{coreService: &fakeCoreService{err: errBoom}, logger: logging.New()}
	_, err := a.ListTasks(ctx, TaskFilter{})
	assert.ErrorIs(t, err, errBoom)
	assert.ErrorIs(t, a.AddTaskLog(ctx, "x", "m", "info"), errBoom)
}
