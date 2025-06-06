package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Tool implements a web search tool
type Tool struct {
	apiKey     string
	engineID   string
	httpClient *http.Client
	cache      map[string]cacheEntry
}

type cacheEntry struct {
	result    string
	timestamp time.Time
}

// Option represents an option for configuring the tool
type Option func(*Tool)

// WithHTTPClient sets the HTTP client for the tool
func WithHTTPClient(client *http.Client) Option {
	return func(t *Tool) {
		t.httpClient = client
	}
}

// New creates a new web search tool
func New(apiKey, engineID string, options ...Option) *Tool {
	tool := &Tool{
		apiKey:     apiKey,
		engineID:   engineID,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cache:      make(map[string]cacheEntry),
	}

	for _, option := range options {
		option(tool)
	}

	return tool
}

// Name returns the name of the tool
func (t *Tool) Name() string {
	return "web_search"
}

// Description returns a description of what the tool does
func (t *Tool) Description() string {
	return "Search the web for information on a given query"
}

// Parameters returns the parameters that the tool accepts
func (t *Tool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"query": {
			Type:        "string",
			Description: "The search query",
			Required:    true,
		},
		"num_results": {
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

	// Get num_results parameter
	numResults := 5
	if num, ok := params["num_results"].(float64); ok {
		numResults = int(num)
	}

	// Check cache
	if entry, ok := t.cache[query]; ok {
		if time.Since(entry.timestamp) < 1*time.Hour {
			return entry.result, nil
		}
	}

	// Get organization ID for API key management
	orgID, _ := multitenancy.GetOrgID(ctx)

	// Build request URL
	searchURL := fmt.Sprintf(
		"https://www.googleapis.com/customsearch/v1?key=%s&cx=%s&q=%s&num=%d",
		t.apiKey,
		t.engineID,
		url.QueryEscape(query),
		numResults,
	)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add organization ID to request headers if available
	if orgID != "" {
		req.Header.Set("X-Organization-ID", orgID)
	}

	// Execute request
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search API returned status code %d: %s", resp.StatusCode, resp.Status)
	}

	// Parse response
	var result struct {
		Items []struct {
			Title       string `json:"title"`
			Link        string `json:"link"`
			Snippet     string `json:"snippet"`
			DisplayLink string `json:"displayLink"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Format results
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for '%s':\n\n", query))
	for i, item := range result.Items {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.Title))
		sb.WriteString(fmt.Sprintf("   URL: %s\n", item.Link))
		sb.WriteString(fmt.Sprintf("   %s\n\n", item.Snippet))
	}

	// Cache result
	t.cache[query] = cacheEntry{
		result:    sb.String(),
		timestamp: time.Now(),
	}

	return sb.String(), nil
}

func (t *Tool) Execute(ctx context.Context, args string) (string, error) {
	// Parse args as JSON
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse args: %w", err)
	}

	// Execute search
	return t.Run(ctx, params.Query)
}
