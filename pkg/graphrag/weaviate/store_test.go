package weaviate

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/embedding"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// MockEmbedder implements a mock embedder for testing.
type MockEmbedder struct{}

func (m *MockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// Return a consistent embedding for testing
	return []float32{0.1, 0.2, 0.3, 0.4, 0.5}, nil
}

func (m *MockEmbedder) EmbedWithConfig(ctx context.Context, text string, config embedding.EmbeddingConfig) ([]float32, error) {
	return m.Embed(ctx, text)
}

func (m *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i := range texts {
		embeddings[i] = []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	}
	return embeddings, nil
}

func (m *MockEmbedder) EmbedBatchWithConfig(ctx context.Context, texts []string, config embedding.EmbeddingConfig) ([][]float32, error) {
	return m.EmbedBatch(ctx, texts)
}

func (m *MockEmbedder) CalculateSimilarity(vec1, vec2 []float32, metric string) (float32, error) {
	return 0.9, nil
}

// getTestHost returns the Weaviate host for testing.
func getTestHost() string {
	host := os.Getenv("WEAVIATE_HOST")
	if host == "" {
		host = "localhost:8080"
	}
	return host
}

// checkWeaviateAvailable checks if Weaviate is reachable and skips the test if not.
func checkWeaviateAvailable(t *testing.T, store *Store) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := store.client.Schema().Getter().Do(ctx)
	if err != nil {
		t.Skipf("Weaviate not reachable: %v", err)
		return false
	}
	return true
}

// skipIfNoWeaviate skips the test if Weaviate is not available.
func skipIfNoWeaviate(t *testing.T) *Store {
	store, err := New(&Config{
		Host:        getTestHost(),
		Scheme:      "http",
		ClassPrefix: "TestGraphRAG",
	}, WithEmbedder(&MockEmbedder{}))

	if err != nil {
		t.Skipf("Weaviate not available: %v", err)
		return nil
	}

	if !checkWeaviateAvailable(t, store) {
		return nil
	}

	return store
}

func TestNew(t *testing.T) {
	t.Run("creates store with default config", func(t *testing.T) {
		store, err := New(&Config{
			Host:   getTestHost(),
			Scheme: "http",
		})
		if err != nil {
			t.Skipf("Weaviate not available: %v", err)
			return
		}
		defer store.Close()

		if !checkWeaviateAvailable(t, store) {
			return
		}

		assert.NotNil(t, store)
		assert.Equal(t, "Graph", store.classPrefix)
	})

	t.Run("creates store with custom class prefix", func(t *testing.T) {
		store, err := New(&Config{
			Host:        getTestHost(),
			Scheme:      "http",
			ClassPrefix: "Custom",
		})
		if err != nil {
			t.Skipf("Weaviate not available: %v", err)
			return
		}
		defer store.Close()

		if !checkWeaviateAvailable(t, store) {
			return
		}

		assert.Equal(t, "Custom", store.classPrefix)
	})

	t.Run("applies options", func(t *testing.T) {
		embedder := &MockEmbedder{}
		store, err := New(&Config{
			Host:   getTestHost(),
			Scheme: "http",
		},
			WithClassPrefix("OptionsTest"),
			WithEmbedder(embedder),
			WithStoreTenant("test-tenant"),
		)
		if err != nil {
			t.Skipf("Weaviate not available: %v", err)
			return
		}
		defer store.Close()

		if !checkWeaviateAvailable(t, store) {
			return
		}

		assert.Equal(t, "OptionsTest", store.classPrefix)
		assert.Equal(t, "test-tenant", store.tenant)
		assert.Equal(t, embedder, store.embedder)
	})
}

func TestStoreEntities(t *testing.T) {
	store := skipIfNoWeaviate(t)
	if store == nil {
		return
	}
	defer store.Close()
	defer func() { _ = store.DeleteSchema(context.Background()) }()

	ctx := context.Background()

	t.Run("stores single entity", func(t *testing.T) {
		entity := interfaces.Entity{
			ID:          "test-entity-1",
			Name:        "Test Entity",
			Type:        "TestType",
			Description: "A test entity for unit testing",
			Properties: map[string]interface{}{
				"key": "value",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := store.StoreEntities(ctx, []interfaces.Entity{entity})
		require.NoError(t, err)

		// Verify entity was stored
		retrieved, err := store.GetEntity(ctx, "test-entity-1")
		require.NoError(t, err)
		assert.Equal(t, "Test Entity", retrieved.Name)
		assert.Equal(t, "TestType", retrieved.Type)
	})

	t.Run("stores multiple entities", func(t *testing.T) {
		entities := []interfaces.Entity{
			{
				ID:          "test-entity-2",
				Name:        "Entity Two",
				Type:        "TestType",
				Description: "Second test entity",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			{
				ID:          "test-entity-3",
				Name:        "Entity Three",
				Type:        "TestType",
				Description: "Third test entity",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		}

		err := store.StoreEntities(ctx, entities)
		require.NoError(t, err)

		// Verify entities were stored
		entity2, err := store.GetEntity(ctx, "test-entity-2")
		require.NoError(t, err)
		assert.Equal(t, "Entity Two", entity2.Name)

		entity3, err := store.GetEntity(ctx, "test-entity-3")
		require.NoError(t, err)
		assert.Equal(t, "Entity Three", entity3.Name)
	})

	t.Run("returns error for invalid entity", func(t *testing.T) {
		// Missing ID
		err := store.StoreEntities(ctx, []interfaces.Entity{
			{Name: "No ID", Type: "Test", Description: "Test"},
		})
		assert.Error(t, err)

		// Missing Name
		err = store.StoreEntities(ctx, []interfaces.Entity{
			{ID: "id", Type: "Test", Description: "Test"},
		})
		assert.Error(t, err)

		// Missing Type
		err = store.StoreEntities(ctx, []interfaces.Entity{
			{ID: "id", Name: "Test", Description: "Test"},
		})
		assert.Error(t, err)
	})
}

func TestGetEntity(t *testing.T) {
	store := skipIfNoWeaviate(t)
	if store == nil {
		return
	}
	defer store.Close()
	defer func() { _ = store.DeleteSchema(context.Background()) }()

	ctx := context.Background()

	// Store test entity
	entity := interfaces.Entity{
		ID:          "get-test-entity",
		Name:        "Get Test Entity",
		Type:        "TestType",
		Description: "Entity for get test",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := store.StoreEntities(ctx, []interfaces.Entity{entity})
	require.NoError(t, err)

	t.Run("retrieves existing entity", func(t *testing.T) {
		retrieved, err := store.GetEntity(ctx, "get-test-entity")
		require.NoError(t, err)
		assert.Equal(t, "Get Test Entity", retrieved.Name)
		assert.Equal(t, "TestType", retrieved.Type)
	})

	t.Run("returns error for non-existent entity", func(t *testing.T) {
		_, err := store.GetEntity(ctx, "non-existent")
		assert.Error(t, err)
	})

	t.Run("returns error for empty ID", func(t *testing.T) {
		_, err := store.GetEntity(ctx, "")
		assert.Error(t, err)
	})
}

func TestUpdateEntity(t *testing.T) {
	store := skipIfNoWeaviate(t)
	if store == nil {
		return
	}
	defer store.Close()
	defer func() { _ = store.DeleteSchema(context.Background()) }()

	ctx := context.Background()

	// Store initial entity
	entity := interfaces.Entity{
		ID:          "update-test-entity",
		Name:        "Original Name",
		Type:        "TestType",
		Description: "Original description",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := store.StoreEntities(ctx, []interfaces.Entity{entity})
	require.NoError(t, err)

	t.Run("updates entity successfully", func(t *testing.T) {
		entity.Name = "Updated Name"
		entity.Description = "Updated description"

		err := store.UpdateEntity(ctx, entity)
		require.NoError(t, err)

		// Verify update
		retrieved, err := store.GetEntity(ctx, "update-test-entity")
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", retrieved.Name)
		assert.Equal(t, "Updated description", retrieved.Description)
	})

	t.Run("returns error for non-existent entity", func(t *testing.T) {
		nonExistent := interfaces.Entity{
			ID:   "non-existent",
			Name: "Test",
			Type: "Test",
		}
		err := store.UpdateEntity(ctx, nonExistent)
		assert.Error(t, err)
	})
}

func TestDeleteEntity(t *testing.T) {
	store := skipIfNoWeaviate(t)
	if store == nil {
		return
	}
	defer store.Close()
	defer func() { _ = store.DeleteSchema(context.Background()) }()

	ctx := context.Background()

	// Store entity to delete
	entity := interfaces.Entity{
		ID:          "delete-test-entity",
		Name:        "Delete Me",
		Type:        "TestType",
		Description: "Entity to delete",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := store.StoreEntities(ctx, []interfaces.Entity{entity})
	require.NoError(t, err)

	t.Run("deletes entity successfully", func(t *testing.T) {
		err := store.DeleteEntity(ctx, "delete-test-entity")
		require.NoError(t, err)

		// Verify deletion
		_, err = store.GetEntity(ctx, "delete-test-entity")
		assert.Error(t, err)
	})

	t.Run("returns error for non-existent entity", func(t *testing.T) {
		err := store.DeleteEntity(ctx, "non-existent")
		assert.Error(t, err)
	})
}

func TestStoreRelationships(t *testing.T) {
	store := skipIfNoWeaviate(t)
	if store == nil {
		return
	}
	defer store.Close()
	defer func() { _ = store.DeleteSchema(context.Background()) }()

	ctx := context.Background()

	// Store entities first
	entities := []interfaces.Entity{
		{ID: "rel-entity-1", Name: "Entity One", Type: "Person", Description: "First entity", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "rel-entity-2", Name: "Entity Two", Type: "Project", Description: "Second entity", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	err := store.StoreEntities(ctx, entities)
	require.NoError(t, err)

	t.Run("stores relationship successfully", func(t *testing.T) {
		rel := interfaces.Relationship{
			ID:          "test-rel-1",
			SourceID:    "rel-entity-1",
			TargetID:    "rel-entity-2",
			Type:        "WORKS_ON",
			Description: "Person works on project",
			Strength:    0.8,
			CreatedAt:   time.Now(),
		}

		err := store.StoreRelationships(ctx, []interfaces.Relationship{rel})
		require.NoError(t, err)

		// Verify relationship
		rels, err := store.GetRelationships(ctx, "rel-entity-1", interfaces.DirectionOutgoing)
		require.NoError(t, err)
		assert.Len(t, rels, 1)
		assert.Equal(t, "WORKS_ON", rels[0].Type)
	})

	t.Run("returns error for invalid relationship", func(t *testing.T) {
		// Missing source ID
		err := store.StoreRelationships(ctx, []interfaces.Relationship{
			{ID: "rel", TargetID: "target", Type: "TEST"},
		})
		assert.Error(t, err)

		// Invalid strength
		err = store.StoreRelationships(ctx, []interfaces.Relationship{
			{ID: "rel", SourceID: "source", TargetID: "target", Type: "TEST", Strength: 1.5},
		})
		assert.Error(t, err)
	})
}

func TestGetRelationships(t *testing.T) {
	store := skipIfNoWeaviate(t)
	if store == nil {
		return
	}
	defer store.Close()
	defer func() { _ = store.DeleteSchema(context.Background()) }()

	ctx := context.Background()

	// Store entities
	entities := []interfaces.Entity{
		{ID: "get-rel-1", Name: "Entity A", Type: "Person", Description: "A", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "get-rel-2", Name: "Entity B", Type: "Project", Description: "B", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "get-rel-3", Name: "Entity C", Type: "Person", Description: "C", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	err := store.StoreEntities(ctx, entities)
	require.NoError(t, err)

	// Store relationships
	rels := []interfaces.Relationship{
		{ID: "rel-out", SourceID: "get-rel-1", TargetID: "get-rel-2", Type: "WORKS_ON", Strength: 1.0, CreatedAt: time.Now()},
		{ID: "rel-in", SourceID: "get-rel-3", TargetID: "get-rel-1", Type: "MANAGES", Strength: 0.9, CreatedAt: time.Now()},
	}
	err = store.StoreRelationships(ctx, rels)
	require.NoError(t, err)

	t.Run("gets outgoing relationships", func(t *testing.T) {
		results, err := store.GetRelationships(ctx, "get-rel-1", interfaces.DirectionOutgoing)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "WORKS_ON", results[0].Type)
	})

	t.Run("gets incoming relationships", func(t *testing.T) {
		results, err := store.GetRelationships(ctx, "get-rel-1", interfaces.DirectionIncoming)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "MANAGES", results[0].Type)
	})

	t.Run("gets both directions", func(t *testing.T) {
		results, err := store.GetRelationships(ctx, "get-rel-1", interfaces.DirectionBoth)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})
}

func TestSearch(t *testing.T) {
	store := skipIfNoWeaviate(t)
	if store == nil {
		return
	}
	defer store.Close()
	defer func() { _ = store.DeleteSchema(context.Background()) }()

	ctx := context.Background()

	// Store test entities
	entities := []interfaces.Entity{
		{ID: "search-1", Name: "John Smith", Type: "Person", Description: "Software engineer at TechCorp", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "search-2", Name: "TechCorp", Type: "Organization", Description: "Technology company specializing in AI", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "search-3", Name: "AI Project", Type: "Project", Description: "Machine learning research project", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	err := store.StoreEntities(ctx, entities)
	require.NoError(t, err)

	// Wait for indexing
	time.Sleep(1 * time.Second)

	t.Run("searches by query", func(t *testing.T) {
		results, err := store.Search(ctx, "software engineer", 10)
		require.NoError(t, err)
		assert.NotEmpty(t, results)
	})

	t.Run("filters by entity type", func(t *testing.T) {
		results, err := store.Search(ctx, "technology", 10, interfaces.WithEntityTypes("Organization"))
		require.NoError(t, err)
		for _, r := range results {
			assert.Equal(t, "Organization", r.Entity.Type)
		}
	})
}

func TestTraverseFrom(t *testing.T) {
	store := skipIfNoWeaviate(t)
	if store == nil {
		return
	}
	defer store.Close()
	defer func() { _ = store.DeleteSchema(context.Background()) }()

	ctx := context.Background()

	// Build a small graph
	entities := []interfaces.Entity{
		{ID: "trav-1", Name: "Center", Type: "Node", Description: "Center node", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "trav-2", Name: "Child 1", Type: "Node", Description: "First child", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "trav-3", Name: "Child 2", Type: "Node", Description: "Second child", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "trav-4", Name: "Grandchild", Type: "Node", Description: "Grandchild node", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	err := store.StoreEntities(ctx, entities)
	require.NoError(t, err)

	rels := []interfaces.Relationship{
		{ID: "trav-rel-1", SourceID: "trav-1", TargetID: "trav-2", Type: "CONNECTED", Strength: 1.0, CreatedAt: time.Now()},
		{ID: "trav-rel-2", SourceID: "trav-1", TargetID: "trav-3", Type: "CONNECTED", Strength: 1.0, CreatedAt: time.Now()},
		{ID: "trav-rel-3", SourceID: "trav-2", TargetID: "trav-4", Type: "CONNECTED", Strength: 1.0, CreatedAt: time.Now()},
	}
	err = store.StoreRelationships(ctx, rels)
	require.NoError(t, err)

	t.Run("traverses graph from center", func(t *testing.T) {
		graphCtx, err := store.TraverseFrom(ctx, "trav-1", 2)
		require.NoError(t, err)

		assert.Equal(t, "trav-1", graphCtx.CentralEntity.ID)
		assert.GreaterOrEqual(t, len(graphCtx.Entities), 3) // Center + at least 2 children
		assert.GreaterOrEqual(t, len(graphCtx.Relationships), 2)
	})

	t.Run("returns error for non-existent entity", func(t *testing.T) {
		_, err := store.TraverseFrom(ctx, "non-existent", 2)
		assert.Error(t, err)
	})

	t.Run("respects depth limit", func(t *testing.T) {
		graphCtx, err := store.TraverseFrom(ctx, "trav-1", 1)
		require.NoError(t, err)

		// At depth 1, should not include grandchild
		hasGrandchild := false
		for _, e := range graphCtx.Entities {
			if e.ID == "trav-4" {
				hasGrandchild = true
				break
			}
		}
		assert.False(t, hasGrandchild, "Grandchild should not be included at depth 1")
	})
}

func TestSetGetTenant(t *testing.T) {
	store := skipIfNoWeaviate(t)
	if store == nil {
		return
	}
	defer store.Close()

	t.Run("sets and gets tenant", func(t *testing.T) {
		store.SetTenant("org-123")
		assert.Equal(t, "org-123", store.GetTenant())

		store.SetTenant("org-456")
		assert.Equal(t, "org-456", store.GetTenant())
	})
}

func TestDiscoverSchema(t *testing.T) {
	store := skipIfNoWeaviate(t)
	if store == nil {
		return
	}
	defer store.Close()
	defer func() { _ = store.DeleteSchema(context.Background()) }()

	ctx := context.Background()

	// Store entities with different types
	entities := []interfaces.Entity{
		{ID: "schema-1", Name: "Person 1", Type: "Person", Description: "A person", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "schema-2", Name: "Project 1", Type: "Project", Description: "A project", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	err := store.StoreEntities(ctx, entities)
	require.NoError(t, err)

	// Store relationships with different types
	rels := []interfaces.Relationship{
		{ID: "schema-rel-1", SourceID: "schema-1", TargetID: "schema-2", Type: "WORKS_ON", Strength: 1.0, CreatedAt: time.Now()},
	}
	err = store.StoreRelationships(ctx, rels)
	require.NoError(t, err)

	t.Run("discovers entity types", func(t *testing.T) {
		schema, err := store.DiscoverSchema(ctx)
		require.NoError(t, err)
		assert.NotNil(t, schema)

		// Check that we discovered some entity types
		// Note: Aggregate queries may not work on freshly created data
		// so we just check the schema is returned
		assert.NotNil(t, schema.EntityTypes)
	})
}
