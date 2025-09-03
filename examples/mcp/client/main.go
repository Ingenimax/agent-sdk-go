package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func main() {
	logger := log.New(os.Stderr, "AGENT: ", log.LstdFlags)

	// Example: create an OpenAI LLM (replace with your API key and model)
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		logger.Println("Warning: OPENAI_API_KEY environment variable not set.")
		logger.Fatal("Please set the OPENAI_API_KEY environment variable.")
	}

	llm := openai.NewClient(apiKey, openai.WithModel("gpt-4o-mini"))

	// Create MCP servers
	var mcpServers []interfaces.MCPServer

	// Create an HTTP-based MCP server client
	httpServer, err := mcp.NewHTTPServer(context.Background(), mcp.HTTPServerConfig{
		BaseURL: "http://localhost:8083/mcp",
	})
	if err != nil {
		logger.Printf("Warning: Failed to initialize HTTP MCP server: %v", err)
		logger.Println("Continuing without MCP server support.")
	} else {
		mcpServers = append(mcpServers, httpServer)
		logger.Println("Successfully initialized HTTP MCP server.")

		// Example: List tools from MCP servers
		fmt.Println("Available HTTP MCP tools:")
		tools, err := httpServer.ListTools(context.Background())
		if err != nil {
			logger.Printf("Failed to list tools: %v", err)
		} else {
			for _, tool := range tools {
				fmt.Printf("- %s: %s\n", tool.Name, tool.Description)
			}
		}
	}

	stdioServer, err := mcp.NewStdioServer(context.Background(), mcp.StdioServerConfig{
		Command: "go",
		Args:    []string{"run", "./server-stdio/main.go"},
	})
	if err != nil {
		logger.Printf("Warning: Failed to initialize STDIO MCP server: %v", err)
		logger.Println("Continuing without MCP server support.")
	} else {
		mcpServers = append(mcpServers, stdioServer)
		logger.Println("Successfully initialized STDIO MCP server.")

		// Example: List tools from MCP servers
		fmt.Println("Available STDIO MCP tools:")
		tools, err := stdioServer.ListTools(context.Background())
		if err != nil {
			logger.Printf("Failed to list tools: %v", err)
		}
		for _, tool := range tools {
			fmt.Printf("- %s: %s\n", tool.Name, tool.Description)
		}
	}

	// Create the agent with MCP server support
	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMCPServers(mcpServers),
		agent.WithRequirePlanApproval(false),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are an AI assistant that answers questions and can use tools from MCP servers."),
		agent.WithName("MCPAgentExample"),
	)
	if err != nil {
		logger.Fatalf("Failed to create agent: %v", err)
	}

	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "structured-response-demo")

	inputs := []string{
		"Use the time tool to get the current time in RFC3339 format.", // http tool
		"What should I drink to focus?",                                // http tool
		"What should I eat for breakfast?",                             // stdio tool
	}

	for _, input := range inputs {
		fmt.Println("\nRunning agent with query that might use MCP tools:", input)
		response, err := myAgent.Run(ctx, input)
		if err != nil {
			logger.Fatalf("Agent run failed: %v", err)
		}

		fmt.Println("\nAgent response:", response)
	}
}
