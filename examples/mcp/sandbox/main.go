// Command sandbox runs an MCP server in a locked-down Docker container instead
// of directly on the host.
//
// It starts the reference filesystem server (the mcp/filesystem image) with no
// network and one host folder shared read-only, then reads a file through it
// and shows that writing is refused. If OPENAI_API_KEY is set, it also gives
// the same sandboxed server to an agent.
//
// Requires a running Docker daemon:
//
//	go run ./examples/mcp/sandbox
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/sandbox"
)

// image is the server's container image. In production, pin it by digest
// ("mcp/filesystem@sha256:...") so its contents cannot change underneath you.
const image = "mcp/filesystem"

// The server's command and arguments inside the container.
var (
	serverCommand = "node"
	serverArgs    = []string{"/app/dist/index.js", "/projects"}
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	logger := logging.New()

	// A host folder to share with the server, read-only.
	dir, err := os.MkdirTemp("", "sandbox-example-")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("Hello from the host.\n"), 0o600); err != nil {
		log.Fatal(err)
	}

	executor, err := sandbox.NewDockerExecutor(sandbox.Config{
		Image:           image,
		AllowedCommands: []string{serverCommand},
		MountPaths:      []sandbox.MountPath{{Host: dir, Container: "/projects"}},
		// The defaults already give no network, a read-only root filesystem,
		// no Linux capabilities, and memory, CPU and process limits.
	}, logger)
	if err != nil {
		log.Fatalf("sandbox: %v", err)
	}
	// Close removes any container this executor started that is still running.
	defer func() { _ = executor.Close(context.Background()) }()

	// Use the executor directly for a server you start yourself.
	server, err := mcp.NewStdioServer(ctx, mcp.StdioServerConfig{
		Command:  serverCommand,
		Args:     serverArgs,
		Logger:   logger,
		Executor: executor,
	})
	if err != nil {
		log.Fatalf("start sandboxed server: %v", err)
	}
	defer func() { _ = server.Close() }()

	tools, err := server.ListTools(ctx)
	if err != nil {
		log.Fatalf("list tools: %v", err)
	}
	fmt.Printf("The sandboxed server has %d tools.\n", len(tools))

	call := func(tool string, args map[string]any) {
		resp, err := server.CallTool(ctx, tool, args)
		if err != nil {
			fmt.Printf("%s: error: %v\n", tool, err)
			return
		}
		out, _ := json.Marshal(resp.Content)
		fmt.Printf("%s (error=%v): %s\n", tool, resp.IsError, out)
	}
	call("read_file", map[string]any{"path": "/projects/notes.txt"})
	// The folder is mounted read-only, so this fails inside the container.
	call("write_file", map[string]any{"path": "/projects/new.txt", "content": "hi"})

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("Set OPENAI_API_KEY to also run the server behind an agent.")
		return
	}

	// Give the executor to an agent: every stdio MCP server the agent starts
	// then runs in its own container.
	a, err := agent.NewAgent(
		agent.WithLLM(openai.NewClient(apiKey)),
		agent.WithSystemPrompt("You answer questions about the files in /projects using your tools."),
		agent.WithLazyMCPConfigs([]agent.LazyMCPConfig{{
			Name:    "files",
			Type:    "stdio",
			Command: serverCommand,
			Args:    serverArgs,
		}}),
		agent.WithMCPCommandExecutor(executor),
		agent.WithRequirePlanApproval(false),
		agent.WithMaxIterations(4),
	)
	if err != nil {
		log.Fatalf("agent: %v", err)
	}

	ctx = multitenancy.WithOrgID(ctx, "example-org")
	ctx = memory.WithConversationID(ctx, "sandbox-example")
	answer, err := a.Run(ctx, "What does /projects/notes.txt say?")
	if err != nil {
		log.Fatalf("run: %v", err)
	}
	fmt.Println(answer)
}
