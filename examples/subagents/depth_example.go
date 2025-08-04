package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// This example demonstrates how the max depth validation works for sub-agents
func main() {
	logger := logging.New()
	ctx := context.Background()

	// Check for API key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		logger.Error(ctx, "OPENAI_API_KEY environment variable is not set", nil)
		os.Exit(1)
	}

	llm := openai.NewClient(apiKey, openai.WithLogger(logger))

	fmt.Println("=== Sub-Agents Max Depth Example ===\n")

	// Example 1: Valid shallow hierarchy (depth = 2)
	fmt.Println("1. Creating a valid shallow hierarchy (depth = 2):")
	if err := createShallowHierarchy(llm, logger); err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
	} else {
		fmt.Println("   ✅ Success: Shallow hierarchy created")
	}

	// Example 2: Valid deep hierarchy (depth = 5, at the limit)
	fmt.Println("\n2. Creating a valid deep hierarchy (depth = 5, at limit):")
	if err := createDeepHierarchy(llm, logger); err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
	} else {
		fmt.Println("   ✅ Success: Deep hierarchy created at maximum depth")
	}

	// Example 3: Invalid deep hierarchy (depth = 6, exceeds limit)
	fmt.Println("\n3. Attempting to create invalid deep hierarchy (depth = 6, exceeds limit):")
	if err := createTooDeepHierarchy(llm, logger); err != nil {
		fmt.Printf("   ✅ Expected error caught: %v\n", err)
	} else {
		fmt.Println("   ❌ Should have failed but didn't")
	}

	// Example 4: Runtime recursion depth checking
	fmt.Println("\n4. Demonstrating runtime recursion depth checking:")
	demonstrateRuntimeDepthCheck(llm, logger)

	// Example 5: Complex hierarchy with branching
	fmt.Println("\n5. Creating a complex branching hierarchy:")
	createBranchingHierarchy(llm, logger)
}

// Example 1: Shallow hierarchy (Main -> Level1 -> Level2)
func createShallowHierarchy(llm interfaces.LLM, logger logging.Logger) error {
	// Create Level 2 agent (leaf)
	level2Agent, err := agent.NewAgent(
		agent.WithName("Level2Agent"),
		agent.WithDescription("Level 2 specialist"),
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are a level 2 specialist."),
	)
	if err != nil {
		return fmt.Errorf("failed to create level 2 agent: %w", err)
	}

	// Create Level 1 agent with Level 2 as sub-agent
	level1Agent, err := agent.NewAgent(
		agent.WithName("Level1Agent"),
		agent.WithDescription("Level 1 coordinator"),
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithAgents(level2Agent),
		agent.WithSystemPrompt("You are a level 1 coordinator with access to Level2Agent."),
	)
	if err != nil {
		return fmt.Errorf("failed to create level 1 agent: %w", err)
	}

	// Create Main agent with Level 1 as sub-agent
	mainAgent, err := agent.NewAgent(
		agent.WithName("MainAgent"),
		agent.WithDescription("Main orchestrator"),
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithAgents(level1Agent),
		agent.WithSystemPrompt("You are the main orchestrator with access to Level1Agent."),
	)
	if err != nil {
		return fmt.Errorf("failed to create main agent: %w", err)
	}

	fmt.Printf("   Hierarchy: %s -> %s -> %s (depth = 2)\n",
		mainAgent.GetName(),
		level1Agent.GetName(),
		level2Agent.GetName())

	return nil
}

// Example 2: Deep hierarchy at maximum depth (5 levels)
func createDeepHierarchy(llm interfaces.LLM, logger logging.Logger) error {
	// Create a chain of 6 agents (0 to 5), resulting in depth of 5
	var agents []*agent.Agent

	// Create agents from bottom up
	for i := 5; i >= 0; i-- {
		agentName := fmt.Sprintf("Agent%d", i)
		agentDesc := fmt.Sprintf("Level %d agent", i)
		
		agentOpts := []agent.Option{
			agent.WithName(agentName),
			agent.WithDescription(agentDesc),
			agent.WithLLM(llm),
			agent.WithMemory(memory.NewConversationBuffer()),
			agent.WithSystemPrompt(fmt.Sprintf("You are agent at level %d.", i)),
		}

		// Add the previous agent as a sub-agent (except for the leaf)
		if i < 5 && len(agents) > 0 {
			agentOpts = append(agentOpts, agent.WithAgents(agents[0]))
		}

		newAgent, err := agent.NewAgent(agentOpts...)
		if err != nil {
			return fmt.Errorf("failed to create agent %d: %w", i, err)
		}

		// Prepend to maintain order
		agents = append([]*agent.Agent{newAgent}, agents...)
	}

	// Print the hierarchy
	fmt.Print("   Hierarchy: ")
	for i, a := range agents {
		fmt.Print(a.GetName())
		if i < len(agents)-1 {
			fmt.Print(" -> ")
		}
	}
	fmt.Println(" (depth = 5)")

	return nil
}

// Example 3: Too deep hierarchy (exceeds maximum depth)
func createTooDeepHierarchy(llm interfaces.LLM, logger logging.Logger) error {
	// Try to create a chain of 7 agents (0 to 6), resulting in depth of 6
	var agents []*agent.Agent

	// Create agents from bottom up
	for i := 6; i >= 0; i-- {
		agentName := fmt.Sprintf("DeepAgent%d", i)
		agentDesc := fmt.Sprintf("Deep level %d agent", i)
		
		agentOpts := []agent.Option{
			agent.WithName(agentName),
			agent.WithDescription(agentDesc),
			agent.WithLLM(llm),
			agent.WithMemory(memory.NewConversationBuffer()),
			agent.WithSystemPrompt(fmt.Sprintf("You are agent at deep level %d.", i)),
		}

		// Add the previous agent as a sub-agent (except for the leaf)
		if i < 6 && len(agents) > 0 {
			agentOpts = append(agentOpts, agent.WithAgents(agents[0]))
		}

		newAgent, err := agent.NewAgent(agentOpts...)
		if err != nil {
			// This should fail when we try to create an agent that would exceed max depth
			return err
		}

		// Prepend to maintain order
		agents = append([]*agent.Agent{newAgent}, agents...)
	}

	return nil
}

// Example 4: Runtime recursion depth checking
func demonstrateRuntimeDepthCheck(llm interfaces.LLM, logger logging.Logger) {
	// Create a simple agent that demonstrates runtime depth checking
	_, err := agent.NewAgent(
		agent.WithName("RecursiveAgent"),
		agent.WithDescription("Agent that demonstrates runtime depth checking"),
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are a recursive agent."),
	)
	if err != nil {
		fmt.Printf("   Failed to create recursive agent: %v\n", err)
		return
	}

	// Simulate different recursion depths
	fmt.Println("   Testing context recursion depth limits:")
	
	ctx := context.Background()
	
	// Test at different depths
	for depth := 0; depth <= 6; depth++ {
		// Create a context with the specified recursion depth
		testCtx := ctx
		for i := 0; i < depth; i++ {
			testCtx = agent.WithSubAgentContext(testCtx, 
				fmt.Sprintf("Parent%d", i), 
				fmt.Sprintf("Child%d", i+1))
		}
		
		// Check if this depth is valid
		currentDepth := agent.GetRecursionDepth(testCtx)
		err := agent.ValidateRecursionDepth(testCtx)
		
		if err != nil {
			fmt.Printf("   - Depth %d: ❌ Exceeds limit (error: %v)\n", currentDepth, err)
		} else {
			fmt.Printf("   - Depth %d: ✅ Within limit\n", currentDepth)
		}
	}
}

// Example 5: Complex branching hierarchy
func createBranchingHierarchy(llm interfaces.LLM, logger logging.Logger) {
	// Create leaf agents (Level 3)
	dataAgent, _ := agent.NewAgent(
		agent.WithName("DataAgent"),
		agent.WithDescription("Data processing specialist"),
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
	)

	analyticsAgent, _ := agent.NewAgent(
		agent.WithName("AnalyticsAgent"),
		agent.WithDescription("Analytics specialist"),
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
	)

	reportAgent, _ := agent.NewAgent(
		agent.WithName("ReportAgent"),
		agent.WithDescription("Report generation specialist"),
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
	)

	// Create middle layer (Level 2)
	// Business agent has data and analytics as sub-agents
	businessAgent, _ := agent.NewAgent(
		agent.WithName("BusinessAgent"),
		agent.WithDescription("Business logic coordinator"),
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithAgents(dataAgent, analyticsAgent),
	)

	// Technical agent has report as sub-agent
	technicalAgent, _ := agent.NewAgent(
		agent.WithName("TechnicalAgent"),
		agent.WithDescription("Technical coordinator"),
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithAgents(reportAgent),
	)

	// Create root (Level 1)
	rootAgent, err := agent.NewAgent(
		agent.WithName("RootAgent"),
		agent.WithDescription("Root orchestrator"),
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithAgents(businessAgent, technicalAgent),
	)

	if err != nil {
		fmt.Printf("   ❌ Failed to create branching hierarchy: %v\n", err)
	} else {
		fmt.Println("   ✅ Successfully created branching hierarchy:")
		fmt.Println("      RootAgent")
		fmt.Println("      ├── BusinessAgent")
		fmt.Println("      │   ├── DataAgent")
		fmt.Println("      │   └── AnalyticsAgent")
		fmt.Println("      └── TechnicalAgent")
		fmt.Println("          └── ReportAgent")
		fmt.Printf("   Max depth: 3, Total agents: 6\n")

		// Show sub-agent counts
		fmt.Printf("   - RootAgent has %d sub-agents\n", len(rootAgent.GetSubAgents()))
		fmt.Printf("   - BusinessAgent has %d sub-agents\n", len(businessAgent.GetSubAgents()))
		fmt.Printf("   - TechnicalAgent has %d sub-agents\n", len(technicalAgent.GetSubAgents()))
	}
}

// Helper function to visualize agent hierarchy
func printAgentHierarchy(agent *agent.Agent, prefix string, isLast bool) {
	// Print current agent
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	if prefix != "" {
		fmt.Printf("%s%s%s\n", prefix, connector, agent.GetName())
	} else {
		fmt.Printf("%s\n", agent.GetName())
	}

	// Prepare prefix for children
	childPrefix := prefix
	if prefix != "" {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	// Print sub-agents
	subAgents := agent.GetSubAgents()
	for i, subAgent := range subAgents {
		printAgentHierarchy(subAgent, childPrefix, i == len(subAgents)-1)
	}
}