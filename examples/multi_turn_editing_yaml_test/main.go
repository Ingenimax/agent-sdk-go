// Package main demonstrates YAML-based multi-turn image editing with the embedded web UI.
// This allows iterative image creation and refinement through conversation.
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

const defaultPort = 8091

func main() {
	fmt.Println("=== Multi-Turn Image Editing YAML Test with Web UI ===")
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
		fmt.Printf("  Role: %s\n", truncate(cfg.Role, 50))
		fmt.Printf("  Goal: %s\n", truncate(cfg.Goal, 50))
		if cfg.LLMProvider != nil {
			fmt.Printf("  LLM Provider: %s\n", cfg.LLMProvider.Provider)
			fmt.Printf("  LLM Model: %s\n", cfg.LLMProvider.Model)
		}
		if cfg.ImageGeneration != nil {
			fmt.Printf("  ImageGen Provider: %s\n", cfg.ImageGeneration.Provider)
			fmt.Printf("  ImageGen Model: %s\n", cfg.ImageGeneration.Model)
			if cfg.ImageGeneration.MultiTurnEditing != nil {
				enabled := cfg.ImageGeneration.MultiTurnEditing.Enabled == nil || *cfg.ImageGeneration.MultiTurnEditing.Enabled
				fmt.Printf("  Multi-Turn Editing Enabled: %v\n", enabled)
				fmt.Printf("  Multi-Turn Model: %s\n", cfg.ImageGeneration.MultiTurnEditing.Model)
				fmt.Printf("  Session Timeout: %s\n", cfg.ImageGeneration.MultiTurnEditing.SessionTimeout)
				if cfg.ImageGeneration.MultiTurnEditing.MaxSessionsPerOrg != nil {
					fmt.Printf("  Max Sessions Per Org: %d\n", *cfg.ImageGeneration.MultiTurnEditing.MaxSessionsPerOrg)
				}
			}
			if cfg.ImageGeneration.Storage != nil {
				fmt.Printf("  Storage Type: %s\n", cfg.ImageGeneration.Storage.Type)
				if cfg.ImageGeneration.Storage.GCS != nil {
					fmt.Printf("  GCS Bucket: %s\n", cfg.ImageGeneration.Storage.GCS.Bucket)
				}
			}
		}
		fmt.Printf("=== END DEBUG ===\n\n")
	}

	// Create agent from YAML config
	ag, err := agent.NewAgentFromConfig("image_editor", configs, nil)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Debug: Print agent details after creation
	fmt.Printf("\n=== DEBUG: Agent After Creation ===\n")
	fmt.Printf("  Name: %s\n", ag.GetName())
	fmt.Printf("  Tools count: %d\n", len(ag.GetTools()))
	for _, tool := range ag.GetTools() {
		fmt.Printf("    - Tool: %s (%s)\n", tool.Name(), tool.Description()[:min(50, len(tool.Description()))]+"...")
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
	fmt.Println("Starting Multi-Turn Image Editor Agent UI (YAML-configured)")
	fmt.Printf("Open your browser: http://localhost:%d\n", port)
	fmt.Println()
	fmt.Println("=== Multi-Turn Editing Workflow ===")
	fmt.Println()
	fmt.Println("Try this conversation flow:")
	fmt.Println()
	fmt.Println("1. Start: 'Create an infographic about the water cycle'")
	fmt.Println("   -> Agent starts a session and generates initial image")
	fmt.Println()
	fmt.Println("2. Edit: 'Add labels to each part (evaporation, condensation, precipitation)'")
	fmt.Println("   -> Agent continues the session and adds labels")
	fmt.Println()
	fmt.Println("3. Edit: 'Change the language to Spanish'")
	fmt.Println("   -> Agent continues and translates the labels")
	fmt.Println()
	fmt.Println("4. End: 'I'm done with the image'")
	fmt.Println("   -> Agent closes the session")
	fmt.Println()
	fmt.Println("The session maintains context between edits, so the model")
	fmt.Println("understands what image you're referring to!")
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
		"examples/multi_turn_editing_yaml_test/agents.yaml",
		filepath.Join(os.Getenv("PWD"), "agents.yaml"),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return "agents.yaml"
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
