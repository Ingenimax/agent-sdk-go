package tavily

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

const (
	// The base URL should be separate from the search endpoint
	tavilyAPIBaseURL     = "https://api.tavily.com"
	tavilySearchEndpoint = "/search" // Note: Using the correct endpoint path
	defaultTimeout       = 30 * time.Second
)

// TavilyTool is a tool for performing web searches via Tavily API
type TavilyTool struct {
	apiKey string
}

// NewTavilyTool creates a new Tavily search tool with the given API key
func NewTavilyTool(apiKey string) *TavilyTool {
	return &TavilyTool{
		apiKey: apiKey,
	}
}

// Name returns the name of the tool
func (t *TavilyTool) Name() string {
	return "tavily_search"
}

// Description returns a description of what the tool does
func (t *TavilyTool) Description() string {
	return "Search the web for current information on topics, news, and data using the Tavily search engine."
}

// Parameters returns the parameters that the tool accepts
func (t *TavilyTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"query": {
			Type:        "string",
			Description: "The search query to look up information for",
			Required:    true,
		},
		"search_depth": {
			Type:        "string",
			Description: "The depth of search to perform (basic or advanced)",
			Required:    false,
			Default:     "basic",
			Enum:        []interface{}{"basic", "advanced"},
		},
		"include_domains": {
			Type:        "string",
			Description: "Comma-separated list of domains to include in the search",
			Required:    false,
		},
		"exclude_domains": {
			Type:        "string",
			Description: "Comma-separated list of domains to exclude from the search",
			Required:    false,
		},
	}
}

// SearchRequest represents a request to the Tavily search API
type SearchRequest struct {
	Query          string   `json:"query"`
	SearchDepth    string   `json:"search_depth,omitempty"`
	IncludeDomains []string `json:"include_domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
	MaxResults     int      `json:"max_results,omitempty"`
	ApiKey         string   `json:"api_key"` // Some APIs require the key in the payload

}

// SearchResult represents a result from the Tavily search API
type SearchResult struct {
	Results []struct {
		URL     string `json:"url"`
		Content string `json:"content"`
		Title   string `json:"title"`
	} `json:"results"`
	Answer string `json:"answer,omitempty"`
}

// Run executes the tool with the given input
func (t *TavilyTool) Run(ctx context.Context, input string) (string, error) {
	// In this simplified version, we treat the input as the search query
	return t.searchTavily(ctx, input, "basic", nil, nil)
}

// Execute executes the tool with the given arguments
func (t *TavilyTool) Execute(ctx context.Context, args string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	query, ok := params["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("query parameter is required")
	}

	searchDepth := "basic"
	if depth, ok := params["search_depth"].(string); ok && (depth == "basic" || depth == "advanced") {
		searchDepth = depth
	}

	var includeDomains, excludeDomains []string
	if include, ok := params["include_domains"].(string); ok && include != "" {
		// In a real implementation, you'd parse the comma-separated list
		// For simplicity, this is not implemented in this example
	}
	if exclude, ok := params["exclude_domains"].(string); ok && exclude != "" {
		// In a real implementation, you'd parse the comma-separated list
		// For simplicity, this is not implemented in this example
	}

	return t.searchTavily(ctx, query, searchDepth, includeDomains, excludeDomains)
}

// searchTavily performs a search using the Tavily API
func (t *TavilyTool) searchTavily(ctx context.Context, query, searchDepth string, includeDomains, excludeDomains []string) (string, error) {
	tavilySearchURL := tavilyAPIBaseURL + tavilySearchEndpoint

	client := &http.Client{
		Timeout: defaultTimeout,
	}

	// Prepare the search request
	searchReq := SearchRequest{
		Query:          query,
		SearchDepth:    searchDepth,
		IncludeDomains: includeDomains,
		ExcludeDomains: excludeDomains,
		MaxResults:     5, // Reasonable default
		ApiKey:         t.apiKey,
	}

	jsonData, err := json.Marshal(searchReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", tavilySearchURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for error status codes
	if resp.StatusCode != http.StatusOK {
		errorMsg := string(body)
		if errorMsg == "" {
			// If the error message is empty, provide a more helpful message
			switch resp.StatusCode {
			case http.StatusUnauthorized:
				errorMsg = "authentication failed - please check your API key"
			case http.StatusBadRequest:
				errorMsg = "bad request - please check your search parameters"
			default:
				errorMsg = fmt.Sprintf("HTTP status code: %d", resp.StatusCode)
			}
		}
		return "", fmt.Errorf("tavily API error: %s", errorMsg)
	}

	// Parse the response
	var searchResult SearchResult
	if err := json.Unmarshal(body, &searchResult); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Format the results into a readable string
	var result string
	if searchResult.Answer != "" {
		result = "Answer: " + searchResult.Answer + "\n\n"
	}

	result += "Search Results:\n"
	for i, r := range searchResult.Results {
		result += fmt.Sprintf("%d. %s\n   URL: %s\n   %s\n\n", i+1, r.Title, r.URL, r.Content)
	}

	return result, nil
}
