// Package main demonstrates image generation with the embedded web UI.
// This example creates an agent with image generation capabilities and serves
// it through the microservice wrapper with the embedded Next.js UI.
//
// Generated images can be stored in:
// 1. Google Cloud Storage (GCS) - for production use with public URLs
// 2. Local filesystem with HTTP serving - for development (default)
//
// The UI supports displaying generated images with:
// - Inline image preview
// - Click-to-enlarge lightbox
// - Download functionality
//
// Environment variables:
//   - GEMINI_API_KEY or Vertex AI credentials for image generation
//
// For GCS storage (optional):
//   - GCS_BUCKET: The GCS bucket name for storing images
//   - GCS_PROJECT: The GCP project ID (required for bucket creation)
//   - GOOGLE_APPLICATION_CREDENTIALS: Path to service account JSON (or use ADC)
//
// For local storage (default):
//   - Images are stored in ./generated-images and served at /images/*
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
	imgstorage "github.com/Ingenimax/agent-sdk-go/pkg/storage"
	_ "github.com/Ingenimax/agent-sdk-go/pkg/storage/gcs"   // Register GCS storage
	_ "github.com/Ingenimax/agent-sdk-go/pkg/storage/local" // Register local storage
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/imagegen"
	"google.golang.org/genai"
)

const (
	defaultPort = 8090
)

var localStoragePath string

func init() {
	// Get the directory where the executable is located
	// This ensures images are stored relative to the example, not the working directory
	execPath, err := os.Executable()
	if err != nil {
		localStoragePath = "./generated-images"
		return
	}
	localStoragePath = filepath.Join(filepath.Dir(execPath), "generated-images")

	// For `go run`, use current working directory
	if strings.Contains(execPath, "go-build") {
		cwd, err := os.Getwd()
		if err != nil {
			localStoragePath = "./generated-images"
			return
		}
		localStoragePath = filepath.Join(cwd, "generated-images")
	}
}

func main() {
	ctx := context.Background()

	fmt.Println("=== Image Generation UI Example ===")
	fmt.Println()

	// Load .env file if present
	if err := agent.LoadEnvFile(".env"); err != nil {
		log.Printf("Warning: could not load .env file: %v", err)
	}

	// Determine port from environment or use default
	port := defaultPort
	if portStr := agent.GetEnvValue("PORT"); portStr != "" {
		if p, err := fmt.Sscanf(portStr, "%d", &port); err != nil || p != 1 {
			port = defaultPort
		}
	}

	// Create storage (GCS or local with HTTP serving)
	imgStorage, storageType, err := createImageStorage(ctx, port)
	if err != nil {
		log.Fatalf("Failed to create image storage: %v", err)
	}
	fmt.Printf("Using %s storage for generated images\n", storageType)

	// Create the agent with image generation capability
	myAgent, err := createImageGenAgent(ctx, imgStorage)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Create UI configuration
	uiConfig := &microservice.UIConfig{
		Enabled:     true,
		DefaultPath: "/",
		DevMode:     false,
		Theme:       "dark",
		Features: microservice.UIFeatures{
			Chat:      true,
			Memory:    true,
			AgentInfo: true,
			Settings:  true,
		},
	}

	// Create HTTP server with UI
	server := microservice.NewHTTPServerWithUI(myAgent, port, uiConfig)

	// If using local storage, start a separate goroutine to serve images
	if storageType == "local" {
		go func() {
			// Serve images on a different port to avoid conflicts with the main server
			imagePort := port + 1
			fmt.Printf("Serving generated images at http://localhost:%d/\n", imagePort)

			// Ensure directory exists
			// #nosec G301 -- Example code uses 0755 for local development directory
			if err := os.MkdirAll(localStoragePath, 0755); err != nil {
				log.Printf("Warning: could not create image directory: %v", err)
				return
			}

			// Create file server with CORS
			fs := http.FileServer(http.Dir(localStoragePath))
			http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				if r.Method == "OPTIONS" {
					w.WriteHeader(http.StatusOK)
					return
				}
				fs.ServeHTTP(w, r)
			})

			// #nosec G114 -- Example code for local development, timeouts not critical
			if err := http.ListenAndServe(fmt.Sprintf(":%d", imagePort), nil); err != nil {
				log.Printf("Image server error: %v", err)
			}
		}()
	}

	// Start the server
	fmt.Println()
	fmt.Println("Starting Image Generation Agent UI")
	fmt.Printf("Open your browser: http://localhost:%d\n", port)
	fmt.Println()
	fmt.Println("Try asking:")
	fmt.Println("  - 'Generate an image of a sunset over mountains'")
	fmt.Println("  - 'Create a minimalist logo for a tech company'")
	fmt.Println("  - 'Draw a cute cartoon cat'")
	fmt.Println()

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// createImageStorage creates an image storage backend.
// Returns the storage, storage type name, and any error.
func createImageStorage(ctx context.Context, port int) (imgstorage.ImageStorage, string, error) {
	// Try GCS first if configured
	bucket := agent.GetEnvValue("GCS_BUCKET")
	if bucket != "" {
		storage, err := createGCSStorage(ctx)
		if err != nil {
			log.Printf("Warning: GCS storage failed (%v), falling back to local storage", err)
		} else {
			return storage, "GCS", nil
		}
	}

	// Fall back to local storage with HTTP URLs
	return createLocalStorage(port)
}

// createLocalStorage creates a local filesystem storage with HTTP URLs.
func createLocalStorage(port int) (imgstorage.ImageStorage, string, error) {
	// Images will be served from a separate HTTP server
	imagePort := port + 1
	baseURL := fmt.Sprintf("http://localhost:%d", imagePort)

	// Ensure directory exists
	// #nosec G301 -- Example code uses 0755 for local development directory
	if err := os.MkdirAll(localStoragePath, 0755); err != nil {
		return nil, "", fmt.Errorf("failed to create image directory: %w", err)
	}

	cfg := imgstorage.LocalConfig{
		Path:    localStoragePath,
		BaseURL: baseURL,
	}

	fmt.Printf("Creating local storage: path=%s, baseURL=%s\n", cfg.Path, cfg.BaseURL)

	if imgstorage.NewLocalStorage == nil {
		return nil, "", fmt.Errorf("local storage factory not registered - ensure pkg/storage/local is imported")
	}

	storage, err := imgstorage.NewLocalStorage(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create local storage: %w", err)
	}

	fmt.Printf("Local storage created successfully: %s\n", storage.Name())
	return storage, "local", nil
}

// createImageGenAgent creates an agent with image generation capability.
func createImageGenAgent(ctx context.Context, imgStorage imgstorage.ImageStorage) (*agent.Agent, error) {
	// Get client options for image generation model
	imageOpts, err := getGeminiOptions(gemini.ModelGemini25FlashImage)
	if err != nil {
		return nil, fmt.Errorf("image model configuration error: %w", err)
	}

	// Create image generation client
	imageClient, err := gemini.NewClient(ctx, imageOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create image client: %w", err)
	}

	// Create image generation tool
	imgTool := imagegen.New(imageClient, imgStorage,
		imagegen.WithMaxPromptLength(2000),
		imagegen.WithDefaultAspectRatio("1:1"),
		imagegen.WithDefaultFormat("png"),
	)

	// Get client options for text model (for conversation)
	textOpts, err := getGeminiOptions(gemini.ModelGemini25Flash)
	if err != nil {
		return nil, fmt.Errorf("text model configuration error: %w", err)
	}

	// Create text LLM for the agent
	textClient, err := gemini.NewClient(ctx, textOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create text client: %w", err)
	}

	// Create agent with image generation capability
	ag, err := agent.NewAgent(
		agent.WithLLM(textClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(imgTool),
		agent.WithName("ImageGenAgent"),
		agent.WithDescription("An AI assistant that can generate images based on text descriptions"),
		agent.WithSystemPrompt(`You are a helpful AI assistant with image generation capabilities.

When a user asks you to create, generate, draw, or make an image, use the generate_image tool.

Guidelines for using the image generation tool:
1. Create detailed, descriptive prompts that capture what the user wants
2. Include relevant details like style, colors, lighting, and mood
3. For logos or icons, mention "minimalist", "simple", or "clean" as appropriate
4. For artistic images, suggest styles like "digital art", "watercolor", "photorealistic", etc.

IMPORTANT: After generating an image, you MUST include the image in your response using the exact markdown syntax from the tool result. Look for the line that starts with "![Generated image](" and include it exactly as-is in your response so the user can see the image. Then describe what was created and ask if the user would like any modifications.

If the user asks about something unrelated to images, respond helpfully as a general assistant.`),
		agent.WithRequirePlanApproval(false),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	return ag, nil
}

// createGCSStorage creates a GCS storage backend from environment variables.
// If the bucket doesn't exist, it will be created.
// Supports both ADC and VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT.
func createGCSStorage(ctx context.Context) (imgstorage.ImageStorage, error) {
	bucket := agent.GetEnvValue("GCS_BUCKET")
	if bucket == "" {
		return nil, fmt.Errorf("GCS_BUCKET environment variable not set")
	}

	projectID := agent.GetEnvValue("GCS_PROJECT")
	if projectID == "" {
		// Try to get from Vertex AI project
		projectID = agent.GetEnvValue("VERTEX_AI_PROJECT")
	}

	// Get credentials JSON if available (supports base64 encoded or raw JSON)
	credentialsJSON := ""
	creds := agent.GetEnvValue("VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT")
	fmt.Printf("[GCS] VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT length: %d\n", len(creds))
	if creds != "" {
		// Try to decode base64
		if decoded, err := base64.StdEncoding.DecodeString(creds); err == nil {
			fmt.Printf("[GCS] Base64 decoded length: %d\n", len(decoded))
			if len(decoded) > 0 && decoded[0] == '{' {
				credentialsJSON = string(decoded)
				fmt.Printf("[GCS] Using base64 decoded credentials (length=%d)\n", len(credentialsJSON))
			}
		} else {
			fmt.Printf("[GCS] Base64 decode failed: %v\n", err)
		}
		// If not base64, use as-is
		if credentialsJSON == "" && len(creds) > 0 && creds[0] == '{' {
			credentialsJSON = creds
			fmt.Printf("[GCS] Using raw JSON credentials (length=%d)\n", len(credentialsJSON))
		}
	}
	fmt.Printf("[GCS] Final credentialsJSON length: %d\n", len(credentialsJSON))

	// Ensure bucket exists (pass credentials for bucket check/creation)
	if err := ensureBucketExistsWithCreds(ctx, bucket, projectID, credentialsJSON); err != nil {
		return nil, fmt.Errorf("failed to ensure bucket exists: %w", err)
	}

	// Optional: prefix for organizing images in the bucket
	prefix := agent.GetEnvValue("GCS_PREFIX")
	if prefix == "" {
		prefix = "generated-images"
	}

	cfg := imgstorage.GCSConfig{
		Bucket:              bucket,
		Prefix:              prefix,
		CredentialsJSON:     credentialsJSON, // Pass credentials to storage
		UseSignedURLs:       true,            // Use signed URLs when using service account
		SignedURLExpiration: 24 * time.Hour,
	}

	fmt.Printf("[GCS] Creating GCS storage: bucket=%s, prefix=%s, credsLen=%d, useSignedURLs=%v\n",
		cfg.Bucket, cfg.Prefix, len(cfg.CredentialsJSON), cfg.UseSignedURLs)
	return imgstorage.NewGCSStorage(cfg)
}

// ensureBucketExists checks if the bucket exists and creates it if it doesn't.
// Uses Application Default Credentials.
func ensureBucketExists(ctx context.Context, bucketName, projectID string) error {
	return ensureBucketExistsWithCreds(ctx, bucketName, projectID, "")
}

// ensureBucketExistsWithCreds checks if the bucket exists and creates it if it doesn't.
// Supports explicit credentials JSON or falls back to ADC.
func ensureBucketExistsWithCreds(ctx context.Context, bucketName, projectID, credentialsJSON string) error {
	var client *storage.Client
	var err error

	fmt.Printf("[GCS] ensureBucketExistsWithCreds: bucket=%s, projectID=%s, credsLen=%d\n", bucketName, projectID, len(credentialsJSON))

	if credentialsJSON != "" {
		// Use explicit credentials
		fmt.Println("[GCS] Using explicit credentials JSON for bucket check")
		client, err = storage.NewClient(ctx, option.WithCredentialsJSON([]byte(credentialsJSON)))
	} else {
		// Use Application Default Credentials
		fmt.Println("[GCS] Using Application Default Credentials for bucket check")
		client, err = storage.NewClient(ctx)
	}
	if err != nil {
		return fmt.Errorf("failed to create GCS client: %w", err)
	}
	defer func() {
		if cerr := client.Close(); cerr != nil {
			log.Printf("Warning: failed to close GCS client: %v", cerr)
		}
	}()

	bucket := client.Bucket(bucketName)

	// Check if bucket exists by trying to get its attributes
	_, err = bucket.Attrs(ctx)
	if err == nil {
		// Bucket exists
		fmt.Printf("Using existing GCS bucket: %s\n", bucketName)
		return nil
	}

	// If error is not "not found", return it
	if err != storage.ErrBucketNotExist {
		return fmt.Errorf("failed to check bucket: %w", err)
	}

	// Bucket doesn't exist, create it
	if projectID == "" {
		return fmt.Errorf("bucket %s doesn't exist and GCS_PROJECT not set for creation", bucketName)
	}

	fmt.Printf("Creating GCS bucket: %s in project %s\n", bucketName, projectID)

	// Create bucket with public access for images
	bucketAttrs := &storage.BucketAttrs{
		Location:     "US", // Multi-region for better availability
		StorageClass: "STANDARD",
		UniformBucketLevelAccess: storage.UniformBucketLevelAccess{
			Enabled: true,
		},
	}

	if err := bucket.Create(ctx, projectID, bucketAttrs); err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	// Make bucket publicly readable for images
	policy, err := bucket.IAM().Policy(ctx)
	if err != nil {
		return fmt.Errorf("failed to get bucket policy: %w", err)
	}

	policy.Add("allUsers", "roles/storage.objectViewer")
	if err := bucket.IAM().SetPolicy(ctx, policy); err != nil {
		log.Printf("Warning: could not set public access policy: %v", err)
		log.Printf("Images may require signed URLs. Set UseSignedURLs=true in GCSConfig.")
	}

	fmt.Printf("Created GCS bucket: %s\n", bucketName)
	return nil
}

// getGeminiOptions returns the appropriate Gemini client options based on environment.
// Supports both API key and Vertex AI authentication.
func getGeminiOptions(model string) ([]gemini.Option, error) {
	opts := []gemini.Option{gemini.WithModel(model)}

	// Check for Vertex AI credentials first
	vertexCreds := agent.GetEnvValue("VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT")
	vertexProject := agent.GetEnvValue("VERTEX_AI_PROJECT")
	vertexRegion := agent.GetEnvValue("VERTEX_AI_REGION")

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
	apiKey := agent.GetEnvValue("GEMINI_API_KEY")
	if apiKey != "" {
		opts = append(opts, gemini.WithAPIKey(apiKey))
		return opts, nil
	}

	return nil, fmt.Errorf("no credentials found: set GEMINI_API_KEY or Vertex AI environment variables")
}
