// Package main provides a simple GCS storage test for image generation.
// Run with: go run gcs_test.go
//
// Required environment variables:
//   - VERTEX_AI_PROJECT: GCP project ID
//   - VERTEX_AI_REGION: GCP region (default: us-central1)
//   - VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT: Service account JSON (base64 or raw)
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage/gcs"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/imagegen"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== GCS Storage Test for Image Generation ===")
	fmt.Println()

	// Step 1: Get credentials
	fmt.Println("Step 1: Loading credentials...")
	vertexCreds := os.Getenv("VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT")
	vertexProject := os.Getenv("VERTEX_AI_PROJECT")
	vertexRegion := os.Getenv("VERTEX_AI_REGION")

	if vertexCreds == "" {
		fmt.Println("ERROR: VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT not set")
		os.Exit(1)
	}
	if vertexProject == "" {
		fmt.Println("ERROR: VERTEX_AI_PROJECT not set")
		os.Exit(1)
	}
	if vertexRegion == "" {
		vertexRegion = "us-central1"
	}

	fmt.Printf("  Project: %s\n", vertexProject)
	fmt.Printf("  Region: %s\n", vertexRegion)
	fmt.Printf("  Credentials length: %d\n", len(vertexCreds))

	// Decode base64 credentials if needed
	credentialsJSON := vertexCreds
	if decoded, err := base64.StdEncoding.DecodeString(vertexCreds); err == nil {
		if len(decoded) > 0 && decoded[0] == '{' {
			credentialsJSON = string(decoded)
			fmt.Println("  Credentials: decoded from base64")
		}
	}
	fmt.Printf("  Credentials JSON length: %d, starts with brace: %v\n", len(credentialsJSON), credentialsJSON[0] == '{')
	fmt.Println()

	// Step 2: Create GCS storage
	fmt.Println("Step 2: Creating GCS storage...")
	gcsStorage, err := gcs.New(storage.GCSConfig{
		Bucket:          "agentgogo-generated-images",
		Prefix:          "test-images/",
		CredentialsJSON: credentialsJSON,
		UseSignedURLs:   true,
	})
	if err != nil {
		fmt.Printf("ERROR: Failed to create GCS storage: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Storage created: %s\n", gcsStorage.Name())
	fmt.Println()

	// Step 3: Create Gemini client for image generation
	fmt.Println("Step 3: Creating Gemini image client...")
	imageClient, err := gemini.NewClient(ctx,
		gemini.WithModel(gemini.ModelGemini25FlashImage),
		gemini.WithBackend(genai.BackendVertexAI),
		gemini.WithCredentialsJSON([]byte(credentialsJSON)),
		gemini.WithProjectID(vertexProject),
		gemini.WithLocation(vertexRegion),
	)
	if err != nil {
		fmt.Printf("ERROR: Failed to create Gemini client: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Client created, supports image generation: %v\n", imageClient.SupportsImageGeneration())
	fmt.Println()

	// Step 4: Create image generation tool
	fmt.Println("Step 4: Creating image generation tool...")
	imgTool := imagegen.New(imageClient, gcsStorage)
	fmt.Printf("  Tool created: %s\n", imgTool.Name())
	fmt.Println()

	// Step 5: Generate an image
	fmt.Println("Step 5: Generating image...")
	result, err := imgTool.Execute(ctx, `{"prompt": "A simple blue circle on white background"}`)
	if err != nil {
		fmt.Printf("ERROR: Image generation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Result:")
	fmt.Println(result)
	fmt.Println()

	// Step 6: Test direct storage
	fmt.Println("Step 6: Testing direct GCS upload...")
	testImage := &interfaces.GeneratedImage{
		Data:     []byte("test image data"),
		MimeType: "image/png",
	}
	url, err := gcsStorage.Store(ctx, testImage, storage.StorageMetadata{
		Prompt: "test upload",
	})
	if err != nil {
		fmt.Printf("ERROR: Direct storage failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Uploaded to: %s\n", url)
	fmt.Println()

	fmt.Println("=== All tests passed! ===")
}
