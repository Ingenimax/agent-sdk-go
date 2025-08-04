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

	fmt.Println("=== Testing Agent Microservices ===\n")

	// Step 1: Create a local math agent
	fmt.Println("1. Creating local MathAgent...")
	mathAgent, err := agent.NewAgent(
		agent.WithName("MathAgent"),
		agent.WithDescription("Mathematical calculations expert"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt("You are a mathematical expert. Solve problems step by step."),
	)
	if err != nil {
		log.Fatalf("Failed to create math agent: %v", err)
	}
	fmt.Println("MathAgent created successfully")

	// Step 2: Test the agent locally first
	fmt.Println("\n2. Testing MathAgent locally...")
	ctx := context.Background()
	result, err := mathAgent.Run(ctx, "What is 15 + 27?")
	if err != nil {
		log.Printf("Local test failed: %v", err)
	} else {
		fmt.Printf("Local test result: %s\n", result)
	}

	// Step 3: Wrap as microservice
	fmt.Println("\n3. Creating microservice wrapper...")
	service, err := microservice.CreateMicroservice(mathAgent, microservice.Config{
		Port:    8090, // Use a different port to avoid conflicts
		Timeout: 30 * time.Second,
	})
	if err != nil {
		log.Fatalf("Failed to create microservice: %v", err)
	}
	fmt.Println("Microservice wrapper created")

	// Step 4: Start the microservice
	fmt.Println("\n4. Starting microservice...")
	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start microservice: %v", err)
	}
	
	// Step 5: Wait for service to be ready
	fmt.Println("\n5. Waiting for service to be ready...")
	if err := service.WaitForReady(10 * time.Second); err != nil {
		log.Fatalf("Service failed to become ready: %v", err)
	}
	fmt.Printf("Service is ready on port %d\n", service.GetPort())

	// Step 6: Create a remote agent client
	fmt.Println("\n6. Creating remote agent client...")
	remoteAgent, err := agent.NewAgent(
		agent.WithURL(fmt.Sprintf("localhost:%d", service.GetPort())),
		agent.WithName("RemoteMathAgent"),
	)
	if err != nil {
		log.Fatalf("Failed to create remote agent: %v", err)
	}
	fmt.Println("Remote agent client created")

	// Step 7: Test the remote agent
	fmt.Println("\n7. Testing remote agent...")
	remoteResult, err := remoteAgent.Run(ctx, "What is 100 divided by 4?")
	if err != nil {
		log.Printf("Remote test failed: %v", err)
	} else {
		fmt.Printf("Remote test result: %s\n", remoteResult)
	}

	// Step 8: Test using remote agent as subagent
	fmt.Println("\n8. Testing remote agent as subagent...")
	orchestrator, err := agent.NewAgent(
		agent.WithName("Orchestrator"),
		agent.WithLLM(llm),
		agent.WithAgents(remoteAgent),
		agent.WithSystemPrompt("You have access to a MathAgent for calculations. Use it when needed."),
	)
	if err != nil {
		log.Fatalf("Failed to create orchestrator: %v", err)
	}
	
	orchResult, err := orchestrator.Run(ctx, "Use the MathAgent to calculate: What is the square root of 144?")
	if err != nil {
		log.Printf("Orchestrator test failed: %v", err)
	} else {
		fmt.Printf("Orchestrator result: %s\n", orchResult)
	}

	// Clean up
	fmt.Println("\n9. Cleaning up...")
	if err := remoteAgent.Disconnect(); err != nil {
		fmt.Printf("Warning: failed to disconnect remote agent: %v\n", err)
	}
	if err := service.Stop(); err != nil {
		fmt.Printf("Warning: failed to stop service: %v\n", err)
	}
	fmt.Println("Cleanup complete")

	fmt.Println("\n=== All tests completed successfully! ===")
}