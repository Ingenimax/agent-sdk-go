package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/vllm"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
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

	// Create vLLM client
	vllmClient := vllm.NewClient(
		vllm.WithModel(model),
		vllm.WithLogger(logger),
		vllm.WithBaseURL(baseURL),
	)

	// Create memory for the agent
	mem := memory.NewConversationBuffer()

	// Create agent with vLLM backend
	myAgent, err := agent.NewAgent(
		agent.WithLLM(vllmClient),
		agent.WithMemory(mem),
	)
	if err != nil {
		logger.Error(ctx, "Failed to create agent", map[string]interface{}{"error": err.Error()})
		return
	}

	fmt.Println("=== vLLM Agent Integration Examples ===\n")

	// Test 1: Basic conversation
	fmt.Println("1. Basic Conversation")

	response, err := myAgent.Run(ctx, "Hello! I'm a new user. Can you help me learn about Go programming?")
	if err != nil {
		logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("Agent: %s\n\n", response)
	}

	// Test 2: Follow-up conversation (testing memory)
	fmt.Println("2. Follow-up Conversation (Testing Memory)")

	response, err = myAgent.Run(ctx, "Can you give me a simple example of what we just discussed?")
	if err != nil {
		logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("Agent: %s\n\n", response)
	}

	// Test 3: Technical question
	fmt.Println("3. Technical Question")

	response, err = myAgent.Run(ctx, "What are the main differences between slices and arrays in Go?")
	if err != nil {
		logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("Agent: %s\n\n", response)
	}

	// Test 4: Code generation request
	fmt.Println("4. Code Generation Request")

	response, err = myAgent.Run(ctx, "Can you write a function that reverses a string in Go?")
	if err != nil {
		logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("Agent: %s\n\n", response)
	}

	// Test 5: Complex problem solving
	fmt.Println("5. Complex Problem Solving")

	response, err = myAgent.Run(ctx, "I need to build a simple web server in Go that serves static files. Can you help me design it?")
	if err != nil {
		logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("Agent: %s\n\n", response)
	}

	// Test 6: Different system prompts
	fmt.Println("6. Different System Prompts")

	// Create a new agent with a different system prompt
	creativeAgent, err := agent.NewAgent(
		agent.WithLLM(vllmClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are a creative writing assistant who specializes in storytelling and creative expression."),
	)
	if err != nil {
		logger.Error(ctx, "Failed to create creative agent", map[string]interface{}{"error": err.Error()})
	} else {
		response, err = creativeAgent.Run(ctx, "Write a short story about a programmer who discovers a magical bug in their code.")
		if err != nil {
			logger.Error(ctx, "Failed to run creative agent", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Creative Agent: %s\n\n", response)
		}
	}

	// Test 7: Memory persistence test
	fmt.Println("7. Memory Persistence Test")

	// Create a new agent with fresh memory
	memoryAgent, err := agent.NewAgent(
		agent.WithLLM(vllmClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are a helpful assistant with a good memory. Remember important details from our conversation."),
	)
	if err != nil {
		logger.Error(ctx, "Failed to create memory agent", map[string]interface{}{"error": err.Error()})
	} else {
		// First message
		response, err = memoryAgent.Run(ctx, "My name is Alice and I'm learning Python. I'm particularly interested in data science.")
		if err != nil {
			logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Agent: %s\n\n", response)
		}

		// Second message (should remember the context)
		response, err = memoryAgent.Run(ctx, "What would you recommend for someone like me to start with?")
		if err != nil {
			logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Agent: %s\n\n", response)
		}

		// Third message (testing memory retention)
		response, err = memoryAgent.Run(ctx, "Can you remind me what my name is and what I'm interested in?")
		if err != nil {
			logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Agent: %s\n\n", response)
		}
	}

	// Test 8: Performance comparison
	fmt.Println("8. Performance Comparison")

	// Test multiple quick requests
	prompts := []string{
		"What is a variable in programming?",
		"Explain what a function is.",
		"Describe the concept of loops.",
		"What is object-oriented programming?",
	}

	for i, prompt := range prompts {
		fmt.Printf("Request %d: %s\n", i+1, prompt)
		response, err := myAgent.Run(ctx, prompt)
		if err != nil {
			logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Response: %s\n\n", response)
		}
	}

	// Test 9: Error handling
	fmt.Println("9. Error Handling Test")

	// Test with a very long prompt that might cause issues
	longPrompt := "Please provide a very detailed explanation of the following complex topic: " +
		"Explain the differences between various programming paradigms including imperative, " +
		"declarative, functional, object-oriented, and logic programming. Include examples " +
		"of languages that represent each paradigm, discuss their advantages and disadvantages, " +
		"and explain when you might choose one over the other. Also discuss how these paradigms " +
		"have evolved over time and what modern programming languages are doing to combine " +
		"multiple paradigms. Finally, provide practical advice for developers on how to choose " +
		"the right paradigm for their specific use case."

	response, err = myAgent.Run(ctx, longPrompt)
	if err != nil {
		logger.Error(ctx, "Failed to handle long prompt", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("Long prompt response: %s\n\n", response)
	}

	// Test 10: Model information
	fmt.Println("10. Model Information")

	// Get information about the model being used
	fmt.Printf("Using vLLM model: %s\n", vllmClient.Name())
	fmt.Printf("Model: %s\n", vllmClient.Model)
	fmt.Printf("Base URL: %s\n", baseURL)

	// Test listing models
	models, err := vllmClient.ListModels(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to list models", map[string]interface{}{"error": err.Error()})
	} else {
		fmt.Printf("Available models: %v\n", models)
	}

	fmt.Println("\n=== vLLM Agent Integration Examples Completed ===")
}
