package planner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
)

// AIPlanner is an interface for AI-based planning services
type AIPlanner interface {
	GeneratePlan(ctx context.Context, task *core.Task) (string, error)
}

// SimpleLLMPlanner implements a simple AI planner using LLM
type SimpleLLMPlanner struct {
	// LLM client to use for generating plans
	llm interfaces.LLM
	// Logger for the planner
	logger logging.Logger
	// System prompt for the LLM
	systemPrompt string
}

// NewSimpleLLMPlanner creates a new SimpleLLMPlanner with the provided LLM client
func NewSimpleLLMPlanner(llm interfaces.LLM, logger logging.Logger) *SimpleLLMPlanner {
	if logger == nil {
		logger = logging.New()
	}

	return &SimpleLLMPlanner{
		llm:    llm,
		logger: logger,
		systemPrompt: `You are an expert task planner. Your role is to create detailed,
structured plans for completing tasks based on task descriptions. Each plan should include:
- An analysis phase to understand the requirements
- An implementation phase with specific steps
- A verification phase to ensure the task is complete
Keep your plans thorough but concise, and organize them with clear sections and numbered steps.`,
	}
}

// NewSimpleLLMPlannerWithSystemPrompt creates a new SimpleLLMPlanner with a custom system prompt
func NewSimpleLLMPlannerWithSystemPrompt(llm interfaces.LLM, logger logging.Logger, systemPrompt string) *SimpleLLMPlanner {
	planner := NewSimpleLLMPlanner(llm, logger)
	planner.systemPrompt = systemPrompt
	return planner
}

// GeneratePlan generates a plan using an LLM
func (p *SimpleLLMPlanner) GeneratePlan(ctx context.Context, task *core.Task) (string, error) {
	if p.llm == nil {
		return "", fmt.Errorf("LLM client not configured for planner")
	}

	// Create context information to help the LLM understand the task better
	taskContext := map[string]interface{}{
		"id":          task.ID,
		"name":        task.Name,
		"description": task.Description,
	}

	// Include relevant metadata if available
	if task.Metadata != nil {
		if priority, ok := task.Metadata["priority"].(string); ok {
			taskContext["priority"] = priority
		}
		if deadline, ok := task.Metadata["deadline"].(string); ok {
			taskContext["deadline"] = deadline
		}
		if complexity, ok := task.Metadata["complexity"].(string); ok {
			taskContext["complexity"] = complexity
		}
		if taskType, ok := task.Metadata["type"].(string); ok {
			taskContext["type"] = taskType
		}
	}

	// Serialize the task context
	contextJSON, err := json.Marshal(taskContext)
	if err != nil {
		p.logger.Error(ctx, "Failed to serialize task context", map[string]interface{}{
			"task_id": task.ID,
			"error":   err.Error(),
		})
		return "", fmt.Errorf("failed to serialize task context: %w", err)
	}

	// Create the prompt
	prompt := fmt.Sprintf(
		"Create a detailed plan for the following task:\n\n%s\n\n"+
			"The plan should be thorough and include clear steps to accomplish the task. "+
			"Start with an analysis phase, then list the implementation steps, and end with verification steps.",
		string(contextJSON),
	)

	p.logger.Debug(ctx, "Generating plan with LLM", map[string]interface{}{
		"task_id": task.ID,
	})

	// Generate the plan using the LLM
	response, err := p.llm.Generate(ctx, prompt, func(opts *interfaces.GenerateOptions) {
		opts.SystemMessage = p.systemPrompt
	})

	if err != nil {
		p.logger.Error(ctx, "Failed to generate plan with LLM", map[string]interface{}{
			"task_id": task.ID,
			"error":   err.Error(),
		})
		return "", fmt.Errorf("failed to generate plan: %w", err)
	}

	p.logger.Info(ctx, "Successfully generated plan with LLM", map[string]interface{}{
		"task_id":      task.ID,
		"plan_length":  len(response),
		"plan_preview": truncateString(response, 100),
	})

	return response, nil
}

// truncateString truncates a string to the specified length and adds "..." if truncated
func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length] + "..."
}

// MockAIPlanner implements a simple mock AI planner
type MockAIPlanner struct{}

// GeneratePlan generates a mock plan
func (p *MockAIPlanner) GeneratePlan(ctx context.Context, task *core.Task) (string, error) {
	// Simple template-based plan generation
	return fmt.Sprintf(
		"Plan for task: %s\n\n"+
			"1. Analyze the requirements: %s\n"+
			"2. Break down into sub-tasks\n"+
			"3. Implement each sub-task\n"+
			"4. Test the implementation\n"+
			"5. Review and finalize\n",
		task.Name, task.Description,
	), nil
}

// CorePlanner implements the interfaces.TaskPlanner interface
type CorePlanner struct {
	logger    logging.Logger
	aiPlanner AIPlanner
}

// NewCorePlanner creates a new core task planner
func NewCorePlanner(logger logging.Logger) interfaces.TaskPlanner {
	// By default, use the mock AI planner
	// In a production environment, you would configure this with a real AI service
	return &CorePlanner{
		logger:    logger,
		aiPlanner: &MockAIPlanner{},
	}
}

// NewCorePlannerWithAI creates a new core task planner with a specific AI planner
func NewCorePlannerWithAI(logger logging.Logger, aiPlanner AIPlanner) interfaces.TaskPlanner {
	return &CorePlanner{
		logger:    logger,
		aiPlanner: aiPlanner,
	}
}

// CreatePlan creates a plan for a task
func (p *CorePlanner) CreatePlan(ctx context.Context, taskObj interface{}) (string, error) {
	// Try to convert to core.Task
	task, ok := taskObj.(*core.Task)
	if !ok {
		return "Simple default plan", nil
	}

	p.logger.Info(ctx, "Creating plan for task", map[string]interface{}{
		"task_id": task.ID,
	})

	// Use AI service to generate the plan
	if p.aiPlanner != nil {
		plan, err := p.aiPlanner.GeneratePlan(ctx, task)
		if err != nil {
			p.logger.Error(ctx, "Failed to generate AI plan", map[string]interface{}{
				"task_id": task.ID,
				"error":   err.Error(),
			})
			// Fall back to simple plan on error
		} else {
			// Successfully generated AI plan
			return plan, nil
		}
	}

	// Fallback to a simple plan if AI planning fails or is not available
	plan := fmt.Sprintf("Plan for task: %s\n\n", task.Name)
	plan += fmt.Sprintf("1. Analyze the task: %s\n", task.Description)
	plan += "2. Determine required steps\n"
	plan += "3. Execute each step in order\n"
	plan += "4. Verify results\n"
	plan += "5. Report completion"

	return plan, nil
}

// AnalyzeTaskContext extracts key information from task metadata and description
// to generate more contextually relevant plans
func (p *CorePlanner) AnalyzeTaskContext(task *core.Task) map[string]interface{} {
	context := map[string]interface{}{
		"name":        task.Name,
		"description": task.Description,
	}

	// Extract any structured metadata that might help with planning
	if task.Metadata != nil {
		// Look for specific metadata keys that might help with planning
		if priority, ok := task.Metadata["priority"].(string); ok {
			context["priority"] = priority
		}
		if deadline, ok := task.Metadata["deadline"].(string); ok {
			context["deadline"] = deadline
		}
		if complexity, ok := task.Metadata["complexity"].(string); ok {
			context["complexity"] = complexity
		}
		if taskType, ok := task.Metadata["type"].(string); ok {
			context["type"] = taskType
		}
	}

	// In a more advanced implementation, you could use NLP to extract entities,
	// keywords, and topics from the task description

	return context
}

// SerializeTaskForAI prepares a task for sending to an AI service
func (p *CorePlanner) SerializeTaskForAI(task *core.Task) (string, error) {
	// Create a simplified version of the task to send to the AI service
	aiTask := struct {
		ID          string                 `json:"id"`
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
		Context     map[string]interface{} `json:"context,omitempty"`
	}{
		ID:          task.ID,
		Name:        task.Name,
		Description: task.Description,
		Metadata:    task.Metadata,
		Context:     p.AnalyzeTaskContext(task),
	}

	// Serialize to JSON
	data, err := json.Marshal(aiTask)
	if err != nil {
		return "", fmt.Errorf("failed to serialize task: %w", err)
	}

	return string(data), nil
}
