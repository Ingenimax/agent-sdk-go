package agentconfig

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
)

// ExampleBasicUsage shows the simplest way to load an agent with automatic source detection
func ExampleBasicUsage() {
	ctx := context.Background()

	// Example 1: Load agent with automatic source detection (recommended)
	agentInstance, err := LoadAgentAuto(ctx, "research-assistant", "production")
	if err != nil {
		log.Fatalf("Failed to load agent: %v", err)
	}

	fmt.Printf("Agent loaded from source: %s\n", agentInstance.GetConfig().ConfigSource.Type)
}

// ExampleExplicitSources shows how to force specific configuration sources
func ExampleExplicitSources() {
	ctx := context.Background()

	// Example 2: Force remote configuration only
	remoteAgent, err := LoadAgentFromRemote(ctx, "research-assistant", "production")
	if err != nil {
		log.Printf("Remote loading failed: %v", err)
		// Handle fallback or error
		return
	}

	// Example 3: Force local configuration only
	localAgent, err := LoadAgentFromLocal(ctx, "research-assistant", "production")
	if err != nil {
		log.Printf("Local loading failed: %v", err)
		// Handle error
		return
	}

	fmt.Printf("Remote agent loaded from: %s\n", remoteAgent.GetConfig().ConfigSource.Source)
	fmt.Printf("Local agent loaded from: %s\n", localAgent.GetConfig().ConfigSource.Source)
}

// ExampleConfigurationPreview shows how to preview configurations without creating agents
func ExampleConfigurationPreview() {
	ctx := context.Background()

	// Example 4: Preview configuration without creating agent
	config, err := PreviewAgentConfig(ctx, "research-assistant", "production")
	if err != nil {
		log.Fatalf("Failed to preview config: %v", err)
	}

	fmt.Printf("Config loaded from: %s\n", config.ConfigSource.Type)
	fmt.Printf("Resolved variables: %v\n", config.ConfigSource.Variables)
	fmt.Printf("Agent role: %s\n", config.Role)
	fmt.Printf("Agent goal: %s\n", config.Goal)
}

// ExampleAdvancedOptions shows advanced configuration options
func ExampleAdvancedOptions() {
	ctx := context.Background()

	// Load with custom options
	loadOptions := []LoadOption{
		WithLocalFallback("./configs/research.yaml"), // Specific fallback file
		WithCache(10 * time.Minute),                  // Longer cache
		WithEnvOverrides(),                           // Enable env var overrides
		WithVerbose(),                               // Enable logging
	}

	// Agent options for customization
	agentOptions := []agent.Option{
		agent.WithMaxIterations(5),
		agent.WithRequirePlanApproval(false),
	}

	agentInstance, err := LoadAgentWithOptions(ctx, "research-assistant", "staging", loadOptions, agentOptions...)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("Configuration loaded successfully from %s\n", agentInstance.GetConfig().ConfigSource.Source)
}

// ExampleWithVariables shows how to pass custom variables
func ExampleWithVariables() {
	ctx := context.Background()

	// Custom variables for template substitution
	variables := map[string]string{
		"topic":        "artificial intelligence",
		"search_depth": "comprehensive",
	}

	agentInstance, err := LoadAgentWithVariables(ctx, "research-assistant", "development", variables)
	if err != nil {
		log.Fatalf("Failed to load agent with variables: %v", err)
	}

	fmt.Printf("Agent loaded with custom variables: %v\n", variables)
	fmt.Printf("Agent backstory: %s\n", agentInstance.GetConfig().Backstory)
}

// ExampleErrorHandling shows proper error handling patterns
func ExampleErrorHandling() {
	ctx := context.Background()

	// Try loading with error handling
	agentInstance, err := LoadAgentAuto(ctx, "nonexistent-agent", "production")
	if err != nil {
		// Check if it's a specific error type
		if err.Error() == "failed to load agent config from any source" {
			log.Printf("Agent not found in any configuration source")
			// Try creating a default agent or prompt user
		} else {
			log.Printf("Configuration error: %v", err)
		}
		return
	}

	fmt.Printf("Agent loaded successfully: %s\n", agentInstance.GetConfig().ConfigSource.Type)
}

// ExampleMigrationFromOldAPI shows how to migrate from old file-based loading
func ExampleMigrationFromOldAPI() {
	ctx := context.Background()

	// OLD WAY (still works):
	// configs, err := agent.LoadAgentConfigsFromFile("./agents.yaml")
	// agent, err := agent.NewAgentFromConfig("research-assistant", configs, nil)

	// NEW WAY (recommended):
	agentInstance, err := LoadAgentAuto(ctx, "research-assistant", "production")
	if err != nil {
		log.Fatalf("Failed to load agent: %v", err)
	}

	fmt.Printf("Migrated to new configuration system: %s\n", agentInstance.GetConfig().ConfigSource.Type)
}