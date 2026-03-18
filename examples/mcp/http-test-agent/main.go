// Simple agent that connects to an MCP server over HTTP for testing.
// Set OPENAI_API_KEY and ensure the MCP server at the URL is reachable.
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
	"github.com/joho/godotenv"
)

// MCP server URL for testing (token in query string).

func main() {
	godotenv.Load()
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")
	mcpServerURL := os.Getenv("MCP_SERVER_URL")

	llm := openai.NewClient(apiKey,
		openai.WithModel("gemini-2.5-flash"),
		openai.WithBaseURL(baseURL),
	)

	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are a helpful assistant with access to MCP tools. Use them when relevant."),
		agent.WithMCPURLs(mcpServerURL),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "mcp-test-conv")

	query := "What tools do you have available? List them briefly."
	if len(os.Args) > 1 {
		query = os.Args[1]
	}

	fmt.Printf("Query: %s\n\n", query)
	response, err := myAgent.Run(ctx, query)
	if err != nil {
		log.Fatalf("Run failed: %v", err)
	}
	fmt.Printf("Response: %s\n", response)
}
