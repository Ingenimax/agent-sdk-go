# Configuration Merge

`agentconfig.MergeAgentConfig` combines two `agent.AgentConfig` values, letting
one supply defaults for fields the other leaves empty.

> **Changed:** this page previously documented merging a remote configuration
> with a local one. Remote configuration loading has been **removed** — see
> [Upgrading](upgrading.md#security-fix-remote-configuration-loading-removed).
> `MergeAgentConfig` remains as a general-purpose utility, and the loader no
> longer calls it. If you want two configs combined, you now do it explicitly.

## Signature

```go
func MergeAgentConfig(primary, base *agent.AgentConfig, strategy MergeStrategy) *agent.AgentConfig
```

Returns a deep copy. Neither input is modified.

## Strategies

| Strategy | Effect |
| --- | --- |
| `MergeStrategyPrimaryPriority` | `primary` wins; `base` fills gaps |
| `MergeStrategyBasePriority` | `base` wins; `primary` fills gaps |
| `MergeStrategyNone` | No merging |

`MergeStrategyRemotePriority` and `MergeStrategyLocalPriority` remain as
deprecated aliases for the first two. The old names no longer describe anything:
both inputs are local now.

## Usage

```go
shared, err := agentconfig.LoadAgentConfig(ctx, "base-agent", "production",
    agentconfig.WithLocalPath("./configs/shared-defaults.yaml"),
)
if err != nil {
    return err
}

specific, err := agentconfig.LoadAgentConfig(ctx, "research-assistant", "production")
if err != nil {
    return err
}

// The agent's own config wins; shared defaults fill anything it omits.
merged := agentconfig.MergeAgentConfig(specific, shared, agentconfig.MergeStrategyPrimaryPriority)

agent, err := agent.NewAgentFromConfigObject(ctx, merged, nil)
```

## Merge behaviour by field type

| Field type | Behaviour |
| --- | --- |
| Strings (`Role`, `Goal`, `Backstory`) | Non-empty value from the priority config wins; empty falls through |
| Pointers (`MaxIterations`, `RequirePlanApproval`) | Non-nil from the priority config wins |
| `Tools` slice | Union, deduplicated by name |
| `SubAgents` map | Merged by key; priority config wins per key |
| `LLM` | Merged field by field, not replaced wholesale |
| `MCP` | Merged by server name |
| `ConfigSource` | Marked `merged`, retaining the priority config's source |

A `nil` input returns a deep copy of the other. Two `nil` inputs return `nil`.

## Result metadata

```go
merged := agentconfig.MergeAgentConfig(specific, shared, agentconfig.MergeStrategyPrimaryPriority)

if merged.ConfigSource.Type == string(agentconfig.ConfigSourceMerged) {
    // came from two configs
}
```

## Adding fields to AgentConfig

`AgentConfig` fields must be added by hand to **both** `deepCopyAgentConfig` and
`MergeAgentConfig` in `pkg/agentconfig/unified_loader.go`. There is no reflection
fallback, and an omission fails silently at runtime rather than at compile time —
the field simply disappears when a config is copied or merged.

If you add a field, add it to both functions and cover it in `merge_test.go`.

## See also

- [Configuration loader](unified-config-loader.md)
- [Upgrading](upgrading.md)
