// Package main demonstrates image generation capabilities using the Agent SDK.
// It shows three approaches: direct LLM usage, agent with tool, and YAML configuration.
//
// Supports two authentication methods:
//   - GEMINI_API_KEY: Standard API key authentication
//   - Vertex AI: Using VERTEX_AI_PROJECT, VERTEX_AI_REGION, and
//     VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage/local"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/imagegen"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()
	// Add org ID and conversation ID to context for multitenancy support
	ctx = multitenancy.WithOrgID(ctx, "demo-org")
	ctx = memory.WithConversationID(ctx, "demo-conversation")

	fmt.Println("=== Image Generation Examples ===")
	fmt.Println()

	// Run examples
	directLLMExample(ctx)
	agentWithToolExample(ctx)
	yamlConfigExample(ctx)
}

// getGeminiOptions returns the appropriate Gemini client options based on environment.
// Supports both API key and Vertex AI authentication.
func getGeminiOptions(model string) ([]gemini.Option, error) {
	opts := []gemini.Option{gemini.WithModel(model)}

	// Check for Vertex AI credentials first
	vertexCreds := os.Getenv("VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT")
	vertexProject := os.Getenv("VERTEX_AI_PROJECT")
	vertexRegion := os.Getenv("VERTEX_AI_REGION")

	if vertexCreds != "" && vertexProject != "" {
		if vertexRegion == "" {
			vertexRegion = "us-central1"
		}
		// Decode base64 credentials if needed
		credentialsJSON := []byte(vertexCreds)
		if decoded, err := base64.StdEncoding.DecodeString(vertexCreds); err == nil {
			credentialsJSON = decoded
		}
		opts = append(opts,
			gemini.WithBackend(genai.BackendVertexAI),
			gemini.WithCredentialsJSON(credentialsJSON),
			gemini.WithProjectID(vertexProject),
			gemini.WithLocation(vertexRegion),
		)
		return opts, nil
	}

	// Fall back to API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey != "" {
		opts = append(opts, gemini.WithAPIKey(apiKey))
		return opts, nil
	}

	return nil, fmt.Errorf("no credentials found: set GEMINI_API_KEY or Vertex AI environment variables")
}

// directLLMExample shows how to generate images directly using the Gemini client.
func directLLMExample(ctx context.Context) {
	fmt.Println("1. Direct LLM Image Generation")
	fmt.Println("-------------------------------")

	// Get client options based on available credentials
	opts, err := getGeminiOptions(gemini.ModelGemini25FlashImage)
	if err != nil {
		log.Printf("Configuration error: %v\n\n", err)
		return
	}

	// Create Gemini client with image generation model
	client, err := gemini.NewClient(ctx, opts...)
	if err != nil {
		log.Printf("Failed to create client: %v\n\n", err)
		return
	}

	// Verify image generation support
	if !client.SupportsImageGeneration() {
		log.Println("Model does not support image generation")
		fmt.Println()
		return
	}

	fmt.Printf("Supported formats: %v\n", client.SupportedImageFormats())

	// Generate an image
	response, err := client.GenerateImage(ctx, interfaces.ImageGenerationRequest{
		Prompt: "A minimalist logo of a blue mountain with a rising sun",
		Options: &interfaces.ImageGenerationOptions{
			AspectRatio:  "1:1",
			OutputFormat: "png",
		},
	})
	if err != nil {
		log.Printf("Generation failed: %v\n\n", err)
		return
	}

	// Display results
	for i, img := range response.Images {
		fmt.Printf("Image %d: %s, %d bytes\n", i+1, img.MimeType, len(img.Data))
	}
	fmt.Println()
}

// agentWithToolExample shows how to use image generation as an agent tool.
func agentWithToolExample(ctx context.Context) {
	fmt.Println("2. Agent with Image Generation Tool")
	fmt.Println("------------------------------------")

	// Get client options for image generation model
	imageOpts, err := getGeminiOptions(gemini.ModelGemini25FlashImage)
	if err != nil {
		log.Printf("Configuration error: %v\n\n", err)
		return
	}

	// Create image generation client
	imageClient, err := gemini.NewClient(ctx, imageOpts...)
	if err != nil {
		log.Printf("Failed to create image client: %v\n\n", err)
		return
	}

	// Create local storage for generated images
	imgStorage, err := local.New(storage.LocalConfig{
		Path:    "/tmp/agent-sdk-images",
		BaseURL: "file:///tmp/agent-sdk-images",
	})
	if err != nil {
		log.Printf("Failed to create storage: %v\n\n", err)
		return
	}

	// Create image generation tool
	imgTool := imagegen.New(imageClient, imgStorage,
		imagegen.WithMaxPromptLength(1000),
		imagegen.WithDefaultAspectRatio("16:9"),
	)

	// Get client options for text model
	textOpts, err := getGeminiOptions(gemini.ModelGemini25Flash)
	if err != nil {
		log.Printf("Configuration error: %v\n\n", err)
		return
	}

	// Create text LLM for the agent
	textClient, err := gemini.NewClient(ctx, textOpts...)
	if err != nil {
		log.Printf("Failed to create text client: %v\n\n", err)
		return
	}

	// Create agent with image generation capability
	ag, err := agent.NewAgent(
		agent.WithLLM(textClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(imgTool),
		agent.WithSystemPrompt("You are a helpful assistant that can generate images. When asked to create an image, use the generate_image tool with a detailed prompt."),
		agent.WithRequirePlanApproval(false),
	)
	if err != nil {
		log.Printf("Failed to create agent: %v\n\n", err)
		return
	}

	// Ask agent to generate an image
	response, err := ag.Run(ctx, "Create an image of a futuristic city at night")
	if err != nil {
		log.Printf("Agent failed: %v\n\n", err)
		return
	}

	fmt.Printf("Agent response: %s\n\n", response)
}

// yamlConfigExample shows how to use YAML configuration for image generation.
func yamlConfigExample(ctx context.Context) {
	fmt.Println("3. YAML Configuration Example")
	fmt.Println("------------------------------")

	// Check if config file exists
	configPath := "examples/image_generation/agents.yaml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("Skipping: agents.yaml not found")
		fmt.Println("See agents.yaml.example for configuration template")
		fmt.Println()
		return
	}

	// Load agent from YAML configuration
	configs, err := agent.LoadAgentConfigsFromFile(configPath)
	if err != nil {
		log.Printf("Failed to load config: %v\n\n", err)
		return
	}

	// Create agent from config
	ag, err := agent.NewAgentFromConfig("creative_agent", configs, nil)
	if err != nil {
		log.Printf("Failed to create agent: %v\n\n", err)
		return
	}

	// Use the agent
	response, err := ag.Run(ctx, "Generate a simple icon of a lightbulb")
	if err != nil {
		log.Printf("Agent failed: %v\n\n", err)
		return
	}

	fmt.Printf("Agent response: %s\n\n", response)
}
