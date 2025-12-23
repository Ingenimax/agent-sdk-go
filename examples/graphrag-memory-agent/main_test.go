// Package main contains tests for the GraphRAG memory agent example.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// MockGraphRAGStore implements interfaces.GraphRAGStore for testing.
type MockGraphRAGStore struct {
	entities      map[string]interfaces.Entity
	relationships map[string]interfaces.Relationship
	tenant        string
	searchResults []interfaces.GraphSearchResult
	searchErr     error
	deleteErr     error
}

// NewMockGraphRAGStore creates a new mock store.
func NewMockGraphRAGStore() *MockGraphRAGStore {
	return &MockGraphRAGStore{
		entities:      make(map[string]interfaces.Entity),
		relationships: make(map[string]interfaces.Relationship),
	}
}

// StoreEntities stores entities in the mock.
func (m *MockGraphRAGStore) StoreEntities(ctx context.Context, entities []interfaces.Entity, options ...interfaces.GraphStoreOption) error {
	for _, e := range entities {
		if e.ID == "" {
			return fmt.Errorf("entity ID is required")
		}
		m.entities[e.ID] = e
	}
	return nil
}

// GetEntity retrieves an entity from the mock.
func (m *MockGraphRAGStore) GetEntity(ctx context.Context, id string, options ...interfaces.GraphStoreOption) (*interfaces.Entity, error) {
	if e, ok := m.entities[id]; ok {
		return &e, nil
	}
	return nil, fmt.Errorf("entity not found: %s", id)
}

// UpdateEntity updates an entity in the mock.
func (m *MockGraphRAGStore) UpdateEntity(ctx context.Context, entity interfaces.Entity, options ...interfaces.GraphStoreOption) error {
	if _, ok := m.entities[entity.ID]; !ok {
		return fmt.Errorf("entity not found: %s", entity.ID)
	}
	m.entities[entity.ID] = entity
	return nil
}

// DeleteEntity deletes an entity from the mock.
func (m *MockGraphRAGStore) DeleteEntity(ctx context.Context, id string, options ...interfaces.GraphStoreOption) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.entities[id]; !ok {
		return fmt.Errorf("entity not found: %s", id)
	}
	delete(m.entities, id)
	return nil
}

// StoreRelationships stores relationships in the mock.
func (m *MockGraphRAGStore) StoreRelationships(ctx context.Context, relationships []interfaces.Relationship, options ...interfaces.GraphStoreOption) error {
	for _, r := range relationships {
		if r.SourceID == "" {
			return fmt.Errorf("source ID is required")
		}
		m.relationships[r.ID] = r
	}
	return nil
}

// GetRelationships retrieves relationships from the mock.
func (m *MockGraphRAGStore) GetRelationships(ctx context.Context, entityID string, direction interfaces.RelationshipDirection, options ...interfaces.GraphSearchOption) ([]interfaces.Relationship, error) {
	var results []interfaces.Relationship
	for _, r := range m.relationships {
		switch direction {
		case interfaces.DirectionOutgoing:
			if r.SourceID == entityID {
				results = append(results, r)
			}
		case interfaces.DirectionIncoming:
			if r.TargetID == entityID {
				results = append(results, r)
			}
		case interfaces.DirectionBoth:
			if r.SourceID == entityID || r.TargetID == entityID {
				results = append(results, r)
			}
		}
	}
	return results, nil
}

// DeleteRelationship deletes a relationship from the mock.
func (m *MockGraphRAGStore) DeleteRelationship(ctx context.Context, id string, options ...interfaces.GraphStoreOption) error {
	delete(m.relationships, id)
	return nil
}

// Search searches the mock store.
func (m *MockGraphRAGStore) Search(ctx context.Context, query string, limit int, options ...interfaces.GraphSearchOption) ([]interfaces.GraphSearchResult, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	// If custom results are set, return them
	if len(m.searchResults) > 0 {
		return m.searchResults, nil
	}
	// Otherwise, return all entities as results
	var results []interfaces.GraphSearchResult
	for _, e := range m.entities {
		results = append(results, interfaces.GraphSearchResult{
			Entity: e,
			Score:  1.0,
		})
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// LocalSearch performs local search in the mock.
func (m *MockGraphRAGStore) LocalSearch(ctx context.Context, query string, entityID string, depth int, options ...interfaces.GraphSearchOption) ([]interfaces.GraphSearchResult, error) {
	return m.Search(ctx, query, 10, options...)
}

// GlobalSearch performs global search in the mock.
func (m *MockGraphRAGStore) GlobalSearch(ctx context.Context, query string, communityLevel int, options ...interfaces.GraphSearchOption) ([]interfaces.GraphSearchResult, error) {
	return m.Search(ctx, query, 10, options...)
}

// TraverseFrom traverses the graph from an entity.
func (m *MockGraphRAGStore) TraverseFrom(ctx context.Context, entityID string, depth int, options ...interfaces.GraphSearchOption) (*interfaces.GraphContext, error) {
	e, err := m.GetEntity(ctx, entityID)
	if err != nil {
		return nil, err
	}
	return &interfaces.GraphContext{
		CentralEntity: *e,
		Entities:      []interfaces.Entity{*e},
		Depth:         depth,
	}, nil
}

// ShortestPath finds the shortest path between entities.
func (m *MockGraphRAGStore) ShortestPath(ctx context.Context, sourceID, targetID string, options ...interfaces.GraphSearchOption) (*interfaces.GraphPath, error) {
	source, err := m.GetEntity(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	target, err := m.GetEntity(ctx, targetID)
	if err != nil {
		return nil, err
	}
	return &interfaces.GraphPath{
		Source: *source,
		Target: *target,
		Length: 1,
	}, nil
}

// ExtractFromText extracts entities from text.
func (m *MockGraphRAGStore) ExtractFromText(ctx context.Context, text string, llm interfaces.LLM, options ...interfaces.ExtractionOption) (*interfaces.ExtractionResult, error) {
	return &interfaces.ExtractionResult{
		SourceText: text,
		Confidence: 1.0,
	}, nil
}

// ApplySchema applies a schema.
func (m *MockGraphRAGStore) ApplySchema(ctx context.Context, schema interfaces.GraphSchema) error {
	return nil
}

// DiscoverSchema discovers the schema.
func (m *MockGraphRAGStore) DiscoverSchema(ctx context.Context) (*interfaces.GraphSchema, error) {
	return &interfaces.GraphSchema{}, nil
}

// SetTenant sets the tenant.
func (m *MockGraphRAGStore) SetTenant(tenant string) {
	m.tenant = tenant
}

// GetTenant gets the tenant.
func (m *MockGraphRAGStore) GetTenant() string {
	return m.tenant
}

// Close closes the mock store.
func (m *MockGraphRAGStore) Close() error {
	return nil
}

// --- Test Cases ---

func TestShowMemory_EmptyStore(t *testing.T) {
	store := NewMockGraphRAGStore()
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "test-user")

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	showMemory(ctx, store)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "Current Memory Contents")
	assert.Contains(t, output, "Memory is empty")
}

func TestShowMemory_WithEntities(t *testing.T) {
	store := NewMockGraphRAGStore()
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "test-user")

	// Add some entities
	now := time.Now()
	err := store.StoreEntities(ctx, []interfaces.Entity{
		{ID: "person-alex", Name: "Alex", Type: "Person", Description: "Software engineer", CreatedAt: now, UpdatedAt: now},
		{ID: "org-acme", Name: "Acme Corp", Type: "Organization", Description: "Tech company", CreatedAt: now, UpdatedAt: now},
		{ID: "project-phoenix", Name: "Phoenix", Type: "Project", Description: "Main project", CreatedAt: now, UpdatedAt: now},
	})
	require.NoError(t, err)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	showMemory(ctx, store)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "Current Memory Contents")
	assert.Contains(t, output, "Alex")
	assert.Contains(t, output, "Acme Corp")
	assert.Contains(t, output, "Phoenix")
}

func TestShowMemory_WithRelationships(t *testing.T) {
	store := NewMockGraphRAGStore()
	ctx := context.Background()

	now := time.Now()

	// Add entity with relationships in the search result
	entity := interfaces.Entity{
		ID:          "person-alex",
		Name:        "Alex",
		Type:        "Person",
		Description: "Software engineer",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	relationship := interfaces.Relationship{
		ID:       "rel-1",
		SourceID: "person-alex",
		TargetID: "org-acme",
		Type:     "WORKS_AT",
	}

	// Set custom search results with relationships
	store.searchResults = []interfaces.GraphSearchResult{
		{
			Entity: entity,
			Score:  1.0,
			Path:   []interfaces.Relationship{relationship},
		},
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	showMemory(ctx, store)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "Alex")
	assert.Contains(t, output, "WORKS_AT")
	assert.Contains(t, output, "org-acme")
}

func TestShowMemory_SearchError(t *testing.T) {
	store := NewMockGraphRAGStore()
	store.searchErr = fmt.Errorf("search failed")
	ctx := context.Background()

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	showMemory(ctx, store)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "Could not retrieve memory")
}

func TestClearMemory_EmptyStore(t *testing.T) {
	store := NewMockGraphRAGStore()
	ctx := context.Background()

	// Should not panic on empty store
	clearMemory(ctx, store)

	assert.Empty(t, store.entities)
}

func TestClearMemory_WithEntities(t *testing.T) {
	store := NewMockGraphRAGStore()
	ctx := context.Background()

	now := time.Now()

	// Add some entities
	err := store.StoreEntities(ctx, []interfaces.Entity{
		{ID: "person-alex", Name: "Alex", Type: "Person", Description: "Test", CreatedAt: now, UpdatedAt: now},
		{ID: "org-acme", Name: "Acme", Type: "Organization", Description: "Test", CreatedAt: now, UpdatedAt: now},
	})
	require.NoError(t, err)
	assert.Len(t, store.entities, 2)

	// Clear memory
	clearMemory(ctx, store)

	// All entities should be deleted
	assert.Empty(t, store.entities)
}

func TestClearMemory_SearchError(t *testing.T) {
	store := NewMockGraphRAGStore()
	store.searchErr = fmt.Errorf("search failed")
	ctx := context.Background()

	// Add an entity directly
	now := time.Now()
	store.entities["test"] = interfaces.Entity{ID: "test", Name: "Test", Type: "Test", CreatedAt: now, UpdatedAt: now}

	// Should not panic, just return early
	clearMemory(ctx, store)

	// Entity should still exist because search failed
	assert.Len(t, store.entities, 1)
}

func TestCommandParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // "quit", "memory", "clear", or "chat"
	}{
		{"quit lowercase", "quit", "quit"},
		{"quit uppercase", "QUIT", "quit"},
		{"exit lowercase", "exit", "quit"},
		{"exit uppercase", "EXIT", "quit"},
		{"memory lowercase", "memory", "memory"},
		{"memory uppercase", "MEMORY", "memory"},
		{"show memory", "show memory", "memory"},
		{"what do you remember", "what do you remember", "memory"},
		{"clear lowercase", "clear", "clear"},
		{"clear uppercase", "CLEAR", "clear"},
		{"forget everything", "forget everything", "clear"},
		{"reset", "reset", "clear"},
		{"regular chat", "hello there", "chat"},
		{"question", "what is my name?", "chat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommand(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// parseCommand is a helper function that mirrors the command parsing logic in main()
func parseCommand(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "quit", "exit":
		return "quit"
	case "memory", "show memory", "what do you remember":
		return "memory"
	case "clear", "forget everything", "reset":
		return "clear"
	default:
		return "chat"
	}
}

func TestContextSetup(t *testing.T) {
	t.Run("sets organization ID in context", func(t *testing.T) {
		ctx := context.Background()
		userID := "test-user-123"
		ctx = multitenancy.WithOrgID(ctx, userID)

		orgID, err := multitenancy.GetOrgID(ctx)
		require.NoError(t, err)
		assert.Equal(t, userID, orgID)
	})

	t.Run("sets conversation ID in context", func(t *testing.T) {
		ctx := context.Background()
		sessionID := fmt.Sprintf("session-%d", time.Now().Unix())
		ctx = memory.WithConversationID(ctx, sessionID)

		convID, ok := memory.GetConversationID(ctx)
		assert.True(t, ok)
		assert.Equal(t, sessionID, convID)
	})

	t.Run("both IDs can coexist in context", func(t *testing.T) {
		ctx := context.Background()

		userID := "test-user-456"
		ctx = multitenancy.WithOrgID(ctx, userID)

		sessionID := fmt.Sprintf("session-%d", time.Now().Unix())
		ctx = memory.WithConversationID(ctx, sessionID)

		// Both should be retrievable
		orgID, err := multitenancy.GetOrgID(ctx)
		require.NoError(t, err)
		assert.Equal(t, userID, orgID)

		convID, ok := memory.GetConversationID(ctx)
		assert.True(t, ok)
		assert.Equal(t, sessionID, convID)
	})
}

func TestMemoryAgentPrompt(t *testing.T) {
	t.Run("contains required tool names", func(t *testing.T) {
		assert.Contains(t, memoryAgentPrompt, "graphrag_search")
		assert.Contains(t, memoryAgentPrompt, "graphrag_add_entity")
		assert.Contains(t, memoryAgentPrompt, "graphrag_add_relationship")
		assert.Contains(t, memoryAgentPrompt, "graphrag_get_context")
		assert.Contains(t, memoryAgentPrompt, "graphrag_extract")
	})

	t.Run("contains entity type guidance", func(t *testing.T) {
		assert.Contains(t, memoryAgentPrompt, "Person")
		assert.Contains(t, memoryAgentPrompt, "Organization")
		assert.Contains(t, memoryAgentPrompt, "Project")
		assert.Contains(t, memoryAgentPrompt, "Skill")
		assert.Contains(t, memoryAgentPrompt, "Location")
	})

	t.Run("contains relationship type guidance", func(t *testing.T) {
		assert.Contains(t, memoryAgentPrompt, "WORKS_AT")
		assert.Contains(t, memoryAgentPrompt, "WORKS_ON")
		assert.Contains(t, memoryAgentPrompt, "MANAGES")
		assert.Contains(t, memoryAgentPrompt, "KNOWS")
	})

	t.Run("contains example workflow", func(t *testing.T) {
		assert.Contains(t, memoryAgentPrompt, "Example Workflow")
		assert.Contains(t, memoryAgentPrompt, "TechCorp")
	})
}

func TestGroupByType(t *testing.T) {
	results := []interfaces.GraphSearchResult{
		{Entity: interfaces.Entity{ID: "1", Name: "Alex", Type: "Person"}},
		{Entity: interfaces.Entity{ID: "2", Name: "Bob", Type: "Person"}},
		{Entity: interfaces.Entity{ID: "3", Name: "Acme", Type: "Organization"}},
		{Entity: interfaces.Entity{ID: "4", Name: "Phoenix", Type: "Project"}},
	}

	// Group by type (same logic as showMemory)
	byType := make(map[string][]interfaces.GraphSearchResult)
	for _, r := range results {
		byType[r.Entity.Type] = append(byType[r.Entity.Type], r)
	}

	assert.Len(t, byType["Person"], 2)
	assert.Len(t, byType["Organization"], 1)
	assert.Len(t, byType["Project"], 1)
}

func TestMockGraphRAGStore_BasicOperations(t *testing.T) {
	store := NewMockGraphRAGStore()
	ctx := context.Background()
	now := time.Now()

	t.Run("store and retrieve entity", func(t *testing.T) {
		entity := interfaces.Entity{
			ID:          "test-1",
			Name:        "Test Entity",
			Type:        "Test",
			Description: "A test entity",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		err := store.StoreEntities(ctx, []interfaces.Entity{entity})
		require.NoError(t, err)

		retrieved, err := store.GetEntity(ctx, "test-1")
		require.NoError(t, err)
		assert.Equal(t, "Test Entity", retrieved.Name)
	})

	t.Run("update entity", func(t *testing.T) {
		entity := interfaces.Entity{
			ID:          "test-2",
			Name:        "Original",
			Type:        "Test",
			Description: "Original description",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		err := store.StoreEntities(ctx, []interfaces.Entity{entity})
		require.NoError(t, err)

		entity.Name = "Updated"
		err = store.UpdateEntity(ctx, entity)
		require.NoError(t, err)

		retrieved, err := store.GetEntity(ctx, "test-2")
		require.NoError(t, err)
		assert.Equal(t, "Updated", retrieved.Name)
	})

	t.Run("delete entity", func(t *testing.T) {
		entity := interfaces.Entity{
			ID:          "test-3",
			Name:        "To Delete",
			Type:        "Test",
			Description: "Will be deleted",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		err := store.StoreEntities(ctx, []interfaces.Entity{entity})
		require.NoError(t, err)

		err = store.DeleteEntity(ctx, "test-3")
		require.NoError(t, err)

		_, err = store.GetEntity(ctx, "test-3")
		assert.Error(t, err)
	})

	t.Run("store and retrieve relationships", func(t *testing.T) {
		rel := interfaces.Relationship{
			ID:       "rel-1",
			SourceID: "entity-a",
			TargetID: "entity-b",
			Type:     "CONNECTED",
			Strength: 0.8,
		}

		err := store.StoreRelationships(ctx, []interfaces.Relationship{rel})
		require.NoError(t, err)

		rels, err := store.GetRelationships(ctx, "entity-a", interfaces.DirectionOutgoing)
		require.NoError(t, err)
		assert.Len(t, rels, 1)
		assert.Equal(t, "CONNECTED", rels[0].Type)
	})
}

func TestMockGraphRAGStore_TenantManagement(t *testing.T) {
	store := NewMockGraphRAGStore()

	t.Run("set and get tenant", func(t *testing.T) {
		store.SetTenant("org-123")
		assert.Equal(t, "org-123", store.GetTenant())

		store.SetTenant("org-456")
		assert.Equal(t, "org-456", store.GetTenant())
	})
}

func TestMockGraphRAGStore_GraphTraversal(t *testing.T) {
	store := NewMockGraphRAGStore()
	ctx := context.Background()
	now := time.Now()

	entity := interfaces.Entity{
		ID:          "center",
		Name:        "Center Node",
		Type:        "Node",
		Description: "Central entity",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err := store.StoreEntities(ctx, []interfaces.Entity{entity})
	require.NoError(t, err)

	t.Run("traverse from entity", func(t *testing.T) {
		graphCtx, err := store.TraverseFrom(ctx, "center", 2)
		require.NoError(t, err)
		assert.Equal(t, "center", graphCtx.CentralEntity.ID)
	})

	t.Run("traverse from non-existent entity", func(t *testing.T) {
		_, err := store.TraverseFrom(ctx, "non-existent", 2)
		assert.Error(t, err)
	})
}

func TestMockGraphRAGStore_ShortestPath(t *testing.T) {
	store := NewMockGraphRAGStore()
	ctx := context.Background()
	now := time.Now()

	entities := []interfaces.Entity{
		{ID: "source", Name: "Source", Type: "Node", CreatedAt: now, UpdatedAt: now},
		{ID: "target", Name: "Target", Type: "Node", CreatedAt: now, UpdatedAt: now},
	}
	err := store.StoreEntities(ctx, entities)
	require.NoError(t, err)

	t.Run("finds path between entities", func(t *testing.T) {
		path, err := store.ShortestPath(ctx, "source", "target")
		require.NoError(t, err)
		assert.Equal(t, "source", path.Source.ID)
		assert.Equal(t, "target", path.Target.ID)
	})

	t.Run("returns error for non-existent source", func(t *testing.T) {
		_, err := store.ShortestPath(ctx, "non-existent", "target")
		assert.Error(t, err)
	})
}
