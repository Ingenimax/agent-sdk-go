package mcp

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// Builder provides a fluent interface for creating MCP server configurations
type Builder struct {
	servers      []interfaces.MCPServer
	lazyConfigs  []LazyMCPServerConfig
	logger       logging.Logger
	retryOptions *RetryOptions
	healthCheck  bool
	timeout      time.Duration
	errors       []error
}

// RetryOptions configures retry behavior for MCP connections
type RetryOptions struct {
	MaxAttempts       int
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	BackoffMultiplier float64
}

// NewBuilder creates a new MCP configuration builder
func NewBuilder() *Builder {
	return &Builder{
		logger: logging.New(),
		retryOptions: &RetryOptions{
			MaxAttempts:       5,
			InitialDelay:      1 * time.Second,
			MaxDelay:          30 * time.Second,
			BackoffMultiplier: 2.0,
		},
		timeout:     30 * time.Second,
		healthCheck: true,
	}
}

// WithLogger sets a custom logger
func (b *Builder) WithLogger(logger logging.Logger) *Builder {
	b.logger = logger
	return b
}

// WithRetry configures retry options
func (b *Builder) WithRetry(maxAttempts int, initialDelay time.Duration) *Builder {
	b.retryOptions.MaxAttempts = maxAttempts
	b.retryOptions.InitialDelay = initialDelay
	return b
}

// WithTimeout sets the connection timeout
func (b *Builder) WithTimeout(timeout time.Duration) *Builder {
	b.timeout = timeout
	return b
}

// WithHealthCheck enables or disables health checking
func (b *Builder) WithHealthCheck(enabled bool) *Builder {
	b.healthCheck = enabled
	return b
}

// AddServer adds an MCP server from a URL string
// Supports formats:
// - stdio://command/path/to/executable
// - http://localhost:8080/mcp
// - https://api.example.com/mcp?token=xxx
// - https://api.example.com/mcp?token=xxx&transport=streamable  (or transport=sse)
// - mcp://preset-name (for presets)
//
// For HTTP URLs, optional query params: token (auth), transport (sse or streamable).
func (b *Builder) AddServer(urlStr string) *Builder {
	server, config, err := b.parseServerURL(urlStr)
	if err != nil {
		b.errors = append(b.errors, fmt.Errorf("failed to parse server URL %q: %w", urlStr, err))
		return b
	}

	if server != nil {
		b.servers = append(b.servers, server)
	} else if config != nil {
		b.lazyConfigs = append(b.lazyConfigs, *config)
	}

	return b
}

// AddStdioServer adds a stdio-based MCP server
func (b *Builder) AddStdioServer(name, command string, args ...string) *Builder {
	config := LazyMCPServerConfig{
		Name:    name,
		Type:    "stdio",
		Command: command,
		Args:    args,
	}
	b.lazyConfigs = append(b.lazyConfigs, config)
	return b
}

// AddHTTPServer adds an HTTP-based MCP server
func (b *Builder) AddHTTPServer(name, baseURL string) *Builder {
	config := LazyMCPServerConfig{
		Name: name,
		Type: "http",
		URL:  baseURL,
	}
	b.lazyConfigs = append(b.lazyConfigs, config)
	return b
}

// AddHTTPServerWithAuth adds an HTTP-based MCP server with authentication
func (b *Builder) AddHTTPServerWithAuth(name, baseURL, token string) *Builder {
	// Parse URL and add token as query parameter if not present
	u, err := url.Parse(baseURL)
	if err != nil {
		b.errors = append(b.errors, fmt.Errorf("invalid URL %q: %w", baseURL, err))
		return b
	}

	// Validate URL has proper scheme for HTTP server
	if u.Scheme != "http" && u.Scheme != "https" {
		b.errors = append(b.errors, fmt.Errorf("invalid URL scheme for HTTP server %q: expected http or https, got %q", baseURL, u.Scheme))
		return b
	}

	// Add token to query parameters
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()

	config := LazyMCPServerConfig{
		Name: name,
		Type: "http",
		URL:  u.String(),
	}
	b.lazyConfigs = append(b.lazyConfigs, config)
	return b
}

// AddPreset adds a predefined MCP server configuration
func (b *Builder) AddPreset(presetName string) *Builder {
	preset, err := GetPreset(presetName)
	if err != nil {
		b.errors = append(b.errors, err)
		return b
	}

	// Apply preset configuration
	switch preset.Type {
	case "stdio":
		b.AddStdioServer(preset.Name, preset.Command, preset.Args...)
	case "http":
		b.AddHTTPServer(preset.Name, preset.URL)
	}

	return b
}

// Build creates the MCP servers and returns them
func (b *Builder) Build(ctx context.Context) ([]interfaces.MCPServer, []LazyMCPServerConfig, error) {
	// Check for any errors accumulated during building
	if len(b.errors) > 0 {
		return nil, nil, fmt.Errorf("builder errors: %v", b.errors)
	}

	// Initialize non-lazy servers if health check is enabled
	if b.healthCheck {
		for _, config := range b.lazyConfigs {
			// Only initialize if it's a critical server (could be configured)
			if b.shouldInitializeEagerly(config) {
				server, err := b.initializeServer(ctx, config)
				if err != nil {
					b.logger.Warn(ctx, "Failed to initialize MCP server", map[string]interface{}{
						"server_name": config.Name,
						"error":       err.Error(),
					})
					// Continue with other servers instead of failing
					continue
				}
				b.servers = append(b.servers, server)
			}
		}
	}

	return b.servers, b.lazyConfigs, nil
}

// BuildLazy returns only lazy configurations without initializing any servers
func (b *Builder) BuildLazy() ([]LazyMCPServerConfig, error) {
	if len(b.errors) > 0 {
		return nil, fmt.Errorf("builder errors: %v", b.errors)
	}
	return b.lazyConfigs, nil
}

// parseServerURL parses an MCP server URL and returns either a server or a lazy config
func (b *Builder) parseServerURL(urlStr string) (interfaces.MCPServer, *LazyMCPServerConfig, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, nil, err
	}

	switch u.Scheme {
	case "stdio":
		// Format: stdio://name/path/to/command?arg1=val1&arg2=val2
		parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
		if len(parts) < 2 {
			return nil, nil, fmt.Errorf("invalid stdio URL format")
		}

		name := u.Host
		if name == "" {
			name = parts[0]
			parts = parts[1:]
		}

		command := "/" + strings.Join(parts, "/")

		// Parse query parameters as arguments
		args := []string{}
		for key, values := range u.Query() {
			for _, value := range values {
				if value != "" {
					args = append(args, fmt.Sprintf("--%s=%s", key, value))
				} else {
					args = append(args, fmt.Sprintf("--%s", key))
				}
			}
		}

		config := &LazyMCPServerConfig{
			Name:    name,
			Type:    "stdio",
			Command: command,
			Args:    args,
		}
		return nil, config, nil

	case "http", "https":
		// Format: http://host:port/path?token=xxx[&transport=streamable|sse]
		// transport: "streamable" or "sse"; if omitted, default is SSE with fallback to streamable
		name := u.Host
		q := u.Query()
		token := q.Get("token")
		transport := strings.ToLower(strings.TrimSpace(q.Get("transport")))
		q.Del("transport")
		q.Del("token")
		u.RawQuery = q.Encode()
		config := &LazyMCPServerConfig{
			Name:              name,
			Type:              "http",
			URL:               u.String(),
			Token:             token,
			HttpTransportMode: transport,
		}
		// Normalize so only "sse" or "streamable" are set; anything else stays empty (default)
		if transport != "sse" && transport != "streamable" {
			config.HttpTransportMode = ""
		}
		return nil, config, nil

	case "mcp":
		// Format: mcp://preset-name
		presetName := u.Host + u.Path
		preset, err := GetPreset(presetName)
		if err != nil {
			return nil, nil, err
		}
		return nil, &preset, nil

	default:
		return nil, nil, fmt.Errorf("unsupported URL scheme: %s", u.Scheme)
	}
}

// initializeServer creates an MCP server from a configuration
func (b *Builder) initializeServer(ctx context.Context, config LazyMCPServerConfig) (interfaces.MCPServer, error) {
	// Use context with timeout
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	var server interfaces.MCPServer
	var err error

	switch config.Type {
	case "stdio":
		server, err = NewStdioServerWithRetry(ctx, StdioServerConfig{
			Command: config.Command,
			Args:    config.Args,
			Env:     config.Env,
			Logger:  b.logger,
		}, &RetryConfig{
			MaxAttempts:       b.retryOptions.MaxAttempts,
			InitialDelay:      b.retryOptions.InitialDelay,
			MaxDelay:          b.retryOptions.MaxDelay,
			BackoffMultiplier: b.retryOptions.BackoffMultiplier,
		})
	case "http":
		// URL is already clean (token/transport stripped in parseServerURL); use config.Token
		server, err = NewHTTPServerWithRetry(ctx, HTTPServerConfig{
			BaseURL:      config.URL,
			Token:        config.Token,
			Logger:       b.logger,
			ProtocolType: ServerProtocolType(config.HttpTransportMode),
		}, &RetryConfig{
			MaxAttempts:       b.retryOptions.MaxAttempts,
			InitialDelay:      b.retryOptions.InitialDelay,
			MaxDelay:          b.retryOptions.MaxDelay,
			BackoffMultiplier: b.retryOptions.BackoffMultiplier,
		})
	default:
		return nil, fmt.Errorf("unsupported server type: %s", config.Type)
	}

	if err != nil && b.retryOptions.MaxAttempts > 1 {
		server, err = b.retryConnection(ctx, config)
	}

	return server, err
}

// retryConnection attempts to connect with exponential backoff
func (b *Builder) retryConnection(ctx context.Context, config LazyMCPServerConfig) (interfaces.MCPServer, error) {
	delay := b.retryOptions.InitialDelay

	for attempt := 1; attempt <= b.retryOptions.MaxAttempts; attempt++ {
		b.logger.Debug(ctx, "Retrying MCP connection", map[string]interface{}{
			"server_name":  config.Name,
			"attempt":      attempt,
			"max_attempts": b.retryOptions.MaxAttempts,
		})

		server, err := b.initializeServer(ctx, config)
		if err == nil {
			return server, nil
		}

		if attempt < b.retryOptions.MaxAttempts {
			time.Sleep(delay)
			delay = time.Duration(float64(delay) * b.retryOptions.BackoffMultiplier)
			if delay > b.retryOptions.MaxDelay {
				delay = b.retryOptions.MaxDelay
			}
		}
	}

	return nil, fmt.Errorf("failed to connect after %d attempts", b.retryOptions.MaxAttempts)
}

// shouldInitializeEagerly determines if a server should be initialized immediately
func (b *Builder) shouldInitializeEagerly(config LazyMCPServerConfig) bool {
	// For now, all servers are lazy by default
	// This can be extended to check for critical servers
	return false
}
