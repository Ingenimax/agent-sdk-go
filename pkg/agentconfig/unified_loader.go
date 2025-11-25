package agentconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"gopkg.in/yaml.v3"
)

// ConfigSource indicates where the configuration came from
type ConfigSource string

const (
	ConfigSourceRemote ConfigSource = "remote"
	ConfigSourceLocal  ConfigSource = "local"
	ConfigSourceCache  ConfigSource = "cache"
)

// LoadOptions configures how agent configurations are loaded
type LoadOptions struct {
	// Source preferences
	PreferRemote       bool   // Try remote first
	AllowFallback      bool   // Fall back to local if remote fails
	LocalPath          string // Specific local file path

	// Caching
	EnableCache        bool
	CacheTimeout       time.Duration

	// Behavior
	EnableEnvOverrides bool
	Verbose           bool  // Log loading steps
}

// DefaultLoadOptions returns sensible defaults
func DefaultLoadOptions() *LoadOptions {
	return &LoadOptions{
		PreferRemote:       true,  // Try remote first
		AllowFallback:      true,  // Fall back to local if remote fails
		EnableCache:        true,
		CacheTimeout:       5 * time.Minute,
		EnableEnvOverrides: true,
		Verbose:           false,
	}
}

// LoadOption is a functional option
type LoadOption func(*LoadOptions)

// WithLocalFallback enables fallback to local file
func WithLocalFallback(path string) LoadOption {
	return func(opts *LoadOptions) {
		opts.AllowFallback = true
		opts.LocalPath = path
	}
}

// WithCache enables caching with specified timeout
func WithCache(timeout time.Duration) LoadOption {
	return func(opts *LoadOptions) {
		opts.EnableCache = true
		opts.CacheTimeout = timeout
	}
}

// WithoutCache disables caching
func WithoutCache() LoadOption {
	return func(opts *LoadOptions) {
		opts.EnableCache = false
	}
}

// WithEnvOverrides enables environment variable overrides
func WithEnvOverrides() LoadOption {
	return func(opts *LoadOptions) {
		opts.EnableEnvOverrides = true
	}
}

// WithVerbose enables verbose logging
func WithVerbose() LoadOption {
	return func(opts *LoadOptions) {
		opts.Verbose = true
	}
}

// WithRemoteOnly forces remote configuration only
func WithRemoteOnly() LoadOption {
	return func(opts *LoadOptions) {
		opts.PreferRemote = true
		opts.AllowFallback = false
	}
}

// WithLocalOnly forces local configuration only
func WithLocalOnly() LoadOption {
	return func(opts *LoadOptions) {
		opts.PreferRemote = false
		opts.AllowFallback = false
	}
}

// LoadAgentConfig is the main entry point for loading agent configurations
// It uses AGENT_DEPLOYMENT_ID to load configuration from remote, then falls back to local if configured
func LoadAgentConfig(ctx context.Context, agentName, environment string, options ...LoadOption) (*agent.AgentConfig, error) {
	// Get agent deployment ID from environment
	agentID := os.Getenv("AGENT_DEPLOYMENT_ID")
	if agentID == "" {
		return nil, fmt.Errorf("AGENT_DEPLOYMENT_ID environment variable is required")
	}

	// Apply options
	opts := DefaultLoadOptions()
	for _, option := range options {
		option(opts)
	}

	if opts.Verbose {
		fmt.Printf("Loading agent config: agent_id=%s (env: %s)\n", agentID, environment)
	}

	// Try cache first if enabled
	if opts.EnableCache {
		cacheKey := fmt.Sprintf("%s:%s", agentID, environment)
		if cached := getFromCache(cacheKey); cached != nil {
			if opts.Verbose {
				fmt.Printf("Loaded from cache: %s\n", cacheKey)
			}
			return cached, nil
		}
	}

	var config *agent.AgentConfig
	var source ConfigSource
	var err error

	// Try remote first if preferred
	if opts.PreferRemote {
		config, err = loadFromRemoteByID(ctx, agentID, environment, opts)
		if err == nil {
			source = ConfigSourceRemote
		} else if opts.Verbose {
			fmt.Printf("Remote loading failed: %v\n", err)
		}
	}

	// Fall back to local if remote failed and fallback is enabled
	if config == nil && opts.AllowFallback {
		config, err = loadFromLocal(agentName, environment, opts)
		if err == nil {
			source = ConfigSourceLocal
		} else if opts.Verbose {
			fmt.Printf("Local loading failed: %v\n", err)
		}
	}

	if config == nil {
		return nil, fmt.Errorf("failed to load agent config from any source: %w", err)
	}

	// Add source metadata
	if config.ConfigSource == nil {
		config.ConfigSource = &agent.ConfigSourceMetadata{}
	}
	config.ConfigSource.Type = string(source)
	config.ConfigSource.AgentID = agentID
	config.ConfigSource.AgentName = agentName
	config.ConfigSource.Environment = environment
	config.ConfigSource.LoadedAt = time.Now()

	// Apply environment overrides if enabled
	if opts.EnableEnvOverrides {
		*config = agent.ExpandAgentConfig(*config)
	}

	// Cache the result if enabled
	if opts.EnableCache {
		cacheKey := fmt.Sprintf("%s:%s", agentID, environment)
		cacheConfig(cacheKey, config, opts.CacheTimeout)
	}

	if opts.Verbose {
		fmt.Printf("Successfully loaded from %s\n", source)
	}

	return config, nil
}

// loadFromRemoteByID loads configuration from starops-config-service using agent_id
func loadFromRemoteByID(ctx context.Context, agentID, environment string, opts *LoadOptions) (*agent.AgentConfig, error) {
	// Create client
	client, err := NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create config client: %w", err)
	}

	// Fetch from remote service using agent_id
	response, err := client.FetchAgentConfig(ctx, agentID, environment)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote config: %w", err)
	}

	// Parse the resolved YAML into AgentConfig
	var config agent.AgentConfig
	if err := yaml.Unmarshal([]byte(response.ResolvedYAML), &config); err != nil {
		return nil, fmt.Errorf("failed to parse remote YAML: %w", err)
	}

	// Set source metadata
	config.ConfigSource = &agent.ConfigSourceMetadata{
		Type:      "remote",
		Source:    fmt.Sprintf("starops-config-service://agent_id=%s/%s", agentID, environment),
		Variables: response.ResolvedVariables,
	}

	return &config, nil
}

// loadFromRemote loads configuration from starops-config-service
// Deprecated: Use loadFromRemoteByID instead
func loadFromRemote(ctx context.Context, agentName, environment string, opts *LoadOptions) (*agent.AgentConfig, error) {
	// Create client
	client, err := NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create config client: %w", err)
	}

	// Fetch from remote service
	response, err := client.FetchAgentConfig(ctx, agentName, environment)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote config: %w", err)
	}

	// Parse the resolved YAML into AgentConfig
	var config agent.AgentConfig
	if err := yaml.Unmarshal([]byte(response.ResolvedYAML), &config); err != nil {
		return nil, fmt.Errorf("failed to parse remote YAML: %w", err)
	}

	// Set source metadata
	config.ConfigSource = &agent.ConfigSourceMetadata{
		Type:      "remote",
		Source:    fmt.Sprintf("starops-config-service://%s/%s", agentName, environment),
		Variables: response.ResolvedVariables,
	}

	return &config, nil
}

// loadFromLocal loads configuration from local YAML file
func loadFromLocal(agentName, environment string, opts *LoadOptions) (*agent.AgentConfig, error) {
	// Determine file path
	localPath := opts.LocalPath
	if localPath == "" {
		// Try common locations
		possiblePaths := []string{
			fmt.Sprintf("./configs/%s.yaml", agentName),
			fmt.Sprintf("./configs/%s-%s.yaml", agentName, environment),
			fmt.Sprintf("./agents/%s.yaml", agentName),
			fmt.Sprintf("./%s.yaml", agentName),
		}

		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				localPath = path
				break
			}
		}

		if localPath == "" {
			return nil, fmt.Errorf("no local configuration file found for agent %s", agentName)
		}
	}

	// Use existing LoadAgentConfigsFromFile to load the file
	configs, err := agent.LoadAgentConfigsFromFile(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load local config: %w", err)
	}

	// Get the specific agent config
	config, exists := configs[agentName]
	if !exists {
		// Try loading as single agent config
		// #nosec G304 - localPath is controlled by application logic, not user input
		data, err := os.ReadFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
	}

	// Set source metadata
	absPath, _ := filepath.Abs(localPath)
	config.ConfigSource = &agent.ConfigSourceMetadata{
		Type:   "local",
		Source: absPath,
	}

	return &config, nil
}