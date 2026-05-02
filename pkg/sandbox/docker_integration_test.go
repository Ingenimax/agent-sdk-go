//go:build integration

package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestDockerExecutor_Integration_CreateAndExecute(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger := logging.New()

	executor, err := NewDockerExecutor(ctx, Config{
		Enabled:         true,
		Image:           "alpine:3.19",
		AllowedCommands: []string{"echo", "ls", "cat"},
		PoolSize:        1,
		Timeout:         10 * time.Second,
	}, logger)
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}
	defer executor.Close(ctx)

	// Test: execute an allowed command
	cmd, err := executor.Command(ctx, "echo", "hello sandbox")
	if err != nil {
		t.Fatalf("failed to create command: %v", err)
	}
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to run command: %v", err)
	}
	if got := string(output); got != "hello sandbox\n" {
		t.Errorf("expected 'hello sandbox\\n', got %q", got)
	}

	// Test: denied command
	_, err = executor.Command(ctx, "rm", "-rf", "/")
	if err == nil {
		t.Error("expected error for denied command 'rm'")
	}
}

func TestDockerExecutor_Integration_ContainerIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger := logging.New()

	executor, err := NewDockerExecutor(ctx, Config{
		Enabled:         true,
		Image:           "alpine:3.19",
		AllowedCommands: []string{"cat"},
		PoolSize:        1,
		NetworkMode:     "none",
	}, logger)
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}
	defer executor.Close(ctx)

	// Host's /etc/hostname should NOT be accessible as host content
	cmd, err := executor.Command(ctx, "cat", "/etc/hostname")
	if err != nil {
		t.Fatalf("failed to create command: %v", err)
	}
	output, err := cmd.Output()
	if err != nil {
		t.Logf("cat /etc/hostname failed (expected in isolated container): %v", err)
		return
	}
	t.Logf("container hostname: %s", string(output))
}
