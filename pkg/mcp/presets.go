package mcp

import (
	"fmt"
	"os"
)

// PresetServer represents a predefined MCP server configuration
type PresetServer struct {
	Name        string
	Description string
	Type        string // "stdio" or "http"
	Command     string
	Args        []string
	Env         []string
	URL         string
	RequiredEnv []string // Environment variables that must be set
}

// Common MCP server presets
var presets = map[string]PresetServer{
	// File system operations
	"filesystem": {
		Name:        "filesystem",
		Description: "MCP server for file system operations",
		Type:        "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-filesystem"},
	},

	// GitHub operations
	"github": {
		Name:        "github",
		Description: "MCP server for GitHub operations",
		Type:        "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-github"},
		RequiredEnv: []string{"GITHUB_TOKEN"},
	},

	// Git operations
	"git": {
		Name:        "git",
		Description: "MCP server for Git operations",
		Type:        "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-git"},
	},

	// PostgreSQL database
	"postgres": {
		Name:        "postgres",
		Description: "MCP server for PostgreSQL database operations",
		Type:        "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-postgres"},
		RequiredEnv: []string{"DATABASE_URL"},
	},

	// Slack integration
	"slack": {
		Name:        "slack",
		Description: "MCP server for Slack operations",
		Type:        "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-slack"},
		RequiredEnv: []string{"SLACK_BOT_TOKEN", "SLACK_TEAM_ID"},
	},

	// Google Drive
	"gdrive": {
		Name:        "gdrive",
		Description: "MCP server for Google Drive operations",
		Type:        "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-gdrive"},
		RequiredEnv: []string{"GOOGLE_CREDENTIALS"},
	},

	// Puppeteer for web automation
	"puppeteer": {
		Name:        "puppeteer",
		Description: "MCP server for web automation with Puppeteer",
		Type:        "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-puppeteer"},
	},

	// Memory/knowledge base
	"memory": {
		Name:        "memory",
		Description: "MCP server for memory and knowledge management",
		Type:        "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-memory"},
	},

	// Fetch for HTTP requests
	"fetch": {
		Name:        "fetch",
		Description: "MCP server for making HTTP requests",
		Type:        "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-fetch"},
	},

	// Brave Search
	"brave-search": {
		Name:        "brave-search",
		Description: "MCP server for Brave Search API",
		Type:        "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-brave-search"},
		RequiredEnv: []string{"BRAVE_API_KEY"},
	},

	// Time and date operations
	"time": {
		Name:        "time",
		Description: "MCP server for time and date operations",
		Type:        "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-time"},
	},

	// SQLite database
	"sqlite": {
		Name:        "sqlite",
		Description: "MCP server for SQLite database operations",
		Type:        "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-sqlite", "path/to/database.db"},
	},

	// Docker operations
	"docker": {
		Name:        "docker",
		Description: "MCP server for Docker container management",
		Type:        "stdio",
		Command:     "docker",
		Args: []string{
			"run", "--rm", "-i",
			"--volume", "/var/run/docker.sock:/var/run/docker.sock",
			"mcp/docker-server:latest",
		},
	},

	// Kubernetes operations
	"kubectl": {
		Name:        "kubectl",
		Description: "MCP server for Kubernetes operations",
		Type:        "stdio",
		Command:     "kubectl-mcp",
		Args:        []string{"serve"},
	},

	// AWS operations
	"aws": {
		Name:        "aws",
		Description: "MCP server for AWS operations",
		Type:        "stdio",
		Command:     "docker",
		Args: []string{
			"run", "--rm", "-i",
			"--env", "AWS_REGION",
			"--env", "AWS_ACCESS_KEY_ID",
			"--env", "AWS_SECRET_ACCESS_KEY",
			"public.ecr.aws/awslabs-mcp/awslabs/aws-api-mcp-server:latest",
		},
		RequiredEnv: []string{"AWS_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"},
	},
}

// GetPreset returns a preset configuration by name
func GetPreset(name string) (LazyMCPServerConfig, error) {
	preset, exists := presets[name]
	if !exists {
		return LazyMCPServerConfig{}, fmt.Errorf("preset %q not found", name)
	}

	// Check required environment variables
	for _, envVar := range preset.RequiredEnv {
		if os.Getenv(envVar) == "" {
			return LazyMCPServerConfig{}, fmt.Errorf("preset %q requires environment variable %s to be set", name, envVar)
		}
	}

	// Build environment variables list
	env := preset.Env
	for _, envVar := range preset.RequiredEnv {
		env = append(env, fmt.Sprintf("%s=%s", envVar, os.Getenv(envVar)))
	}

	return LazyMCPServerConfig{
		Name:    preset.Name,
		Type:    preset.Type,
		Command: preset.Command,
		Args:    preset.Args,
		Env:     env,
		URL:     preset.URL,
	}, nil
}

// ListPresets returns a list of available preset names
func ListPresets() []string {
	names := make([]string, 0, len(presets))
	for name := range presets {
		names = append(names, name)
	}
	return names
}

// GetPresetInfo returns information about a preset
func GetPresetInfo(name string) (string, error) {
	preset, exists := presets[name]
	if !exists {
		return "", fmt.Errorf("preset %q not found", name)
	}

	info := fmt.Sprintf("Name: %s\nDescription: %s\nType: %s\n",
		preset.Name, preset.Description, preset.Type)

	if preset.Type == "stdio" {
		info += fmt.Sprintf("Command: %s\n", preset.Command)
		if len(preset.Args) > 0 {
			info += fmt.Sprintf("Args: %v\n", preset.Args)
		}
	} else if preset.Type == "http" {
		info += fmt.Sprintf("URL: %s\n", preset.URL)
	}

	if len(preset.RequiredEnv) > 0 {
		info += fmt.Sprintf("Required Environment Variables: %v\n", preset.RequiredEnv)
	}

	return info, nil
}