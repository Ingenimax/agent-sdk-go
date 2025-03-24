package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	agentsdk "github.com/Ingenimax/agent-sdk-go/pkg"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/api"
)

func main() {
	// Create a logger
	logger := logging.New()

	// Create a task executor
	taskExecutor := agentsdk.NewTaskExecutor()

	// Register a simple task
	taskExecutor.RegisterTask("hello", func(ctx context.Context, params interface{}) (interface{}, error) {
		name, ok := params.(string)
		if !ok {
			name = "World"
		}
		return fmt.Sprintf("Hello, %s!", name), nil
	})

	// Execute the task synchronously
	logger.Info(context.Background(), "Executing task synchronously", nil)
	result, err := taskExecutor.ExecuteSync(context.Background(), "hello", "John", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Result: %v\n", result.Data)
	}

	// Execute the task asynchronously
	logger.Info(context.Background(), "Executing task asynchronously", nil)
	resultChan, err := taskExecutor.ExecuteAsync(context.Background(), "hello", "Jane", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		result := <-resultChan
		fmt.Printf("Result: %v\n", result.Data)
	}

	// Create an API client
	apiClient := agentsdk.NewAPIClient("https://jsonplaceholder.typicode.com", 10*time.Second)

	// Register an API task
	taskExecutor.RegisterTask("get_todos", func(ctx context.Context, params interface{}) (interface{}, error) {
		// Create API request
		apiRequest := api.Request{
			Method: "GET",
			Path:   "/todos/1",
		}

		// Execute the request
		response, err := apiClient.Do(ctx, apiRequest)
		return response, err
	})

	// Execute the API task
	logger.Info(context.Background(), "Executing API task", nil)
	timeout := 5 * time.Second
	retryPolicy := &interfaces.RetryPolicy{
		MaxRetries:        3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        1 * time.Second,
		BackoffMultiplier: 2.0,
	}

	result, err = taskExecutor.ExecuteSync(context.Background(), "get_todos", nil, &interfaces.TaskOptions{
		Timeout:     &timeout,
		RetryPolicy: retryPolicy,
		Metadata: map[string]interface{}{
			"purpose": "example",
		},
	})

	if err != nil {
		logger.Error(context.Background(), "Error executing task", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		// Check if the result is a byte array and try to decode it as JSON
		if byteData, ok := result.Data.([]byte); ok {
			var todoData map[string]interface{}
			if err := json.Unmarshal(byteData, &todoData); err != nil {
				logger.Error(context.Background(), "Failed to unmarshal JSON response", map[string]interface{}{
					"error": err.Error(),
				})
				logger.Info(context.Background(), "Result (raw bytes)", map[string]interface{}{
					"data": string(byteData),
				})
			} else {
				logger.Info(context.Background(), "Result decoded", map[string]interface{}{
					"data": todoData,
				})
			}
		} else {
			logger.Info(context.Background(), "Result", map[string]interface{}{
				"data": fmt.Sprintf("%v", result.Data),
			})
		}
		logger.Info(context.Background(), "Metadata", map[string]interface{}{
			"metadata": result.Metadata,
		})
	}

	// Note: Temporal client functionality has been temporarily removed
	// For demonstration purposes, we'll create a mock workflow task
	logger.Info(context.Background(), "Temporal client functionality has been removed in restructuring", nil)
	logger.Info(context.Background(), "Creating a mock workflow task instead", nil)

	// Register a mock workflow task
	taskExecutor.RegisterTask("mock_workflow", func(ctx context.Context, params interface{}) (interface{}, error) {
		// Simulate a workflow execution
		time.Sleep(500 * time.Millisecond)

		input, ok := params.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected map[string]interface{} input, got %T", params)
		}

		return map[string]interface{}{
			"result": fmt.Sprintf("Processed: %v", input["input"]),
			"status": "completed",
		}, nil
	})

	// Execute the mock workflow task
	logger.Info(context.Background(), "Executing mock workflow task", nil)
	result, err = taskExecutor.ExecuteSync(context.Background(), "mock_workflow", map[string]interface{}{
		"input": "example input",
	}, nil)

	if err != nil {
		logger.Error(context.Background(), "Error executing task", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		logger.Info(context.Background(), "Result", map[string]interface{}{
			"data": fmt.Sprintf("%v", result.Data),
		})
		logger.Info(context.Background(), "Error", map[string]interface{}{
			"error": fmt.Sprintf("%v", result.Error),
		})
		logger.Info(context.Background(), "Metadata", map[string]interface{}{
			"metadata": result.Metadata,
		})
	}
}
