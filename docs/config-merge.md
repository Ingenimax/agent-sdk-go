# Agent Configuration Merge

This guide covers the configuration merge feature in agent-sdk-go, which enables intelligent combination of remote (starops-config-service) and local YAML configurations with configurable priority strategies.

## Overview

The configuration merge feature allows you to:

- **Combine remote and local configurations** - Merge both sources intelligently
- **Flexible priority strategies** - Choose which source takes precedence
- **Gap filling** - Use one source to provide defaults for fields missing in the other
- **Deep merging** - Recursive merge of complex objects and sub-agents
- **Tool consolidation** - Combine tools from both sources without duplicates
- **Production-ready** - Fully tested with comprehensive error handling

### Key Benefit

Instead of choosing between remote OR local configuration, you can now use both:
- **Remote as source of truth** with local providing development defaults
- **Local overrides** for testing with remote providing stable baseline
- **Combined toolsets** from both configurations

## Quick Start

### Recommended: Remote Priority Merge

Use this approach for production environments where the config server is authoritative, but local files provide safe defaults:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/agentconfig"
)

func main() {
    ctx := context.Background()

    // Remote config wins, local fills gaps
    config, err := agentconfig.LoadAgentConfig(
        ctx,
        "research-assistant",
        "production",
        agentconfig.WithRemotePriorityMerge(),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Use the config
    fmt.Printf("Loaded merged config from: %s\n", config.ConfigSource.Source)
}
```

### Local Priority Merge for Development

Use this approach when testing local changes while falling back to remote defaults:

```go
// Local config wins, remote fills gaps
config, err := agentconfig.LoadAgentConfig(
    ctx,
    "research-assistant",
    "development",
    agentconfig.WithLocalPriorityMerge(),
)
```

## Use Cases

### Use Case 1: Centralized Config with Local Defaults

**Scenario**: Your team manages production configs centrally, but developers need local defaults for fields not yet configured.

**Remote Config (config server)**:
```yaml
research-assistant:
  role: "Senior Research Analyst"
  goal: "Conduct comprehensive market research"
  llm_provider:
    provider: "anthropic"
    config:
      api_key: "${ANTHROPIC_API_KEY}"
```

**Local Config** (`./configs/research-assistant.yaml`):
```yaml
research-assistant:
  role: "Junior Researcher"  # Will be overridden by remote
  backstory: "Expert in data analysis with 10 years experience"  # Will be kept
  llm_provider:
    provider: "openai"  # Will be overridden
    model: "gpt-4"      # Will be kept (not in remote)
  tools:
    - name: "calculator"
      type: "built-in"
    - name: "web-search"
      type: "custom"
  memory:
    type: "redis"
    config:
      address: "${REDIS_ADDRESS}"
```

**Merged Result** (with `WithRemotePriorityMerge()`):
```yaml
research-assistant:
  role: "Senior Research Analyst"         # From remote (override)
  goal: "Conduct comprehensive market research"  # From remote
  backstory: "Expert in data analysis with 10 years experience"  # From local (gap fill)
  llm_provider:
    provider: "anthropic"  # From remote (override)
    model: "gpt-4"         # From local (gap fill)
    config:
      api_key: "${ANTHROPIC_API_KEY}"
  tools:
    - name: "calculator"   # From local (combined)
      type: "built-in"
    - name: "web-search"   # From local (combined)
      type: "custom"
  memory:                  # From local (not in remote)
    type: "redis"
    config:
      address: "${REDIS_ADDRESS}"
```

### Use Case 2: Testing Local Changes

**Scenario**: You're developing a new feature locally but want production settings as fallback.

```go
config, err := agentconfig.LoadAgentConfig(
    ctx,
    "research-assistant",
    "development",
    agentconfig.WithLocalPriorityMerge(),  // Local changes take priority
    agentconfig.WithVerbose(),              // See merge details
)
```

### Use Case 3: Progressive Configuration Migration

**Scenario**: Migrating configs to config server incrementally while maintaining local defaults.

```go
// During migration, remote config is incomplete
// Local provides full configuration as fallback
config, err := agentconfig.LoadAgentConfig(
    ctx,
    "legacy-agent",
    "production",
    agentconfig.WithRemotePriorityMerge(),  // Use remote fields as they're added
    agentconfig.WithCache(10 * time.Minute),
)
```

## Merge Strategies

### Available Strategies

```go
// Strategy 1: No merging (default - backwards compatible)
// Uses remote if available, falls back to local on error
config, err := agentconfig.LoadAgentConfig(ctx, name, env)

// Strategy 2: Remote Priority Merge (Recommended for production)
// Remote values override local, local fills empty fields
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
)

// Strategy 3: Local Priority Merge (Useful for development)
// Local values override remote, remote fills empty fields
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithLocalPriorityMerge(),
)

// Strategy 4: Custom strategy (Advanced)
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithMergeStrategy(agentconfig.MergeStrategyRemotePriority),
)
```

### Strategy Comparison

| Strategy | Primary Source | Fallback Use | Best For |
|----------|---------------|--------------|----------|
| `None` (default) | Remote or local | On error only | Backward compatibility |
| `RemotePriority` | Remote config | Fills gaps | Production, centralized management |
| `LocalPriority` | Local config | Fills gaps | Development, local testing |

## Merge Behavior

### String Fields

All string fields (`role`, `goal`, `backstory`, etc.) follow this rule:

- **Primary non-empty** → use primary value
- **Primary empty** → use base value

**Example (Remote Priority)**:
```go
// Remote: role = "Senior Engineer"
// Local:  role = "Junior Engineer"
// Result: role = "Senior Engineer" (remote wins)

// Remote: role = ""
// Local:  role = "Developer"
// Result: role = "Developer" (local fills gap)
```

### Pointer Fields

Fields like `MaxIterations`, `RequirePlanApproval`:

- **Primary is nil** → use base value
- **Primary is non-nil** → use primary value (even if base is non-nil)

**Example**:
```go
// Remote: MaxIterations = nil
// Local:  MaxIterations = 5
// Result: MaxIterations = 5

// Remote: MaxIterations = 10
// Local:  MaxIterations = 5
// Result: MaxIterations = 10
```

### Tools Array

Tools are merged by name to avoid duplicates:

1. Keep all tools from primary config
2. Append tools from base config that don't exist in primary (matched by `name` field)

**Example (Remote Priority)**:
```yaml
# Remote tools
tools:
  - name: "web-search"
    type: "custom"

# Local tools
tools:
  - name: "web-search"  # Duplicate - ignored
    type: "custom"
  - name: "calculator"  # New - appended
    type: "built-in"

# Merged result
tools:
  - name: "web-search"  # From remote
    type: "custom"
  - name: "calculator"  # From local (appended)
    type: "built-in"
```

### Sub-Agents Map

Sub-agents are recursively merged using the same strategy:

1. Merge matching sub-agents (by name) recursively
2. Add sub-agents from base that don't exist in primary

**Example**:
```yaml
# Remote sub-agents
sub_agents:
  researcher:
    role: "Data Researcher"
    goal: "Find data"

# Local sub-agents
sub_agents:
  researcher:           # Matching - will be merged
    backstory: "Expert analyst"
  writer:               # New - will be added
    role: "Content Writer"

# Merged result
sub_agents:
  researcher:
    role: "Data Researcher"      # From remote
    goal: "Find data"            # From remote
    backstory: "Expert analyst"  # From local (merged)
  writer:                        # From local (added)
    role: "Content Writer"
```

### LLM Provider

Deep merge of provider configuration:

```yaml
# Remote LLM config
llm_provider:
  provider: "anthropic"
  config:
    temperature: 0.7

# Local LLM config
llm_provider:
  provider: "openai"  # Will be overridden
  model: "gpt-4"      # Will be kept
  config:
    temperature: 0.9  # Will be overridden
    max_tokens: 4000  # Will be kept

# Merged result (remote priority)
llm_provider:
  provider: "anthropic"  # From remote
  model: "gpt-4"         # From local
  config:
    temperature: 0.7     # From remote
    max_tokens: 4000     # From local
```

### Memory and Runtime

Complex objects are merged at the field level:

- If primary has the object, use it (with field-level deep merge)
- If primary doesn't have the object, use base object entirely

### Config Source Metadata

When configs are merged, metadata is updated:

```go
config.ConfigSource.Type     // "merged"
config.ConfigSource.Source   // "merged(https://config-server/api + /local/path.yaml)"
config.ConfigSource.Variables // Combined map from both sources
```

## Advanced Usage

### Combining Options

```go
config, err := agentconfig.LoadAgentConfig(
    ctx,
    "research-assistant",
    "production",
    agentconfig.WithRemotePriorityMerge(),           // Enable merging
    agentconfig.WithLocalFallback("./custom.yaml"),  // Custom local file
    agentconfig.WithCache(10 * time.Minute),         // Cache merged result
    agentconfig.WithEnvOverrides(),                  // Expand ${ENV_VARS}
    agentconfig.WithVerbose(),                       // Log merge details
)
```

### Checking Merge Results

```go
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
    agentconfig.WithVerbose(),
)

if config.ConfigSource != nil {
    fmt.Printf("Type: %s\n", config.ConfigSource.Type)
    // Output: Type: merged

    fmt.Printf("Source: %s\n", config.ConfigSource.Source)
    // Output: Source: merged(https://config.example.com/api + ./configs/agent.yaml)

    fmt.Printf("Variables: %v\n", config.ConfigSource.Variables)
    // Output: Variables: map[API_KEY:... REDIS_ADDR:...]
}
```

### Environment Variable Expansion

Merge happens **before** environment variable expansion:

```go
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),  // Step 1: Merge configs
    agentconfig.WithEnvOverrides(),          // Step 2: Expand ${VAR}
)
```

**Processing Order**:
1. Load remote config
2. Load local config
3. **Merge** remote and local according to strategy
4. Expand environment variables in merged result
5. Cache final config

## Error Handling

### Merge Error Behavior

When merge strategy is enabled:

```go
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
)
```

**Behavior**:
- **Both fail** → Returns error
- **Remote only succeeds** → Uses remote config exclusively
- **Local only succeeds** → Uses local config exclusively
- **Both succeed** → Merges according to strategy

### Fallback Without Merge (Default)

When merge is NOT enabled (default behavior):

```go
config, err := agentconfig.LoadAgentConfig(ctx, name, env)
```

**Behavior (backwards compatible)**:
- Try remote first
- On remote error, fallback to local
- Return error only if both fail
- No merging occurs

## Best Practices

### Production Environments

```go
// Recommended: Config server authoritative, local as safety net
config, err := agentconfig.LoadAgentConfig(
    ctx,
    agentName,
    "production",
    agentconfig.WithRemotePriorityMerge(),
    agentconfig.WithCache(5 * time.Minute),
    agentconfig.WithEnvOverrides(),
)
```

**Why?**
- Centralized config management
- Local files provide safe defaults if remote is unreachable
- Consistent configuration across deployments
- Easy rollback via config server

### Development Environments

```go
// Option 1: Test local changes with remote defaults
config, err := agentconfig.LoadAgentConfig(
    ctx,
    agentName,
    "development",
    agentconfig.WithLocalPriorityMerge(),  // Local overrides remote
)

// Option 2: Pure local development
config, err := agentconfig.LoadAgentConfig(
    ctx,
    agentName,
    "development",
    agentconfig.WithLocalOnly(),  // No remote calls
)
```

### Staging Environments

```go
// Balance between production and development
config, err := agentconfig.LoadAgentConfig(
    ctx,
    agentName,
    "staging",
    agentconfig.WithRemotePriorityMerge(),  // Like production
    agentconfig.WithVerbose(),               // But with debug info
)
```

### Configuration Migration

When migrating from local-only to config server:

```go
// Phase 1: Start with remote priority merge
// Gradually move fields to remote config
config, err := agentconfig.LoadAgentConfig(
    ctx,
    agentName,
    env,
    agentconfig.WithRemotePriorityMerge(),
)

// Phase 2: Once fully migrated, switch to remote-only
config, err := agentconfig.LoadAgentConfig(
    ctx,
    agentName,
    env,
    agentconfig.WithRemoteOnly(),  // No local fallback
)
```

## Migration Guide

### From Non-Merge to Merge

#### Before (No Merge)
```go
// Uses remote OR local (fallback on error)
config, err := agentconfig.LoadAgentConfig(ctx, name, env)
```

#### After (With Merge)
```go
// Uses remote AND local (merged intelligently)
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
)
```

### Backwards Compatibility

The default behavior (no merge strategy specified) remains **completely unchanged**:

```go
// This code continues to work exactly as before
config, err := agentconfig.LoadAgentConfig(ctx, name, env)
```

The merge feature is **opt-in** - you must explicitly enable it.

## Implementation Details

### Types and Constants

```go
// Merge strategy enumeration
type MergeStrategy string

const (
    MergeStrategyNone           MergeStrategy = "none"
    MergeStrategyRemotePriority MergeStrategy = "remote_priority"
    MergeStrategyLocalPriority  MergeStrategy = "local_priority"
)

// Config source type for merged configs
const (
    ConfigSourceMerged ConfigSource = "merged"
)
```

### Load Options

```go
type LoadOptions struct {
    PreferRemote       bool
    AllowFallback      bool
    LocalPath          string
    EnableCache        bool
    CacheTimeout       time.Duration
    EnableEnvOverrides bool
    Verbose            bool
    MergeStrategy      MergeStrategy  // New field
}
```

### Core Function

```go
// MergeAgentConfig merges two configurations with specified strategy
func MergeAgentConfig(primary, base *AgentConfig, strategy MergeStrategy) *AgentConfig
```

**Parameters**:
- `primary`: The configuration with higher priority
- `base`: The configuration used to fill gaps
- `strategy`: Determines merge behavior

**Returns**: Merged AgentConfig with updated ConfigSource metadata

### Processing Flow

```
1. LoadAgentConfig() called with merge strategy
2. Attempt to load remote config
3. Attempt to load local config
4. If merge strategy enabled:
   a. Check if both configs available
   b. Determine primary/base based on strategy
   c. Call MergeAgentConfig(primary, base, strategy)
   d. Update ConfigSource to "merged"
5. Apply environment variable expansion
6. Cache result
7. Return merged config
```

## Performance Considerations

### Caching Merged Configs

Merged configs are cached the same as regular configs:

```go
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
    agentconfig.WithCache(10 * time.Minute),  // Cache merged result
)
```

**Cache key format**: `merged:agentName:environment`

### Memory Usage

Merge operation is in-memory only:
- No additional remote API calls during merge
- Merged config size ≈ sum of both configs
- Caching reduces repeated merge operations

### Load Time

| Scenario | Load Time | Notes |
|----------|-----------|-------|
| Cache hit | ~1ms | Fastest |
| Remote only | ~50-200ms | Network latency |
| Local only | ~5-10ms | File I/O |
| Merge (both) | ~55-210ms | Load both + merge (~1ms) |

## Testing

### Test Coverage

The merge feature includes comprehensive tests:

- ✅ Remote priority merge with various field types
- ✅ Local priority merge
- ✅ Nil handling (nil remote, nil local, both nil)
- ✅ ConfigSource metadata merging
- ✅ Tool deduplication and appending
- ✅ Deep merge of LLMProvider
- ✅ Recursive merge of SubAgents
- ✅ String and pointer field merging
- ✅ Complex object merging

### Example Test Cases

```go
// Test remote priority merge
func TestMergeRemotePriority(t *testing.T) {
    remote := &AgentConfig{
        Role: "Senior Engineer",
        Goal: "Build systems",
    }

    local := &AgentConfig{
        Role: "Junior Engineer",
        Backstory: "Expert developer",
    }

    merged := MergeAgentConfig(remote, local, MergeStrategyRemotePriority)

    assert.Equal(t, "Senior Engineer", merged.Role)      // From remote
    assert.Equal(t, "Build systems", merged.Goal)        // From remote
    assert.Equal(t, "Expert developer", merged.Backstory) // From local
}
```

## Troubleshooting

### Issue: Configs not merging

**Symptom**: Only seeing remote or local config, not merged.

**Solution**:
```go
// Ensure merge strategy is enabled
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),  // Don't forget this!
    agentconfig.WithVerbose(),               // Check logs
)
```

### Issue: Wrong values in merged config

**Symptom**: Unexpected values from wrong source.

**Debugging**:
```go
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
    agentconfig.WithVerbose(),
)

// Check which source was used
fmt.Printf("Config type: %s\n", config.ConfigSource.Type)
fmt.Printf("Config source: %s\n", config.ConfigSource.Source)
```

**Check**:
- Verify merge strategy (remote vs local priority)
- Check if fields are actually empty in primary config
- Enable verbose logging to see merge decisions

### Issue: Both configs fail to load

**Symptom**: Error: "failed to load config for merging"

**Solution**:
```go
// When merge is enabled, at least one source must succeed
// Check:
// 1. Remote service is accessible (STAROPS_CONFIG_SERVICE_HOST)
// 2. Auth token is valid (STAROPS_AUTH_TOKEN)
// 3. Local config file exists at expected path
```

### Issue: Tools duplicated

**Symptom**: Same tool appearing twice in merged config.

**Solution**:
- Tools are deduplicated by `name` field
- Ensure tools have unique names
- Check that tool names match exactly (case-sensitive)

## Security Considerations

### Secret Handling

Secrets are handled safely during merge:

```go
// Remote config (secrets from config server)
llm_provider:
  config:
    api_key: "${ANTHROPIC_API_KEY}"  # Resolved by remote

// Local config (development overrides)
llm_provider:
  model: "claude-3-sonnet"

// Merged config
llm_provider:
  model: "claude-3-sonnet"           # From local
  config:
    api_key: "${ANTHROPIC_API_KEY}"  # From remote (secure)
```

**Best Practices**:
- Keep secrets in remote config (managed centrally)
- Don't commit secrets to local YAML files
- Use environment variables for local development
- Remote config server provides audit trail

### Configuration Validation

Validate merged configs before use:

```go
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
)
if err != nil {
    log.Fatal(err)
}

// Validate required fields are present
if config.Role == "" {
    log.Fatal("Missing required field: role")
}
if config.Goal == "" {
    log.Fatal("Missing required field: goal")
}
```

## API Reference

### Option Functions

```go
// Merge-specific options
func WithRemotePriorityMerge() LoadOption
func WithLocalPriorityMerge() LoadOption
func WithMergeStrategy(strategy MergeStrategy) LoadOption

// General options (work with merge)
func WithLocalFallback(path string) LoadOption
func WithCache(timeout time.Duration) LoadOption
func WithEnvOverrides() LoadOption
func WithVerbose() LoadOption
```

### Merge Function

```go
// MergeAgentConfig performs intelligent merge of two configs
func MergeAgentConfig(primary, base *AgentConfig, strategy MergeStrategy) *AgentConfig
```

### Config Source Check

```go
// Check if config was merged
if config.ConfigSource.Type == "merged" {
    fmt.Println("Config was merged from multiple sources")
    fmt.Printf("Sources: %s\n", config.ConfigSource.Source)
}
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/Ingenimax/agent-sdk-go/pkg/agentconfig"
)

func main() {
    ctx := context.Background()

    // Example 1: Production - remote priority merge
    prodConfig, err := agentconfig.LoadAgentConfig(
        ctx,
        "research-assistant",
        "production",
        agentconfig.WithRemotePriorityMerge(),
        agentconfig.WithCache(5 * time.Minute),
        agentconfig.WithEnvOverrides(),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Production config loaded from: %s\n",
        prodConfig.ConfigSource.Source)

    // Example 2: Development - local priority merge
    devConfig, err := agentconfig.LoadAgentConfig(
        ctx,
        "research-assistant",
        "development",
        agentconfig.WithLocalPriorityMerge(),
        agentconfig.WithVerbose(),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Example 3: Check merge results
    if devConfig.ConfigSource.Type == "merged" {
        fmt.Println("Config was merged successfully!")
        fmt.Printf("  Primary source: development config\n")
        fmt.Printf("  Fallback source: remote config\n")
        fmt.Printf("  Combined sources: %s\n",
            devConfig.ConfigSource.Source)
    }

    // Example 4: Custom merge with all options
    customConfig, err := agentconfig.LoadAgentConfig(
        ctx,
        "custom-agent",
        "staging",
        agentconfig.WithMergeStrategy(agentconfig.MergeStrategyRemotePriority),
        agentconfig.WithLocalFallback("./configs/custom.yaml"),
        agentconfig.WithCache(10 * time.Minute),
        agentconfig.WithEnvOverrides(),
        agentconfig.WithVerbose(),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Inspect merged configuration
    fmt.Printf("\nCustom Agent Configuration:\n")
    fmt.Printf("  Role: %s\n", customConfig.Role)
    fmt.Printf("  Goal: %s\n", customConfig.Goal)
    fmt.Printf("  Tools: %d\n", len(customConfig.Tools))
    fmt.Printf("  Sub-agents: %d\n", len(customConfig.SubAgents))
    if customConfig.ConfigSource != nil {
        fmt.Printf("  Source: %s\n", customConfig.ConfigSource.Type)
        fmt.Printf("  Loaded at: %v\n", customConfig.ConfigSource.LoadedAt)
    }
}
```

## Summary

The configuration merge feature provides intelligent combination of remote and local agent configurations:

- **Flexible strategies** - Choose remote or local priority based on your needs
- **Deep merging** - Recursively merges all configuration elements
- **Gap filling** - Uses one source to complete fields missing in the other
- **Tool consolidation** - Combines tools without duplicates
- **Production-ready** - Fully tested with comprehensive error handling
- **Backwards compatible** - Opt-in feature, existing code unchanged

Use `WithRemotePriorityMerge()` for production environments where the config server is authoritative, and `WithLocalPriorityMerge()` for development when testing local changes.

For more information, see:
- [Unified Configuration Loader](unified-config-loader.md) - Main configuration system
- [Environment Variables](environment_variables.md) - Variable handling
- [Agent Configuration](agent.md) - Core agent concepts
