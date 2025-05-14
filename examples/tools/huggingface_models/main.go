// Agent with tool to search huggingface models

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"
	toolsregistry "github.com/Ingenimax/agent-sdk-go/pkg/tools"
	hftools "github.com/Ingenimax/agent-sdk-go/pkg/tools/huggingface"
)

type HuggingFaceResponse struct {
	UserResponse       string `json:"user_response" description:"The response to the user's query"`
	MostLikedModelName string `json:"most_liked_model" description:"The most liked model name from the search"`
}

func main() {
	// Create a logger
	logger := logging.New()

	// Get configuration
	cfg := config.Get()

	// Create a new agent with OpenAI
	openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey,
		openai.WithLogger(logger))

	// Create tools registry
	toolRegistry := toolsregistry.NewRegistry()

	// Add Hugging Face model search tool
	hfTool := hftools.New()
	toolRegistry.Register(hfTool)

	responseFormat := structuredoutput.NewResponseFormat(HuggingFaceResponse{})

	// Create the agent with the tools
	agent, err := agent.NewAgent(
		agent.WithLLM(openaiClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(toolRegistry.List()...),
		agent.WithRequirePlanApproval(false),
		agent.WithSystemPrompt(`
		You are a helpful AI assistant that can help users find and learn about machine learning models. 
		When users ask about models, use the Hugging Face search tool to find relevant information.
		You should always use the Hugging Face search tool to find relevant information.
		`),
		agent.WithName("HuggingFaceAssistant"),
		agent.WithResponseFormat(*responseFormat),
	)
	if err != nil {
		logger.Error(context.Background(), "Failed to create agent", map[string]interface{}{"error": err.Error()})
		return
	}

	// Log created agent
	logger.Info(context.Background(), "Created agent with tools", map[string]interface{}{"tools": toolRegistry.List()})

	// Create a context with organization ID and conversation ID
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-123")

	// Example queries to test the agent
	queries := []string{
		"What are some popular text generation models on Hugging Face?",
		"Find me a good model for sentiment analysis",
		"What's the best model for image classification?",
	}

	// Run the agent with each query
	for _, query := range queries {
		fmt.Printf("\nQuery: %s\n", query)
		response, err := agent.Run(ctx, query)
		if err != nil {
			logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
			continue
		}

		var huggingFaceResponse HuggingFaceResponse
		err = json.Unmarshal([]byte(response), &huggingFaceResponse)
		if err != nil {
			logger.Error(ctx, "Failed to unmarshal response", map[string]interface{}{"error": err.Error()})
			continue
		}

		// Print markdown response with line breaks and formatting
		lines := strings.Split(huggingFaceResponse.UserResponse, "\n")
		for _, line := range lines {
			fmt.Printf("Response: %s\n", strings.TrimSpace(line))
		}
		fmt.Printf("Most liked model: %s\n", huggingFaceResponse.MostLikedModelName)
	}
}
