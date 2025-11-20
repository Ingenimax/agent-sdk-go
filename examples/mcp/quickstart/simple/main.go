package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Simple example showing the easiest way to add MCP servers to an agent
func main() {
	// Get OpenAI API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create LLM client
	llm := openai.NewClient(apiKey, openai.WithModel("gpt-4o-mini"))

	// Create agent with MCP servers using simple URL format
	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are a helpful AI assistant with access to various tools and services."),

		// Add MCP servers using simple URLs
		agent.WithMCPURLs(
			// Local file system access (requires mcp-filesystem to be installed)
			"stdio://filesystem/usr/local/bin/mcp-filesystem",

			// Time/date operations (requires mcp-time to be installed)
			"stdio://time/usr/local/bin/mcp-time",
		),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Create context with organization ID and conversation ID
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-123")

	// Example queries that will use MCP tools
	queries := []string{
		"What's the current date and time?",
		"List the files in the current directory",
		"Create a file called 'hello.txt' with the content 'Hello from MCP!'",
	}

	for i, query := range queries {
		fmt.Printf("\n=== Query %d ===\n", i+1)
		fmt.Printf("User: %s\n", query)

		response, err := myAgent.Run(ctx, query)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		fmt.Printf("Assistant: %s\n", response)
	}
}
