// Package sandbox runs stdio MCP servers in Docker containers instead of
// directly on the host.
//
// DockerExecutor implements mcp.CommandExecutor. Give it to an agent with
// agent.WithMCPCommandExecutor, or set it on mcp.StdioServerConfig.Executor
// for a server you start yourself:
//
//	executor, err := sandbox.NewDockerExecutor(sandbox.Config{
//		Image:           "mcp/filesystem@sha256:...",
//		AllowedCommands: []string{"node"},
//		MountPaths:      []sandbox.MountPath{{Host: "/srv/docs", Container: "/projects"}},
//	}, logger)
//	if err != nil {
//		return err
//	}
//	defer executor.Close(context.Background())
//
//	a, err := agent.NewAgent(
//		agent.WithLLM(llm),
//		agent.WithLazyMCPConfigs([]agent.LazyMCPConfig{{
//			Name:    "files",
//			Type:    "stdio",
//			Command: "node",
//			Args:    []string{"/app/dist/index.js", "/projects"},
//		}}),
//		agent.WithMCPCommandExecutor(executor),
//	)
//
// # What the sandbox does
//
// Each server gets its own new container (docker run --rm -i), which runs
// only that server and is removed when the server exits. Containers are never
// reused or shared, so servers cannot see each other's files or processes.
// Each container:
//
//   - runs the command with --entrypoint, so the image's own entrypoint is
//     ignored and only allowed commands start
//   - has a read-only root filesystem, plus a writable tmpfs at /tmp that
//     cannot hold executables unless Config.AllowExecInTmp is set
//   - drops all Linux capabilities and sets no-new-privileges
//   - has memory, CPU and process-count limits
//   - has no network unless Config.NetworkMode allows it ("host" is refused)
//   - runs an init process (--init) that reaps the server's child processes
//   - sees only the host paths in Config.MountPaths, read-only by default
//   - receives only the server's configured environment variables, never the
//     host's environment
//
// # What it does not do
//
//   - Containers share the host's kernel. Docker isolation is not a virtual
//     machine boundary; a kernel exploit can escape it.
//   - The allowlist checks the command the agent asks to start. It does not
//     limit what that program runs once it is inside the container.
//   - The image decides what is in the container. Pin it by digest
//     ("image@sha256:...") so it cannot change underneath you.
//     NewDockerExecutor logs a warning when the image is not pinned.
//   - Allowing network access lets the server reach anything the Docker
//     network can reach, including services on your private network.
//
// # Cleanup
//
// When a server stops normally, its input closes, the server exits and Docker
// removes the container. If the context given to Command is canceled, the
// container is force-removed. Close force-removes every container the
// executor started that is still running. If the host process is killed
// before any of that runs, Docker closes the server's input and well-behaved
// MCP servers exit; to find any that did not, every container carries the
// label "agent-sdk-go.sandbox":
//
//	docker ps --filter label=agent-sdk-go.sandbox
package sandbox
