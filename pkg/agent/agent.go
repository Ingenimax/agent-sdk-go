package agent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/executionplan"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Agent represents an AI agent
type Agent struct {
	llm                  interfaces.LLM
	memory               interfaces.Memory
	tools                []interfaces.Tool
	orgID                string
	tracer               interfaces.Tracer
	guardrails           interfaces.Guardrails
	systemPrompt         string
	name                 string                   // Name of the agent, e.g., "PlatformOps", "Math", "Research"
	requirePlanApproval  bool                     // New field to control whether execution plans require approval
	planStore            *executionplan.Store     // Store for execution plans
	planGenerator        *executionplan.Generator // Generator for execution plans
	planExecutor         *executionplan.Executor  // Executor for execution plans
	generatedAgentConfig *AgentConfig
	generatedTaskConfigs TaskConfigs
	responseFormat       *interfaces.ResponseFormat // Response format for the agent
	llmConfig            *interfaces.LLMConfig
	currentPlan          *executionplan.ExecutionPlan
}

// Option represents an option for configuring an agent
type Option func(*Agent)

// WithLLM sets the LLM for the agent
func WithLLM(llm interfaces.LLM) Option {
	return func(a *Agent) {
		a.llm = llm
	}
}

// WithMemory sets the memory for the agent
func WithMemory(memory interfaces.Memory) Option {
	return func(a *Agent) {
		a.memory = memory
	}
}

// WithTools sets the tools for the agent
func WithTools(tools ...interfaces.Tool) Option {
	return func(a *Agent) {
		a.tools = tools
	}
}

// WithOrgID sets the organization ID for multi-tenancy
func WithOrgID(orgID string) Option {
	return func(a *Agent) {
		a.orgID = orgID
	}
}

// WithTracer sets the tracer for the agent
func WithTracer(tracer interfaces.Tracer) Option {
	return func(a *Agent) {
		a.tracer = tracer
	}
}

// WithGuardrails sets the guardrails for the agent
func WithGuardrails(guardrails interfaces.Guardrails) Option {
	return func(a *Agent) {
		a.guardrails = guardrails
	}
}

// WithSystemPrompt sets the system prompt for the agent
func WithSystemPrompt(prompt string) Option {
	return func(a *Agent) {
		a.systemPrompt = prompt
	}
}

// WithRequirePlanApproval sets whether execution plans require user approval
func WithRequirePlanApproval(require bool) Option {
	return func(a *Agent) {
		a.requirePlanApproval = require
	}
}

// WithName sets the name for the agent
func WithName(name string) Option {
	return func(a *Agent) {
		a.name = name
	}
}

// WithAgentConfig sets the agent configuration from a YAML config
func WithAgentConfig(config AgentConfig, variables map[string]string) Option {
	return func(a *Agent) {
		systemPrompt := FormatSystemPromptFromConfig(config, variables)
		a.systemPrompt = systemPrompt
	}
}

// WithResponseFormat sets the response format for the agent
func WithResponseFormat(formatType interfaces.ResponseFormat) Option {
	return func(a *Agent) {
		a.responseFormat = &formatType
	}
}

func WithLLMConfig(config interfaces.LLMConfig) Option {
	return func(a *Agent) {
		a.llmConfig = &config
	}
}

// NewAgent creates a new agent with the given options
func NewAgent(options ...Option) (*Agent, error) {
	agent := &Agent{
		requirePlanApproval: true, // Default to requiring approval
	}

	for _, option := range options {
		option(agent)
	}

	// Validate required fields
	if agent.llm == nil {
		return nil, fmt.Errorf("LLM is required")
	}

	// Initialize execution plan components
	agent.planStore = executionplan.NewStore()
	agent.planGenerator = executionplan.NewGenerator(agent.llm, agent.tools, agent.systemPrompt)
	agent.planExecutor = executionplan.NewExecutor(agent.tools)

	return agent, nil
}

// NewAgentWithAutoConfig creates a new agent with automatic configuration generation
// based on the system prompt if explicit configuration is not provided
func NewAgentWithAutoConfig(ctx context.Context, options ...Option) (*Agent, error) {
	// First create an agent with the provided options
	agent, err := NewAgent(options...)
	if err != nil {
		return nil, err
	}

	// If the agent doesn't have a name, set a default one
	if agent.name == "" {
		agent.name = "Auto-Configured Agent"
	}

	// If the system prompt is provided but no configuration was explicitly set,
	// generate configuration using the LLM
	if agent.systemPrompt != "" {
		// Generate agent and task configurations from the system prompt
		agentConfig, taskConfigs, err := GenerateConfigFromSystemPrompt(ctx, agent.llm, agent.systemPrompt)
		if err != nil {
			// If we fail to generate configs, just continue with the manual system prompt
			// We don't want to fail agent creation just because auto-config failed
			return agent, nil
		}

		// Create a task configuration map
		taskConfigMap := make(TaskConfigs)
		for i, taskConfig := range taskConfigs {
			taskName := fmt.Sprintf("auto_task_%d", i+1)
			taskConfig.Agent = agent.name // Set the task to use this agent
			taskConfigMap[taskName] = taskConfig
		}

		// Store generated configurations in agent so they can be accessed later
		agent.generatedAgentConfig = &agentConfig
		agent.generatedTaskConfigs = taskConfigMap
	}

	return agent, nil
}

// NewAgentFromConfig creates a new agent from a YAML configuration
func NewAgentFromConfig(agentName string, configs AgentConfigs, variables map[string]string, options ...Option) (*Agent, error) {
	config, exists := configs[agentName]
	if !exists {
		return nil, fmt.Errorf("agent configuration for %s not found", agentName)
	}

	// Add the agent config option
	configOption := WithAgentConfig(config, variables)
	nameOption := WithName(agentName)

	// Combine all options
	allOptions := append([]Option{configOption, nameOption}, options...)

	return NewAgent(allOptions...)
}

// CreateAgentForTask creates a new agent for a specific task
func CreateAgentForTask(taskName string, agentConfigs AgentConfigs, taskConfigs TaskConfigs, variables map[string]string, options ...Option) (*Agent, error) {
	agentName, err := GetAgentForTask(taskConfigs, taskName)
	if err != nil {
		return nil, err
	}

	return NewAgentFromConfig(agentName, agentConfigs, variables, options...)
}

// Run runs the agent with the given input
func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	// If orgID is set on the agent, add it to the context
	if a.orgID != "" {
		ctx = multitenancy.WithOrgID(ctx, a.orgID)
	}

	// Start tracing if available
	var span interfaces.Span
	if a.tracer != nil {
		ctx, span = a.tracer.StartSpan(ctx, "agent.Run")
		defer span.End()
	}

	// Add user message to memory
	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    "user",
			Content: input,
		}); err != nil {
			return "", fmt.Errorf("failed to add user message to memory: %w", err)
		}
	}

	// Apply guardrails to input if available
	if a.guardrails != nil {
		guardedInput, err := a.guardrails.ProcessInput(ctx, input)
		if err != nil {
			return "", fmt.Errorf("guardrails error: %w", err)
		}
		input = guardedInput
	}

	// Check if the input is related to an existing plan
	taskID, action, planInput := a.extractPlanAction(input)
	if taskID != "" {
		result, err := a.handlePlanAction(ctx, taskID, action, planInput)
		if err != nil {
			return "", err
		}
		return result, nil
	}

	// Check if the user is asking about the agent's role or identity
	if a.systemPrompt != "" && a.isAskingAboutRole(input) {
		response := a.generateRoleResponse()

		// Add the role response to memory if available
		if a.memory != nil {
			if err := a.memory.AddMessage(ctx, interfaces.Message{
				Role:    "assistant",
				Content: response,
			}); err != nil {
				return "", fmt.Errorf("failed to add role response to memory: %w", err)
			}
		}

		return response, nil
	}

	// If tools are available and plan approval is required, generate an execution plan
	if len(a.tools) > 0 && a.requirePlanApproval {
		result, err := a.runWithExecutionPlan(ctx, input)
		if err != nil {
			return "", err
		}
		return result, nil
	}

	// Otherwise, run without an execution plan
	result, err := a.runWithoutExecutionPlan(ctx, input)
	if err != nil {
		return "", err
	}

	return result, nil
}

// extractPlanAction extracts plan-related actions from user input
func (a *Agent) extractPlanAction(input string) (string, string, string) {
	// Check if the input is a plan action
	if strings.HasPrefix(input, "approve plan ") {
		taskID := strings.TrimPrefix(input, "approve plan ")
		return taskID, "approve", ""
	}

	if strings.HasPrefix(input, "modify plan ") {
		parts := strings.SplitN(strings.TrimPrefix(input, "modify plan "), " ", 2)
		if len(parts) != 2 {
			return "", "", ""
		}
		return parts[0], "modify", parts[1]
	}

	return "", "", ""
}

// handlePlanAction handles a plan-related action
func (a *Agent) handlePlanAction(ctx context.Context, taskID, action, planInput string) (string, error) {
	// Get the plan from the store
	plan, exists := a.planStore.GetPlanByTaskID(taskID)
	if !exists {
		return "", fmt.Errorf("plan with task ID %s not found", taskID)
	}

	// Handle the action
	switch action {
	case "approve":
		return a.approvePlan(ctx, plan)
	case "modify":
		// Parse the plan input
		modifiedPlan, err := executionplan.ParseExecutionPlanFromResponse(planInput)
		if err != nil {
			return "", fmt.Errorf("failed to parse modified plan: %w", err)
		}

		// Update the plan
		plan = modifiedPlan
		plan.Status = executionplan.StatusPendingApproval

		// Store the updated plan
		a.planStore.StorePlan(plan)

		// Format the plan for display
		formattedPlan := executionplan.FormatExecutionPlan(plan)

		// Add a message to memory asking for user approval
		if a.memory != nil {
			if err := a.memory.AddMessage(ctx, interfaces.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("I've updated the execution plan. Please review it and let me know if you'd like to proceed:\n\n%s", formattedPlan),
			}); err != nil {
				return "", fmt.Errorf("failed to add plan message to memory: %w", err)
			}
		}

		return formattedPlan, nil
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// approvePlan approves an execution plan for execution
func (a *Agent) approvePlan(ctx context.Context, plan *executionplan.ExecutionPlan) (string, error) {
	// Update the plan status to approved
	plan.Status = executionplan.StatusApproved

	// Execute the plan
	result, err := a.planExecutor.ExecutePlan(ctx, plan)
	if err != nil {
		return "", fmt.Errorf("failed to execute plan: %w", err)
	}

	// Format the result
	formattedResult := fmt.Sprintf("Plan executed successfully:\n\n%s", result)

	// Add the result to memory if available
	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    "assistant",
			Content: formattedResult,
		}); err != nil {
			return "", fmt.Errorf("failed to add result to memory: %w", err)
		}
	}

	return formattedResult, nil
}

// modifyPlan modifies a plan based on user input
func (a *Agent) modifyPlan(ctx context.Context, plan *executionplan.ExecutionPlan, input string) (interface{}, error) {
	// Add the modification request to memory
	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    "user",
			Content: "I'd like to modify the plan: " + input,
		}); err != nil {
			return nil, fmt.Errorf("failed to add modification request to memory: %w", err)
		}
	}

	// Modify the plan
	modifiedPlan, err := a.planGenerator.ModifyExecutionPlan(ctx, plan, input)
	if err != nil {
		return nil, fmt.Errorf("failed to modify plan: %w", err)
	}

	// Update the plan in the store
	a.planStore.StorePlan(modifiedPlan)

	// Format the modified plan
	formattedPlan := executionplan.FormatExecutionPlan(modifiedPlan)

	// Add the modified plan to memory
	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    "assistant",
			Content: "I've updated the execution plan based on your feedback:\n\n" + formattedPlan + "\nDo you approve this plan? You can modify it further if needed.",
		}); err != nil {
			return nil, fmt.Errorf("failed to add modified plan to memory: %w", err)
		}
	}

	return "I've updated the execution plan based on your feedback:\n\n" + formattedPlan + "\nDo you approve this plan? You can modify it further if needed.", nil
}

// cancelPlan cancels a plan
func (a *Agent) cancelPlan(plan *executionplan.ExecutionPlan) (interface{}, error) {
	a.planExecutor.CancelPlan(plan)

	return "Plan cancelled. What would you like to do instead?", nil
}

// getPlanStatus returns the status of a plan
func (a *Agent) getPlanStatus(plan *executionplan.ExecutionPlan) (interface{}, error) {
	status := a.planExecutor.GetPlanStatus(plan)
	formattedPlan := executionplan.FormatExecutionPlan(plan)

	return fmt.Sprintf("Current plan status: %s\n\n%s", status, formattedPlan), nil
}

// runWithExecutionPlan runs the agent with an execution plan
func (a *Agent) runWithExecutionPlan(ctx context.Context, input string) (string, error) {
	// Generate execution plan
	plan, err := a.GenerateExecutionPlan(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to generate execution plan: %w", err)
	}

	// Store the plan
	a.currentPlan = plan

	// Format the plan for display
	formattedPlan := executionplan.FormatExecutionPlan(plan)

	// Add a message to memory asking for user approval
	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    "assistant",
			Content: fmt.Sprintf("I've created an execution plan. Please review it and let me know if you'd like to proceed:\n\n%s", formattedPlan),
		}); err != nil {
			return "", fmt.Errorf("failed to add plan message to memory: %w", err)
		}
	}

	return formattedPlan, nil
}

// runWithoutExecutionPlan runs the agent without an execution plan
func (a *Agent) runWithoutExecutionPlan(ctx context.Context, input string) (string, error) {
	// Get conversation history if memory is available
	var prompt string
	if a.memory != nil {
		history, err := a.memory.GetMessages(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to get conversation history: %w", err)
		}

		// Format history into prompt
		prompt = formatHistoryIntoPrompt(history)
	} else {
		prompt = input
	}

	// Add system prompt as a generate option
	generateOptions := []interfaces.GenerateOption{}
	if a.systemPrompt != "" {
		generateOptions = append(generateOptions, openai.WithSystemMessage(a.systemPrompt))
	}

	// Add response format as a generate option if available
	if a.responseFormat != nil {
		generateOptions = append(generateOptions, openai.WithResponseFormat(*a.responseFormat))
	}

	if a.llmConfig != nil {
		generateOptions = append(generateOptions, func(options *interfaces.GenerateOptions) {
			options.LLMConfig = a.llmConfig
		})
	}

	// Generate response with tools if available
	var response string
	var err error
	if len(a.tools) > 0 {
		response, err = a.llm.GenerateWithTools(ctx, prompt, a.tools, generateOptions...)
	} else {
		response, err = a.llm.Generate(ctx, prompt, generateOptions...)
	}
	if err != nil {
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	// Apply guardrails to output if available
	if a.guardrails != nil {
		guardedResponse, err := a.guardrails.ProcessOutput(ctx, fmt.Sprintf("%v", response))
		if err != nil {
			return "", fmt.Errorf("guardrails error: %w", err)
		}
		response = guardedResponse
	}

	// Add agent message to memory
	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    "assistant",
			Content: fmt.Sprintf("%v", response),
		}); err != nil {
			return "", fmt.Errorf("failed to add agent message to memory: %w", err)
		}
	}

	return response, nil
}

// GetTaskByID returns a task by its ID
func (a *Agent) GetTaskByID(taskID string) (*executionplan.ExecutionPlan, bool) {
	return a.planStore.GetPlanByTaskID(taskID)
}

// ListTasks returns a list of all tasks
func (a *Agent) ListTasks() []*executionplan.ExecutionPlan {
	return a.planStore.ListPlans()
}

// formatHistoryIntoPrompt formats conversation history into a prompt
func formatHistoryIntoPrompt(history []interfaces.Message) string {
	var prompt strings.Builder

	for _, msg := range history {
		prompt.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}

	return prompt.String()
}

// ApproveExecutionPlan approves an execution plan for execution
func (a *Agent) ApproveExecutionPlan(ctx context.Context, plan *executionplan.ExecutionPlan) (string, error) {
	result, err := a.approvePlan(ctx, plan)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", result), nil
}

// ModifyExecutionPlan modifies an execution plan based on user input
func (a *Agent) ModifyExecutionPlan(ctx context.Context, plan *executionplan.ExecutionPlan, modifications string) (*executionplan.ExecutionPlan, error) {
	return a.planGenerator.ModifyExecutionPlan(ctx, plan, modifications)
}

// GenerateExecutionPlan generates an execution plan
func (a *Agent) GenerateExecutionPlan(ctx context.Context, input string) (*executionplan.ExecutionPlan, error) {
	return a.planGenerator.GenerateExecutionPlan(ctx, input)
}

// isAskingAboutRole checks if the user is asking about the agent's role
func (a *Agent) isAskingAboutRole(input string) bool {
	// Convert input to lowercase for case-insensitive matching
	input = strings.ToLower(input)

	// Check for common role-related questions
	roleQuestions := []string{
		"who are you",
		"what are you",
		"what is your role",
		"what do you do",
		"what can you do",
		"what are your capabilities",
		"what are your skills",
		"what are your functions",
		"what are your responsibilities",
		"what is your purpose",
		"what is your job",
		"what is your task",
		"what is your mission",
		"what is your goal",
		"what is your objective",
	}

	for _, question := range roleQuestions {
		if strings.Contains(input, question) {
			return true
		}
	}

	return false
}

// generateRoleResponse generates a response about the agent's role
func (a *Agent) generateRoleResponse() string {
	// If no system prompt is set, return a generic response
	if a.systemPrompt == "" {
		return "I am an AI assistant designed to help you with various tasks. I can understand and respond to your questions, and I'm here to assist you in any way I can."
	}

	// Extract the first sentence or paragraph from the system prompt
	// This is typically where the role is defined
	lines := strings.Split(a.systemPrompt, "\n")
	if len(lines) > 0 {
		// Get the first non-empty line
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				return line
			}
		}
	}

	// Fallback to the full system prompt if no suitable line is found
	return a.systemPrompt
}

// ExecuteTaskFromConfig executes a task using its YAML configuration
func (a *Agent) ExecuteTaskFromConfig(ctx context.Context, taskName string, taskConfigs TaskConfigs, variables map[string]string) (interface{}, error) {
	taskConfig, exists := taskConfigs[taskName]
	if !exists {
		return nil, fmt.Errorf("task configuration for %s not found", taskName)
	}

	// Replace variables in the task description
	description := taskConfig.Description
	for key, value := range variables {
		placeholder := fmt.Sprintf("{%s}", key)
		description = strings.ReplaceAll(description, placeholder, value)
	}

	// Run the agent with the task description
	result, err := a.Run(ctx, description)
	if err != nil {
		return nil, fmt.Errorf("failed to execute task %s: %w", taskName, err)
	}

	// If an output file is specified, write the result to the file
	if taskConfig.OutputFile != "" {
		outputPath := taskConfig.OutputFile
		for key, value := range variables {
			placeholder := fmt.Sprintf("{%s}", key)
			outputPath = strings.ReplaceAll(outputPath, placeholder, value)
		}

		err := os.WriteFile(outputPath, []byte(fmt.Sprintf("%v", result)), 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to write output to file %s: %w", outputPath, err)
		}
	}

	return result, nil
}

// GetGeneratedAgentConfig returns the automatically generated agent configuration, if any
func (a *Agent) GetGeneratedAgentConfig() *AgentConfig {
	return a.generatedAgentConfig
}

// GetGeneratedTaskConfigs returns the automatically generated task configurations, if any
func (a *Agent) GetGeneratedTaskConfigs() TaskConfigs {
	return a.generatedTaskConfigs
}

// SetSystemPrompt sets the system prompt for the agent
func (a *Agent) SetSystemPrompt(prompt string) {
	a.systemPrompt = prompt
}

// SetResponseFormat sets the response format for the agent
func (a *Agent) SetResponseFormat(format interface{}) {
	if rf, ok := format.(interfaces.ResponseFormat); ok {
		a.responseFormat = &rf
	}
}

// AddTool adds a tool to the agent
func (a *Agent) AddTool(tool interfaces.Tool) {
	a.tools = append(a.tools, tool)
}
