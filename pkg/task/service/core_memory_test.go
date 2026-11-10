package service

import (
	"context"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCoreService() *CoreMemoryService {
	return NewCoreMemoryService(logging.New(), nil).(*CoreMemoryService)
}

func mustCreate(t *testing.T, s *CoreMemoryService, req interface{}) *core.Task {
	t.Helper()
	obj, err := s.CreateTask(context.Background(), req)
	require.NoError(t, err)
	return obj.(*core.Task)
}

func TestCoreMemoryCreateTask(t *testing.T) {
	ctx := context.Background()
	s := newCoreService()

	typed := mustCreate(t, s, core.CreateTaskRequest{
		Name: "n", Description: "d", UserID: "u", ConversationID: "c",
		Input: map[string]interface{}{"i": 1}, Metadata: map[string]interface{}{"m": 2},
	})
	assert.NotEmpty(t, typed.ID)
	assert.Equal(t, core.StatusPending, typed.Status)
	assert.Equal(t, "n", typed.Name)
	assert.Equal(t, "c", typed.ConversationID)
	assert.Equal(t, map[string]interface{}{"i": 1}, typed.Input)
	assert.Equal(t, map[string]interface{}{"m": 2}, typed.Metadata)
	assert.Empty(t, typed.Steps)

	fromMap := mustCreate(t, s, map[string]interface{}{
		"name": "n2", "description": "d2", "user_id": "u2",
		"conversation_id": "c2",
		"metadata":        map[string]interface{}{"m": 3},
		"input":           map[string]interface{}{"i": 4},
	})
	assert.Equal(t, "n2", fromMap.Name)
	assert.Equal(t, "d2", fromMap.Description)
	assert.Equal(t, "u2", fromMap.UserID)
	assert.Equal(t, "c2", fromMap.ConversationID)
	assert.Equal(t, map[string]interface{}{"m": 3}, fromMap.Metadata)
	assert.Equal(t, map[string]interface{}{"i": 4}, fromMap.Input)

	minimal := mustCreate(t, s, map[string]interface{}{"name": "n3", "description": "", "user_id": ""})
	assert.Empty(t, minimal.ConversationID)
	assert.Nil(t, minimal.Metadata)

	_, err := s.CreateTask(ctx, 42)
	assert.EqualError(t, err, "invalid request type")

	got, err := s.GetTask(ctx, typed.ID)
	require.NoError(t, err)
	assert.Same(t, typed, got)
	_, err = s.GetTask(ctx, "missing")
	assert.EqualError(t, err, "task not found: missing")

	logs, err := s.GetTaskLogs(ctx, typed.ID)
	require.NoError(t, err)
	assert.Empty(t, logs)
}

func TestCoreMemoryListTasks(t *testing.T) {
	ctx := context.Background()
	s := newCoreService()
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	add := func(id, user, conv string, status core.Status, offset time.Duration) {
		s.tasks[id] = &core.Task{ID: id, UserID: user, ConversationID: conv, Status: status, CreatedAt: base.Add(offset)}
	}
	add("a", "u1", "c1", core.StatusPending, 0)
	add("b", "u1", "c2", core.StatusCompleted, time.Hour)
	add("c", "u2", "c1", core.StatusFailed, 2*time.Hour)

	from := base.Add(30 * time.Minute)
	to := base.Add(90 * time.Minute)

	tests := []struct {
		name   string
		filter interface{}
		want   []string
	}{
		{name: "nil filter", filter: nil, want: []string{"a", "b", "c"}},
		{name: "user", filter: core.TaskFilter{UserID: "u1"}, want: []string{"a", "b"}},
		{name: "conversation", filter: core.TaskFilter{ConversationID: "c1"}, want: []string{"a", "c"}},
		{name: "status", filter: core.TaskFilter{Status: core.StatusFailed}, want: []string{"c"}},
		{name: "from date", filter: core.TaskFilter{FromDate: &from}, want: []string{"b", "c"}},
		{name: "to date", filter: core.TaskFilter{ToDate: &to}, want: []string{"a", "b"}},
		{name: "map filter", filter: map[string]interface{}{"user_id": "u1", "conversation_id": "c2", "status": "completed"}, want: []string{"b"}},
		{name: "limit larger than results", filter: core.TaskFilter{Limit: 5}, want: []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ListTasks(ctx, tt.filter)
			require.NoError(t, err)
			ids := make([]string, len(got))
			for i, obj := range got {
				ids[i] = obj.(*core.Task).ID
			}
			assert.ElementsMatch(t, tt.want, ids)
		})
	}

	// Map iteration order is random, so only the page sizes are stable.
	pages := []struct {
		filter core.TaskFilter
		want   int
	}{
		{core.TaskFilter{Limit: 2}, 2},
		{core.TaskFilter{Limit: 2, Offset: 2}, 1},
		{core.TaskFilter{Limit: 1, Offset: 1}, 1},
		{core.TaskFilter{Limit: 2, Offset: 5}, 0},
	}
	for _, p := range pages {
		got, err := s.ListTasks(ctx, p.filter)
		require.NoError(t, err)
		assert.Len(t, got, p.want, "limit %d offset %d", p.filter.Limit, p.filter.Offset)
	}
}

func TestCoreMemoryApproveTaskPlan(t *testing.T) {
	ctx := context.Background()

	_, err := newCoreService().ApproveTaskPlan(ctx, "missing", core.ApproveTaskPlanRequest{})
	assert.EqualError(t, err, "task not found: missing")

	tests := []struct {
		name    string
		status  core.Status
		req     interface{}
		want    core.Status
		wantErr string
	}{
		{name: "approve typed", status: core.StatusAwaitingApproval, req: core.ApproveTaskPlanRequest{Approved: true}, want: core.StatusExecuting},
		{name: "reject typed", status: core.StatusAwaitingApproval, req: core.ApproveTaskPlanRequest{Approved: false}, want: core.StatusPlanning},
		{name: "approve from planning via map", status: core.StatusPlanning, req: map[string]interface{}{"approved": true, "feedback": "ok"}, want: core.StatusExecuting},
		{name: "empty map rejects", status: core.StatusPlanning, req: map[string]interface{}{}, want: core.StatusPlanning},
		{name: "invalid request", status: core.StatusPlanning, req: "bad", wantErr: "invalid request type"},
		{name: "wrong state", status: core.StatusCompleted, req: core.ApproveTaskPlanRequest{Approved: true}, wantErr: "task is not in a state that can be approved: completed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newCoreService()
			tk := mustCreate(t, s, core.CreateTaskRequest{Name: "n"})
			tk.Status = tt.status

			obj, err := s.ApproveTaskPlan(ctx, tk.ID, tt.req)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				assert.Equal(t, tt.status, tk.Status)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, obj.(*core.Task).Status)
		})
	}
}

func TestCoreMemoryUpdateTaskStatus(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		status        string
		wantCompleted bool
	}{
		{"executing", false},
		{"completed", true},
		{"failed", true},
		{"planning", false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			s := newCoreService()
			tk := mustCreate(t, s, core.CreateTaskRequest{})
			_, err := s.UpdateTask(ctx, tk.ID, []core.TaskUpdate{{Field: "status", Value: tt.status}})
			require.NoError(t, err)
			assert.Equal(t, core.Status(tt.status), tk.Status)
			assert.Equal(t, tt.wantCompleted, tk.CompletedAt != nil)
		})
	}
}

func TestCoreMemoryUpdateTaskSteps(t *testing.T) {
	ctx := context.Background()
	s := newCoreService()
	tk := mustCreate(t, s, core.CreateTaskRequest{})

	_, err := s.UpdateTask(ctx, tk.ID, []core.TaskUpdate{
		{Field: "add_step", Value: map[string]interface{}{"name": "a", "description": "da", "type": "chat"}},
		{Field: "add_step", Value: map[string]interface{}{}},
		{Field: "add_step", Value: map[string]interface{}{}},
		{Field: "add_step", Value: "ignored"},
		{Field: "plan", Value: "the plan"},
		{Field: "plan", Value: 1},
		{Field: "status", Value: 1},
	})
	require.NoError(t, err)
	require.Len(t, tk.Steps, 3)
	assert.Equal(t, "a", tk.Steps[0].Name)
	assert.Equal(t, "da", tk.Steps[0].Description)
	assert.Equal(t, "chat", tk.Steps[0].Type)
	assert.Equal(t, 0, tk.Steps[0].OrderIndex)
	assert.Equal(t, 2, tk.Steps[2].OrderIndex)
	assert.Equal(t, core.StatusPending, tk.Steps[1].Status)
	assert.Equal(t, "the plan", tk.Plan)
	assert.Equal(t, core.StatusPending, tk.Status)

	s0, s1, s2 := tk.Steps[0], tk.Steps[1], tk.Steps[2]
	output := map[string]interface{}{"r": "x"}
	_, err = s.UpdateTask(ctx, tk.ID, []core.TaskUpdate{
		{Field: "update_step", Value: map[string]interface{}{"id": s0.ID, "status": "completed", "output": output}},
		{Field: "update_step", Value: map[string]interface{}{"id": s1.ID, "status": "failed", "error": "bad"}},
		{Field: "update_step", Value: map[string]interface{}{"id": s2.ID, "status": "executing"}},
		{Field: "update_step", Value: map[string]interface{}{"status": "failed"}},
		{Field: "update_step", Value: map[string]interface{}{"id": "unknown", "status": "failed"}},
		{Field: "update_step", Value: "ignored"},
	})
	require.NoError(t, err)

	assert.Equal(t, core.StatusCompleted, s0.Status)
	assert.NotNil(t, s0.CompletedAt)
	assert.Equal(t, output, s0.Output)

	assert.Equal(t, core.StatusFailed, s1.Status)
	assert.NotNil(t, s1.FailedAt)
	assert.Equal(t, "bad", s1.Error)

	assert.Equal(t, core.StatusExecuting, s2.Status)
	assert.Nil(t, s2.CompletedAt)
	assert.Nil(t, s2.FailedAt)
}

func TestCoreMemoryUpdateTaskInputs(t *testing.T) {
	ctx := context.Background()
	s := newCoreService()
	tk := mustCreate(t, s, core.CreateTaskRequest{})

	_, err := s.UpdateTask(ctx, "missing", []core.TaskUpdate{})
	assert.EqualError(t, err, "task not found: missing")

	_, err = s.UpdateTask(ctx, tk.ID, "bad")
	assert.EqualError(t, err, "invalid updates type")

	_, err = s.UpdateTask(ctx, tk.ID, []interface{}{
		map[string]interface{}{"field": "plan", "value": "from map"},
		map[string]interface{}{"field": "status"},
		"not a map",
	})
	require.NoError(t, err)
	assert.Equal(t, "from map", tk.Plan)
	assert.Equal(t, core.StatusPending, tk.Status)
}

func TestCoreMemoryLogs(t *testing.T) {
	ctx := context.Background()
	s := newCoreService()
	tk := mustCreate(t, s, core.CreateTaskRequest{})

	assert.EqualError(t, s.AddTaskLog(ctx, "missing", "m", "info"), "task not found: missing")
	_, err := s.GetTaskLogs(ctx, "missing")
	assert.EqualError(t, err, "task not found: missing")

	require.NoError(t, s.AddTaskLog(ctx, tk.ID, "first", "info"))
	require.NoError(t, s.AddTaskLog(ctx, tk.ID, "second", "error"))

	logs, err := s.GetTaskLogs(ctx, tk.ID)
	require.NoError(t, err)
	require.Len(t, logs, 2)
	assert.Equal(t, "first", logs[0].Message)
	assert.Equal(t, "info", logs[0].Level)
	assert.Equal(t, "second", logs[1].Message)
	assert.Equal(t, "error", logs[1].Level)
	assert.Equal(t, tk.ID, logs[1].TaskID)
	assert.NotEqual(t, logs[0].ID, logs[1].ID)
}
