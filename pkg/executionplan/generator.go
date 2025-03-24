package executionplan

import (
	"context"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

// Generator handles generation of execution plans
type Generator struct {
	llm          interfaces.LLM
	tools        []interfaces.Tool
	systemPrompt string
}

// NewGenerator creates a new execution plan generator
func NewGenerator(llm interfaces.LLM, tools []interfaces.Tool, systemPrompt string) *Generator {
	return &Generator{
		llm:          llm,
		tools:        tools,
		systemPrompt: systemPrompt,
	}
}

// GenerateExecutionPlan generates an execution plan based on the user input
func (g *Generator) GenerateExecutionPlan(ctx context.Context, input string) (*ExecutionPlan, error) {
	// If no tools are available, return an error
	if len(g.tools) == 0 {
		return nil, fmt.Errorf("no tools available for execution planning")
	}

	// Create a prompt for the LLM to generate an execution plan
	prompt := CreateExecutionPlanPrompt(input, g.tools)

	// Add system prompt as a generate option
	generateOptions := []interfaces.GenerateOption{}
	if g.systemPrompt != "" {
		generateOptions = append(generateOptions, openai.WithSystemMessage(g.systemPrompt))
	}

	// Generate the execution plan using the LLM
	response, err := g.llm.Generate(ctx, prompt, generateOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate execution plan: %w", err)
	}

	// Parse the execution plan from the LLM response
	plan, err := ParseExecutionPlanFromResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse execution plan: %w", err)
	}

	// Update the status to pending approval
	plan.Status = StatusPendingApproval

	return plan, nil
}

// ModifyExecutionPlan modifies an execution plan based on user input
func (g *Generator) ModifyExecutionPlan(ctx context.Context, plan *ExecutionPlan, modifications string) (*ExecutionPlan, error) {
	// Create a prompt for the LLM to modify the execution plan
	prompt := fmt.Sprintf(`
You are an AI assistant that modifies execution plans based on user feedback. 
Here is the current execution plan:

%s

The user has requested the following modifications:
%s

Please modify the execution plan according to the user's request and return the updated plan in the same JSON format:
{
  "description": "High-level description of what the plan will accomplish",
  "steps": [
    {
      "toolName": "Name of the tool to use",
      "description": "Description of what this step will accomplish",
      "input": "Input to provide to the tool",
      "parameters": {
        "param1": "value1",
        "param2": "value2"
      }
    }
  ]
}

Ensure that:
1. Each step uses a valid tool from the list of available tools
2. All required parameters for each tool are provided
3. The plan is comprehensive and addresses all aspects of the user's request
4. The plan is presented in valid JSON format

Modified Execution Plan:
`, FormatExecutionPlan(plan), modifications)

	// Add system prompt as a generate option
	generateOptions := []interfaces.GenerateOption{}
	if g.systemPrompt != "" {
		generateOptions = append(generateOptions, openai.WithSystemMessage(g.systemPrompt))
	}

	// Generate the modified execution plan using the LLM
	response, err := g.llm.Generate(ctx, prompt, generateOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate modified execution plan: %w", err)
	}

	// Parse the modified execution plan from the LLM response
	modifiedPlan, err := ParseExecutionPlanFromResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse modified execution plan: %w", err)
	}

	// Preserve the task ID from the original plan
	modifiedPlan.TaskID = plan.TaskID
	modifiedPlan.Status = StatusPendingApproval

	return modifiedPlan, nil
}
