# Agent Configuration Loader

`pkg/agentconfig` loads agent definitions from YAML files and constructs agents
from them, with environment-variable expansion, variable substitution and
optional caching.

> **Changed:** this loader previously fetched configuration over HTTP from a
> remote configuration service. That transport has been **removed** — it gave
> the configuration service arbitrary command execution inside the agent
> process. Configuration is now loaded from local files only. See
> [Upgrading](upgrading.md#security-fix-remote-configuration-loading-removed)
> for the details and a migration path.

## Quick start

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/agentconfig"

agent, err := agentconfig.LoadAgentFromLocal(ctx, "research-assistant", "production")
if err != nil {
    log.Fatal(err)
}

response, err := agent.Run(ctx, "Summarise today's incidents")
```

## Where configuration is found

With no explicit path, these locations are tried in order for an agent named
`research-assistant` in environment `production`:

1. `./configs/research-assistant.yaml`
2. `./configs/research-assistant-production.yaml`
3. `./agents/research-assistant.yaml`
4. `./research-assistant.yaml`

The first that exists wins. Point at a specific file with `WithLocalPath`:

```go
config, err := agentconfig.LoadAgentConfig(ctx, "research-assistant", "production",
    agentconfig.WithLocalPath("./deploy/agents/research.yaml"),
)
```

The file may contain either a map of agents keyed by name, or a single agent
config:

```yaml
research-assistant:
  role: "Research Assistant"
  goal: "Find and summarise information"
  backstory: "A meticulous researcher"
  llm:
    provider: openai
    model: gpt-4o
```

## Load options

| Option | Effect |
| --- | --- |
| `WithLocalPath(path)` | Load from a specific file instead of searching |
| `WithCache(timeout)` | Cache the parsed config for `timeout` |
| `WithoutCache()` | Bypass the cache |
| `WithEnvOverrides()` | Expand environment variables in the config (on by default) |
| `WithVerbose()` | Print each loading step |

Deprecated, kept so existing code compiles:

| Deprecated | Use instead |
| --- | --- |
| `WithLocalFallback(path)` | `WithLocalPath(path)` |
| `WithLocalOnly()` | nothing — loading is always local |

## Entry points

| Function | Returns | Notes |
| --- | --- | --- |
| `LoadAgentFromLocal(ctx, name, env, opts...)` | `*agent.Agent` | The common case |
| `LoadAgentConfig(ctx, name, env, opts...)` | `*agent.AgentConfig` | Config only, no agent |
| `PreviewAgentConfig(ctx, name, env)` | `*agent.AgentConfig` | Resolved config, never cached |
| `LoadAgentWithOptions(ctx, name, env, loadOpts, agentOpts...)` | `*agent.Agent` | Full control over both |
| `LoadAgentWithVariables(ctx, name, env, vars, opts...)` | `*agent.Agent` | With template substitution |

`LoadAgentAuto` is a deprecated alias for `LoadAgentFromLocal`. It previously
tried a remote service first.

## Variable substitution

`LoadAgentWithVariables` substitutes into templated fields:

```go
agent, err := agentconfig.LoadAgentWithVariables(ctx, "research-assistant", "production",
    map[string]string{
        "topic":        "artificial intelligence",
        "search_depth": "comprehensive",
    },
)
```

Environment variables are expanded separately, and are on by default. Both
`${VAR}` and `${VAR:-default}` forms are supported.

## Source metadata

A loaded config records where it came from:

```go
config, _ := agentconfig.PreviewAgentConfig(ctx, "research-assistant", "production")

fmt.Println(config.ConfigSource.Type)        // always "local"
fmt.Println(config.ConfigSource.Source)      // absolute path to the file
fmt.Println(config.ConfigSource.Environment) // "production"
fmt.Println(config.ConfigSource.LoadedAt)
```

`ConfigSource.Type` is always `"local"`. Nothing can produce a config marked
`"remote"` any more; a test asserts this structurally so a reintroduced fetch
fails the build rather than lying dormant.

## Caching

Configs are cached by `name:environment` for five minutes by default.

```go
agentconfig.ClearCache()                  // everything
agentconfig.ClearCacheEntry("agent:prod") // one entry
agentconfig.GetCacheStats()               // sizes
agentconfig.CleanupExpiredEntries()
```

`PreviewAgentConfig` never caches, so it always reflects the file on disk.

## Error handling

Loading fails closed. A missing file is an error rather than a silent fallback:

```go
config, err := agentconfig.LoadAgentConfig(ctx, "missing-agent", "production")
// err: failed to load agent config: no local configuration file found for agent missing-agent
```

## Security notes

The `mcp:` section of an agent config names a local executable, its arguments
and its environment, and `pkg/mcp` runs it. **Treat any config file as
executable code.** Validation before launch is limited to the binary path being
absolute, existing, and not a directory — which `/bin/sh` satisfies.

Load configuration only from sources you control. If you build your own
retrieval on top of this loader, validate the `mcp:` section before writing the
file to disk.

## See also

- [Configuration merge](config-merge.md) — combining two configs
- [Upgrading](upgrading.md) — what changed and why
- [LLM YAML configuration](llm-yaml-configuration.md)
- [MCP guide](mcp-guide.md)
