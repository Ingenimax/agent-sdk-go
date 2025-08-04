package main

import (
	"context"
	"fmt"
	"log"
	"os"
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

	fmt.Println("=== Debug Test for Microservice ===\n")

	// Create a simple agent
	fmt.Println("Creating agent...")
	testAgent, err := agent.NewAgent(
		agent.WithName("TestAgent"),
		agent.WithDescription("Test agent for debugging"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt("You are a helpful assistant."),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}
	fmt.Println("Agent created")

	// Create microservice
	fmt.Println("\nCreating microservice...")
	service, err := microservice.CreateMicroservice(testAgent, microservice.Config{
		Port: 9999,
	})
	if err != nil {
		log.Fatalf("Failed to create microservice: %v", err)
	}
	fmt.Println("Microservice created")

	// Start microservice
	fmt.Println("\nStarting microservice...")
	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start microservice: %v", err)
	}
	fmt.Printf("Start() called, running status: %v\n", service.IsRunning())
	
	// Check running status over time
	fmt.Println("\nChecking running status...")
	for i := 0; i < 5; i++ {
		time.Sleep(200 * time.Millisecond)
		fmt.Printf("  After %dms: running = %v, port = %d\n", 
			(i+1)*200, service.IsRunning(), service.GetPort())
	}

	// Try WaitForReady
	fmt.Println("\nCalling WaitForReady...")
	if err := service.WaitForReady(5 * time.Second); err != nil {
		fmt.Printf("WaitForReady failed: %v\n", err)
	} else {
		fmt.Println("Service is ready!")
	}

	// Test with remote client
	fmt.Println("\nTesting remote client...")
	remoteAgent, err := agent.NewAgent(
		agent.WithURL("localhost:9999"),
	)
	if err != nil {
		fmt.Printf("Failed to create remote agent: %v\n", err)
	} else {
		fmt.Println("Remote agent created")
		
		ctx := context.Background()
		result, err := remoteAgent.Run(ctx, "Say hello")
		if err != nil {
			fmt.Printf("Remote call failed: %v\n", err)
		} else {
			fmt.Printf("Remote call succeeded: %s\n", result)
		}
		
		if err := remoteAgent.Disconnect(); err != nil {
			fmt.Printf("Warning: failed to disconnect remote agent: %v\n", err)
		}
	}

	// Clean up
	fmt.Println("\nStopping service...")
	if err := service.Stop(); err != nil {
		fmt.Printf("Warning: failed to stop service: %v\n", err)
	}
	fmt.Println("Service stopped")
}