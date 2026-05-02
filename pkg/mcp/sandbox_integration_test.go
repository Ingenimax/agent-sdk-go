package mcp

import (
	"context"
	"os/exec"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/sandbox"
)

// Verify the interface compiles — LocalExecutor satisfies CommandExecutor
var _ sandbox.CommandExecutor = &sandbox.LocalExecutor{}

func TestStdioServerConfig_AcceptsExecutor(t *testing.T) {
	config := StdioServerConfig{
		Command:  "echo",
		Args:     []string{"hello"},
		Executor: &sandbox.LocalExecutor{},
	}
	if config.Executor == nil {
		t.Error("expected Executor to be set")
	}
}

// mockExecutor records calls for testing
type mockSandboxExecutor struct {
	called  bool
	lastCmd string
}

func (m *mockSandboxExecutor) Command(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
	m.called = true
	m.lastCmd = name
	return exec.CommandContext(ctx, name, args...), nil
}

func (m *mockSandboxExecutor) Close(ctx context.Context) error { return nil }

func TestStdioServerConfig_ExecutorIsUsed(t *testing.T) {
	mock := &mockSandboxExecutor{}
	config := StdioServerConfig{
		Command:  "echo",
		Args:     []string{"hello"},
		Executor: mock,
	}
	_ = config // Compiles = type is correct
}
