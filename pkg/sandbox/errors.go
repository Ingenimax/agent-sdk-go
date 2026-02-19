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
