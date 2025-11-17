package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Example showing how to use predefined MCP server presets
func main() {
	// Get API keys from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Set up other environment variables for MCP servers
	// Uncomment and set these as needed:
	// os.Setenv("GITHUB_TOKEN", "your-github-token")
	// os.Setenv("BRAVE_API_KEY", "your-brave-api-key")

	// Create LLM client
	llm := openai.NewClient(apiKey, openai.WithModel("gpt-4o-mini"))

	// List available presets
	fmt.Println("Available MCP server presets:")
	presets := mcp.ListPresets()
	for _, preset := range presets {
		info, err := mcp.GetPresetInfo(preset)
		if err == nil {
			fmt.Printf("- %s: %s\n", preset, info)
		}
	}
	fmt.Println()

	// Create agent with preset MCP servers
	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are a helpful AI assistant with access to various tools and services."),

		// Add common MCP servers using presets
		agent.WithMCPPresets(
			"filesystem", // File system operations
			"time",       // Date/time operations
			"fetch",      // HTTP requests
			// "github",     // Uncomment if GITHUB_TOKEN is set
			// "brave-search", // Uncomment if BRAVE_API_KEY is set
		),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Create context with organization ID and conversation ID
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-123")

	// Example queries that will use preset MCP tools
	queries := []string{
		"What's the current timestamp in RFC3339 format?",
		"Make an HTTP GET request to https://api.github.com and tell me about the response",
		"Check if there's a README.md file in the current directory",
	}

	for i, query := range queries {
		fmt.Printf("\n=== Example %d ===\n", i+1)
		fmt.Printf("User: %s\n", query)

		response, err := myAgent.Run(ctx, query)
		if err != nil {
			fmt.Printf("Error: %v\n", err)

			// Show user-friendly error message
			if mcpErr, ok := err.(*mcp.MCPError); ok {
				fmt.Printf("Friendly error: %s\n", mcp.FormatUserFriendlyError(mcpErr))
			}
			continue
		}

		fmt.Printf("Assistant: %s\n", response)
	}
}
