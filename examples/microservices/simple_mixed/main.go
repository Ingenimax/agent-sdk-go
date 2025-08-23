package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Ingenimax/agent-sdk-go/examples/microservices/shared"
	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
)

func main() {
	fmt.Println("Simple Mixed Agents Example")
	fmt.Printf("Using LLM: %s\n", shared.GetProviderInfo())
	
	// Create an LLM client based on environment configuration
	llm, err := shared.CreateLLM()
	if err != nil {
		log.Fatalf("Failed to create LLM client: %v", err)
	}

	// Step 1: Create and start a Math Agent microservice
	fmt.Println("1. Creating Math Agent microservice...")
	mathAgent, err := agent.NewAgent(
		agent.WithName("MathAgent"),
		agent.WithDescription("Mathematical calculations"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt("You are a math expert. Answer concisely."),
	)
	if err != nil {
		log.Fatalf("Failed to create math agent: %v", err)
	}

	mathService, err := microservice.CreateMicroservice(mathAgent, microservice.Config{
		Port: 9001,
	})
	if err != nil {
		log.Fatalf("Failed to create math microservice: %v", err)
	}

	if err := mathService.Start(); err != nil {
		log.Fatalf("Failed to start math microservice: %v", err)
	}

	// Wait for the service to be ready
	if err := mathService.WaitForReady(10 * time.Second); err != nil {
		log.Fatalf("Math microservice failed to become ready: %v", err)
	}
	fmt.Printf("Math Agent running on port %d\n", mathService.GetPort())

	// Step 2: Create a local Code Agent
	fmt.Println("\n2. Creating local Code Agent...")
	codeAgent, err := agent.NewAgent(
		agent.WithName("CodeAgent"),
		agent.WithDescription("Programming and code generation"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt("You are a coding expert. Provide clean code."),
	)
	if err != nil {
		log.Fatalf("Failed to create code agent: %v", err)
	}
	fmt.Println("Code Agent created locally")

	// Step 3: Connect to the Math Agent as a remote agent
	fmt.Println("\n3. Connecting to Math Agent remotely...")
	remoteMathAgent, err := agent.NewAgent(
		agent.WithURL("localhost:9001"),
		agent.WithName("RemoteMathAgent"),
	)
	if err != nil {
		log.Fatalf("Failed to connect to remote math agent: %v", err)
	}
	fmt.Println("Connected to remote Math Agent")

	// Step 4: Create orchestrator with both local and remote agents
	fmt.Println("\n4. Creating orchestrator with mixed agents...")
	orchestrator, err := agent.NewAgent(
		agent.WithName("Orchestrator"),
		agent.WithLLM(llm),
		agent.WithAgents(codeAgent, remoteMathAgent), // Mix local and remote!
		agent.WithSystemPrompt("You coordinate between CodeAgent for programming and MathAgent for calculations."),
	)
	if err != nil {
		log.Fatalf("Failed to create orchestrator: %v", err)
	}
	fmt.Println("Orchestrator created with local and remote agents")

	// Step 5: Test the system
	fmt.Println("\n5. Testing mixed agent system...")
	ctx := context.Background()
	
	tests := []string{
		"What is 25 * 4?",
		"Write a Python function to calculate factorial",
		"Calculate the sum of squares from 1 to 10",
	}

	for i, test := range tests {
		fmt.Printf("\nTest %d: %s\n", i+1, test)
		result, err := orchestrator.Run(ctx, test)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Printf("Result: %s\n", result)
		}
	}

	// Clean up
	fmt.Println("\n6. Cleaning up...")
	if err := remoteMathAgent.Disconnect(); err != nil {
		fmt.Printf("Warning: failed to disconnect remote math agent: %v\n", err)
	}
	if err := mathService.Stop(); err != nil {
		fmt.Printf("Warning: failed to stop math service: %v\n", err)
	}
	fmt.Println("Cleanup complete")

	fmt.Println("\nMixed agents example completed successfully!")
}