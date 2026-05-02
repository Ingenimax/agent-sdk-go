package sandbox

import (
	"context"
	"os/exec"
)

// CommandExecutor creates exec.Cmd instances, optionally sandboxed.
// Returns *exec.Cmd so MCP's CommandTransport can attach stdin/stdout pipes.
type CommandExecutor interface {
	// Command creates an exec.Cmd for the given command and args.
	Command(ctx context.Context, name string, args ...string) (*exec.Cmd, error)
	// Close releases sandbox resources (stops containers, etc.).
	Close(ctx context.Context) error
}

// LocalExecutor runs commands directly on the host with no sandboxing.
// This is the default executor when no sandbox is configured.
type LocalExecutor struct{}

// Command creates an exec.Cmd that runs directly on the host.
func (l *LocalExecutor) Command(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, name, args...), nil
}

// Close is a no-op for LocalExecutor.
func (l *LocalExecutor) Close(_ context.Context) error {
	return nil
}
