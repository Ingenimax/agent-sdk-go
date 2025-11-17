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
	fmt.Println("=== Standalone MCP Configuration Example ===")

	// Create LLM client
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("❌ OPENAI_API_KEY environment variable is required")
		fmt.Println("Please set your OpenAI API key:")
		fmt.Println("export OPENAI_API_KEY=your_api_key_here")
		return
	}
	llm := openai.NewClient(apiKey)

	// Method 1: Load standalone MCP configuration
	fmt.Println("1. Loading standalone MCP configuration...")
	mcpConfig, err := agent.LoadMCPConfigFromYAML("standalone_mcp_config.yaml")
	if err != nil {
		fmt.Printf("Failed to load MCP config: %v\n", err)
		return
	}
	fmt.Printf("✅ Loaded MCP config with %d servers\n", len(mcpConfig.MCPServers))

	// Validate configuration
	if err := agent.ValidateMCPConfig(mcpConfig); err != nil {
		fmt.Printf("❌ Invalid MCP config: %v\n", err)
		return
	}
	fmt.Println("✅ Configuration validation passed")

	// Create agent with MCP configuration
	fmt.Println("\n2. Creating agent with MCP tools...")
	devopsAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMCPConfig(mcpConfig),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithMaxIterations(5),           // Allow tool calling
		agent.WithRequirePlanApproval(false), // Execute tools without approval
	)
	if err != nil {
		fmt.Printf("Failed to create agent: %v\n", err)
		return
	}
	fmt.Println("✅ Agent created with MCP configuration")

	// Show configured tools
	agentConfig := agent.GetMCPConfigFromAgent(devopsAgent)
	fmt.Println("\n3. Available MCP tools:")
	for serverName, server := range agentConfig.MCPServers {
		serverType := server.GetServerType()
		fmt.Printf("   ✅ %s (%s)\n", serverName, serverType)
	}
	fmt.Printf("\n📊 Total: %d servers\n", len(agentConfig.MCPServers))

	// Export configuration to different formats
	fmt.Println("\n4. Exporting configuration...")
	if err := agent.SaveMCPConfigToJSON(mcpConfig, "exported_devops_config.json"); err != nil {
		fmt.Printf("❌ JSON export failed: %v\n", err)
	} else {
		fmt.Println("✅ Configuration exported to JSON")
	}

	if err := agent.SaveMCPConfigToYAML(mcpConfig, "exported_devops_config.yaml"); err != nil {
		fmt.Printf("❌ YAML export failed: %v\n", err)
	} else {
		fmt.Println("✅ Configuration exported to YAML")
	}

	// Test agent execution with MCP tools
	fmt.Println("\n5. Testing agent execution with MCP tools...")
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "standalone-mcp-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "standalone-mcp-conversation")

	testPrompt := "What tools and capabilities do you have available? Can you help me analyze system logs?"
	fmt.Printf("   User: %s\n", testPrompt)

	response, err := devopsAgent.Run(ctx, testPrompt)
	if err != nil {
		fmt.Printf("❌ Agent execution failed: %v\n", err)
		fmt.Println("   Note: Some MCP servers may not be available in this demo environment")
	} else {
		fmt.Printf("   Agent: %s\n", response)
	}

	fmt.Println("\n🎉 Standalone MCP Configuration Demo Complete!")
	fmt.Println("\nFeatures Demonstrated:")
	fmt.Println("• Loading MCP config from standalone YAML file")
	fmt.Println("• Configuration validation")
	fmt.Println("• Agent creation with MCP tools")
	fmt.Println("• Agent execution with real prompts")
	fmt.Println("• Configuration export to JSON/YAML")
}
