package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/executor"
)

// TaskAPI provides a way to execute tasks over an API
type TaskAPI struct {
	client *Client
}

// NewTaskAPI creates a new task API with the given client
func NewTaskAPI(client *Client) *TaskAPI {
	return &TaskAPI{
		client: client,
	}
}

// Task returns a TaskFunc that executes a task via API
func (a *TaskAPI) Task(request Request) executor.TaskFunc {
	return func(ctx context.Context, params interface{}) (interface{}, error) {
		// If params are provided, update the request body
		if params != nil {
			request.Body = params
		}

		// Execute the request
		resp, err := a.client.Do(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("failed to execute API task: %w", err)
		}

		// Check response status
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("API task failed with status %d: %s", resp.StatusCode, string(resp.Body))
		}

		return resp.Body, nil
	}
}

// RegisterWithExecutor registers API tasks with an executor
func (a *TaskAPI) RegisterWithExecutor(exec *executor.TaskExecutor, taskName string, request Request) {
	exec.RegisterTask(taskName, a.Task(request))
}

// ExecuteTask executes a task via the API
func (a *TaskAPI) ExecuteTask(ctx context.Context, taskID string) error {
	// Construct the request to execute a task
	req := Request{
		Method: "POST",
		Path:   fmt.Sprintf("/tasks/%s/execute", taskID),
	}

	resp, err := a.client.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to execute task via API: %w", err)
	}

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("task execution failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}

	return nil
}

// GetTaskStatus gets the status of a task
func (a *TaskAPI) GetTaskStatus(ctx context.Context, taskID string) (core.Status, error) {
	// Construct the request to get a task
	req := Request{
		Method: "GET",
		Path:   fmt.Sprintf("/tasks/%s", taskID),
	}

	resp, err := a.client.Do(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to get task status via API: %w", err)
	}

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("get task status failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}

	// Parse the response to get the task status
	var taskResponse struct {
		Status core.Status `json:"status"`
	}
	if err := json.Unmarshal(resp.Body, &taskResponse); err != nil {
		return "", fmt.Errorf("failed to parse task status response: %w", err)
	}

	return taskResponse.Status, nil
}
