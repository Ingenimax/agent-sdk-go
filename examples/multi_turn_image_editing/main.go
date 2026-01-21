// Package main demonstrates multi-turn image editing capabilities using the Agent SDK.
// This example shows how to use conversational image generation where you can
// iteratively refine images through multiple exchanges.
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
	"path/filepath"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Multi-Turn Image Editing Example ===")
	fmt.Println()

	// Run the multi-turn editing example
	if err := multiTurnEditingExample(ctx); err != nil {
		log.Fatalf("Example failed: %v", err)
	}
}

// getGeminiOptions returns the appropriate Gemini client options based on environment.
func getGeminiOptions(model string) ([]gemini.Option, error) {
	opts := []gemini.Option{gemini.WithModel(model)}

	// Check for Vertex AI credentials first
	vertexCreds := os.Getenv("VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT")
	vertexProject := os.Getenv("VERTEX_AI_PROJECT")
	vertexRegion := os.Getenv("VERTEX_AI_REGION")

	if vertexCreds != "" && vertexProject != "" {
		fmt.Println("Using Vertex AI authentication")

		// Decode base64 credentials if needed
		var credsJSON []byte
		decoded, err := base64.StdEncoding.DecodeString(vertexCreds)
		if err == nil && len(decoded) > 0 && decoded[0] == '{' {
			credsJSON = decoded
		} else {
			credsJSON = []byte(vertexCreds)
		}

		if vertexRegion == "" {
			vertexRegion = "us-central1"
		}

		opts = append(opts,
			gemini.WithBackend(genai.BackendVertexAI),
			gemini.WithCredentialsJSON(credsJSON),
			gemini.WithProjectID(vertexProject),
			gemini.WithLocation(vertexRegion),
		)
		return opts, nil
	}

	// Fall back to API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no credentials found: set GEMINI_API_KEY or Vertex AI credentials")
	}

	fmt.Println("Using API key authentication")
	opts = append(opts, gemini.WithAPIKey(apiKey))
	return opts, nil
}

// multiTurnEditingExample demonstrates iterative image creation and refinement.
func multiTurnEditingExample(ctx context.Context) error {
	fmt.Println("--- Multi-Turn Image Editing ---")
	fmt.Println()

	// Get client options - use the multi-turn editing model
	model := gemini.ModelGemini3ProImagePreview
	fmt.Printf("Using model: %s\n", model)

	opts, err := getGeminiOptions(model)
	if err != nil {
		return fmt.Errorf("failed to get Gemini options: %w", err)
	}

	// Create Gemini client
	client, err := gemini.NewClient(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Gemini client: %w", err)
	}

	// Check if the model supports multi-turn image editing
	if !client.SupportsMultiTurnImageEditing() {
		return fmt.Errorf("model %s does not support multi-turn image editing", model)
	}

	fmt.Println("Model supports multi-turn image editing!")
	fmt.Println()

	// Create an image editing session
	session, err := client.CreateImageEditSession(ctx, &interfaces.ImageEditSessionOptions{
		Model: model,
	})
	if err != nil {
		return fmt.Errorf("failed to create image edit session: %w", err)
	}
	defer session.Close()

	fmt.Println("Session created successfully!")
	fmt.Println()

	// --- Turn 1: Generate initial image ---
	fmt.Println("Turn 1: Generating initial image...")
	fmt.Println("Prompt: Create a simple infographic showing the water cycle with clouds, rain, and a river")
	fmt.Println()

	resp1, err := session.SendMessage(ctx, "Create a simple infographic showing the water cycle with clouds, rain, and a river", &interfaces.ImageEditOptions{
		AspectRatio: "16:9",
		ImageSize:   "1K",
	})
	if err != nil {
		return fmt.Errorf("turn 1 failed: %w", err)
	}

	printResponse("Turn 1 Response", resp1)

	// Save the first image if generated
	if len(resp1.Images) > 0 {
		if err := saveImage(resp1.Images[0], "turn1_water_cycle.png"); err != nil {
			fmt.Printf("Warning: could not save image: %v\n", err)
		}
	}

	// --- Turn 2: Modify the image ---
	fmt.Println()
	fmt.Println("Turn 2: Modifying the image...")
	fmt.Println("Prompt: Add labels to each part of the water cycle (evaporation, condensation, precipitation)")
	fmt.Println()

	resp2, err := session.SendMessage(ctx, "Add labels to each part of the water cycle (evaporation, condensation, precipitation)", nil)
	if err != nil {
		return fmt.Errorf("turn 2 failed: %w", err)
	}

	printResponse("Turn 2 Response", resp2)

	// Save the second image if generated
	if len(resp2.Images) > 0 {
		if err := saveImage(resp2.Images[0], "turn2_water_cycle_labeled.png"); err != nil {
			fmt.Printf("Warning: could not save image: %v\n", err)
		}
	}

	// --- Turn 3: Another modification ---
	fmt.Println()
	fmt.Println("Turn 3: Translating to Spanish...")
	fmt.Println("Prompt: Change all the labels to Spanish")
	fmt.Println()

	resp3, err := session.SendMessage(ctx, "Change all the labels to Spanish", nil)
	if err != nil {
		return fmt.Errorf("turn 3 failed: %w", err)
	}

	printResponse("Turn 3 Response", resp3)

	// Save the third image if generated
	if len(resp3.Images) > 0 {
		if err := saveImage(resp3.Images[0], "turn3_water_cycle_spanish.png"); err != nil {
			fmt.Printf("Warning: could not save image: %v\n", err)
		}
	}

	// Get conversation history
	fmt.Println()
	fmt.Println("--- Conversation History ---")
	history := session.GetHistory()
	fmt.Printf("Total turns in history: %d\n", len(history))
	for i, turn := range history {
		imgCount := len(turn.Images)
		msgPreview := turn.Message
		if len(msgPreview) > 50 {
			msgPreview = msgPreview[:50] + "..."
		}
		fmt.Printf("  Turn %d [%s]: %s (images: %d)\n", i+1, turn.Role, msgPreview, imgCount)
	}

	fmt.Println()
	fmt.Println("Multi-turn image editing completed successfully!")
	return nil
}

func printResponse(label string, resp *interfaces.ImageEditResponse) {
	fmt.Printf("%s:\n", label)
	if resp.Text != "" {
		fmt.Printf("  Text: %s\n", resp.Text)
	}
	fmt.Printf("  Images generated: %d\n", len(resp.Images))
	if resp.Usage != nil {
		fmt.Printf("  Tokens: %d input, %d output\n", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}
	for i, img := range resp.Images {
		fmt.Printf("  Image %d: %s, %d bytes\n", i+1, img.MimeType, len(img.Data))
	}
}

func saveImage(img interfaces.GeneratedImage, filename string) error {
	// Create output directory
	outputDir := "output"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	path := filepath.Join(outputDir, filename)

	// Get image data
	data := img.Data
	if len(data) == 0 && img.Base64 != "" {
		var err error
		data, err = base64.StdEncoding.DecodeString(img.Base64)
		if err != nil {
			return fmt.Errorf("failed to decode base64 image: %w", err)
		}
	}

	if len(data) == 0 {
		return fmt.Errorf("no image data available")
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write image file: %w", err)
	}

	fmt.Printf("  Saved image to: %s\n", path)
	return nil
}
