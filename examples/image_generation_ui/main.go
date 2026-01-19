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
	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
	imgstorage "github.com/Ingenimax/agent-sdk-go/pkg/storage"
	_ "github.com/Ingenimax/agent-sdk-go/pkg/storage/gcs"   // Register GCS storage
	_ "github.com/Ingenimax/agent-sdk-go/pkg/storage/local" // Register local storage
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/imagegen"
	"google.golang.org/api/iterator"
	"google.golang.org/genai"
)

const (
	defaultPort = 8080
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

	// Determine port
	port := defaultPort

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
	bucket := os.Getenv("GCS_BUCKET")
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
func createGCSStorage(ctx context.Context) (imgstorage.ImageStorage, error) {
	bucket := os.Getenv("GCS_BUCKET")
	if bucket == "" {
		return nil, fmt.Errorf("GCS_BUCKET environment variable not set")
	}

	projectID := os.Getenv("GCS_PROJECT")
	if projectID == "" {
		// Try to get from Vertex AI project
		projectID = os.Getenv("VERTEX_AI_PROJECT")
	}

	// Ensure bucket exists
	if err := ensureBucketExists(ctx, bucket, projectID); err != nil {
		return nil, fmt.Errorf("failed to ensure bucket exists: %w", err)
	}

	// Optional: prefix for organizing images in the bucket
	prefix := os.Getenv("GCS_PREFIX")
	if prefix == "" {
		prefix = "generated-images"
	}

	cfg := imgstorage.GCSConfig{
		Bucket:              bucket,
		Prefix:              prefix,
		UseSignedURLs:       false, // Use public URLs (bucket configured for public access)
		SignedURLExpiration: 24 * time.Hour,
	}

	return imgstorage.NewGCSStorage(cfg)
}

// ensureBucketExists checks if the bucket exists and creates it if it doesn't.
func ensureBucketExists(ctx context.Context, bucketName, projectID string) error {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create GCS client: %w", err)
	}
	defer client.Close()

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

// listBuckets lists all buckets in the project (for debugging)
func listBuckets(ctx context.Context, projectID string) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Printf("Failed to create client: %v", err)
		return
	}
	defer client.Close()

	it := client.Buckets(ctx, projectID)
	fmt.Println("Available buckets:")
	for {
		battrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Failed to list buckets: %v", err)
			return
		}
		fmt.Printf("  - %s\n", battrs.Name)
	}
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
