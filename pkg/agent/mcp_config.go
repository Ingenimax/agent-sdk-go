package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
)

// MCPServerConfig represents a single MCP server configuration
type MCPServerConfig struct {
	Command string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args    []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	URL     string            `json:"url,omitempty" yaml:"url,omitempty"`
	Token   string            `json:"token,omitempty" yaml:"token,omitempty"`
}

// MCPDiscoveredServerInfo represents metadata discovered from the server at runtime
type MCPDiscoveredServerInfo struct {
	Name         string                     `json:"name,omitempty"`
	Title        string                     `json:"title,omitempty"`
	Version      string                     `json:"version,omitempty"`
	Capabilities *MCPDiscoveredCapabilities `json:"capabilities,omitempty"`
}

// MCPDiscoveredCapabilities represents capabilities discovered from the server
type MCPDiscoveredCapabilities struct {
	SupportsTools     bool `json:"supportsTools,omitempty"`
	SupportsResources bool `json:"supportsResources,omitempty"`
	SupportsPrompts   bool `json:"supportsPrompts,omitempty"`
}

// GetServerType returns the server type based on configuration
func (c *MCPServerConfig) GetServerType() string {
	if c.URL != "" {
		return "http"
	}
	if c.Command != "" {
		return "stdio"
	}
	return "stdio" // Default to stdio
}

// MCPConfiguration represents the complete MCP configuration
type MCPConfiguration struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers" yaml:"mcpServers"`
	Global     *MCPGlobalConfig           `json:"global,omitempty" yaml:"global,omitempty"`
}

// MCPGlobalConfig represents global MCP settings
type MCPGlobalConfig struct {
	Timeout         string `json:"timeout,omitempty" yaml:"timeout,omitempty"` // e.g., "30s"
	RetryAttempts   int    `json:"retry_attempts,omitempty" yaml:"retry_attempts,omitempty"`
	HealthCheck     *bool  `json:"health_check,omitempty" yaml:"health_check,omitempty"`
	EnableResources *bool  `json:"enable_resources,omitempty" yaml:"enable_resources,omitempty"`
	EnablePrompts   *bool  `json:"enable_prompts,omitempty" yaml:"enable_prompts,omitempty"`
	EnableSampling  *bool  `json:"enable_sampling,omitempty" yaml:"enable_sampling,omitempty"`
	EnableSchemas   *bool  `json:"enable_schemas,omitempty" yaml:"enable_schemas,omitempty"`
	LogLevel        string `json:"log_level,omitempty" yaml:"log_level,omitempty"`
}

// LoadMCPConfigFromJSON loads MCP configuration from a JSON file
func LoadMCPConfigFromJSON(filePath string) (*MCPConfiguration, error) {
	// #nosec G304 - filePath is provided by the developer/user and expected to be a configuration file path
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
	// #nosec G304 - filePath is provided by the developer/user and expected to be a configuration file path
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
			return
		}

		fmt.Printf("MCP Config loaded from JSON: %s\n", filePath)
		applyMCPConfig(a, config)
	}
}

// WithMCPConfigFromYAML adds MCP servers from a YAML configuration file
func WithMCPConfigFromYAML(filePath string) Option {
	return func(a *Agent) {
		config, err := LoadMCPConfigFromYAML(filePath)
		if err != nil {
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

	// Apply global configuration if present, with defaults
	globalConfig := config.Global
	if globalConfig == nil {
		// Set defaults when no global config provided
		trueVal := true
		globalConfig = &MCPGlobalConfig{
			Timeout:         "30s",
			RetryAttempts:   3,
			HealthCheck:     &trueVal,
			EnableResources: &trueVal,
			EnablePrompts:   &trueVal,
			EnableSampling:  &trueVal,
			EnableSchemas:   &trueVal,
			LogLevel:        "info",
		}
	} else {
		// Apply defaults for unspecified values
		if globalConfig.Timeout == "" {
			globalConfig.Timeout = "30s"
		}
		if globalConfig.RetryAttempts == 0 {
			globalConfig.RetryAttempts = 3
		}
		// Set defaults for nil pointers (unspecified values)
		if globalConfig.HealthCheck == nil {
			trueVal := true
			globalConfig.HealthCheck = &trueVal
		}
		if globalConfig.EnableResources == nil {
			trueVal := true
			globalConfig.EnableResources = &trueVal
		}
		if globalConfig.EnablePrompts == nil {
			trueVal := true
			globalConfig.EnablePrompts = &trueVal
		}
		if globalConfig.EnableSampling == nil {
			trueVal := true
			globalConfig.EnableSampling = &trueVal
		}
		if globalConfig.EnableSchemas == nil {
			trueVal := true
			globalConfig.EnableSchemas = &trueVal
		}
		if globalConfig.LogLevel == "" {
			globalConfig.LogLevel = "info"
		}
	}

	// Apply timeout to builder
	if globalConfig.Timeout != "" {
		if timeout, err := time.ParseDuration(globalConfig.Timeout); err == nil {
			builder.WithTimeout(timeout)
			if a.logger != nil {
				a.logger.Debug(ctx, "MCP timeout configured", map[string]interface{}{
					"timeout": globalConfig.Timeout,
				})
			}
		}
	}

	// Apply retry attempts to builder
	if globalConfig.RetryAttempts > 0 {
		builder.WithRetry(globalConfig.RetryAttempts, 1*time.Second)
		if a.logger != nil {
			a.logger.Debug(ctx, "MCP retry attempts configured", map[string]interface{}{
				"retry_attempts": globalConfig.RetryAttempts,
			})
		}
	}

	// Apply health check to builder
	builder.WithHealthCheck(*globalConfig.HealthCheck)

	if a.logger != nil {
		a.logger.Debug(ctx, "MCP global configuration applied", map[string]interface{}{
			"health_check":     *globalConfig.HealthCheck,
			"enable_resources": *globalConfig.EnableResources,
			"enable_prompts":   *globalConfig.EnablePrompts,
			"enable_sampling":  *globalConfig.EnableSampling,
			"enable_schemas":   *globalConfig.EnableSchemas,
			"log_level":        globalConfig.LogLevel,
			"timeout":          globalConfig.Timeout,
			"retry_attempts":   globalConfig.RetryAttempts,
		})
	}

	// Convert server configurations to lazy MCP configs
	var lazyConfigs []LazyMCPConfig
	enabledCount := 0

	for serverName, serverConfig := range config.MCPServers {
		serverType := serverConfig.GetServerType()

		switch serverType {
		case "stdio":
			builder.AddStdioServer(serverName, serverConfig.Command, serverConfig.Args...)

			// Convert environment map to string slice format
			// Resolve environment variable placeholders (e.g., ${VAR_NAME})
			var envSlice []string
			for key, value := range serverConfig.Env {
				// Expand environment variables in the value
				resolvedValue := os.ExpandEnv(value)
				envSlice = append(envSlice, fmt.Sprintf("%s=%s", key, resolvedValue))

			}

			lazyConfig := LazyMCPConfig{
				Name:    serverName,
				Type:    "stdio",
				Command: serverConfig.Command,
				Args:    serverConfig.Args,
				Env:     envSlice,
				Tools:   []LazyMCPToolConfig{}, // Will discover dynamically
			}
			lazyConfigs = append(lazyConfigs, lazyConfig)

		case "http":
			if serverConfig.Token != "" {
				builder.AddHTTPServerWithAuth(serverName, serverConfig.URL, serverConfig.Token)
			} else {
				builder.AddHTTPServer(serverName, serverConfig.URL)
			}

			lazyConfig := LazyMCPConfig{
				Name:  serverName,
				Type:  "http",
				URL:   serverConfig.URL,
				Tools: []LazyMCPToolConfig{}, // Will discover dynamically
			}
			lazyConfigs = append(lazyConfigs, lazyConfig)

		default:
			if a.logger != nil {
				a.logger.Warn(ctx, "Unknown MCP server type", map[string]interface{}{
					"server_name": serverName,
					"server_type": serverType,
				})
			}
			continue
		}

		enabledCount++
		if a.logger != nil {
			a.logger.Info(ctx, "Configured MCP server from config", map[string]interface{}{
				"server_name": serverName,
				"server_type": serverType,
			})
		}
	}

	// Set lazy MCP configs on agent
	a.lazyMCPConfigs = lazyConfigs

	if a.logger != nil {
		a.logger.Info(ctx, "Applied MCP configuration", map[string]interface{}{
			"total_servers":   len(config.MCPServers),
			"enabled_servers": enabledCount,
		})
	}
}

// SaveMCPConfigToJSON saves MCP configuration to a JSON file
func SaveMCPConfigToJSON(config *MCPConfiguration, filePath string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// #nosec G306 - 0644 permissions are appropriate for config files that may need to be read by other processes
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

	// #nosec G306 - 0644 permissions are appropriate for config files that may need to be read by other processes
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write YAML file: %w", err)
	}

	return nil
}

// GetMCPConfigFromAgent extracts MCP configuration from an agent
func GetMCPConfigFromAgent(a *Agent) *MCPConfiguration {
	config := &MCPConfiguration{
		MCPServers: make(map[string]MCPServerConfig),
	}

	// Convert lazy MCP configs to server configs
	for _, lazyConfig := range a.lazyMCPConfigs {
		// Convert environment slice back to map
		envMap := make(map[string]string)
		for _, envVar := range lazyConfig.Env {
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}

		serverConfig := MCPServerConfig{
			URL:     lazyConfig.URL,
			Command: lazyConfig.Command,
			Args:    lazyConfig.Args,
			Env:     envMap,
		}
		config.MCPServers[lazyConfig.Name] = serverConfig
	}

	return config
}

// ValidateMCPConfig validates an MCP configuration
func ValidateMCPConfig(config *MCPConfiguration) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if config.MCPServers == nil {
		return fmt.Errorf("mcpServers cannot be nil")
	}

	for serverName, server := range config.MCPServers {
		// Check for required fields
		if serverName == "" {
			return fmt.Errorf("server name cannot be empty")
		}

		serverType := server.GetServerType()

		// Type-specific validation
		switch serverType {
		case "stdio":
			if server.Command == "" {
				return fmt.Errorf("server %s: command is required for stdio type", serverName)
			}
		case "http":
			if server.URL == "" {
				return fmt.Errorf("server %s: url is required for http type", serverName)
			}
		}
	}

	return nil
}
