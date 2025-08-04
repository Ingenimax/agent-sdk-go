package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
)

func main() {
	// Get OpenAI API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create an LLM client
	llm := openai.NewClient(apiKey)

	// Create a mathematical agent
	mathAgent, err := agent.NewAgent(
		agent.WithName("MathAgent"),
		agent.WithDescription("A specialized agent for mathematical calculations and problem solving"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt("You are a mathematical expert. You excel at solving complex mathematical problems, performing calculations, and explaining mathematical concepts clearly. Always show your work step by step."),
	)
	if err != nil {
		log.Fatalf("Failed to create math agent: %v", err)
	}

	// Create microservice configuration
	config := microservice.Config{
		Port:    8080,
		Timeout: 30 * time.Second,
	}

	// Create the microservice
	service, err := microservice.CreateMicroservice(mathAgent, config)
	if err != nil {
		log.Fatalf("Failed to create microservice: %v", err)
	}

	// Start the microservice
	fmt.Println("Starting Math Agent microservice...")
	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start microservice: %v", err)
	}

	// Wait for the service to be ready
	if err := service.WaitForReady(10 * time.Second); err != nil {
		log.Fatalf("Microservice failed to become ready: %v", err)
	}

	fmt.Printf("Math Agent microservice is running on %s\n", service.GetURL())
	fmt.Println("You can now connect to this agent from other processes using:")
	fmt.Printf("  agent.NewAgent(agent.WithURL(\"%s\"))\n", service.GetURL())
	fmt.Println()
	fmt.Println("To test the service, try running the remote_client example in another terminal.")
	fmt.Println("Press Ctrl+C to stop the service...")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\nShutting down microservice...")

	// Stop the service
	if err := service.Stop(); err != nil {
		log.Printf("Error stopping microservice: %v", err)
	}

	fmt.Println("Microservice stopped successfully")
}

