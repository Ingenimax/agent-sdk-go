//go:build test
// +build test

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create LLM client
	llm := openai.NewClient(apiKey)

	// Create conversation buffer memory
	bufferMemory := memory.NewConversationBuffer(
		memory.WithMaxSize(10),
	)

	// Create tools registry with some example tools
	toolRegistry := tools.NewRegistry()

	// Add web search tool if we have Google API credentials
	googleAPIKey := os.Getenv("GOOGLE_API_KEY")
	googleSearchEngineID := os.Getenv("GOOGLE_SEARCH_ENGINE_ID")

	if googleAPIKey != "" && googleSearchEngineID != "" {
		searchTool := websearch.New(googleAPIKey, googleSearchEngineID)
		toolRegistry.Register(searchTool)
		fmt.Println("✅ Web search tool added")
	} else {
		fmt.Println("ℹ️  Web search tool not added (missing Google API credentials)")
	}

	// Get all tools from registry
	allTools := toolRegistry.List()

	// Create agent with tools and memory
	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithName("UITestAgent"),
		agent.WithDescription("A test AI assistant with tools and UI for testing purposes"),
		agent.WithSystemPrompt("You are a helpful AI assistant with access to web search and other tools. You can help users find information and answer questions."),
		agent.WithMemory(bufferMemory),
		agent.WithTools(allTools...),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Create UI configuration
	uiConfig := &microservice.UIConfig{
		Enabled:     true,
		DefaultPath: "/",
		DevMode:     false,
		Theme:       "light",
		Features: microservice.UIFeatures{
			Chat:      true,
			Memory:    true,
			AgentInfo: true,
			Settings:  true,
		},
	}

	// Use port 3030 as specified
	port := 3030

	// Create HTTP server with UI
	server := microservice.NewHTTPServerWithUI(myAgent, port, uiConfig)

	// Setup graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		fmt.Println("\nShutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Stop(ctx); err != nil {
			log.Printf("Error shutting down server: %v", err)
		}

		os.Exit(0)
	}()

	// Start the server
	fmt.Printf("Starting Test Agent UI server on http://localhost:%d\n", port)
	fmt.Println("\nFeatures:")
	fmt.Println("  ✅ Conversation buffer memory")
	fmt.Printf("  ✅ %d tools available\n", len(allTools))
	fmt.Println("  ✅ Tab-based navigation (Chat, Agent Info, Tools, Memory, Sub-Agents, Settings)")
	fmt.Println("  ✅ Real-time streaming chat")

	fmt.Println("\n🚀 Open your browser and test the new UI!")
	fmt.Println("   - Try the Tools tab to see available tools")
	fmt.Println("   - Check Agent Info for agent configuration")
	fmt.Println("   - Browse Memory to see conversation history")
	fmt.Println("   - Sub-Agents tab shows sub-agent management")
	fmt.Println("\nPress Ctrl+C to stop the server")

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
