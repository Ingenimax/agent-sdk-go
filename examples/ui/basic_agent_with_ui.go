package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
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

	// Create agent with tools and configuration
	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithName("UIAssistant"),
		agent.WithDescription("An AI assistant with a beautiful web interface"),
		agent.WithSystemPrompt("You are a helpful AI assistant. Provide clear and concise responses."),
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

	// Create HTTP server with UI
	port := 8080
	server := microservice.NewHTTPServerWithUI(myAgent, port, uiConfig)

	// Start the server
	fmt.Printf("Starting Agent UI server on http://localhost:%d\n", port)
	fmt.Println("Open your browser to interact with the agent!")

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
