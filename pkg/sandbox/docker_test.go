package sandbox

import (
	"context"
	"errors"
	"testing"
)

func TestDockerExecutor_Command_DeniedByAllowlist(t *testing.T) {
	executor := &DockerExecutor{
		config:    Config{Enabled: true, Image: "ubuntu:22.04"},
		allowlist: NewAllowlist([]string{"git"}, nil),
		pool: &Pool{
			containers: []Container{{ID: "test", Name: "test", Ready: true}},
		},
	}

	_, err := executor.Command(context.Background(), "rm", "-rf", "/")
	if err == nil {
		t.Fatal("expected error for denied command")
	}
	if !errors.Is(err, ErrCommandDenied) {
		t.Errorf("expected ErrCommandDenied, got: %v", err)
	}
}

func TestDockerExecutor_Command_AllowedCommand(t *testing.T) {
	executor := &DockerExecutor{
		config:    Config{Enabled: true, Image: "ubuntu:22.04"},
		allowlist: NewAllowlist([]string{"git"}, nil),
		pool: &Pool{
			containers: []Container{{ID: "abc123", Name: "sandbox-0", Ready: true}},
		},
	}

	cmd, err := executor.Command(context.Background(), "git", "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	args := cmd.Args
	if len(args) < 6 {
		t.Fatalf("expected at least 6 docker exec args, got: %v", args)
	}
	if args[1] != "exec" {
		t.Errorf("expected 'exec', got %q", args[1])
	}
	if args[2] != "-i" {
		t.Errorf("expected '-i', got %q", args[2])
	}
	if args[3] != "abc123" {
		t.Errorf("expected container ID 'abc123', got %q", args[3])
	}
	if args[4] != "git" {
		t.Errorf("expected command 'git', got %q", args[4])
	}
	if args[5] != "status" {
		t.Errorf("expected arg 'status', got %q", args[5])
	}
}

func TestDockerExecutor_Close(t *testing.T) {
	var closed []string
	closeFn := func(ctx context.Context, id string) error {
		closed = append(closed, id)
		return nil
	}
	executor := &DockerExecutor{
		config: Config{Enabled: true},
		pool: &Pool{
			containers: []Container{{ID: "abc", Name: "s-0", Ready: true}},
			closeFn:    closeFn,
		},
	}

	if err := executor.Close(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(closed) != 1 || closed[0] != "abc" {
		t.Errorf("expected container 'abc' to be closed, got: %v", closed)
	}
}

func TestBuildContainerArgs(t *testing.T) {
	config := Config{
		Image:       "ubuntu:22.04",
		MemoryLimit: "256m",
		CPULimit:    "0.5",
		NetworkMode: "none",
		MountPaths: []MountPath{
			{Host: "/data", Container: "/mnt/data", ReadOnly: true},
		},
	}

	args := buildContainerArgs(config, "test-container")

	// Check key flags are present
	expected := map[string]bool{
		"run": false, "-d": false, "--read-only": false,
		"--cap-drop": false, "ALL": false,
		"--security-opt": false, "no-new-privileges": false,
	}
	for _, arg := range args {
		if _, ok := expected[arg]; ok {
			expected[arg] = true
		}
	}
	for flag, found := range expected {
		if !found {
			t.Errorf("expected flag %q in args", flag)
		}
	}

	// Check mount is present with :ro suffix
	foundMount := false
	for _, arg := range args {
		if arg == "/data:/mnt/data:ro" {
			foundMount = true
		}
	}
	if !foundMount {
		t.Error("expected mount /data:/mnt/data:ro in args")
	}

	// Last args should be image + sleep infinity
	if args[len(args)-3] != "ubuntu:22.04" {
		t.Errorf("expected image as third-to-last arg, got %q", args[len(args)-3])
	}
	if args[len(args)-2] != "sleep" || args[len(args)-1] != "infinity" {
		t.Error("expected 'sleep infinity' as last args")
	}
}
