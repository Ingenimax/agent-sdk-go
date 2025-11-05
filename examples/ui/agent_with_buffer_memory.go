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
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create LLM client
	llm := openai.NewClient(apiKey)

	// Create conversation buffer memory (keeps last 10 messages)
	bufferMemory := memory.NewConversationBuffer(
		memory.WithMaxSize(10), // Keep last 10 messages
	)

	// Create agent with buffer memory
	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithName("UIAgentWithBuffer"),
		agent.WithDescription("An AI assistant with conversation buffer memory and web interface"),
		agent.WithSystemPrompt("You are a helpful AI assistant with conversation memory. You can remember the last 10 messages of our conversation."),
		agent.WithMemory(bufferMemory),
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
			Memory:    true,  // Enable memory browser
			AgentInfo: true,
			Settings:  true,
		},
	}

	// Get port from environment or use default
	port := 8081
	if portStr := os.Getenv("PORT"); portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}

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
	fmt.Printf("Starting Agent UI server with buffer memory on http://localhost:%d\n", port)
	fmt.Println("\nFeatures:")
	fmt.Println("  ✅ Conversation buffer memory - remembers last 10 messages")
	fmt.Println("  ✅ Memory browser - view conversation history")
	fmt.Println("  ✅ Search functionality - find past conversations")
	fmt.Println("  ✅ Light theme")
	fmt.Println("  ✅ Real-time streaming chat")

	fmt.Println("\n🚀 Open your browser and start chatting!")
	fmt.Println("   The agent will remember your last 10 messages.")
	fmt.Println("   Click 'View History' to see the agent's memory.")
	fmt.Println("\n💡 Try having a longer conversation to see how the buffer works!")
	fmt.Println("   Older messages will be automatically removed when the buffer is full.")
	fmt.Println("\nPress Ctrl+C to stop the server")

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}