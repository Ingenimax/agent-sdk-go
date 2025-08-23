package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

// WeatherTool is a mock weather tool for demonstration
type WeatherTool struct{}

func (t *WeatherTool) Name() string {
	return "get_weather"
}

func (t *WeatherTool) Description() string {
	return "Get current weather information for any location worldwide"
}

func (t *WeatherTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"location": {
			Type:        "string",
			Description: "The city or location to get weather for",
			Required:    true,
		},
		"units": {
			Type:        "string",
			Description: "Temperature units",
			Required:    false,
			Default:     "celsius",
			Enum:        []interface{}{"celsius", "fahrenheit"},
		},
	}
}

func (t *WeatherTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *WeatherTool) Execute(ctx context.Context, args string) (string, error) {
	// Mock weather response
	return "Current weather: Partly cloudy, 23°C, humidity 65%, wind 8 km/h from the west", nil
}

func main() {
	fmt.Println("🤖 Gemini Agent Integration Example")
	fmt.Println("===================================")
	fmt.Println()

	// Get API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY environment variable is required")
	}

	ctx := context.Background()

	// Create Gemini LLM client
	geminiClient, err := gemini.NewClient(
		apiKey,
		gemini.WithModel(gemini.ModelGemini25Flash),
	)
	if err != nil {
		log.Fatalf("Failed to create Gemini client: %v", err)
	}

	fmt.Printf("Created Gemini client with model: %s\n\n", geminiClient.GetModel())

	// Create tools
	tools := []interfaces.Tool{
		calculator.New(),
		&WeatherTool{},
	}

	// Create agent with Gemini LLM
	myAgent, err := agent.NewAgent(
		agent.WithLLM(geminiClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(tools...),
		agent.WithSystemPrompt("You are a helpful AI assistant with access to weather information and calculation tools. Always be informative and helpful."),
		agent.WithName("GeminiAssistant"),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Create context with organization and conversation IDs for multi-tenancy
	ctx = multitenancy.WithOrgID(ctx, "example-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "gemini-demo-conversation")

	fmt.Println("=== Example 1: Simple Query ===")
	response, err := myAgent.Run(ctx, "Hello! What can you help me with today?")
	if err != nil {
		log.Fatalf("Failed to run agent: %v", err)
	}
	fmt.Printf("Agent: %s\n\n", response)

	fmt.Println("=== Example 2: Weather Query (Tool Usage) ===")
	response, err = myAgent.Run(ctx, "What's the weather like in New York?")
	if err != nil {
		log.Fatalf("Failed to run agent: %v", err)
	}
	fmt.Printf("Agent: %s\n\n", response)

	fmt.Println("=== Example 3: Math Calculation (Tool Usage) ===")
	response, err = myAgent.Run(ctx, "Can you calculate the compound interest for $1000 at 5% annual rate for 3 years?")
	if err != nil {
		log.Fatalf("Failed to run agent: %v", err)
	}
	fmt.Printf("Agent: %s\n\n", response)

	fmt.Println("=== Example 4: Complex Query (Multiple Tools) ===")
	response, err = myAgent.Run(ctx, "I'm planning a trip to Tokyo. Can you tell me the weather there and also calculate how much 500 USD would be if converted at a rate of 150 yen per dollar?")
	if err != nil {
		log.Fatalf("Failed to run agent: %v", err)
	}
	fmt.Printf("Agent: %s\n\n", response)

	fmt.Println("=== Example 5: Memory Test (Follow-up Question) ===")
	response, err = myAgent.Run(ctx, "Based on our previous conversation, which city did I ask about the weather for?")
	if err != nil {
		log.Fatalf("Failed to run agent: %v", err)
	}
	fmt.Printf("Agent: %s\n\n", response)

	fmt.Println("=== Example 6: Streaming Agent Response ===")
	fmt.Print("Streaming Agent Response: ")

	// Note: This requires the agent to support streaming, which may need additional implementation
	// For now, we'll demonstrate the concept with the LLM directly

	stream, err := geminiClient.GenerateWithToolsStream(ctx,
		"Tell me a story about a robot who learns to use tools, and also calculate what 25 * 17 equals somewhere in the story",
		tools,
		gemini.WithSystemMessage("You are a creative storyteller who can use tools when needed in your stories."),
	)
	if err != nil {
		log.Fatalf("Failed to create stream: %v", err)
	}

	for event := range stream {
		switch event.Type {
		case interfaces.StreamEventContentDelta:
			fmt.Print(event.Content)
		case interfaces.StreamEventToolUse:
			if event.ToolCall != nil {
				fmt.Printf(" [Using %s] ", event.ToolCall.Name)
			}
		case interfaces.StreamEventError:
			fmt.Printf("\nError: %v\n", event.Error)
			break
		case interfaces.StreamEventMessageStop:
			fmt.Println("\n[Story completed]")
		}
	}
	fmt.Println()

	fmt.Println("=== Example 7: Reasoning Mode Demonstration ===")

	// Create agents with different reasoning modes
	comprehensiveAgent, err := agent.NewAgent(
		agent.WithLLM(geminiClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(calculator.New()),
		agent.WithSystemPrompt("You are a math tutor. Explain your reasoning step by step."),
		agent.WithName("ComprehensiveGeminiTutor"),
	)
	if err != nil {
		log.Fatalf("Failed to create comprehensive agent: %v", err)
	}

	minimalAgent, err := agent.NewAgent(
		agent.WithLLM(geminiClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(calculator.New()),
		agent.WithSystemPrompt("You are a quick math helper. Give brief explanations."),
		agent.WithName("MinimalGeminiHelper"),
	)
	if err != nil {
		log.Fatalf("Failed to create minimal agent: %v", err)
	}

	mathProblem := "Solve this word problem: Sarah has 24 apples. She gives away 1/3 of them to her friends and then buys 15 more. How many apples does she have now?"

	fmt.Println("Comprehensive reasoning:")
	response, err = comprehensiveAgent.Run(ctx, mathProblem)
	if err != nil {
		log.Printf("Error with comprehensive agent: %v", err)
	} else {
		fmt.Printf("%s\n\n", response)
	}

	fmt.Println("Minimal reasoning:")
	response, err = minimalAgent.Run(ctx, mathProblem)
	if err != nil {
		log.Printf("Error with minimal agent: %v", err)
	} else {
		fmt.Printf("%s\n\n", response)
	}

	fmt.Println("=== Example 8: Structured Output with Agent ===")

	// For structured output, we'll use the LLM directly as agent integration
	// with structured output may require additional implementation
	schema := interfaces.JSONSchema{
		"type": "object",
		"properties": map[string]interface{}{
			"task_analysis": map[string]interface{}{
				"type":        "string",
				"description": "Analysis of the task requested",
			},
			"tools_needed": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"description": "List of tools that would be needed",
			},
			"estimated_steps": map[string]interface{}{
				"type":        "integer",
				"description": "Estimated number of steps to complete",
			},
			"complexity": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"low", "medium", "high"},
				"description": "Task complexity level",
			},
		},
		"required": []string{"task_analysis", "tools_needed", "estimated_steps", "complexity"},
	}

	structuredResponse, err := geminiClient.Generate(ctx,
		"Analyze this task: Plan a birthday party for 20 people including venue, catering, and entertainment",
		gemini.WithResponseFormat(interfaces.ResponseFormat{
			Type:   interfaces.ResponseFormatJSON,
			Name:   "TaskAnalysis",
			Schema: schema,
		}),
		gemini.WithSystemMessage("You are a professional event planner analyzing tasks."),
	)
	if err != nil {
		log.Printf("Error with structured output: %v", err)
	} else {
		fmt.Printf("Structured task analysis: %s\n\n", structuredResponse)
	}

	fmt.Println("✅ Gemini agent integration examples completed!")
	fmt.Println("\nKey features demonstrated:")
	fmt.Println("- Agent creation with Gemini LLM")
	fmt.Println("- Tool integration (calculator, weather)")
	fmt.Println("- Memory and conversation continuity")
	fmt.Println("- Multi-tenancy support")
	fmt.Println("- Streaming capabilities")
	fmt.Println("- Different reasoning modes")
	fmt.Println("- Structured output generation")
}