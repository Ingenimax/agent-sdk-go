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

// Compile-time check that DockerExecutor implements CommandExecutor.
var _ CommandExecutor = (*DockerExecutor)(nil)

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
