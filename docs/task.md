# Task Execution Package

The task execution package provides functionality for executing tasks synchronously and asynchronously, including API calls and Temporal workflows.

## Features

- Execute tasks synchronously and asynchronously
- Built-in retry mechanism with configurable retry policies
- API client for making HTTP requests
- Temporal workflow integration
- Task cancellation and status tracking

## Usage

### Basic Task Execution

```go
// Create a task executor
executor := agentsdk.NewTaskExecutor()

// Register a task
executor.RegisterTask("hello", func(ctx context.Context, params interface{}) (interface{}, error) {
    name, ok := params.(string)
    if !ok {
        name = "World"
    }
    return fmt.Sprintf("Hello, %s!", name), nil
})

// Execute the task synchronously
result, err := executor.ExecuteSync(context.Background(), "hello", "John", nil)
if err != nil {
    fmt.Printf("Error: %v\n", err)
} else {
    fmt.Printf("Result: %v\n", result.Data)
}

// Execute the task asynchronously
resultChan, err := executor.ExecuteAsync(context.Background(), "hello", "Jane", nil)
if err != nil {
    fmt.Printf("Error: %v\n", err)
} else {
    result := <-resultChan
    fmt.Printf("Result: %v\n", result.Data)
}
```

### API Task Execution

```go
// Create an API client
apiClient := agentsdk.NewAPIClient("https://api.example.com", 10*time.Second)

// Register an API task
executor.RegisterTask("get_data", agentsdk.APITask(apiClient, task.APIRequest{
    Method: "GET",
    Path:   "/data",
    Query:  map[string]string{"limit": "10"},
}))

// Execute the API task with retry policy
timeout := 5 * time.Second
retryPolicy := &interfaces.RetryPolicy{
    MaxRetries:        3,
    InitialBackoff:    100 * time.Millisecond,
    MaxBackoff:        1 * time.Second,
    BackoffMultiplier: 2.0,
}

result, err := executor.ExecuteSync(context.Background(), "get_data", nil, &interfaces.TaskOptions{
    Timeout:     &timeout,
    RetryPolicy: retryPolicy,
})
```

### Temporal Workflow Execution

```go
// Create a Temporal client
temporalClient := agentsdk.NewTemporalClient(task.TemporalConfig{
    HostPort:                 "localhost:7233",
    Namespace:                "default",
    TaskQueue:                "example",
    WorkflowIDPrefix:         "example-",
    WorkflowExecutionTimeout: 10 * time.Minute,
    WorkflowRunTimeout:       5 * time.Minute,
    WorkflowTaskTimeout:      10 * time.Second,
})

// Register a Temporal workflow task
executor.RegisterTask("example_workflow", agentsdk.TemporalWorkflowTask(temporalClient, "ExampleWorkflow"))

// Execute the Temporal workflow task
result, err := executor.ExecuteSync(context.Background(), "example_workflow", map[string]interface{}{
    "input": "example input",
}, nil)
```

## Task Options

You can configure task execution with the following options:

```go
options := &interfaces.TaskOptions{
    // Timeout specifies the maximum duration for task execution
    Timeout: &timeout,

    // RetryPolicy specifies the retry policy for the task
    RetryPolicy: &interfaces.RetryPolicy{
        MaxRetries:        3,
        InitialBackoff:    100 * time.Millisecond,
        MaxBackoff:        1 * time.Second,
        BackoffMultiplier: 2.0,
    },

    // Metadata contains additional information for the task execution
    Metadata: map[string]interface{}{
        "purpose": "example",
    },
}
```

## Task Result

The task result contains the following information:

```go
type TaskResult struct {
    // Data contains the result data
    Data interface{}

    // Error contains any error that occurred during task execution
    Error error

    // Metadata contains additional information about the task execution
    Metadata map[string]interface{}
}
```
