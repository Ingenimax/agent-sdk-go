package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/vllm"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
)

func main() {
	// Create a logger
	logger := logging.New()
	ctx := context.Background()

	// Get vLLM base URL from environment or use default
	baseURL := os.Getenv("VLLM_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}

	// Get model from environment or use default
	model := os.Getenv("VLLM_MODEL")
	if model == "" {
		model = "llama-2-7b"
	}

	// Create vLLM client with retry configuration
	client := vllm.NewClient(
		vllm.WithModel(model),
		vllm.WithLogger(logger),
		vllm.WithBaseURL(baseURL),
		vllm.WithRetry(
			retry.WithMaxAttempts(3),
			retry.WithInitialInterval(1),
			retry.WithBackoffCoefficient(2.0),
			retry.WithMaximumInterval(30),
		),
	)

	// Test 1: Basic text generation
	fmt.Println("=== Test 1: Basic Text Generation ===")
	resp, err := client.Generate(
		ctx,
		"Write a haiku about programming",
		vllm.WithTemperature(0.7),
		vllm.WithSystemMessage("You are a creative assistant who specializes in writing haikus."),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate text", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("Generated haiku:\n%s\n\n", resp)
	}

	// Test 2: Chat conversation
	fmt.Println("=== Test 2: Chat Conversation ===")
	messages := []llm.Message{
		{
			Role:    "system",
			Content: "You are a helpful programming assistant.",
		},
		{
			Role:    "user",
			Content: "What's the best way to handle errors in Go?",
		},
	}

	resp, err = client.Chat(ctx, messages, &llm.GenerateParams{
		Temperature: 0.5,
	})
	if err != nil {
		logger.Error(ctx, "Failed to chat", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("Chat response:\n%s\n\n", resp)
	}

	// Test 3: Multi-turn conversation
	fmt.Println("=== Test 3: Multi-turn Conversation ===")
	multiTurnMessages := []llm.Message{
		{
			Role:    "system",
			Content: "You are a senior Go programmer who provides concise code examples.",
		},
		{
			Role:    "user",
			Content: "Show me how to implement a simple HTTP server in Go.",
		},
	}

	resp, err = client.Chat(ctx, multiTurnMessages, &llm.GenerateParams{
		Temperature: 0.3,
	})
	if err != nil {
		logger.Error(ctx, "Failed to chat", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("Multi-turn response:\n%s\n\n", resp)
	}

	// Test 4: Model management
	fmt.Println("=== Test 4: Model Management ===")
	models, err := client.ListModels(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to list models", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Println("Available models:")
		for _, model := range models {
			fmt.Printf("- %s\n", model)
		}
		fmt.Println()
	}

	// Test 5: Different temperature settings
	fmt.Println("=== Test 5: Temperature Variations ===")
	temperatures := []float64{0.1, 0.5, 0.9}
	prompts := []string{
		"Write a short story about a robot learning to paint.",
		"Explain the concept of recursion with a simple example.",
		"Create a recipe for chocolate chip cookies.",
	}

	for i, temp := range temperatures {
		fmt.Printf("Temperature %.1f:\n", temp)
		resp, err := client.Generate(
			ctx,
			prompts[i],
			vllm.WithTemperature(temp),
		)
		if err != nil {
			logger.Error(ctx, "Failed to generate", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Response: %s\n\n", resp)
		}
	}

	// Test 6: System message variations
	fmt.Println("=== Test 6: System Message Variations ===")
	systemMessages := []string{
		"You are a technical writer who explains complex topics simply.",
		"You are a poet who writes in free verse.",
		"You are a chef who specializes in Italian cuisine.",
	}

	for _, sysMsg := range systemMessages {
		fmt.Printf("System: %s\n", sysMsg)
		resp, err := client.Generate(
			ctx,
			"Tell me about machine learning",
			vllm.WithSystemMessage(sysMsg),
			vllm.WithTemperature(0.7),
		)
		if err != nil {
			logger.Error(ctx, "Failed to generate", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Response: %s\n\n", resp)
		}
	}

	// Test 7: Performance test with multiple requests
	fmt.Println("=== Test 7: Performance Test ===")
	testPrompts := []string{
		"Write a one-sentence summary of quantum computing.",
		"Explain what is a binary tree in one sentence.",
		"Describe the color blue in one sentence.",
	}

	for i, prompt := range testPrompts {
		fmt.Printf("Request %d: %s\n", i+1, prompt)
		resp, err := client.Generate(
			ctx,
			prompt,
			vllm.WithTemperature(0.3),
		)
		if err != nil {
			logger.Error(ctx, "Failed to generate", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Response: %s\n\n", resp)
		}
	}

	fmt.Println("=== vLLM Client Example Completed ===")
}
