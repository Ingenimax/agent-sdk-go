package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
)

// The test binary plays three roles, chosen by roleEnv: the test itself, a
// fake docker CLI, and an MCP server that the fake docker "runs".
const (
	roleEnv      = "SANDBOX_TEST_ROLE"
	fakeLogEnv   = "SANDBOX_TEST_DOCKER_LOG"
	fakePSEnv    = "SANDBOX_TEST_DOCKER_PS"
	hostOnlyEnv  = "SANDBOX_TEST_HOST_ONLY"
	mcpServerCmd = "fake-mcp-server"
	sleeperCmd   = "sleeper"
)

func TestMain(m *testing.M) {
	switch os.Getenv(roleEnv) {
	case "docker":
		os.Exit(fakeDocker(os.Args[1:]))
	case "mcp-server":
		os.Exit(fakeMCPServer())
	}
	os.Exit(m.Run())
}

// fakeDocker logs its arguments and imitates the parts of docker that the
// executor uses. For "run" it starts the entrypoint with only the variables
// named by --env, the way docker does, so tests can see what reaches a server.
func fakeDocker(args []string) int {
	if f, err := os.OpenFile(os.Getenv(fakeLogEnv), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		line, _ := json.Marshal(args)
		_, _ = fmt.Fprintln(f, string(line))
		_ = f.Close()
	}
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "ps":
		fmt.Print(os.Getenv(fakePSEnv))
		return 0
	case "rm":
		return 0
	case "run":
	default:
		return 2
	}

	boolFlags := map[string]bool{"--rm": true, "--interactive": true, "--init": true, "--read-only": true}
	var entrypoint string
	var env []string
	rest := args[1:]
	for len(rest) > 0 && strings.HasPrefix(rest[0], "--") {
		flag := rest[0]
		if boolFlags[flag] {
			rest = rest[1:]
			continue
		}
		value := rest[1]
		rest = rest[2:]
		switch flag {
		case "--entrypoint":
			entrypoint = value
		case "--env":
			if strings.Contains(value, "=") {
				env = append(env, value)
			} else if v, ok := os.LookupEnv(value); ok {
				env = append(env, value+"="+v)
			}
		}
	}
	serverArgs := rest[1:] // rest[0] is the image

	switch entrypoint {
	case sleeperCmd:
		time.Sleep(time.Minute)
		return 0
	case mcpServerCmd:
		self, err := os.Executable()
		if err != nil {
			return 1
		}
		// #nosec G204 -- test helper re-running the test binary.
		child := exec.Command(self, serverArgs...)
		child.Env = append(env, roleEnv+"=mcp-server")
		child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := child.Run(); err != nil {
			return 1
		}
		return 0
	}
	return 127
}

// fakeMCPServer serves one tool, getenv, which reports a variable's value.
func fakeMCPServer() int {
	s := gomcp.NewServer(&gomcp.Implementation{Name: "sandboxed", Version: "1.0.0"}, nil)
	s.AddTool(&gomcp.Tool{
		Name:        "getenv",
		Description: "Return an environment variable",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"name": map[string]any{"type": "string"}},
		},
	}, func(_ context.Context, req *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		var in struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &in); err != nil {
			return nil, err
		}
		v, ok := os.LookupEnv(in.Name)
		if !ok {
			v = "(unset)"
		}
		return &gomcp.CallToolResult{Content: []gomcp.Content{&gomcp.TextContent{Text: "value=" + v}}}, nil
	})
	if err := s.Run(context.Background(), &gomcp.StdioTransport{}); err != nil {
		return 1
	}
	return 0
}

// recordingLogger keeps warnings so tests can check them.
type recordingLogger struct {
	mu    sync.Mutex
	warns []string
}

func (l *recordingLogger) Info(context.Context, string, map[string]interface{})  {}
func (l *recordingLogger) Error(context.Context, string, map[string]interface{}) {}
func (l *recordingLogger) Debug(context.Context, string, map[string]interface{}) {}
func (l *recordingLogger) Warn(_ context.Context, msg string, _ map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
}

func (l *recordingLogger) warnings() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.warns)
}

// newFakeExecutor returns an executor whose docker CLI is the fake, and a
// function that returns each docker invocation's arguments.
func newFakeExecutor(t *testing.T, cfg Config) (*DockerExecutor, func() [][]string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "docker.log")
	t.Setenv(roleEnv, "docker")
	t.Setenv(fakeLogEnv, logPath)

	cfg.DockerPath = self
	d, err := NewDockerExecutor(cfg, &recordingLogger{})
	if err != nil {
		t.Fatalf("NewDockerExecutor: %v", err)
	}

	calls := func() [][]string {
		data, err := os.ReadFile(logPath) // #nosec G304 -- test temp file
		if err != nil {
			return nil
		}
		var out [][]string
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			var args []string
			if err := json.Unmarshal([]byte(line), &args); err != nil {
				t.Fatalf("bad log line %q: %v", line, err)
			}
			out = append(out, args)
		}
		return out
	}
	return d, calls
}

func TestNewDockerExecutorRejectsBadConfig(t *testing.T) {
	ok := Config{Image: "alpine:3.20", AllowedCommands: []string{"echo"}, DockerPath: "/usr/bin/docker"}
	tests := []struct {
		name   string
		modify func(*Config)
	}{
		{"no image", func(c *Config) { c.Image = " " }},
		{"image looks like a flag", func(c *Config) { c.Image = "--privileged" }},
		{"host network", func(c *Config) { c.NetworkMode = "host" }},
		{"negative pids limit", func(c *Config) { c.PidsLimit = -1 }},
		{"empty allowed command", func(c *Config) { c.AllowedCommands = []string{""} }},
		{"relative command path", func(c *Config) { c.AllowedCommands = []string{"bin/tool"} }},
		{"relative denied path", func(c *Config) { c.DeniedCommands = []string{"./sh"} }},
		{"relative host mount", func(c *Config) { c.MountPaths = []MountPath{{Host: "data", Container: "/data"}} }},
		{"relative container mount", func(c *Config) { c.MountPaths = []MountPath{{Host: "/data", Container: "data"}} }},
		{"windows container mount", func(c *Config) { c.MountPaths = []MountPath{{Host: "/data", Container: `C:\data`}} }},
		{"comma in mount", func(c *Config) { c.MountPaths = []MountPath{{Host: "/data,readonly=false", Container: "/data"}} }},
		{"quote in mount", func(c *Config) { c.MountPaths = []MountPath{{Host: "/data", Container: `/da"ta`}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ok
			tt.modify(&cfg)
			_, err := NewDockerExecutor(cfg, &recordingLogger{})
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("NewDockerExecutor = %v, want ErrInvalidConfig", err)
			}
		})
	}

	t.Run("windows host mount is accepted", func(t *testing.T) {
		cfg := ok
		cfg.MountPaths = []MountPath{{Host: `C:\data`, Container: "/data"}}
		if _, err := NewDockerExecutor(cfg, &recordingLogger{}); err != nil {
			t.Fatalf("NewDockerExecutor = %v", err)
		}
	})
}

func TestNewDockerExecutorNeedsDocker(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := NewDockerExecutor(Config{Image: "alpine:3.20"}, nil)
	if !errors.Is(err, ErrDockerNotFound) {
		t.Fatalf("NewDockerExecutor = %v, want ErrDockerNotFound", err)
	}
}

func TestNewDockerExecutorWarnsWhenImageIsNotPinned(t *testing.T) {
	for _, tt := range []struct {
		image    string
		wantWarn bool
	}{
		{"alpine:3.20", true},
		{"alpine@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", false},
	} {
		logger := &recordingLogger{}
		if _, err := NewDockerExecutor(Config{Image: tt.image, DockerPath: "/usr/bin/docker"}, logger); err != nil {
			t.Fatal(err)
		}
		if got := len(logger.warnings()) > 0; got != tt.wantWarn {
			t.Errorf("image %q: warned = %v, want %v", tt.image, got, tt.wantWarn)
		}
	}
}

func TestRunArgs(t *testing.T) {
	d, err := NewDockerExecutor(Config{
		Image:           "mcp/time@sha256:abc",
		AllowedCommands: []string{"mcp-server-time"},
		NetworkMode:     "bridge",
		User:            "1000:1000",
		MountPaths: []MountPath{
			{Host: "/srv/docs", Container: "/docs"},
			{Host: "/srv/out", Container: "/out", Writable: true},
		},
		DockerPath: "/usr/bin/docker",
	}, &recordingLogger{})
	if err != nil {
		t.Fatal(err)
	}

	args, cliEnv := d.runArgs("agent-sdk-mcp-x", "mcp-server-time", []string{"--local-timezone", "UTC"},
		[]string{"API_TOKEN=s3cret", "HOME=/tmp", "DOCKER_HOST=tcp://inner:2375", "MALFORMED", "=nokey"})

	joined := strings.Join(args, " ")
	for _, want := range []string{
		"run --rm --interactive --init",
		"--name agent-sdk-mcp-x",
		"--label " + LabelKey + "=" + d.ID(),
		"--network bridge",
		"--memory " + DefaultMemoryLimit,
		"--cpus " + DefaultCPULimit,
		"--pids-limit 64",
		"--read-only",
		"--tmpfs /tmp:rw,nosuid,nodev,size=64m,noexec",
		"--cap-drop ALL",
		"--security-opt no-new-privileges",
		"--entrypoint mcp-server-time",
		"--user 1000:1000",
		"--mount type=bind,source=/srv/docs,target=/docs,readonly",
		"--mount type=bind,source=/srv/out,target=/out --",
		"--env API_TOKEN --",
		"--env HOME=/tmp",
		"--env DOCKER_HOST=tcp://inner:2375",
	} {
		if !strings.Contains(joined+" --", want) {
			t.Errorf("docker args missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "s3cret") {
		t.Errorf("secret value appears in the docker arguments: %s", joined)
	}
	if strings.Contains(joined, "MALFORMED") || strings.Contains(joined, "nokey") {
		t.Errorf("malformed variables were passed: %s", joined)
	}
	// The image comes after every flag, then the server's own arguments.
	if got := args[len(args)-3:]; !slices.Equal(got, []string{"mcp/time@sha256:abc", "--local-timezone", "UTC"}) {
		t.Errorf("args end with %q, want image then server args", got)
	}

	if !slices.Contains(cliEnv, "API_TOKEN=s3cret") {
		t.Error("secret value is not in the docker CLI's environment")
	}
	if slices.Contains(cliEnv, "HOME=/tmp") || slices.Contains(cliEnv, "DOCKER_HOST=tcp://inner:2375") {
		t.Error("variables the docker CLI reads were changed for the CLI itself")
	}
}

func TestRunArgsAllowExecInTmp(t *testing.T) {
	d, err := NewDockerExecutor(Config{Image: "node:22", AllowExecInTmp: true, TmpfsSize: "512m", DockerPath: "/usr/bin/docker"}, &recordingLogger{})
	if err != nil {
		t.Fatal(err)
	}
	args, _ := d.runArgs("n", "npx", nil, nil)
	if !slices.Contains(args, "/tmp:rw,nosuid,nodev,size=512m,exec") {
		t.Errorf("tmpfs is not executable: %v", args)
	}
}

func TestCommandRefusesDisallowedCommandsAndClosedExecutor(t *testing.T) {
	d, calls := newFakeExecutor(t, Config{Image: "alpine:3.20", AllowedCommands: []string{"echo"}})

	if _, err := d.Command(t.Context(), "sh", []string{"-c", "id"}, nil); !errors.Is(err, ErrCommandDenied) {
		t.Fatalf("Command(sh) = %v, want ErrCommandDenied", err)
	}
	if err := d.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := d.Command(t.Context(), "echo", nil, nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("Command after Close = %v, want ErrClosed", err)
	}
	for _, c := range calls() {
		if c[0] == "run" {
			t.Fatalf("a refused command reached docker: %v", c)
		}
	}
}

// TestServerRunsThroughExecutor starts a real MCP server through the
// executor, with the fake docker standing in for Docker.
func TestServerRunsThroughExecutor(t *testing.T) {
	d, calls := newFakeExecutor(t, Config{Image: "alpine:3.20", AllowedCommands: []string{mcpServerCmd}})
	t.Setenv(hostOnlyEnv, "leaked")

	// The command does not exist on the host; only the executor resolves it.
	server, err := mcp.NewStdioServer(t.Context(), mcp.StdioServerConfig{
		Command:  mcpServerCmd,
		Env:      []string{"API_TOKEN=s3cret"},
		Logger:   logging.New(),
		Executor: d,
	})
	if err != nil {
		t.Fatalf("NewStdioServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	tools, err := server.ListTools(t.Context())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "getenv" {
		t.Fatalf("tools = %+v, want getenv", tools)
	}

	for name, want := range map[string]string{
		"API_TOKEN": "value=s3cret",
		hostOnlyEnv: "value=(unset)", // the host's environment does not reach the server
	} {
		resp, err := server.CallTool(t.Context(), "getenv", map[string]any{"name": name})
		if err != nil {
			t.Fatalf("CallTool(%s): %v", name, err)
		}
		got, err := json.Marshal(resp.Content)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), want) {
			t.Errorf("getenv %s = %s, want %s", name, got, want)
		}
	}

	var run []string
	for _, c := range calls() {
		if c[0] == "run" {
			run = c
		}
	}
	if run == nil {
		t.Fatal("docker run was not called")
	}
	if strings.Contains(strings.Join(run, " "), "s3cret") {
		t.Errorf("secret value appears in docker run arguments: %v", run)
	}

	// Close finds this executor's containers by label and removes them.
	t.Setenv(fakePSEnv, "c1\nc2\n")
	if err := d.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	all := calls()
	if !slices.ContainsFunc(all, func(c []string) bool {
		return slices.Equal(c, []string{"ps", "--all", "--quiet", "--filter", "label=" + LabelKey + "=" + d.ID()})
	}) {
		t.Errorf("Close did not list containers by label: %v", all)
	}
	if !slices.ContainsFunc(all, func(c []string) bool { return slices.Equal(c, []string{"rm", "--force", "c1", "c2"}) }) {
		t.Errorf("Close did not remove the listed containers: %v", all)
	}
}

func TestCanceledContextRemovesContainer(t *testing.T) {
	d, calls := newFakeExecutor(t, Config{Image: "alpine:3.20", AllowedCommands: []string{sleeperCmd}})

	ctx, cancel := context.WithCancel(t.Context())
	cmd, err := d.Command(ctx, sleeperCmd, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Wait until the fake docker has started and logged its arguments.
	var name string
	for deadline := time.Now().Add(10 * time.Second); name == "" && time.Now().Before(deadline); {
		for _, c := range calls() {
			if c[0] == "run" {
				name = c[slices.Index(c, "--name")+1]
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if name == "" {
		t.Fatal("docker run was not called")
	}
	cancel()
	if err := cmd.Wait(); err == nil {
		t.Fatal("Wait = nil, want the canceled command to fail")
	}
	if !slices.ContainsFunc(calls(), func(c []string) bool { return slices.Equal(c, []string{"rm", "--force", name}) }) {
		t.Errorf("container %s was not removed after cancel: %v", name, calls())
	}
}

func TestCloseReportsDockerFailure(t *testing.T) {
	d, err := NewDockerExecutor(Config{Image: "alpine:3.20", DockerPath: filepath.Join(t.TempDir(), "missing-docker")}, &recordingLogger{})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(t.Context()); err == nil {
		t.Fatal("Close = nil, want an error when docker cannot run")
	}
}

func TestIsNoSuchContainer(t *testing.T) {
	if !isNoSuchContainer([]byte("Error response from daemon: No such container: abc")) {
		t.Error("did not recognize a missing container")
	}
	if isNoSuchContainer([]byte("permission denied")) {
		t.Error("treated another error as a missing container")
	}
}
