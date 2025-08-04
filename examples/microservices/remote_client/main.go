package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
)

func main() {
	fmt.Println("🔌 Connecting to remote Math Agent microservice...")

	// Create a remote agent that connects to the microservice
	// Make sure the basic_microservice example is running first!
	remoteAgent, err := agent.NewAgent(
		agent.WithURL("localhost:8080"),
		agent.WithName("RemoteMathAgent"), // Optional: will be fetched from remote if not set
	)
	if err != nil {
		log.Fatalf("Failed to create remote agent: %v", err)
	}

	fmt.Printf("✅ Connected to remote agent: %s\n", remoteAgent.GetName())
	fmt.Printf("   Description: %s\n", remoteAgent.GetDescription())
	fmt.Printf("   Remote URL: %s\n", remoteAgent.GetRemoteURL())
	fmt.Println()

	// Test the remote agent with various mathematical problems
	testProblems := []string{
		"What is 15 * 23?",
		"Calculate the area of a circle with radius 7",
		"Solve the equation: 2x + 5 = 17",
		"What is the derivative of x^3 + 2x^2 - 5x + 1?",
		"Convert 75 degrees Fahrenheit to Celsius",
	}

	ctx := context.Background()

	for i, problem := range testProblems {
		fmt.Printf("🧮 Problem %d: %s\n", i+1, problem)
		
		start := time.Now()
		result, err := remoteAgent.Run(ctx, problem)
		duration := time.Since(start)
		
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
		} else {
			fmt.Printf("✅ Answer (took %v): %s\n", duration, result)
		}
		fmt.Println()
		
		// Add a small delay between requests
		time.Sleep(500 * time.Millisecond)
	}

	// Demonstrate that the remote agent works exactly like a local agent
	fmt.Println("🔄 Testing agent information methods...")
	fmt.Printf("Agent Name: %s\n", remoteAgent.GetName())
	fmt.Printf("Agent Description: %s\n", remoteAgent.GetDescription())
	fmt.Printf("Is Remote: %t\n", remoteAgent.IsRemote())
	fmt.Printf("Capabilities: %s\n", remoteAgent.GetCapabilities())

	// Clean up
	if err := remoteAgent.Disconnect(); err != nil {
		log.Printf("Warning: Failed to disconnect from remote agent: %v", err)
	} else {
		fmt.Println("🔌 Disconnected from remote agent")
	}

	fmt.Println("\n✅ Remote agent client demonstration completed!")
}