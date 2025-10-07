package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// VertexConfig contains configuration for Google Vertex AI
type VertexConfig struct {
	Enabled     bool
	ProjectID   string
	Region      string
	AccessToken string
	TokenSource oauth2.TokenSource
	Credentials *google.Credentials

	regions            []string
	currentRegionIndex int
	mu                 sync.Mutex
}

// NewVertexConfig creates a new VertexConfig using Application Default Credentials
func NewVertexConfig(ctx context.Context, region, projectID string) (*VertexConfig, error) {
	if region == "" {
		return nil, fmt.Errorf("region is required for Vertex AI")
	}
	if projectID == "" {
		return nil, fmt.Errorf("projectID is required for Vertex AI")
	}

	// Find default credentials
	credentials, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to find default credentials: %w", err)
	}

	config := &VertexConfig{
		Enabled:     true,
		ProjectID:   projectID,
		Region:      region,
		TokenSource: credentials.TokenSource,
		Credentials: credentials,
	}
	config.parseRegions()
	return config, nil
}

// NewVertexConfigWithCredentials creates a new VertexConfig with explicit credentials file
func NewVertexConfigWithCredentials(ctx context.Context, region, projectID, credentialsPath string) (*VertexConfig, error) {
	if region == "" {
		return nil, fmt.Errorf("region is required for Vertex AI")
	}
	if projectID == "" {
		return nil, fmt.Errorf("projectID is required for Vertex AI")
	}
	if credentialsPath == "" {
		return nil, fmt.Errorf("credentialsPath is required")
	}

	// Read credentials file
	credentialsFile, err := os.Open(credentialsPath) // #nosec G304 - credentialsPath is validated and comes from trusted source
	if err != nil {
		return nil, fmt.Errorf("failed to open credentials file %s: %w", credentialsPath, err)
	}
	defer func() {
		_ = credentialsFile.Close()
	}()

	credentialsJSON, err := io.ReadAll(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file %s: %w", credentialsPath, err)
	}

	credentials, err := google.CredentialsFromJSON(ctx, credentialsJSON, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to load credentials from %s: %w", credentialsPath, err)
	}

	config := &VertexConfig{
		Enabled:     true,
		ProjectID:   projectID,
		Region:      region,
		TokenSource: credentials.TokenSource,
		Credentials: credentials,
	}
	config.parseRegions()
	return config, nil
}

// parseRegions splits the Region field by comma and stores as a list
func (vc *VertexConfig) parseRegions() {
	if vc.Region == "" {
		vc.regions = []string{}
		return
	}

	parts := strings.Split(vc.Region, ",")
	vc.regions = make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			vc.regions = append(vc.regions, trimmed)
		}
	}
	vc.currentRegionIndex = 0
}

// GetCurrentRegion returns the current region for round-robin rotation
func (vc *VertexConfig) GetCurrentRegion() string {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if len(vc.regions) == 0 {
		return vc.Region
	}
	return vc.regions[vc.currentRegionIndex]
}

// RotateRegion moves to the next region in round-robin fashion
func (vc *VertexConfig) RotateRegion() {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if len(vc.regions) <= 1 {
		return
	}

	vc.currentRegionIndex = (vc.currentRegionIndex + 1) % len(vc.regions)
}

// GetBaseURL returns the Vertex AI base URL for the configured region
func (vc *VertexConfig) GetBaseURL() string {
	if !vc.Enabled {
		return ""
	}

	if vc.GetCurrentRegion() == "global" {
		return "https://aiplatform.googleapis.com"
	}

	return fmt.Sprintf("https://%s-aiplatform.googleapis.com", vc.GetCurrentRegion())
}

// GetAuthHeaders returns the authentication headers for Vertex AI requests
func (vc *VertexConfig) GetAuthHeaders(ctx context.Context) (map[string]string, error) {
	if !vc.Enabled {
		return nil, fmt.Errorf("vertex AI is not enabled")
	}

	var token string

	// Use explicit access token if provided
	if vc.AccessToken != "" {
		token = vc.AccessToken
	} else if vc.TokenSource != nil {
		// Get token from token source
		oauthToken, err := vc.TokenSource.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to get access token: %w", err)
		}
		token = oauthToken.AccessToken
	} else {
		return nil, fmt.Errorf("no authentication method available")
	}

	return map[string]string{
		"Authorization": "Bearer " + token,
	}, nil
}

// TransformRequest converts an Anthropic request to Vertex AI format
// Returns the full URL, headers, and modified request body
func (vc *VertexConfig) TransformRequest(req *CompletionRequest, method, path string) (string, map[string]string, []byte, error) {
	if !vc.Enabled {
		return "", nil, nil, fmt.Errorf("vertex AI is not enabled")
	}

	// Store the model for URL construction
	model := req.Model
	if model == "" {
		return "", nil, nil, fmt.Errorf("model is required for Vertex AI")
	}

	// Create a copy of the request for modification
	vertexReq := *req

	// Remove model from body (it goes in the URL for Vertex AI)
	vertexReq.Model = ""

	// Add Vertex AI specific anthropic_version
	vertexReq.AnthropicVersion = "vertex-2023-10-16"

	// Determine the endpoint based on the path and streaming
	var endpoint string
	if strings.Contains(path, "messages") {
		if req.Stream {
			endpoint = "streamRawPredict"
		} else {
			endpoint = "rawPredict"
		}
	} else {
		// For other endpoints (like token counting), use rawPredict
		endpoint = "rawPredict"
	}

	// Build the Vertex AI URL
	url := fmt.Sprintf(
		"%s/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:%s",
		vc.GetBaseURL(),
		vc.ProjectID,
		vc.GetCurrentRegion(),
		model,
		endpoint,
	)

	// Set headers
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	// Marshal the modified request
	reqBody, err := json.Marshal(vertexReq)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to marshal Vertex AI request: %w", err)
	}

	return url, headers, reqBody, nil
}

// CreateVertexHTTPRequest creates an HTTP request configured for Vertex AI
func (vc *VertexConfig) CreateVertexHTTPRequest(ctx context.Context, req *CompletionRequest, method, path string) (*http.Request, error) {
	if !vc.Enabled {
		return nil, fmt.Errorf("vertex AI is not enabled")
	}

	// Transform the request
	url, headers, body, err := vc.TransformRequest(req, method, path)
	if err != nil {
		return nil, fmt.Errorf("failed to transform request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set basic headers
	for key, value := range headers {
		httpReq.Header.Set(key, value)
	}

	// Set authentication headers
	authHeaders, err := vc.GetAuthHeaders(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth headers: %w", err)
	}

	for key, value := range authHeaders {
		httpReq.Header.Set(key, value)
	}

	// Set additional headers for streaming if needed
	if req.Stream {
		httpReq.Header.Set("Accept", "text/event-stream")
		httpReq.Header.Set("Cache-Control", "no-cache")
	}

	return httpReq, nil
}

// CreateVertexStreamingHTTPRequest creates an HTTP request configured for Vertex AI streaming
func (vc *VertexConfig) CreateVertexStreamingHTTPRequest(ctx context.Context, req *CompletionRequest, method, path string) (*http.Request, error) {
	// Ensure streaming is enabled
	req.Stream = true

	// Use the same function but with streaming headers
	httpReq, err := vc.CreateVertexHTTPRequest(ctx, req, method, path)
	if err != nil {
		return nil, err
	}

	// Ensure streaming headers are set
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")

	return httpReq, nil
}

// IsVertexModel checks if a model name is in Vertex AI format (contains @)
func IsVertexModel(model string) bool {
	return strings.Contains(model, "@")
}

// ConvertToVertexModel converts a standard Anthropic model name to Vertex AI format
// This is a basic mapping - users should use the correct Vertex model names
func ConvertToVertexModel(model string) string {
	// Basic mapping for common models - users should specify the correct Vertex model names
	switch model {
	case Claude35Haiku:
		return "claude-3-5-haiku@20241022"
	case Claude35Sonnet:
		return "claude-3-5-sonnet-v2@20241022"
	case Claude3Opus:
		return "claude-3-opus@20240229"
	case Claude37Sonnet:
		return "claude-3-7-sonnet@20250219"
	case ClaudeSonnet4:
		return "claude-sonnet-4-v1@20250514"
	case ClaudeOpus4:
		return "claude-opus-4-v1@20250514"
	case ClaudeOpus41:
		return "claude-opus-4-1@20250805"
	default:
		// Return as-is if already in Vertex format or unknown
		return model
	}
}

// ValidateVertexConfig validates the Vertex AI configuration
func (vc *VertexConfig) ValidateVertexConfig() error {
	if !vc.Enabled {
		return nil
	}

	if vc.Region == "" {
		return fmt.Errorf("region is required for Vertex AI")
	}

	if vc.ProjectID == "" {
		return fmt.Errorf("projectID is required for Vertex AI")
	}

	if vc.TokenSource == nil && vc.AccessToken == "" {
		return fmt.Errorf("either TokenSource or AccessToken must be provided for Vertex AI")
	}

	return nil
}

// GetSupportedRegions returns a list of regions that support Anthropic models on Vertex AI
func GetSupportedRegions() []string {
	return []string{
		"us-central1",
		"us-east5",
		"europe-west1",
		"europe-west4",
		"asia-southeast1",
		"asia-northeast3",
	}
}

// IsRegionSupported checks if a region supports Anthropic models on Vertex AI
func IsRegionSupported(region string) bool {
	supportedRegions := GetSupportedRegions()
	for _, supported := range supportedRegions {
		if region == supported {
			return true
		}
	}
	return false
}

// Policy represents the retry policy (imported from pkg/retry)
type Policy struct {
	InitialInterval    time.Duration
	BackoffCoefficient float64
	MaximumInterval    time.Duration
	MaximumAttempts    int32
}

// VertexRetryExecutor wraps retry execution with region rotation for Vertex AI
type VertexRetryExecutor struct {
	vertexConfig *VertexConfig
	policy       *Policy
	logger       logging.Logger
}

// NewVertexRetryExecutor creates a new retry executor for Vertex AI with region rotation
func NewVertexRetryExecutor(vertexConfig *VertexConfig, policy *Policy) *VertexRetryExecutor {
	return &VertexRetryExecutor{
		vertexConfig: vertexConfig,
		policy:       policy,
		logger:       logging.New(),
	}
}

// Execute executes the operation with retries and region rotation
func (e *VertexRetryExecutor) Execute(ctx context.Context, operation func() error) error {
	var lastErr error
	attempt := int32(0)
	currentInterval := e.policy.InitialInterval
	contextCancelled := false

	for attempt < e.policy.MaximumAttempts {
		// Check context but don't immediately return - just log it
		select {
		case <-ctx.Done():
			if !contextCancelled {
				contextCancelled = true
				e.logger.Warn(ctx, "Context cancelled but continuing with retry attempts", map[string]interface{}{
					"attempt": attempt,
					"error":   ctx.Err(),
				})
			}
		default:
		}

		currentRegion := e.vertexConfig.GetCurrentRegion()
		e.logger.Debug(ctx, "Attempting operation", map[string]interface{}{
			"attempt":      attempt + 1,
			"max_attempts": e.policy.MaximumAttempts,
			"region":       currentRegion,
		})

		if err := operation(); err == nil {
			e.logger.Debug(ctx, "Operation succeeded", map[string]interface{}{
				"attempt": attempt + 1,
				"region":  currentRegion,
			})
			return nil
		} else {
			lastErr = err
			attempt++

			if attempt >= e.policy.MaximumAttempts {
				e.logger.Debug(ctx, "Maximum attempts reached", map[string]interface{}{
					"attempt": attempt,
					"error":   err.Error(),
					"region":  currentRegion,
				})
				break
			}

			e.vertexConfig.RotateRegion()
			nextRegion := e.vertexConfig.GetCurrentRegion()

			nextInterval := time.Duration(float64(currentInterval) * e.policy.BackoffCoefficient)
			if nextInterval > e.policy.MaximumInterval {
				nextInterval = e.policy.MaximumInterval
			}

			e.logger.Debug(ctx, "Operation failed, rotating region and scheduling retry", map[string]interface{}{
				"attempt":          attempt,
				"error":            err.Error(),
				"current_region":   currentRegion,
				"next_region":      nextRegion,
				"current_interval": currentInterval,
				"next_interval":    nextInterval,
			})

			// Wait for retry interval, but don't fail on context cancellation
			select {
			case <-time.After(currentInterval):
				currentInterval = nextInterval
			case <-ctx.Done():
				if !contextCancelled {
					contextCancelled = true
					e.logger.Warn(ctx, "Context cancelled during retry delay, but continuing", map[string]interface{}{
						"attempt": attempt,
						"error":   ctx.Err(),
					})
				}
				// Still wait a bit before retry even if context cancelled
				time.Sleep(currentInterval)
				currentInterval = nextInterval
			}
		}
	}

	return lastErr
}
