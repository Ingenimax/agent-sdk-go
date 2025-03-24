package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
	// Get OpenAI API key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create the LLM client
	llm := openai.NewClient(apiKey)

	// Create example directory
	exampleDir := "config_example"
	err := os.MkdirAll(exampleDir, 0755)
	if err != nil {
		log.Fatalf("Error creating directory: %v", err)
	}

	fmt.Println("=== Combined Configuration Example ===")
	fmt.Println("This example demonstrates both YAML configuration and auto-configuration features.")

	// Step 1: Auto-generate agent and task configurations
	fmt.Println("\n--- Step 1: Auto-generating Agent Configuration ---")
	autoConfiguredAgent, err := createAutoConfiguredAgent(llm, exampleDir)
	if err != nil {
		log.Fatalf("Error creating auto-configured agent: %v", err)
	}

	// Step 2: Load YAML configurations
	fmt.Println("\n--- Step 2: Loading YAML Configurations ---")
	yamlAgent, err := loadYamlAgent(llm, exampleDir)
	if err != nil {
		log.Fatalf("Error loading YAML agent: %v", err)
	}

	// Step 3: Execute tasks with each agent
	fmt.Println("\n--- Step 3: Executing Tasks ---")

	// Execute a task with auto-configured agent
	fmt.Println("\nExecuting task with auto-configured agent:")
	executeExampleTask(autoConfiguredAgent)

	// Execute a task with YAML-configured agent
	fmt.Println("\nExecuting task with YAML-configured agent:")
	executeExampleTask(yamlAgent)
}

func createAutoConfiguredAgent(llm *openai.OpenAIClient, configDir string) (*agent.Agent, error) {
	// Define a system prompt for the agent
	systemPrompt := "You are a helpful travel advisor. You assist users in planning trips, " +
		"providing recommendations for destinations, accommodations, and activities based on their preferences. " +
		"You are knowledgeable about global travel destinations and can provide personalized itineraries."

	fmt.Println("System prompt:", systemPrompt)

	// Create agent with auto-configuration
	fmt.Println("Creating agent with auto-configuration...")
	createdAgent, err := agent.NewAgentWithAutoConfig(
		context.Background(),
		agent.WithLLM(llm),
		agent.WithSystemPrompt(systemPrompt),
		agent.WithName("Travel Advisor"),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating agent: %w", err)
	}

	// Retrieve generated configurations
	agentConfig := createdAgent.GetGeneratedAgentConfig()
	taskConfigs := createdAgent.GetGeneratedTaskConfigs()

	if agentConfig == nil {
		return nil, fmt.Errorf("failed to auto-generate agent configuration")
	}

	// Print generated agent configuration
	fmt.Println("\nGenerated agent configuration:")
	fmt.Printf("Role: %s\n", agentConfig.Role)
	fmt.Printf("Goal: %s\n", agentConfig.Goal)

	// Print number of generated tasks
	fmt.Printf("\nGenerated %d task configurations\n", len(taskConfigs))
	for name := range taskConfigs {
		fmt.Printf("- %s\n", name)
	}

	// Save generated configurations to YAML files
	fmt.Println("\nSaving generated configurations to YAML files...")

	// Save agent configs
	agentConfigMap := make(map[string]agent.AgentConfig)
	agentConfigMap["Travel Advisor"] = *agentConfig

	agentYamlPath := filepath.Join(configDir, "agent_config.yaml")
	agentYaml, err := os.Create(agentYamlPath)
	if err != nil {
		return nil, fmt.Errorf("error creating agent config file: %w", err)
	}
	defer agentYaml.Close()

	if err := agent.SaveAgentConfigsToFile(agentConfigMap, agentYaml); err != nil {
		return nil, fmt.Errorf("error saving agent configurations: %w", err)
	}

	// Save task configs
	taskYamlPath := filepath.Join(configDir, "task_config.yaml")
	taskYaml, err := os.Create(taskYamlPath)
	if err != nil {
		return nil, fmt.Errorf("error creating task config file: %w", err)
	}
	defer taskYaml.Close()

	if err := agent.SaveTaskConfigsToFile(taskConfigs, taskYaml); err != nil {
		return nil, fmt.Errorf("error saving task configurations: %w", err)
	}

	fmt.Printf("Configurations saved to %s/agent_config.yaml and %s/task_config.yaml\n", configDir, configDir)

	return createdAgent, nil
}

func loadYamlAgent(llm *openai.OpenAIClient, configDir string) (*agent.Agent, error) {
	// Load agent configs
	fmt.Println("Loading agent configurations from YAML files...")
	agentConfigPath := filepath.Join(configDir, "agent_config.yaml")
	agentConfigs, err := agent.LoadAgentConfigsFromFile(agentConfigPath)
	if err != nil {
		return nil, fmt.Errorf("error loading agent configs: %w", err)
	}

	// Load task configs
	taskConfigPath := filepath.Join(configDir, "task_config.yaml")
	taskConfigs, err := agent.LoadTaskConfigsFromFile(taskConfigPath)
	if err != nil {
		return nil, fmt.Errorf("error loading task configs: %w", err)
	}

	// Print available agents
	fmt.Println("\nLoaded agent configurations:")
	for name, config := range agentConfigs {
		fmt.Printf("- %s: %s\n", name, config.Role)
	}

	// Select the travel advisor agent
	agentName := "Travel Advisor"
	_, exists := agentConfigs[agentName]
	if !exists {
		return nil, fmt.Errorf("agent '%s' not found in configuration", agentName)
	}

	// Count tasks for this agent
	var agentTaskCount int
	for _, taskConfig := range taskConfigs {
		if taskConfig.Agent == agentName {
			agentTaskCount++
		}
	}
	fmt.Printf("\nLoaded %d tasks for agent '%s'\n", agentTaskCount, agentName)

	// Create agent from YAML configs
	variables := make(map[string]string)
	agentFromYaml, err := agent.NewAgentFromConfig(
		agentName,
		agentConfigs,
		variables,
		agent.WithLLM(llm),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating agent from YAML config: %w", err)
	}

	fmt.Printf("Successfully created agent '%s' from YAML configuration\n", agentName)
	return agentFromYaml, nil
}

func executeExampleTask(agent *agent.Agent) {
	// Define a simple task input
	taskInput := "I'm planning a trip to Japan in the spring. Can you suggest a 5-day itinerary?"

	// Execute the task directly using the agent
	fmt.Printf("Task input: %s\n", taskInput)
	fmt.Println("Executing task...")

	response, err := agent.Run(context.Background(), taskInput)
	if err != nil {
		fmt.Printf("Error executing task: %v\n", err)
		return
	}

	fmt.Println("\nTask result:")
	fmt.Println(response)
}
