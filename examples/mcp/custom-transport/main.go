package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	sdkmcp "github.com/Ingenimax/agent-sdk-go/pkg/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// HiArgs is the argument type for the SayHi tool.
type HiArgs struct {
	Name string `json:"name" mcp:"the name to say hi to"`
}

// SayHi is a tool handler that responds with a greeting.
func SayHi(ctx context.Context, req *mcp.CallToolRequest, args HiArgs) (*mcp.CallToolResult, struct{}, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Hi " + args.Name},
		},
	}, struct{}{}, nil
}

func main() {
	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "server", Version: "v0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "greet", Description: "say hi"}, SayHi)
	serverSession, err := server.Connect(ctx, t1, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Get API key from environment variable
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create OpenAI LLM client
	llm := openai.NewClient(
		apiKey,
		openai.WithModel("gpt-4o-mini"), // Using a cost-effective model for the example
	)

	// Create MCP server config with the custom transport
	mcpServerCfg := sdkmcp.LazyMCPServerConfig{
		Name:                "CustomTransportMCPServer",
		Type:                "custom",
		CustomTransportType: "inmemory",
		CustomMCPTransport:  t2,
	}

	mcpServer, err := sdkmcp.GetOrCreateServerFromCache(ctx, mcpServerCfg)
	if err != nil {
		log.Println("[ERROR]: Failed to create custom transport server:", err)
		return
	}
	// Create agent with token usage tracking support
	ag, err := agent.NewAgent(
		agent.WithName("CustomTransportAgent"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt("You are a helpful assistant that provides concise answers."),
		agent.WithMCPServers([]interfaces.MCPServer{mcpServer}),
		agent.WithRequirePlanApproval(false),
	)
	if err != nil {
		log.Fatal("Failed to create agent:", err)
	}

	defer func() {
		if err := mcpServer.Close(); err != nil {
			log.Println("[ERROR]: Failed to close MCP server:", err)
		}
		if err := serverSession.Close(); err != nil {
			log.Println("[ERROR]: Failed to close server session:", err)
		}
	}()

	simpleResponse, err := ag.Run(ctx, "Hello from CustomTransportAgent!")
	if err != nil {
		log.Fatal("Failed to run agent:", err)
	}

	fmt.Printf("Response: %s\n\n", simpleResponse)

}
