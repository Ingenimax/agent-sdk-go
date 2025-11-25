# Unified Agent Configuration Loader

This guide covers the unified agent configuration system in agent-sdk-go, which enables loading agent configurations from both remote (starops-config-service) and local YAML files with automatic fallback.

## Overview

The unified configuration loader provides a centralized approach to agent configuration management with these key features:

- **Dual Configuration Sources**: Load from remote starops-config-service or local YAML files
- **Automatic Source Detection**: Try remote first, fall back to local if unavailable
- **In-Memory Caching**: Configurable TTL-based caching for improved performance
- **Environment Variable Resolution**: Automatic variable substitution with priority handling
- **Backward Compatible**: Existing file-based configurations continue to work unchanged
- **Comprehensive API**: Flexible options for all use cases

## Quick Start

### Basic Usage (Recommended)

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/agentconfig"
)

func main() {
    ctx := context.Background()

    // Load agent with automatic source detection
    // Tries remote config first, falls back to local files
    agent, err := agentconfig.LoadAgentAuto(ctx, "research-assistant", "production")
    if err != nil {
        log.Fatal(err)
    }

    // Use the agent
    response, err := agent.Run(ctx, "Research latest developments in AI")
    fmt.Println(response)
}
```

### Explicit Source Control

```go
// Force remote configuration only
agent, err := agentconfig.LoadAgentFromRemote(ctx, "research-assistant", "production")

// Force local configuration only
agent, err := agentconfig.LoadAgentFromLocal(ctx, "research-assistant", "production")
```

### Configuration Preview

Preview configuration without creating an agent:

```go
// Preview config without creating agent
config, err := agentconfig.PreviewAgentConfig(ctx, "research-assistant", "production")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Config loaded from: %s\n", config.ConfigSource.Type)
fmt.Printf("Agent role: %s\n", config.Role)
fmt.Printf("Agent goal: %s\n", config.Goal)
```

## Architecture

### Configuration Loading Flow

```
1. Call LoadAgentConfig(name, env, options)
2. Check cache (if enabled)
3. Try remote configuration:
   - Call starops-config-service API
   - Get resolved YAML with variables substituted
   - Parse into AgentConfig struct
4. On failure, try local fallback:
   - Search common file paths
   - Load from YAML file
   - Apply environment variable expansion
5. Apply environment overrides
6. Cache result (if enabled)
7. Return AgentConfig with source metadata
```

### Components

#### 1. Enhanced ConfigurationClient
Located in `pkg/agentconfig/client.go`:
- **Method**: `FetchAgentConfig(agentName, environment)` - fetches resolved YAML configs
- **Response Types**: `AgentConfigResponse` and `ResolvedAgentConfigResponse`
- **Integration**: Works with `/api/v1/configurations` endpoint

#### 2. ConfigSource Metadata
Added to `pkg/agent/config.go`:
- **Struct**: `ConfigSourceMetadata` - tracks where config was loaded from
- **Fields**: Type, source, agent name, environment, variables, load time
- **Backward Compatible**: Optional field in AgentConfig

#### 3. Unified Configuration Loader
Implemented in `pkg/agentconfig/unified_loader.go`:
- **Main Function**: `LoadAgentConfig()` - unified loading interface
- **Source Priority**: Remote first, then local fallback
- **Caching**: In-memory cache with configurable TTL
- **Options**: Flexible configuration with functional options pattern

#### 4. Cache System
Implemented in `pkg/agentconfig/cache.go`:
- **In-Memory Cache**: Simple map-based cache with TTL
- **Thread-Safe**: Uses RWMutex for concurrent access
- **Configurable**: TTL and cache control options

#### 5. Public API
Convenience functions in `pkg/agentconfig/api.go`:
- High-level API for common use cases
- Simple wrappers for typical scenarios
- Clear examples for different usage patterns

## Configuration Sources

### Remote Configuration (starops-config-service)

The system automatically tries to load from starops-config-service first. Set these environment variables:

```bash
export STAROPS_CONFIG_SERVICE_HOST="https://config.starops.ai"
export STAROPS_AUTH_TOKEN="your-auth-token"
```

The remote service provides:
- Centralized configuration management across environments
- Resolved YAML with variables substituted server-side
- Audit trail for configuration access
- Real-time configuration updates

### Local Configuration (YAML files)

If remote config is unavailable, the system falls back to local YAML files in these locations:
- `./configs/{agent-name}.yaml`
- `./configs/{agent-name}-{environment}.yaml`
- `./agents/{agent-name}.yaml`
- `./{agent-name}.yaml`

Local files provide:
- Offline development capability
- Quick prototyping without remote dependencies
- Version control integration
- Environment-specific overrides

## Environment Variables

The system supports environment variable substitution with the following priority (highest to lowest):

1. **OS Environment Variables** (highest priority)
2. **Remote Variables** (from starops-config-service)
3. **Local .env Files**
4. **Template Variables** (embedded in configuration)

### Example Configuration

```yaml
# In your YAML config
llm_provider:
  provider: "${LLM_PROVIDER:-anthropic}"
  model: "${ANTHROPIC_MODEL:-claude-sonnet-4}"
  config:
    api_key: "${ANTHROPIC_API_KEY}"

variables:
  topic: "${RESEARCH_TOPIC:-artificial intelligence}"
  search_depth: "${SEARCH_DEPTH:-comprehensive}"
```

## Advanced Usage

### Custom Options

```go
// Custom loading options
loadOptions := []agentconfig.LoadOption{
    agentconfig.WithLocalFallback("./configs/research.yaml"),
    agentconfig.WithCache(10 * time.Minute),
    agentconfig.WithEnvOverrides(),
    agentconfig.WithVerbose(),
}

// Agent creation options
agentOptions := []agent.Option{
    agent.WithMaxIterations(5),
    agent.WithRequirePlanApproval(false),
}

agent, err := agentconfig.LoadAgentWithOptions(ctx, "research-assistant", "staging",
    loadOptions, agentOptions...)
```

### Available Load Options

```go
// Source control
WithRemoteOnly()                    // Only try remote configuration
WithLocalOnly()                     // Only try local files
WithLocalFallback(path string)      // Specify custom fallback path

// Caching
WithCache(timeout time.Duration)    // Enable cache with custom TTL
WithoutCache()                      // Disable caching

// Behavior
WithEnvOverrides()                  // Enable environment variable overrides
WithVerbose()                       // Enable verbose logging
```

### Configuration Source Metadata

Access metadata about where the configuration was loaded from:

```go
config := agent.GetConfig()
if config.ConfigSource != nil {
    fmt.Printf("Loaded from: %s\n", config.ConfigSource.Type)          // "remote" or "local"
    fmt.Printf("Source: %s\n", config.ConfigSource.Source)             // URL or file path
    fmt.Printf("Agent: %s\n", config.ConfigSource.AgentName)           // Agent name
    fmt.Printf("Environment: %s\n", config.ConfigSource.Environment)   // Environment
    fmt.Printf("Variables: %v\n", config.ConfigSource.Variables)       // Resolved variables
    fmt.Printf("Loaded at: %v\n", config.ConfigSource.LoadedAt)        // Load timestamp
}
```

## Cache Management

The loader includes built-in cache management capabilities:

```go
// Get cache statistics
stats := agentconfig.GetCacheStats()
fmt.Printf("Cache entries: %d valid, %d expired\n", stats["valid"], stats["expired"])

// Clear entire cache
agentconfig.ClearCache()

// Clear specific entry
agentconfig.ClearCacheEntry("research-assistant:production")

// Clean up expired entries only
agentconfig.CleanupExpiredEntries()
```

### Cache Strategy

- **Default TTL**: 5 minutes for remote configurations
- **Cache Key Format**: `source:agentName:environment`
- **Thread-Safe**: Uses RWMutex for concurrent access
- **Process-Local**: Memory-based cache per process

## Integration with Variables

Pass custom variables for template substitution:

```go
// Pass custom variables for template substitution
variables := map[string]string{
    "topic": "artificial intelligence",
    "search_depth": "comprehensive",
}

agent, err := agentconfig.LoadAgentWithVariables(ctx, "research-assistant", "development", variables)
```

## Error Handling

### Graceful Error Handling

```go
agent, err := agentconfig.LoadAgentAuto(ctx, "research-assistant", "production")
if err != nil {
    if strings.Contains(err.Error(), "not found") {
        log.Printf("Agent configuration not found in any source")
        // Handle missing config
    } else if strings.Contains(err.Error(), "unauthorized") {
        log.Printf("Check your authentication credentials")
        // Handle auth error
    } else {
        log.Printf("Configuration error: %v", err)
        // Handle other errors
    }
    return
}
```

### Graceful Degradation

The system handles failures gracefully:
- **Remote service unavailable** → Falls back to local files
- **Missing local files** → Clear error message
- **Network timeouts** → Configurable retry with exponential backoff
- **Invalid YAML** → Validation errors with line numbers

### Logging Levels

- **INFO**: Source detection decisions
- **WARN**: Fallback operations
- **ERROR**: Configuration errors
- **DEBUG**: Variable resolution details

## Migration Guide

### Migrating from File-Based Configuration

#### Old Way
```go
configs, err := agent.LoadAgentConfigsFromFile("./agents.yaml")
if err != nil {
    log.Fatal(err)
}

agent, err := agent.NewAgentFromConfig("research-assistant", configs, nil)
```

#### New Way (Recommended)
```go
agent, err := agentconfig.LoadAgentAuto(ctx, "research-assistant", "production")
```

### Migration Path

1. **Phase 1**: Install new SDK version (backward compatible)
2. **Phase 2**: Optionally start using new `LoadAgentAuto()` factory
3. **Phase 3**: Migrate specific agents to remote configuration
4. **Phase 4**: Fully centralized (local files as fallback only)

### Backward Compatibility

All existing code continues to work unchanged:
- `LoadAgentConfigsFromFile()` calls work as before
- `NewAgentFromFile()` unchanged
- AgentConfig struct expanded but maintains all existing fields
- Environment variable expansion behavior preserved

## Best Practices

1. **Use `LoadAgentAuto()` for most cases** - Provides automatic fallback for reliability
2. **Set appropriate cache timeouts** - Default 5 minutes is usually good
3. **Use environment-specific configurations** - Separate dev/staging/prod
4. **Handle missing configurations gracefully** - Provide fallbacks or defaults
5. **Monitor cache performance** - Use `GetCacheStats()` for optimization
6. **Use verbose logging during development** - `WithVerbose()` option
7. **Keep secrets in environment variables** - Never in YAML files
8. **Validate configurations early** - Use `PreviewAgentConfig()` during startup
9. **Test fallback scenarios** - Ensure local files work when remote is unavailable
10. **Use source metadata** - Track configuration sources for debugging

## Security Considerations

### Secret Handling

- API keys and secrets resolved by starops-config-service
- No secrets stored in local cache
- Environment variables take precedence over remote values
- Audit trail for configuration access via remote service

### Path Validation

- Local file paths validated for safety
- No path traversal vulnerabilities
- Only reads from approved directories
- File type validation (YAML only)

## Performance

### Caching Strategy

- Default 5-minute TTL for remote configurations
- Cache key format: `source:agentName:environment`
- Thread-safe with RWMutex
- Memory-based (process-local)

### Network Optimization

- HTTP client pooling
- Configurable timeouts (default 30s)
- Compression support
- Connection reuse

## API Reference

### Main Loading Functions

```go
// Automatic source detection with fallback (Recommended)
func LoadAgentAuto(ctx context.Context, agentName, environment string) (*agent.Agent, error)

// Remote configuration only
func LoadAgentFromRemote(ctx context.Context, agentName, environment string) (*agent.Agent, error)

// Local configuration only
func LoadAgentFromLocal(ctx context.Context, agentName, environment string) (*agent.Agent, error)

// Preview configuration without creating agent
func PreviewAgentConfig(ctx context.Context, agentName, environment string) (*agent.AgentConfig, error)

// Load with custom options
func LoadAgentWithOptions(ctx context.Context, agentName, environment string,
    loadOpts []LoadOption, agentOpts ...agent.Option) (*agent.Agent, error)

// Load with custom variables
func LoadAgentWithVariables(ctx context.Context, agentName, environment string,
    variables map[string]string) (*agent.Agent, error)
```

### Configuration Types

```go
// Configuration source metadata
type ConfigSourceMetadata struct {
    Type        string            `yaml:"type" json:"type"`        // "local", "remote"
    Source      string            `yaml:"source" json:"source"`    // file path or service URL
    AgentName   string            `yaml:"agent_name,omitempty" json:"agent_name,omitempty"`
    Environment string            `yaml:"environment,omitempty" json:"environment,omitempty"`
    Variables   map[string]string `yaml:"variables,omitempty" json:"variables,omitempty"`
    LoadedAt    time.Time         `yaml:"loaded_at" json:"loaded_at"`
}

// Response from remote service
type AgentConfigResponse struct {
    AgentConfig       AgentConfig       `json:"agent_config"`
    GeneratedYAML     string            `json:"generated_yaml"`
    ResolvedYAML      string            `json:"resolved_yaml"`
    ResolvedVariables map[string]string `json:"resolved_variables"`
    MissingVariables  []string          `json:"missing_variables"`
}

// Load options for flexible configuration
type LoadOptions struct {
    PreferRemote       bool
    AllowFallback      bool
    LocalPath          string
    EnableCache        bool
    CacheTimeout       time.Duration
    EnableEnvOverrides bool
    Verbose            bool
}
```

### Functional Options

```go
// Source control
WithRemoteOnly() LoadOption
WithLocalOnly() LoadOption
WithLocalFallback(path string) LoadOption

// Caching
WithCache(timeout time.Duration) LoadOption
WithoutCache() LoadOption

// Behavior
WithEnvOverrides() LoadOption
WithVerbose() LoadOption
```

## File Structure

The implementation spans these files:

```
pkg/
├── agent/
│   ├── config.go           # Enhanced with ConfigSourceMetadata
│   ├── factory.go          # Agent factory functions
│   ├── agent.go            # Enhanced with GetConfig()
│   └── env.go              # Environment variable handling
├── agentconfig/
│   ├── client.go           # Extended with FetchAgentConfig
│   ├── models.go           # Extended with response types
│   ├── unified_loader.go   # Unified loading logic
│   ├── cache.go            # Caching system
│   ├── api.go              # Convenience functions
│   └── examples.go         # Usage examples
```

## Complete Example

Here's a comprehensive example showing multiple features:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/agentconfig"
)

func main() {
    ctx := context.Background()

    // Example 1: Simple auto-loading (recommended)
    agent1, err := agentconfig.LoadAgentAuto(ctx, "research-assistant", "production")
    if err != nil {
        log.Printf("Auto load failed: %v", err)
    } else {
        fmt.Println("Agent loaded successfully via auto-detection")
    }

    // Example 2: Preview configuration before loading
    config, err := agentconfig.PreviewAgentConfig(ctx, "research-assistant", "staging")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Configuration preview:\n")
    fmt.Printf("  Source: %s (%s)\n", config.ConfigSource.Type, config.ConfigSource.Source)
    fmt.Printf("  Role: %s\n", config.Role)
    fmt.Printf("  Goal: %s\n", config.Goal)

    // Example 3: Advanced loading with custom options
    loadOptions := []agentconfig.LoadOption{
        agentconfig.WithCache(10 * time.Minute),
        agentconfig.WithLocalFallback("./configs/fallback.yaml"),
        agentconfig.WithEnvOverrides(),
        agentconfig.WithVerbose(),
    }

    agentOptions := []agent.Option{
        agent.WithMaxIterations(5),
        agent.WithRequirePlanApproval(false),
    }

    agent2, err := agentconfig.LoadAgentWithOptions(ctx,
        "research-assistant", "development",
        loadOptions, agentOptions...)
    if err != nil {
        log.Fatal(err)
    }

    // Example 4: Load with custom variables
    variables := map[string]string{
        "topic": "quantum computing",
        "search_depth": "comprehensive",
    }

    agent3, err := agentconfig.LoadAgentWithVariables(ctx,
        "research-assistant", "development", variables)
    if err != nil {
        log.Fatal(err)
    }

    // Example 5: Cache management
    stats := agentconfig.GetCacheStats()
    fmt.Printf("Cache stats: %d valid, %d expired\n", stats["valid"], stats["expired"])

    // Clean up expired entries
    agentconfig.CleanupExpiredEntries()

    // Example 6: Use the agent
    response, err := agent3.Run(ctx, "Research latest developments in quantum computing")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Agent response: %s\n", response)

    // Example 7: Inspect configuration source
    finalConfig := agent3.GetConfig()
    if finalConfig.ConfigSource != nil {
        fmt.Printf("\nConfiguration Details:\n")
        fmt.Printf("  Loaded from: %s\n", finalConfig.ConfigSource.Type)
        fmt.Printf("  Source: %s\n", finalConfig.ConfigSource.Source)
        fmt.Printf("  Environment: %s\n", finalConfig.ConfigSource.Environment)
        fmt.Printf("  Loaded at: %v\n", finalConfig.ConfigSource.LoadedAt)
        fmt.Printf("  Variables used: %v\n", finalConfig.ConfigSource.Variables)
    }
}
```

## Troubleshooting

### Common Issues

**Issue**: Configuration not found
```
Error: agent configuration not found in any source
```
**Solution**: Ensure either remote service is configured OR local YAML files exist in expected locations.

**Issue**: Authentication failed
```
Error: unauthorized: invalid auth token
```
**Solution**: Verify `STAROPS_AUTH_TOKEN` environment variable is set correctly.

**Issue**: Cache not working
```
Always loading from remote even with cache enabled
```
**Solution**: Check cache timeout settings and ensure cache isn't being cleared elsewhere.

**Issue**: Variables not resolving
```
Configuration contains unresolved ${VAR} placeholders
```
**Solution**: Verify environment variables are set or check variable priority order.

## Future Enhancements

Planned features for future versions:

- Configuration templates and inheritance
- Real-time configuration updates via webhooks
- Configuration validation against schemas
- A/B testing support for configurations
- Configuration versioning and rollback
- Metrics for cache hit/miss rates
- Configuration load time monitoring
- Source distribution analytics

## Summary

The unified configuration loader provides a robust, flexible system for managing agent configurations across different environments and sources. By supporting both remote and local configurations with automatic fallback, it ensures reliability while enabling centralized management. The comprehensive API and backward compatibility make it easy to adopt in existing projects while providing powerful features for advanced use cases.

For more examples and detailed API documentation, see the code in `pkg/agentconfig/examples.go`.
