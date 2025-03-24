# Task Execution Example

This example demonstrates how to use the task execution capabilities of the Agent SDK. It shows three different approaches to task execution:

1. Basic task execution (synchronous and asynchronous)
2. API task execution using the API client
3. Mock workflow task execution

## What You'll Learn

- How to create and register task functions
- How to execute tasks synchronously and asynchronously
- How to use the API client for HTTP requests
- How to handle task results and errors
- How to configure task options like timeouts and retries

## Running the Example

```bash
go run main.go
```

## Code Explanation

### Basic Task Execution

The example starts by creating a simple "hello" task and executing it both synchronously and asynchronously:

```go
// Register a simple task
taskExecutor.RegisterTask("hello", func(ctx context.Context, params interface{}) (interface{}, error) {
    name, ok := params.(string)
    if !ok {
        name = "World"
    }
    return fmt.Sprintf("Hello, %s!", name), nil
})

// Execute the task synchronously
result, err := taskExecutor.ExecuteSync(context.Background(), "hello", "John", nil)

// Execute the task asynchronously
resultChan, err := taskExecutor.ExecuteAsync(context.Background(), "hello", "Jane", nil)
```

### API Task Execution

The example then demonstrates how to use the API client to make HTTP requests:

```go
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
```

The API task also demonstrates how to use task options for timeout and retry policies:

```go
result, err = taskExecutor.ExecuteSync(context.Background(), "get_todos", nil, &interfaces.TaskOptions{
    Timeout:     &timeout,
    RetryPolicy: retryPolicy,
    Metadata: map[string]interface{}{
        "purpose": "example",
    },
})
```

### Mock Workflow Task

Finally, the example shows a mock workflow task that simulates processing input data:

```go
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
```

## Key Components

- **TaskExecutor**: Responsible for registering and executing tasks
- **API Client**: Handles HTTP requests to external services
- **TaskOptions**: Configures task execution with timeouts, retries, and metadata
- **TaskResult**: Contains the result of a task execution, including data, errors, and metadata

## Next Steps

- Try modifying the example to use different API endpoints
- Create your own task functions for specific use cases
- Experiment with different retry policies and timeout settings
- Implement real workflow tasks using a workflow engine 