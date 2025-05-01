package main

import (
	"context"
	"os"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
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

	// Create client with retry configuration
	client := openai.NewClient(
		apiKey,
		openai.WithModel("gpt-4o-mini"),
		openai.WithLogger(logger),
		openai.WithRetry(
			retry.WithMaxAttempts(3),
			retry.WithInitialInterval(time.Second),
			retry.WithBackoffCoefficient(2.0),
			retry.WithMaximumInterval(time.Second*30),
		),
	)

	// Example 1: Simple text generation with retry
	logger.Info(ctx, "Example 1: Simple text generation with retry", nil)
	resp, err := client.Generate(
		ctx,
		"Write a short poem about resilience",
		openai.WithSystemMessage("You are a creative poet who writes about overcoming challenges."),
		openai.WithTemperature(0.7),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, "Generated text", map[string]interface{}{"text": resp})

	// Example 2: Chat with retry and custom parameters
	logger.Info(ctx, "Example 2: Chat with retry and custom parameters", nil)
	messages := []llm.Message{
		{
			Role:    "system",
			Content: "You are a helpful assistant who provides detailed explanations.",
		},
		{
			Role:    "user",
			Content: "Explain the concept of exponential backoff in retry mechanisms.",
		},
	}

	resp, err = client.Chat(ctx, messages, &llm.GenerateParams{
		Temperature: 0.5,
	})
	if err != nil {
		logger.Error(ctx, "Failed to chat", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, "Chat response", map[string]interface{}{"text": resp})

	// Example 3: Simulating a failure scenario
	apiKey = "invalid"
	client = openai.NewClient(
		apiKey,
		openai.WithModel("gpt-4o-mini"),
		openai.WithLogger(logger),
		openai.WithRetry(
			retry.WithMaxAttempts(3),
			retry.WithInitialInterval(time.Second*5),
			retry.WithBackoffCoefficient(2.0),
			retry.WithMaximumInterval(time.Second*30),
		),
	)

	resp, err = client.Generate(
		ctx,
		"Write a short poem about resilience",
		openai.WithSystemMessage("You are a creative poet who writes about overcoming challenges."),
		openai.WithTemperature(0.7),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, "Generated text", map[string]interface{}{"text": resp})

}
