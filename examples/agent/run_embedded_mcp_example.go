package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func main() {
	fmt.Println("=== Running Embedded MCP Config Example with OpenAI ===")

	// Get OpenAI API key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("❌ OPENAI_API_KEY environment variable is required")
		fmt.Println("Please set your OpenAI API key:")
		fmt.Println("export OPENAI_API_KEY=your_api_key_here")
		return
	}
	fmt.Println("✅ Using OpenAI API key from environment")

	// Create OpenAI LLM client
	fmt.Println("✅ Creating OpenAI client...")
	llm := openai.NewClient(apiKey)

	// Load agent configuration with embedded MCP
	fmt.Println("📁 Loading agent config with embedded MCP servers...")
	agentConfigs, err := agent.LoadAgentConfigsFromFile("embedded_mcp_config.yaml")
	if err != nil {
		fmt.Printf("❌ Failed to load config: %v\n", err)
		return
	}
	fmt.Printf("✅ Loaded %d agent configurations\n", len(agentConfigs))

	// Create agent instance
	fmt.Println("🤖 Creating agent with MCP tools...")
	agentInstance, err := agent.NewAgentFromConfig(
		"devops_expert",
		agentConfigs,
		map[string]string{},
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithMaxIterations(5),           // Allow tool calling
		agent.WithRequirePlanApproval(false), // Execute tools without approval
	)
	if err != nil {
		fmt.Printf("❌ Failed to create agent: %v\n", err)
		return
	}

	// Show MCP configuration
	mcpConfig := agent.GetMCPConfigFromAgent(agentInstance)
	if mcpConfig != nil {
		fmt.Printf("🛠️  Agent configured with %d MCP servers:\n", len(mcpConfig.Servers))
		for _, server := range mcpConfig.Servers {
			status := "✅"
			if !server.Enabled {
				status = "⏸️"
			}
			fmt.Printf("   %s %s (%s): %s\n", status, server.Name, server.Type, server.Description)
		}
	}

	// Test a simple interaction
	fmt.Println("\n🚀 Testing agent interaction...")
	testPrompt := "List the files in /private/var/log directory"

	fmt.Printf("User: %s\n", testPrompt)

	// Create context with organization ID and conversation ID
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "mcp-example-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "mcp-example-conversation")
	response, err := agentInstance.Run(ctx, testPrompt)
	if err != nil {
		fmt.Printf("❌ Agent error: %v\n", err)
		return
	}

	fmt.Printf("Agent: %s\n", response)

	fmt.Println("\n🎉 Example completed successfully!")
	fmt.Println("\nKey Features Demonstrated:")
	fmt.Println("• Real OpenAI integration with agent")
	fmt.Println("• MCP servers embedded in agent YAML")
	fmt.Println("• Agent can interact and use MCP tools")
	fmt.Println("• Production-ready configuration pattern")
}
