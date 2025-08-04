package main

import (
	"context"
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

	fmt.Println("🏗️  Setting up Microservice Manager...")

	// Create a microservice manager
	manager := microservice.NewMicroserviceManager()

	// Create multiple specialized agents
	agents := []struct {
		name        string
		description string
		systemPrompt string
		port        int
	}{
		{
			name:        "MathAgent",
			description: "Specialized in mathematical calculations and problem solving",
			systemPrompt: "You are a mathematical expert. Solve problems step by step and show your work clearly.",
			port:        8080,
		},
		{
			name:        "WritingAgent", 
			description: "Specialized in writing, editing, and content creation",
			systemPrompt: "You are a writing expert. Help with creative writing, editing, grammar, and content creation.",
			port:        8081,
		},
		{
			name:        "CodeAgent",
			description: "Specialized in software development and programming",
			systemPrompt: "You are a coding expert. Write clean, efficient code and explain programming concepts clearly.",
			port:        8082,
		},
		{
			name:        "ResearchAgent",
			description: "Specialized in research, analysis, and information gathering",
			systemPrompt: "You are a research expert. Provide thorough analysis and well-researched information.",
			port:        8083,
		},
	}

	// Create and register microservices
	for _, agentConfig := range agents {
		fmt.Printf("📦 Creating %s...\n", agentConfig.name)
		
		// Create the agent
		ag, err := agent.NewAgent(
			agent.WithName(agentConfig.name),
			agent.WithDescription(agentConfig.description),
			agent.WithLLM(llm),
			agent.WithSystemPrompt(agentConfig.systemPrompt),
		)
		if err != nil {
			log.Fatalf("Failed to create %s: %v", agentConfig.name, err)
		}

		// Create microservice
		service, err := microservice.CreateMicroservice(ag, microservice.Config{
			Port: agentConfig.port,
		})
		if err != nil {
			log.Fatalf("Failed to create microservice for %s: %v", agentConfig.name, err)
		}

		// Register with manager
		if err := manager.RegisterService(agentConfig.name, service); err != nil {
			log.Fatalf("Failed to register %s: %v", agentConfig.name, err)
		}

		fmt.Printf("✅ %s registered on port %d\n", agentConfig.name, agentConfig.port)
	}

	fmt.Printf("\n🚀 Starting all %d microservices...\n", len(agents))

	// Start all services
	if err := manager.StartAll(); err != nil {
		log.Fatalf("Failed to start all services: %v", err)
	}

	// Wait for all services to be ready
	fmt.Println("⏳ Waiting for all services to be ready...")
	for _, agentConfig := range agents {
		service, exists := manager.GetService(agentConfig.name)
		if !exists {
			log.Fatalf("Service %s not found in manager", agentConfig.name)
		}
		
		if err := service.WaitForReady(10 * time.Second); err != nil {
			log.Fatalf("Service %s failed to become ready: %v", agentConfig.name, err)
		}
	}

	fmt.Println("\n✅ All microservices are running!")
	fmt.Println("\n📋 Service Registry:")
	for _, serviceName := range manager.ListServices() {
		service, _ := manager.GetService(serviceName)
		fmt.Printf("   • %s: %s (Port: %d)\n", 
			serviceName, 
			service.GetAgent().GetDescription(),
			service.GetPort())
	}

	// Demonstrate using the remote agents
	fmt.Println("\n🔄 Testing distributed agent system...")
	
	// Create remote connections to all our services
	var remoteAgents []*agent.Agent
	for _, agentConfig := range agents {
		remoteAgent, err := agent.NewAgent(
			agent.WithURL(fmt.Sprintf("localhost:%d", agentConfig.port)),
		)
		if err != nil {
			log.Printf("Warning: Failed to connect to %s: %v", agentConfig.name, err)
			continue
		}
		remoteAgents = append(remoteAgents, remoteAgent)
	}

	// Create an orchestrator that uses all remote agents
	orchestrator, err := agent.NewAgent(
		agent.WithName("Orchestrator"),
		agent.WithDescription("Orchestrates tasks across multiple specialized agents"),
		agent.WithLLM(llm),
		agent.WithAgents(remoteAgents...),
		agent.WithSystemPrompt("You are an intelligent orchestrator with access to specialized agents: MathAgent for calculations, WritingAgent for text tasks, CodeAgent for programming, and ResearchAgent for analysis. Delegate tasks to the most appropriate agent."),
	)
	if err != nil {
		log.Fatalf("Failed to create orchestrator: %v", err)
	}

	fmt.Printf("🎭 Created orchestrator with %d remote agents\n", len(remoteAgents))

	// Test the distributed system
	testTasks := []string{
		"Calculate the compound interest on $1000 at 5% annual rate for 10 years",
		"Write a creative short story about a robot learning to paint",
		"Create a Python function to implement binary search",
		"Research the environmental impact of electric vehicles vs gasoline cars",
	}

	ctx := context.Background()
	for i, task := range testTasks {
		fmt.Printf("\n🎯 Task %d: %s\n", i+1, task)
		
		start := time.Now()
		result, err := orchestrator.Run(ctx, task)
		duration := time.Since(start)
		
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
		} else {
			// Truncate long responses for display
			displayResult := result
			if len(displayResult) > 200 {
				displayResult = displayResult[:200] + "..."
			}
			fmt.Printf("✅ Result (took %v): %s\n", duration, displayResult)
		}
	}

	fmt.Println("\n📊 Service Health Check:")
	for _, serviceName := range manager.ListServices() {
		service, _ := manager.GetService(serviceName)
		if service.IsRunning() {
			fmt.Printf("   ✅ %s: Running on port %d\n", serviceName, service.GetPort())
		} else {
			fmt.Printf("   ❌ %s: Not running\n", serviceName)
		}
	}

	// Set up graceful shutdown
	fmt.Println("\n🎮 Microservice Manager is running. Press Ctrl+C to shutdown all services...")
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\n🛑 Shutting down all microservices...")

	// Disconnect remote agents
	for _, remoteAgent := range remoteAgents {
		remoteAgent.Disconnect()
	}

	// Stop all services
	if err := manager.StopAll(); err != nil {
		log.Printf("Error stopping services: %v", err)
	}

	fmt.Println("✅ All microservices stopped successfully")
	fmt.Println("\n📈 Session Summary:")
	fmt.Printf("   • Managed %d microservices\n", len(agents))
	fmt.Printf("   • Processed %d distributed tasks\n", len(testTasks))
	fmt.Println("   • Demonstrated seamless local/remote agent integration")
}