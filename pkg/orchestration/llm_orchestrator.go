package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// Step represents a single step in an orchestration plan
type Step struct {
	// AgentID is the ID of the agent to execute
	AgentID string `json:"agent_id"`

	// Input is the input to provide to the agent
	Input string `json:"input"`

	// Description explains the purpose of this step
	Description string `json:"description"`

	// DependsOn lists the IDs of steps that must complete before this one
	DependsOn []string `json:"depends_on,omitempty"`
}

// Plan represents an orchestration plan
type Plan struct {
	// Steps is the list of steps in the plan
	Steps []Step `json:"steps"`

	// FinalAgentID is the ID of the agent that should provide the final response
	FinalAgentID string `json:"final_agent_id"`
}

// LLMOrchestrator orchestrates the execution of a query using multiple agents
type LLMOrchestrator struct {
	registry *AgentRegistry
	planner  interfaces.LLM
	logger   logging.Logger
}

// NewLLMOrchestrator creates a new LLM orchestrator
func NewLLMOrchestrator(registry *AgentRegistry, planner interfaces.LLM) *LLMOrchestrator {
	return &LLMOrchestrator{
		registry: registry,
		planner:  planner,
		logger:   logging.New(),
	}
}

// WithLogger sets the logger for the orchestrator
func (o *LLMOrchestrator) WithLogger(logger logging.Logger) *LLMOrchestrator {
	o.logger = logger
	return o
}

// Execute executes a query using the orchestrator
func (o *LLMOrchestrator) Execute(ctx context.Context, query string) (string, error) {
	o.logger.Info(ctx, "Starting execution for query", map[string]interface{}{"query": query})

	// Create a plan
	plan, err := o.createPlan(ctx, query)
	if err != nil {
		o.logger.Error(ctx, "Failed to create plan", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("failed to create plan: %w", err)
	}

	// Execute the plan
	result, err := o.executePlan(ctx, plan)
	if err != nil {
		o.logger.Error(ctx, "Failed to execute plan", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("failed to execute plan: %w", err)
	}

	// Generate final response
	finalResponse, err := o.generateFinalResponse(ctx, plan, result)
	if err != nil {
		o.logger.Error(ctx, "Failed to generate final response", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("failed to generate final response: %w", err)
	}

	o.logger.Info(ctx, "Execution completed successfully", nil)
	return finalResponse, nil
}

// createPlan creates a plan for executing a query
func (o *LLMOrchestrator) createPlan(ctx context.Context, query string) (*Plan, error) {
	// Get available agents
	agents := o.registry.List()
	agentDescriptions := make(map[string]string)

	for id, agent := range agents {
		// Get agent description from system prompt using reflection
		agentValue := reflect.ValueOf(agent).Elem()
		systemPromptField := agentValue.FieldByName("systemPrompt")

		var description string
		if systemPromptField.IsValid() && systemPromptField.Kind() == reflect.String {
			systemPrompt := systemPromptField.String()
			// Extract first line as description
			description = strings.Split(systemPrompt, "\n")[0]
		} else {
			// Fallback to using the agent ID
			description = id
		}
		agentDescriptions[id] = description
	}

	// Create a prompt for the LLM
	prompt := fmt.Sprintf(`You are an orchestrator that creates plans to solve complex problems using multiple specialized agents.

Available agents:
%s

User query: %s

Create a plan to solve this query using the available agents. The plan should be a JSON object with the following structure:
{
  "steps": [
    {
      "agent_id": "string",
      "input": "string",
      "description": "string",
      "depends_on": ["step_0", "step_1"]
    }
  ],
  "final_agent_id": "string"
}

Each step should specify which agent to use, what input to provide, a description of the step's purpose, and any dependencies on other steps.

IMPORTANT RULES:
1. For dependencies, use the step index in the format "step_0", "step_1", etc. to refer to previous steps. Do not use agent IDs as dependencies.
2. To reference the output of a previous step in the input field, use the format {{step_0}}, {{step_1}}, etc.
3. Make sure all dependencies are valid - a step can only depend on steps with lower indices.
4. The final step should depend on all previous steps that contribute to the final answer.
5. The final_agent_id should specify which agent should provide the final response to the user.
6. Ensure the dependency chain is complete and there are no circular dependencies.

Respond with only the JSON plan.`, formatAgentDescriptions(agentDescriptions), query)

	// Generate a plan
	response, err := o.planner.Generate(ctx, prompt)
	if err != nil {
		o.logger.Error(ctx, "Failed to generate plan", map[string]interface{}{"error": err.Error()})
		return nil, fmt.Errorf("failed to generate plan: %w", err)
	}

	// Extract JSON from response
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		o.logger.Error(ctx, "Failed to extract JSON from response", nil)
		return nil, fmt.Errorf("failed to extract JSON from response: %s", response)
	}

	// Parse the plan
	var plan Plan
	err = json.Unmarshal([]byte(jsonStr), &plan)
	if err != nil {
		o.logger.Error(ctx, "Failed to parse plan", map[string]interface{}{"error": err.Error()})
		return nil, fmt.Errorf("failed to parse plan: %w", err)
	}

	// Validate and fix dependencies if needed
	for i := range plan.Steps {
		for j, dep := range plan.Steps[i].DependsOn {
			// If the dependency doesn't start with "step_", prepend it
			if !strings.HasPrefix(dep, "step_") {
				// Check if it's a numeric index
				if _, err := strconv.Atoi(dep); err == nil {
					plan.Steps[i].DependsOn[j] = "step_" + dep
					o.logger.Info(ctx, "Fixed dependency", map[string]interface{}{"from": dep, "to": plan.Steps[i].DependsOn[j]})
				}
			}
		}
	}

	// Ensure the final step depends on all previous steps
	if len(plan.Steps) > 0 {
		finalStepIndex := len(plan.Steps) - 1
		finalStep := &plan.Steps[finalStepIndex]

		// Create a map of existing dependencies for quick lookup
		existingDeps := make(map[string]bool)
		for _, dep := range finalStep.DependsOn {
			existingDeps[dep] = true
		}

		// Add dependencies for all previous steps if not already present
		for i := 0; i < finalStepIndex; i++ {
			depID := fmt.Sprintf("step_%d", i)
			if !existingDeps[depID] {
				finalStep.DependsOn = append(finalStep.DependsOn, depID)
				o.logger.Info(ctx, "Added missing dependency", map[string]interface{}{"dep": depID, "to": finalStep.DependsOn})
			}
		}
	}

	o.logger.Info(ctx, "Plan created with", map[string]interface{}{"steps": len(plan.Steps)})
	return &plan, nil
}

// executePlan executes an orchestration plan
func (o *LLMOrchestrator) executePlan(ctx context.Context, plan *Plan) (map[string]string, error) {
	o.logger.Info(ctx, "Executing plan with", map[string]interface{}{"steps": len(plan.Steps)})

	// Log the plan structure for debugging
	for i, step := range plan.Steps {
		stepID := fmt.Sprintf("step_%d", i)
		dependsOnStr := "none"
		if len(step.DependsOn) > 0 {
			dependsOnStr = strings.Join(step.DependsOn, ", ")
		}
		o.logger.Info(ctx, "Plan step", map[string]interface{}{"id": stepID, "agent": step.AgentID, "depends_on": dependsOnStr, "description": step.Description})
	}

	results := make(map[string]string)
	completed := make(map[string]bool)

	// Create a map of step names to step IDs for dependency resolution
	stepNameToID := make(map[string]string)
	for i, step := range plan.Steps {
		stepID := fmt.Sprintf("step_%d", i)
		// Use agent_id as a fallback name for the step
		stepNameToID[step.AgentID] = stepID
		// Also map the step index as a string
		stepNameToID[fmt.Sprintf("%d", i)] = stepID
		// And map the step_X format directly
		stepNameToID[stepID] = stepID
	}

	// Execute steps until all are completed
	maxIterations := len(plan.Steps) * 3 // Prevent infinite loops
	iteration := 0

	for len(completed) < len(plan.Steps) && iteration < maxIterations {
		iteration++
		stepsExecutedThisIteration := false
		pendingSteps := []string{}

		// Find steps that can be executed
		for i, step := range plan.Steps {
			stepID := fmt.Sprintf("step_%d", i)

			// Skip completed steps
			if completed[stepID] {
				continue
			}

			// Check dependencies
			canExecute := true
			pendingDeps := []string{}

			for _, depName := range step.DependsOn {
				// Try to resolve the dependency name to a step ID
				depID, exists := stepNameToID[depName]
				if !exists {
					// If we can't resolve it, use it as is (might be a direct step_X reference)
					depID = depName
					o.logger.Info(ctx, "Warning: Could not resolve dependency name", map[string]interface{}{"name": depName})
				}

				if !completed[depID] {
					canExecute = false
					pendingDeps = append(pendingDeps, depID)
				}
			}

			if !canExecute {
				pendingSteps = append(pendingSteps, stepID)
				o.logger.Info(ctx, "Step is waiting for dependencies", map[string]interface{}{"step": stepID, "depends_on": strings.Join(pendingDeps, ", ")})
				continue
			}

			o.logger.Info(ctx, "Executing step", map[string]interface{}{"step": stepID, "agent": step.AgentID})

			// Execute step
			agent, ok := o.registry.Get(step.AgentID)
			if !ok {
				o.logger.Error(ctx, "Agent not found", map[string]interface{}{"agent": step.AgentID})
				return nil, fmt.Errorf("agent not found: %s", step.AgentID)
			}

			// Prepare input with context from dependencies
			input := step.Input
			for _, depName := range step.DependsOn {
				// Try to resolve the dependency name to a step ID
				depID, exists := stepNameToID[depName]
				if !exists {
					// If we can't resolve it, use it as is
					depID = depName
				}

				// Replace both the original name and the resolved ID in the template
				input = strings.ReplaceAll(input, fmt.Sprintf("{{%s}}", depName), results[depID])
				input = strings.ReplaceAll(input, fmt.Sprintf("{{%s}}", depID), results[depID])
			}

			// Execute agent
			result, err := agent.Run(ctx, input)
			if err != nil {
				o.logger.Error(ctx, "Failed to execute step", map[string]interface{}{"step": stepID, "error": err.Error()})
				return nil, fmt.Errorf("failed to execute step %s: %w", stepID, err)
			}

			// Store result
			o.logger.Info(ctx, "Step completed successfully", map[string]interface{}{"step": stepID})
			results[stepID] = result
			completed[stepID] = true
			stepsExecutedThisIteration = true

			// Also store the result under the agent ID for backward compatibility
			results[step.AgentID] = result

			// Log character count for research and summary agents
			switch step.AgentID {
			case "research":
				o.logger.Info(ctx, "Research agent output", map[string]interface{}{"length": len(result)})
			case "summary":
				o.logger.Info(ctx, "Summary agent output", map[string]interface{}{"length": len(result)})
			}
		}

		// Check for deadlock
		if !stepsExecutedThisIteration && len(completed) < len(plan.Steps) {
			o.logger.Info(ctx, "Potential deadlock detected", map[string]interface{}{"pending_steps": strings.Join(pendingSteps, ", ")})

			// If we're on the last step and have a creative and summary result, we can proceed
			if len(completed) == len(plan.Steps)-1 &&
				results["creative"] != "" && results["summary"] != "" {
				o.logger.Info(ctx, "Proceeding with execution despite missing one step, as we have both creative and summary results", nil)
				break
			}

			// If we have at least one result, we can try to continue with what we have
			if len(results) > 0 {
				o.logger.Info(ctx, "Attempting to continue with partial results", map[string]interface{}{"completed": len(completed), "total": len(plan.Steps)})
				break
			}

			return nil, fmt.Errorf("deadlock detected: no steps can be executed")
		}

		// Sleep to avoid busy waiting
		time.Sleep(100 * time.Millisecond)
	}

	if iteration >= maxIterations {
		o.logger.Info(ctx, "Warning: Reached maximum iterations", map[string]interface{}{"iterations": maxIterations, "completed": len(completed), "total": len(plan.Steps)})
	}

	// Log comparison of research vs summary if both exist
	if researchResult, hasResearch := results["research"]; hasResearch {
		if summaryResult, hasSummary := results["summary"]; hasSummary {
			researchLen := len(researchResult)
			summaryLen := len(summaryResult)
			compressionRatio := float64(summaryLen) / float64(researchLen) * 100
			o.logger.Info(ctx, "Character count comparison", map[string]interface{}{"research": researchLen, "summary": summaryLen, "compression_ratio": fmt.Sprintf("%.1f%% of original", compressionRatio)})
		}
	}

	o.logger.Info(ctx, "All steps completed successfully", map[string]interface{}{"completed": len(completed)})
	return results, nil
}

// generateFinalResponse generates the final response
func (o *LLMOrchestrator) generateFinalResponse(ctx context.Context, plan *Plan, results map[string]string) (string, error) {
	o.logger.Info(ctx, "Generating final response using agent", map[string]interface{}{"agent": plan.FinalAgentID})

	// Get the final agent
	finalAgent, ok := o.registry.Get(plan.FinalAgentID)
	if !ok {
		// If the specified final agent is not available, try to use a fallback
		o.logger.Info(ctx, "Final agent not found, trying to use a fallback", map[string]interface{}{"agent": plan.FinalAgentID})

		// Try to use summary agent as fallback
		if summaryAgent, ok := o.registry.Get("summary"); ok {
			finalAgent = summaryAgent
			o.logger.Info(ctx, "Using summary agent as fallback for final response", nil)
		} else if creativeAgent, ok := o.registry.Get("creative"); ok {
			// Try creative agent as second fallback
			finalAgent = creativeAgent
			o.logger.Info(ctx, "Using creative agent as fallback for final response", nil)
		} else {
			// No suitable fallback found
			return "", fmt.Errorf("no suitable agent found for generating final response")
		}
	}

	// Create the final prompt
	var finalPrompt strings.Builder
	finalPrompt.WriteString("Based on the following information, provide a comprehensive response:\n\n")

	// Add the results from each step
	completedSteps := 0
	for i, step := range plan.Steps {
		stepID := fmt.Sprintf("step_%d", i)
		if result, ok := results[stepID]; ok {
			finalPrompt.WriteString(fmt.Sprintf("--- %s (%s) ---\n%s\n\n", step.Description, step.AgentID, result))
			completedSteps++
		}
	}

	o.logger.Info(ctx, "Completed steps before generating final response", map[string]interface{}{"completed": completedSteps, "total": len(plan.Steps)})

	// Generate the final response
	finalResponse, err := finalAgent.Run(ctx, finalPrompt.String())
	if err != nil {
		o.logger.Error(ctx, "Failed to generate final response", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("failed to generate final response: %w", err)
	}

	o.logger.Info(ctx, "Final response generated successfully", nil)
	return finalResponse, nil
}

// formatAgentDescriptions formats agent descriptions for the prompt
func formatAgentDescriptions(descriptions map[string]string) string {
	var result strings.Builder
	for id, desc := range descriptions {
		result.WriteString(fmt.Sprintf("- %s: %s\n", id, desc))
	}
	return result.String()
}

// extractJSON extracts JSON from a string
func extractJSON(s string) string {
	// Find the first { and the last }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")

	if start == -1 || end == -1 || end <= start {
		return ""
	}

	return s[start : end+1]
}
