package main

import (
	"context"
	"fmt"
	"log"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
)

func main() {
	// Create Anthropic client (pass empty string to use ANTHROPIC_API_KEY env var)
	client := anthropic.NewClient("",
		anthropic.WithModel("claude-3-haiku-20240307"),
	)

	ctx := context.Background()
	prompt := "Explain the concept of recursion in programming in one paragraph."

	// Example 1: Traditional method (returns only content)
	fmt.Println("=== Traditional Generate Method ===")
	content, err := client.Generate(ctx, prompt)
	if err != nil {
		log.Fatalf("Error with traditional method: %v", err)
	}
	fmt.Printf("Response: %s\n\n", content)

	// Example 2: Detailed method (returns content + token usage)
	fmt.Println("=== New GenerateDetailed Method ===")
	response, err := client.GenerateDetailed(ctx, prompt)
	if err != nil {
		log.Fatalf("Error with detailed method: %v", err)
	}

	fmt.Printf("Response: %s\n", response.Content)
	fmt.Printf("Model: %s\n", response.Model)
	fmt.Printf("Stop Reason: %s\n", response.StopReason)

	if response.Usage != nil {
		fmt.Printf("\n--- Token Usage ---\n")
		fmt.Printf("Input Tokens: %d\n", response.Usage.InputTokens)
		fmt.Printf("Output Tokens: %d\n", response.Usage.OutputTokens)
		fmt.Printf("Total Tokens: %d\n", response.Usage.TotalTokens)
		if response.Usage.ReasoningTokens > 0 {
			fmt.Printf("Reasoning Tokens: %d\n", response.Usage.ReasoningTokens)
		}

		// Calculate estimated cost (example pricing - adjust based on actual Anthropic pricing)
		inputCost := float64(response.Usage.InputTokens) * 0.25 / 1000000   // $0.25 per 1M input tokens
		outputCost := float64(response.Usage.OutputTokens) * 1.25 / 1000000 // $1.25 per 1M output tokens
		totalCost := inputCost + outputCost
		fmt.Printf("Estimated Cost: $%.6f\n", totalCost)
	} else {
		fmt.Println("Token usage information not available")
	}

	if response.Metadata != nil {
		fmt.Printf("\n--- Metadata ---\n")
		for key, value := range response.Metadata {
			fmt.Printf("%s: %v\n", key, value)
		}
	}

	// Example 3: With tools (demonstrating tools + token usage)
	fmt.Println("\n=== GenerateWithToolsDetailed Method ===")
	tools := []interfaces.Tool{
		// Add a simple mock tool for demonstration
		&MockCalculatorTool{},
	}

	toolPrompt := "What's 25 * 4? Use the calculator tool to compute this."
	toolResponse, err := client.GenerateWithToolsDetailed(ctx, toolPrompt, tools)
	if err != nil {
		log.Fatalf("Error with tools detailed method: %v", err)
	}

	fmt.Printf("Response: %s\n", toolResponse.Content)
	if toolResponse.Usage != nil {
		fmt.Printf("Total Tokens Used: %d\n", toolResponse.Usage.TotalTokens)
	}

	fmt.Println("\n=== Summary ===")
	fmt.Println("✅ Token usage tracking successfully implemented!")
	fmt.Println("✅ Backward compatibility maintained - existing code continues to work")
	fmt.Println("✅ New detailed methods provide rich response information")
	fmt.Println("✅ Usage information enables cost tracking and optimization")
}

// MockCalculatorTool is a simple tool for demonstration
type MockCalculatorTool struct{}

func (t *MockCalculatorTool) Name() string {
	return "calculator"
}

func (t *MockCalculatorTool) Description() string {
	return "Performs basic arithmetic calculations"
}

func (t *MockCalculatorTool) Run(ctx context.Context, input string) (string, error) {
	return "100", nil // Mock result for 25 * 4
}

func (t *MockCalculatorTool) Execute(ctx context.Context, input string) (string, error) {
	return "100", nil // Mock result for 25 * 4
}

func (t *MockCalculatorTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"expression": {
			Type:        "string",
			Description: "The mathematical expression to evaluate",
			Required:    true,
		},
	}
}