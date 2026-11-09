package graphrag

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

type extractOutput struct {
	Entities []struct {
		ID          string         `json:"id"`
		Name        string         `json:"name"`
		Type        string         `json:"type"`
		Description string         `json:"description"`
		Properties  map[string]any `json:"properties"`
	} `json:"entities"`
	Relationships []struct {
		ID       string  `json:"id"`
		SourceID string  `json:"source_id"`
		TargetID string  `json:"target_id"`
		Type     string  `json:"type"`
		Strength float32 `json:"strength"`
	} `json:"relationships"`
	Confidence float32 `json:"confidence"`
	Stored     bool    `json:"stored"`
	Summary    string  `json:"summary"`
}

func sampleExtraction() *interfaces.ExtractionResult {
	return &interfaces.ExtractionResult{
		Entities: []interfaces.Entity{
			{ID: "a", Name: "Ada", Type: "Person", Description: "d", Properties: map[string]any{"k": "v"}},
			{ID: "p", Name: "Engine", Type: "Project"},
		},
		Relationships: []interfaces.Relationship{
			{ID: "r", SourceID: "a", TargetID: "p", Type: "WORKS_ON", Strength: 0.8},
		},
		Confidence: 0.9,
	}
}

func TestExtractMetadata(t *testing.T) {
	tool := NewExtractTool(&fakeStore{}, fakeLLM{})

	assert.Equal(t, "graphrag_extract", tool.Name())
	assert.NotEmpty(t, tool.Description())

	params := tool.Parameters()
	require.Contains(t, params, "text")
	assert.True(t, params["text"].Required)
	for _, key := range []string{"store_results", "schema_guided", "entity_types", "relationship_types"} {
		assert.Contains(t, params, key)
	}
}

func TestExtractWithoutStoring(t *testing.T) {
	store := &fakeStore{extractResult: sampleExtraction()}
	llm := fakeLLM{}
	tool := NewExtractTool(store, llm)

	out, err := tool.Execute(context.Background(), `{
		"text": "Ada works on Engine",
		"schema_guided": true,
		"entity_types": ["Person", "Project"],
		"relationship_types": ["WORKS_ON"]
	}`)
	require.NoError(t, err)

	assert.Equal(t, "Ada works on Engine", store.extractText)
	assert.Equal(t, llm, store.extractLLM)
	assert.True(t, store.extractOpts.SchemaGuided)
	assert.Equal(t, []string{"Person", "Project"}, store.extractOpts.EntityTypes)
	assert.Equal(t, []string{"WORKS_ON"}, store.extractOpts.RelationshipTypes)
	assert.Empty(t, store.storedEntities)
	assert.Empty(t, store.storedRelationships)

	var got extractOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Len(t, got.Entities, 2)
	assert.Equal(t, "Ada", got.Entities[0].Name)
	assert.Equal(t, map[string]any{"k": "v"}, got.Entities[0].Properties)
	require.Len(t, got.Relationships, 1)
	assert.Equal(t, "WORKS_ON", got.Relationships[0].Type)
	assert.InDelta(t, 0.8, got.Relationships[0].Strength, 1e-6)
	assert.False(t, got.Stored)
	assert.Equal(t, "Extracted 2 entities and 1 relationships with 90% confidence.", got.Summary)
}

func TestExtractRunWithPlainText(t *testing.T) {
	store := &fakeStore{extractResult: &interfaces.ExtractionResult{}}
	tool := NewExtractTool(store, fakeLLM{})

	out, err := tool.Run(context.Background(), `Ada said "hello"`)
	require.NoError(t, err)
	assert.Equal(t, `Ada said "hello"`, store.extractText)
	assert.Empty(t, store.extractOpts.EntityTypes)

	var got extractOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Empty(t, got.Entities)
	assert.Empty(t, got.Relationships)
	assert.Equal(t, "Extracted 0 entities and 0 relationships with 0% confidence.", got.Summary)
}

func TestExtractRunWithJSON(t *testing.T) {
	store := &fakeStore{extractResult: &interfaces.ExtractionResult{}}
	_, err := NewExtractTool(store, fakeLLM{}).Run(context.Background(), `  {"text": "hi"}`)
	require.NoError(t, err)
	assert.Equal(t, "hi", store.extractText)
}

func TestExtractStoresResults(t *testing.T) {
	store := &fakeStore{extractResult: sampleExtraction()}
	out, err := NewExtractTool(store, fakeLLM{}).Execute(context.Background(), `{"text": "t", "store_results": true}`)
	require.NoError(t, err)

	assert.Len(t, store.storedEntities, 2)
	assert.Len(t, store.storedRelationships, 1)

	var got extractOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.True(t, got.Stored)
	assert.Equal(t, "Extracted 2 entities and 1 relationships with 90% confidence and stored in the knowledge graph.", got.Summary)
}

func TestExtractStoresEntitiesWithoutRelationships(t *testing.T) {
	result := sampleExtraction()
	result.Relationships = nil
	store := &fakeStore{extractResult: result, storeRelsErr: errors.New("must not be called")}

	_, err := NewExtractTool(store, fakeLLM{}).Execute(context.Background(), `{"text": "t", "store_results": true}`)
	require.NoError(t, err)
	assert.Len(t, store.storedEntities, 2)
}

func TestExtractSkipsStoreWhenNoEntities(t *testing.T) {
	store := &fakeStore{
		extractResult:    &interfaces.ExtractionResult{},
		storeEntitiesErr: errors.New("must not be called"),
	}
	out, err := NewExtractTool(store, fakeLLM{}).Execute(context.Background(), `{"text": "t", "store_results": true}`)
	require.NoError(t, err)

	var got extractOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.False(t, got.Stored)
}

func TestExtractErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		store   *fakeStore
		wantErr string
	}{
		{name: "invalid JSON", args: `{bad`, store: &fakeStore{}, wantErr: "failed to parse arguments"},
		{name: "missing text", args: `{}`, store: &fakeStore{}, wantErr: "text is required"},
		{
			name:    "extraction failure",
			args:    `{"text": "t"}`,
			store:   &fakeStore{extractErr: errors.New("llm down")},
			wantErr: "extraction failed: llm down",
		},
		{
			name:    "store entities failure",
			args:    `{"text": "t", "store_results": true}`,
			store:   &fakeStore{extractResult: sampleExtraction(), storeEntitiesErr: errors.New("boom")},
			wantErr: "failed to store entities: boom",
		},
		{
			name:    "store relationships failure",
			args:    `{"text": "t", "store_results": true}`,
			store:   &fakeStore{extractResult: sampleExtraction(), storeRelsErr: errors.New("boom")},
			wantErr: "failed to store relationships: boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewExtractTool(tt.store, fakeLLM{}).Execute(context.Background(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
