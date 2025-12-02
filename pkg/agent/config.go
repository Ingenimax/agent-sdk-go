package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"gopkg.in/yaml.v3"
)

// ConfigSourceMetadata tracks where a configuration was loaded from
type ConfigSourceMetadata struct {
	Type        string            `yaml:"type" json:"type"`     // "local", "remote"
	Source      string            `yaml:"source" json:"source"` // file path or service URL
	AgentID     string            `yaml:"agent_id,omitempty" json:"agent_id,omitempty"`
	AgentName   string            `yaml:"agent_name,omitempty" json:"agent_name,omitempty"`
	Environment string            `yaml:"environment,omitempty" json:"environment,omitempty"`
	Variables   map[string]string `yaml:"variables,omitempty" json:"variables,omitempty"`
	LoadedAt    time.Time         `yaml:"loaded_at" json:"loaded_at"`
}

// ResponseFormatConfig represents the configuration for the response format of an agent or task
type ResponseFormatConfig struct {
	Type             string                 `yaml:"type"`
	SchemaName       string                 `yaml:"schema_name"`
	SchemaDefinition map[string]interface{} `yaml:"schema_definition"`
}

// AgentConfig represents the configuration for an agent loaded from YAML
type AgentConfig struct {
	Role           string                `yaml:"role"`
	Goal           string                `yaml:"goal"`
	Backstory      string                `yaml:"backstory"`
	ResponseFormat *ResponseFormatConfig `yaml:"response_format,omitempty"`
	MCP            *MCPConfiguration     `yaml:"mcp,omitempty"`

	// NEW: Behavioral settings
	MaxIterations       *int  `yaml:"max_iterations,omitempty"`
	RequirePlanApproval *bool `yaml:"require_plan_approval,omitempty"`

	// NEW: Complex configuration objects
	StreamConfig *StreamConfigYAML `yaml:"stream_config,omitempty"`
	LLMConfig    *LLMConfigYAML    `yaml:"llm_config,omitempty"`

	// NEW: LLM Provider configuration
	LLMProvider *LLMProviderYAML `yaml:"llm_provider,omitempty"`

	// NEW: Tool configurations
	Tools []ToolConfigYAML `yaml:"tools,omitempty"`

	// NEW: Memory configuration (config only)
	Memory *MemoryConfigYAML `yaml:"memory,omitempty"`

	// NEW: Runtime settings
	Runtime *RuntimeConfigYAML `yaml:"runtime,omitempty"`

	// NEW: Sub-agents configuration (recursive)
	SubAgents map[string]AgentConfig `yaml:"sub_agents,omitempty"`

	// NEW: Configuration source metadata
	ConfigSource *ConfigSourceMetadata `yaml:"config_source,omitempty" json:"config_source,omitempty"`
}

// TaskConfig represents a task definition loaded from YAML
type TaskConfig struct {
	Description    string                `yaml:"description"`
	ExpectedOutput string                `yaml:"expected_output"`
	Agent          string                `yaml:"agent"`
	OutputFile     string                `yaml:"output_file,omitempty"`
	ResponseFormat *ResponseFormatConfig `yaml:"response_format,omitempty"`
}

// StreamConfigYAML represents streaming configuration in YAML
type StreamConfigYAML struct {
	BufferSize                  *int  `yaml:"buffer_size,omitempty"`
	IncludeToolProgress         *bool `yaml:"include_tool_progress,omitempty"`
	IncludeIntermediateMessages *bool `yaml:"include_intermediate_messages,omitempty"`
}

// LLMConfigYAML represents LLM configuration in YAML
type LLMConfigYAML struct {
	Temperature      *float64 `yaml:"temperature,omitempty"`
	TopP             *float64 `yaml:"top_p,omitempty"`
	FrequencyPenalty *float64 `yaml:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `yaml:"presence_penalty,omitempty"`
	StopSequences    []string `yaml:"stop_sequences,omitempty"`
	EnableReasoning  *bool    `yaml:"enable_reasoning,omitempty"`
	ReasoningBudget  *int     `yaml:"reasoning_budget,omitempty"`
	Reasoning        *string  `yaml:"reasoning,omitempty"`
}

// LLMProviderYAML represents LLM provider configuration in YAML
type LLMProviderYAML struct {
	Provider string                 `yaml:"provider"`
	Model    string                 `yaml:"model,omitempty"`
	Config   map[string]interface{} `yaml:"config,omitempty"`
}

// ToolConfigYAML represents tool configuration in YAML
type ToolConfigYAML struct {
	Type        string                 `yaml:"type"` // "builtin", "custom", "mcp", "agent"
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description,omitempty"`
	Config      map[string]interface{} `yaml:"config,omitempty"`
	Enabled     *bool                  `yaml:"enabled,omitempty"`

	// For agent tools
	URL     string `yaml:"url,omitempty"`     // Remote agent URL
	Timeout string `yaml:"timeout,omitempty"` // Timeout duration
}

// MemoryConfigYAML represents memory configuration in YAML
type MemoryConfigYAML struct {
	Type   string                 `yaml:"type"` // "buffer", "redis", "vector"
	Config map[string]interface{} `yaml:"config,omitempty"`
}

// RuntimeConfigYAML represents runtime behavior settings in YAML
type RuntimeConfigYAML struct {
	LogLevel        string `yaml:"log_level,omitempty"` // "debug", "info", "warn", "error"
	EnableTracing   *bool  `yaml:"enable_tracing,omitempty"`
	EnableMetrics   *bool  `yaml:"enable_metrics,omitempty"`
	TimeoutDuration string `yaml:"timeout_duration,omitempty"` // "30s", "5m"
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

// ConvertYAMLSchemaToResponseFormat converts a ResponseFormatConfig to interfaces.ResponseFormat
func ConvertYAMLSchemaToResponseFormat(config *ResponseFormatConfig) (*interfaces.ResponseFormat, error) {
	if config == nil {
		return nil, nil
	}

	schema := interfaces.JSONSchema(config.SchemaDefinition)
	return &interfaces.ResponseFormat{
		Type:   interfaces.ResponseFormatType(config.Type),
		Name:   config.SchemaName,
		Schema: schema,
	}, nil
}

// expandEnvironmentVariables expands environment variables in various types
func expandEnvironmentVariables(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		return ExpandEnv(v)
	case map[string]interface{}:
		return expandConfigMap(v)
	case []interface{}:
		expanded := make([]interface{}, len(v))
		for i, item := range v {
			expanded[i] = expandEnvironmentVariables(item)
		}
		return expanded
	default:
		return value
	}
}

// expandConfigMap expands environment variables in a configuration map
func expandConfigMap(config map[string]interface{}) map[string]interface{} {
	expanded := make(map[string]interface{})
	for key, value := range config {
		expanded[key] = expandEnvironmentVariables(value)
	}
	return expanded
}

// ExpandAgentConfig expands environment variables in agent configuration
func ExpandAgentConfig(config AgentConfig) AgentConfig {
	expanded := config
	expanded.Role = ExpandEnv(config.Role)
	expanded.Goal = ExpandEnv(config.Goal)
	expanded.Backstory = ExpandEnv(config.Backstory)

	// Expand memory configuration
	if config.Memory != nil && config.Memory.Config != nil {
		expanded.Memory = &MemoryConfigYAML{
			Type:   config.Memory.Type,
			Config: expandConfigMap(config.Memory.Config),
		}
	}

	// Expand tool configurations
	if config.Tools != nil {
		expandedTools := make([]ToolConfigYAML, len(config.Tools))
		for i, tool := range config.Tools {
			expandedTools[i] = ToolConfigYAML{
				Type:        tool.Type,
				Name:        tool.Name,
				Description: ExpandEnv(tool.Description),
				Config:      expandConfigMap(tool.Config),
				Enabled:     tool.Enabled,
				URL:         ExpandEnv(tool.URL),
				Timeout:     ExpandEnv(tool.Timeout),
			}
		}
		expanded.Tools = expandedTools
	}

	// Expand runtime configuration
	if config.Runtime != nil {
		expanded.Runtime = &RuntimeConfigYAML{
			LogLevel:        ExpandEnv(config.Runtime.LogLevel),
			EnableTracing:   config.Runtime.EnableTracing,
			EnableMetrics:   config.Runtime.EnableMetrics,
			TimeoutDuration: ExpandEnv(config.Runtime.TimeoutDuration),
		}
	}

	// Recursively expand sub-agents configuration
	if config.SubAgents != nil {
		expandedSubAgents := make(map[string]AgentConfig)
		for name, subAgentConfig := range config.SubAgents {
			expandedSubAgents[name] = ExpandAgentConfig(subAgentConfig) // Recursive expansion
		}
		expanded.SubAgents = expandedSubAgents
	}

	// Expand LLM provider configuration
	if config.LLMProvider != nil {
		expanded.LLMProvider = &LLMProviderYAML{
			Provider: ExpandEnv(config.LLMProvider.Provider),
			Model:    ExpandEnv(config.LLMProvider.Model),
			Config:   expandConfigMap(config.LLMProvider.Config),
		}
	}

	// Expand MCP configuration
	if config.MCP != nil {
		expandedMCP := &MCPConfiguration{
			MCPServers: make(map[string]MCPServerConfig),
			Global:     config.MCP.Global,
		}

		for serverName, serverConfig := range config.MCP.MCPServers {
			expandedServerConfig := MCPServerConfig{
				Command: ExpandEnv(serverConfig.Command),
				Args:    make([]string, len(serverConfig.Args)),
				Env:     make(map[string]string),
				URL:     ExpandEnv(serverConfig.URL),
				Token:   ExpandEnv(serverConfig.Token),
			}

			// Expand args
			for i, arg := range serverConfig.Args {
				expandedServerConfig.Args[i] = ExpandEnv(arg)
			}

			// Expand environment variables
			for key, value := range serverConfig.Env {
				expandedServerConfig.Env[key] = ExpandEnv(value)
			}

			expandedMCP.MCPServers[serverName] = expandedServerConfig
		}

		expanded.MCP = expandedMCP
	}

	return expanded
}

// convertStreamConfigYAMLToInterface converts StreamConfigYAML to interfaces.StreamConfig
func convertStreamConfigYAMLToInterface(config *StreamConfigYAML) *interfaces.StreamConfig {
	if config == nil {
		return nil
	}

	streamConfig := &interfaces.StreamConfig{}

	if config.BufferSize != nil {
		streamConfig.BufferSize = *config.BufferSize
	}
	if config.IncludeToolProgress != nil {
		streamConfig.IncludeToolProgress = *config.IncludeToolProgress
	}
	if config.IncludeIntermediateMessages != nil {
		streamConfig.IncludeIntermediateMessages = *config.IncludeIntermediateMessages
	}

	return streamConfig
}

// convertLLMConfigYAMLToInterface converts LLMConfigYAML to interfaces.LLMConfig
func convertLLMConfigYAMLToInterface(config *LLMConfigYAML) *interfaces.LLMConfig {
	if config == nil {
		return nil
	}

	llmConfig := &interfaces.LLMConfig{}

	if config.Temperature != nil {
		llmConfig.Temperature = *config.Temperature
	}
	if config.TopP != nil {
		llmConfig.TopP = *config.TopP
	}
	if config.FrequencyPenalty != nil {
		llmConfig.FrequencyPenalty = *config.FrequencyPenalty
	}
	if config.PresencePenalty != nil {
		llmConfig.PresencePenalty = *config.PresencePenalty
	}
	if config.StopSequences != nil {
		llmConfig.StopSequences = config.StopSequences
	}
	if config.EnableReasoning != nil {
		llmConfig.EnableReasoning = *config.EnableReasoning
	}
	if config.ReasoningBudget != nil {
		llmConfig.ReasoningBudget = *config.ReasoningBudget
	}
	if config.Reasoning != nil {
		llmConfig.Reasoning = *config.Reasoning
	}

	return llmConfig
}

// convertMemoryConfigYAMLToInterface converts MemoryConfigYAML to runtime memory config (placeholder)
// Note: This returns config data that will be used at runtime to create actual memory instances
func convertMemoryConfigYAMLToInterface(config *MemoryConfigYAML) map[string]interface{} {
	if config == nil {
		return nil
	}

	result := map[string]interface{}{
		"type": config.Type,
	}

	if config.Config != nil {
		for k, v := range config.Config {
			result[k] = v
		}
	}

	return result
}
