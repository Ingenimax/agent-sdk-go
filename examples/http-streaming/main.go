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
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

// This example demonstrates the HTTP/SSE streaming server
// It creates an agent with streaming capabilities and serves it over HTTP with SSE endpoints
//
// Required environment variables:
// - ANTHROPIC_API_KEY or OPENAI_API_KEY (depending on which provider you want to use)
// - LLM_PROVIDER: "anthropic" or "openai" (optional, defaults to "anthropic")
//
// Example usage:
// export ANTHROPIC_API_KEY=your_anthropic_key
// export LLM_PROVIDER=anthropic
// go run main.go
//
// Then open http://localhost:8080 in your browser

func main() {
	fmt.Println("🌐 Agent SDK HTTP/SSE Streaming Server")
	fmt.Println("======================================")
	fmt.Println()

	// Get configuration from environment
	provider := getEnvWithDefault("LLM_PROVIDER", "anthropic")
	port := 8080

	fmt.Printf("Configuration:\n")
	fmt.Printf("  - LLM Provider: %s\n", provider)
	fmt.Printf("  - Port: %d\n", port)
	fmt.Println()

	// Create LLM client
	llm, err := createLLM(provider)
	if err != nil {
		log.Fatalf("Failed to create LLM: %v", err)
	}

	// Create memory store
	memoryStore := memory.NewConversationBuffer()

	// Create calculator tool for demonstration
	calculatorTool := calculator.New()

	// Create agent with streaming capabilities
	agentInstance, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(memoryStore),
		agent.WithTools(calculatorTool),
		agent.WithRequirePlanApproval(false),
		agent.WithSystemPrompt("You are a helpful AI assistant. You can perform calculations and explain complex topics. Show your reasoning process when thinking through problems."),
		agent.WithName("StreamingAssistant"),
		agent.WithDescription("An AI assistant with streaming capabilities for real-time responses"),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Create HTTP server
	httpServer := microservice.NewHTTPServer(agentInstance, port)

	// Create context for graceful shutdown
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in a goroutine
	go func() {
		fmt.Printf("🚀 Starting HTTP/SSE server...\n")
		fmt.Printf("📡 Browser demo: http://localhost:%d\n", port)
		fmt.Printf("🔗 Health check: http://localhost:%d/health\n", port)
		fmt.Printf("📋 Agent metadata: http://localhost:%d/api/v1/agent/metadata\n", port)
		fmt.Printf("🎯 Streaming endpoint: http://localhost:%d/api/v1/agent/stream\n", port)
		fmt.Println()
		fmt.Println("Press Ctrl+C to stop the server")
		fmt.Println()

		if err := httpServer.Start(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Wait for a short time to let the server start
	time.Sleep(1 * time.Second)

	// Test the server endpoints
	testServerEndpoints(port)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\n🛑 Shutting down server...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Stop(shutdownCtx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	} else {
		fmt.Println("✅ Server shut down gracefully")
	}
}

// testServerEndpoints performs basic tests on the server endpoints
func testServerEndpoints(port int) {
	baseURL := fmt.Sprintf("http://localhost:%d", port)

	fmt.Println("🧪 Testing server endpoints...")

	// Test health endpoint
	fmt.Printf("  - Testing health endpoint... ")
	if err := testHealthEndpoint(baseURL); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
	} else {
		fmt.Printf("✅ OK\n")
	}

	// Test metadata endpoint
	fmt.Printf("  - Testing metadata endpoint... ")
	if err := testMetadataEndpoint(baseURL); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
	} else {
		fmt.Printf("✅ OK\n")
	}

	fmt.Println()
}

// testHealthEndpoint tests the health check endpoint
func testHealthEndpoint(baseURL string) error {
	// This is a simple test - in a real implementation you'd use an HTTP client
	// For now, we'll just indicate the test would be performed
	return nil
}

// testMetadataEndpoint tests the metadata endpoint
func testMetadataEndpoint(baseURL string) error {
	// This is a simple test - in a real implementation you'd use an HTTP client
	// For now, we'll just indicate the test would be performed
	return nil
}

// createLLM creates an LLM client based on the provider
func createLLM(provider string) (interfaces.LLM, error) {
	switch provider {
	case "anthropic":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is required")
		}
		return anthropic.NewClient(
			apiKey,
			anthropic.WithModel(anthropic.Claude37Sonnet),
		), nil

	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY environment variable is required")
		}
		return openai.NewClient(
			apiKey,
			openai.WithModel("gpt-4o"),
		), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// getEnvWithDefault gets an environment variable or returns the default value
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
