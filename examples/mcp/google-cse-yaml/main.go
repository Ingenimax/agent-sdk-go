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

	agentConfigs, err := agent.LoadAgentConfigsFromFile("agent.yaml")
	if err != nil {
		log.Fatalf("Failed to load agent configuration: %v", err)
	}

	myAgent, err := agent.NewAgentFromConfig(
		"web_search_agent",
		agentConfigs,
		nil,
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
	)
	if err != nil {
		log.Fatalf("Failed to create agent from config: %v", err)
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
