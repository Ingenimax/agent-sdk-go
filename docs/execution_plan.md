# Execution Plan Package

The `executionplan` package provides a structured way to create, modify, and execute plans for complex tasks. This package enables transparency and control over actions that an AI agent might take by presenting a plan to the user for approval before execution.

## Overview

The execution plan system consists of several key components:

1. **ExecutionPlan**: A struct representing a plan with steps to execute
2. **Generator**: Creates and modifies execution plans
3. **Executor**: Executes approved plans
4. **Store**: Manages storage and retrieval of plans

This architecture allows for a clear separation of concerns and greater flexibility in how plans are created, stored, and executed.

## Key Concepts

### ExecutionPlan

An `ExecutionPlan` represents a set of steps that need to be executed to accomplish a task. Each plan has:

- A list of execution steps
- A high-level description
- A unique task ID
- A status (draft, pending approval, approved, executing, completed, failed, cancelled)
- Timestamps for creation and updates
- A flag indicating whether the user has approved the plan

### ExecutionStep

Each `ExecutionStep` represents a single action within a plan. Steps contain:

- The name of the tool to execute
- Input for the tool
- A description of what the step will accomplish
- Parameters for the tool execution

### Plan Status

An execution plan can be in one of the following statuses:

- `StatusDraft`: The plan is in a draft state and not yet ready for approval
- `StatusPendingApproval`: The plan is waiting for user approval
- `StatusApproved`: The plan has been approved by the user
- `StatusExecuting`: The plan is currently being executed
- `StatusCompleted`: The plan has been successfully executed
- `StatusFailed`: The plan execution failed
- `StatusCancelled`: The plan was cancelled by the user

## Using the Package

### Creating a Generator

The `Generator` is responsible for creating and modifying execution plans:

```go
// Create a generator
generator := executionplan.NewGenerator(
    llmClient,  // An LLM implementation
    tools,      // List of available tools
    systemPrompt, // Optional system prompt for the LLM
)

// Generate an execution plan
plan, err := generator.GenerateExecutionPlan(ctx, userInput)
if err != nil {
    // Handle error
}

// Modify an execution plan based on user feedback
modifiedPlan, err := generator.ModifyExecutionPlan(ctx, plan, userFeedback)
if err != nil {
    // Handle error
}
```

### Creating an Executor

The `Executor` is responsible for executing approved plans:

```go
// Create an executor
executor := executionplan.NewExecutor(tools)

// Execute an approved plan
result, err := executor.ExecutePlan(ctx, approvedPlan)
if err != nil {
    // Handle error
}

// Cancel a plan
executor.CancelPlan(plan)

// Get plan status
status := executor.GetPlanStatus(plan)
```

### Using the Store

The `Store` is responsible for storing and retrieving plans:

```go
// Create a store
store := executionplan.NewStore()

// Store a plan
store.StorePlan(plan)

// Get a plan by task ID
plan, exists := store.GetPlanByTaskID(taskID)

// List all plans
plans := store.ListPlans()

// Delete a plan
deleted := store.DeletePlan(taskID)
```

### Formatting Plans for Display

The package provides a function to format plans for display:

```go
// Format a plan for display
formattedPlan := executionplan.FormatExecutionPlan(plan)
fmt.Println(formattedPlan)
```

## Integration with Agents

The most common use case is to integrate the execution plan package with the Agent SDK:

```go
// Create an agent with execution plan support
agent, err := agent.NewAgent(
    agent.WithLLM(llmClient),
    agent.WithTools(tools...),
    agent.WithRequirePlanApproval(true), // Enable execution plan workflow
)

// Generate a plan
plan, err := agent.GenerateExecutionPlan(ctx, userInput)

// Modify a plan
modifiedPlan, err := agent.ModifyExecutionPlan(ctx, plan, userFeedback)

// Approve and execute a plan
result, err := agent.ApproveExecutionPlan(ctx, plan)
```

## Advanced Customization

### Custom Plan Generation

You can implement custom plan generation by creating your own `Generator` implementation:

```go
type CustomGenerator struct {
    // Your fields here
}

func (g *CustomGenerator) GenerateExecutionPlan(ctx context.Context, input string) (*executionplan.ExecutionPlan, error) {
    // Your custom implementation
}
```

### Custom Plan Execution

Similarly, you can implement custom plan execution:

```go
type CustomExecutor struct {
    // Your fields here
}

func (e *CustomExecutor) ExecutePlan(ctx context.Context, plan *executionplan.ExecutionPlan) (string, error) {
    // Your custom implementation
}
```

## Best Practices

1. **Clear Step Descriptions**: Ensure each step has a clear, human-readable description
2. **Appropriate Granularity**: Break complex tasks into appropriate steps - not too many, not too few
3. **Parameter Validation**: Validate parameters before executing steps
4. **Error Handling**: Implement proper error handling during execution
5. **User Feedback**: Provide clear feedback to users during the approval process
6. **Security**: Consider security implications of each step before execution

## Example

Here's a complete example of using the execution plan package:

```go
package main

import (
    "context"
    "fmt"

    "github.com/Ingenimax/agent-sdk-go/pkg/executionplan"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
    // Create an LLM client
    llmClient := openai.NewClient("your-api-key")

    // Create some tools
    tools := []interfaces.Tool{
        // Your tools here
    }

    // Create a generator
    generator := executionplan.NewGenerator(llmClient, tools, "You are a helpful assistant")

    // Generate a plan
    plan, err := generator.GenerateExecutionPlan(context.Background(), "Deploy a web application")
    if err != nil {
        panic(err)
    }

    // Format the plan for display
    formattedPlan := executionplan.FormatExecutionPlan(plan)
    fmt.Println(formattedPlan)

    // Create an executor
    executor := executionplan.NewExecutor(tools)

    // Execute the plan (after user approval)
    plan.UserApproved = true
    result, err := executor.ExecutePlan(context.Background(), plan)
    if err != nil {
        panic(err)
    }

    fmt.Println(result)
}
```
