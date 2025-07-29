package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/ollama"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
)

func main() {
	// Create a logger
	logger := logging.New()
	ctx := context.Background()

	// Get Ollama base URL from environment or use default
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	// Get model from environment or use default
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen3:0.6b"
	}

	// Create Ollama client with retry configuration
	client := ollama.NewClient(
		ollama.WithModel(model),
		ollama.WithLogger(logger),
		ollama.WithBaseURL(baseURL),
		ollama.WithRetry(
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
		ollama.WithTemperature(0.7),
		ollama.WithSystemMessage("You are a creative assistant who specializes in writing haikus."),
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
		Temperature: 0.5,
	})
	if err != nil {
		logger.Error(ctx, "Failed to chat", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("First response:\n%s\n\n", resp)
	}

	// Add the assistant's response to continue the conversation
	multiTurnMessages = append(multiTurnMessages, llm.Message{
		Role:    "assistant",
		Content: resp,
	})

	// Add a follow-up question
	multiTurnMessages = append(multiTurnMessages, llm.Message{
		Role:    "user",
		Content: "How would I add middleware for logging requests?",
	})

	// Get the next response
	resp, err = client.Chat(ctx, multiTurnMessages, &llm.GenerateParams{
		Temperature: 0.5,
	})
	if err != nil {
		logger.Error(ctx, "Failed to chat", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("Follow-up response:\n%s\n\n", resp)
	}

	// Test 4: List available models
	fmt.Println("=== Test 4: List Available Models ===")
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

	// Test 5: Generate with different parameters
	fmt.Println("=== Test 5: Generate with Different Parameters ===")
	resp, err = client.Generate(
		ctx,
		"Explain the concept of recursion in programming",
		ollama.WithTemperature(0.3), // Lower temperature for more focused response
		ollama.WithTopP(0.9),
		ollama.WithStopSequences([]string{"In conclusion", "To summarize"}),
		ollama.WithSystemMessage("You are a programming teacher who explains complex concepts simply."),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate with parameters", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("Parameterized response:\n%s\n\n", resp)
	}

	// Test 6: Error handling - try with non-existent model
	fmt.Println("=== Test 6: Error Handling ===")
	errorClient := ollama.NewClient(
		ollama.WithModel("non-existent-model"),
		ollama.WithLogger(logger),
		ollama.WithBaseURL(baseURL),
	)

	_, err = errorClient.Generate(ctx, "This should fail")
	if err != nil {
		fmt.Printf("Expected error (model not found): %v\n\n", err)
	} else {
		fmt.Println("Unexpected: request succeeded with non-existent model")
	}

	fmt.Println("=== Ollama Example Completed ===")
}
