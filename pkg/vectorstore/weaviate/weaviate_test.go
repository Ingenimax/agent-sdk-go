package weaviate_test

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/embedding"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	weaviatestore "github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/weaviate"
)

// MockEmbedder implements a simple mock embedding client for testing
type MockEmbedder struct{}

func (m *MockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}

func (m *MockEmbedder) EmbedWithConfig(ctx context.Context, text string, config embedding.EmbeddingConfig) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}

func (m *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2, 0.3}
	}
	return result, nil
}

func (m *MockEmbedder) EmbedBatchWithConfig(ctx context.Context, texts []string, config embedding.EmbeddingConfig) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2, 0.3}
	}
	return result, nil
}

func (m *MockEmbedder) CalculateSimilarity(vec1, vec2 []float32, metric string) (float32, error) {
	return 0.95, nil
}

func TestStore(t *testing.T) {
	// Skip test when running in CI or if no Weaviate instance available
	t.Skip("Skipping test that requires a Weaviate instance")

	config := &interfaces.VectorStoreConfig{
		Host:   "localhost:8080",
		Scheme: "http",
	}

	mockEmbedder := &MockEmbedder{}
	store := weaviatestore.New(config,
		weaviatestore.WithClassPrefix("Document"),
		weaviatestore.WithEmbedder(mockEmbedder),
	)

	ctx := multitenancy.WithOrgID(context.Background(), "test-org")

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
	if err != nil {
		t.Fatalf("Failed to store documents: %v", err)
	}

	// Test searching
	results, err := store.Search(ctx, "test document", 2)
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	// Test getting documents
	retrieved, err := store.Get(ctx, "doc1")
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}

	if retrieved == nil {
		t.Fatalf("Expected document, got nil")
		return // This return is never reached but helps linter understand
	}

	if retrieved.Content != docs[0].Content {
		t.Errorf("Expected content %q, got %q", docs[0].Content, retrieved.Content)
	}

	// Test deleting
	err = store.Delete(ctx, []string{"doc1", "doc2"})
	if err != nil {
		t.Fatalf("Failed to delete documents: %v", err)
	}

	// Verify deletion
	results, err = store.Search(ctx, "test document", 2)
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results after deletion, got %d", len(results))
	}
}
