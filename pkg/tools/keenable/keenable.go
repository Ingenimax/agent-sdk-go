// Package keenable provides a web search tool backed by the Keenable Search API.
//
// The tool works without an API key: requests then go to Keenable's keyless
// public endpoint, which is rate-limited per IP. Pass a key (for example from
// KEENABLE_API_KEY) to use the keyed endpoint and lift that limit.
//
// It is a drop-in alternative to the Google Custom Search based websearch tool:
// it uses the same tool name ("web_search"), the same parameters (query,
// num_results) and the same plain-text result format, so register one or the
// other.
package keenable

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

const (
	// DefaultBaseURL is the Keenable API base URL.
	DefaultBaseURL = "https://api.keenable.ai"

	defaultNumResults = 5
	maxNumResults     = 50
	maxResponseBytes  = 4 << 20
	cacheTTL          = time.Hour
)

// Tool implements a web search tool using Keenable.
type Tool struct {
	apiKey     string
	baseURL    string
	name       string
	title      string
	httpClient *http.Client

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	result    string
	timestamp time.Time
}

// Option configures the tool.
type Option func(*Tool)

// WithHTTPClient sets the HTTP client used for requests.
func WithHTTPClient(client *http.Client) Option {
	return func(t *Tool) { t.httpClient = client }
}

// WithBaseURL overrides the API base URL (useful for tests or proxies).
func WithBaseURL(baseURL string) Option {
	return func(t *Tool) { t.baseURL = strings.TrimRight(baseURL, "/") }
}

// WithName overrides the tool name. The default is "web_search".
func WithName(name string) Option {
	return func(t *Tool) { t.name = name }
}

// WithTitle sets the X-Keenable-Title header that identifies the calling app
// on keyless requests. The default is "agent-sdk-go".
func WithTitle(title string) Option {
	return func(t *Tool) { t.title = title }
}

// New creates a Keenable search tool. apiKey may be empty to use the keyless
// public endpoint.
func New(apiKey string, options ...Option) *Tool {
	t := &Tool{
		apiKey:     strings.TrimSpace(apiKey),
		baseURL:    DefaultBaseURL,
		name:       "web_search",
		title:      "agent-sdk-go",
		httpClient: &http.Client{Timeout: 15 * time.Second},
		cache:      make(map[string]cacheEntry),
	}
	for _, option := range options {
		option(t)
	}
	return t
}

// Name returns the name of the tool.
func (t *Tool) Name() string { return t.name }

// DisplayName implements interfaces.ToolWithDisplayName.
func (t *Tool) DisplayName() string { return "Web Search (Keenable)" }

// Description returns a description of what the tool does.
func (t *Tool) Description() string {
	return "Search the web for information on a given query"
}

// Internal implements interfaces.InternalTool.
func (t *Tool) Internal() bool { return false }

// Parameters returns the parameters that the tool accepts.
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
			Default:     defaultNumResults,
		},
	}
}

type searchRequest struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

type searchResponse struct {
	Results []struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Snippet     string `json:"snippet"`
		PublishedAt string `json:"published_at"`
	} `json:"results"`
}

// Run executes a search. input is either JSON ({"query": ..., "num_results": N})
// or a plain query string.
func (t *Tool) Run(ctx context.Context, input string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		params = map[string]interface{}{"query": input}
	}

	query, _ := params["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("query parameter is required")
	}

	numResults := defaultNumResults
	if num, ok := params["num_results"].(float64); ok {
		numResults = int(num)
	}
	if numResults < 1 {
		numResults = 1
	}
	if numResults > maxNumResults {
		numResults = maxNumResults
	}

	cacheKey := fmt.Sprintf("%d\x00%s", numResults, query)
	t.mu.Lock()
	if entry, ok := t.cache[cacheKey]; ok && time.Since(entry.timestamp) < cacheTTL {
		t.mu.Unlock()
		return entry.result, nil
	}
	t.mu.Unlock()

	body, err := json.Marshal(searchRequest{Query: query, MaxResults: numResults})
	if err != nil {
		return "", fmt.Errorf("failed to encode request: %w", err)
	}

	endpoint := t.baseURL + "/v1/search/public"
	if t.apiKey != "" {
		endpoint = t.baseURL + "/v1/search"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if t.title != "" {
		req.Header.Set("X-Keenable-Title", t.title)
	}
	if t.apiKey != "" {
		req.Header.Set("X-API-Key", t.apiKey)
	}
	if orgID, _ := multitenancy.GetOrgID(ctx); orgID != "" {
		req.Header.Set("X-Organization-ID", orgID)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("keenable search returned status code %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var result searchResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Search results for '%s':\n\n", query)
	n := 0
	for _, item := range result.Results {
		if n >= numResults {
			break
		}
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		n++
		fmt.Fprintf(&sb, "%d. %s\n", n, item.Title)
		fmt.Fprintf(&sb, "   URL: %s\n", item.URL)
		if item.PublishedAt != "" {
			fmt.Fprintf(&sb, "   Published: %s\n", item.PublishedAt)
		}
		fmt.Fprintf(&sb, "   %s\n\n", item.Snippet)
	}

	out := sb.String()
	t.mu.Lock()
	t.cache[cacheKey] = cacheEntry{result: out, timestamp: time.Now()}
	t.mu.Unlock()
	return out, nil
}

// Execute runs the tool with JSON arguments.
func (t *Tool) Execute(ctx context.Context, args string) (string, error) {
	return t.Run(ctx, args)
}
