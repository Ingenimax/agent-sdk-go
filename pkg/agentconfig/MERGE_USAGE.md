# Agent Config Merge Feature

This document explains how to use the config merging feature to combine remote (config server) and local YAML configurations.

## Overview

The merge feature allows you to:
- **Have config server as the source of truth** (recommended: `MergeStrategyRemotePriority`)
- **Use local config as defaults** for fields not defined in the config server
- **Combine tools, sub-agents, and configurations** from both sources

## Use Cases

### Use Case 1: Config Server with Local Fallbacks (Recommended)

**Scenario:** Your production config is in the config server, but you want local defaults for development or fields not yet configured remotely.

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/agentconfig"
)

// Load with remote priority - config server wins, local fills gaps
config, err := agentconfig.LoadAgentConfig(
    context.Background(),
    "research-agent",
    "production",
    agentconfig.WithRemotePriorityMerge(), // Enable merge with remote priority
    agentconfig.WithVerbose(),             // See what's being merged
)
```

**Example Merge:**

Remote config (from config server):
```yaml
research-agent:
  role: "Senior Research Analyst"
  goal: "Conduct comprehensive research"
  llm_provider:
    provider: "anthropic"
```

Local config (`./configs/research-agent.yaml`):
```yaml
research-agent:
  role: "Junior Researcher"  # Will be overridden
  goal: "Basic research"      # Will be overridden
  backstory: "Expert in data analysis"  # Will be kept (not in remote)
  llm_provider:
    provider: "openai"  # Will be overridden
    model: "gpt-4"      # Will be kept (not in remote)
  tools:
    - name: "calculator"
      type: "math"
  memory:
    type: "redis"
    config:
      address: "${REDIS_ADDRESS}"
```

**Result:**
```yaml
research-agent:
  role: "Senior Research Analyst"         # From remote
  goal: "Conduct comprehensive research"  # From remote
  backstory: "Expert in data analysis"    # From local (gap fill)
  llm_provider:
    provider: "anthropic"  # From remote
    model: "gpt-4"         # From local (gap fill)
  tools:
    - name: "calculator"   # From local (not in remote)
      type: "math"
  memory:                  # From local (not in remote)
    type: "redis"
    config:
      address: "${REDIS_ADDRESS}"
```

### Use Case 2: Local Development with Remote Defaults

**Scenario:** You want to test changes locally but fall back to remote config for unmodified fields.

```go
config, err := agentconfig.LoadAgentConfig(
    context.Background(),
    "dev-agent",
    "development",
    agentconfig.WithLocalPriorityMerge(), // Local wins, remote fills gaps
)
```

## Configuration Options

### Available Strategies

```go
// No merging (default - backwards compatible)
// Uses remote if available, falls back to local on error
agentconfig.LoadAgentConfig(ctx, name, env)

// Remote priority merge
// Config server values override local, local fills gaps
agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
)

// Local priority merge
// Local values override remote, remote fills gaps
agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithLocalPriorityMerge(),
)

// Custom strategy
agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithMergeStrategy(agentconfig.MergeStrategyRemotePriority),
)
```

### Combining Options

```go
config, err := agentconfig.LoadAgentConfig(
    ctx,
    "my-agent",
    "production",
    agentconfig.WithRemotePriorityMerge(),  // Enable merging
    agentconfig.WithLocalFallback("./custom-config.yaml"), // Specific local file
    agentconfig.WithCache(10 * time.Minute), // Cache merged result
    agentconfig.WithEnvOverrides(),          // Expand ${ENV_VARS}
    agentconfig.WithVerbose(),               // Log merge details
)
```

## Merge Behavior Details

### String Fields
- **Non-empty values** from primary config (remote or local depending on strategy) are used
- **Empty values** are filled from the base config

### Pointer Fields
- If primary has `nil`, base value is used
- If primary has value, it's used regardless of base

### Complex Objects

#### Tools
- All tools from primary config are kept
- Tools from base config are **appended** if they don't exist in primary (matched by `name`)

#### Sub-Agents
- Recursively merged using the same strategy
- Sub-agents from both configs are combined
- Matching sub-agents (by name) are merged deeply

#### LLM Provider
- Provider and model are merged as strings (primary priority)
- Config maps use primary if present, otherwise base

#### Memory & Runtime
- If primary has the object, it's used (with deep merge of fields)
- If primary doesn't have it, base object is used entirely

### Configuration Source Metadata

When configs are merged, the `ConfigSource` metadata is updated:

```go
config.ConfigSource.Type // "merged"
config.ConfigSource.Source // "merged(remote-url + /local/path.yaml)"
config.ConfigSource.Variables // Combined from both sources
```

## Error Handling

### When Merging is Enabled

```go
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
)
```

**Behavior:**
- If **both** configs fail to load → returns error
- If **remote only** loads → uses remote exclusively
- If **local only** loads → uses local exclusively
- If **both** load → merges them according to strategy

### When Merging is Disabled (Default)

```go
config, err := agentconfig.LoadAgentConfig(ctx, name, env)
```

**Behavior (backwards compatible):**
- Tries remote first (if `PreferRemote=true`, which is default)
- Falls back to local on remote error (if `AllowFallback=true`, which is default)
- Returns error only if both fail

## Environment Variable Expansion

Merge happens **before** environment variable expansion:

```go
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
    agentconfig.WithEnvOverrides(), // Expands ${VAR} after merge
)
```

**Flow:**
1. Load remote config
2. Load local config
3. **Merge** them
4. Expand environment variables in merged result

## Checking Config Source

```go
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
    agentconfig.WithVerbose(),
)

if config.ConfigSource != nil {
    fmt.Printf("Type: %s\n", config.ConfigSource.Type) // "remote", "local", or "merged"
    fmt.Printf("Source: %s\n", config.ConfigSource.Source)
    fmt.Printf("Variables: %v\n", config.ConfigSource.Variables)
}
```

## Best Practices

### For Production

```go
// Recommended: Config server is authoritative, local provides safe defaults
config, err := agentconfig.LoadAgentConfig(
    ctx,
    agentName,
    "production",
    agentconfig.WithRemotePriorityMerge(),
    agentconfig.WithCache(5 * time.Minute),
)
```

### For Development

```go
// Option 1: Test local changes with remote defaults
config, err := agentconfig.LoadAgentConfig(
    ctx,
    agentName,
    "development",
    agentconfig.WithLocalPriorityMerge(),
)

// Option 2: Pure local development (no remote needed)
config, err := agentconfig.LoadAgentConfig(
    ctx,
    agentName,
    "development",
    agentconfig.WithLocalOnly(),
)
```

### For Testing

```go
// Use specific local file, no remote
config, err := agentconfig.LoadAgentConfig(
    ctx,
    agentName,
    "test",
    agentconfig.WithLocalOnly(),
    agentconfig.WithLocalFallback("./testdata/test-config.yaml"),
)
```

## Migration from Non-Merge to Merge

If you're currently using the SDK without merging:

```go
// Before (no merging)
config, err := agentconfig.LoadAgentConfig(ctx, name, env)
// Uses remote OR local (fallback on error)
```

```go
// After (with merging)
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
)
// Uses remote AND local (merged)
```

**Backwards Compatibility:** The default behavior (no merge strategy specified) remains unchanged.

## Troubleshooting

### "Failed to load config for merging: remote error: X, local error: Y"

Both configs failed to load. Check:
- `AGENT_DEPLOYMENT_ID` environment variable is set
- Config server is accessible
- Local config file exists at expected path

### Merge not working as expected

Enable verbose logging to see the merge process:

```go
config, err := agentconfig.LoadAgentConfig(ctx, name, env,
    agentconfig.WithRemotePriorityMerge(),
    agentconfig.WithVerbose(), // Shows: "Merge strategy enabled: remote_priority"
)
```

### Want to see which config won

Check the `ConfigSource` metadata:

```go
if config.ConfigSource.Type == "merged" {
    fmt.Println("Configs were merged!")
    fmt.Println("Sources:", config.ConfigSource.Source)
}
```
