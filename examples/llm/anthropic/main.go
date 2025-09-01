package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

// This example demonstrates how to use the Anthropic Claude models with direct API.
// For Vertex AI usage, see the vertex/ subdirectory.
//
// Before running, set the following environment variables:
//
// Required:
// - ANTHROPIC_API_KEY: Your Anthropic API key
//
// Optional:
// - ANTHROPIC_MODEL: The Claude model to use (e.g., "claude-3-haiku-20240307", "claude-3-7-sonnet-20240307")
// - ANTHROPIC_TEMPERATURE: Temperature setting (default: 0.7)
// - ANTHROPIC_TIMEOUT: Request timeout in seconds (default: 60)
// - ANTHROPIC_BASE_URL: Custom API endpoint URL (optional)
//
// Example setup:
// export ANTHROPIC_API_KEY=your_api_key_here
// export ANTHROPIC_MODEL=claude-3-7-sonnet-20240307
// export ANTHROPIC_TEMPERATURE=0.5
// go run main.go
//
// For Vertex AI usage:
// cd vertex/
// export GOOGLE_CLOUD_PROJECT=your-project-id
// export GOOGLE_CLOUD_REGION=us-central1
// gcloud auth application-default login
// go run main.go

// getEnvWithDefault gets an environment variable or returns the default value
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvFloatWithDefault gets a float environment variable or returns the default value
func getEnvFloatWithDefault(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getEnvIntWithDefault gets an int environment variable or returns the default value
func getEnvIntWithDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func main() {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("ANTHROPIC_API_KEY environment variable is required")
		os.Exit(1)
	}

	// Get model from environment or use Claude3Haiku as default
	model := getEnvWithDefault("ANTHROPIC_MODEL", anthropic.Claude37Sonnet)
	// Ensure we have a model specified
	if model == "" {
		fmt.Println("ANTHROPIC_MODEL must be specified")
		os.Exit(1)
	}

	// Get other configuration from environment variables
	baseURL := getEnvWithDefault("ANTHROPIC_BASE_URL", "https://api.anthropic.com")
	timeout := getEnvIntWithDefault("ANTHROPIC_TIMEOUT", 60)
	temperature := getEnvFloatWithDefault("ANTHROPIC_TEMPERATURE", 0.7)

	// Create a new context
	ctx := context.Background()

	fmt.Println("Anthropic Claude Examples")
	fmt.Println("========================")
	fmt.Println()

	// Example 1: Basic generation with environment variables
	fmt.Println("Example 1: Basic generation with environment variables")
	fmt.Println("----------------------------------------------------")
	basicGeneration(ctx, apiKey, model, baseURL, timeout, temperature)
	fmt.Println()

	// Example 2: Using Claude-3.7-Sonnet with system messages
	fmt.Println("Example 2: Using Claude-3.7-Sonnet with system messages")
	fmt.Println("-----------------------------------------------------")
	claudeSonnetGeneration(ctx, apiKey, baseURL, timeout)
	fmt.Println()

	// Example 3: Using reasoning parameter (note: officially unsupported but Claude still gives step-by-step solutions)
	fmt.Println("Example 3: Using reasoning parameter (note: officially unsupported but Claude still gives step-by-step solutions)")
	fmt.Println("----------------------------------------------------------------------------------------")
	reasoningGeneration(ctx, apiKey, model, baseURL, timeout)
	fmt.Println()

	// Example 4: Using tools
	fmt.Println("Example 4: Using tools")
	fmt.Println("---------------------")
	toolUsage(ctx, apiKey, model, baseURL, timeout)
	fmt.Println()

	// Example 5: Creating an agent with Anthropic
	fmt.Println("Example 5: Creating an agent with Anthropic")
	fmt.Println("------------------------------------------")
	createAgent(ctx, apiKey, model, baseURL, timeout)
	fmt.Println()
}

func basicGeneration(ctx context.Context, apiKey, model, baseURL string, timeout int, temperature float64) {
	// Create Anthropic client with configuration explicitly provided
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(model), // Model must be specified explicitly
		anthropic.WithBaseURL(baseURL),
		anthropic.WithHTTPClient(&http.Client{Timeout: time.Duration(timeout) * time.Second}),
	)

	// Generate text
	response, err := client.Generate(
		ctx,
		"What is the capital of France?",
		anthropic.WithTemperature(temperature),
	)
	if err != nil {
		fmt.Printf("Error generating text: %v\n", err)
		return
	}

	// Print response
	fmt.Printf("Response: %s\n", response)
}

func claudeSonnetGeneration(ctx context.Context, apiKey, baseURL string, timeout int) {
	// Create Anthropic client with Claude-3.7-Sonnet model explicitly specified
	client := anthropic.NewClient(
		apiKey,
		// Always specify the model explicitly
		anthropic.WithModel(anthropic.Claude37Sonnet),
		anthropic.WithBaseURL(baseURL),
		anthropic.WithHTTPClient(&http.Client{Timeout: time.Duration(timeout) * time.Second}),
	)

	// Generate text with a system message
	response, err := client.Generate(
		ctx,
		"Tell me a short story about a robot",
		anthropic.WithSystemMessage("You are a creative writing assistant. Keep your responses under 100 words."),
		anthropic.WithTemperature(0.8),
	)
	if err != nil {
		fmt.Printf("Error generating text: %v\n", err)
		return
	}

	// Print response
	fmt.Printf("Response: %s\n", response)
}

func reasoningGeneration(ctx context.Context, apiKey, model, baseURL string, timeout int) {
	// Create Anthropic client with model explicitly specified
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(model), // Model must be specified explicitly
		anthropic.WithBaseURL(baseURL),
		anthropic.WithHTTPClient(&http.Client{Timeout: time.Duration(timeout) * time.Second}),
	)

	// Generate text with comprehensive reasoning
	response, err := client.Generate(
		ctx,
		"How would you solve this equation: 3x + 7 = 22?",
		anthropic.WithReasoning("comprehensive"),
	)
	if err != nil {
		fmt.Printf("Error generating text: %v\n", err)
		return
	}

	// Print response
	fmt.Printf("Response: %s\n", response)
}

func toolUsage(ctx context.Context, apiKey, model, baseURL string, timeout int) {
	// Add organization ID to context for tools
	ctx = multitenancy.WithOrgID(ctx, "example-org-id")

	// Create Anthropic client with model explicitly specified
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(model), // Model must be specified explicitly
		anthropic.WithBaseURL(baseURL),
		anthropic.WithHTTPClient(&http.Client{Timeout: time.Duration(timeout) * time.Second}),
	)

	// Create calculator tool
	calculatorTool := calculator.New()

	// Generate text with tools
	response, err := client.GenerateWithTools(
		ctx,
		"What is 123 multiplied by 456, and can you add 789 to the result?",
		[]interfaces.Tool{calculatorTool},
		anthropic.WithSystemMessage("You are a helpful assistant. Use tools when appropriate."),
		anthropic.WithTemperature(0.2),
	)
	if err != nil {
		fmt.Printf("Error generating text with tools: %v\n", err)

		// Fallback to regular generation if tools fail
		fmt.Println("Falling back to regular generation...")
		fallbackResponse, fallbackErr := client.Generate(
			ctx,
			"What is 123 multiplied by 456, and can you add 789 to the result? Please calculate the answer step by step.",
			anthropic.WithSystemMessage("You are a helpful assistant skilled at mathematical calculations."),
			anthropic.WithTemperature(0.2),
		)
		if fallbackErr != nil {
			fmt.Printf("Fallback also failed: %v\n", fallbackErr)
			return
		}
		response = fallbackResponse
	}

	// Print response
	fmt.Printf("Response: %s\n", response)
}

func createAgent(ctx context.Context, apiKey, model, baseURL string, timeout int) {
	// Add organization ID to context for the agent
	ctx = multitenancy.WithOrgID(ctx, "example-org-id")

	// Add a conversation ID
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "example-conversation")

	// Create Anthropic client with model explicitly specified
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(model), // Model must be specified explicitly
		anthropic.WithBaseURL(baseURL),
		anthropic.WithHTTPClient(&http.Client{Timeout: time.Duration(timeout) * time.Second}),
	)

	// Create a memory store
	memoryStore := memory.NewConversationBuffer()

	// Create a new agent with Anthropic
	agentInstance, err := agent.NewAgent(
		agent.WithLLM(client),
		agent.WithMemory(memoryStore),
		agent.WithSystemPrompt("You are a helpful AI assistant specialized in answering science questions."),
	)
	if err != nil {
		fmt.Printf("Failed to create agent: %v\n", err)
		return
	}

	// Run the agent
	response, err := agentInstance.Run(ctx, "What is the difference between mitosis and meiosis?")
	if err != nil {
		fmt.Printf("Failed to run agent: %v\n", err)
		return
	}

	// Print response
	fmt.Printf("Agent response: %s\n", response)
}
