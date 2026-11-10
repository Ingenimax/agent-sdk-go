package weaviate

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

const extractionReply = `Sure, here it is:
{
  "entities": [
    {"name": "Ana", "type": "Person", "description": "an engineer", "properties": {"age": 30}},
    {"name": "Acme", "type": "Organization", "description": "a company"},
    {"name": "", "type": "Person"}
  ],
  "relationships": [
    {"source": "Ana", "target": "Acme", "type": "works_at", "description": "employment", "strength": 0.8},
    {"source": "Ana", "target": "Acme", "type": "founded"},
    {"source": "Ana", "target": "Nobody", "type": "knows"},
    {"source": "Nobody", "target": "Ana", "type": "knows"},
    {"source": "", "target": "Ana", "type": "knows"}
  ],
  "confidence": 0.9
}
Hope this helps.`

func TestExtractFromTextBasics(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t, testutil.NewFakeWeaviate(t))

	_, err := store.ExtractFromText(ctx, "text", nil)
	require.ErrorIs(t, err, graphrag.ErrNoLLM)

	llm := &testutil.FakeLLM{DefaultResponse: "unused"}
	got, err := store.ExtractFromText(ctx, "", llm)
	require.NoError(t, err)
	assert.Empty(t, got.Entities)
	assert.Empty(t, got.Relationships)
	assert.Empty(t, llm.Prompts(), "empty text never reaches the LLM")

	_, err = store.ExtractFromText(ctx, "text", &testutil.FakeLLM{Err: errors.New("down")})
	require.ErrorIs(t, err, graphrag.ErrExtractionFailed)
	require.ErrorContains(t, err, "down")
}

func TestExtractFromTextParsesResponse(t *testing.T) {
	store := newTestStore(t, testutil.NewFakeWeaviate(t))
	llm := &testutil.FakeLLM{DefaultResponse: extractionReply}

	got, err := store.ExtractFromText(context.Background(), "Ana works at Acme.", llm, interfaces.WithDedupThreshold(0))
	require.NoError(t, err)
	assert.Equal(t, "Ana works at Acme.", got.SourceText)
	assert.InDelta(t, 0.9, got.Confidence, 1e-6)

	require.Len(t, got.Entities, 2)
	ana, acme := got.Entities[0], got.Entities[1]
	assert.Equal(t, "Ana", ana.Name)
	assert.Equal(t, "Person", ana.Type)
	assert.Equal(t, "an engineer", ana.Description)
	assert.Equal(t, map[string]interface{}{"age": 30.0}, ana.Properties)
	assert.NotEmpty(t, ana.ID)
	assert.NotEqual(t, ana.ID, acme.ID)
	assert.False(t, ana.CreatedAt.IsZero())

	require.Len(t, got.Relationships, 2, "relationships to unknown entities are dropped")
	works := got.Relationships[0]
	assert.Equal(t, ana.ID, works.SourceID)
	assert.Equal(t, acme.ID, works.TargetID)
	assert.Equal(t, "WORKS_AT", works.Type)
	assert.InDelta(t, 0.8, works.Strength, 1e-6)
	assert.InDelta(t, 1.0, got.Relationships[1].Strength, 1e-6, "missing strength defaults to 1.0")

	assert.Contains(t, lastPrompt(llm), "Ana works at Acme.")
	assert.NotContains(t, lastPrompt(llm), "Use the following schema")
}

func TestExtractFromTextFilters(t *testing.T) {
	store := newTestStore(t, testutil.NewFakeWeaviate(t))
	llm := &testutil.FakeLLM{DefaultResponse: extractionReply}

	got, err := store.ExtractFromText(context.Background(), "text", llm,
		interfaces.WithExtractionEntityTypes("person", "organization"),
		interfaces.WithExtractionRelationshipTypes("FOUNDED"),
		interfaces.WithDedupThreshold(0),
	)
	require.NoError(t, err)
	assert.Len(t, got.Entities, 2, "entity type matching ignores case")
	require.Len(t, got.Relationships, 1)
	assert.Equal(t, "FOUNDED", got.Relationships[0].Type)
	assert.Contains(t, lastPrompt(llm), "Focus on entity types: person, organization")
	assert.Contains(t, lastPrompt(llm), "Focus on relationship types: FOUNDED")

	got, err = store.ExtractFromText(context.Background(), "text", llm, interfaces.WithExtractionEntityTypes("Place"))
	require.NoError(t, err)
	assert.Empty(t, got.Entities)
	assert.Empty(t, got.Relationships)

	// The overall confidence is below the minimum, so every entity is dropped.
	got, err = store.ExtractFromText(context.Background(), "text", llm, interfaces.WithMinConfidence(0.95))
	require.NoError(t, err)
	assert.Empty(t, got.Entities)

	got, err = store.ExtractFromText(context.Background(), "text", llm, interfaces.WithMaxEntities(1), interfaces.WithDedupThreshold(0))
	require.NoError(t, err)
	assert.Len(t, got.Entities, 1)
}

func TestExtractFromTextSchemaGuidedPrompt(t *testing.T) {
	schema := &interfaces.GraphSchema{
		EntityTypes: []interfaces.EntityTypeSchema{{Name: "Person", Description: "a human"}},
		RelationshipTypes: []interfaces.RelationshipTypeSchema{{
			Name: "KNOWS", Description: "acquainted", SourceTypes: []string{"Person"}, TargetTypes: []string{"Person"},
		}},
	}
	store := newTestStore(t, testutil.NewFakeWeaviate(t), WithSchema(schema))
	llm := &testutil.FakeLLM{DefaultResponse: `{"entities":[]}`}

	got, err := store.ExtractFromText(context.Background(), "text", llm, interfaces.WithSchemaGuided(true))
	require.NoError(t, err)
	assert.InDelta(t, 0.7, got.Confidence, 1e-6, "missing confidence defaults to 0.7")
	assert.Contains(t, lastPrompt(llm), "Use the following schema")
	assert.Contains(t, lastPrompt(llm), "- Person: a human")
	assert.Contains(t, lastPrompt(llm), "- KNOWS: acquainted (from [Person] to [Person])")
}

func TestExtractFromTextUnparsableResponse(t *testing.T) {
	store := newTestStore(t, testutil.NewFakeWeaviate(t))
	for _, reply := range []string{"no json here", `{"entities": "wrong type"}`} {
		got, err := store.ExtractFromText(context.Background(), "text", &testutil.FakeLLM{DefaultResponse: reply})
		require.NoError(t, err, reply)
		assert.Empty(t, got.Entities)
		assert.InDelta(t, 0.3, got.Confidence, 1e-6, "an unparsable reply gives a low-confidence empty result")
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"no braces", ""},
		{`prefix {"a":1} suffix`, `{"a":1}`},
		{`{"a":{"b":{}}} {"c":2}`, `{"a":{"b":{}}}`},
		{`{"open": {`, ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, extractJSON(tt.in), tt.in)
	}
}

func TestDeduplicateEntities(t *testing.T) {
	ctx := context.Background()
	emb := &fakeEmbedder{vectors: map[string][]float32{
		"Ana an engineer":           {1, 0},
		"Ana Silva senior engineer": {0.99, 0.01},
		"Acme a company":            {0, 1},
	}}
	store := newTestStore(t, testutil.NewFakeWeaviate(t), WithEmbedder(emb))

	entities := []interfaces.Entity{
		{ID: "1", Name: "Ana", Description: "an engineer", Properties: map[string]interface{}{"a": 1}},
		{ID: "2", Name: "Ana Silva", Description: "senior engineer", Properties: map[string]interface{}{"a": 2, "b": 3}},
		{ID: "3", Name: "Acme", Description: "a company"},
	}
	got, err := store.deduplicateEntities(ctx, entities, 0.9)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "1", got[0].ID, "the first entity is kept")
	assert.Equal(t, "Ana Silva", got[0].Name, "the longer name wins")
	assert.Equal(t, "senior engineer", got[0].Description, "the longer description wins")
	assert.Equal(t, map[string]interface{}{"a": 1, "b": 3}, got[0].Properties, "existing properties are kept")
	assert.Equal(t, "3", got[1].ID)

	single, err := store.deduplicateEntities(ctx, entities[:1], 0.9)
	require.NoError(t, err)
	assert.Len(t, single, 1)
}

func TestDeduplicateEntitiesMergesIntoEntityWithoutProperties(t *testing.T) {
	emb := &fakeEmbedder{similarity: func(_, _ []float32) (float32, error) { return 1, nil }}
	store := newTestStore(t, testutil.NewFakeWeaviate(t), WithEmbedder(emb))

	got, err := store.deduplicateEntities(context.Background(), []interfaces.Entity{
		{ID: "1", Name: "Long name", Description: "long description"},
		{ID: "2", Name: "x", Description: "y", Properties: map[string]interface{}{"k": "v"}},
	}, 0.5)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Long name", got[0].Name)
	assert.Equal(t, map[string]interface{}{"k": "v"}, got[0].Properties)
}

func TestDeduplicateEntitiesErrors(t *testing.T) {
	ctx := context.Background()
	entities := []interfaces.Entity{{ID: "1", Name: "a"}, {ID: "2", Name: "b"}}

	store := newTestStore(t, testutil.NewFakeWeaviate(t), WithEmbedder(&fakeEmbedder{batchErr: errors.New("x")}))
	_, err := store.deduplicateEntities(ctx, entities, 0.5)
	require.ErrorContains(t, err, "failed to generate embeddings")

	// Similarity errors mean no merge.
	store = newTestStore(t, testutil.NewFakeWeaviate(t), WithEmbedder(&fakeEmbedder{
		similarity: func(_, _ []float32) (float32, error) { return 0, errors.New("x") },
	}))
	got, err := store.deduplicateEntities(ctx, entities, 0.5)
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestExtractFromTextDeduplicates(t *testing.T) {
	reply := `{"entities":[{"name":"Ana","type":"Person"},{"name":"Ana B","type":"Person"}],"confidence":0.9}`

	same := &fakeEmbedder{similarity: func(_, _ []float32) (float32, error) { return 1, nil }}
	store := newTestStore(t, testutil.NewFakeWeaviate(t), WithEmbedder(same))
	got, err := store.ExtractFromText(context.Background(), "text", &testutil.FakeLLM{DefaultResponse: reply})
	require.NoError(t, err)
	require.Len(t, got.Entities, 1)
	assert.Equal(t, "Ana B", got.Entities[0].Name)

	// A dedup failure keeps the original entities.
	failing := &fakeEmbedder{batchErr: errors.New("x")}
	store = newTestStore(t, testutil.NewFakeWeaviate(t), WithEmbedder(failing))
	got, err = store.ExtractFromText(context.Background(), "text", &testutil.FakeLLM{DefaultResponse: reply})
	require.NoError(t, err)
	assert.Len(t, got.Entities, 2)
}
