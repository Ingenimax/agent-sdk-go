package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Example showing how to use the MCP builder for advanced configuration
func main() {
	// Get OpenAI API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create LLM client
	llm := openai.NewClient(apiKey, openai.WithModel("gpt-4o-mini"))

	// Build MCP configuration with custom settings
	builder := mcp.NewBuilder().
		// Configure retry behavior - 3 attempts with 2 second initial delay
		WithRetry(3, 2*time.Second).
		// Set connection timeout to 30 seconds
		WithTimeout(30 * time.Second).
		// Enable health checks to verify servers on startup
		WithHealthCheck(true)

	// Add different types of MCP servers
	builder.
		// Add a stdio-based server (local command)
		AddStdioServer("time-server", "date", "+%Y-%m-%d %H:%M:%S").
		// Add an HTTP-based server (if running locally)
		AddHTTPServer("local-api", "http://localhost:8080/mcp").
		// Add a server with authentication
		AddHTTPServerWithAuth("secure-api", "https://api.example.com/mcp", "your-token").
		// Add preset servers
		AddPreset("filesystem")

	// Build the configuration (this will create lazy configurations)
	lazyConfigs, err := builder.BuildLazy()
	if err != nil {
		log.Fatalf("Failed to build MCP configuration: %v", err)
	}

	fmt.Printf("Created %d MCP server configurations\n", len(lazyConfigs))
	for _, config := range lazyConfigs {
		fmt.Printf("- %s (%s)\n", config.Name, config.Type)
	}

	// Convert mcp.LazyMCPServerConfig to agent.LazyMCPConfig
	agentConfigs := make([]agent.LazyMCPConfig, len(lazyConfigs))
	for i, config := range lazyConfigs {
		agentConfigs[i] = agent.LazyMCPConfig{
			Name:    config.Name,
			Type:    config.Type,
			Command: config.Command,
			Args:    config.Args,
			Env:     config.Env,
			URL:     config.URL,
		}
	}

	// Create agent with the built MCP configuration
	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are a helpful AI assistant with access to various tools and services."),

		// Use the converted lazy configurations - servers will be initialized when first used
		agent.WithLazyMCPConfigs(agentConfigs),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Create context with organization ID and conversation ID
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-123")

	// Example queries
	queries := []string{
		"What files are in the current directory?",
		"What's the current date and time?",
		"Create a file called 'test.txt' with some sample content",
	}

	for i, query := range queries {
		fmt.Printf("\n=== Example %d ===\n", i+1)
		fmt.Printf("User: %s\n", query)

		response, err := myAgent.Run(ctx, query)
		if err != nil {
			fmt.Printf("Error: %v\n", err)

			// Handle MCP errors gracefully
			if mcpErr, ok := err.(*mcp.MCPError); ok {
				fmt.Printf("Error type: %s\n", mcpErr.ErrorType)
				fmt.Printf("Retryable: %t\n", mcpErr.IsRetryable())
				fmt.Printf("User-friendly: %s\n", mcp.FormatUserFriendlyError(mcpErr))
			}
			continue
		}

		fmt.Printf("Assistant: %s\n", response)
	}

	// Example: Building servers eagerly for immediate initialization
	fmt.Println("\n=== Eager Initialization Example ===")

	eagerBuilder := mcp.NewBuilder().
		WithHealthCheck(true). // This will initialize servers immediately
		AddPreset("time")

	servers, _, err := eagerBuilder.Build(context.Background())
	if err != nil {
		log.Printf("Failed to build servers eagerly: %v", err)
	} else {
		fmt.Printf("Successfully initialized %d servers eagerly\n", len(servers))

		// You can now use these servers directly or add them to an agent
		eagerAgent, err := agent.NewAgent(
			agent.WithLLM(llm),
			agent.WithMCPServers(servers),
		)
		if err == nil {
			response, err := eagerAgent.Run(ctx, "What time is it?")
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Printf("Eager agent response: %s\n", response)
			}
		}
	}
}
