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
}
