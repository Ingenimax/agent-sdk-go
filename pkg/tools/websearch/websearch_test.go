package websearch_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

func TestWebSearch(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		// Check query parameters
		if !strings.Contains(r.URL.String(), "key=test-key") {
			t.Errorf("Expected API key parameter, not found in URL: %s", r.URL.String())
		}

		if !strings.Contains(r.URL.String(), "cx=test-engine") {
			t.Errorf("Expected search engine ID parameter, not found in URL: %s", r.URL.String())
		}

		query := r.URL.Query().Get("q")
		if query != "test query" {
			t.Errorf("Expected query 'test query', got '%s'", query)
		}

		// Check organization ID header
		orgID := r.Header.Get("X-Organization-ID")
		if orgID != "test-org" {
			t.Errorf("Expected organization ID 'test-org', got '%s'", orgID)
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"title":       "Test Result 1",
					"link":        "https://example.com/1",
					"snippet":     "This is the first test result.",
					"displayLink": "example.com",
				},
				{
					"title":       "Test Result 2",
					"link":        "https://example.com/2",
					"snippet":     "This is the second test result.",
					"displayLink": "example.com",
				},
			},
		})
	}))
	defer server.Close()

	// Create tool with custom base URL
	tool := websearch.New(
		"test-key",
		"test-engine",
		websearch.WithHTTPClient(server.Client()),
		websearch.WithBaseURL(server.URL), // Add this option to override the Google API URL
	)

	// Create context with organization ID
	ctx := multitenancy.WithOrgID(context.Background(), "test-org")

	// Test with string input
	result, err := tool.Run(ctx, "test query")
	if err != nil {
		t.Fatalf("Failed to run tool: %v", err)
	}

	// Verify result
	if !contains(result, "Test Result 1") {
		t.Errorf("Expected result to contain 'Test Result 1', got '%s'", result)
	}
	if !contains(result, "Test Result 2") {
		t.Errorf("Expected result to contain 'Test Result 2', got '%s'", result)
	}

	// Test with JSON input
	input := `{"query": "test query", "num_results": 2}`
	result, err = tool.Run(ctx, input)
	if err != nil {
		t.Fatalf("Failed to run tool: %v", err)
	}

	// Verify result
	if !contains(result, "Test Result 1") {
		t.Errorf("Expected result to contain 'Test Result 1', got '%s'", result)
	}
	if !contains(result, "Test Result 2") {
		t.Errorf("Expected result to contain 'Test Result 2', got '%s'", result)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
