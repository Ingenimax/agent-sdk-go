package agentconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// ConfigurationClient handles fetching configurations from the StarOps config service
type ConfigurationClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new ConfigurationClient using the host from config
func NewClient() (*ConfigurationClient, error) {
	cfg := config.Get()
	if cfg.ConfigService.Host == "" {
		return nil, fmt.Errorf("STAROPS_CONFIG_SERVICE_HOST is not configured")
	}

	return &ConfigurationClient{
		baseURL: cfg.ConfigService.Host,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// FetchDeploymentConfig fetches configuration for a specific agent deployment
// Returns a map where keys are configuration keys and values are the resolved values
func (c *ConfigurationClient) FetchDeploymentConfig(ctx context.Context, deploymentID, environment string) (map[string]string, error) {
	if deploymentID == "" {
		return nil, fmt.Errorf("deploymentID cannot be empty")
	}
	if environment == "" {
		return nil, fmt.Errorf("environment cannot be empty")
	}

	// Build the request URL with query parameters
	reqURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	reqURL.Path = "/api/v1/configurations"

	// Add query parameters
	params := url.Values{}
	params.Set("instance_id", deploymentID)
	params.Set("environment", environment)
	reqURL.RawQuery = params.Encode()

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log or handle close error if needed
			_ = closeErr
		}
	}()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse the JSON response
	var configs []ConfigurationResponse
	if err := json.Unmarshal(body, &configs); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to map[string]string
	result := make(map[string]string, len(configs))
	for _, config := range configs {
		// Extract the value from the ResolvedConfigurationValue
		// The value field contains the actual resolved value (for both plain and secret types)
		result[config.Key] = config.Value.Value
	}

	return result, nil
}

// FetchAgentConfig retrieves a resolved agent configuration by agent_id and environment
func (c *ConfigurationClient) FetchAgentConfig(ctx context.Context, agentID, environment string) (*AgentConfigResponse, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agentID cannot be empty")
	}
	if environment == "" {
		environment = "development" // Default environment
	}

	// Build the request URL
	reqURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// Use the resolved endpoint that returns generated YAML
	reqURL.Path = fmt.Sprintf("/api/v1/agent-configs/%s/resolved", agentID)

	// Add query parameters
	params := url.Values{}
	params.Set("environment", environment)
	reqURL.RawQuery = params.Encode()

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add authentication if available
	if authToken := c.getAuthToken(); authToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log or handle close error if needed
			_ = closeErr
		}
	}()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check status code
	switch resp.StatusCode {
	case http.StatusOK:
		// Success - parse response
		var response AgentConfigResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return &response, nil

	case http.StatusNotFound:
		return nil, fmt.Errorf("agent configuration not found: agent_id=%s, environment=%s", agentID, environment)

	case http.StatusUnauthorized:
		return nil, fmt.Errorf("unauthorized: check your authentication credentials")

	default:
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}
}

// getAuthToken retrieves the auth token from environment variables
func (c *ConfigurationClient) getAuthToken() string {
	// First try environment variable
	if token := os.Getenv("STAROPS_AUTH_TOKEN"); token != "" {
		return token
	}

	// Then try from config service token
	if token := os.Getenv("STAROPS_CONFIG_SERVICE_TOKEN"); token != "" {
		return token
	}

	return ""
}
