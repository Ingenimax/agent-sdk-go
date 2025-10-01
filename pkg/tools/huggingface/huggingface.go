package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

type HuggingFaceModel struct {
	ID          string    `json:"_id"`
	ModelID     string    `json:"modelId"`
	Name        string    `json:"id"`
	Description string    `json:"description,omitempty"`
	URL         string    `json:"url,omitempty"`
	Downloads   int       `json:"downloads"`
	Likes       int       `json:"likes"`
	Private     bool      `json:"private"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"lastModified,omitempty"`
	Tags        []string  `json:"tags"`
	Task        string    `json:"pipeline_tag"`
	LibraryName string    `json:"library_name"`
}

// Tool implements a Hugging Face model search tool
type Tool struct {
	baseURL    string
	httpClient *http.Client
}

// Option represents an option for configuring the tool
type Option func(*Tool)

// WithBaseURL sets the base URL for the Hugging Face API
func WithBaseURL(baseURL string) Option {
	return func(t *Tool) {
		t.baseURL = baseURL
	}
}

// WithHTTPClient sets the HTTP client for the tool
func WithHTTPClient(client *http.Client) Option {
	return func(t *Tool) {
		t.httpClient = client
	}
}

// New creates a new Hugging Face model search tool
func New(options ...Option) *Tool {
	tool := &Tool{
		baseURL:    "https://huggingface.co",
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	for _, option := range options {
		option(tool)
	}

	return tool
}

// Name returns the name of the tool
func (t *Tool) Name() string {
	return "huggingface_search"
}

// DisplayName implements interfaces.ToolWithDisplayName.DisplayName
func (t *Tool) DisplayName() string {
	return "Hugging Face Search"
}

// Description returns a description of what the tool does
func (t *Tool) Description() string {
	return "Search Hugging Face for AI models matching the given query"
}

// Internal implements interfaces.InternalTool.Internal
func (t *Tool) Internal() bool {
	return false
}

// Parameters returns the parameters that the tool accepts
func (t *Tool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"query": {
			Type:        "string",
			Description: "The search query",
			Required:    true,
		},
		"limit": {
			Type:        "integer",
			Description: "Number of results to return",
			Required:    false,
			Default:     5,
		},
	}
}

// Run executes the tool with the given input
func (t *Tool) Run(ctx context.Context, input string) (string, error) {
	// Parse input as JSON
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		// If not JSON, treat the input as the query
		params = map[string]interface{}{
			"query": input,
		}
	}

	// Get query parameter
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("query parameter is required")
	}

	// Get limit parameter
	limit := 5
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	// URL encode the search query
	encodedQuery := url.QueryEscape(query)

	// Build request URL
	url := fmt.Sprintf("%s/api/models?search=%s&sort=downloads&direction=-1&limit=%d", t.baseURL, encodedQuery, limit)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("hugging Face API returned non-200 status code: %d", resp.StatusCode)
	}

	var models []HuggingFaceModel
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Format results
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d models matching '%s':\n\n", len(models), query))
	for i, model := range models {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, model.Name))
		sb.WriteString(fmt.Sprintf("   ID: %s\n", model.ModelID))
		if model.Description != "" {
			sb.WriteString(fmt.Sprintf("   Description: %s\n", model.Description))
		}
		sb.WriteString(fmt.Sprintf("   Downloads: %d\n", model.Downloads))
		sb.WriteString(fmt.Sprintf("   Likes: %d\n", model.Likes))
		sb.WriteString(fmt.Sprintf("   Task: %s\n", model.Task))
		sb.WriteString(fmt.Sprintf("   Library: %s\n", model.LibraryName))
		sb.WriteString(fmt.Sprintf("   Tags: %v\n\n", model.Tags))
	}

	return sb.String(), nil
}

// Execute implements the tool interface
func (t *Tool) Execute(ctx context.Context, args string) (string, error) {
	// Parse args as JSON
	var params struct {
		Query string `json:"query"`
		Limit int    `json:"limit,omitempty"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse args: %w", err)
	}

	// Execute search
	return t.Run(ctx, fmt.Sprintf(`{"query": "%s", "limit": %d}`, params.Query, params.Limit))
}
