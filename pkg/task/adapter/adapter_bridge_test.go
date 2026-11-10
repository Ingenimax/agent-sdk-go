package adapter

import (
	"context"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBridge(fake *fakeTaskService) Service {
	return NewCoreBridgeAdapter(fake, logging.New())
}

func TestCoreBridgeAdapterCreateTask(t *testing.T) {
	now := time.Now()
	fake := &fakeTaskService{obj: &core.Task{
		ID:          "t1",
		Name:        "name",
		Description: "desc",
		Status:      core.StatusAwaitingApproval,
		UserID:      "u1",
		CreatedAt:   now,
		CompletedAt: &now,
		Plan:        "a plan",
		Steps: []*core.Step{
			{ID: "s1", Description: "d1", Status: core.StatusCompleted, OrderIndex: 0, Output: map[string]interface{}{"result": "out"}},
			{ID: "s2", Description: "d2", Status: core.StatusPending, OrderIndex: 1, Output: map[string]interface{}{"result": 5}},
			{ID: "s3", Status: core.StatusFailed, Error: "bad"},
		},
	}}

	got, err := newBridge(fake).CreateTask(context.Background(), task.CreateTaskRequest{
		Title: "name", Description: "desc", UserID: "u1", TaskKind: "k", Metadata: map[string]interface{}{"m": 1},
	})
	require.NoError(t, err)

	assert.Equal(t, core.CreateTaskRequest{
		Name: "name", Description: "desc", UserID: "u1", Metadata: map[string]interface{}{"m": 1},
	}, fake.req)

	assert.Equal(t, "t1", got.ID)
	assert.Equal(t, "name", got.Title)
	assert.Equal(t, task.StatusApproval, got.Status)
	assert.Equal(t, &now, got.CompletedAt)
	require.Len(t, got.Steps, 3)
	assert.Equal(t, task.Step{ID: "s1", PlanID: "s1_plan", Description: "d1", Status: task.StatusCompleted, Order: 0, Output: "out"}, got.Steps[0])
	assert.Empty(t, got.Steps[1].Output, "non-string results are dropped")
	assert.Equal(t, "bad", got.Steps[2].Error)

	require.NotNil(t, got.Plan)
	assert.Equal(t, "t1_plan", got.Plan.ID)
	assert.Equal(t, "t1", got.Plan.TaskID)
	assert.False(t, got.Plan.IsApproved)
	assert.Equal(t, got.Steps, got.Plan.Steps)
}

func TestCoreBridgeAdapterPlanApproval(t *testing.T) {
	tests := []struct {
		status core.Status
		want   bool
	}{
		{core.StatusExecuting, true},
		{core.StatusCompleted, true},
		{core.StatusPlanning, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			fake := &fakeTaskService{obj: &core.Task{ID: "t", Status: tt.status, Plan: "p"}}
			got, err := newBridge(fake).GetTask(context.Background(), "t")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Plan.IsApproved)
			assert.Nil(t, got.Steps)
		})
	}

	fake := &fakeTaskService{obj: &core.Task{ID: "t"}}
	got, err := newBridge(fake).GetTask(context.Background(), "t")
	require.NoError(t, err)
	assert.Nil(t, got.Plan, "no plan without a plan string")
}

func TestCoreBridgeAdapterStatusMapping(t *testing.T) {
	a := &CoreBridgeAdapter{}
	pairs := []struct {
		core core.Status
		task task.Status
	}{
		{core.StatusPending, task.StatusPending},
		{core.StatusPlanning, task.StatusPlanning},
		{core.StatusAwaitingApproval, task.StatusApproval},
		{core.StatusExecuting, task.StatusExecuting},
		{core.StatusCompleted, task.StatusCompleted},
		{core.StatusFailed, task.StatusFailed},
	}
	for _, p := range pairs {
		assert.Equal(t, p.task, a.coreStatusToTaskStatus(p.core), string(p.core))
		assert.Equal(t, p.core, a.taskStatusToCoreStatus(p.task), string(p.task))
	}
	assert.Equal(t, core.StatusPending, a.taskStatusToCoreStatus("unknown"))
}

func TestCoreBridgeAdapterListTasks(t *testing.T) {
	from := time.Now().Add(-time.Hour)
	to := time.Now()
	fake := &fakeTaskService{list: []interface{}{&core.Task{ID: "a"}, "skip", &core.Task{ID: "b"}}}
	b := newBridge(fake)

	got, err := b.ListTasks(context.Background(), task.TaskFilter{
		UserID:         "u",
		ConversationID: "c",
		Status:         []task.Status{task.StatusApproval},
		CreatedAfter:   &from,
		CreatedBefore:  &to,
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].ID)
	assert.Equal(t, "b", got[1].ID)
	assert.Equal(t, core.TaskFilter{
		UserID:         "u",
		ConversationID: "c",
		Status:         core.StatusAwaitingApproval,
		FromDate:       &from,
		ToDate:         &to,
	}, fake.filter)

	_, err = b.ListTasks(context.Background(), task.TaskFilter{})
	require.NoError(t, err)
	assert.Equal(t, core.TaskFilter{}, fake.filter, "no statuses means no status filter")

	fake.err = errBoom
	_, err = b.ListTasks(context.Background(), task.TaskFilter{})
	assert.ErrorIs(t, err, errBoom)
}

func TestCoreBridgeAdapterApproveAndLog(t *testing.T) {
	fake := &fakeTaskService{obj: &core.Task{ID: "t1", Status: core.StatusExecuting}}
	b := newBridge(fake)

	got, err := b.ApproveTaskPlan(context.Background(), "t1", task.ApproveTaskPlanRequest{Approved: true, Feedback: "f"})
	require.NoError(t, err)
	assert.Equal(t, task.StatusExecuting, got.Status)
	assert.Equal(t, "t1", fake.taskID)
	assert.Equal(t, core.ApproveTaskPlanRequest{Approved: true, Feedback: "f"}, fake.req)

	require.NoError(t, b.AddTaskLog(context.Background(), "t1", "hello", "info"))
	assert.Equal(t, []string{"info:hello"}, fake.logCalls)
}

func TestCoreBridgeAdapterUpdateTask(t *testing.T) {
	fake := &fakeTaskService{obj: &core.Task{ID: "t1"}}
	_, err := newBridge(fake).UpdateTask(context.Background(), "t1", []task.TaskUpdate{
		{Type: "add_step", Description: "new step"},
		{Type: "update_status", Status: "awaiting_approval"},
		{Type: "modify_step", StepID: "s1", Status: "completed"},
		{Type: "add_comment", Description: "note"},
	})
	require.NoError(t, err)

	assert.Equal(t, []core.TaskUpdate{
		{Field: "add_step", Value: map[string]interface{}{"name": "Step", "description": "new step", "type": "task"}},
		{Field: "status", Value: "awaiting_approval"},
		{Field: "update_step", Value: map[string]interface{}{"id": "s1", "status": "completed"}},
		{Field: "add_comment", Value: "note"},
	}, fake.updates)
}

func TestCoreBridgeAdapterErrors(t *testing.T) {
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
			b := newBridge(tt.fake)
			_, err := b.CreateTask(ctx, task.CreateTaskRequest{})
			assert.EqualError(t, err, tt.want)
			_, err = b.GetTask(ctx, "t")
			assert.EqualError(t, err, tt.want)
			_, err = b.ApproveTaskPlan(ctx, "t", task.ApproveTaskPlanRequest{})
			assert.EqualError(t, err, tt.want)
			_, err = b.UpdateTask(ctx, "t", nil)
			assert.EqualError(t, err, tt.want)
		})
	}

	assert.Nil(t, (&CoreBridgeAdapter{}).coreTaskToTask(nil))
}
