package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
	// Parse command line flags
	systemPrompt := flag.String("system-prompt", "", "System prompt to use for agent auto-configuration")
	agentName := flag.String("agent-name", "Auto-Configured Agent", "Name for the auto-configured agent")
	openaiApiKey := flag.String("openai-key", "", "OpenAI API key (or set OPENAI_API_KEY env var)")
	saveConfigsToFiles := flag.Bool("save-configs", false, "Save generated configs to YAML files")
	flag.Parse()

	// Validate required flags
	if *systemPrompt == "" {
		fmt.Println("Usage: auto_config --system-prompt=\"Your system prompt here\" [--agent-name=\"Agent Name\"] [--openai-key=<key>] [--save-configs]")
		os.Exit(1)
	}

	// Get OpenAI API key from flag or environment variable
	apiKey := *openaiApiKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			log.Fatal("OpenAI API key not provided. Use --openai-key flag or set OPENAI_API_KEY environment variable.")
		}
	}

	// Create the LLM client
	llm := openai.NewClient(apiKey)

	// Create agent with auto-config
	createdAgent, err := agent.NewAgentWithAutoConfig(
		context.Background(),
		agent.WithLLM(llm),
		agent.WithSystemPrompt(*systemPrompt),
		agent.WithName(*agentName),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Check if auto-configuration succeeded
	agentConfig := createdAgent.GetGeneratedAgentConfig()
	taskConfigs := createdAgent.GetGeneratedTaskConfigs()

	if agentConfig == nil {
		log.Fatal("Failed to auto-generate agent configuration")
	}

	// Print generated configurations
	fmt.Println("\n=== Generated Agent Configuration ===")
	fmt.Printf("Role: %s\n", agentConfig.Role)
	fmt.Printf("Goal: %s\n", agentConfig.Goal)
	fmt.Printf("Backstory: %s\n", agentConfig.Backstory)

	fmt.Println("\n=== Generated Task Configurations ===")
	if len(taskConfigs) == 0 {
		fmt.Println("No tasks generated")
	} else {
		for name, taskConfig := range taskConfigs {
			fmt.Printf("\nTask: %s\n", name)
			fmt.Printf("Description: %s\n", taskConfig.Description)
			fmt.Printf("Expected Output: %s\n", taskConfig.ExpectedOutput)
			if taskConfig.OutputFile != "" {
				fmt.Printf("Output File: %s\n", taskConfig.OutputFile)
			}
		}
	}

	// Save configurations to YAML files if requested
	if *saveConfigsToFiles {
		// Save agent config
		agentConfigMap := make(map[string]agent.AgentConfig)
		agentConfigMap[*agentName] = *agentConfig

		agentYaml, err := os.Create("auto_agent_config.yaml")
		if err != nil {
			log.Fatalf("Failed to create agent config file: %v", err)
		}
		defer agentYaml.Close()

		if err := agent.SaveAgentConfigsToFile(agentConfigMap, agentYaml); err != nil {
			log.Fatalf("Failed to save agent configurations: %v", err)
		}

		// Save task configs
		taskYaml, err := os.Create("auto_task_config.yaml")
		if err != nil {
			log.Fatalf("Failed to create task config file: %v", err)
		}
		defer taskYaml.Close()

		if err := agent.SaveTaskConfigsToFile(taskConfigs, taskYaml); err != nil {
			log.Fatalf("Failed to save task configurations: %v", err)
		}

		fmt.Println("\nConfigurations saved to auto_agent_config.yaml and auto_task_config.yaml")
	}

	fmt.Println("\nUse these configurations in your application or save them to YAML files.")
}
