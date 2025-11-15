package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
)

// MCPToolConfig represents a tool provided by an MCP server
type MCPToolConfig struct {
	Name        string      `json:"name" yaml:"name"`
	Description string      `json:"description" yaml:"description"`
	Schema      interface{} `json:"schema" yaml:"schema"`
}

// MCPServerConfig represents MCP server configuration for JSON/YAML
type MCPServerConfig struct {
	Name        string            `json:"name" yaml:"name"`
	Type        string            `json:"type" yaml:"type"` // "stdio" or "http"
	URL         string            `json:"url,omitempty" yaml:"url,omitempty"`
	Command     string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args        []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env         []string          `json:"env,omitempty" yaml:"env,omitempty"`
	Token       string            `json:"token,omitempty" yaml:"token,omitempty"`
	Enabled     bool              `json:"enabled" yaml:"enabled"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	Config      map[string]string `json:"config,omitempty" yaml:"config,omitempty"`
	Tools       []MCPToolConfig   `json:"tools,omitempty" yaml:"tools,omitempty"`
}

// MCPConfiguration represents the complete MCP configuration
type MCPConfiguration struct {
	Servers []MCPServerConfig `json:"servers" yaml:"servers"`
	Global  MCPGlobalConfig   `json:"global,omitempty" yaml:"global,omitempty"`
}

// MCPGlobalConfig represents global MCP settings
type MCPGlobalConfig struct {
	Timeout         string `json:"timeout,omitempty" yaml:"timeout,omitempty"` // e.g., "30s"
	RetryAttempts   int    `json:"retry_attempts,omitempty" yaml:"retry_attempts,omitempty"`
	HealthCheck     bool   `json:"health_check" yaml:"health_check"`
	EnableResources bool   `json:"enable_resources" yaml:"enable_resources"`
	EnablePrompts   bool   `json:"enable_prompts" yaml:"enable_prompts"`
	EnableSampling  bool   `json:"enable_sampling" yaml:"enable_sampling"`
	EnableSchemas   bool   `json:"enable_schemas" yaml:"enable_schemas"`
	LogLevel        string `json:"log_level,omitempty" yaml:"log_level,omitempty"`
}

// LoadMCPConfigFromJSON loads MCP configuration from a JSON file
func LoadMCPConfigFromJSON(filePath string) (*MCPConfiguration, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON file: %w", err)
	}

	var config MCPConfiguration
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &config, nil
}

// LoadMCPConfigFromYAML loads MCP configuration from a YAML file
func LoadMCPConfigFromYAML(filePath string) (*MCPConfiguration, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file: %w", err)
	}

	var config MCPConfiguration
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &config, nil
}

// WithMCPConfigFromJSON adds MCP servers from a JSON configuration file
func WithMCPConfigFromJSON(filePath string) Option {
	return func(a *Agent) {
		config, err := LoadMCPConfigFromJSON(filePath)
		if err != nil {
			a.logger.Error(context.Background(), "Failed to load MCP config from JSON", map[string]interface{}{
				"file_path": filePath,
				"error":     err.Error(),
			})
			return
		}

		applyMCPConfig(a, config)
	}
}

// WithMCPConfigFromYAML adds MCP servers from a YAML configuration file
func WithMCPConfigFromYAML(filePath string) Option {
	return func(a *Agent) {
		config, err := LoadMCPConfigFromYAML(filePath)
		if err != nil {
			a.logger.Error(context.Background(), "Failed to load MCP config from YAML", map[string]interface{}{
				"file_path": filePath,
				"error":     err.Error(),
			})
			return
		}

		applyMCPConfig(a, config)
	}
}

// WithMCPConfig adds MCP servers from configuration object
func WithMCPConfig(config *MCPConfiguration) Option {
	return func(a *Agent) {
		applyMCPConfig(a, config)
	}
}

// applyMCPConfig applies MCP configuration to an agent
func applyMCPConfig(a *Agent, config *MCPConfiguration) {
	if config == nil {
		return
	}

	ctx := context.Background()

	// Create MCP builder
	builder := mcp.NewBuilder()

	// Apply global configuration
	if config.Global.Timeout != "" {
		// Parse timeout and apply (implementation depends on builder supporting this)
		if a.logger != nil {
			a.logger.Debug(ctx, "MCP global timeout configured", map[string]interface{}{
				"timeout": config.Global.Timeout,
			})
		}
	}

	if config.Global.RetryAttempts > 0 {
		// Apply retry configuration (implementation depends on builder supporting this)
		if a.logger != nil {
			a.logger.Debug(ctx, "MCP retry attempts configured", map[string]interface{}{
				"retry_attempts": config.Global.RetryAttempts,
			})
		}
	}

	// Convert server configurations to lazy MCP configs
	var lazyConfigs []LazyMCPConfig
	enabledCount := 0

	for _, serverConfig := range config.Servers {
		if !serverConfig.Enabled {
			if a.logger != nil {
				a.logger.Debug(ctx, "Skipping disabled MCP server", map[string]interface{}{
					"server_name": serverConfig.Name,
				})
			}
			continue
		}

		switch serverConfig.Type {
		case "stdio":
			builder.AddStdioServer(serverConfig.Name, serverConfig.Command, serverConfig.Args...)

			// Convert tools if specified, or leave empty to discover dynamically
			var toolConfigs []LazyMCPToolConfig
			for _, tool := range serverConfig.Tools {
				toolConfigs = append(toolConfigs, LazyMCPToolConfig{
					Name:        tool.Name,
					Description: tool.Description,
					Schema:      tool.Schema,
				})
			}

			lazyConfig := LazyMCPConfig{
				Name:    serverConfig.Name,
				Type:    "stdio",
				Command: serverConfig.Command,
				Args:    serverConfig.Args,
				Env:     serverConfig.Env,
				Tools:   toolConfigs, // Empty if not specified - will discover dynamically
			}
			lazyConfigs = append(lazyConfigs, lazyConfig)

		case "http":
			if serverConfig.Token != "" {
				builder.AddHTTPServerWithAuth(serverConfig.Name, serverConfig.URL, serverConfig.Token)
			} else {
				builder.AddHTTPServer(serverConfig.Name, serverConfig.URL)
			}

			// Convert tools if specified, or leave empty to discover dynamically
			var toolConfigs []LazyMCPToolConfig
			for _, tool := range serverConfig.Tools {
				toolConfigs = append(toolConfigs, LazyMCPToolConfig{
					Name:        tool.Name,
					Description: tool.Description,
					Schema:      tool.Schema,
				})
			}

			lazyConfig := LazyMCPConfig{
				Name:  serverConfig.Name,
				Type:  "http",
				URL:   serverConfig.URL,
				Tools: toolConfigs, // Empty if not specified - will discover dynamically
			}
			lazyConfigs = append(lazyConfigs, lazyConfig)

		default:
			if a.logger != nil {
				a.logger.Warn(ctx, "Unknown MCP server type", map[string]interface{}{
					"server_name": serverConfig.Name,
					"server_type": serverConfig.Type,
				})
			}
			continue
		}

		enabledCount++
		if a.logger != nil {
			a.logger.Info(ctx, "Configured MCP server from config", map[string]interface{}{
				"server_name": serverConfig.Name,
				"server_type": serverConfig.Type,
				"description": serverConfig.Description,
			})
		}
	}

	// Set lazy MCP configs on agent
	a.lazyMCPConfigs = lazyConfigs

	if a.logger != nil {
		a.logger.Info(ctx, "Applied MCP configuration", map[string]interface{}{
			"total_servers":   len(config.Servers),
			"enabled_servers": enabledCount,
			"global_config":   config.Global,
		})
	}
}

// SaveMCPConfigToJSON saves MCP configuration to a JSON file
func SaveMCPConfigToJSON(config *MCPConfiguration, filePath string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	return nil
}

// SaveMCPConfigToYAML saves MCP configuration to a YAML file
func SaveMCPConfigToYAML(config *MCPConfiguration, filePath string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write YAML file: %w", err)
	}

	return nil
}

// GetMCPConfigFromAgent extracts MCP configuration from an agent
func GetMCPConfigFromAgent(a *Agent) *MCPConfiguration {
	config := &MCPConfiguration{
		Servers: make([]MCPServerConfig, 0),
		Global: MCPGlobalConfig{
			HealthCheck:     true,
			EnableResources: true,
			EnablePrompts:   true,
			EnableSampling:  true,
			EnableSchemas:   true,
		},
	}

	// Convert lazy MCP configs to server configs
	for _, lazyConfig := range a.lazyMCPConfigs {
		serverConfig := MCPServerConfig{
			Name:    lazyConfig.Name,
			Type:    lazyConfig.Type,
			URL:     lazyConfig.URL,
			Command: lazyConfig.Command,
			Args:    lazyConfig.Args,
			Env:     lazyConfig.Env,
			Enabled: true,
		}
		config.Servers = append(config.Servers, serverConfig)
	}

	return config
}

// ValidateMCPConfig validates an MCP configuration
func ValidateMCPConfig(config *MCPConfiguration) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	serverNames := make(map[string]bool)

	for i, server := range config.Servers {
		// Check for required fields
		if server.Name == "" {
			return fmt.Errorf("server %d: name is required", i)
		}

		// Check for duplicate names
		if serverNames[server.Name] {
			return fmt.Errorf("duplicate server name: %s", server.Name)
		}
		serverNames[server.Name] = true

		// Validate server type
		if server.Type != "stdio" && server.Type != "http" {
			return fmt.Errorf("server %s: invalid type %s (must be 'stdio' or 'http')", server.Name, server.Type)
		}

		// Type-specific validation
		switch server.Type {
		case "stdio":
			if server.Command == "" {
				return fmt.Errorf("server %s: command is required for stdio type", server.Name)
			}
		case "http":
			if server.URL == "" {
				return fmt.Errorf("server %s: url is required for http type", server.Name)
			}
		}
	}

	return nil
}
