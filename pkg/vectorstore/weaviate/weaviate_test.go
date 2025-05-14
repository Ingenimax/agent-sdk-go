package weaviate_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	weaviatestore "github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/weaviate"
	"github.com/stretchr/testify/assert"
)

func setupMockServer(t *testing.T) *httptest.Server {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Respond to schema request
		if r.URL.Path == "/v1/schema" {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"classes": []map[string]interface{}{
					{
						"class": "Document_test-org",
						"properties": []map[string]interface{}{
							{"name": "content", "dataType": []string{"text"}},
							{"name": "metadata", "dataType": []string{"object"}},
						},
					},
				},
			}); err != nil {
				t.Errorf("Failed to encode schema response: %v", err)
			}
			return
		}

		// Respond to class creation request
		if r.Method == "POST" && r.URL.Path == "/v1/schema" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Respond to object creation
		if r.Method == "POST" && r.URL.Path == "/v1/objects" {
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "doc1",
			}); err != nil {
				t.Errorf("Failed to encode object creation response: %v", err)
			}
			return
		}

		// Respond to search request
		if r.Method == "POST" && r.URL.Path == "/v1/graphql" {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"Get": map[string]interface{}{
						"Document_test-org": []map[string]interface{}{
							{
								"id":      "doc1",
								"content": "This is a test document",
								"metadata": map[string]interface{}{
									"source": "test",
								},
							},
							{
								"id":      "doc2",
								"content": "This is another test document",
								"metadata": map[string]interface{}{
									"source": "test",
								},
							},
						},
					},
				},
			}); err != nil {
				t.Errorf("Failed to encode search response: %v", err)
			}
			return
		}

		// Respond to get request
		if r.Method == "GET" && r.URL.Path == "/v1/objects/Document_test-org/doc1" {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "doc1",
				"properties": map[string]interface{}{
					"content": "This is a test document",
					"metadata": map[string]interface{}{
						"source": "test",
					},
				},
			}); err != nil {
				t.Errorf("Failed to encode get response: %v", err)
			}
			return
		}

		// Respond to delete request
		if r.Method == "DELETE" && (r.URL.Path == "/v1/objects/Document_test-org/doc1" ||
			r.URL.Path == "/v1/objects/Document_test-org/doc2") {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Default response
		w.WriteHeader(http.StatusNotFound)
	}))

	return server
}

func setupTestClient(t *testing.T) (*interfaces.VectorStoreConfig, *httptest.Server) {
	server := setupMockServer(t)

	// Parse the server URL to extract host without scheme
	host := server.URL[7:] // Remove "http://" prefix

	return &interfaces.VectorStoreConfig{
		Host:   host, // Just the host without scheme
		APIKey: "test-key",
		Scheme: "http", // Specify scheme separately
	}, server
}

func TestStore(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	config, server := setupTestClient(t)
	defer server.Close()

	// Create store with logger
	store := weaviatestore.New(config, weaviatestore.WithLogger(logging.NewNoOpLogger()))

	// Use a context with timeout for safety
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ctx = multitenancy.WithOrgID(ctx, "test-org")

	// Test storing documents
	docs := []interfaces.Document{
		{
			ID:      "doc1",
			Content: "This is a test document",
			Metadata: map[string]interface{}{
				"source": "test",
			},
		},
		{
			ID:      "doc2",
			Content: "This is another test document",
			Metadata: map[string]interface{}{
				"source": "test",
			},
		},
	}

	err := store.Store(ctx, docs)
	assert.NoError(t, err)

	// Test searching
	results, err := store.Search(ctx, "test document", 2)
	assert.NoError(t, err)
	assert.Len(t, results, 2)

	// Test getting documents
	retrieved, err := store.Get(ctx, []string{"doc1"})
	assert.NoError(t, err)
	assert.Len(t, retrieved, 1)
	assert.Equal(t, docs[0].Content, retrieved[0].Content)

	// Test deleting
	err = store.Delete(ctx, []string{"doc1", "doc2"})
	assert.NoError(t, err)
}
