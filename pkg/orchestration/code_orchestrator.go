package orchestration

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// TaskStatus represents the status of a task
type TaskStatus string

const (
	// TaskPending indicates the task is pending
	TaskPending TaskStatus = "pending"

	// TaskRunning indicates the task is running
	TaskRunning TaskStatus = "running"

	// TaskCompleted indicates the task is completed
	TaskCompleted TaskStatus = "completed"

	// TaskFailed indicates the task failed
	TaskFailed TaskStatus = "failed"
)

// Task represents a task to be executed by an agent
type Task struct {
	// ID is the unique identifier for the task
	ID string

	// AgentID is the ID of the agent to execute the task
	AgentID string

	// Input is the input to provide to the agent
	Input string

	// Dependencies are the IDs of tasks that must complete before this one
	Dependencies []string

	// Status is the current status of the task
	Status TaskStatus

	// Result is the result of the task
	Result string

	// Error is any error that occurred during execution
	Error error
}

// Workflow represents a workflow of tasks
type Workflow struct {
	// Tasks is the list of tasks in the workflow
	Tasks []*Task

	// Results is a map of task IDs to results
	Results map[string]string

	// Errors is a map of task IDs to errors
	Errors map[string]error

	// FinalTaskID is the ID of the task that produces the final result
	FinalTaskID string
}

// NewWorkflow creates a new workflow
func NewWorkflow() *Workflow {
	return &Workflow{
		Tasks:   make([]*Task, 0),
		Results: make(map[string]string),
		Errors:  make(map[string]error),
	}
}

// AddTask adds a task to the workflow
func (w *Workflow) AddTask(id string, agentID string, input string, dependencies []string) {
	task := &Task{
		ID:           id,
		AgentID:      agentID,
		Input:        input,
		Dependencies: dependencies,
		Status:       TaskPending,
	}

	w.Tasks = append(w.Tasks, task)
}

// SetFinalTask sets the final task
func (w *Workflow) SetFinalTask(id string) {
	w.FinalTaskID = id
}

// CodeOrchestrator orchestrates agents using code-defined workflows
type CodeOrchestrator struct {
	registry *AgentRegistry
}

// NewCodeOrchestrator creates a new code orchestrator
func NewCodeOrchestrator(registry *AgentRegistry) *CodeOrchestrator {
	return &CodeOrchestrator{
		registry: registry,
	}
}

// ExecuteWorkflow executes a workflow, running each task as soon as its
// dependencies have completed.
//
// Scheduling is dependency-driven: every task gets one goroutine that waits on
// its dependencies' completion channels. There is no central monitor.
//
// The previous design had a monitor goroutine that received on an unbuffered
// completion channel and re-scanned for runnable tasks. It had four defects:
//
//   - Deadlock. executeTask ended with an unbuffered `completionCh <- task.ID`
//     and no select on ctx.Done(). Once the monitor saw every task finished it
//     returned, so a second task finishing at the same moment blocked forever on
//     that send, its deferred wg.Done() never ran, and wg.Wait() hung.
//   - Duplicate execution. The monitor launched any task in TaskPending, but
//     TaskRunning was set inside the spawned goroutine. Two completions arriving
//     before that write both launched the same task -- running the agent twice
//     and billing for it twice.
//   - Data races. workflow.Results and workflow.Errors were written from N
//     goroutines with no lock at all, and task.Status was written by workers
//     while the monitor read it.
//   - wg.Add from the monitor goroutine, potentially after wg.Wait had already
//     observed a zero counter.
//
// Every wg.Add now happens before wg.Wait, all shared state is mutex-guarded,
// and a cycle is rejected up front rather than deadlocking.
func (o *CodeOrchestrator) ExecuteWorkflow(ctx context.Context, workflow *Workflow) (string, error) {
	if err := validateWorkflow(workflow); err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// One completion channel per task, closed when that task reaches a terminal
	// state. Closing rather than sending means every waiter is released exactly
	// once and nothing can block on a receiver that has gone away.
	done := make(map[string]chan struct{}, len(workflow.Tasks))
	for _, task := range workflow.Tasks {
		done[task.ID] = make(chan struct{})
	}

	// Guards workflow.Results, workflow.Errors and every mutable Task field.
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, task := range workflow.Tasks {
		wg.Add(1) // every Add happens here, before the Wait below
		go func(task *Task) {
			defer wg.Done()
			defer close(done[task.ID])

			// Wait for dependencies. A cancelled context abandons the task
			// rather than leaving the goroutine parked.
			for _, depID := range task.Dependencies {
				select {
				case <-done[depID]:
				case <-ctx.Done():
					mu.Lock()
					task.Status = TaskFailed
					task.Error = ctx.Err()
					workflow.Errors[task.ID] = task.Error
					mu.Unlock()
					return
				}
			}

			// A task whose dependency failed cannot run: its input would be
			// missing the result it was written to consume.
			mu.Lock()
			for _, depID := range task.Dependencies {
				if depErr, failed := workflow.Errors[depID]; failed {
					task.Status = TaskFailed
					task.Error = fmt.Errorf("dependency %s failed: %w", depID, depErr)
					workflow.Errors[task.ID] = task.Error
					mu.Unlock()
					return
				}
			}

			// Build the input from dependency results while still holding the
			// lock, so no concurrent writer can tear the map.
			input := task.Input
			for _, depID := range task.Dependencies {
				if result, ok := workflow.Results[depID]; ok {
					input = fmt.Sprintf("%s\n\nResult from %s: %s", input, depID, result)
				}
			}
			task.Status = TaskRunning
			agentID := task.AgentID
			mu.Unlock()

			agent, ok := o.registry.Get(agentID)
			if !ok {
				mu.Lock()
				task.Status = TaskFailed
				task.Error = fmt.Errorf("agent not found: %s", agentID)
				workflow.Errors[task.ID] = task.Error
				mu.Unlock()
				return
			}

			result, err := agent.Run(ctx, input)

			mu.Lock()
			if err != nil {
				task.Status = TaskFailed
				task.Error = fmt.Errorf("agent execution failed: %w", err)
				workflow.Errors[task.ID] = task.Error
			} else {
				task.Status = TaskCompleted
				task.Result = result
				workflow.Results[task.ID] = result
			}
			mu.Unlock()
		}(task)
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	if workflow.FinalTaskID == "" {
		return "", nil
	}
	if err, ok := workflow.Errors[workflow.FinalTaskID]; ok {
		return "", fmt.Errorf("final task failed: %w", err)
	}
	if result, ok := workflow.Results[workflow.FinalTaskID]; ok {
		return result, nil
	}
	return "", fmt.Errorf("final task result not found")
}

// validateWorkflow rejects a workflow that cannot be scheduled.
//
// Dependency cycles and references to unknown tasks both used to manifest as a
// hang: a task waits for a dependency that will never complete. Detecting them
// up front turns a hang into an error naming the problem.
func validateWorkflow(workflow *Workflow) error {
	if workflow == nil {
		return fmt.Errorf("workflow is nil")
	}

	byID := make(map[string]*Task, len(workflow.Tasks))
	for _, task := range workflow.Tasks {
		if task == nil {
			return fmt.Errorf("workflow contains a nil task")
		}
		if _, duplicate := byID[task.ID]; duplicate {
			return fmt.Errorf("duplicate task ID %q", task.ID)
		}
		byID[task.ID] = task
	}

	for _, task := range workflow.Tasks {
		for _, depID := range task.Dependencies {
			if _, ok := byID[depID]; !ok {
				return fmt.Errorf("task %q depends on unknown task %q", task.ID, depID)
			}
		}
	}

	if workflow.FinalTaskID != "" {
		if _, ok := byID[workflow.FinalTaskID]; !ok {
			return fmt.Errorf("final task %q is not in the workflow", workflow.FinalTaskID)
		}
	}

	// Iterative depth-first search, three-colour marking.
	const (
		unvisited  = 0
		inProgress = 1
		finished   = 2
	)
	state := make(map[string]int, len(byID))

	var visit func(id string, path []string) error
	visit = func(id string, path []string) error {
		switch state[id] {
		case inProgress:
			return fmt.Errorf("dependency cycle: %s -> %s", strings.Join(path, " -> "), id)
		case finished:
			return nil
		}
		state[id] = inProgress
		for _, depID := range byID[id].Dependencies {
			if err := visit(depID, append(path, id)); err != nil {
				return err
			}
		}
		state[id] = finished
		return nil
	}

	for _, task := range workflow.Tasks {
		if err := visit(task.ID, nil); err != nil {
			return err
		}
	}

	return nil
}
