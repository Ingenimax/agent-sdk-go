package multitenancy_test

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/embedding"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// MockEmbedder implements a simple mock for testing
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

// MockVectorStore implements a simple mock VectorStore for testing
type MockVectorStore struct {
	documents map[string][]interfaces.Document
	lastOrgID string
}

func NewMockVectorStore() *MockVectorStore {
	return &MockVectorStore{
		documents: make(map[string][]interfaces.Document),
	}
}

func (s *MockVectorStore) Store(ctx context.Context, documents []interfaces.Document, options ...interfaces.StoreOption) error {
	// Get organization ID
	orgID, _ := multitenancy.GetOrgID(ctx)
	s.lastOrgID = orgID

	// Create key for this organization
	key := "docs_" + orgID
	s.documents[key] = documents
	return nil
}

func (s *MockVectorStore) Search(ctx context.Context, query string, limit int, options ...interfaces.SearchOption) ([]interfaces.SearchResult, error) {
	return nil, nil
}

func (s *MockVectorStore) SearchByVector(ctx context.Context, vector []float32, limit int, options ...interfaces.SearchOption) ([]interfaces.SearchResult, error) {
	return nil, nil
}

func (s *MockVectorStore) Delete(ctx context.Context, ids []string, options ...interfaces.DeleteOption) error {
	return nil
}

func (s *MockVectorStore) Get(ctx context.Context, ids []string) ([]interfaces.Document, error) {
	return nil, nil
}

func TestMultiTenancy(t *testing.T) {
	// Create a config manager
	configManager := multitenancy.NewConfigManager()

	// Register two tenants
	err := configManager.RegisterTenant(&multitenancy.TenantConfig{
		OrgID: "org1",
		LLMAPIKeys: map[string]string{
			"openai": "org1-api-key",
		},
	})
	if err != nil {
		t.Fatalf("Failed to register tenant: %v", err)
	}

	err = configManager.RegisterTenant(&multitenancy.TenantConfig{
		OrgID: "org2",
		LLMAPIKeys: map[string]string{
			"openai": "org2-api-key",
		},
	})
	if err != nil {
		t.Fatalf("Failed to register tenant: %v", err)
	}

	// Create contexts for each organization
	ctx1 := multitenancy.WithOrgID(context.Background(), "org1")
	ctx2 := multitenancy.WithOrgID(context.Background(), "org2")

	// Test that we can get the correct API keys for each organization
	apiKey1, err := configManager.GetLLMAPIKey(ctx1, "openai")
	if err != nil {
		t.Fatalf("Failed to get API key: %v", err)
	}
	if apiKey1 != "org1-api-key" {
		t.Errorf("Expected API key 'org1-api-key', got '%s'", apiKey1)
	}

	apiKey2, err := configManager.GetLLMAPIKey(ctx2, "openai")
	if err != nil {
		t.Fatalf("Failed to get API key: %v", err)
	}
	if apiKey2 != "org2-api-key" {
		t.Errorf("Expected API key 'org2-api-key', got '%s'", apiKey2)
	}

	// Create a mock vector store
	store := NewMockVectorStore()

	// Test with first organization
	docs1 := []interfaces.Document{
		{
			ID:      "doc1",
			Content: "First organization test document",
		},
	}
	err = store.Store(ctx1, docs1)
	if err != nil {
		t.Fatalf("Failed to store documents for org1: %v", err)
	}

	if store.lastOrgID != "org1" {
		t.Errorf("Expected store operation to use org1, got '%s'", store.lastOrgID)
	}

	// Test with second organization
	docs2 := []interfaces.Document{
		{
			ID:      "doc2",
			Content: "Second organization test document",
		},
	}
	err = store.Store(ctx2, docs2)
	if err != nil {
		t.Fatalf("Failed to store documents for org2: %v", err)
	}

	if store.lastOrgID != "org2" {
		t.Errorf("Expected store operation to use org2, got '%s'", store.lastOrgID)
	}

	// Verify that documents were stored separately
	if len(store.documents["docs_org1"]) != 1 ||
		len(store.documents["docs_org2"]) != 1 {
		t.Errorf("Documents not stored properly by organization")
	}

	if store.documents["docs_org1"][0].ID != "doc1" ||
		store.documents["docs_org2"][0].ID != "doc2" {
		t.Errorf("Document IDs not stored correctly by organization")
	}
}
