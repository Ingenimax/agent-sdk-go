package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/planner"
	"github.com/google/uuid"
)

func main() {
	// Create a logger
	logger := logging.New()
	ctx := context.Background()

	// Set up an LLM client
	llmClient := setupLLMClient(logger)

	// Create different types of planners
	simpleLLMPlanner := planner.NewSimpleLLMPlanner(llmClient, logger)
	mockAIPlanner := &planner.MockAIPlanner{}

	// Create a planner with a custom system prompt for software development
	softwareDevSystemPrompt := `You are an expert software development planner. Your role is to create detailed,
structured plans for software development tasks based on the task description. Each plan should include:
- An analysis phase that includes requirements gathering and technical assessment
- A design phase that covers architectural decisions and component design
- An implementation phase with specific coding tasks
- A testing phase with unit, integration, and system testing
- A deployment phase with release steps and monitoring
Keep your plans thorough yet concise, with an emphasis on software engineering best practices.`

	customPromptPlanner := planner.NewSimpleLLMPlannerWithSystemPrompt(llmClient, logger, softwareDevSystemPrompt)

	// Create a CorePlanner with the real LLM-based planner
	corePlanner := planner.NewCorePlannerWithAI(logger, simpleLLMPlanner)

	// Create a CorePlanner with the mock planner for comparison
	corePlannerWithMock := planner.NewCorePlannerWithAI(logger, mockAIPlanner)

	// Create some example tasks
	tasks := []*core.Task{
		createTask("Data Processing Task", "Process customer transaction data to identify patterns and generate insights."),
		createTask("API Integration", "Develop an API integration with the payment provider to handle subscription renewals."),
		createTask("Bug Fix", "Fix the authentication issue in the login flow that occurs with certain browsers."),
		createTask("Database Optimization", "Optimize database queries to improve application performance."),
		createTask("Mobile App Feature", "Implement a new user profile screen with editing capabilities in the mobile app."),
	}

	fmt.Println("===== Testing Different AI Planners =====")
	fmt.Println()

	// Test SimpleLLMPlanner directly
	fmt.Println("===== Real LLM-based Planner =====")
	task := tasks[0] // Data processing task
	plan, err := simpleLLMPlanner.GeneratePlan(ctx, task)
	if err != nil {
		logger.Error(ctx, "Failed to generate plan with SimpleLLMPlanner", map[string]interface{}{
			"task_id": task.ID,
			"error":   err.Error(),
		})
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println(plan)
	}
	fmt.Println()

	// Test MockAIPlanner directly
	fmt.Println("===== MockAIPlanner =====")
	task = tasks[1] // API Integration task
	plan, err = mockAIPlanner.GeneratePlan(ctx, task)
	if err != nil {
		logger.Error(ctx, "Failed to generate plan with MockAIPlanner", map[string]interface{}{
			"task_id": task.ID,
			"error":   err.Error(),
		})
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println(plan)
	}
	fmt.Println()

	// Test CustomPromptPlanner with software development focus
	fmt.Println("===== Custom System Prompt Planner (Software Development) =====")
	task = tasks[4] // Mobile App Feature task
	plan, err = customPromptPlanner.GeneratePlan(ctx, task)
	if err != nil {
		logger.Error(ctx, "Failed to generate plan with CustomPromptPlanner", map[string]interface{}{
			"task_id": task.ID,
			"error":   err.Error(),
		})
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println(plan)
	}
	fmt.Println()

	// Test CorePlanner with real LLM (via SimpleLLMPlanner)
	fmt.Println("===== CorePlanner with real LLM =====")
	task = tasks[2] // Bug Fix task
	plan, err = corePlanner.CreatePlan(ctx, task)
	if err != nil {
		logger.Error(ctx, "Failed to generate plan with CorePlanner", map[string]interface{}{
			"task_id": task.ID,
			"error":   err.Error(),
		})
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println(plan)
	}
	fmt.Println()

	// Test CorePlanner with MockAIPlanner
	fmt.Println("===== CorePlanner with MockAIPlanner =====")
	task = tasks[3] // Database Optimization task
	plan, err = corePlannerWithMock.CreatePlan(ctx, task)
	if err != nil {
		logger.Error(ctx, "Failed to generate plan with CorePlanner + MockAIPlanner", map[string]interface{}{
			"task_id": task.ID,
			"error":   err.Error(),
		})
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println(plan)
	}
	fmt.Println()

	// Test fallback behavior by passing a non-task
	fmt.Println("===== Fallback Behavior (non-task input) =====")
	plan, err = corePlanner.CreatePlan(ctx, "This is not a task")
	if err != nil {
		logger.Error(ctx, "Failed in fallback scenario", map[string]interface{}{
			"error": err.Error(),
		})
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println(plan)
	}
}

// setupLLMClient creates and configures an LLM client
func setupLLMClient(logger logging.Logger) interfaces.LLM {
	// Try to get API key from environment variable
	apiKey := os.Getenv("OPENAI_API_KEY")

	// If no API key is found, use a mock key with a warning
	if apiKey == "" {
		fmt.Println("⚠️ Warning: No OPENAI_API_KEY environment variable found.")
		fmt.Println("    Using mock planning response. For real LLM responses, set your API key.")

		// Return a simple mock implementation
		return &mockLLM{}
	}

	// Get configuration from environment or use defaults
	cfg := config.Get()

	// Create OpenAI client with appropriate options
	model := cfg.LLM.OpenAI.Model
	if model == "" {
		model = "gpt-4o-mini" // Default model
	}

	client := openai.NewClient(
		apiKey,
		openai.WithModel(model),
		openai.WithLogger(logger),
	)

	return client
}

// mockLLM is a simple mock implementation of the LLM interface
type mockLLM struct{}

// Generate returns a mock response
func (m *mockLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// Apply options to get system message if present
	opts := &interfaces.GenerateOptions{}
	for _, option := range options {
		option(opts)
	}

	// Check if this is the software development prompt
	if opts.SystemMessage != "" && len(opts.SystemMessage) > 100 &&
		(opts.SystemMessage[:100] == "You are an expert software development planner" ||
			opts.SystemMessage[:100] == "You are an expert software development planner. Your role is to create detailed,") {
		// Return a software development focused plan
		return `## Software Development Plan for Mobile App Feature

### Analysis Phase
1. Review requirements for the user profile screen
2. Identify required user data fields and editing capabilities
3. Analyze UI/UX requirements and platform constraints
4. Define technical constraints and dependencies

### Design Phase
5. Create UI mockups and wireframes
6. Design database schema and API endpoints
7. Plan navigation flow and component hierarchy
8. Define state management approach

### Implementation Phase
9. Set up project structure and base components
10. Implement read-only profile view
11. Develop form components for editing
12. Implement data validation and error handling
13. Connect to backend APIs
14. Add state management and navigation

### Testing Phase
15. Write unit tests for components
16. Perform integration testing with backend
17. Conduct usability testing
18. Execute performance testing on target devices

### Deployment Phase
19. Prepare for app store submission
20. Create release notes and documentation
21. Deploy to staging environment
22. Monitor post-release performance and issues`, nil
	}

	// Simple mock response that looks like an AI-generated plan
	return `## Plan for the Task

### Analysis Phase
1. Understand the requirements and objectives
2. Identify necessary resources and dependencies
3. Analyze potential challenges and constraints

### Implementation Phase
4. Break down the task into manageable sub-tasks
5. Implement each component sequentially
6. Integrate components and ensure compatibility
7. Perform initial testing and debugging

### Verification Phase
8. Test thoroughly against requirements
9. Document the implementation and results
10. Prepare for review and handoff`, nil
}

// GenerateWithTools is not used in this example
func (m *mockLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	return m.Generate(ctx, prompt, options...)
}

// Name returns the name of the mock LLM
func (m *mockLLM) Name() string {
	return "mock-llm"
}

// Helper function to create a task
func createTask(name, description string) *core.Task {
	now := time.Now()
	return &core.Task{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Status:      core.StatusPending,
		UserID:      "user123",
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata: map[string]interface{}{
			"priority":   "high",
			"deadline":   now.Add(7 * 24 * time.Hour).Format(time.RFC3339),
			"complexity": "medium",
		},
	}
}
