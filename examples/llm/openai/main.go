package main

import (
	"context"
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

	// Test text generation with system message
	resp, err := client.Generate(
		ctx,
		"Write a haiku about programming",
		openai.WithSystemMessage("You are a creative assistant who specializes in writing haikus."),
		openai.WithTemperature(0.7),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, "Generated text", map[string]interface{}{"text": resp})

	// Test chat
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

	resp, err = client.Chat(ctx, messages, nil)
	if err != nil {
		logger.Error(ctx, "Failed to chat", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, "Chat response", map[string]interface{}{"text": resp})

	// Example of multi-turn conversation with Chat method
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
		os.Exit(1)
	}
	logger.Info(ctx, "First response", map[string]interface{}{"text": resp})

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
		os.Exit(1)
	}
	logger.Info(ctx, "Follow-up response", map[string]interface{}{"text": resp})
}
