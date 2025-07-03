package main

import (
	"context"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

func main() {
	// Create a logger
	logger := logging.New()

	// Get configuration
	cfg := config.Get()

	// Create a new agent with OpenAI
	openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey,
		openai.WithLogger(logger))

	agent, err := agent.NewAgent(
		agent.WithLLM(openaiClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(createTools(logger).List()...),
		agent.WithSystemPrompt("You are a helpful AI assistant. When you don't know the answer or need real-time information, use the available tools to find the information."),
		agent.WithMaxIterations(5), // Allow up to 5 tool-calling iterations
		agent.WithName("ResearchAssistant"),
	)
	if err != nil {
		logger.Error(context.Background(), "Failed to create agent", map[string]interface{}{"error": err.Error()})
		return
	}

	// Log created agent
	logger.Info(context.Background(), "Created agent with tools", map[string]interface{}{"tools": createTools(logger).List()})

	// Create a context with organization ID and conversation ID
	ctx := context.Background()
	// If you have an organization ID, add it to the context
	// ctx = multitenancy.WithOrgID(ctx, "your-org-id")

	// For testing without an org ID, you can use a default one
	ctx = multitenancy.WithOrgID(ctx, "default-org")

	// Add a conversation ID to the context
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-123")

	// Run the agent with the context that includes the organization ID and conversation ID
	response, err := agent.Run(ctx, "What's the weather in San Francisco?")
	if err != nil {
		logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
		return
	}

	// Log agent response
	logger.Info(ctx, "Agent response", map[string]interface{}{"response": response})

	fmt.Println(response)
}

func createTools(logger logging.Logger) *tools.Registry {
	// Get configuration
	cfg := config.Get()

	// Create tools registry
	toolRegistry := tools.NewRegistry()

	// Add web search tool if API keys are available
	if cfg.Tools.WebSearch.GoogleAPIKey != "" && cfg.Tools.WebSearch.GoogleSearchEngineID != "" {
		logger.Info(context.Background(), "Adding Google Search tool", map[string]interface{}{"engineID": cfg.Tools.WebSearch.GoogleSearchEngineID})
		searchTool := websearch.New(
			cfg.Tools.WebSearch.GoogleAPIKey,
			cfg.Tools.WebSearch.GoogleSearchEngineID,
		)
		toolRegistry.Register(searchTool)
	} else {
		logger.Info(context.Background(), "Skipping Google Search tool - missing API keys", nil)
	}

	return toolRegistry
}
