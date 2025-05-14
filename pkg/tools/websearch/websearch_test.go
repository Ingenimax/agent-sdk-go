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

		// Check for query parameters
		if !strings.Contains(r.URL.String(), "key=") || !strings.Contains(r.URL.String(), "cx=") {
			t.Errorf("Request URL does not contain required parameters: %s", r.URL.String())
		}

		// Send response with status 200
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(map[string]interface{}{
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
		if err != nil {
			t.Fatalf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create a custom client that redirects all requests to our test server
	client := &http.Client{
		Transport: &mockTransport{
			server: server,
		},
	}

	// Create tool with our mock client
	tool := websearch.New(
		"test-key",
		"test-engine",
		websearch.WithHTTPClient(client),
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

// mockTransport redirects all requests to the test server
type mockTransport struct {
	server *httptest.Server
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Create a new URL pointing to our test server but maintaining the path and query
	newURL := t.server.URL + req.URL.Path
	if req.URL.RawQuery != "" {
		newURL += "?" + req.URL.RawQuery
	}

	// Create a new request to the test server
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}

	// Copy headers
	newReq.Header = req.Header

	// Make the request to our test server
	return t.server.Client().Transport.RoundTrip(newReq)
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
