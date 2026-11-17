package sandbox

import "errors"

var (
	// ErrCommandDenied is returned when a command is not allowed to start.
	ErrCommandDenied = errors.New("sandbox: command not allowed")

	// ErrDockerNotFound is returned when the docker CLI cannot be found.
	ErrDockerNotFound = errors.New("sandbox: docker CLI not found")

	// ErrInvalidConfig is returned when a Config cannot be used.
	ErrInvalidConfig = errors.New("sandbox: invalid config")

	// ErrClosed is returned when Command is called after Close.
	ErrClosed = errors.New("sandbox: executor is closed")
)
