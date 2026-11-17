package mcp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// fakeExecutor records what it is asked to start and runs the test binary as
// an MCP server in its place.
type fakeExecutor struct {
	err    error
	nilCmd bool

	mu    sync.Mutex
	calls []fakeExecutorCall
}

type fakeExecutorCall struct {
	name string
	args []string
	env  []string
}

func (f *fakeExecutor) Command(ctx context.Context, name string, args []string, env []string) (*exec.Cmd, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fakeExecutorCall{name: name, args: args, env: env})
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if f.nilCmd {
		return nil, nil
	}
	// #nosec G204 -- the test binary itself.
	cmd := exec.CommandContext(ctx, os.Args[0])
	cmd.Env = append(os.Environ(), stdioServerEnv+"=1")
	return cmd, nil
}

func (f *fakeExecutor) recorded() []fakeExecutorCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeExecutorCall(nil), f.calls...)
}

func TestNewStdioServerUsesExecutor(t *testing.T) {
	executor := &fakeExecutor{}
	server, err := NewStdioServer(t.Context(), StdioServerConfig{
		// Not on the host: the executor, not the host's PATH, resolves it.
		Command:  "only-inside-the-sandbox",
		Args:     []string{"--flag"},
		Env:      []string{"API_KEY=secret"},
		Logger:   logging.New(),
		Executor: executor,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	resp, err := server.CallTool(t.Context(), "echo", map[string]any{"text": "sandboxed"})
	require.NoError(t, err)
	assert.Equal(t, "echo: sandboxed", resp.Content.([]mcp.Content)[0].(*mcp.TextContent).Text)

	calls := executor.recorded()
	require.Len(t, calls, 1)
	assert.Equal(t, "only-inside-the-sandbox", calls[0].name)
	assert.Equal(t, []string{"--flag"}, calls[0].args)
	assert.Equal(t, []string{"API_KEY=secret"}, calls[0].env)
}

func TestNewStdioServerExecutorErrors(t *testing.T) {
	refused := errors.New("not allowed")

	_, err := NewStdioServer(t.Context(), StdioServerConfig{
		Command:  "tool",
		Logger:   logging.New(),
		Executor: &fakeExecutor{err: refused},
	})
	require.ErrorIs(t, err, refused)
	assert.ErrorContains(t, err, `command executor refused "tool"`)

	_, err = NewStdioServer(t.Context(), StdioServerConfig{
		Command:  "tool",
		Logger:   logging.New(),
		Executor: &fakeExecutor{nilCmd: true},
	})
	assert.ErrorContains(t, err, "returned no command")
}

func TestLazyCacheKeepsExecutorsApart(t *testing.T) {
	sandboxA, sandboxB := &fakeExecutor{}, &fakeExecutor{}
	base := LazyMCPServerConfig{Name: t.Name(), Type: "stdio", Command: "server"}
	withA, withB := base, base
	withA.Executor = sandboxA
	withB.Executor = sandboxB
	forgetServer(t, withA)
	forgetServer(t, withB)

	serverA, err := GetOrCreateServerFromCache(t.Context(), withA)
	require.NoError(t, err)
	serverB, err := GetOrCreateServerFromCache(t.Context(), withB)
	require.NoError(t, err)
	assert.NotSame(t, serverA, serverB, "servers from different executors were shared")

	again, err := GetOrCreateServerFromCache(t.Context(), withA)
	require.NoError(t, err)
	assert.Same(t, serverA, again)
	assert.Len(t, sandboxA.recorded(), 1)
	assert.Len(t, sandboxB.recorded(), 1)

	assert.NotEqual(t, lazyServerKey(base), lazyServerKey(withA), "a sandboxed server shares the host server's key")
}

type valueExecutor struct{ id int }

func (valueExecutor) Command(context.Context, string, []string, []string) (*exec.Cmd, error) {
	return nil, errors.New("unused")
}

func TestExecutorKey(t *testing.T) {
	a, b := &fakeExecutor{}, &fakeExecutor{}
	assert.Empty(t, executorKey(nil))
	assert.Equal(t, executorKey(a), executorKey(a))
	assert.NotEqual(t, executorKey(a), executorKey(b))
	assert.Equal(t, executorKey(valueExecutor{1}), executorKey(valueExecutor{1}))
	assert.NotEqual(t, executorKey(valueExecutor{1}), executorKey(valueExecutor{2}))
}
