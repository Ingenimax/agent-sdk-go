package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/executionplan"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

// SimpleTool is a basic tool for demonstration purposes
type SimpleTool struct {
	name        string
	description string
}

// Name returns the name of the tool
func (t *SimpleTool) Name() string {
	return t.name
}

// Description returns a description of what the tool does
func (t *SimpleTool) Description() string {
	return t.description
}

// Run executes the tool with the given input
func (t *SimpleTool) Run(ctx context.Context, input string) (string, error) {
	return fmt.Sprintf("Executed %s with input: %s", t.name, input), nil
}

// Parameters returns the parameters that the tool accepts
func (t *SimpleTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"param1": {
			Type:        "string",
			Description: "A sample parameter",
			Required:    true,
		},
		"param2": {
			Type:        "number",
			Description: "Another sample parameter",
			Required:    false,
			Default:     42,
		},
	}
}

// Execute executes the tool with the given arguments
func (t *SimpleTool) Execute(ctx context.Context, args string) (string, error) {
	return fmt.Sprintf("Executed %s with arguments: %s", t.name, args), nil
}

func main() {
	// Create a context
	ctx := context.Background()

	// In a real application, you would initialize an LLM here
	// For example:
	// openaiLLM, err := llm.NewOpenAI(os.Getenv("OPENAI_API_KEY"), "gpt-4")
	// For this example, we'll use a mock LLM
	mockLLM := &MockLLM{}

	// Create tools
	toolRegistry := tools.NewRegistry()

	// Register some tools
	calcTool := calculator.NewCalculator()
	toolRegistry.Register(calcTool)

	// Register some simple tools for demonstration
	toolRegistry.Register(&SimpleTool{
		name:        "weather",
		description: "Get the weather for a location",
	})

	toolRegistry.Register(&SimpleTool{
		name:        "search",
		description: "Search the web for information",
	})

	// Create an agent with the LLM and tools
	myAgent, err := agent.NewAgent(
		agent.WithLLM(mockLLM),
		agent.WithTools(toolRegistry.List()...),
		agent.WithRequirePlanApproval(true), // Enable execution plan approval
	)
	if err != nil {
		fmt.Println("Error creating agent:", err)
		os.Exit(1)
	}

	// Create a scanner for user input
	scanner := bufio.NewScanner(os.Stdin)

	// Start the conversation
	fmt.Println("Welcome to the Execution Plan Example!")
	fmt.Println("Type 'exit' to quit.")
	fmt.Println()

	// Keep track of the current execution plan
	var currentPlan *executionplan.ExecutionPlan

	for {
		fmt.Print("User: ")
		if !scanner.Scan() {
			break
		}

		userInput := scanner.Text()
		if userInput == "exit" {
			break
		}

		// Check if the user is approving or modifying a plan
		if currentPlan != nil {
			if strings.HasPrefix(strings.ToLower(userInput), "approve") || strings.HasPrefix(strings.ToLower(userInput), "yes") {
				// User approves the plan
				fmt.Println("\nAgent: Executing the approved plan...")
				result, err := myAgent.ApproveExecutionPlan(ctx, currentPlan)
				if err != nil {
					fmt.Println("Error executing plan:", err)
					continue
				}
				fmt.Println(result)
				currentPlan = nil
				continue
			} else if strings.HasPrefix(strings.ToLower(userInput), "modify") || strings.Contains(strings.ToLower(userInput), "change") {
				// User wants to modify the plan
				fmt.Println("\nAgent: Modifying the plan based on your feedback...")
				modifiedPlan, err := myAgent.ModifyExecutionPlan(ctx, currentPlan, userInput)
				if err != nil {
					fmt.Println("Error modifying plan:", err)
					continue
				}
				currentPlan = modifiedPlan
				fmt.Println("I've updated the execution plan based on your feedback:")
				fmt.Println(executionplan.FormatExecutionPlan(currentPlan))
				fmt.Println("Do you approve this plan? You can modify it further if needed.")
				continue
			} else if strings.HasPrefix(strings.ToLower(userInput), "cancel") || strings.HasPrefix(strings.ToLower(userInput), "no") {
				// User cancels the plan
				fmt.Println("\nAgent: Plan cancelled. What would you like to do instead?")
				currentPlan = nil
				continue
			}
		}

		// Process the user input
		response, err := myAgent.Run(ctx, userInput)
		if err != nil {
			fmt.Println("Error:", err)
			continue
		}

		fmt.Println("\nAgent:", response)

		// Check if the response contains an execution plan
		if strings.Contains(response, "Execution Plan:") {
			// Extract the execution plan from the response
			// This is a simplistic approach; in a real implementation, you would use a more robust method
			fmt.Println("\nWould you like to approve, modify, or cancel this plan?")

			// Generate an execution plan
			fmt.Println("\nAgent: Generating an execution plan for your request...")
			plan, err := myAgent.GenerateExecutionPlan(ctx, userInput)
			if err != nil {
				fmt.Println("Error generating plan:", err)
				continue
			}

			// Store the plan and show it to the user
			currentPlan = plan
			fmt.Println("I've created an execution plan for your request:")
			fmt.Println(executionplan.FormatExecutionPlan(plan))
			fmt.Println("Do you approve this plan? You can modify it if needed.")
		}
	}

	fmt.Println("Goodbye!")
}

// MockLLM is a mock implementation of the LLM interface for demonstration purposes
type MockLLM struct{}

// Generate generates text based on the provided prompt
func (m *MockLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// For demonstration purposes, return a simple execution plan
	if strings.Contains(prompt, "Create an execution plan") {
		return `{
			"description": "Search for weather information and calculate temperature conversion",
			"steps": [
				{
					"toolName": "search",
					"description": "Search for current weather information",
					"input": "current weather in New York",
					"parameters": {
						"param1": "weather"
					}
				},
				{
					"toolName": "calculator",
					"description": "Convert temperature from Fahrenheit to Celsius",
					"input": "convert 75F to C",
					"parameters": {}
				}
			]
		}`, nil
	}

	// For plan modification, return a modified plan
	if strings.Contains(prompt, "modify the execution plan") {
		return `{
			"description": "Search for weather information and calculate temperature conversion",
			"steps": [
				{
					"toolName": "weather",
					"description": "Get current weather information directly",
					"input": "New York",
					"parameters": {
						"param1": "New York"
					}
				},
				{
					"toolName": "calculator",
					"description": "Convert temperature from Fahrenheit to Celsius",
					"input": "convert 75F to C",
					"parameters": {}
				}
			]
		}`, nil
	}

	return "I'll help you with that. Let me create an execution plan.", nil
}

// GenerateWithTools generates text and can use tools
func (m *MockLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	return m.Generate(ctx, prompt, options...)
}

// Name returns the name of the LLM provider
func (m *MockLLM) Name() string {
	return "MockLLM"
}
