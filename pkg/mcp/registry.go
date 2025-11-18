package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// RegistryClient handles interaction with MCP Registry for server discovery
type RegistryClient struct {
	baseURL    string
	httpClient *http.Client
	logger     logging.Logger
}

// DefaultRegistryURL is the official MCP Registry URL
const DefaultRegistryURL = "https://registry.modelcontextprotocol.io"

// NewRegistryClient creates a new MCP Registry client
func NewRegistryClient(baseURL string) *RegistryClient {
	if baseURL == "" {
		baseURL = DefaultRegistryURL
	}

	return &RegistryClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logging.New(),
	}
}

// RegistryServer represents a server entry in the MCP Registry
type RegistryServer struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Namespace   string         `json:"namespace"`
	Version     string         `json:"version"`
	Tags        []string       `json:"tags,omitempty"`
	Category    string         `json:"category,omitempty"`
	Author      RegistryAuthor `json:"author"`
	Repository  RepositoryInfo `json:"repository,omitempty"`
	License     string         `json:"license,omitempty"`
	Homepage    string         `json:"homepage,omitempty"`

	// Installation and configuration
	Installation  InstallationInfo  `json:"installation"`
	Configuration ConfigurationInfo `json:"configuration,omitempty"`

	// Capabilities
	Tools     []RegistryTool     `json:"tools,omitempty"`
	Resources []RegistryResource `json:"resources,omitempty"`
	Prompts   []RegistryPrompt   `json:"prompts,omitempty"`

	// Registry metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Downloads int       `json:"downloads,omitempty"`
	Rating    float64   `json:"rating,omitempty"`
	Verified  bool      `json:"verified"`
}

// RegistryAuthor represents the author of a registry server
type RegistryAuthor struct {
	Name   string `json:"name"`
	Email  string `json:"email,omitempty"`
	URL    string `json:"url,omitempty"`
	GitHub string `json:"github,omitempty"`
}

// RepositoryInfo represents repository information
type RepositoryInfo struct {
	Type string `json:"type"` // "git", "github", etc.
	URL  string `json:"url"`
	Ref  string `json:"ref,omitempty"` // branch, tag, or commit
}

// InstallationInfo describes how to install and run the server
type InstallationInfo struct {
	Type    string                 `json:"type"`    // "npm", "pip", "docker", "binary", "stdio"
	Command string                 `json:"command"` // Command to run the server
	Args    []string               `json:"args,omitempty"`
	Env     map[string]string      `json:"env,omitempty"`
	Config  map[string]interface{} `json:"config,omitempty"`
}

// ConfigurationInfo describes server configuration options
type ConfigurationInfo struct {
	Required []ConfigOption `json:"required,omitempty"`
	Optional []ConfigOption `json:"optional,omitempty"`
}

// ConfigOption represents a configuration option
type ConfigOption struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Type        string      `json:"type"` // "string", "number", "boolean", "array"
	Default     interface{} `json:"default,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	Required    bool        `json:"required"`
	Sensitive   bool        `json:"sensitive"` // For API keys, passwords, etc.
}

// RegistryTool represents a tool advertised by the server
type RegistryTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Category    string                 `json:"category,omitempty"`
}

// RegistryResource represents a resource advertised by the server
type RegistryResource struct {
	Type        string   `json:"type"` // "file", "database", "api", etc.
	Description string   `json:"description"`
	Pattern     string   `json:"pattern,omitempty"` // URI pattern
	MimeTypes   []string `json:"mime_types,omitempty"`
}

// RegistryPrompt represents a prompt advertised by the server
type RegistryPrompt struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Variables   []string `json:"variables,omitempty"`
	Category    string   `json:"category,omitempty"`
}

// SearchOptions configures server search parameters
type SearchOptions struct {
	Query    string   `json:"query,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Category string   `json:"category,omitempty"`
	Author   string   `json:"author,omitempty"`
	Verified bool     `json:"verified,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	Offset   int      `json:"offset,omitempty"`
}

// SearchResponse represents the response from server search
type SearchResponse struct {
	Servers []RegistryServer `json:"servers"`
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
}

// ListServers retrieves all available servers from the registry
func (rc *RegistryClient) ListServers(ctx context.Context, opts *SearchOptions) (*SearchResponse, error) {
	endpoint := "/api/v1/servers"

	params := url.Values{}
	if opts != nil {
		if opts.Query != "" {
			params.Set("q", opts.Query)
		}
		if opts.Category != "" {
			params.Set("category", opts.Category)
		}
		if opts.Author != "" {
			params.Set("author", opts.Author)
		}
		if opts.Verified {
			params.Set("verified", "true")
		}
		if opts.Limit > 0 {
			params.Set("limit", fmt.Sprintf("%d", opts.Limit))
		}
		if opts.Offset > 0 {
			params.Set("offset", fmt.Sprintf("%d", opts.Offset))
		}
		if len(opts.Tags) > 0 {
			params.Set("tags", strings.Join(opts.Tags, ","))
		}
	}

	fullURL := rc.baseURL + endpoint
	if len(params) > 0 {
		fullURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "agent-sdk-go/1.0")

	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("Failed to close response body: %v\n", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registry request failed: HTTP %d, body: %s", resp.StatusCode, string(body))
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	rc.logger.Debug(ctx, "Retrieved servers from registry", map[string]interface{}{
		"count": len(searchResp.Servers),
		"total": searchResp.Total,
		"query": opts.Query,
	})

	return &searchResp, nil
}

// GetServer retrieves a specific server by ID
func (rc *RegistryClient) GetServer(ctx context.Context, serverID string) (*RegistryServer, error) {
	endpoint := fmt.Sprintf("/api/v1/servers/%s", url.PathEscape(serverID))
	fullURL := rc.baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "agent-sdk-go/1.0")

	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("Failed to close response body: %v\n", closeErr)
		}
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("server not found: %s", serverID)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registry request failed: HTTP %d, body: %s", resp.StatusCode, string(body))
	}

	var server RegistryServer
	if err := json.NewDecoder(resp.Body).Decode(&server); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	rc.logger.Debug(ctx, "Retrieved server from registry", map[string]interface{}{
		"server_id": serverID,
		"name":      server.Name,
		"version":   server.Version,
	})

	return &server, nil
}

// SearchServers searches for servers matching criteria
func (rc *RegistryClient) SearchServers(ctx context.Context, query string) (*SearchResponse, error) {
	opts := &SearchOptions{
		Query: query,
		Limit: 50,
	}

	return rc.ListServers(ctx, opts)
}

// GetServersByCategory retrieves servers in a specific category
func (rc *RegistryClient) GetServersByCategory(ctx context.Context, category string) (*SearchResponse, error) {
	opts := &SearchOptions{
		Category: category,
		Limit:    100,
	}

	return rc.ListServers(ctx, opts)
}

// GetVerifiedServers retrieves only verified servers
func (rc *RegistryClient) GetVerifiedServers(ctx context.Context) (*SearchResponse, error) {
	opts := &SearchOptions{
		Verified: true,
		Limit:    100,
	}

	return rc.ListServers(ctx, opts)
}

// GetServersByTags retrieves servers with specific tags
func (rc *RegistryClient) GetServersByTags(ctx context.Context, tags []string) (*SearchResponse, error) {
	opts := &SearchOptions{
		Tags:  tags,
		Limit: 100,
	}

	return rc.ListServers(ctx, opts)
}

// RegistryManager integrates registry discovery with MCP server creation
type RegistryManager struct {
	registryClient *RegistryClient
	builder        *Builder
	logger         logging.Logger
}

// NewRegistryManager creates a new registry manager
func NewRegistryManager(registryURL string) *RegistryManager {
	return &RegistryManager{
		registryClient: NewRegistryClient(registryURL),
		builder:        NewBuilder(),
		logger:         logging.New(),
	}
}

// DiscoverAndInstallServer discovers a server from registry and creates MCP server config
func (rm *RegistryManager) DiscoverAndInstallServer(ctx context.Context, serverID string) (*LazyMCPServerConfig, error) {
	// Get server information from registry
	server, err := rm.registryClient.GetServer(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to discover server: %w", err)
	}

	// Convert registry server to MCP server configuration
	config, err := rm.registryServerToConfig(server)
	if err != nil {
		return nil, fmt.Errorf("failed to create server config: %w", err)
	}

	rm.logger.Info(ctx, "Successfully discovered and configured server from registry", map[string]interface{}{
		"server_id": serverID,
		"name":      server.Name,
		"type":      config.Type,
	})

	return config, nil
}

// DiscoverServersByCapability finds servers that provide specific capabilities
func (rm *RegistryManager) DiscoverServersByCapability(ctx context.Context, capability string) ([]*RegistryServer, error) {
	// Search for servers that might provide the capability
	response, err := rm.registryClient.SearchServers(ctx, capability)
	if err != nil {
		return nil, err
	}

	var matchingServers []*RegistryServer
	for i, server := range response.Servers {
		if rm.serverProvidesCapability(&response.Servers[i], capability) {
			matchingServers = append(matchingServers, &server)
		}
	}

	return matchingServers, nil
}

// BuildServerFromRegistry creates an MCP server from a registry entry
func (rm *RegistryManager) BuildServerFromRegistry(ctx context.Context, serverID string) (interfaces.MCPServer, error) {
	config, err := rm.DiscoverAndInstallServer(ctx, serverID)
	if err != nil {
		return nil, err
	}

	// Use the builder to create the server
	server, err := rm.builder.initializeServer(ctx, *config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize server: %w", err)
	}

	return server, nil
}

// Helper methods

// registryServerToConfig converts a registry server to an MCP server config
func (rm *RegistryManager) registryServerToConfig(server *RegistryServer) (*LazyMCPServerConfig, error) {
	config := &LazyMCPServerConfig{
		Name: server.Name,
	}

	// Determine server type and configuration based on installation info
	switch server.Installation.Type {
	case "stdio":
		config.Type = "stdio"
		config.Command = server.Installation.Command
		config.Args = server.Installation.Args

		// Convert environment variables
		if len(server.Installation.Env) > 0 {
			for key, value := range server.Installation.Env {
				envVar := fmt.Sprintf("%s=%s", key, value)
				config.Env = append(config.Env, envVar)
			}
		}

	case "npm":
		// For npm packages, create stdio config
		config.Type = "stdio"
		config.Command = "npx"
		config.Args = []string{server.Installation.Command}
		if len(server.Installation.Args) > 0 {
			config.Args = append(config.Args, server.Installation.Args...)
		}

	case "pip", "python":
		// For Python packages
		config.Type = "stdio"
		config.Command = "python"
		config.Args = []string{"-m", server.Installation.Command}
		if len(server.Installation.Args) > 0 {
			config.Args = append(config.Args, server.Installation.Args...)
		}

	case "docker":
		// For Docker containers
		config.Type = "stdio"
		config.Command = "docker"
		config.Args = []string{"run", "--rm", "-i", server.Installation.Command}
		if len(server.Installation.Args) > 0 {
			config.Args = append(config.Args, server.Installation.Args...)
		}

	case "http":
		// For HTTP-based servers
		config.Type = "http"
		config.URL = server.Installation.Command

		// Note: OAuth configuration removed for simplified implementation

	default:
		return nil, fmt.Errorf("unsupported installation type: %s", server.Installation.Type)
	}

	return config, nil
}

// serverProvidesCapability checks if a server provides a specific capability
func (rm *RegistryManager) serverProvidesCapability(server *RegistryServer, capability string) bool {
	capability = strings.ToLower(capability)

	// Check tools
	for _, tool := range server.Tools {
		if strings.Contains(strings.ToLower(tool.Name), capability) ||
			strings.Contains(strings.ToLower(tool.Description), capability) ||
			strings.Contains(strings.ToLower(tool.Category), capability) {
			return true
		}
	}

	// Check resources
	for _, resource := range server.Resources {
		if strings.Contains(strings.ToLower(resource.Type), capability) ||
			strings.Contains(strings.ToLower(resource.Description), capability) {
			return true
		}
	}

	// Check prompts
	for _, prompt := range server.Prompts {
		if strings.Contains(strings.ToLower(prompt.Name), capability) ||
			strings.Contains(strings.ToLower(prompt.Description), capability) ||
			strings.Contains(strings.ToLower(prompt.Category), capability) {
			return true
		}
	}

	// Check tags and description
	for _, tag := range server.Tags {
		if strings.Contains(strings.ToLower(tag), capability) {
			return true
		}
	}

	if strings.Contains(strings.ToLower(server.Description), capability) ||
		strings.Contains(strings.ToLower(server.Category), capability) {
		return true
	}

	return false
}

// Popular server discovery helpers

// DiscoverFileSystemServers finds servers that can work with files
func (rm *RegistryManager) DiscoverFileSystemServers(ctx context.Context) ([]*RegistryServer, error) {
	return rm.DiscoverServersByCapability(ctx, "filesystem")
}

// DiscoverDatabaseServers finds servers that can work with databases
func (rm *RegistryManager) DiscoverDatabaseServers(ctx context.Context) ([]*RegistryServer, error) {
	return rm.DiscoverServersByCapability(ctx, "database")
}

// DiscoverWebServers finds servers that can work with web APIs
func (rm *RegistryManager) DiscoverWebServers(ctx context.Context) ([]*RegistryServer, error) {
	return rm.DiscoverServersByCapability(ctx, "web")
}

// DiscoverCodeServers finds servers that can work with code
func (rm *RegistryManager) DiscoverCodeServers(ctx context.Context) ([]*RegistryServer, error) {
	return rm.DiscoverServersByCapability(ctx, "code")
}

// GetPopularServers retrieves the most popular/downloaded servers
func (rm *RegistryManager) GetPopularServers(ctx context.Context) (*SearchResponse, error) {
	// This would typically be sorted by downloads/rating on the server side
	opts := &SearchOptions{
		Verified: true,
		Limit:    20,
	}

	return rm.registryClient.ListServers(ctx, opts)
}
