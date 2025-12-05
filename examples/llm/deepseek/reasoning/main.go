package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/deepseek"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		log.Fatal("DEEPSEEK_API_KEY environment variable is required")
	}

	// Create DeepSeek client with reasoning model
	client := deepseek.NewClient(
		apiKey,
		deepseek.WithModel("deepseek-reasoner"),
	)

	ctx := context.Background()

	fmt.Println("=== DeepSeek Reasoning Mode Example ===\n")
	fmt.Println("Using model: deepseek-reasoner (DeepSeek-R1)")
	fmt.Println("This model provides enhanced reasoning capabilities for complex problems.\n")

	// Example 1: Mathematical reasoning
	fmt.Println("--- Example 1: Mathematical Reasoning ---")
	mathProblem := `A farmer has 17 sheep, and all but 9 die. How many sheep are left?
Think through this step-by-step and explain your reasoning.`

	response, err := client.GenerateDetailed(
		ctx,
		mathProblem,
		interfaces.WithTemperature(0.1), // Lower temperature for more focused reasoning
	)
	if err != nil {
		log.Fatalf("Generation error: %v", err)
	}

	fmt.Printf("Question: %s\n", mathProblem)
	fmt.Printf("\nAnswer: %s\n", response.Content)
	fmt.Printf("\nToken Usage:\n")
	fmt.Printf("  Input: %d tokens\n", response.Usage.InputTokens)
	fmt.Printf("  Output: %d tokens\n", response.Usage.OutputTokens)
	fmt.Printf("  Total: %d tokens\n\n", response.Usage.TotalTokens)

	// Example 2: Logical reasoning
	fmt.Println("--- Example 2: Logical Reasoning ---")
	logicProblem := `If all bloops are razzies and all razzies are lazzies, are all bloops definitely lazzies?
Explain your logical reasoning process.`

	responseText, err := client.Generate(
		ctx,
		logicProblem,
		interfaces.WithTemperature(0.1),
	)
	if err != nil {
		log.Fatalf("Generation error: %v", err)
	}

	fmt.Printf("Question: %s\n", logicProblem)
	fmt.Printf("\nAnswer: %s\n\n", responseText)

	// Example 3: Complex problem-solving
	fmt.Println("--- Example 3: Complex Problem Solving ---")
	complexProblem := `You have a 3-gallon jug and a 5-gallon jug.
How can you measure exactly 4 gallons of water?
Provide a step-by-step solution with clear reasoning.`

	response, err = client.GenerateDetailed(
		ctx,
		complexProblem,
		interfaces.WithTemperature(0.2),
		interfaces.WithSystemMessage("You are a logical problem solver. Break down complex problems into clear steps."),
	)
	if err != nil {
		log.Fatalf("Generation error: %v", err)
	}

	fmt.Printf("Problem: %s\n", complexProblem)
	fmt.Printf("\nSolution: %s\n", response.Content)
	fmt.Printf("\nToken Usage: %d input + %d output = %d total\n",
		response.Usage.InputTokens,
		response.Usage.OutputTokens,
		response.Usage.TotalTokens)

	fmt.Println("\n=== Reasoning Mode Demo Complete ===")
	fmt.Println("\nNote: The deepseek-reasoner model excels at:")
	fmt.Println("  - Step-by-step reasoning")
	fmt.Println("  - Complex mathematical problems")
	fmt.Println("  - Logical deduction")
	fmt.Println("  - Multi-step problem solving")
}
