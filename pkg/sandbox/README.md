# MCP sandbox

Package `sandbox` runs stdio MCP servers in Docker containers instead of directly on the host. Use it when an agent starts MCP servers you do not fully trust, or when you want to limit what a server can reach if it is compromised.

It plugs into the SDK through `mcp.CommandExecutor`, so nothing else in the SDK depends on Docker. Agents that never configure a sandbox run their servers on the host, as before.

## Quick start

```go
executor, err := sandbox.NewDockerExecutor(sandbox.Config{
    Image:           "mcp/filesystem@sha256:...", // pin by digest
    AllowedCommands: []string{"node"},
    MountPaths: []sandbox.MountPath{
        {Host: "/srv/docs", Container: "/projects"}, // read-only by default
    },
}, logger)
if err != nil {
    return err
}
defer executor.Close(context.Background())

myAgent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithLazyMCPConfigs([]agent.LazyMCPConfig{{
        Name:    "files",
        Type:    "stdio",
        Command: "node",
        Args:    []string{"/app/dist/index.js", "/projects"},
    }}),
    agent.WithMCPCommandExecutor(executor),
)
```

`WithMCPCommandExecutor` covers every stdio server the agent starts: from `WithLazyMCPConfigs`, `WithMCPURLs`, `WithMCPPresets` and MCP YAML configuration. For a server you create yourself, set `mcp.StdioServerConfig.Executor`. Servers passed to `WithMCPServers` are already running, so the option cannot apply to them.

A runnable example is in [`examples/mcp/sandbox`](../../examples/mcp/sandbox).

## Configuration

| Field | Default | Meaning |
|---|---|---|
| `Image` | required | Image the server runs in. Pin it by digest. |
| `AllowedCommands` | none | Commands servers may start. A bare name (`node`) is looked up on the container's `PATH`; an absolute path (`/usr/local/bin/node`) must match exactly. Empty allows nothing. |
| `DeniedCommands` | none | Refused even when allowed, matched by base name (`sh` refuses `/bin/sh` too). |
| `NetworkMode` | `none` | As for `docker run --network`. Use `bridge` for servers that call APIs. `host` is refused. |
| `MemoryLimit` | `256m` | As for `docker run --memory`. |
| `CPULimit` | `0.5` | As for `docker run --cpus`. |
| `PidsLimit` | `64` | Maximum processes in the container. |
| `TmpfsSize` | `64m` | Size of the writable `/tmp`. |
| `AllowExecInTmp` | `false` | Lets programs run from `/tmp`. Turn it on only for servers that download a program at start and then run it from `/tmp`. |
| `User` | image default | As for `docker run --user`. |
| `MountPaths` | none | Host paths to share. Read-only unless `Writable` is set. |
| `DockerPath` | `docker` on `PATH` | The docker CLI to run. |

### Servers started with npx

A Node server launched with `npx` downloads itself at start, so it needs network access and a writable home. With an image such as `node:22-alpine` (pinned by digest), allow `npx`, set `NetworkMode: "bridge"`, and give the server `HOME=/tmp` in its environment:

```go
agent.WithLazyMCPConfigs([]agent.LazyMCPConfig{{
    Name:    "files",
    Type:    "stdio",
    Command: "npx",
    Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/projects"},
    Env:     []string{"HOME=/tmp"},
}})
```

Prefer an image that already contains the server, which can run with no network at all.

## Security model

Each server gets its own new container (`docker run --rm -i`). Containers are never reused or shared, so servers cannot see each other's files or processes. Every container:

- starts the command with `--entrypoint`, so the image's own entrypoint is ignored and only allowed commands run
- has a read-only root filesystem and a size-limited `/tmp` that cannot hold executables (unless `AllowExecInTmp` is set)
- drops all Linux capabilities and sets `no-new-privileges`
- has memory, CPU and process-count limits
- has no network by default
- runs an init process that reaps the server's child processes
- sees only the host paths in `MountPaths`, read-only by default
- receives only the server's configured environment variables, never the host's

Secret values are handed to the docker CLI through its environment and named with `--env KEY`, so they do not appear in the host's process list. `HOME`, `PATH` and `DOCKER_*` are passed inline instead, so setting them for a server does not change how the docker CLI itself behaves.

What it does not protect against:

- **Kernel exploits.** Containers share the host's kernel. Docker isolation is not a virtual machine boundary.
- **What an allowed program does.** The allowlist checks the command the agent asks to start, not what that program runs once inside the container.
- **A changing image.** The image decides what is in the container. Pin it by digest; `NewDockerExecutor` logs a warning when it is not pinned.
- **The network you allow.** With `bridge`, a server can reach anything the Docker network can reach, including services on your private network.

## Cleanup

When a server stops normally, its input closes, it exits, and Docker removes the container. If the context given to `Command` is canceled, the container is force-removed. `Close` force-removes every container the executor started that still exists.

If the host process is killed before any of that runs, Docker closes the server's input and well-behaved MCP servers exit. Every container carries the label `agent-sdk-go.sandbox`, so you can find any that did not:

```bash
docker ps --filter label=agent-sdk-go.sandbox
```

## Tests

Unit tests use a stand-in for the docker CLI and need no Docker. The integration tests check the restrictions against a real daemon:

```bash
go test -tags integration ./pkg/sandbox/
```
