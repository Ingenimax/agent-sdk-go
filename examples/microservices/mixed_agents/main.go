package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
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

	fmt.Println("🚀 Setting up mixed local and remote agent system...")

	// Step 1: Create and start a Research Agent microservice
	researchAgent, err := agent.NewAgent(
		agent.WithName("ResearchAgent"),
		agent.WithDescription("A specialized agent for research, information gathering, and analysis"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt("You are a research expert. You excel at gathering information, analyzing data, conducting literature reviews, and providing comprehensive research summaries. Always cite your reasoning and provide detailed explanations."),
	)
	if err != nil {
		log.Fatalf("Failed to create research agent: %v", err)
	}

	// Start Research Agent as a microservice (use different port to avoid conflicts)
	researchService, err := microservice.CreateMicroservice(researchAgent, microservice.Config{
		Port: 8091,  // Changed from 8081 to avoid conflicts
	})
	if err != nil {
		log.Fatalf("Failed to create research microservice: %v", err)
	}

	// Start the service (it runs in the background)
	if err := researchService.Start(); err != nil {
		log.Fatalf("Failed to start research microservice: %v", err)
	}

	// Wait for service to be ready
	if err := researchService.WaitForReady(10 * time.Second); err != nil {
		log.Fatalf("Research microservice failed to become ready: %v", err)
	}
	fmt.Printf("✅ Research Agent microservice started on %s\n", researchService.GetURL())

	// Step 2: Create a local Code Agent
	codeAgent, err := agent.NewAgent(
		agent.WithName("CodeAgent"),
		agent.WithDescription("A specialized agent for code generation, debugging, and software development"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt("You are a coding expert. You excel at writing clean, efficient code in multiple programming languages, debugging issues, explaining code concepts, and following best practices. Always provide working code examples."),
	)
	if err != nil {
		log.Fatalf("Failed to create code agent: %v", err)
	}
	fmt.Println("✅ Local Code Agent created")

	// Step 3: Create a remote connection to an external Math Agent
	// Note: This assumes you're running the basic_microservice example on port 8080
	remoteMathAgent, err := agent.NewAgent(
		agent.WithURL("localhost:8080"),
		agent.WithName("MathAgent"), // Will be overridden by remote metadata
	)
	if err != nil {
		fmt.Printf("⚠️  Warning: Could not connect to Math Agent on localhost:8080: %v\n", err)
		fmt.Println("   Make sure to run the basic_microservice example first!")
		remoteMathAgent = nil
	} else {
		fmt.Printf("✅ Connected to remote Math Agent: %s\n", remoteMathAgent.GetName())
	}

	// Step 4: Create a remote connection to our Research Agent
	remoteResearchAgent, err := agent.NewAgent(
		agent.WithURL("localhost:8091"),  // Updated to match the new port
	)
	if err != nil {
		log.Fatalf("Failed to create remote research agent connection: %v", err)
	}
	fmt.Printf("✅ Connected to remote Research Agent: %s\n", remoteResearchAgent.GetName())

	// Step 5: Create the main orchestrator agent with mixed subagents
	var subAgents []*agent.Agent
	subAgents = append(subAgents, codeAgent)        // Local agent
	subAgents = append(subAgents, remoteResearchAgent) // Remote agent (our own service)
	
	if remoteMathAgent != nil {
		subAgents = append(subAgents, remoteMathAgent) // Remote agent (external service)
	}

	mainAgent, err := agent.NewAgent(
		agent.WithName("MainOrchestrator"),
		agent.WithDescription("An orchestrator agent that delegates tasks to specialized agents"),
		agent.WithLLM(llm),
		agent.WithAgents(subAgents...),
		agent.WithSystemPrompt("You are an intelligent orchestrator. You have access to specialized agents for different tasks: CodeAgent for programming tasks, ResearchAgent for research and analysis, and MathAgent for mathematical calculations. Delegate tasks to the appropriate agent based on the user's request."),
	)
	if err != nil {
		log.Fatalf("Failed to create main agent: %v", err)
	}

	fmt.Printf("✅ Main Orchestrator created with %d subagents\n", len(subAgents))
	fmt.Println()

	// Step 6: Test the mixed agent system
	testTasks := []string{
		"Research the current state of quantum computing and its potential applications",
		"Write a Python function to calculate the factorial of a number recursively",
		"What is the integral of sin(x) * cos(x) dx?",
		"Analyze the pros and cons of microservices architecture in software development",
	}

	ctx := context.Background()

	for i, task := range testTasks {
		fmt.Printf("🎯 Task %d: %s\n", i+1, task)
		fmt.Println("   (This will be delegated to the appropriate agent)")
		
		start := time.Now()
		result, err := mainAgent.Run(ctx, task)
		duration := time.Since(start)
		
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
		} else {
			fmt.Printf("✅ Result (took %v):\n%s\n", duration, result)
		}
		fmt.Println(strings.Repeat("-", 80))
		
		// Add delay between tasks
		time.Sleep(1 * time.Second)
	}

	// Step 7: Cleanup
	fmt.Println("\n🧹 Cleaning up...")
	
	// Disconnect from remote agents
	if remoteMathAgent != nil {
		remoteMathAgent.Disconnect()
	}
	remoteResearchAgent.Disconnect()
	
	// Stop our microservice
	if err := researchService.Stop(); err != nil {
		log.Printf("Error stopping research service: %v", err)
	}

	fmt.Println("✅ Mixed agent system demonstration completed!")
	fmt.Println("\n📊 Summary:")
	fmt.Println("   - Created 1 local agent (CodeAgent)")
	fmt.Println("   - Started 1 microservice (ResearchAgent)")
	fmt.Println("   - Connected to 2 remote agents")
	fmt.Println("   - Orchestrated tasks across local and remote agents")
}

