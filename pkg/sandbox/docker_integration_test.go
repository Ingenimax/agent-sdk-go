//go:build integration

package sandbox

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// These tests need a running Docker daemon and pull alpine:3.20.
// Run them with: go test -tags integration ./pkg/sandbox/

func newIntegrationExecutor(t *testing.T, cfg Config) *DockerExecutor {
	t.Helper()
	if cfg.Image == "" {
		cfg.Image = "alpine:3.20"
	}
	d, err := NewDockerExecutor(cfg, nil)
	if err != nil {
		t.Fatalf("NewDockerExecutor: %v", err)
	}
	t.Cleanup(func() { _ = d.Close(context.Background()) })
	return d
}

func runInSandbox(t *testing.T, d *DockerExecutor, name string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd, err := d.Command(ctx, name, args, nil)
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func TestIntegrationContainerIsLockedDown(t *testing.T) {
	d := newIntegrationExecutor(t, Config{AllowedCommands: []string{"sh"}})

	checks := []struct {
		name   string
		script string
		want   string
	}{
		{"root filesystem is read-only", "touch /etc/x 2>/dev/null && echo writable || echo read-only", "read-only"},
		{"/tmp is writable", "touch /tmp/x && echo ok", "ok"},
		{"/tmp is not executable", "cp /bin/busybox /tmp/true && /tmp/true 2>/dev/null && echo exec || echo noexec", "noexec"},
		{"no network", "wget -q -T 3 -O /dev/null http://1.1.1.1 2>/dev/null && echo online || echo offline", "offline"},
		{"capabilities dropped", "grep CapEff /proc/self/status | tr -d '\\t'", "CapEff:0000000000000000"},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			out, err := runInSandbox(t, d, "sh", "-c", c.script)
			if err != nil {
				t.Fatalf("run: %v: %s", err, out)
			}
			if !strings.HasSuffix(out, c.want) {
				t.Fatalf("got %q, want %q", out, c.want)
			}
		})
	}
}

func TestIntegrationEnvironmentIsPassedByName(t *testing.T) {
	t.Setenv("SANDBOX_INTEGRATION_HOST_ONLY", "leaked")
	d := newIntegrationExecutor(t, Config{AllowedCommands: []string{"sh"}})

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd, err := d.Command(ctx, "sh", []string{"-c", `echo "token=$API_TOKEN host=${SANDBOX_INTEGRATION_HOST_ONLY:-unset}"`}, []string{"API_TOKEN=s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "token=s3cret host=unset" {
		t.Fatalf("got %q", got)
	}
}

// TestIntegrationContainerGoesAwayWhenInputCloses checks the cleanup path an
// MCP server relies on: when its input closes, the container is removed.
func TestIntegrationContainerGoesAwayWhenInputCloses(t *testing.T) {
	d := newIntegrationExecutor(t, Config{AllowedCommands: []string{"cat"}})

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd, err := d.Command(ctx, "cat", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	assertNoContainers(t, d)
}

func TestIntegrationCancelRemovesContainer(t *testing.T) {
	d := newIntegrationExecutor(t, Config{AllowedCommands: []string{"sleep"}})

	ctx, cancel := context.WithCancel(t.Context())
	cmd, err := d.Command(ctx, "sleep", []string{"300"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitForContainers(t, d, 1)
	cancel()
	_ = cmd.Wait()
	assertNoContainers(t, d)
}

func TestIntegrationCloseRemovesRunningContainers(t *testing.T) {
	d := newIntegrationExecutor(t, Config{AllowedCommands: []string{"sleep"}})

	cmd, err := d.Command(t.Context(), "sleep", []string{"300"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitForContainers(t, d, 1)
	if err := d.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_ = cmd.Wait()
	assertNoContainers(t, d)
}

func listContainers(t *testing.T, d *DockerExecutor) []string {
	t.Helper()
	out, err := exec.Command(d.dockerPath, "ps", "--all", "--quiet", "--filter", "label="+LabelKey+"="+d.ID()).Output() // #nosec G204 -- test
	if err != nil {
		t.Fatalf("docker ps: %v", err)
	}
	return strings.Fields(string(out))
}

func waitForContainers(t *testing.T, d *DockerExecutor, n int) {
	t.Helper()
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if len(listContainers(t, d)) >= n {
			return
		}
	}
	t.Fatalf("container did not start")
}

func assertNoContainers(t *testing.T, d *DockerExecutor) {
	t.Helper()
	var left []string
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if left = listContainers(t, d); len(left) == 0 {
			return
		}
	}
	t.Fatalf("containers left behind: %v", left)
}

// TestIntegrationAllowExecInTmp is the control for the noexec check above.
func TestIntegrationAllowExecInTmp(t *testing.T) {
	d := newIntegrationExecutor(t, Config{AllowedCommands: []string{"sh"}, AllowExecInTmp: true})
	out, err := runInSandbox(t, d, "sh", "-c", "cp /bin/busybox /tmp/true && /tmp/true && echo exec")
	if err != nil {
		t.Fatalf("run: %v: %s", err, out)
	}
	if out != "exec" {
		t.Fatalf("got %q, want exec", out)
	}
}

// TestIntegrationKilledCLIStillRemovesContainer kills the docker CLI outright,
// as when the host process dies, without the executor's cleanup running.
// Docker then closes the server's input, the server exits, and --rm removes
// the container.
func TestIntegrationKilledCLIStillRemovesContainer(t *testing.T) {
	d := newIntegrationExecutor(t, Config{AllowedCommands: []string{"cat"}})

	cmd, err := d.Command(context.Background(), "cat", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close() }()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitForContainers(t, d, 1)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	assertNoContainers(t, d)
}
