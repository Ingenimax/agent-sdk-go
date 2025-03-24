package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
	// Print welcome message
	fmt.Println("=== Agent Configuration Wizard ===")
	fmt.Println("This wizard helps you create and use agent configurations.")

	// Get OpenAI API key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Print("Enter your OpenAI API key: ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		apiKey = scanner.Text()
		if apiKey == "" {
			log.Fatal("OpenAI API key is required")
		}
	}

	// Create the LLM client
	llm := openai.NewClient(apiKey)

	// Menu
	for {
		fmt.Println("\nChoose an option:")
		fmt.Println("1. Create agent config from system prompt")
		fmt.Println("2. Load existing agent config")
		fmt.Println("3. Exit")
		fmt.Print("Enter choice (1-3): ")

		var choice string
		fmt.Scanln(&choice)

		switch choice {
		case "1":
			createAgentFromSystemPrompt(llm)
		case "2":
			loadExistingAgentConfig(llm)
		case "3":
			fmt.Println("Exiting...")
			return
		default:
			fmt.Println("Invalid choice. Please try again.")
		}
	}
}

func createAgentFromSystemPrompt(llm *openai.OpenAIClient) {
	// Get system prompt
	fmt.Println("\n=== Create Agent from System Prompt ===")
	fmt.Println("Enter a system prompt that describes the agent's role and capabilities.")
	fmt.Print("System prompt: ")

	reader := bufio.NewReader(os.Stdin)
	systemPrompt, _ := reader.ReadString('\n')
	systemPrompt = strings.TrimSpace(systemPrompt)

	if systemPrompt == "" {
		fmt.Println("System prompt cannot be empty")
		return
	}

	// Get agent name
	fmt.Print("Agent name (default: Auto-Configured Agent): ")
	var agentName string
	reader = bufio.NewReader(os.Stdin)
	agentName, _ = reader.ReadString('\n')
	agentName = strings.TrimSpace(agentName)

	if agentName == "" {
		agentName = "Auto-Configured Agent"
	}

	// Create agent with auto-config
	fmt.Println("\nGenerating agent configuration from prompt...")
	createdAgent, err := agent.NewAgentWithAutoConfig(
		context.Background(),
		agent.WithLLM(llm),
		agent.WithSystemPrompt(systemPrompt),
		agent.WithName(agentName),
	)
	if err != nil {
		fmt.Printf("Error creating agent: %v\n", err)
		return
	}

	// Check if auto-configuration succeeded
	agentConfig := createdAgent.GetGeneratedAgentConfig()
	taskConfigs := createdAgent.GetGeneratedTaskConfigs()

	if agentConfig == nil {
		fmt.Println("Failed to auto-generate agent configuration")
		return
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

	// Ask if the user wants to save the configurations
	fmt.Print("\nDo you want to save these configurations to YAML files? (y/n): ")
	var saveChoice string
	fmt.Scanln(&saveChoice)

	if strings.ToLower(saveChoice) == "y" {
		// Create a directory for the agent
		configDir := strings.ReplaceAll(agentName, " ", "_")
		err := os.MkdirAll(configDir, 0755)
		if err != nil {
			fmt.Printf("Error creating directory: %v\n", err)
			return
		}

		// Save agent config
		agentConfigMap := make(map[string]agent.AgentConfig)
		agentConfigMap[agentName] = *agentConfig

		agentYamlPath := filepath.Join(configDir, "agent_config.yaml")
		agentYaml, err := os.Create(agentYamlPath)
		if err != nil {
			fmt.Printf("Error creating agent config file: %v\n", err)
			return
		}
		defer agentYaml.Close()

		if err := agent.SaveAgentConfigsToFile(agentConfigMap, agentYaml); err != nil {
			fmt.Printf("Error saving agent configurations: %v\n", err)
			return
		}

		// Save task configs
		taskYamlPath := filepath.Join(configDir, "task_config.yaml")
		taskYaml, err := os.Create(taskYamlPath)
		if err != nil {
			fmt.Printf("Error creating task config file: %v\n", err)
			return
		}
		defer taskYaml.Close()

		if err := agent.SaveTaskConfigsToFile(taskConfigs, taskYaml); err != nil {
			fmt.Printf("Error saving task configurations: %v\n", err)
			return
		}

		fmt.Printf("\nConfigurations saved to %s/agent_config.yaml and %s/task_config.yaml\n", configDir, configDir)

		// Ask if the user wants to execute a task
		executeTask(llm, agentName, agentConfigMap, taskConfigs)
	}
}

func loadExistingAgentConfig(llm *openai.OpenAIClient) {
	fmt.Println("\n=== Load Existing Agent Configuration ===")

	// Ask for agent config directory
	fmt.Print("Enter the directory containing agent_config.yaml and task_config.yaml: ")
	var configDir string
	fmt.Scanln(&configDir)

	if configDir == "" {
		fmt.Println("Directory cannot be empty")
		return
	}

	// Load agent configs
	agentConfigPath := filepath.Join(configDir, "agent_config.yaml")
	agentConfigs, err := agent.LoadAgentConfigsFromFile(agentConfigPath)
	if err != nil {
		fmt.Printf("Error loading agent configs: %v\n", err)
		return
	}

	// Load task configs
	taskConfigPath := filepath.Join(configDir, "task_config.yaml")
	taskConfigs, err := agent.LoadTaskConfigsFromFile(taskConfigPath)
	if err != nil {
		fmt.Printf("Error loading task configs: %v\n", err)
		return
	}

	// Print available agents
	fmt.Println("\nAvailable agents:")
	var agentNames []string
	for name := range agentConfigs {
		agentNames = append(agentNames, name)
		fmt.Printf("- %s\n", name)
	}

	if len(agentNames) == 0 {
		fmt.Println("No agents found in configuration")
		return
	}

	// Select agent
	fmt.Print("\nEnter agent name: ")
	var agentName string
	reader := bufio.NewReader(os.Stdin)
	agentName, _ = reader.ReadString('\n')
	agentName = strings.TrimSpace(agentName)

	if agentName == "" {
		fmt.Println("Agent name cannot be empty")
		return
	}

	// Check if agent exists
	_, exists := agentConfigs[agentName]
	if !exists {
		fmt.Printf("Agent '%s' not found in configuration\n", agentName)
		return
	}

	// Print tasks for the selected agent
	fmt.Println("\nAvailable tasks:")
	var agentTasks []string
	for name, taskConfig := range taskConfigs {
		if taskConfig.Agent == agentName {
			agentTasks = append(agentTasks, name)
			fmt.Printf("- %s: %s\n", name, taskConfig.Description)
		}
	}

	if len(agentTasks) == 0 {
		fmt.Printf("No tasks found for agent '%s'\n", agentName)
		return
	}

	// Execute a task
	executeTask(llm, agentName, agentConfigs, taskConfigs)
}

func executeTask(llm *openai.OpenAIClient, agentName string, agentConfigs agent.AgentConfigs, taskConfigs agent.TaskConfigs) {
	// Print tasks for the selected agent
	fmt.Println("\nAvailable tasks:")
	var agentTasks []string
	for name, taskConfig := range taskConfigs {
		if taskConfig.Agent == agentName {
			agentTasks = append(agentTasks, name)
			fmt.Printf("- %s: %s\n", name, taskConfig.Description)
		}
	}

	if len(agentTasks) == 0 {
		fmt.Printf("No tasks found for agent '%s'\n", agentName)
		return
	}

	// Ask if the user wants to execute a task
	fmt.Print("\nDo you want to execute a task? (y/n): ")
	var executeChoice string
	fmt.Scanln(&executeChoice)

	if strings.ToLower(executeChoice) != "y" {
		return
	}

	// Select task
	fmt.Print("Enter task name: ")
	var taskName string
	reader := bufio.NewReader(os.Stdin)
	taskName, _ = reader.ReadString('\n')
	taskName = strings.TrimSpace(taskName)

	if taskName == "" {
		fmt.Println("Task name cannot be empty")
		return
	}

	// Check if task exists
	taskConfig, exists := taskConfigs[taskName]
	if !exists {
		fmt.Printf("Task '%s' not found in configuration\n", taskName)
		return
	}

	// Check if task is for the selected agent
	if taskConfig.Agent != agentName {
		fmt.Printf("Task '%s' is not for agent '%s'\n", taskName, agentName)
		return
	}

	// Collect variables for template substitution
	variables := make(map[string]string)

	// Parse the task description to find variables
	description := taskConfig.Description
	varStart := strings.Index(description, "{")
	for varStart != -1 {
		varEnd := strings.Index(description[varStart:], "}")
		if varEnd == -1 {
			break
		}

		varEnd += varStart
		varName := description[varStart+1 : varEnd]

		// Check if we already have this variable
		if _, exists := variables[varName]; !exists {
			fmt.Printf("Enter value for {%s}: ", varName)
			var varValue string
			reader = bufio.NewReader(os.Stdin)
			varValue, _ = reader.ReadString('\n')
			varValue = strings.TrimSpace(varValue)

			variables[varName] = varValue
		}

		// Find next variable
		varStart = strings.Index(description[varEnd+1:], "{")
		if varStart != -1 {
			varStart += varEnd + 1
		}
	}

	// Create agent for the task
	fmt.Printf("\nCreating agent '%s' for task '%s'...\n", agentName, taskName)
	createdAgent, err := agent.CreateAgentForTask(taskName, agentConfigs, taskConfigs, variables, agent.WithLLM(llm))
	if err != nil {
		fmt.Printf("Error creating agent: %v\n", err)
		return
	}

	// Execute the task
	fmt.Printf("Executing task '%s'...\n", taskName)
	result, err := createdAgent.ExecuteTaskFromConfig(context.Background(), taskName, taskConfigs, variables)
	if err != nil {
		fmt.Printf("Error executing task: %v\n", err)
		return
	}

	// Print the result
	fmt.Println("\n=== Task Result ===")
	fmt.Println(result)

	// Check if the task has an output file
	if taskConfig.OutputFile != "" {
		outputPath := taskConfig.OutputFile
		for key, value := range variables {
			placeholder := fmt.Sprintf("{%s}", key)
			outputPath = strings.ReplaceAll(outputPath, placeholder, value)
		}
		fmt.Printf("\nOutput also saved to: %s\n", outputPath)
	}
}
