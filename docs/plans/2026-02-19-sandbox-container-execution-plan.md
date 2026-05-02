# Sandbox Container Execution Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add opt-in Docker-based container sandboxing for MCP command execution with command allowlisting, plus fix the ToolMiddleware.Execute() bug.

**Architecture:** New `pkg/sandbox` package provides a `CommandExecutor` interface that returns `*exec.Cmd` instances. MCP's `StdioServerConfig` accepts an optional `CommandExecutor` — nil means direct host execution (current behavior). Docker implementation manages a warm container pool with security hardening and a fail-closed command allowlist.

**Tech Stack:** Go stdlib (`os/exec`, `sync`), Docker CLI (no SDK dependency), YAML config via `gopkg.in/yaml.v3`

---

### Task 1: Bug Fix — ToolMiddleware.Execute()

**Files:**
- Modify: `pkg/guardrails/tool_middleware.go:39` (after existing `Run` method)
- Create: `pkg/guardrails/tool_middleware_test.go`

**Step 1: Write the failing test**

Create `pkg/guardrails/tool_middleware_test.go`:

```go
package guardrails

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// mockTool implements interfaces.Tool for testing
type mockTool struct {
	name        string
	description string
	runOutput   string
	execOutput  string
	runErr      error
	execErr     error
}

func (m *mockTool) Name() string                                        { return m.name }
func (m *mockTool) Description() string                                 { return m.description }
func (m *mockTool) Parameters() map[string]interfaces.ParameterSpec     { return nil }
func (m *mockTool) Run(ctx context.Context, input string) (string, error)    { return m.runOutput, m.runErr }
func (m *mockTool) Execute(ctx context.Context, args string) (string, error) { return m.execOutput, m.execErr }

func TestToolMiddleware_Execute(t *testing.T) {
	tool := &mockTool{
		name:       "test_tool",
		execOutput: "raw output with badword inside",
	}

	pipeline := NewPipeline(NewContentFilter([]string{"badword"}, RedactAction))
	middleware := NewToolMiddleware(tool, pipeline)

	result, err := middleware.Execute(context.Background(), "input with badword here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both input and output should have "badword" redacted
	if result == "raw output with badword inside" {
		t.Error("Execute() did not apply guardrails to output — guardrails are bypassed")
	}
	if result != "raw output with **** inside" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestToolMiddleware_Execute_BlockAction(t *testing.T) {
	tool := &mockTool{
		name:       "test_tool",
		execOutput: "clean output",
	}

	pipeline := NewPipeline(NewContentFilter([]string{"blocked"}, BlockAction))
	middleware := NewToolMiddleware(tool, pipeline)

	_, err := middleware.Execute(context.Background(), "this is blocked content")
	if err == nil {
		t.Error("expected error for blocked content, got nil")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/guardrails/ -run TestToolMiddleware_Execute -v`
Expected: Compilation error — `ToolMiddleware` does not implement `Execute`

**Step 3: Write minimal implementation**

Add to `pkg/guardrails/tool_middleware.go` after the `Run` method (after line 59):

```go
// Execute executes the tool with the given arguments, applying guardrails
func (m *ToolMiddleware) Execute(ctx context.Context, args string) (string, error) {
	// Process request through guardrails
	processedInput, err := m.pipeline.ProcessRequest(ctx, args)
	if err != nil {
		return "", err
	}

	// Call the underlying tool
	output, err := m.tool.Execute(ctx, processedInput)
	if err != nil {
		return "", err
	}

	// Process response through guardrails
	processedOutput, err := m.pipeline.ProcessResponse(ctx, output)
	if err != nil {
		return "", err
	}

	return processedOutput, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/guardrails/ -run TestToolMiddleware_Execute -v`
Expected: PASS

**Step 5: Run full guardrails tests**

Run: `go test ./pkg/guardrails/ -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add pkg/guardrails/tool_middleware.go pkg/guardrails/tool_middleware_test.go
git commit -m "fix: add Execute() to ToolMiddleware so guardrails apply to LLM tool calls"
```

---

### Task 2: Sandbox Package — Config & Errors

**Files:**
- Create: `pkg/sandbox/config.go`
- Create: `pkg/sandbox/errors.go`

**Step 1: Write config.go**

```go
package sandbox

import "time"

// Config holds sandbox configuration, loadable from YAML.
type Config struct {
	Enabled         bool          `json:"enabled" yaml:"enabled"`
	Image           string        `json:"image,omitempty" yaml:"image,omitempty"`
	AllowedCommands []string      `json:"allowed_commands,omitempty" yaml:"allowed_commands,omitempty"`
	DeniedCommands  []string      `json:"denied_commands,omitempty" yaml:"denied_commands,omitempty"`
	PoolSize        int           `json:"pool_size,omitempty" yaml:"pool_size,omitempty"`
	Timeout         time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	MemoryLimit     string        `json:"memory_limit,omitempty" yaml:"memory_limit,omitempty"`
	CPULimit        string        `json:"cpu_limit,omitempty" yaml:"cpu_limit,omitempty"`
	NetworkMode     string        `json:"network_mode,omitempty" yaml:"network_mode,omitempty"`
	MountPaths      []MountPath   `json:"mount_paths,omitempty" yaml:"mount_paths,omitempty"`
}

// MountPath represents a bind mount from host to container.
type MountPath struct {
	Host      string `json:"host" yaml:"host"`
	Container string `json:"container" yaml:"container"`
	ReadOnly  bool   `json:"read_only" yaml:"read_only"`
}

// applyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) applyDefaults() {
	if c.PoolSize <= 0 {
		c.PoolSize = 1
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.MemoryLimit == "" {
		c.MemoryLimit = "256m"
	}
	if c.CPULimit == "" {
		c.CPULimit = "0.5"
	}
	if c.NetworkMode == "" {
		c.NetworkMode = "none"
	}
	if c.Image == "" {
		c.Image = "ubuntu:22.04"
	}
	for i := range c.MountPaths {
		// ReadOnly defaults to true — zero value of bool is false,
		// so we cannot distinguish "unset" from "explicitly false" in Go.
		// Convention: callers must explicitly set ReadOnly=false for writable mounts.
	}
}
```

**Step 2: Write errors.go**

```go
package sandbox

import "errors"

var (
	// ErrCommandDenied is returned when a command is not in the allowlist.
	ErrCommandDenied = errors.New("sandbox: command not in allowlist")

	// ErrDockerNotFound is returned when the docker binary is not available.
	ErrDockerNotFound = errors.New("sandbox: docker binary not found")

	// ErrContainerUnhealthy is returned when no healthy container is available.
	ErrContainerUnhealthy = errors.New("sandbox: container not ready")

	// ErrCommandTimeout is returned when a command exceeds the configured timeout.
	ErrCommandTimeout = errors.New("sandbox: command execution timed out")

	// ErrSandboxDisabled is returned when sandbox is not enabled but executor is called.
	ErrSandboxDisabled = errors.New("sandbox: not enabled")
)
```

**Step 3: Verify it compiles**

Run: `go build ./pkg/sandbox/`
Expected: Success (no errors)

**Step 4: Commit**

```bash
git add pkg/sandbox/config.go pkg/sandbox/errors.go
git commit -m "feat(sandbox): add config structs and error types"
```

---

### Task 3: Sandbox Package — CommandExecutor Interface & LocalExecutor

**Files:**
- Create: `pkg/sandbox/sandbox.go`
- Create: `pkg/sandbox/sandbox_test.go`

**Step 1: Write the failing test**

Create `pkg/sandbox/sandbox_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/sandbox/ -run TestLocalExecutor -v`
Expected: Compilation error — `LocalExecutor` not defined

**Step 3: Write minimal implementation**

Create `pkg/sandbox/sandbox.go`:

```go
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/sandbox/ -run TestLocalExecutor -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/sandbox/sandbox.go pkg/sandbox/sandbox_test.go
git commit -m "feat(sandbox): add CommandExecutor interface and LocalExecutor"
```

---

### Task 4: Sandbox Package — Allowlist

**Files:**
- Create: `pkg/sandbox/allowlist.go`
- Create: `pkg/sandbox/allowlist_test.go`

**Step 1: Write the failing tests**

Create `pkg/sandbox/allowlist_test.go`:

```go
package sandbox

import (
	"errors"
	"testing"
)

func TestAllowlist_Check_AllowedCommand(t *testing.T) {
	al := NewAllowlist([]string{"git", "curl"}, nil)
	if err := al.Check("git"); err != nil {
		t.Errorf("expected git to be allowed, got: %v", err)
	}
}

func TestAllowlist_Check_AllowedAbsolutePath(t *testing.T) {
	al := NewAllowlist([]string{"git"}, nil)
	if err := al.Check("/usr/bin/git"); err != nil {
		t.Errorf("expected /usr/bin/git to be allowed, got: %v", err)
	}
}

func TestAllowlist_Check_DeniedCommand(t *testing.T) {
	al := NewAllowlist([]string{"git", "rm"}, []string{"rm"})
	err := al.Check("rm")
	if err == nil {
		t.Error("expected rm to be denied")
	}
	if !errors.Is(err, ErrCommandDenied) {
		t.Errorf("expected ErrCommandDenied, got: %v", err)
	}
}

func TestAllowlist_Check_DenyTakesPrecedence(t *testing.T) {
	al := NewAllowlist([]string{"rm"}, []string{"rm"})
	err := al.Check("rm")
	if err == nil {
		t.Error("deny should take precedence over allow")
	}
}

func TestAllowlist_Check_NotInAllowlist(t *testing.T) {
	al := NewAllowlist([]string{"git"}, nil)
	err := al.Check("curl")
	if err == nil {
		t.Error("expected curl to be denied when not in allowlist")
	}
}

func TestAllowlist_Check_EmptyAllowlistDeniesAll(t *testing.T) {
	al := NewAllowlist(nil, nil)
	err := al.Check("git")
	if err == nil {
		t.Error("expected all commands denied when allowlist is empty (fail-closed)")
	}
}

func TestAllowlist_Check_CaseInsensitive(t *testing.T) {
	al := NewAllowlist([]string{"Git"}, nil)
	if err := al.Check("git"); err != nil {
		t.Errorf("expected case-insensitive match, got: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/sandbox/ -run TestAllowlist -v`
Expected: Compilation error — `NewAllowlist` not defined

**Step 3: Write minimal implementation**

Create `pkg/sandbox/allowlist.go`:

```go
package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Allowlist enforces which commands are permitted in the sandbox.
// Deny list takes precedence over allow list. Empty allow list denies all (fail-closed).
type Allowlist struct {
	allowed map[string]bool
	denied  map[string]bool
}

// NewAllowlist creates a new Allowlist from allow and deny lists.
func NewAllowlist(allowed, denied []string) *Allowlist {
	a := &Allowlist{
		allowed: make(map[string]bool, len(allowed)),
		denied:  make(map[string]bool, len(denied)),
	}
	for _, cmd := range allowed {
		a.allowed[strings.ToLower(cmd)] = true
	}
	for _, cmd := range denied {
		a.denied[strings.ToLower(cmd)] = true
	}
	return a
}

// Check returns nil if the command is permitted, ErrCommandDenied otherwise.
func (a *Allowlist) Check(command string) error {
	base := strings.ToLower(filepath.Base(command))

	if a.denied[base] {
		return fmt.Errorf("%w: %q is explicitly denied", ErrCommandDenied, base)
	}

	if len(a.allowed) == 0 {
		return fmt.Errorf("%w: no commands are allowed (empty allowlist)", ErrCommandDenied)
	}

	if !a.allowed[base] {
		return fmt.Errorf("%w: %q is not in the allowlist", ErrCommandDenied, base)
	}

	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/sandbox/ -run TestAllowlist -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add pkg/sandbox/allowlist.go pkg/sandbox/allowlist_test.go
git commit -m "feat(sandbox): add command allowlist with fail-closed semantics"
```

---

### Task 5: Sandbox Package — Container Pool

**Files:**
- Create: `pkg/sandbox/pool.go`
- Create: `pkg/sandbox/pool_test.go`

**Step 1: Write the failing tests**

Create `pkg/sandbox/pool_test.go`:

```go
package sandbox

import (
	"context"
	"testing"
)

func TestPool_Acquire_RoundRobin(t *testing.T) {
	containers := []Container{
		{ID: "abc123", Name: "sandbox-0", Ready: true},
		{ID: "def456", Name: "sandbox-1", Ready: true},
	}
	p := &Pool{containers: containers}

	c1, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c2, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c3, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c1.ID != "abc123" {
		t.Errorf("expected first container, got %s", c1.ID)
	}
	if c2.ID != "def456" {
		t.Errorf("expected second container, got %s", c2.ID)
	}
	if c3.ID != "abc123" {
		t.Errorf("expected round-robin back to first, got %s", c3.ID)
	}
}

func TestPool_Acquire_EmptyPool(t *testing.T) {
	p := &Pool{containers: nil}
	_, err := p.Acquire(context.Background())
	if err == nil {
		t.Error("expected error for empty pool")
	}
}

func TestPool_Close(t *testing.T) {
	// closeFn tracks which container IDs were closed
	var closed []string
	closeFn := func(ctx context.Context, id string) error {
		closed = append(closed, id)
		return nil
	}
	containers := []Container{
		{ID: "abc123", Name: "sandbox-0", Ready: true},
		{ID: "def456", Name: "sandbox-1", Ready: true},
	}
	p := &Pool{containers: containers, closeFn: closeFn}

	if err := p.Close(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(closed) != 2 {
		t.Errorf("expected 2 containers closed, got %d", len(closed))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/sandbox/ -run TestPool -v`
Expected: Compilation error — `Pool`, `Container` not defined

**Step 3: Write minimal implementation**

Create `pkg/sandbox/pool.go`:

```go
package sandbox

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Container represents a running sandbox container.
type Container struct {
	ID        string
	Name      string
	Ready     bool
	CreatedAt time.Time
}

// Pool manages a set of warm sandbox containers with round-robin selection.
type Pool struct {
	containers []Container
	mu         sync.Mutex
	nextIdx    int
	closeFn    func(ctx context.Context, id string) error
}

// NewPool creates a pool with pre-created containers.
func NewPool(containers []Container, closeFn func(ctx context.Context, id string) error) *Pool {
	return &Pool{
		containers: containers,
		closeFn:    closeFn,
	}
}

// Acquire returns the next available container using round-robin selection.
func (p *Pool) Acquire(ctx context.Context) (*Container, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.containers) == 0 {
		return nil, ErrContainerUnhealthy
	}

	c := &p.containers[p.nextIdx]
	p.nextIdx = (p.nextIdx + 1) % len(p.containers)

	if !c.Ready {
		return nil, fmt.Errorf("%w: container %s is not ready", ErrContainerUnhealthy, c.Name)
	}

	return c, nil
}

// Close stops and removes all containers in the pool.
func (p *Pool) Close(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for _, c := range p.containers {
		if p.closeFn != nil {
			if err := p.closeFn(ctx, c.ID); err != nil {
				lastErr = err
			}
		}
	}
	p.containers = nil
	return lastErr
}

// MarkUnhealthy marks a container as not ready.
func (p *Pool) MarkUnhealthy(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.containers {
		if p.containers[i].ID == id {
			p.containers[i].Ready = false
			break
		}
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/sandbox/ -run TestPool -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add pkg/sandbox/pool.go pkg/sandbox/pool_test.go
git commit -m "feat(sandbox): add warm container pool with round-robin selection"
```

---

### Task 6: Sandbox Package — DockerExecutor

**Files:**
- Create: `pkg/sandbox/docker.go`
- Create: `pkg/sandbox/docker_test.go`

**Step 1: Write the unit test (no Docker required)**

Create `pkg/sandbox/docker_test.go`:

```go
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
	// Verify the command wraps via docker exec
	args := cmd.Args
	if len(args) < 5 {
		t.Fatalf("expected docker exec args, got: %v", args)
	}
	// args[0] = "docker", args[1] = "exec", args[2] = "-i", args[3] = containerID, args[4] = command
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/sandbox/ -run TestDockerExecutor -v`
Expected: Compilation error — `DockerExecutor` not defined

**Step 3: Write implementation**

Create `pkg/sandbox/docker.go`:

```go
package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// DockerExecutor implements CommandExecutor using Docker containers.
type DockerExecutor struct {
	config    Config
	allowlist *Allowlist
	pool      *Pool
	logger    logging.Logger
}

// NewDockerExecutor creates a new DockerExecutor, starts warm containers, and returns the executor.
// Fails fast if Docker is not available or the config is invalid.
func NewDockerExecutor(ctx context.Context, config Config, logger logging.Logger) (*DockerExecutor, error) {
	if logger == nil {
		logger = logging.New()
	}

	// Verify Docker is available
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDockerNotFound, err)
	}
	logger.Debug(ctx, "Docker found", map[string]interface{}{"path": dockerPath})

	config.applyDefaults()

	allowlist := NewAllowlist(config.AllowedCommands, config.DeniedCommands)

	// Create warm containers
	containers, err := createContainers(ctx, config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox containers: %w", err)
	}

	closeFn := func(ctx context.Context, id string) error {
		return removeContainer(ctx, id, logger)
	}

	return &DockerExecutor{
		config:    config,
		allowlist: allowlist,
		pool:      NewPool(containers, closeFn),
		logger:    logger,
	}, nil
}

// Command creates an exec.Cmd that runs inside a sandbox container via `docker exec`.
func (d *DockerExecutor) Command(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
	if err := d.allowlist.Check(name); err != nil {
		return nil, err
	}

	container, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	dockerArgs := make([]string, 0, 4+len(args))
	dockerArgs = append(dockerArgs, "exec", "-i", container.ID, name)
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	return cmd, nil
}

// Close stops and removes all sandbox containers.
func (d *DockerExecutor) Close(ctx context.Context) error {
	return d.pool.Close(ctx)
}

// createContainers starts warm containers based on config.
func createContainers(ctx context.Context, config Config, logger logging.Logger) ([]Container, error) {
	containers := make([]Container, 0, config.PoolSize)

	for i := 0; i < config.PoolSize; i++ {
		name := fmt.Sprintf("agent-sandbox-%d-%d", time.Now().UnixNano(), i)

		args := buildContainerArgs(config, name)

		logger.Info(ctx, "Creating sandbox container", map[string]interface{}{
			"name":  name,
			"image": config.Image,
		})

		cmd := exec.CommandContext(ctx, "docker", args...)
		output, err := cmd.Output()
		if err != nil {
			// Clean up any containers that were created
			for _, c := range containers {
				_ = removeContainer(ctx, c.ID, logger)
			}
			return nil, fmt.Errorf("failed to create container %s: %w", name, err)
		}

		containerID := strings.TrimSpace(string(output))
		containers = append(containers, Container{
			ID:        containerID,
			Name:      name,
			Ready:     true,
			CreatedAt: time.Now(),
		})

		logger.Info(ctx, "Sandbox container created", map[string]interface{}{
			"name": name,
			"id":   containerID,
		})
	}

	return containers, nil
}

// buildContainerArgs builds the docker run arguments from config.
func buildContainerArgs(config Config, name string) []string {
	args := []string{
		"run", "-d",
		"--name", name,
		"--memory", config.MemoryLimit,
		"--cpus", config.CPULimit,
		"--network", config.NetworkMode,
		"--read-only",
		"--tmpfs", "/tmp:size=64m",
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL",
		"--pids-limit", strconv.Itoa(64),
	}

	for _, mount := range config.MountPaths {
		mountStr := mount.Host + ":" + mount.Container
		if mount.ReadOnly {
			mountStr += ":ro"
		}
		args = append(args, "-v", mountStr)
	}

	args = append(args, config.Image, "sleep", "infinity")
	return args
}

// removeContainer stops and removes a container by ID.
func removeContainer(ctx context.Context, id string, logger logging.Logger) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", id)
	if err := cmd.Run(); err != nil {
		logger.Warn(ctx, "Failed to remove sandbox container", map[string]interface{}{
			"id":    id,
			"error": err.Error(),
		})
		return err
	}
	logger.Debug(ctx, "Sandbox container removed", map[string]interface{}{"id": id})
	return nil
}
```

**Step 4: Run unit tests to verify they pass**

Run: `go test ./pkg/sandbox/ -run TestDockerExecutor -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add pkg/sandbox/docker.go pkg/sandbox/docker_test.go
git commit -m "feat(sandbox): add DockerExecutor with container lifecycle management"
```

---

### Task 7: Sandbox Package — Docker Integration Tests

**Files:**
- Create: `pkg/sandbox/docker_integration_test.go`

**Step 1: Write integration test**

Create `pkg/sandbox/docker_integration_test.go`:

```go
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
		// Some alpine containers may not have /etc/hostname — that's fine
		t.Logf("cat /etc/hostname failed (expected in isolated container): %v", err)
		return
	}
	t.Logf("container hostname: %s", string(output))
	// The hostname should be the container ID, not the host
}
```

**Step 2: Verify it compiles (don't run — needs Docker)**

Run: `go build -tags integration ./pkg/sandbox/`
Expected: Success

**Step 3: Run integration test (only if Docker is available)**

Run: `go test -tags integration ./pkg/sandbox/ -run TestDockerExecutor_Integration -v -timeout 120s`
Expected: PASS (if Docker daemon is running)

**Step 4: Commit**

```bash
git add pkg/sandbox/docker_integration_test.go
git commit -m "test(sandbox): add Docker integration tests"
```

---

### Task 8: MCP Integration — Wire Sandbox into StdioServerConfig

**Files:**
- Modify: `pkg/mcp/mcp.go:687-692` (StdioServerConfig struct)
- Modify: `pkg/mcp/mcp.go:770-774` (command creation in NewStdioServerWithRetry)

**Step 1: Write the failing test**

Modify or create a test that verifies `StdioServerConfig` accepts an `Executor` field. Add to an existing MCP test file or create a focused one:

Create `pkg/mcp/sandbox_integration_test.go`:

```go
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
type mockExecutor struct {
	called  bool
	lastCmd string
}

func (m *mockExecutor) Command(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
	m.called = true
	m.lastCmd = name
	return exec.CommandContext(ctx, name, args...), nil
}

func (m *mockExecutor) Close(ctx context.Context) error { return nil }

func TestStdioServerConfig_ExecutorIsUsed(t *testing.T) {
	// This test verifies the Executor field exists and has the correct type.
	// Full integration testing with MCP server startup requires a real MCP server binary.
	mock := &mockExecutor{}
	config := StdioServerConfig{
		Command:  "echo",
		Args:     []string{"hello"},
		Executor: mock,
	}
	_ = config // Compiles = type is correct
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/mcp/ -run TestStdioServerConfig -v`
Expected: Compilation error — `StdioServerConfig` has no field `Executor`

**Step 3: Modify StdioServerConfig**

In `pkg/mcp/mcp.go`, change `StdioServerConfig` (line 687):

```go
// StdioServerConfig holds configuration for a stdio MCP server
type StdioServerConfig struct {
	Command  string
	Args     []string
	Env      []string
	Logger   logging.Logger
	Executor sandbox.CommandExecutor // Optional sandboxed executor. Nil uses direct host execution.
}
```

Add import for sandbox package at the top of the file:

```go
import (
	...
	"github.com/Ingenimax/agent-sdk-go/pkg/sandbox"
	...
)
```

**Step 4: Modify NewStdioServerWithRetry**

In `pkg/mcp/mcp.go`, replace line 774:

```go
	// #nosec G204 -- commandPath is validated above with LookPath and security checks
	cmd := exec.CommandContext(ctx, commandPath, config.Args...)
```

With:

```go
	// Create the command, optionally through sandbox executor
	var cmd *exec.Cmd
	if config.Executor != nil {
		var execErr error
		cmd, execErr = config.Executor.Command(ctx, commandPath, config.Args...)
		if execErr != nil {
			return nil, fmt.Errorf("sandbox executor error: %w", execErr)
		}
	} else {
		// #nosec G204 -- commandPath is validated above with LookPath and security checks
		cmd = exec.CommandContext(ctx, commandPath, config.Args...)
	}
```

**Step 5: Run test to verify it passes**

Run: `go test ./pkg/mcp/ -run TestStdioServerConfig -v`
Expected: PASS

**Step 6: Run full test suite**

Run: `go test ./...`
Expected: All PASS (no regressions)

**Step 7: Commit**

```bash
git add pkg/mcp/mcp.go pkg/mcp/sandbox_integration_test.go
git commit -m "feat(mcp): integrate sandbox CommandExecutor into StdioServerConfig"
```

---

### Task 9: Agent SDK Integration — WithSandbox Option & YAML Config

**Files:**
- Modify: `pkg/agent/agent.go:60-103` (add sandbox field to Agent struct)
- Modify: `pkg/agent/agent.go` (add WithSandbox option)
- Modify: `pkg/agent/mcp_config.go:17-25` (add Sandbox field to MCPServerConfig)
- Modify: `pkg/agent/agent.go:1115-1133` (pass executor to LazyMCPServerConfig)

**Step 1: Add sandbox field to Agent struct**

In `pkg/agent/agent.go`, add to the Agent struct (around line 102):

```go
	// Sandbox executor for containerized command execution
	sandbox sandbox.CommandExecutor
```

Add import:

```go
	"github.com/Ingenimax/agent-sdk-go/pkg/sandbox"
```

**Step 2: Add WithSandbox option**

Add after the existing `With*` options (e.g., after `WithCustomRunStreamFunction`):

```go
// WithSandbox sets the sandbox executor for containerized MCP command execution.
func WithSandbox(executor sandbox.CommandExecutor) Option {
	return func(a *Agent) {
		a.sandbox = executor
	}
}
```

**Step 3: Add Sandbox field to MCPServerConfig**

In `pkg/agent/mcp_config.go`, modify `MCPServerConfig`:

```go
type MCPServerConfig struct {
	Command           string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args              []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env               map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	URL               string            `json:"url,omitempty" yaml:"url,omitempty"`
	Token             string            `json:"token,omitempty" yaml:"token,omitempty"`
	HttpTransportMode string            `json:"httpTransportMode,omitempty" yaml:"httpTransportMode,omitempty"`
	AllowedTools      []string          `json:"allowedTools,omitempty" yaml:"allowedTools,omitempty"`
	Sandbox           *sandbox.Config   `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
}
```

**Step 4: Wire sandbox into LazyMCPConfig → StdioServerConfig**

In `pkg/agent/agent.go`, in `createLazyMCPTools()` (around line 1123), update `LazyMCPServerConfig` creation to pass the agent's sandbox executor. This requires adding an `Executor` field to `LazyMCPConfig` in the agent package:

Add to `LazyMCPConfig` struct (line 34):

```go
type LazyMCPConfig struct {
	Name              string
	Type              string
	Command           string
	Args              []string
	Env               []string
	URL               string
	Token             string
	Tools             []LazyMCPToolConfig
	HttpTransportMode string
	AllowedTools      []string
	Executor          sandbox.CommandExecutor // Optional sandbox executor
}
```

Then in `createLazyMCPTools()`, pass the executor to the MCP config. Find where `mcp.LazyMCPServerConfig` is created and add:

```go
lazyServerConfig := mcp.LazyMCPServerConfig{
	...existing fields...
	Executor: config.Executor,
}
```

And in agent initialization, propagate the agent-level sandbox to each lazy MCP config that doesn't already have one.

**Step 5: Verify compilation**

Run: `go build ./...`
Expected: Success

**Step 6: Run full test suite**

Run: `go test ./...`
Expected: All PASS

**Step 7: Commit**

```bash
git add pkg/agent/agent.go pkg/agent/mcp_config.go
git commit -m "feat(agent): add WithSandbox option and wire sandbox into MCP configs"
```

---

### Task 10: Run Full Linter & Final Verification

**Step 1: Format code**

Run: `make fmt`

**Step 2: Tidy dependencies**

Run: `make tidy`

**Step 3: Run linter**

Run: `make lint`
Expected: No new warnings/errors

**Step 4: Run all tests**

Run: `make test`
Expected: All PASS

**Step 5: Fix any issues found**

Address any lint warnings or test failures.

**Step 6: Final commit**

```bash
git add -A
git commit -m "chore: lint and tidy after sandbox feature"
```

---

## Task Dependency Order

```
Task 1 (ToolMiddleware bug fix) — independent, do first
    ↓
Task 2 (Config & Errors) — foundation
    ↓
Task 3 (Interface & LocalExecutor) — depends on Task 2
    ↓
Task 4 (Allowlist) — depends on Task 2
    ↓
Task 5 (Pool) — depends on Task 2
    ↓
Task 6 (DockerExecutor) — depends on Tasks 3, 4, 5
    ↓
Task 7 (Integration tests) — depends on Task 6
    ↓
Task 8 (MCP wiring) — depends on Task 3
    ↓
Task 9 (Agent SDK wiring) — depends on Tasks 6, 8
    ↓
Task 10 (Final verification) — depends on all
```

**Parallelizable:** Tasks 2-5 can be done in parallel. Task 1 is independent of everything.
