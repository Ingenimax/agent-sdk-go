package executor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBoom = errors.New("boom")

// flakyTask fails until it has been called failures+1 times.
func flakyTask(failures int32, calls *atomic.Int32) TaskFunc {
	return func(context.Context, interface{}) (interface{}, error) {
		if calls.Add(1) <= failures {
			return nil, errBoom
		}
		return "ok", nil
	}
}

func TestExecuteStep(t *testing.T) {
	tests := []struct {
		name       string
		fn         TaskFunc
		wantStatus core.Status
		wantOutput map[string]interface{}
		wantErr    bool
	}{
		{
			name:       "map result is stored as is",
			fn:         func(context.Context, interface{}) (interface{}, error) { return map[string]interface{}{"a": 1}, nil },
			wantStatus: core.StatusCompleted,
			wantOutput: map[string]interface{}{"a": 1},
		},
		{
			name:       "scalar result is wrapped",
			fn:         func(context.Context, interface{}) (interface{}, error) { return 42, nil },
			wantStatus: core.StatusCompleted,
			wantOutput: map[string]interface{}{"result": 42},
		},
		{
			name:       "nil result leaves output empty",
			fn:         func(context.Context, interface{}) (interface{}, error) { return nil, nil },
			wantStatus: core.StatusCompleted,
		},
		{
			name:       "error marks the step failed",
			fn:         func(context.Context, interface{}) (interface{}, error) { return nil, errBoom },
			wantStatus: core.StatusFailed,
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewTaskExecutor()
			var gotParams interface{}
			e.RegisterTask("work", func(ctx context.Context, p interface{}) (interface{}, error) {
				gotParams = p
				return tt.fn(ctx, p)
			})
			step := &core.Step{Type: "work", Status: core.StatusPending, Context: map[string]interface{}{"in": "x"}}

			err := e.ExecuteStep(context.Background(), &core.Task{}, step)

			assert.Equal(t, step.Context, gotParams)
			assert.Equal(t, tt.wantStatus, step.Status)
			assert.Equal(t, tt.wantOutput, step.Output)
			if tt.wantErr {
				assert.ErrorIs(t, err, errBoom)
				assert.Equal(t, "boom", step.Error)
				assert.NotNil(t, step.FailedAt)
				assert.Nil(t, step.CompletedAt)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, step.CompletedAt)
			assert.Nil(t, step.FailedAt)
		})
	}
}

func TestExecuteStepUnregistered(t *testing.T) {
	step := &core.Step{Type: "missing", Status: core.StatusPending}
	err := NewTaskExecutor().ExecuteStep(context.Background(), &core.Task{}, step)
	assert.EqualError(t, err, "task missing not registered")
	assert.Equal(t, core.StatusPending, step.Status)
}

func TestExecuteTask(t *testing.T) {
	ok := func(context.Context, interface{}) (interface{}, error) { return nil, nil }
	fail := func(context.Context, interface{}) (interface{}, error) { return nil, errBoom }

	t.Run("no steps", func(t *testing.T) {
		e := NewTaskExecutor()
		assert.EqualError(t, e.ExecuteTask(context.Background(), nil), "task has no steps")
		assert.EqualError(t, e.ExecuteTask(context.Background(), &core.Task{}), "task has no steps")
	})

	t.Run("all steps complete", func(t *testing.T) {
		e := NewTaskExecutor()
		e.RegisterTask("ok", ok)
		task := &core.Task{Steps: []*core.Step{
			{Type: "ok", Status: core.StatusPending},
			{Type: "ok", Status: core.StatusCompleted},
			{Type: "ok", Status: core.StatusPending},
		}}
		require.NoError(t, e.ExecuteTask(context.Background(), task))
		assert.Equal(t, core.StatusCompleted, task.Status)
		assert.NotNil(t, task.CompletedAt)
	})

	t.Run("non pending steps are skipped", func(t *testing.T) {
		e := NewTaskExecutor()
		e.RegisterTask("ok", ok)
		task := &core.Task{Steps: []*core.Step{
			{Type: "ok", Status: core.StatusPending},
			{Type: "unregistered", Status: core.StatusFailed},
		}}
		require.NoError(t, e.ExecuteTask(context.Background(), task))
		assert.Equal(t, core.StatusExecuting, task.Status, "a failed step keeps the task from completing")
		assert.Nil(t, task.CompletedAt)
	})

	t.Run("a failing step stops execution", func(t *testing.T) {
		e := NewTaskExecutor()
		e.RegisterTask("ok", ok)
		e.RegisterTask("fail", fail)
		task := &core.Task{Steps: []*core.Step{
			{Type: "fail", Status: core.StatusPending},
			{Type: "ok", Status: core.StatusPending},
		}}
		assert.ErrorIs(t, e.ExecuteTask(context.Background(), task), errBoom)
		assert.Equal(t, core.StatusFailed, task.Status)
		assert.NotNil(t, task.FailedAt)
		assert.Equal(t, core.StatusPending, task.Steps[1].Status)
	})
}

func TestExecuteSync(t *testing.T) {
	ctx := context.Background()

	t.Run("unregistered", func(t *testing.T) {
		_, err := NewTaskExecutor().ExecuteSync(ctx, "missing", nil, nil)
		assert.EqualError(t, err, "task missing not registered")
	})

	t.Run("nil options", func(t *testing.T) {
		e := NewTaskExecutor()
		e.RegisterTask("echo", func(_ context.Context, p interface{}) (interface{}, error) { return p, nil })
		res, err := e.ExecuteSync(ctx, "echo", "hi", nil)
		require.NoError(t, err)
		assert.Equal(t, "hi", res.Data)
		assert.NoError(t, res.Error)
		assert.Contains(t, res.Metadata, "executionTime")
	})

	t.Run("task error is returned in the result", func(t *testing.T) {
		e := NewTaskExecutor()
		var calls atomic.Int32
		e.RegisterTask("flaky", flakyTask(5, &calls))
		res, err := e.ExecuteSync(ctx, "flaky", nil, nil)
		require.NoError(t, err)
		assert.ErrorIs(t, res.Error, errBoom)
		assert.Equal(t, int32(1), calls.Load(), "no retries without a retry policy")
	})

	t.Run("retries and metadata", func(t *testing.T) {
		e := NewTaskExecutor()
		var calls atomic.Int32
		e.RegisterTask("flaky", flakyTask(2, &calls))
		timeout := time.Second
		res, err := e.ExecuteSync(ctx, "flaky", nil, &interfaces.TaskOptions{
			Timeout:     &timeout,
			RetryPolicy: &interfaces.RetryPolicy{MaxRetries: 3, InitialBackoff: time.Millisecond},
			Metadata:    map[string]interface{}{"k": "v"},
		})
		require.NoError(t, err)
		assert.NoError(t, res.Error)
		assert.Equal(t, "ok", res.Data)
		assert.Equal(t, int32(3), calls.Load())
		assert.Equal(t, "v", res.Metadata["k"])
		assert.Contains(t, res.Metadata, "executionTime")
	})

	t.Run("retries exhausted", func(t *testing.T) {
		e := NewTaskExecutor()
		var calls atomic.Int32
		e.RegisterTask("flaky", flakyTask(10, &calls))
		timeout := time.Second
		res, err := e.ExecuteSync(ctx, "flaky", nil, &interfaces.TaskOptions{
			Timeout:     &timeout,
			RetryPolicy: &interfaces.RetryPolicy{MaxRetries: 2},
		})
		require.NoError(t, err)
		assert.ErrorIs(t, res.Error, errBoom)
		assert.Equal(t, int32(3), calls.Load())
	})

	t.Run("timeout cancels the backoff wait", func(t *testing.T) {
		e := NewTaskExecutor()
		var calls atomic.Int32
		e.RegisterTask("flaky", flakyTask(10, &calls))
		timeout := 20 * time.Millisecond
		res, err := e.ExecuteSync(ctx, "flaky", nil, &interfaces.TaskOptions{
			Timeout:     &timeout,
			RetryPolicy: &interfaces.RetryPolicy{MaxRetries: 5, InitialBackoff: time.Minute},
		})
		require.NoError(t, err)
		assert.ErrorIs(t, res.Error, context.DeadlineExceeded)
		assert.Equal(t, int32(1), calls.Load())
	})
}

func TestExecuteAsync(t *testing.T) {
	ctx := context.Background()

	t.Run("unregistered", func(t *testing.T) {
		ch, err := NewTaskExecutor().ExecuteAsync(ctx, "missing", nil, nil)
		assert.Nil(t, ch)
		assert.EqualError(t, err, "task missing not registered")
	})

	t.Run("nil options", func(t *testing.T) {
		e := NewTaskExecutor()
		e.RegisterTask("echo", func(_ context.Context, p interface{}) (interface{}, error) { return p, nil })
		ch, err := e.ExecuteAsync(ctx, "echo", 7, nil)
		require.NoError(t, err)
		res := <-ch
		require.NotNil(t, res)
		assert.Equal(t, 7, res.Data)
		assert.NoError(t, res.Error)
		_, open := <-ch
		assert.False(t, open, "channel is closed after the result")
	})

	t.Run("retries and metadata", func(t *testing.T) {
		e := NewTaskExecutor()
		var calls atomic.Int32
		e.RegisterTask("flaky", flakyTask(1, &calls))
		timeout := time.Second
		ch, err := e.ExecuteAsync(ctx, "flaky", nil, &interfaces.TaskOptions{
			Timeout:     &timeout,
			RetryPolicy: &interfaces.RetryPolicy{MaxRetries: 1, InitialBackoff: time.Millisecond},
			Metadata:    map[string]interface{}{"k": "v"},
		})
		require.NoError(t, err)
		res := <-ch
		assert.NoError(t, res.Error)
		assert.Equal(t, "ok", res.Data)
		assert.Equal(t, int32(2), calls.Load())
		assert.Equal(t, "v", res.Metadata["k"])
		assert.Contains(t, res.Metadata, "executionTime")
	})

	t.Run("options without timeout still retry", func(t *testing.T) {
		e := NewTaskExecutor()
		var calls atomic.Int32
		e.RegisterTask("flaky", flakyTask(1, &calls))
		ch, err := e.ExecuteAsync(ctx, "flaky", nil, &interfaces.TaskOptions{
			RetryPolicy: &interfaces.RetryPolicy{MaxRetries: 1},
		})
		require.NoError(t, err)
		res := <-ch
		assert.NoError(t, res.Error)
		assert.Equal(t, int32(2), calls.Load())
	})
}
