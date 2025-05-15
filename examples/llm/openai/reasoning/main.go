package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func main() {
	// Create a logger
	logger := logging.New()
	ctx := context.Background()

	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		logger.Error(ctx, "OPENAI_API_KEY environment variable is required", nil)
		os.Exit(1)
	}

	// Create client
	client := openai.NewClient(
		apiKey,
		openai.WithModel("gpt-4o-mini"),
		openai.WithLogger(logger),
	)

	// Question that requires reasoning
	question := "What is the probability of rolling a sum of 7 with two standard dice?"

	// Test with no reasoning (direct answer)
	fmt.Println("\n=== No Reasoning ===")
	resp, err := client.Generate(
		ctx,
		question,
		openai.WithReasoning("none"),
		openai.WithTemperature(0.2),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	fmt.Println(resp)

	// Test with minimal reasoning
	fmt.Println("\n=== Minimal Reasoning ===")
	resp, err = client.Generate(
		ctx,
		question,
		openai.WithReasoning("minimal"),
		openai.WithTemperature(0.2),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	fmt.Println(resp)

	// Test with comprehensive reasoning
	fmt.Println("\n=== Comprehensive Reasoning ===")
	resp, err = client.Generate(
		ctx,
		question,
		openai.WithReasoning("comprehensive"),
		openai.WithTemperature(0.2),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	fmt.Println(resp)

	// Demonstrate reasoning with system message
	fmt.Println("\n=== Reasoning with System Message ===")
	resp, err = client.Generate(
		ctx,
		question,
		openai.WithSystemMessage("You are a mathematics tutor for high school students."),
		openai.WithReasoning("comprehensive"),
		openai.WithTemperature(0.2),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	fmt.Println(resp)

	// Demonstrate reasoning with Chat method
	fmt.Println("\n=== Reasoning with Chat API ===")
	messages := []llm.Message{
		{
			Role:    "system",
			Content: "You are a helpful assistant who explains concepts clearly.",
		},
		{
			Role:    "user",
			Content: "Explain the concept of recursion in programming.",
		},
	}

	resp, err = client.Chat(
		ctx,
		messages,
		&llm.GenerateParams{
			Temperature: 0.3,
			Reasoning:   "comprehensive",
		},
	)
	if err != nil {
		logger.Error(ctx, "Failed to chat", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	fmt.Println(resp)
}
