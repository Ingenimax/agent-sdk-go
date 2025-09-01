package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

// This example demonstrates how to use Anthropic Claude models via Google Cloud Vertex AI.
// Before running, ensure you have:
//
// 1. Google Cloud project with Vertex AI API enabled
// 2. Authentication set up (one of the following):
//    - Application Default Credentials: `gcloud auth application-default login`
//    - Service Account Key: Set GOOGLE_APPLICATION_CREDENTIALS environment variable
//    - Or provide explicit credentials path
//
// Required environment variables:
// - GOOGLE_CLOUD_PROJECT: Your Google Cloud project ID
// - GOOGLE_CLOUD_REGION: Region where Vertex AI is available (e.g., us-central1)
//
// Optional:
// - GOOGLE_APPLICATION_CREDENTIALS: Path to service account key (if not using ADC)
//
// Example setup:
// export GOOGLE_CLOUD_PROJECT=my-project-id
// export GOOGLE_CLOUD_REGION=us-central1
// gcloud auth application-default login
// go run main.go

func main() {
	// Get required configuration from environment
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		fmt.Println("GOOGLE_CLOUD_PROJECT environment variable is required")
		fmt.Println("Set it to your Google Cloud project ID")
		os.Exit(1)
	}

	region := os.Getenv("GOOGLE_CLOUD_REGION")
	if region == "" {
		region = "us-central1" // Default region
		fmt.Printf("GOOGLE_CLOUD_REGION not set, using default: %s\n", region)
	}

	// Check if the region supports Anthropic models
	if !anthropic.IsRegionSupported(region) {
		fmt.Printf("Warning: Region '%s' may not support Anthropic models on Vertex AI\n", region)
		fmt.Printf("Supported regions: %v\n", anthropic.GetSupportedRegions())
	}

	ctx := context.Background()

	fmt.Printf("Anthropic on Vertex AI Examples\n")
	fmt.Printf("=================================\n")
	fmt.Printf("Project: %s\n", projectID)
	fmt.Printf("Region:  %s\n", region)
	fmt.Printf("\n")

	// Example 1: Basic generation with Application Default Credentials
	fmt.Println("Example 1: Basic generation with Application Default Credentials")
	fmt.Println("---------------------------------------------------------------")
	basicGenerationWithADC(ctx, projectID, region)
	fmt.Println()

	// Example 2: Basic generation with explicit credentials
	credentialsPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if credentialsPath != "" {
		fmt.Println("Example 2: Basic generation with explicit credentials")
		fmt.Println("----------------------------------------------------")
		basicGenerationWithCredentials(ctx, projectID, region, credentialsPath)
		fmt.Println()
	}

	// Example 3: Tool usage with Vertex AI
	fmt.Println("Example 3: Tool usage with Vertex AI")
	fmt.Println("------------------------------------")
	toolUsageExample(ctx, projectID, region)
	fmt.Println()

	// Example 4: Streaming example with Vertex AI
	fmt.Println("Example 4: Streaming with Vertex AI")
	fmt.Println("-----------------------------------")
	streamingExample(ctx, projectID, region)
	fmt.Println()
}

func basicGenerationWithADC(ctx context.Context, projectID, region string) {
	// Create Anthropic client configured for Vertex AI with Application Default Credentials
	client := anthropic.NewClient(
		"", // No API key needed for Vertex AI
		anthropic.WithModel("claude-sonnet-4@20250514"), // Use Vertex AI model format
		anthropic.WithVertexAI(region, projectID),
	)

	// Generate text
	response, err := client.Generate(
		ctx,
		"Explain quantum computing in one paragraph.",
		anthropic.WithTemperature(0.7),
	)
	if err != nil {
		fmt.Printf("Error generating text: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", response)
}

func basicGenerationWithCredentials(ctx context.Context, projectID, region, credentialsPath string) {
	// Create Anthropic client configured for Vertex AI with explicit credentials
	client := anthropic.NewClient(
		"", // No API key needed for Vertex AI
		anthropic.WithModel("claude-sonnet-4@20250514"), // Use different model for variety
		anthropic.WithVertexAICredentials(region, projectID, credentialsPath),
	)

	// Generate text
	response, err := client.Generate(
		ctx,
		"Write a haiku about artificial intelligence.",
		anthropic.WithTemperature(0.8),
		anthropic.WithSystemMessage("You are a creative poetry assistant."),
	)
	if err != nil {
		fmt.Printf("Error generating text: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", response)
}

func toolUsageExample(ctx context.Context, projectID, region string) {
	// Add organization ID to context for tools
	ctx = multitenancy.WithOrgID(ctx, "vertex-ai-example")

	// Create Anthropic client configured for Vertex AI
	client := anthropic.NewClient(
		"",
		anthropic.WithModel("claude-sonnet-4@20250514"),
		anthropic.WithVertexAI(region, projectID),
	)

	// Create calculator tool
	calculatorTool := calculator.New()

	// Generate text with tools
	response, err := client.GenerateWithTools(
		ctx,
		"What is 15 * 23, and then add 45 to the result?",
		[]interfaces.Tool{calculatorTool},
		anthropic.WithSystemMessage("You are a helpful assistant. Use tools when appropriate."),
		anthropic.WithTemperature(0.2),
	)
	if err != nil {
		fmt.Printf("Error generating text with tools: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", response)
}

func streamingExample(ctx context.Context, projectID, region string) {
	// Create Anthropic client configured for Vertex AI
	client := anthropic.NewClient(
		"",
		anthropic.WithModel("claude-sonnet-4@20250514"),
		anthropic.WithVertexAI(region, projectID),
	)

	// Check if client supports streaming
	if !client.SupportsStreaming() {
		fmt.Println("Client does not support streaming")
		return
	}

	// Generate streaming response
	eventChan, err := client.GenerateStream(
		ctx,
		"Tell me a short story about a robot learning to paint.",
		anthropic.WithTemperature(0.8),
		anthropic.WithSystemMessage("You are a creative storytelling assistant."),
	)
	if err != nil {
		fmt.Printf("Error starting streaming generation: %v\n", err)
		return
	}

	fmt.Print("Streaming response: ")
	for event := range eventChan {
		switch event.Type {
		case "content_delta":
			fmt.Print(event.Content)
		case "error":
			fmt.Printf("\nError: %v\n", event.Error)
			return
		case "message_stop":
			fmt.Println("\n[Stream complete]")
			return
		}
	}
}
