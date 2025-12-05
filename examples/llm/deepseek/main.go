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

	// Create DeepSeek client
	client := deepseek.NewClient(
		apiKey,
		deepseek.WithModel("deepseek-chat"),
	)

	ctx := context.Background()

	// Example 1: Simple generation
	fmt.Println("=== Example 1: Simple Generation ===")
	response, err := client.Generate(ctx, "What is the capital of France?")
	if err != nil {
		log.Fatalf("Generation error: %v", err)
	}
	fmt.Printf("Response: %s\n\n", response)

	// Example 2: Detailed response with token usage
	fmt.Println("=== Example 2: Detailed Response ===")
	detailedResp, err := client.GenerateDetailed(
		ctx,
		"Explain quantum computing in simple terms",
		interfaces.WithTemperature(0.7),
		interfaces.WithSystemMessage("You are a helpful AI assistant that explains complex topics simply."),
	)
	if err != nil {
		log.Fatalf("Generation error: %v", err)
	}
	fmt.Printf("Content: %s\n", detailedResp.Content)
	fmt.Printf("Model: %s\n", detailedResp.Model)
	fmt.Printf("Input Tokens: %d\n", detailedResp.Usage.InputTokens)
	fmt.Printf("Output Tokens: %d\n", detailedResp.Usage.OutputTokens)
	fmt.Printf("Total Tokens: %d\n\n", detailedResp.Usage.TotalTokens)

	// Example 3: With various options
	fmt.Println("=== Example 3: With Configuration Options ===")
	response, err = client.Generate(
		ctx,
		"Write a short poem about AI",
		interfaces.WithTemperature(0.9),
		interfaces.WithTopP(0.95),
		interfaces.WithSystemMessage("You are a creative poet."),
	)
	if err != nil {
		log.Fatalf("Generation error: %v", err)
	}
	fmt.Printf("Poem:\n%s\n", response)
}
