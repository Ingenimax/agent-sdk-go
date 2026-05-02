package sandbox

import (
	"context"
	"testing"
)

func TestLocalExecutor_Command(t *testing.T) {
	executor := &LocalExecutor{}
	cmd, err := executor.Command(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	if cmd.Path == "" {
		t.Error("expected cmd.Path to be set")
	}
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to run cmd: %v", err)
	}
	if string(output) != "hello\n" {
		t.Errorf("unexpected output: %q", string(output))
	}
}

func TestLocalExecutor_Close(t *testing.T) {
	executor := &LocalExecutor{}
	if err := executor.Close(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Compile-time check that LocalExecutor implements CommandExecutor
var _ CommandExecutor = (*LocalExecutor)(nil)
