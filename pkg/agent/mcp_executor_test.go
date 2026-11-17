package agent

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// recordingExecutor records the stdio servers it is asked to start and
// refuses them, so tests can see which servers went through it.
type recordingExecutor struct {
	mu    sync.Mutex
	names []string
}

func (r *recordingExecutor) Command(_ context.Context, name string, _ []string, _ []string) (*exec.Cmd, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, name)
	return nil, errors.New("refused by test executor")
}

func TestWithMCPCommandExecutor(t *testing.T) {
	executor := &recordingExecutor{}
	a, err := NewAgent(
		WithLLM(&mockLLM{}),
		WithMCPCommandExecutor(executor),
	)
	if err != nil {
		t.Fatal(err)
	}
	if a.mcpExecutor != executor {
		t.Fatal("WithMCPCommandExecutor did not set the executor")
	}

	stdio := a.lazyServerConfig(LazyMCPConfig{Name: "s", Type: "stdio", Command: "server"})
	if stdio.Executor != executor {
		t.Error("stdio server did not get the agent's executor")
	}
	http := a.lazyServerConfig(LazyMCPConfig{Name: "h", Type: "http", URL: "http://127.0.0.1:1/mcp"})
	if http.Executor != nil {
		t.Error("http server got a command executor")
	}

	plain := &Agent{logger: logging.New()}
	if got := plain.lazyServerConfig(LazyMCPConfig{Type: "stdio", Command: "server"}); got.Executor != nil {
		t.Error("an agent without an executor set one on its stdio server")
	}
}

func TestLazyMCPDiscoveryGoesThroughExecutor(t *testing.T) {
	executor := &recordingExecutor{}
	a := &Agent{logger: logging.New(), mcpExecutor: executor, lazyMCPConfigs: []LazyMCPConfig{
		// Not on the host, so only the executor could start it.
		{Name: t.Name(), Type: "stdio", Command: "server-only-in-sandbox"},
	}}
	// The executor refuses, so discovery fails and no tools are created.
	if tools := a.createLazyMCPTools(); len(tools) != 0 {
		t.Fatalf("tools = %v, want none", toolNames(tools))
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if len(executor.names) != 1 || executor.names[0] != "server-only-in-sandbox" {
		t.Fatalf("executor saw %v, want the stdio server", executor.names)
	}
}
