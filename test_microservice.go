package main

import (
	"fmt"
	"log"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
)

func main() {
	fmt.Println("=== Testing Microservice Health Check ===")
	
	// Create a minimal LLM (doesn't need to work, just needs to exist)
	llm := openai.NewClient("fake-key")
	
	// Create agent
	testAgent, err := agent.NewAgent(
		agent.WithName("TestMicroserviceAgent"),
		agent.WithDescription("Test agent for microservice health check"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt("Test prompt"),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}
	
	// Create microservice
	service, err := microservice.CreateMicroservice(testAgent, microservice.Config{
		Port: 0, // Auto-assign port
	})
	if err != nil {
		log.Fatalf("Failed to create microservice: %v", err)
	}
	
	fmt.Println("Starting microservice...")
	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start microservice: %v", err)
	}
	
	fmt.Printf("Microservice started on port %d\n", service.GetPort())
	
	// Test different wait times to see what works
	testWaitTimes := []time.Duration{
		1 * time.Second,
		5 * time.Second,
		10 * time.Second,
		15 * time.Second,
	}
	
	for _, waitTime := range testWaitTimes {
		fmt.Printf("\nTesting WaitForReady with %v timeout...\n", waitTime)
		
		start := time.Now()
		err := service.WaitForReady(waitTime)
		elapsed := time.Since(start)
		
		if err != nil {
			fmt.Printf("❌ WaitForReady failed after %v: %v\n", elapsed, err)
		} else {
			fmt.Printf("✅ WaitForReady succeeded after %v\n", elapsed)
			break
		}
	}
	
	// Cleanup
	fmt.Println("\nStopping microservice...")
	if err := service.Stop(); err != nil {
		log.Printf("Warning: failed to stop microservice: %v", err)
	}
	
	fmt.Println("Test completed.")
}