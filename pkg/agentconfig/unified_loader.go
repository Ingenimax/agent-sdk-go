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
	ConfigSourceLocal ConfigSource = "local"
	ConfigSourceCache ConfigSource = "cache"

	// ConfigSourceMerged marks a config produced by MergeAgentConfig.
	ConfigSourceMerged ConfigSource = "merged"
)

// MergeStrategy determines which of two configs wins when MergeAgentConfig
// combines them.
type MergeStrategy string

const (
	// MergeStrategyNone - no merging.
	MergeStrategyNone MergeStrategy = "none"

	// MergeStrategyPrimaryPriority - the primary config wins; base fills gaps.
	MergeStrategyPrimaryPriority MergeStrategy = "primary_priority"

	// MergeStrategyBasePriority - the base config wins; primary fills gaps.
	MergeStrategyBasePriority MergeStrategy = "base_priority"
)

const (
	// MergeStrategyRemotePriority is retained for source compatibility.
	//
	// Deprecated: remote configuration loading was removed; there is no remote
	// config to prioritise. Use MergeStrategyPrimaryPriority.
	MergeStrategyRemotePriority = MergeStrategyPrimaryPriority

	// MergeStrategyLocalPriority is retained for source compatibility.
	//
	// Deprecated: remote configuration loading was removed; both inputs to
	// MergeAgentConfig are now local. Use MergeStrategyBasePriority.
	MergeStrategyLocalPriority = MergeStrategyBasePriority
)

// LoadOptions configures how agent configurations are loaded
type LoadOptions struct {
	// LocalPath is a specific config file path. When empty, a set of
	// conventional locations is searched.
	LocalPath string

	// Caching
	EnableCache  bool
	CacheTimeout time.Duration

	// Behavior
	EnableEnvOverrides bool
	Verbose            bool // Log loading steps
}

// DefaultLoadOptions returns sensible defaults
func DefaultLoadOptions() *LoadOptions {
	return &LoadOptions{
		EnableCache:        true,
		CacheTimeout:       5 * time.Minute,
		EnableEnvOverrides: true,
		Verbose:            false,
	}
}

// LoadOption is a functional option
type LoadOption func(*LoadOptions)

// WithLocalPath sets an explicit config file path.
func WithLocalPath(path string) LoadOption {
	return func(opts *LoadOptions) {
		opts.LocalPath = path
	}
}

// WithLocalFallback sets an explicit config file path.
//
// Deprecated: configuration is always loaded locally; there is no remote source
// to fall back from. Use WithLocalPath.
func WithLocalFallback(path string) LoadOption {
	return WithLocalPath(path)
}

// WithLocalOnly is a no-op.
//
// Deprecated: configuration is always loaded locally. This option has no effect.
func WithLocalOnly() LoadOption {
	return func(*LoadOptions) {}
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

// LoadAgentConfig is the main entry point for loading agent configurations.
//
// Configuration is loaded from a local YAML file. Remote configuration loading
// was removed: it fetched YAML over HTTP and unmarshalled it straight into an
// AgentConfig whose MCP section names a local executable to run, which handed
// the config server arbitrary command execution inside the agent process.
func LoadAgentConfig(_ context.Context, agentName, environment string, options ...LoadOption) (*agent.AgentConfig, error) {
	opts := DefaultLoadOptions()
	for _, option := range options {
		option(opts)
	}

	if opts.Verbose {
		fmt.Printf("Loading agent config: agent=%s (env: %s)\n", agentName, environment)
	}

	cacheKey := fmt.Sprintf("%s:%s", agentName, environment)
	if opts.EnableCache {
		if cached := getFromCache(cacheKey); cached != nil {
			if opts.Verbose {
				fmt.Printf("Loaded from cache: %s\n", cacheKey)
			}
			return cached, nil
		}
	}

	config, err := loadFromLocal(agentName, environment, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to load agent config: %w", err)
	}

	// Add source metadata (preserve existing metadata if already set by loader)
	if config.ConfigSource == nil {
		config.ConfigSource = &agent.ConfigSourceMetadata{}
	}
	if config.ConfigSource.Type == "" {
		config.ConfigSource.Type = string(ConfigSourceLocal)
	}
	if config.ConfigSource.Environment == "" {
		config.ConfigSource.Environment = environment
	}
	if config.ConfigSource.AgentName == "" {
		config.ConfigSource.AgentName = agentName
	}
	config.ConfigSource.LoadedAt = time.Now()

	// Apply environment overrides if enabled
	if opts.EnableEnvOverrides {
		*config = agent.ExpandAgentConfig(*config)
	}

	if opts.EnableCache {
		cacheConfig(cacheKey, config, opts.CacheTimeout)
	}

	if opts.Verbose {
		fmt.Printf("Successfully loaded from %s\n", ConfigSourceLocal)
	}

	return config, nil
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

// deepCopyAgentConfig creates a deep copy of an AgentConfig to prevent shared state
func deepCopyAgentConfig(src *agent.AgentConfig) *agent.AgentConfig {
	if src == nil {
		return nil
	}

	// Create new config with basic fields (strings are immutable, safe to copy)
	dst := &agent.AgentConfig{
		Role:      src.Role,
		Goal:      src.Goal,
		Backstory: src.Backstory,
	}

	// Deep copy pointer fields
	if src.MaxIterations != nil {
		val := *src.MaxIterations
		dst.MaxIterations = &val
	}

	if src.RequirePlanApproval != nil {
		val := *src.RequirePlanApproval
		dst.RequirePlanApproval = &val
	}

	// Deep copy ResponseFormat
	if src.ResponseFormat != nil {
		dst.ResponseFormat = &agent.ResponseFormatConfig{
			Type:       src.ResponseFormat.Type,
			SchemaName: src.ResponseFormat.SchemaName,
		}
		// Deep copy schema definition map
		if src.ResponseFormat.SchemaDefinition != nil {
			dst.ResponseFormat.SchemaDefinition = deepCopyMap(src.ResponseFormat.SchemaDefinition)
		}
	}

	// Deep copy MCP
	if src.MCP != nil {
		dst.MCP = &agent.MCPConfiguration{
			Global: src.MCP.Global,
		}
		// Deep copy MCPServers map
		if src.MCP.MCPServers != nil {
			dst.MCP.MCPServers = make(map[string]agent.MCPServerConfig)
			for k, v := range src.MCP.MCPServers {
				dst.MCP.MCPServers[k] = agent.MCPServerConfig{
					Command: v.Command,
					Args:    deepCopyStringSlice(v.Args),
					Env:     deepCopyStringMap(v.Env),
					URL:     v.URL,
					Token:   v.Token,
				}
			}
		}
	}

	// Deep copy StreamConfig
	if src.StreamConfig != nil {
		dst.StreamConfig = &agent.StreamConfigYAML{}
		if src.StreamConfig.BufferSize != nil {
			val := *src.StreamConfig.BufferSize
			dst.StreamConfig.BufferSize = &val
		}
		if src.StreamConfig.IncludeToolProgress != nil {
			val := *src.StreamConfig.IncludeToolProgress
			dst.StreamConfig.IncludeToolProgress = &val
		}
		if src.StreamConfig.IncludeIntermediateMessages != nil {
			val := *src.StreamConfig.IncludeIntermediateMessages
			dst.StreamConfig.IncludeIntermediateMessages = &val
		}
	}

	// Deep copy LLMConfig
	if src.LLMConfig != nil {
		dst.LLMConfig = &agent.LLMConfigYAML{}
		if src.LLMConfig.Temperature != nil {
			val := *src.LLMConfig.Temperature
			dst.LLMConfig.Temperature = &val
		}
		if src.LLMConfig.TopP != nil {
			val := *src.LLMConfig.TopP
			dst.LLMConfig.TopP = &val
		}
		if src.LLMConfig.FrequencyPenalty != nil {
			val := *src.LLMConfig.FrequencyPenalty
			dst.LLMConfig.FrequencyPenalty = &val
		}
		if src.LLMConfig.PresencePenalty != nil {
			val := *src.LLMConfig.PresencePenalty
			dst.LLMConfig.PresencePenalty = &val
		}
		if src.LLMConfig.EnableReasoning != nil {
			val := *src.LLMConfig.EnableReasoning
			dst.LLMConfig.EnableReasoning = &val
		}
		if src.LLMConfig.ReasoningBudget != nil {
			val := *src.LLMConfig.ReasoningBudget
			dst.LLMConfig.ReasoningBudget = &val
		}
		if src.LLMConfig.Reasoning != nil {
			val := *src.LLMConfig.Reasoning
			dst.LLMConfig.Reasoning = &val
		}
		dst.LLMConfig.StopSequences = deepCopyStringSlice(src.LLMConfig.StopSequences)
	}

	// Deep copy LLMProvider
	if src.LLMProvider != nil {
		dst.LLMProvider = &agent.LLMProviderYAML{
			Provider: src.LLMProvider.Provider,
			Model:    src.LLMProvider.Model,
			Config:   deepCopyMap(src.LLMProvider.Config),
		}
	}

	// Deep copy Tools slice
	if src.Tools != nil {
		dst.Tools = make([]agent.ToolConfigYAML, len(src.Tools))
		for i, tool := range src.Tools {
			dst.Tools[i] = agent.ToolConfigYAML{
				Type:        tool.Type,
				Name:        tool.Name,
				Description: tool.Description,
				Config:      deepCopyMap(tool.Config),
				URL:         tool.URL,
				Timeout:     tool.Timeout,
			}
			if tool.Enabled != nil {
				val := *tool.Enabled
				dst.Tools[i].Enabled = &val
			}
		}
	}

	// Deep copy Memory
	if src.Memory != nil {
		dst.Memory = &agent.MemoryConfigYAML{
			Type:   src.Memory.Type,
			Config: deepCopyMap(src.Memory.Config),
		}
	}

	// Deep copy Runtime
	if src.Runtime != nil {
		dst.Runtime = &agent.RuntimeConfigYAML{
			LogLevel:        src.Runtime.LogLevel,
			TimeoutDuration: src.Runtime.TimeoutDuration,
		}
		if src.Runtime.EnableTracing != nil {
			val := *src.Runtime.EnableTracing
			dst.Runtime.EnableTracing = &val
		}
		if src.Runtime.EnableMetrics != nil {
			val := *src.Runtime.EnableMetrics
			dst.Runtime.EnableMetrics = &val
		}
	}

	// Deep copy ImageGeneration
	if src.ImageGeneration != nil {
		dst.ImageGeneration = &agent.ImageGenerationYAML{
			Provider: src.ImageGeneration.Provider,
			Model:    src.ImageGeneration.Model,
			Config:   deepCopyMap(src.ImageGeneration.Config),
		}
		// Deep copy the Enabled bool pointer
		if src.ImageGeneration.Enabled != nil {
			val := *src.ImageGeneration.Enabled
			dst.ImageGeneration.Enabled = &val
		}
		if src.ImageGeneration.Storage != nil {
			dst.ImageGeneration.Storage = &agent.ImageStorageYAML{
				Type: src.ImageGeneration.Storage.Type,
			}
			if src.ImageGeneration.Storage.Local != nil {
				dst.ImageGeneration.Storage.Local = &agent.LocalStorageYAML{
					Path:    src.ImageGeneration.Storage.Local.Path,
					BaseURL: src.ImageGeneration.Storage.Local.BaseURL,
				}
			}
			if src.ImageGeneration.Storage.GCS != nil {
				dst.ImageGeneration.Storage.GCS = &agent.GCSStorageYAML{
					Bucket:              src.ImageGeneration.Storage.GCS.Bucket,
					Prefix:              src.ImageGeneration.Storage.GCS.Prefix,
					CredentialsFile:     src.ImageGeneration.Storage.GCS.CredentialsFile,
					CredentialsJSON:     src.ImageGeneration.Storage.GCS.CredentialsJSON,
					SignedURLExpiration: src.ImageGeneration.Storage.GCS.SignedURLExpiration,
				}
			}
		}
		if src.ImageGeneration.MultiTurnEditing != nil {
			dst.ImageGeneration.MultiTurnEditing = &agent.MultiTurnEditingYAML{
				Model:             src.ImageGeneration.MultiTurnEditing.Model,
				SessionTimeout:    src.ImageGeneration.MultiTurnEditing.SessionTimeout,
				MaxSessionsPerOrg: src.ImageGeneration.MultiTurnEditing.MaxSessionsPerOrg,
			}
			// Deep copy the Enabled bool pointer
			if src.ImageGeneration.MultiTurnEditing.Enabled != nil {
				val := *src.ImageGeneration.MultiTurnEditing.Enabled
				dst.ImageGeneration.MultiTurnEditing.Enabled = &val
			}
		}
	}

	// Deep copy SubAgents map (recursive)
	if src.SubAgents != nil {
		dst.SubAgents = make(map[string]agent.AgentConfig)
		for name, subAgent := range src.SubAgents {
			// Recursive deep copy
			if copied := deepCopyAgentConfig(&subAgent); copied != nil {
				// Expand environment variables in sub-agent config
				expanded := agent.ExpandAgentConfig(*copied)
				dst.SubAgents[name] = expanded
			}
		}
	}

	// Deep copy ConfigSource
	if src.ConfigSource != nil {
		dst.ConfigSource = &agent.ConfigSourceMetadata{
			Type:        src.ConfigSource.Type,
			Source:      src.ConfigSource.Source,
			AgentID:     src.ConfigSource.AgentID,
			AgentName:   src.ConfigSource.AgentName,
			Environment: src.ConfigSource.Environment,
			LoadedAt:    src.ConfigSource.LoadedAt,
			Variables:   deepCopyStringMap(src.ConfigSource.Variables),
		}
	}

	return dst
}

// deepCopyStringSlice creates a deep copy of a string slice
func deepCopyStringSlice(src []string) []string {
	if src == nil {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

// deepCopyStringMap creates a deep copy of a map[string]string
func deepCopyStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// deepCopyMap creates a deep copy of a map[string]interface{}
func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = deepCopyValue(v)
	}
	return dst
}

// deepCopyValue creates a deep copy of an interface{} value
func deepCopyValue(src interface{}) interface{} {
	if src == nil {
		return nil
	}

	switch v := src.(type) {
	case map[string]interface{}:
		return deepCopyMap(v)
	case []interface{}:
		dst := make([]interface{}, len(v))
		for i, item := range v {
			dst[i] = deepCopyValue(item)
		}
		return dst
	case []string:
		return deepCopyStringSlice(v)
	case map[string]string:
		return deepCopyStringMap(v)
	default:
		// Primitive types (string, int, bool, float64, etc.) are safe to copy by value
		return v
	}
}

// debugPrintConfig prints the agent config as YAML for debugging
func debugPrintConfig(config *agent.AgentConfig, label string) {
	if config == nil {
		fmt.Printf("\n=== DEBUG: %s ===\nnil\n", label)
		return
	}

	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		fmt.Printf("\n=== DEBUG: %s (YAML marshal error: %v) ===\n", label, err)
		return
	}

	fmt.Printf("\n=== DEBUG: %s ===\n%s\n", label, string(yamlBytes))
}

// MergeAgentConfig merges two AgentConfig structs based on the specified strategy
// For RemotePriority: primary values override base values (remote overrides local)
// For LocalPriority: base values override primary values (local overrides remote)
func MergeAgentConfig(primary, base *agent.AgentConfig, strategy MergeStrategy) *agent.AgentConfig {
	// Debug: Print input configs
	if os.Getenv("DEBUG_CONFIG_MERGE") == "true" {
		debugPrintConfig(primary, "MERGE INPUT - Primary")
		debugPrintConfig(base, "MERGE INPUT - Base")
	}

	if primary == nil {
		result := deepCopyAgentConfig(base)
		if os.Getenv("DEBUG_CONFIG_MERGE") == "true" {
			debugPrintConfig(result, "MERGE OUTPUT (primary nil, returned base)")
		}
		return result
	}
	if base == nil {
		result := deepCopyAgentConfig(primary)
		if os.Getenv("DEBUG_CONFIG_MERGE") == "true" {
			debugPrintConfig(result, "MERGE OUTPUT (base nil, returned primary)")
		}
		return result
	}

	// Create result starting with deep copy of primary to prevent mutation
	result := deepCopyAgentConfig(primary)

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

	// Merge pointer fields (use deep copied base if primary is nil)
	if result.MaxIterations == nil && base.MaxIterations != nil {
		val := *base.MaxIterations
		result.MaxIterations = &val
	}
	if result.RequirePlanApproval == nil && base.RequirePlanApproval != nil {
		val := *base.RequirePlanApproval
		result.RequirePlanApproval = &val
	}

	// Merge ResponseFormat (deep copy from base if needed)
	if result.ResponseFormat == nil && base.ResponseFormat != nil {
		result.ResponseFormat = &agent.ResponseFormatConfig{
			Type:             base.ResponseFormat.Type,
			SchemaName:       base.ResponseFormat.SchemaName,
			SchemaDefinition: deepCopyMap(base.ResponseFormat.SchemaDefinition),
		}
	}

	// Merge MCP (deep copy from base if needed)
	if result.MCP == nil && base.MCP != nil {
		result.MCP = &agent.MCPConfiguration{
			Global: base.MCP.Global,
		}
		if base.MCP.MCPServers != nil {
			result.MCP.MCPServers = make(map[string]agent.MCPServerConfig)
			for k, v := range base.MCP.MCPServers {
				result.MCP.MCPServers[k] = agent.MCPServerConfig{
					Command: v.Command,
					Args:    deepCopyStringSlice(v.Args),
					Env:     deepCopyStringMap(v.Env),
					URL:     v.URL,
					Token:   v.Token,
				}
			}
		}
	}

	// Merge StreamConfig (deep copy from base if needed)
	if result.StreamConfig == nil && base.StreamConfig != nil {
		result.StreamConfig = &agent.StreamConfigYAML{}
		if base.StreamConfig.BufferSize != nil {
			val := *base.StreamConfig.BufferSize
			result.StreamConfig.BufferSize = &val
		}
		if base.StreamConfig.IncludeToolProgress != nil {
			val := *base.StreamConfig.IncludeToolProgress
			result.StreamConfig.IncludeToolProgress = &val
		}
		if base.StreamConfig.IncludeIntermediateMessages != nil {
			val := *base.StreamConfig.IncludeIntermediateMessages
			result.StreamConfig.IncludeIntermediateMessages = &val
		}
	}

	// Merge LLMConfig (deep copy from base if needed)
	if result.LLMConfig == nil && base.LLMConfig != nil {
		result.LLMConfig = &agent.LLMConfigYAML{}
		if base.LLMConfig.Temperature != nil {
			val := *base.LLMConfig.Temperature
			result.LLMConfig.Temperature = &val
		}
		if base.LLMConfig.TopP != nil {
			val := *base.LLMConfig.TopP
			result.LLMConfig.TopP = &val
		}
		if base.LLMConfig.FrequencyPenalty != nil {
			val := *base.LLMConfig.FrequencyPenalty
			result.LLMConfig.FrequencyPenalty = &val
		}
		if base.LLMConfig.PresencePenalty != nil {
			val := *base.LLMConfig.PresencePenalty
			result.LLMConfig.PresencePenalty = &val
		}
		if base.LLMConfig.EnableReasoning != nil {
			val := *base.LLMConfig.EnableReasoning
			result.LLMConfig.EnableReasoning = &val
		}
		if base.LLMConfig.ReasoningBudget != nil {
			val := *base.LLMConfig.ReasoningBudget
			result.LLMConfig.ReasoningBudget = &val
		}
		if base.LLMConfig.Reasoning != nil {
			val := *base.LLMConfig.Reasoning
			result.LLMConfig.Reasoning = &val
		}
		result.LLMConfig.StopSequences = deepCopyStringSlice(base.LLMConfig.StopSequences)
	}

	// Merge LLMProvider (deep copy from base if needed)
	if result.LLMProvider == nil && base.LLMProvider != nil {
		result.LLMProvider = &agent.LLMProviderYAML{
			Provider: base.LLMProvider.Provider,
			Model:    base.LLMProvider.Model,
			Config:   deepCopyMap(base.LLMProvider.Config),
		}
	} else if result.LLMProvider != nil && base.LLMProvider != nil {
		// Deep merge LLMProvider fields
		merged := *result.LLMProvider
		merged.Provider = mergeString(result.LLMProvider.Provider, base.LLMProvider.Provider)
		merged.Model = mergeString(result.LLMProvider.Model, base.LLMProvider.Model)
		if merged.Config == nil && base.LLMProvider.Config != nil {
			merged.Config = deepCopyMap(base.LLMProvider.Config)
		}
		result.LLMProvider = &merged
	}

	// Merge Tools - use primary tools, append deep copied missing tools from base
	if result.Tools == nil && base.Tools != nil {
		// Deep copy base tools
		result.Tools = make([]agent.ToolConfigYAML, len(base.Tools))
		for i, tool := range base.Tools {
			result.Tools[i] = agent.ToolConfigYAML{
				Type:        tool.Type,
				Name:        tool.Name,
				Description: tool.Description,
				Config:      deepCopyMap(tool.Config),
				URL:         tool.URL,
				Timeout:     tool.Timeout,
			}
			if tool.Enabled != nil {
				val := *tool.Enabled
				result.Tools[i].Enabled = &val
			}
		}
	} else if result.Tools != nil && base.Tools != nil {
		// Create a map of existing tool names from primary
		existingTools := make(map[string]bool)
		for _, tool := range result.Tools {
			existingTools[tool.Name] = true
		}
		// Add deep copied base tools that don't exist in primary
		for _, baseTool := range base.Tools {
			if !existingTools[baseTool.Name] {
				newTool := agent.ToolConfigYAML{
					Type:        baseTool.Type,
					Name:        baseTool.Name,
					Description: baseTool.Description,
					Config:      deepCopyMap(baseTool.Config),
					URL:         baseTool.URL,
					Timeout:     baseTool.Timeout,
				}
				if baseTool.Enabled != nil {
					val := *baseTool.Enabled
					newTool.Enabled = &val
				}
				result.Tools = append(result.Tools, newTool)
			}
		}
	}

	// Merge Memory (deep copy from base if needed)
	if result.Memory == nil && base.Memory != nil {
		result.Memory = &agent.MemoryConfigYAML{
			Type:   base.Memory.Type,
			Config: deepCopyMap(base.Memory.Config),
		}
	}

	// Merge Runtime (deep copy from base if needed)
	if result.Runtime == nil && base.Runtime != nil {
		result.Runtime = &agent.RuntimeConfigYAML{
			LogLevel:        base.Runtime.LogLevel,
			TimeoutDuration: base.Runtime.TimeoutDuration,
		}
		if base.Runtime.EnableTracing != nil {
			val := *base.Runtime.EnableTracing
			result.Runtime.EnableTracing = &val
		}
		if base.Runtime.EnableMetrics != nil {
			val := *base.Runtime.EnableMetrics
			result.Runtime.EnableMetrics = &val
		}
	} else if result.Runtime != nil && base.Runtime != nil {
		// Deep merge Runtime fields
		merged := *result.Runtime
		merged.LogLevel = mergeString(result.Runtime.LogLevel, base.Runtime.LogLevel)
		merged.TimeoutDuration = mergeString(result.Runtime.TimeoutDuration, base.Runtime.TimeoutDuration)
		result.Runtime = &merged
	}

	// Merge ImageGeneration (deep copy from base if needed)
	if result.ImageGeneration == nil && base.ImageGeneration != nil {
		result.ImageGeneration = &agent.ImageGenerationYAML{
			Enabled:  base.ImageGeneration.Enabled,
			Provider: base.ImageGeneration.Provider,
			Model:    base.ImageGeneration.Model,
			Config:   deepCopyMap(base.ImageGeneration.Config),
		}
		if base.ImageGeneration.Storage != nil {
			result.ImageGeneration.Storage = &agent.ImageStorageYAML{
				Type: base.ImageGeneration.Storage.Type,
			}
			if base.ImageGeneration.Storage.Local != nil {
				result.ImageGeneration.Storage.Local = &agent.LocalStorageYAML{
					Path:    base.ImageGeneration.Storage.Local.Path,
					BaseURL: base.ImageGeneration.Storage.Local.BaseURL,
				}
			}
			if base.ImageGeneration.Storage.GCS != nil {
				result.ImageGeneration.Storage.GCS = &agent.GCSStorageYAML{
					Bucket:              base.ImageGeneration.Storage.GCS.Bucket,
					Prefix:              base.ImageGeneration.Storage.GCS.Prefix,
					CredentialsFile:     base.ImageGeneration.Storage.GCS.CredentialsFile,
					CredentialsJSON:     base.ImageGeneration.Storage.GCS.CredentialsJSON,
					SignedURLExpiration: base.ImageGeneration.Storage.GCS.SignedURLExpiration,
				}
			}
		}
		if base.ImageGeneration.MultiTurnEditing != nil {
			result.ImageGeneration.MultiTurnEditing = &agent.MultiTurnEditingYAML{
				Enabled:           base.ImageGeneration.MultiTurnEditing.Enabled,
				Model:             base.ImageGeneration.MultiTurnEditing.Model,
				SessionTimeout:    base.ImageGeneration.MultiTurnEditing.SessionTimeout,
				MaxSessionsPerOrg: base.ImageGeneration.MultiTurnEditing.MaxSessionsPerOrg,
			}
		}
	}

	// Merge SubAgents recursively (deep copy from base if needed)
	if result.SubAgents == nil && base.SubAgents != nil {
		// Deep copy all base sub-agents
		result.SubAgents = make(map[string]agent.AgentConfig)
		for name, subAgent := range base.SubAgents {
			if copied := deepCopyAgentConfig(&subAgent); copied != nil {
				result.SubAgents[name] = *copied
			}
		}
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

	// Merge ConfigSource metadata (deep copy to prevent shared state)
	if result.ConfigSource == nil && base.ConfigSource != nil {
		// Deep copy base ConfigSource if result has none
		result.ConfigSource = &agent.ConfigSourceMetadata{
			Type:        base.ConfigSource.Type,
			Source:      base.ConfigSource.Source,
			AgentID:     base.ConfigSource.AgentID,
			AgentName:   base.ConfigSource.AgentName,
			Environment: base.ConfigSource.Environment,
			LoadedAt:    base.ConfigSource.LoadedAt,
			Variables:   deepCopyStringMap(base.ConfigSource.Variables),
		}
	} else if result.ConfigSource != nil && base.ConfigSource != nil {
		result.ConfigSource.Type = string(ConfigSourceMerged)
		// Combine sources
		result.ConfigSource.Source = fmt.Sprintf("merged(%s + %s)",
			result.ConfigSource.Source, base.ConfigSource.Source)
		// Merge variables maps (deep copy)
		if result.ConfigSource.Variables == nil && base.ConfigSource.Variables != nil {
			result.ConfigSource.Variables = deepCopyStringMap(base.ConfigSource.Variables)
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

	// Debug: Print final merged config
	if os.Getenv("DEBUG_CONFIG_MERGE") == "true" {
		debugPrintConfig(result, "MERGE OUTPUT (final merged config)")
	}

	return result
}
