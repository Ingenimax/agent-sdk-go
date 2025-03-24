package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/executionplan"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

func main() {
	// Create a logger
	logger := logging.New()
	logger.Info(context.Background(), "Starting Advanced Agent CLI Tool", nil)

	// Get configuration
	cfg := config.Get()

	// Create OpenAI client
	openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey,
		openai.WithLogger(logger),
		openai.WithModel(cfg.LLM.OpenAI.Model))

	// Create a memory store
	memoryStore := memory.NewConversationBuffer(
		memory.WithMaxSize(100),
	)

	// Create tools registry
	toolRegistry := createAdvancedTools(logger, cfg)

	// Create a new agent
	agentInstance, err := agent.NewAgent(
		agent.WithLLM(openaiClient),
		agent.WithMemory(memoryStore),
		agent.WithTools(toolRegistry.List()...),
		agent.WithSystemPrompt("You are a helpful AI assistant that helps with technical tasks. You can plan and execute complex tasks with user approval."),
		agent.WithName("TaskExecutor"),
		agent.WithRequirePlanApproval(true), // Enable execution plan workflow
	)

	if err != nil {
		logger.Error(context.Background(), "Failed to create agent", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	logger.Info(context.Background(), "Agent created successfully", map[string]interface{}{
		"name": "TaskExecutor",
	})

	// Start the conversation loop
	advancedConversationLoop(agentInstance, logger)
}

func createAdvancedTools(logger logging.Logger, cfg *config.Config) *tools.Registry {
	// Create tools registry
	toolRegistry := tools.NewRegistry()

	// Add web search tool if API keys are available
	if cfg.Tools.WebSearch.GoogleAPIKey != "" && cfg.Tools.WebSearch.GoogleSearchEngineID != "" {
		logger.Info(context.Background(), "Adding Google Search tool", map[string]interface{}{
			"engineID": cfg.Tools.WebSearch.GoogleSearchEngineID,
		})
		searchTool := websearch.New(
			cfg.Tools.WebSearch.GoogleAPIKey,
			cfg.Tools.WebSearch.GoogleSearchEngineID,
		)
		toolRegistry.Register(searchTool)
	} else {
		logger.Info(context.Background(), "Skipping Google Search tool - missing API keys", nil)
	}

	return toolRegistry
}

func advancedConversationLoop(agentInstance *agent.Agent, logger logging.Logger) {
	// Print welcome message
	fmt.Println("Welcome to the Advanced Agent CLI Tool!")
	fmt.Println("Type your questions or commands. The agent will create an execution plan for complex tasks.")
	fmt.Println("Special commands:")
	fmt.Println("  'exit' - Exit the application")
	fmt.Println("  'plans' - List all execution plans")
	fmt.Println("  'approve <task-id>' - Approve an execution plan")
	fmt.Println("  'modify <task-id> <instructions>' - Modify an execution plan")
	fmt.Println("-----------------------------------------")

	// Create a scanner for user input
	scanner := bufio.NewScanner(os.Stdin)

	// Create a base context
	ctx := context.Background()

	// Loop until exit
	for {
		// Prompt for input
		fmt.Print("\nYou: ")
		if !scanner.Scan() {
			break
		}

		// Get the user input
		userInput := scanner.Text()

		// Check for exit command
		if strings.ToLower(userInput) == "exit" {
			fmt.Println("Goodbye!")
			break
		}

		// Check for special commands
		if strings.ToLower(userInput) == "plans" {
			// List all execution plans
			plans := agentInstance.ListTasks()
			fmt.Println("\n=== Execution Plans ===")
			if len(plans) == 0 {
				fmt.Println("No execution plans found.")
			}
			for _, plan := range plans {
				fmt.Printf("Task ID: %s\n", plan.TaskID)
				fmt.Printf("Status: %s\n", plan.Status)
				fmt.Printf("Description: %s\n", plan.Description)
				fmt.Println("-------------------")
			}
			continue
		}

		// Check for approve command
		if strings.HasPrefix(strings.ToLower(userInput), "approve ") {
			parts := strings.SplitN(userInput, " ", 2)
			if len(parts) < 2 {
				fmt.Println("Error: Missing task ID. Usage: approve <task-id>")
				continue
			}
			taskID := parts[1]

			// Run the agent with the approval message
			approvalMsg := fmt.Sprintf("I approve the execution plan for task %s", taskID)
			startTime := time.Now()
			response, err := agentInstance.Run(ctx, approvalMsg)
			processTime := time.Since(startTime)

			// Log the processing time
			logger.Info(ctx, "Agent processed approval", map[string]interface{}{
				"processing_time_ms": processTime.Milliseconds(),
				"task_id":            taskID,
			})

			// Check for errors
			if err != nil {
				logger.Error(ctx, "Error approving execution plan", map[string]interface{}{
					"error":   err.Error(),
					"task_id": taskID,
				})
				fmt.Printf("Error: %s\n", err.Error())
				continue
			}

			// Print the response
			fmt.Printf("Agent: %s\n", response)
			continue
		}

		// Check for modify command
		if strings.HasPrefix(strings.ToLower(userInput), "modify ") {
			parts := strings.SplitN(userInput, " ", 3)
			if len(parts) < 3 {
				fmt.Println("Error: Missing task ID or instructions. Usage: modify <task-id> <instructions>")
				continue
			}
			taskID := parts[1]
			instructions := parts[2]

			// Run the agent with the modification message
			modifyMsg := fmt.Sprintf("Please modify the execution plan for task %s as follows: %s", taskID, instructions)
			startTime := time.Now()
			response, err := agentInstance.Run(ctx, modifyMsg)
			processTime := time.Since(startTime)

			// Log the processing time
			logger.Info(ctx, "Agent processed modification", map[string]interface{}{
				"processing_time_ms": processTime.Milliseconds(),
				"task_id":            taskID,
			})

			// Check for errors
			if err != nil {
				logger.Error(ctx, "Error modifying execution plan", map[string]interface{}{
					"error":   err.Error(),
					"task_id": taskID,
				})
				fmt.Printf("Error: %s\n", err.Error())
				continue
			}

			// Print the response
			fmt.Printf("Agent: %s\n", response)
			continue
		}

		// For regular messages, run the agent
		startTime := time.Now()
		response, err := agentInstance.Run(ctx, userInput)
		processTime := time.Since(startTime)

		// Log the processing time
		logger.Info(ctx, "Agent processed query", map[string]interface{}{
			"processing_time_ms": processTime.Milliseconds(),
		})

		// Check for errors
		if err != nil {
			logger.Error(ctx, "Error running agent", map[string]interface{}{
				"error": err.Error(),
			})
			fmt.Printf("Error: %s\n", err.Error())
			continue
		}

		// Check if we have a pending plan (format the output nicely)
		pendingPlans := getPendingPlans(agentInstance)
		if len(pendingPlans) > 0 {
			// The latest pending plan is likely the one just created
			latestPlan := pendingPlans[len(pendingPlans)-1]
			prettyPrintPlan(latestPlan)
		}

		// Print the response
		fmt.Printf("Agent: %s\n", response)
	}
}

// Get all pending plans from the agent
func getPendingPlans(agentInstance *agent.Agent) []*executionplan.ExecutionPlan {
	allPlans := agentInstance.ListTasks()
	pendingPlans := make([]*executionplan.ExecutionPlan, 0)

	for _, plan := range allPlans {
		if plan.Status == executionplan.StatusPendingApproval {
			pendingPlans = append(pendingPlans, plan)
		}
	}

	return pendingPlans
}

// Pretty print an execution plan
func prettyPrintPlan(plan *executionplan.ExecutionPlan) {
	fmt.Println("\n=== Execution Plan ===")
	fmt.Printf("Task ID: %s\n", plan.TaskID)
	fmt.Printf("Description: %s\n", plan.Description)
	fmt.Printf("Status: %s\n", plan.Status)
	fmt.Println("\nSteps:")

	for i, step := range plan.Steps {
		fmt.Printf("%d. %s\n", i+1, step.Description)

		// If we have tool parameters, pretty print them
		if step.ToolName != "" {
			fmt.Printf("   Tool: %s\n", step.ToolName)

			// Pretty print the parameters if we have them
			if step.Parameters != nil && len(step.Parameters) > 0 {
				// Try to serialize the parameters map to JSON for display
				paramsJSON, err := json.MarshalIndent(step.Parameters, "   ", "  ")
				if err == nil {
					fmt.Println("   Parameters:")
					fmt.Println(string(paramsJSON))
				} else {
					// Fall back to displaying the raw parameters map
					fmt.Println("   Parameters:")
					for key, value := range step.Parameters {
						fmt.Printf("   - %s: %v\n", key, value)
					}
				}
			}
		}
	}

	fmt.Println("\nTo approve this plan, type: approve", plan.TaskID)
	fmt.Println("To modify this plan, type: modify", plan.TaskID, "<your instructions>")
	fmt.Println("======================")
}
