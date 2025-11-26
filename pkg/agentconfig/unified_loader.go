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
	ConfigSourceMerged ConfigSource = "merged" // Remote + Local merged
)

// MergeStrategy determines how configs are merged when both remote and local exist
type MergeStrategy string

const (
	// MergeStrategyNone - No merging, use only one source (default behavior)
	MergeStrategyNone MergeStrategy = "none"

	// MergeStrategyRemotePriority - Remote config is primary, local fills gaps (recommended)
	// Use case: Config server has authority, local provides defaults
	MergeStrategyRemotePriority MergeStrategy = "remote_priority"

	// MergeStrategyLocalPriority - Local config is primary, remote fills gaps
	// Use case: Local development with remote fallbacks
	MergeStrategyLocalPriority MergeStrategy = "local_priority"
)

// LoadOptions configures how agent configurations are loaded
type LoadOptions struct {
	// Source preferences
	PreferRemote       bool   // Try remote first
	AllowFallback      bool   // Fall back to local if remote fails
	LocalPath          string // Specific local file path

	// Merging
	MergeStrategy      MergeStrategy // How to merge remote and local configs

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
		MergeStrategy:      MergeStrategyNone, // No merging by default (backwards compatible)
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

// WithMergeStrategy sets the merge strategy for combining remote and local configs
func WithMergeStrategy(strategy MergeStrategy) LoadOption {
	return func(opts *LoadOptions) {
		opts.MergeStrategy = strategy
		// When merging, we need to load both sources
		if strategy != MergeStrategyNone {
			opts.AllowFallback = true // Ensure we try to load both
		}
	}
}

// WithRemotePriorityMerge enables merging with remote config taking priority
// Local config provides defaults for fields not set in remote
func WithRemotePriorityMerge() LoadOption {
	return WithMergeStrategy(MergeStrategyRemotePriority)
}

// WithLocalPriorityMerge enables merging with local config taking priority
// Remote config provides defaults for fields not set in local
func WithLocalPriorityMerge() LoadOption {
	return WithMergeStrategy(MergeStrategyLocalPriority)
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
	var remoteConfig *agent.AgentConfig
	var localConfig *agent.AgentConfig
	var source ConfigSource
	var err error
	var remoteErr, localErr error

	// If merging is enabled, load both configs
	if opts.MergeStrategy != MergeStrategyNone {
		if opts.Verbose {
			fmt.Printf("Merge strategy enabled: %s\n", opts.MergeStrategy)
		}

		// Load remote config
		remoteConfig, remoteErr = loadFromRemoteByID(ctx, agentID, environment, opts)
		if remoteErr != nil && opts.Verbose {
			fmt.Printf("Remote loading failed (will merge with local if available): %v\n", remoteErr)
		}

		// Load local config
		localConfig, localErr = loadFromLocal(agentName, environment, opts)
		if localErr != nil && opts.Verbose {
			fmt.Printf("Local loading failed (will merge with remote if available): %v\n", localErr)
		}

		// Perform merge based on strategy
		if opts.MergeStrategy == MergeStrategyRemotePriority {
			if remoteConfig != nil && localConfig != nil {
				// Both configs available - merge with remote priority
				config = MergeAgentConfig(remoteConfig, localConfig, opts.MergeStrategy)
				source = ConfigSourceMerged
				if opts.Verbose {
					fmt.Printf("Merged remote (priority) + local configs\n")
				}
			} else if remoteConfig != nil {
				// Only remote available
				config = remoteConfig
				source = ConfigSourceRemote
			} else if localConfig != nil {
				// Only local available
				config = localConfig
				source = ConfigSourceLocal
			}
		} else if opts.MergeStrategy == MergeStrategyLocalPriority {
			if remoteConfig != nil && localConfig != nil {
				// Both configs available - merge with local priority
				config = MergeAgentConfig(localConfig, remoteConfig, opts.MergeStrategy)
				source = ConfigSourceMerged
				if opts.Verbose {
					fmt.Printf("Merged local (priority) + remote configs\n")
				}
			} else if localConfig != nil {
				// Only local available
				config = localConfig
				source = ConfigSourceLocal
			} else if remoteConfig != nil {
				// Only remote available
				config = remoteConfig
				source = ConfigSourceRemote
			}
		}

		// If no config loaded after merge attempt, return error
		if config == nil {
			return nil, fmt.Errorf("failed to load config for merging: remote error: %v, local error: %v", remoteErr, localErr)
		}
	} else {
		// No merging - use original behavior (either/or with fallback)
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
	}

	// Add source metadata (preserve existing metadata if already set by loader)
	if config.ConfigSource == nil {
		config.ConfigSource = &agent.ConfigSourceMetadata{}
	}
	// Only override if not already set by the loader
	if config.ConfigSource.Type == "" {
		config.ConfigSource.Type = string(source)
	}
	if config.ConfigSource.AgentID == "" {
		config.ConfigSource.AgentID = agentID
	}
	if config.ConfigSource.Environment == "" {
		config.ConfigSource.Environment = environment
	}
	// Keep the actual agent name from remote if available, otherwise use the parameter
	if config.ConfigSource.AgentName == "" {
		config.ConfigSource.AgentName = agentName
	}
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

	// Parse the resolved YAML - it has the agent name as top-level key
	// Format: agent_name: { role: "...", goal: "...", ... }
	var wrappedConfig map[string]agent.AgentConfig
	if err := yaml.Unmarshal([]byte(response.ResolvedYAML), &wrappedConfig); err != nil {
		return nil, fmt.Errorf("failed to parse remote YAML: %w", err)
	}

	// Extract the first (and only) agent config from the map
	if len(wrappedConfig) == 0 {
		return nil, fmt.Errorf("no agent configuration found in remote YAML")
	}

	var config agent.AgentConfig
	var actualAgentName string
	for name, cfg := range wrappedConfig {
		actualAgentName = name
		config = cfg
		fmt.Printf("[DEBUG] loadFromRemoteByID - Loaded config for agent: %s\n", actualAgentName)
		fmt.Printf("[DEBUG] loadFromRemoteByID - Role: %s\n", cfg.Role)
		fmt.Printf("[DEBUG] loadFromRemoteByID - Goal: %s\n", cfg.Goal)
		fmt.Printf("[DEBUG] loadFromRemoteByID - Backstory: %s\n", cfg.Backstory)
		break
	}

	// Set source metadata with the actual agent name from YAML
	config.ConfigSource = &agent.ConfigSourceMetadata{
		Type:        "remote",
		Source:      fmt.Sprintf("starops-config-service://agent_id=%s/%s", agentID, environment),
		AgentID:     agentID,
		AgentName:   actualAgentName, // Use the actual agent name from YAML
		Environment: environment,
		Variables:   response.ResolvedVariables,
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

// MergeAgentConfig merges two AgentConfig structs based on the specified strategy
// For RemotePriority: primary values override base values (remote overrides local)
// For LocalPriority: base values override primary values (local overrides remote)
func MergeAgentConfig(primary, base *agent.AgentConfig, strategy MergeStrategy) *agent.AgentConfig {
	if primary == nil {
		return base
	}
	if base == nil {
		return primary
	}

	// Create result starting with primary
	result := *primary

	// Helper to choose between primary and base for string fields
	mergeString := func(primaryVal, baseVal string) string {
		// Primary takes priority, use base only if primary is empty
		if primaryVal != "" {
			return primaryVal
		}
		return baseVal
	}

	// Merge basic string fields
	result.Role = mergeString(primary.Role, base.Role)
	result.Goal = mergeString(primary.Goal, base.Goal)
	result.Backstory = mergeString(primary.Backstory, base.Backstory)

	// Merge pointer fields (use base if primary is nil)
	if result.MaxIterations == nil && base.MaxIterations != nil {
		result.MaxIterations = base.MaxIterations
	}
	if result.RequirePlanApproval == nil && base.RequirePlanApproval != nil {
		result.RequirePlanApproval = base.RequirePlanApproval
	}

	// Merge ResponseFormat
	if result.ResponseFormat == nil && base.ResponseFormat != nil {
		result.ResponseFormat = base.ResponseFormat
	}

	// Merge MCP
	if result.MCP == nil && base.MCP != nil {
		result.MCP = base.MCP
	}

	// Merge StreamConfig
	if result.StreamConfig == nil && base.StreamConfig != nil {
		result.StreamConfig = base.StreamConfig
	}

	// Merge LLMConfig
	if result.LLMConfig == nil && base.LLMConfig != nil {
		result.LLMConfig = base.LLMConfig
	}

	// Merge LLMProvider
	if result.LLMProvider == nil && base.LLMProvider != nil {
		result.LLMProvider = base.LLMProvider
	} else if result.LLMProvider != nil && base.LLMProvider != nil {
		// Deep merge LLMProvider fields
		merged := *result.LLMProvider
		merged.Provider = mergeString(result.LLMProvider.Provider, base.LLMProvider.Provider)
		merged.Model = mergeString(result.LLMProvider.Model, base.LLMProvider.Model)
		if merged.Config == nil && base.LLMProvider.Config != nil {
			merged.Config = base.LLMProvider.Config
		}
		result.LLMProvider = &merged
	}

	// Merge Tools - use primary tools, append missing tools from base
	if result.Tools == nil && base.Tools != nil {
		result.Tools = base.Tools
	} else if result.Tools != nil && base.Tools != nil {
		// Create a map of existing tool names from primary
		existingTools := make(map[string]bool)
		for _, tool := range result.Tools {
			existingTools[tool.Name] = true
		}
		// Add base tools that don't exist in primary
		for _, baseTool := range base.Tools {
			if !existingTools[baseTool.Name] {
				result.Tools = append(result.Tools, baseTool)
			}
		}
	}

	// Merge Memory
	if result.Memory == nil && base.Memory != nil {
		result.Memory = base.Memory
	}

	// Merge Runtime
	if result.Runtime == nil && base.Runtime != nil {
		result.Runtime = base.Runtime
	} else if result.Runtime != nil && base.Runtime != nil {
		// Deep merge Runtime fields
		merged := *result.Runtime
		merged.LogLevel = mergeString(result.Runtime.LogLevel, base.Runtime.LogLevel)
		merged.TimeoutDuration = mergeString(result.Runtime.TimeoutDuration, base.Runtime.TimeoutDuration)
		result.Runtime = &merged
	}

	// Merge SubAgents recursively
	if result.SubAgents == nil && base.SubAgents != nil {
		result.SubAgents = base.SubAgents
	} else if result.SubAgents != nil && base.SubAgents != nil {
		// Merge sub-agents recursively
		for name, baseSubAgent := range base.SubAgents {
			if primarySubAgent, exists := result.SubAgents[name]; exists {
				// Recursively merge this sub-agent
				merged := MergeAgentConfig(&primarySubAgent, &baseSubAgent, strategy)
				result.SubAgents[name] = *merged
			} else {
				// Add base sub-agent if it doesn't exist in primary
				result.SubAgents[name] = baseSubAgent
			}
		}
	}

	// Merge ConfigSource metadata
	if result.ConfigSource != nil && base.ConfigSource != nil {
		result.ConfigSource.Type = string(ConfigSourceMerged)
		// Combine sources
		result.ConfigSource.Source = fmt.Sprintf("merged(%s + %s)",
			result.ConfigSource.Source, base.ConfigSource.Source)
		// Merge variables maps
		if result.ConfigSource.Variables == nil && base.ConfigSource.Variables != nil {
			result.ConfigSource.Variables = base.ConfigSource.Variables
		} else if result.ConfigSource.Variables != nil && base.ConfigSource.Variables != nil {
			merged := make(map[string]string)
			// Add base variables first
			for k, v := range base.ConfigSource.Variables {
				merged[k] = v
			}
			// Override with primary variables
			for k, v := range result.ConfigSource.Variables {
				merged[k] = v
			}
			result.ConfigSource.Variables = merged
		}
	}

	return &result
}