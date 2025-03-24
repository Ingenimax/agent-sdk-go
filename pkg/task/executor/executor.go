package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
)

// TaskOptions contains options for task execution
type TaskOptions struct {
	Timeout      *time.Duration
	MaxRetries   *int
	RetryBackoff *time.Duration
	Metadata     map[string]interface{}
}

// TaskExecutor implements the interfaces.TaskExecutor interface
type TaskExecutor struct {
	// Add fields as needed for configuration
	taskRegistry map[string]TaskFunc
	// Add more fields as needed
}

// TaskFunc is a function that executes a task
type TaskFunc func(ctx context.Context, params interface{}) (interface{}, error)

// NewTaskExecutor creates a new task executor
func NewTaskExecutor() *TaskExecutor {
	return &TaskExecutor{
		taskRegistry: make(map[string]TaskFunc),
	}
}

// RegisterTask registers a task function with the executor
func (e *TaskExecutor) RegisterTask(name string, taskFunc TaskFunc) {
	e.taskRegistry[name] = taskFunc
}

// ExecuteStep executes a single step in a task plan
func (e *TaskExecutor) ExecuteStep(ctx context.Context, t *core.Task, step *core.Step) error {
	// Implementation for executing a single step in a task's plan
	taskFunc, exists := e.taskRegistry[step.Type] // Using type as the task name
	if !exists {
		return fmt.Errorf("task %s not registered", step.Type)
	}

	// Update step status
	step.Status = core.StatusExecuting
	step.CompletedAt = nil // Reset completion time

	// Execute the task
	result, err := taskFunc(ctx, step.Context)

	// Update step with results
	endTime := time.Now()
	if err != nil {
		step.Status = core.StatusFailed
		step.Error = err.Error()
		step.FailedAt = &endTime
		return err
	}

	step.CompletedAt = &endTime

	// Convert result to map if it's not already
	if result != nil {
		if resultMap, ok := result.(map[string]interface{}); ok {
			step.Output = resultMap
		} else {
			// Create a new map with the result
			resultMap := make(map[string]interface{})
			resultMap["result"] = result
			step.Output = resultMap
		}
	}

	step.Status = core.StatusCompleted
	return nil
}

// ExecuteTask executes all steps in a task's plan
func (e *TaskExecutor) ExecuteTask(ctx context.Context, t *core.Task) error {
	if t == nil || len(t.Steps) == 0 {
		return fmt.Errorf("task has no steps")
	}

	// Update task status
	t.Status = core.StatusExecuting

	// Execute steps in order
	for i := range t.Steps {
		step := t.Steps[i]
		if step.Status != core.StatusPending {
			continue // Skip steps that are not pending
		}

		if err := e.ExecuteStep(ctx, t, step); err != nil {
			t.Status = core.StatusFailed
			failTime := time.Now()
			t.FailedAt = &failTime
			return err
		}
	}

	// Check if all steps completed
	allCompleted := true
	for _, step := range t.Steps {
		if step.Status != core.StatusCompleted {
			allCompleted = false
			break
		}
	}

	if allCompleted {
		t.Status = core.StatusCompleted
		completedTime := time.Now()
		t.CompletedAt = &completedTime
	}

	return nil
}

// ExecuteSync executes a task synchronously
func (e *TaskExecutor) ExecuteSync(ctx context.Context, taskName string, params interface{}, opts *interfaces.TaskOptions) (*interfaces.TaskResult, error) {
	taskFunc, exists := e.taskRegistry[taskName]
	if !exists {
		return nil, fmt.Errorf("task %s not registered", taskName)
	}

	// Create local TaskOptions to handle the nil case
	localOpts := &TaskOptions{}

	// Apply timeout if specified
	if opts != nil && opts.Timeout != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *opts.Timeout)
		defer cancel()

		// Convert interfaces.TaskOptions to our local TaskOptions
		localOpts.Timeout = opts.Timeout
		localOpts.Metadata = opts.Metadata

		// Add retry information if available
		if opts.RetryPolicy != nil {
			maxRetries := opts.RetryPolicy.MaxRetries
			localOpts.MaxRetries = &maxRetries
			localOpts.RetryBackoff = &opts.RetryPolicy.InitialBackoff
		}
	}

	// Execute the task with retry if specified
	result, err := e.executeWithRetry(ctx, taskFunc, params, localOpts)

	taskResult := &interfaces.TaskResult{
		Data:     result,
		Error:    err,
		Metadata: make(map[string]interface{}),
	}

	// Add metadata
	if opts != nil && opts.Metadata != nil {
		for k, v := range opts.Metadata {
			taskResult.Metadata[k] = v
		}
	}

	taskResult.Metadata["executionTime"] = time.Now().UTC()

	return taskResult, nil
}

// ExecuteAsync executes a task asynchronously
func (e *TaskExecutor) ExecuteAsync(ctx context.Context, taskName string, params interface{}, opts *interfaces.TaskOptions) (<-chan *interfaces.TaskResult, error) {
	taskFunc, exists := e.taskRegistry[taskName]
	if !exists {
		return nil, fmt.Errorf("task %s not registered", taskName)
	}

	resultChan := make(chan *interfaces.TaskResult, 1)

	go func() {
		defer close(resultChan)

		// Create a new context for the async task
		asyncCtx := ctx

		// Create local TaskOptions to handle the nil case
		localOpts := &TaskOptions{}

		if opts != nil {
			// Apply timeout if specified
			if opts.Timeout != nil {
				var cancel context.CancelFunc
				asyncCtx, cancel = context.WithTimeout(ctx, *opts.Timeout)
				defer cancel()
			}

			// Convert interfaces.TaskOptions to our local TaskOptions
			localOpts.Timeout = opts.Timeout
			localOpts.Metadata = opts.Metadata

			// Add retry information if available
			if opts.RetryPolicy != nil {
				maxRetries := opts.RetryPolicy.MaxRetries
				localOpts.MaxRetries = &maxRetries
				localOpts.RetryBackoff = &opts.RetryPolicy.InitialBackoff
			}
		}

		// Execute the task with retry if specified
		result, err := e.executeWithRetry(asyncCtx, taskFunc, params, localOpts)

		taskResult := &interfaces.TaskResult{
			Data:     result,
			Error:    err,
			Metadata: make(map[string]interface{}),
		}

		// Add metadata
		if opts != nil && opts.Metadata != nil {
			for k, v := range opts.Metadata {
				taskResult.Metadata[k] = v
			}
		}

		taskResult.Metadata["executionTime"] = time.Now().UTC()
		resultChan <- taskResult
	}()

	return resultChan, nil
}

// executeWithRetry executes a task with retry logic
func (e *TaskExecutor) executeWithRetry(ctx context.Context, taskFunc TaskFunc, params interface{}, opts *TaskOptions) (interface{}, error) {
	var result interface{}
	var err error
	var retries int

	maxRetries := 0
	if opts != nil && opts.MaxRetries != nil {
		maxRetries = *opts.MaxRetries
	}

	for retries <= maxRetries {
		// Execute the task
		result, err = taskFunc(ctx, params)
		if err == nil {
			return result, nil
		}

		retries++
		if retries > maxRetries {
			break
		}

		// Wait before retrying if backoff is specified
		if opts != nil && opts.RetryBackoff != nil {
			select {
			case <-time.After(*opts.RetryBackoff):
				// Continue with retry
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return result, err
}
