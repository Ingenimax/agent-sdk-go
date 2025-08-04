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

	fmt.Println("🚀 Simple Mixed Agents Example\n")

	// Step 1: Create and start a Math Agent microservice
	fmt.Println("1️⃣ Creating Math Agent microservice...")
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

	// Give it time to start
	time.Sleep(500 * time.Millisecond)
	fmt.Printf("✅ Math Agent running on port %d\n", mathService.GetPort())

	// Step 2: Create a local Code Agent
	fmt.Println("\n2️⃣ Creating local Code Agent...")
	codeAgent, err := agent.NewAgent(
		agent.WithName("CodeAgent"),
		agent.WithDescription("Programming and code generation"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt("You are a coding expert. Provide clean code."),
	)
	if err != nil {
		log.Fatalf("Failed to create code agent: %v", err)
	}
	fmt.Println("✅ Code Agent created locally")

	// Step 3: Connect to the Math Agent as a remote agent
	fmt.Println("\n3️⃣ Connecting to Math Agent remotely...")
	remoteMathAgent, err := agent.NewAgent(
		agent.WithURL("localhost:9001"),
		agent.WithName("RemoteMathAgent"),
	)
	if err != nil {
		log.Fatalf("Failed to connect to remote math agent: %v", err)
	}
	fmt.Println("✅ Connected to remote Math Agent")

	// Step 4: Create orchestrator with both local and remote agents
	fmt.Println("\n4️⃣ Creating orchestrator with mixed agents...")
	orchestrator, err := agent.NewAgent(
		agent.WithName("Orchestrator"),
		agent.WithLLM(llm),
		agent.WithAgents(codeAgent, remoteMathAgent), // Mix local and remote!
		agent.WithSystemPrompt("You coordinate between CodeAgent for programming and MathAgent for calculations."),
	)
	if err != nil {
		log.Fatalf("Failed to create orchestrator: %v", err)
	}
	fmt.Println("✅ Orchestrator created with local and remote agents")

	// Step 5: Test the system
	fmt.Println("\n5️⃣ Testing mixed agent system...")
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
			fmt.Printf("❌ Error: %v\n", err)
		} else {
			fmt.Printf("✅ Result: %s\n", result)
		}
	}

	// Clean up
	fmt.Println("\n6️⃣ Cleaning up...")
	remoteMathAgent.Disconnect()
	mathService.Stop()
	fmt.Println("✅ Cleanup complete")

	fmt.Println("\n🎉 Mixed agents example completed successfully!")
}