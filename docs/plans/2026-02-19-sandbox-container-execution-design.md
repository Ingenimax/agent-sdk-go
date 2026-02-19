# Sandbox Container Execution Design

**Date:** 2026-02-19
**Status:** Approved
**Scope:** Add container-based sandboxing for MCP command execution with command allowlisting

## Problem

The SDK executes MCP stdio server commands directly on the host via `exec.CommandContext` (`pkg/mcp/mcp.go`). There is no isolation boundary — a malicious or misconfigured MCP server can access the host filesystem, network, and processes. The existing guardrails system only filters text content and does not restrict actual tool execution.

### Bugs Found During Audit

1. **`ToolMiddleware` missing `Execute()` method** — Every LLM provider (OpenAI, Anthropic, Gemini, DeepSeek, Azure) calls `tool.Execute()`, but `ToolMiddleware` only implements `Run()`. Guardrails applied via `ToolMiddleware` are completely bypassed during agent execution. (`pkg/guardrails/tool_middleware.go`)

2. **`ToolRestrictionGuardrail` is text-pattern only** — Regex matches `"use tool <name>"` in prompt text. LLMs use structured tool calls, not this text pattern. Provides zero protection against actual tool invocations. (`pkg/guardrails/tool_restriction.go`)

3. **MCP args unsanitized** — `config.Args` passed directly to `exec.CommandContext()`. While Go's `exec` avoids shell injection, args can contain malicious flags the target command interprets dangerously.

4. **MCP env inherits host environment** — `cmd.Env = append(os.Environ(), config.Env...)` exposes all host env vars to MCP processes.

5. **No execution timeout on MCP commands** — Only the caller's context provides timeout. No default deadline.

## Design

### Approach: Standalone `pkg/sandbox` Package with Docker Runtime

A new opt-in package that provides container-based command execution with command allowlisting. Integrates with MCP's existing `exec.Cmd`-based transport by returning `*exec.Cmd` instances instead of captured output.

### Package Structure

```
pkg/sandbox/
  ├── sandbox.go         # CommandExecutor interface + LocalExecutor (default)
  ├── config.go          # Config structs (YAML-compatible)
  ├── allowlist.go       # Command allowlist logic (allow + deny lists)
  ├── docker.go          # DockerExecutor implementation
  ├── pool.go            # Warm container pool (session-scoped)
  ├── sandbox_test.go
  ├── allowlist_test.go
  ├── docker_test.go     # Integration tests (//go:build integration)
  └── pool_test.go
```

### Core Interface

```go
// CommandExecutor creates exec.Cmd instances, optionally sandboxed.
// Returns *exec.Cmd so MCP's CommandTransport can attach stdin/stdout pipes.
type CommandExecutor interface {
    Command(ctx context.Context, name string, args ...string) (*exec.Cmd, error)
    Close(ctx context.Context) error
}
```

**Why `*exec.Cmd` instead of captured output:** MCP stdio servers communicate via stdin/stdout of the child process. The MCP `CommandTransport` needs pipe access to the process, not just the final output. Returning `*exec.Cmd` lets the sandbox slot in transparently.

### LocalExecutor (Default — No Sandbox)

```go
type LocalExecutor struct{}

func (l *LocalExecutor) Command(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
    return exec.CommandContext(ctx, name, args...), nil
}

func (l *LocalExecutor) Close(ctx context.Context) error { return nil }
```

Zero overhead. Preserves exact current behavior when no sandbox is configured.

### DockerExecutor

```go
type DockerExecutor struct {
    config    Config
    allowlist *Allowlist
    pool      *Pool
    logger    logging.Logger
}
```

**Container creation flags:**
```
docker run -d --name agent-sandbox-<session>-<n>
  --memory <memory_limit>
  --cpus <cpu_limit>
  --network <network_mode>
  --read-only
  --tmpfs /tmp:size=64m
  --security-opt no-new-privileges
  --cap-drop ALL
  --pids-limit 64
  <image>
  sleep infinity
```

**Command execution:**
```go
func (d *DockerExecutor) Command(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
    if err := d.allowlist.Check(name); err != nil {
        return nil, err
    }
    container := d.pool.Acquire()
    dockerArgs := append([]string{"exec", "-i", container.ID, name}, args...)
    return exec.CommandContext(ctx, "docker", dockerArgs...), nil
}
```

### Config

```go
type Config struct {
    Enabled         bool          `yaml:"enabled"`
    Image           string        `yaml:"image"`
    AllowedCommands []string      `yaml:"allowed_commands"`
    DeniedCommands  []string      `yaml:"denied_commands"`
    PoolSize        int           `yaml:"pool_size"`
    Timeout         time.Duration `yaml:"timeout"`
    MemoryLimit     string        `yaml:"memory_limit"`
    CPULimit        string        `yaml:"cpu_limit"`
    NetworkMode     string        `yaml:"network_mode"`
    MountPaths      []MountPath   `yaml:"mount_paths"`
}

type MountPath struct {
    Host      string `yaml:"host"`
    Container string `yaml:"container"`
    ReadOnly  bool   `yaml:"read_only"`
}
```

**Defaults:**
- `PoolSize`: 1
- `Timeout`: 30s
- `MemoryLimit`: "256m"
- `CPULimit`: "0.5"
- `NetworkMode`: "none"
- `MountPaths[].ReadOnly`: true

### Allowlist

```go
type Allowlist struct {
    allowed map[string]bool
    denied  map[string]bool
}

func (a *Allowlist) Check(command string) error
```

Resolution order:
1. Extract base name (`/usr/bin/git` -> `git`)
2. If in `denied` -> reject (always wins)
3. If `allowed` is non-empty and command not in it -> reject
4. If `allowed` is empty -> reject all (fail-closed)
5. Otherwise -> permit

### Warm Container Pool

```go
type Pool struct {
    containers []Container
    mu         sync.Mutex
    nextIdx    int
}
```

- Creates `PoolSize` containers at `NewDockerExecutor()` time
- Round-robin selection via `Acquire()`
- Lazy health recovery: if a container is dead on `Acquire()`, replace it
- `Close()` stops and removes all containers

### Error Types

```go
var (
    ErrCommandDenied      = errors.New("sandbox: command not in allowlist")
    ErrDockerNotFound     = errors.New("sandbox: docker binary not found")
    ErrContainerUnhealthy = errors.New("sandbox: container not ready")
    ErrCommandTimeout     = errors.New("sandbox: command execution timed out")
)
```

### SDK Integration — Go API

```go
agent.New(
    agent.WithLLM(llm),
    agent.WithSandbox(sandbox.NewDockerExecutor(ctx, sandbox.Config{
        Image:           "node:20-slim",
        AllowedCommands: []string{"npx", "node", "ls"},
        PoolSize:        1,
        NetworkMode:     "none",
    }, logger)),
    agent.WithLazyMCPConfigs(configs),
)
```

### SDK Integration — YAML Config

```yaml
mcp:
  mcpServers:
    filesystem:
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem"]
      sandbox:
        enabled: true
        image: "node:20-slim"
        allowed_commands: ["npx", "node", "ls", "cat"]
        denied_commands: ["rm", "dd", "mkfs"]
        pool_size: 1
        timeout: "30s"
        memory_limit: "256m"
        network_mode: "none"
```

### MCP Integration (Minimal Change)

**`StdioServerConfig` — add one field:**
```go
type StdioServerConfig struct {
    Command  string
    Args     []string
    Env      []string
    Logger   logging.Logger
    Executor sandbox.CommandExecutor  // nil defaults to LocalExecutor
}
```

**`NewStdioServerWithRetry` — replace `exec.CommandContext` call:**
```go
executor := config.Executor
if executor == nil {
    executor = &sandbox.LocalExecutor{}
}
cmd, err := executor.Command(ctx, commandPath, config.Args...)
```

Existing users who don't set `Executor` get the exact same behavior as today. Non-breaking change.

### Bug Fix: ToolMiddleware.Execute()

Add the missing `Execute()` method to `ToolMiddleware`:

```go
func (m *ToolMiddleware) Execute(ctx context.Context, args string) (string, error) {
    processedInput, err := m.pipeline.ProcessRequest(ctx, args)
    if err != nil {
        return "", err
    }
    output, err := m.tool.Execute(ctx, processedInput)
    if err != nil {
        return "", err
    }
    processedOutput, err := m.pipeline.ProcessResponse(ctx, output)
    if err != nil {
        return "", err
    }
    return processedOutput, nil
}
```

### Testing Strategy

| File | Type | Requires |
|------|------|----------|
| `sandbox_test.go` | Unit | Nothing |
| `allowlist_test.go` | Unit | Nothing |
| `pool_test.go` | Unit | Nothing (mocks Docker CLI) |
| `docker_test.go` | Integration | Docker daemon (`//go:build integration`) |
| MCP sandbox test | Integration | Docker daemon |

### Security Properties

- **Network isolation**: `--network none` by default
- **Filesystem isolation**: `--read-only` + tmpfs `/tmp` only
- **Privilege isolation**: `--cap-drop ALL`, `--no-new-privileges`
- **Resource limits**: memory, CPU, PID limits
- **Command restriction**: fail-closed allowlist, deny takes precedence
- **No host env leakage**: sandbox containers get only explicitly configured env vars
