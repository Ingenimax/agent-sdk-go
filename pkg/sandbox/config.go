package sandbox

import (
	"fmt"
	"path"
	"strings"
)

// Defaults applied to zero-value Config fields.
const (
	DefaultNetworkMode = "none"
	DefaultMemoryLimit = "256m"
	DefaultCPULimit    = "0.5"
	DefaultPidsLimit   = 64
	DefaultTmpfsSize   = "64m"
)

// Config configures a DockerExecutor.
type Config struct {
	// Image is the container image servers run in. Required. Pin it by
	// digest, for example "mcp/time@sha256:...", so it cannot change.
	Image string `json:"image" yaml:"image"`

	// AllowedCommands lists the commands servers may start. An entry without
	// a slash, such as "npx", allows that name, looked up on the container's
	// PATH. An absolute path, such as "/usr/local/bin/npx", allows exactly
	// that path. Matching is exact and case-sensitive. If the list is empty,
	// nothing is allowed.
	AllowedCommands []string `json:"allowed_commands" yaml:"allowed_commands"`

	// DeniedCommands lists commands refused even when allowed. They match by
	// base name, so denying "sh" refuses "sh", "/bin/sh" and "/usr/bin/sh".
	DeniedCommands []string `json:"denied_commands,omitempty" yaml:"denied_commands,omitempty"`

	// NetworkMode is the container's network, as for docker run --network.
	// The default, "none", gives the server no network at all. Use "bridge"
	// for servers that call external APIs. "host" is refused.
	NetworkMode string `json:"network_mode,omitempty" yaml:"network_mode,omitempty"`

	// MemoryLimit is the container's memory limit, as for docker run
	// --memory. Default "256m".
	MemoryLimit string `json:"memory_limit,omitempty" yaml:"memory_limit,omitempty"`

	// CPULimit is the number of CPUs the container may use, as for docker run
	// --cpus. Default "0.5".
	CPULimit string `json:"cpu_limit,omitempty" yaml:"cpu_limit,omitempty"`

	// PidsLimit caps the number of processes in the container. Default 64.
	PidsLimit int `json:"pids_limit,omitempty" yaml:"pids_limit,omitempty"`

	// TmpfsSize is the size of the writable /tmp. Default "64m".
	TmpfsSize string `json:"tmpfs_size,omitempty" yaml:"tmpfs_size,omitempty"`

	// AllowExecInTmp lets programs run from /tmp. It is off by default, so
	// nothing written at run time can be executed. Turn it on only for
	// servers that download a program at start and then run it from /tmp.
	// Node servers started with npx do not need it; they need network access
	// and HOME=/tmp in the server's environment.
	AllowExecInTmp bool `json:"allow_exec_in_tmp,omitempty" yaml:"allow_exec_in_tmp,omitempty"`

	// User runs the server as this user, as for docker run --user ("uid",
	// "uid:gid" or a user name in the image). Empty keeps the image's user.
	User string `json:"user,omitempty" yaml:"user,omitempty"`

	// MountPaths are host paths to share with the container.
	MountPaths []MountPath `json:"mount_paths,omitempty" yaml:"mount_paths,omitempty"`

	// DockerPath is the docker CLI to run. Empty means "docker" on the
	// host's PATH.
	DockerPath string `json:"docker_path,omitempty" yaml:"docker_path,omitempty"`
}

// MountPath shares a host path with the container. It is read-only unless
// Writable is set.
type MountPath struct {
	Host      string `json:"host" yaml:"host"`
	Container string `json:"container" yaml:"container"`
	Writable  bool   `json:"writable,omitempty" yaml:"writable,omitempty"`
}

// withDefaults returns a copy of c with zero-value fields set to defaults.
func (c Config) withDefaults() Config {
	if c.NetworkMode == "" {
		c.NetworkMode = DefaultNetworkMode
	}
	if c.MemoryLimit == "" {
		c.MemoryLimit = DefaultMemoryLimit
	}
	if c.CPULimit == "" {
		c.CPULimit = DefaultCPULimit
	}
	if c.PidsLimit == 0 {
		c.PidsLimit = DefaultPidsLimit
	}
	if c.TmpfsSize == "" {
		c.TmpfsSize = DefaultTmpfsSize
	}
	return c
}

// validate reports the first problem with c, which must already have its
// defaults applied.
func (c Config) validate() error {
	if strings.TrimSpace(c.Image) == "" {
		return fmt.Errorf("%w: image is required", ErrInvalidConfig)
	}
	// The image is a positional argument of docker run, so a leading dash
	// would be read as a flag.
	if strings.HasPrefix(c.Image, "-") {
		return fmt.Errorf("%w: image %q must not start with '-'", ErrInvalidConfig, c.Image)
	}
	if c.NetworkMode == "host" {
		return fmt.Errorf("%w: network mode \"host\" would give servers the host's network", ErrInvalidConfig)
	}
	if c.PidsLimit < 0 {
		return fmt.Errorf("%w: pids limit must not be negative", ErrInvalidConfig)
	}
	for _, cmd := range append(append([]string(nil), c.AllowedCommands...), c.DeniedCommands...) {
		if cmd == "" {
			return fmt.Errorf("%w: command names must not be empty", ErrInvalidConfig)
		}
		if strings.Contains(cmd, "/") && !path.IsAbs(cmd) {
			return fmt.Errorf("%w: command %q must be a bare name or an absolute path", ErrInvalidConfig, cmd)
		}
	}
	for _, m := range c.MountPaths {
		if err := m.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (m MountPath) validate() error {
	for _, p := range []string{m.Host, m.Container} {
		if !path.IsAbs(p) && !isWindowsAbs(p) {
			return fmt.Errorf("%w: mount path %q must be absolute", ErrInvalidConfig, p)
		}
		// Mounts are passed as docker run --mount type=bind,source=...,
		// so a comma or a quote would change the mount's meaning.
		if strings.ContainsAny(p, ",\"") {
			return fmt.Errorf("%w: mount path %q must not contain a comma or a quote", ErrInvalidConfig, p)
		}
	}
	if !path.IsAbs(m.Container) {
		return fmt.Errorf("%w: container mount path %q must be an absolute Linux path", ErrInvalidConfig, m.Container)
	}
	return nil
}

// isWindowsAbs reports whether p looks like an absolute Windows path, such as
// C:\data or C:/data, for host mounts on Windows.
func isWindowsAbs(p string) bool {
	return len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') &&
		((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z'))
}
