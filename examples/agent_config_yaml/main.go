package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
	// Parse command line flags
	agentConfigPath := flag.String("agent-config", "", "Path to agent configuration YAML file")
	taskConfigPath := flag.String("task-config", "", "Path to task configuration YAML file")
	taskName := flag.String("task", "", "Name of the task to execute")
	topic := flag.String("topic", "Artificial Intelligence", "Topic for the agents to work on")
	openaiApiKey := flag.String("openai-key", "", "OpenAI API key (or set OPENAI_API_KEY env var)")
	flag.Parse()

	// Validate required flags
	if *agentConfigPath == "" || *taskConfigPath == "" || *taskName == "" {
		fmt.Println("Usage: yaml_config --agent-config=<path> --task-config=<path> --task=<name> [--topic=<topic>] [--openai-key=<key>]")
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

	// Load agent configurations
	agentConfigs, err := agent.LoadAgentConfigsFromFile(*agentConfigPath)
	if err != nil {
		log.Fatalf("Failed to load agent configurations: %v", err)
	}

	// Load task configurations
	taskConfigs, err := agent.LoadTaskConfigsFromFile(*taskConfigPath)
	if err != nil {
		log.Fatalf("Failed to load task configurations: %v", err)
	}

	// Create variables map for template substitution
	variables := map[string]string{
		"topic": *topic,
	}

	// Create the agent for the specified task
	agent, err := agent.CreateAgentForTask(*taskName, agentConfigs, taskConfigs, variables, agent.WithLLM(llm))
	if err != nil {
		log.Fatalf("Failed to create agent for task: %v", err)
	}

	// Execute the task
	fmt.Printf("Executing task '%s' with topic '%s'...\n", *taskName, *topic)
	result, err := agent.ExecuteTaskFromConfig(context.Background(), *taskName, taskConfigs, variables)
	if err != nil {
		log.Fatalf("Failed to execute task: %v", err)
	}

	// Print the result
	fmt.Println("\nTask Result:")
	fmt.Println("---------------------------------------------")
	fmt.Println(result)
	fmt.Println("---------------------------------------------")

	// Check if the task has an output file
	taskConfig := taskConfigs[*taskName]
	if taskConfig.OutputFile != "" {
		outputPath := taskConfig.OutputFile
		for key, value := range variables {
			placeholder := fmt.Sprintf("{%s}", key)
			outputPath = filepath.Clean(strings.ReplaceAll(outputPath, placeholder, value))
		}
		fmt.Printf("\nOutput also saved to: %s\n", outputPath)
	}
}
