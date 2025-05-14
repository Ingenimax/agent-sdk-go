package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"gopkg.in/yaml.v3"
)

// AgentConfig represents the configuration for an agent loaded from YAML
type AgentConfig struct {
	Role      string `yaml:"role"`
	Goal      string `yaml:"goal"`
	Backstory string `yaml:"backstory"`
}

// TaskConfig represents a task definition loaded from YAML
type TaskConfig struct {
	Description    string `yaml:"description"`
	ExpectedOutput string `yaml:"expected_output"`
	Agent          string `yaml:"agent"`
	OutputFile     string `yaml:"output_file,omitempty"`
}

// AgentConfigs represents a map of agent configurations
type AgentConfigs map[string]AgentConfig

// TaskConfigs represents a map of task configurations
type TaskConfigs map[string]TaskConfig

// LoadAgentConfigsFromFile loads agent configurations from a YAML file
func LoadAgentConfigsFromFile(filePath string) (AgentConfigs, error) {
	// Validate file path
	if !isValidFilePath(filePath) {
		return nil, fmt.Errorf("invalid file path")
	}

	// Read file safely
	data, err := os.ReadFile(filePath) // #nosec G304 - Path is validated with isValidFilePath() before use
	if err != nil {
		return nil, fmt.Errorf("failed to read agent config file: %w", err)
	}

	var configs AgentConfigs
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent configs: %w", err)
	}

	return configs, nil
}

// isValidFilePath checks if a file path is valid and safe
func isValidFilePath(filePath string) bool {
	// Check for empty path
	if filePath == "" {
		return false
	}

	// Clean and normalize the path
	cleanPath := filepath.Clean(filePath)

	// Check for path traversal attempts
	if strings.Contains(cleanPath, "..") {
		return false
	}

	// Get absolute path
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return false
	}

	// On Unix systems, check if the path is absolute and doesn't start with /proc, /sys, etc.
	// which could lead to sensitive information disclosure
	if strings.HasPrefix(absPath, "/proc") ||
		strings.HasPrefix(absPath, "/sys") ||
		strings.HasPrefix(absPath, "/dev") {
		return false
	}

	// Ensure the file exists
	fileInfo, err := os.Stat(cleanPath)
	if err != nil {
		return false
	}

	// Ensure it's a regular file, not a directory or symlink
	return fileInfo.Mode().IsRegular()
}

// LoadAgentConfigsFromDir loads all agent configurations from YAML files in a directory
func LoadAgentConfigsFromDir(dirPath string) (AgentConfigs, error) {
	// Validate directory path
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to access directory: %w", err)
	}

	if !dirInfo.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", dirPath)
	}

	files, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read agent config directory: %w", err)
	}

	configs := make(AgentConfigs)
	for _, file := range files {
		if file.IsDir() || (!strings.HasSuffix(file.Name(), ".yaml") && !strings.HasSuffix(file.Name(), ".yml")) {
			continue
		}

		filePath := filepath.Join(dirPath, file.Name())

		// Validate the file path before loading
		if !isValidFilePath(filePath) {
			continue // Skip invalid files but don't fail completely
		}

		fileConfigs, err := LoadAgentConfigsFromFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load agent configs from %s: %w", filePath, err)
		}

		// Merge configs
		for name, config := range fileConfigs {
			configs[name] = config
		}
	}

	return configs, nil
}

// LoadTaskConfigsFromFile loads task configurations from a YAML file
func LoadTaskConfigsFromFile(filePath string) (TaskConfigs, error) {
	// Validate file path
	if !isValidFilePath(filePath) {
		return nil, fmt.Errorf("invalid file path")
	}

	// Read file safely
	data, err := os.ReadFile(filePath) // #nosec G304 - Path is validated with isValidFilePath() before use
	if err != nil {
		return nil, fmt.Errorf("failed to read task config file: %w", err)
	}

	var configs TaskConfigs
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task configs: %w", err)
	}

	return configs, nil
}

// LoadTaskConfigsFromDir loads all task configurations from YAML files in a directory
func LoadTaskConfigsFromDir(dirPath string) (TaskConfigs, error) {
	// Validate directory path
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to access directory: %w", err)
	}

	if !dirInfo.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", dirPath)
	}

	files, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read task config directory: %w", err)
	}

	configs := make(TaskConfigs)
	for _, file := range files {
		if file.IsDir() || (!strings.HasSuffix(file.Name(), ".yaml") && !strings.HasSuffix(file.Name(), ".yml")) {
			continue
		}

		filePath := filepath.Join(dirPath, file.Name())

		// Validate the file path before loading
		if !isValidFilePath(filePath) {
			continue // Skip invalid files but don't fail completely
		}

		fileConfigs, err := LoadTaskConfigsFromFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load task configs from %s: %w", filePath, err)
		}

		// Merge configs
		for name, config := range fileConfigs {
			configs[name] = config
		}
	}

	return configs, nil
}

// FormatSystemPromptFromConfig formats a system prompt based on the agent configuration
func FormatSystemPromptFromConfig(config AgentConfig, variables map[string]string) string {
	role := config.Role
	goal := config.Goal
	backstory := config.Backstory

	// Replace variables in the configuration
	for key, value := range variables {
		placeholder := fmt.Sprintf("{%s}", key)
		role = strings.ReplaceAll(role, placeholder, value)
		goal = strings.ReplaceAll(goal, placeholder, value)
		backstory = strings.ReplaceAll(backstory, placeholder, value)
	}

	return fmt.Sprintf("# Role\n%s\n\n# Goal\n%s\n\n# Backstory\n%s", role, goal, backstory)
}

// GetAgentForTask returns the agent name for a given task
func GetAgentForTask(taskConfigs TaskConfigs, taskName string) (string, error) {
	taskConfig, exists := taskConfigs[taskName]
	if !exists {
		return "", fmt.Errorf("task %s not found in configuration", taskName)
	}
	return taskConfig.Agent, nil
}

// GenerateConfigFromSystemPrompt uses the LLM to generate agent and task configurations from a system prompt
func GenerateConfigFromSystemPrompt(ctx context.Context, llm interfaces.LLM, systemPrompt string) (AgentConfig, []TaskConfig, error) {
	if systemPrompt == "" {
		return AgentConfig{}, nil, fmt.Errorf("system prompt cannot be empty")
	}

	// Create a prompt for the LLM to generate agent and task configurations
	prompt := fmt.Sprintf(`
Based on the following system prompt that defines an AI agent's role, create YAML configurations for the agent and potential tasks it can perform.

System prompt:
%s

I need you to create:
1. An agent configuration with role, goal, and backstory
2. At least 2 task configurations that this agent can perform, with description and expected output

Format your response as valid YAML with the following structure (no prose, just YAML):

agent:
  role: >
    [Agent's role/title]
  goal: >
    [Agent's primary goal]
  backstory: >
    [Agent's backstory]

tasks:
  task1_name:
    description: >
      [Description of the first task]
    expected_output: >
      [Expected output format and content]
  
  task2_name:
    description: >
      [Description of the second task]
    expected_output: >
      [Expected output format and content]
    output_file: task2_output.md  # Optional
`, systemPrompt)

	// Generate the configurations using the LLM
	response, err := llm.Generate(ctx, prompt)
	if err != nil {
		return AgentConfig{}, nil, fmt.Errorf("failed to generate configurations: %w", err)
	}

	// Parse the YAML response
	var configs struct {
		Agent AgentConfig           `yaml:"agent"`
		Tasks map[string]TaskConfig `yaml:"tasks"`
	}

	if err := yaml.Unmarshal([]byte(response), &configs); err != nil {
		// Try to extract just the YAML part if there's prose around it
		yamlStart := strings.Index(response, "agent:")
		if yamlStart == -1 {
			return AgentConfig{}, nil, fmt.Errorf("failed to find agent configuration in response: %w", err)
		}

		// Find the end of the YAML block
		var yamlEnd int
		lines := strings.Split(response[yamlStart:], "\n")
		for i, line := range lines {
			if line == "```" || line == "---" {
				yamlEnd = yamlStart + strings.Index(response[yamlStart:], line)
				break
			}
			if i == len(lines)-1 {
				yamlEnd = len(response)
			}
		}

		yamlContent := response[yamlStart:yamlEnd]

		if err := yaml.Unmarshal([]byte(yamlContent), &configs); err != nil {
			return AgentConfig{}, nil, fmt.Errorf("failed to parse generated configurations: %w", err)
		}
	}

	// Convert tasks map to slice
	taskConfigs := make([]TaskConfig, 0, len(configs.Tasks))
	for name, taskConfig := range configs.Tasks {
		// Set the agent name field to the task name since we're creating these for the same agent
		taskConfig.Agent = name
		taskConfigs = append(taskConfigs, taskConfig)
	}

	return configs.Agent, taskConfigs, nil
}

// SaveAgentConfigsToFile saves agent configurations to a YAML file
func SaveAgentConfigsToFile(configs AgentConfigs, file *os.File) error {
	data, err := yaml.Marshal(configs)
	if err != nil {
		return fmt.Errorf("failed to marshal agent configs: %w", err)
	}

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write agent configs to file: %w", err)
	}

	return nil
}

// SaveTaskConfigsToFile saves task configurations to a YAML file
func SaveTaskConfigsToFile(configs TaskConfigs, file *os.File) error {
	data, err := yaml.Marshal(configs)
	if err != nil {
		return fmt.Errorf("failed to marshal task configs: %w", err)
	}

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write task configs to file: %w", err)
	}

	return nil
}
