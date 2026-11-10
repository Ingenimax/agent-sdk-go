package adapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBoom = errors.New("boom")

// fakeTaskService is a configurable interfaces.TaskService that records the
// arguments it receives.
type fakeTaskService struct {
	obj      interface{}
	list     []interface{}
	err      error
	getObj   interface{}
	getErr   error
	req      interface{}
	filter   interface{}
	updates  interface{}
	taskID   string
	logCalls []string
}

func (f *fakeTaskService) CreateTask(_ context.Context, req interface{}) (interface{}, error) {
	f.req = req
	return f.obj, f.err
}

func (f *fakeTaskService) GetTask(_ context.Context, taskID string) (interface{}, error) {
	f.taskID = taskID
	if f.getObj != nil || f.getErr != nil {
		return f.getObj, f.getErr
	}
	return f.obj, f.err
}

func (f *fakeTaskService) ListTasks(_ context.Context, filter interface{}) ([]interface{}, error) {
	f.filter = filter
	return f.list, f.err
}

func (f *fakeTaskService) ApproveTaskPlan(_ context.Context, taskID string, req interface{}) (interface{}, error) {
	f.taskID = taskID
	f.req = req
	return f.obj, f.err
}

func (f *fakeTaskService) UpdateTask(_ context.Context, taskID string, updates interface{}) (interface{}, error) {
	f.taskID = taskID
	f.updates = updates
	return f.obj, f.err
}

func (f *fakeTaskService) AddTaskLog(_ context.Context, taskID, message, level string) error {
	f.taskID = taskID
	f.logCalls = append(f.logCalls, level+":"+message)
	return f.err
}

func TestAdapterOptions(t *testing.T) {
	opts := &AdapterOptions{}
	WithMetadata(true)(opts)
	WithDefaultUserID("u1")(opts)
	assert.Equal(t, AdapterOptions{IncludeMetadata: true, DefaultUserID: "u1"}, *opts)
}

func TestDefaultTaskAdapterRequests(t *testing.T) {
	a := NewDefaultTaskAdapter(logging.New())

	assert.Equal(t,
		task.CreateTaskRequest{Description: "d", UserID: "u", Title: "t", TaskKind: "k"},
		a.ConvertCreateRequest(DefaultCreateRequest{Description: "d", UserID: "u", Title: "t", TaskKind: "k"}))

	assert.Equal(t,
		task.ApproveTaskPlanRequest{Approved: true, Feedback: "f"},
		a.ConvertApproveRequest(DefaultApproveRequest{Approved: true, Feedback: "f"}))

	assert.Nil(t, a.ConvertTaskUpdates(nil))
	assert.Equal(t,
		[]task.TaskUpdate{{Type: "modify_step", StepID: "s", Description: "d", Status: "completed"}},
		a.ConvertTaskUpdates([]DefaultTaskUpdate{{Type: "modify_step", StepID: "s", Description: "d", Status: "completed"}}))
}

func TestDefaultTaskAdapterConvertTask(t *testing.T) {
	a := NewDefaultTaskAdapter(logging.New())
	now := time.Now()
	planStep := task.Step{ID: "p1", Description: "plan step", Status: task.StatusCompleted, StartedAt: &now, CompletedAt: &now, Error: "e", Output: "o"}
	directStep := task.Step{ID: "d1", Description: "direct step", Status: task.StatusPending}

	assert.Nil(t, a.ConvertTask(nil))

	t.Run("plan steps win over direct steps", func(t *testing.T) {
		got := a.ConvertTask(&task.Task{
			ID:             "t1",
			Description:    "desc",
			Status:         task.StatusExecuting,
			Title:          "title",
			TaskKind:       "kind",
			ConversationID: "c1",
			CreatedAt:      now,
			UpdatedAt:      now,
			StartedAt:      &now,
			CompletedAt:    &now,
			UserID:         "u1",
			Requirements:   "req",
			Feedback:       "fb",
			Metadata:       map[string]interface{}{"m": 1},
			Plan:           &task.Plan{Steps: []task.Step{planStep}},
			Steps:          []task.Step{directStep},
		})
		assert.Equal(t, &DefaultTask{
			ID:             "t1",
			Description:    "desc",
			Status:         "executing",
			Title:          "title",
			TaskKind:       "kind",
			ConversationID: "c1",
			CreatedAt:      now,
			UpdatedAt:      now,
			StartedAt:      &now,
			CompletedAt:    &now,
			UserID:         "u1",
			Requirements:   "req",
			Feedback:       "fb",
			Metadata:       map[string]interface{}{"m": 1},
			Steps: []DefaultStep{{
				ID: "p1", Description: "plan step", Status: "completed",
				StartTime: &now, EndTime: &now, Error: "e", Output: "o",
			}},
		}, got)
	})

	t.Run("direct steps when the plan is empty", func(t *testing.T) {
		got := a.ConvertTask(&task.Task{Plan: &task.Plan{}, Steps: []task.Step{directStep}})
		require.Len(t, got.Steps, 1)
		assert.Equal(t, "d1", got.Steps[0].ID)
	})

	t.Run("no steps", func(t *testing.T) {
		assert.Nil(t, a.ConvertTask(&task.Task{ID: "x"}).Steps)
	})

	t.Run("convert tasks", func(t *testing.T) {
		got := a.ConvertTasks([]*task.Task{{ID: "a"}, nil, {ID: "b"}})
		require.Len(t, got, 3)
		assert.Equal(t, "a", got[0].ID)
		assert.Nil(t, got[1])
		assert.Equal(t, "b", got[2].ID)
		assert.Nil(t, a.ConvertTasks(nil))
	})
}

func newAdapterService(fake *fakeTaskService) *AdapterService[*DefaultTask, DefaultCreateRequest, DefaultApproveRequest, DefaultTaskUpdate] {
	return NewAdapterService(logging.New(), fake, NewDefaultTaskAdapter(logging.New()))
}

func TestAdapterServiceSuccess(t *testing.T) {
	ctx := context.Background()
	sdkTask := &task.Task{ID: "t1", Title: "title", Status: task.StatusPending}
	fake := &fakeTaskService{obj: sdkTask, list: []interface{}{sdkTask, "skip", &task.Task{ID: "t2"}}}
	svc := newAdapterService(fake)

	got, err := svc.CreateTask(ctx, DefaultCreateRequest{Description: "d", UserID: "u"})
	require.NoError(t, err)
	assert.Equal(t, "t1", got.ID)
	assert.Equal(t, task.CreateTaskRequest{Description: "d", UserID: "u"}, fake.req)

	got, err = svc.GetTask(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, "title", got.Title)
	assert.Equal(t, "t1", fake.taskID)

	list, err := svc.ListTasks(ctx, "u1")
	require.NoError(t, err)
	require.Len(t, list, 2, "non-task items are skipped")
	assert.Equal(t, "t2", list[1].ID)
	assert.Equal(t, task.TaskFilter{UserID: "u1"}, fake.filter)

	_, err = svc.ApproveTaskPlan(ctx, "t1", DefaultApproveRequest{Approved: true, Feedback: "ok"})
	require.NoError(t, err)
	assert.Equal(t, task.ApproveTaskPlanRequest{Approved: true, Feedback: "ok"}, fake.req)

	_, err = svc.UpdateTask(ctx, "t1", "", []DefaultTaskUpdate{{Type: "add_step", Description: "s"}})
	require.NoError(t, err)
	assert.Equal(t, []task.TaskUpdate{{Type: "add_step", Description: "s"}}, fake.updates)

	// With a conversation ID the task is fetched before the update.
	fake.taskID = ""
	_, err = svc.UpdateTask(ctx, "t1", "conv", nil)
	require.NoError(t, err)
	assert.Equal(t, "t1", fake.taskID)

	require.NoError(t, svc.AddTaskLog(ctx, "t1", "msg", "info"))
	assert.Equal(t, []string{"info:msg"}, fake.logCalls)
}

func TestAdapterServiceErrors(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		fake *fakeTaskService
		want string
	}{
		{name: "service error", fake: &fakeTaskService{err: errBoom}, want: "boom"},
		{name: "wrong type", fake: &fakeTaskService{obj: "not a task"}, want: "unexpected type returned from core service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newAdapterService(tt.fake)

			got, err := svc.CreateTask(ctx, DefaultCreateRequest{})
			assert.EqualError(t, err, tt.want)
			assert.Nil(t, got)
			_, err = svc.GetTask(ctx, "t")
			assert.EqualError(t, err, tt.want)
			_, err = svc.ApproveTaskPlan(ctx, "t", DefaultApproveRequest{})
			assert.EqualError(t, err, tt.want)
			_, err = svc.UpdateTask(ctx, "t", "", nil)
			assert.EqualError(t, err, tt.want)
			_, err = svc.UpdateTask(ctx, "t", "conv", nil)
			assert.EqualError(t, err, tt.want)
		})
	}

	svc := newAdapterService(&fakeTaskService{err: errBoom})
	_, err := svc.ListTasks(ctx, "u")
	assert.ErrorIs(t, err, errBoom)
	assert.ErrorIs(t, svc.AddTaskLog(ctx, "t", "m", "info"), errBoom)

	// The pre-fetch succeeds but the update itself fails.
	svc = newAdapterService(&fakeTaskService{getObj: &task.Task{ID: "t"}, err: errBoom})
	_, err = svc.UpdateTask(ctx, "t", "conv", nil)
	assert.ErrorIs(t, err, errBoom)
}

func TestCoreAdapterCreateTaskRequestToCore(t *testing.T) {
	a := NewCoreAdapter(logging.New())
	want := core.CreateTaskRequest{
		Name:           "n",
		Description:    "d",
		UserID:         "u",
		ConversationID: "c",
		Input:          map[string]interface{}{"i": 1},
		Metadata:       map[string]interface{}{"m": 2},
	}

	assert.Equal(t, want, a.CreateTaskRequestToCore(want))
	assert.Equal(t, want, a.CreateTaskRequestToCore(map[string]interface{}{
		"name":            "n",
		"description":     "d",
		"user_id":         "u",
		"conversation_id": "c",
		"input":           map[string]interface{}{"i": 1},
		"metadata":        map[string]interface{}{"m": 2},
	}))
	assert.Equal(t,
		core.CreateTaskRequest{Name: "n", Description: "d", UserID: "u", ConversationID: "c"},
		a.CreateTaskRequestToCore(map[string]interface{}{
			"name": "n", "description": "d", "user_id": "u", "conversation_id": "c",
		}))
	assert.Equal(t, core.CreateTaskRequest{}, a.CreateTaskRequestToCore(42))
}

func TestCoreAdapterCoreTaskToTask(t *testing.T) {
	a := NewCoreAdapter(logging.New())
	assert.Nil(t, a.CoreTaskToTask(nil))

	now := time.Now()
	step := &core.Step{ID: "s1", Name: "n", Type: "chat", Status: core.StatusCompleted, OrderIndex: 2, CompletedAt: &now, Output: map[string]interface{}{"r": 1}}
	src := &core.Task{
		ID:       "t1",
		Name:     "name",
		Status:   core.StatusExecuting,
		Plan:     "plan",
		Steps:    []*core.Step{step},
		Metadata: map[string]interface{}{"m": 1},
	}

	got, ok := a.CoreTaskToTask(src).(*core.Task)
	require.True(t, ok)
	assert.NotSame(t, src, got)
	assert.Equal(t, src, got)
	require.Len(t, got.Steps, 1)
	assert.NotSame(t, step, got.Steps[0], "steps are copied")
	assert.Equal(t, *step, *got.Steps[0])
}

func TestCoreAdapterFactories(t *testing.T) {
	a := NewCoreAdapter(logging.New())
	meta := map[string]interface{}{"k": "v"}

	tk := a.CreateTaskFromDetails("n", "d", "u", meta)
	assert.NotEmpty(t, tk.ID)
	assert.Equal(t, "n", tk.Name)
	assert.Equal(t, "d", tk.Description)
	assert.Equal(t, "u", tk.UserID)
	assert.Equal(t, core.StatusPending, tk.Status)
	assert.Equal(t, meta, tk.Metadata)
	assert.Empty(t, tk.Steps)
	assert.False(t, tk.CreatedAt.IsZero())
	assert.NotEqual(t, tk.ID, a.CreateTaskFromDetails("n", "d", "u", nil).ID)

	step := a.CreateStep("t1", "build", "exec", 3)
	assert.NotEmpty(t, step.ID)
	assert.Equal(t, "Step build", step.Name)
	assert.Equal(t, "build", step.Description)
	assert.Equal(t, "exec", step.Type)
	assert.Equal(t, 3, step.OrderIndex)
	assert.Equal(t, core.StatusPending, step.Status)
	assert.NotNil(t, step.Output)

	log := a.CreateLog("t1", "msg", "warn")
	assert.NotEmpty(t, log.ID)
	assert.Equal(t, "t1", log.TaskID)
	assert.Equal(t, "msg", log.Message)
	assert.Equal(t, "warn", log.Level)
	assert.False(t, log.CreatedAt.IsZero())
}

func TestCoreAdapterUpdateTaskStatusFromStep(t *testing.T) {
	steps := func(statuses ...core.Status) []*core.Step {
		out := make([]*core.Step, len(statuses))
		for i, s := range statuses {
			out[i] = &core.Step{Status: s}
		}
		return out
	}

	tests := []struct {
		name          string
		status        core.Status
		steps         []*core.Step
		want          core.Status
		wantCompleted bool
		wantFailed    bool
		wantUpdated   bool
	}{
		{name: "not executing is untouched", status: core.StatusPending, steps: steps(core.StatusCompleted), want: core.StatusPending},
		{name: "all completed", status: core.StatusExecuting, steps: steps(core.StatusCompleted, core.StatusCompleted), want: core.StatusCompleted, wantCompleted: true, wantUpdated: true},
		{name: "any failed", status: core.StatusExecuting, steps: steps(core.StatusCompleted, core.StatusFailed), want: core.StatusFailed, wantFailed: true, wantUpdated: true},
		{name: "still running", status: core.StatusExecuting, steps: steps(core.StatusCompleted, core.StatusExecuting), want: core.StatusExecuting, wantUpdated: true},
		{name: "no steps", status: core.StatusExecuting, want: core.StatusExecuting, wantUpdated: true},
	}
	a := NewCoreAdapter(logging.New())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := &core.Task{Status: tt.status, Steps: tt.steps}
			a.UpdateTaskStatusFromStep(tk)
			assert.Equal(t, tt.want, tk.Status)
			assert.Equal(t, tt.wantCompleted, tk.CompletedAt != nil)
			assert.Equal(t, tt.wantFailed, tk.FailedAt != nil)
			assert.Equal(t, tt.wantUpdated, !tk.UpdatedAt.IsZero())
		})
	}
}
