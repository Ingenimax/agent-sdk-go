package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/ollama"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
)

func main() {
	// Create a logger
	logger := logging.New()
	ctx := context.Background()

	// Get Ollama configuration from environment
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen3:0.6b"
	}

	// Create Ollama LLM client
	ollamaClient := ollama.NewClient(
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

	// Create an agent with Ollama
	myAgent, err := agent.NewAgent(
		agent.WithLLM(ollamaClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are a helpful AI assistant powered by Ollama. You can help with programming, writing, and general questions."),
		agent.WithName("OllamaAgent"),
	)
	if err != nil {
		logger.Error(ctx, "Failed to create agent", map[string]interface{}{"error": err.Error()})
		log.Fatal(err)
	}

	// Create context with organization and conversation IDs
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "ollama-demo")

	// Test the agent with different types of queries
	queries := []string{
		"Write a simple Go function to calculate the factorial of a number",
		"What are the benefits of using local LLMs like Ollama?",
		"Explain the concept of recursion with a simple example",
	}

	for i, query := range queries {
		fmt.Printf("\n=== Query %d ===\n", i+1)
		fmt.Printf("User: %s\n", query)

		response, err := myAgent.Run(ctx, query)
		if err != nil {
			logger.Error(ctx, "Agent run failed", map[string]interface{}{"error": err.Error()})
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Printf("Agent: %s\n", response)
		}
	}

	// Test conversation memory
	fmt.Printf("\n=== Testing Conversation Memory ===\n")
	fmt.Println("User: What was the first thing I asked you about?")

	response, err := myAgent.Run(ctx, "What was the first thing I asked you about?")
	if err != nil {
		logger.Error(ctx, "Agent run failed", map[string]interface{}{"error": err.Error()})
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Agent: %s\n", response)
	}

	fmt.Println("\n=== Ollama Agent Integration Example Completed ===")
}
