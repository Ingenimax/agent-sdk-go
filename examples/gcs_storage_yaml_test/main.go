// Package main demonstrates YAML-based image generation with GCS storage and the embedded web UI.
// This is the YAML equivalent of the gcs_storage_test and image_generation_ui examples combined.
//
// Run with: go run main.go
//
// Required environment variables:
//   - VERTEX_AI_PROJECT: GCP project ID
//   - VERTEX_AI_REGION: GCP region (default: us-central1)
//   - VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT: Service account JSON (base64 or raw)
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
	_ "github.com/Ingenimax/agent-sdk-go/pkg/storage/gcs" // Register GCS storage backend
)

const defaultPort = 8090

func main() {
	fmt.Println("=== GCS Storage YAML Test with Web UI ===")
	fmt.Println()

	// Load .env file if present
	if err := agent.LoadEnvFile(".env"); err != nil {
		log.Printf("Warning: could not load .env file: %v", err)
	}

	// Validate environment
	if err := validateEnvironment(); err != nil {
		log.Fatalf("Environment validation failed: %v", err)
	}

	// Find and load YAML config
	configPath := findConfigFile()
	fmt.Printf("Loading agent from: %s\n", configPath)

	configs, err := agent.LoadAgentConfigsFromFile(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	fmt.Printf("Loaded %d agent configuration(s)\n", len(configs))

	// Debug: Print config details
	for name, cfg := range configs {
		fmt.Printf("\n=== DEBUG: Agent Config '%s' ===\n", name)
		fmt.Printf("  Role length: %d\n", len(cfg.Role))
		fmt.Printf("  Goal length: %d\n", len(cfg.Goal))
		fmt.Printf("  Backstory length: %d\n", len(cfg.Backstory))
		if cfg.LLMProvider != nil {
			fmt.Printf("  LLM Provider: %s\n", cfg.LLMProvider.Provider)
			fmt.Printf("  LLM Model: %s\n", cfg.LLMProvider.Model)
			if cfg.LLMProvider.Config != nil {
				for k, v := range cfg.LLMProvider.Config {
					if str, ok := v.(string); ok {
						fmt.Printf("  LLM Config[%s] length: %d\n", k, len(str))
					} else {
						fmt.Printf("  LLM Config[%s]: %v\n", k, v)
					}
				}
			}
		}
		if cfg.ImageGeneration != nil {
			fmt.Printf("  ImageGen Provider: %s\n", cfg.ImageGeneration.Provider)
			fmt.Printf("  ImageGen Model: %s\n", cfg.ImageGeneration.Model)
			if cfg.ImageGeneration.Config != nil {
				for k, v := range cfg.ImageGeneration.Config {
					if str, ok := v.(string); ok {
						fmt.Printf("  ImageGen Config[%s] length: %d\n", k, len(str))
					} else {
						fmt.Printf("  ImageGen Config[%s]: %v\n", k, v)
					}
				}
			}
			if cfg.ImageGeneration.Storage != nil {
				fmt.Printf("  Storage Type: %s\n", cfg.ImageGeneration.Storage.Type)
				if cfg.ImageGeneration.Storage.GCS != nil {
					fmt.Printf("  GCS Bucket: %s\n", cfg.ImageGeneration.Storage.GCS.Bucket)
					fmt.Printf("  GCS CredentialsJSON length: %d\n", len(cfg.ImageGeneration.Storage.GCS.CredentialsJSON))
				}
			}
		}
		fmt.Printf("=== END DEBUG ===\n\n")
	}

	// Create agent from YAML config
	ag, err := agent.NewAgentFromConfig("image_generator", configs, nil)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Debug: Print agent details after creation
	fmt.Printf("\n=== DEBUG: Agent After Creation ===\n")
	fmt.Printf("  Name: %s\n", ag.GetName())
	fmt.Printf("  System Prompt length: %d\n", len(ag.GetSystemPrompt()))
	fmt.Printf("  Tools count: %d\n", len(ag.GetTools()))
	for _, tool := range ag.GetTools() {
		fmt.Printf("    - Tool: %s\n", tool.Name())
	}
	fmt.Printf("=== END DEBUG ===\n\n")

	fmt.Println("Agent created successfully")
	fmt.Println()

	// Determine port
	port := defaultPort
	if portStr := os.Getenv("PORT"); portStr != "" {
		if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
			port = defaultPort
		}
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

	// Create HTTP server with embedded UI
	server := microservice.NewHTTPServerWithUI(ag, port, uiConfig)

	// Start the server
	fmt.Println("Starting Image Generation Agent UI (YAML-configured)")
	fmt.Printf("Open your browser: http://localhost:%d\n", port)
	fmt.Println()
	fmt.Println("Try asking:")
	fmt.Println("  - 'Generate an image of a sunset over mountains'")
	fmt.Println("  - 'Create a minimalist logo for a tech company'")
	fmt.Println("  - 'Draw a simple blue circle on white background'")
	fmt.Println()

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func validateEnvironment() error {
	required := []string{
		"VERTEX_AI_PROJECT",
		"VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT",
	}

	var missing []string
	for _, env := range required {
		if os.Getenv(env) == "" {
			missing = append(missing, env)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missing)
	}

	fmt.Printf("Environment validated:\n")
	fmt.Printf("  Project: %s\n", os.Getenv("VERTEX_AI_PROJECT"))
	region := os.Getenv("VERTEX_AI_REGION")
	if region == "" {
		region = "us-central1"
	}
	fmt.Printf("  Region: %s\n", region)
	fmt.Println()

	return nil
}

func findConfigFile() string {
	paths := []string{
		"agents.yaml",
		"examples/gcs_storage_yaml_test/agents.yaml",
		filepath.Join(os.Getenv("PWD"), "agents.yaml"),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return "agents.yaml"
}
