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
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		// Try OpenAI as fallback
		apiKey = os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			log.Fatal("ANTHROPIC_API_KEY or OPENAI_API_KEY environment variable is required")
		}
	}

	// Create LLM client (Claude)
	llm := anthropic.NewClient(apiKey, anthropic.WithModel("claude-3-opus-20240229"))

	// Create agent with advanced configuration
	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithName("AdvancedUIAssistant"),
		agent.WithDescription("An advanced AI assistant with web interface, tools, and memory"),
		agent.WithSystemPrompt(`You are an advanced AI assistant with the following capabilities:
- Beautiful web interface for user interaction

Provide helpful, accurate, and well-structured responses.`),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Create UI configuration with custom settings
	uiConfig := &microservice.UIConfig{
		Enabled:     true,
		DefaultPath: "/",
		DevMode:     os.Getenv("DEV_MODE") == "true",
		Theme:       "dark", // Start with dark theme
		Features: microservice.UIFeatures{
			Chat:      true,
			Memory:    true,
			AgentInfo: true,
			Settings:  true,
		},
	}

	// Get port from environment or use default
	port := 8080
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
	fmt.Printf("Starting Advanced Agent UI server on http://localhost:%d\n", port)
	fmt.Println("Features enabled:")
	fmt.Println("  - Modern web interface")
	fmt.Println("  - Dark theme")
	fmt.Println("  - Real-time chat")
	fmt.Println("\nOpen your browser to interact with the agent!")
	fmt.Println("Press Ctrl+C to stop the server")

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
