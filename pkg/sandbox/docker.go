package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
)

// LabelKey labels every container a DockerExecutor starts. Its value is the
// executor's ID, so Close removes only that executor's containers.
const LabelKey = "agent-sdk-go.sandbox"

// containerPrefix starts the name of every container a DockerExecutor starts.
const containerPrefix = "agent-sdk-mcp-"

// removeTimeout bounds a force-remove that runs after the caller's context
// is already done.
const removeTimeout = 15 * time.Second

// DockerExecutor runs each stdio MCP server in its own Docker container. It
// implements mcp.CommandExecutor. See the package documentation for what the
// container is allowed to do.
//
// It is safe for concurrent use. Call Close when you are done with it.
type DockerExecutor struct {
	cfg        Config
	dockerPath string
	id         string
	allowlist  *Allowlist
	logger     logging.Logger

	mu     sync.Mutex
	closed bool
}

var _ mcp.CommandExecutor = (*DockerExecutor)(nil)

// NewDockerExecutor checks cfg and returns an executor for it. It does not
// contact the Docker daemon or pull the image; that happens when the first
// server starts. A nil logger uses the default logger.
func NewDockerExecutor(cfg Config, logger logging.Logger) (*DockerExecutor, error) {
	if logger == nil {
		logger = logging.New()
	}
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	dockerPath := cfg.DockerPath
	if dockerPath == "" {
		found, err := exec.LookPath("docker")
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDockerNotFound, err)
		}
		dockerPath = found
	}

	d := &DockerExecutor{
		cfg:        cfg,
		dockerPath: dockerPath,
		id:         randomID(),
		allowlist:  NewAllowlist(cfg.AllowedCommands, cfg.DeniedCommands),
		logger:     logger,
	}

	ctx := context.Background()
	logger.Info(ctx, "MCP sandbox configured", map[string]interface{}{
		"image":       cfg.Image,
		"network":     cfg.NetworkMode,
		"executor_id": d.id,
	})
	if !strings.Contains(cfg.Image, "@sha256:") {
		logger.Warn(ctx, "MCP sandbox image is not pinned by digest, so what it contains can change; use image@sha256:...", map[string]interface{}{
			"image": cfg.Image,
		})
	}
	return d, nil
}

// ID returns the value of the LabelKey label on this executor's containers.
func (d *DockerExecutor) ID() string {
	return d.id
}

// Command returns a docker run command that starts name with args in a new
// container. env holds the server's "KEY=VALUE" variables; they are the only
// environment the server sees.
//
// If ctx is canceled while the server runs, the container is force-removed.
func (d *DockerExecutor) Command(ctx context.Context, name string, args []string, env []string) (*exec.Cmd, error) {
	d.mu.Lock()
	closed := d.closed
	d.mu.Unlock()
	if closed {
		return nil, ErrClosed
	}
	if err := d.allowlist.Check(name); err != nil {
		return nil, err
	}

	containerName := containerPrefix + randomID()
	runArgs, cliEnv := d.runArgs(containerName, name, args, env)

	// #nosec G204 -- no shell is involved: the arguments are built from a
	// validated Config, and the allowlist has approved name.
	cmd := exec.CommandContext(ctx, d.dockerPath, runArgs...)
	cmd.Env = cliEnv
	cmd.Cancel = func() error {
		// Killing the docker CLI alone can leave the container running.
		err := cmd.Process.Kill()
		d.remove(containerName)
		return err
	}

	d.logger.Info(ctx, "Starting MCP server in a sandbox container", map[string]interface{}{
		"container": containerName,
		"command":   name,
		"image":     d.cfg.Image,
	})
	return cmd, nil
}

// Close force-removes every container this executor started that still
// exists, including ones serving a live MCP session, and makes later Command
// calls fail. It can be called more than once.
func (d *DockerExecutor) Close(ctx context.Context) error {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()

	// #nosec G204 -- fixed arguments plus the executor's own random ID.
	list := exec.CommandContext(ctx, d.dockerPath, "ps", "--all", "--quiet", "--filter", "label="+LabelKey+"="+d.id)
	out, err := list.Output()
	if err != nil {
		return fmt.Errorf("sandbox: list containers: %w", err)
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil
	}

	// #nosec G204 -- container IDs come from docker ps.
	rm := exec.CommandContext(ctx, d.dockerPath, append([]string{"rm", "--force"}, ids...)...)
	if out, err := rm.CombinedOutput(); err != nil && !isNoSuchContainer(out) {
		return fmt.Errorf("sandbox: remove containers: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// runArgs returns the docker CLI arguments that start one server, and the
// environment to run the docker CLI with.
func (d *DockerExecutor) runArgs(containerName, name string, args, env []string) ([]string, []string) {
	c := d.cfg

	tmpfs := "/tmp:rw,nosuid,nodev,size=" + c.TmpfsSize
	if c.AllowExecInTmp {
		tmpfs += ",exec"
	} else {
		tmpfs += ",noexec"
	}

	out := []string{
		"run", "--rm", "--interactive", "--init",
		"--name", containerName,
		"--label", LabelKey + "=" + d.id,
		"--network", c.NetworkMode,
		"--memory", c.MemoryLimit,
		"--cpus", c.CPULimit,
		"--pids-limit", strconv.Itoa(c.PidsLimit),
		"--read-only",
		"--tmpfs", tmpfs,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--entrypoint", name,
	}
	if c.User != "" {
		out = append(out, "--user", c.User)
	}
	for _, m := range c.MountPaths {
		spec := "type=bind,source=" + m.Host + ",target=" + m.Container
		if !m.Writable {
			spec += ",readonly"
		}
		out = append(out, "--mount", spec)
	}

	// Values are handed over through the docker CLI's own environment and
	// named with --env KEY, so secrets never appear in the process list.
	// Variables the docker CLI itself reads are passed inline instead, so
	// setting them for the server does not change how the CLI behaves.
	cliEnv := os.Environ()
	for _, kv := range env {
		key, _, ok := strings.Cut(kv, "=")
		if !ok || key == "" {
			continue // not KEY=VALUE; never fall back to the host's value
		}
		if readByDockerCLI(key) {
			out = append(out, "--env", kv)
			continue
		}
		out = append(out, "--env", key)
		cliEnv = append(cliEnv, kv)
	}

	out = append(out, c.Image)
	out = append(out, args...)
	return out, cliEnv
}

// readByDockerCLI reports whether the docker CLI reads the variable key.
func readByDockerCLI(key string) bool {
	return key == "HOME" || key == "PATH" || strings.HasPrefix(key, "DOCKER_")
}

// remove force-removes a container after its context is done, and logs a
// failure, since there is no caller left to return it to.
func (d *DockerExecutor) remove(containerName string) {
	ctx, cancel := context.WithTimeout(context.Background(), removeTimeout)
	defer cancel()
	// #nosec G204 -- containerName is generated by this package.
	out, err := exec.CommandContext(ctx, d.dockerPath, "rm", "--force", containerName).CombinedOutput()
	if err != nil && !isNoSuchContainer(out) {
		d.logger.Warn(ctx, "Failed to remove MCP sandbox container", map[string]interface{}{
			"container": containerName,
			"error":     err.Error(),
			"output":    strings.TrimSpace(string(out)),
		})
	}
}

// isNoSuchContainer reports whether docker output says a container was
// already gone, which is not a failure when removing it.
func isNoSuchContainer(out []byte) bool {
	return bytes.Contains(out, []byte("No such container"))
}

// randomID returns 16 random hex characters.
func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // never fails; see crypto/rand.Read
	return hex.EncodeToString(b)
}
