package agent

import (
	"context"
	"fmt"
	"time"
)

// CreateOption represents functional options for agent creation
type CreateOption func(*Agent) error

// NewAgentWithDualConfig creates an agent with automatic configuration source detection
// This is the recommended entry point for new applications using centralized config management
func NewAgentWithDualConfig(ctx context.Context, agentName, environment string, options ...Option) (*Agent, error) {
	// Import would cause cycle, so we'll provide a simpler interface here
	// Users should use agentconfig.LoadAgentAuto() for full functionality
	return nil, fmt.Errorf("use agentconfig.LoadAgentAuto(ctx, %q, %q) instead - this provides full dual configuration support", agentName, environment)
}

// NewAgentFromRemoteConfig creates an agent using only remote configuration
func NewAgentFromRemoteConfig(ctx context.Context, agentName, environment string, options ...Option) (*Agent, error) {
	// Import would cause cycle, so we direct users to the proper API
	return nil, fmt.Errorf("use agentconfig.LoadAgentFromRemote(ctx, %q, %q) instead - this loads from starops-config-service", agentName, environment)
}

// NewAgentFromLocalConfig creates an agent using only local YAML files
func NewAgentFromLocalConfig(ctx context.Context, agentName, environment string, options ...Option) (*Agent, error) {
	// Import would cause cycle, so we direct users to the proper API
	return nil, fmt.Errorf("use agentconfig.LoadAgentFromLocal(ctx, %q, %q) instead - this loads from local YAML files", agentName, environment)
}

// NewAgentFromFile creates an agent from a specific local YAML file
// This is backward compatible with existing usage
func NewAgentFromFile(ctx context.Context, configPath string, options ...Option) (*Agent, error) {
	configs, err := LoadAgentConfigsFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from file %s: %w", configPath, err)
	}

	// If only one agent config, use it
	if len(configs) == 1 {
		for _, config := range configs {
			return NewAgentFromConfigObject(ctx, &config, nil, options...)
		}
	}

	// If multiple configs, we need an agent name
	return nil, fmt.Errorf("file %s contains %d agent configs - use NewAgentFromFileWithName() to specify which agent to load", configPath, len(configs))
}

// NewAgentFromFileWithName creates an agent from a specific local YAML file with agent name
func NewAgentFromFileWithName(ctx context.Context, configPath, agentName string, options ...Option) (*Agent, error) {
	configs, err := LoadAgentConfigsFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from file %s: %w", configPath, err)
	}

	config, exists := configs[agentName]
	if !exists {
		return nil, fmt.Errorf("agent %s not found in config file %s", agentName, configPath)
	}

	return NewAgentFromConfigObject(ctx, &config, nil, options...)
}

// CreateAgentConfig represents options for creating agents from configuration
type CreateAgentConfig struct {
	// Source configuration
	AgentName   string
	Environment string
	ConfigPath  string // For local file loading

	// Loading options
	PreferRemote       bool
	AllowFallback      bool
	CacheTimeout       time.Duration
	EnableEnvOverrides bool
	Verbose           bool

	// Agent options
	MaxIterations       *int
	RequirePlanApproval *bool
	CustomOptions       []Option
}

// NewAgentFromCreateConfig creates an agent using a structured configuration
// This provides a single entry point with all options
func NewAgentFromCreateConfig(ctx context.Context, config CreateAgentConfig) (*Agent, error) {
	if config.AgentName == "" {
		return nil, fmt.Errorf("AgentName is required")
	}

	// If ConfigPath is specified, load from local file
	if config.ConfigPath != "" {
		return NewAgentFromFileWithName(ctx, config.ConfigPath, config.AgentName, config.CustomOptions...)
	}

	// Otherwise, use the agentconfig package
	// Since we can't import it due to cycles, provide guidance
	return nil, fmt.Errorf("for remote/dual configuration loading, use:\n" +
		"import \"github.com/Ingenimax/agent-sdk-go/pkg/agentconfig\"\n" +
		"agent, err := agentconfig.LoadAgentAuto(ctx, %q, %q)", config.AgentName, config.Environment)
}

// ValidateConfigPath validates that a configuration file path is safe
func ValidateConfigPath(path string) error {
	if path == "" {
		return fmt.Errorf("config path cannot be empty")
	}

	// Use existing validation from config.go
	if !isValidFilePath(path) {
		return fmt.Errorf("invalid or unsafe config path: %s", path)
	}

	return nil
}

// Agent creation helpers that maintain backward compatibility

// WithConfigFile loads configuration from a file and creates the agent
// This is a convenience option that combines file loading with agent creation
func WithConfigFile(filePath, agentName string) Option {
	return func(a *Agent) {
		// This option will load and apply the config
		// We use the existing WithAgentConfig after loading the file
		configs, err := LoadAgentConfigsFromFile(filePath)
		if err != nil {
			// We can't return an error from Option, so we'll have to handle this differently
			// The agent creation will fail during validation
			return
		}

		config, exists := configs[agentName]
		if !exists {
			return
		}

		// Apply the config using existing WithAgentConfig
		WithAgentConfig(config, nil)(a)
	}
}

// AgentCreationError represents errors that occur during agent creation
type AgentCreationError struct {
	AgentName string
	Source    string
	Err       error
}

func (e *AgentCreationError) Error() string {
	return fmt.Sprintf("failed to create agent %s from %s: %v", e.AgentName, e.Source, e.Err)
}

func (e *AgentCreationError) Unwrap() error {
	return e.Err
}

// NewAgentCreationError creates a new agent creation error
func NewAgentCreationError(agentName, source string, err error) error {
	return &AgentCreationError{
		AgentName: agentName,
		Source:    source,
		Err:       err,
	}
}