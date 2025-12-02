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

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	googleAPIKey := os.Getenv("API_KEY")
	if googleAPIKey == "" {
		log.Fatal("API_KEY environment variable is required for Google Custom Search")
	}

	engineID := os.Getenv("ENGINE_ID")
	if engineID == "" {
		log.Fatal("ENGINE_ID environment variable is required for Google Custom Search")
	}

	llm := openai.NewClient(apiKey, openai.WithModel("gpt-4o-mini"))

	lazyMCPConfigs := []agent.LazyMCPConfig{
		{
			Name:    "google-cse",
			Type:    "stdio",
			Command: "uvx",
			Args:    []string{"mcp-google-cse"},
			Env: []string{
				fmt.Sprintf("API_KEY=%s", googleAPIKey),
				fmt.Sprintf("ENGINE_ID=%s", engineID),
			},
			Tools: []agent.LazyMCPToolConfig{
				{
					Name:        "google_search",
					Description: "Searches the custom search engine using the search term and returns a list of results",
					Schema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"search_term": map[string]interface{}{
								"type":        "string",
								"description": "The search query",
							},
						},
						"required": []string{"search_term"},
					},
				},
			},
		},
	}

	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithLazyMCPConfigs(lazyMCPConfigs),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are a helpful AI assistant that can search the web to find information. Always use the google_search tool to find current and accurate information."),
		agent.WithName("WebSearchAgent"),
		agent.WithRequirePlanApproval(false),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "web-search-demo")

	query := "What are the best websites for learning AI in 2025?"
	fmt.Printf("User: %s\n\n", query)

	response, err := myAgent.Run(ctx, query)
	if err != nil {
		log.Fatalf("Failed to run agent: %v", err)
	}

	fmt.Printf("Agent Response:\n%s\n", response)
}
