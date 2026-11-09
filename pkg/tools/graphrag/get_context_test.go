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

type contextOutput struct {
	CentralEntity struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"central_entity"`
	Depth    int `json:"depth"`
	Entities []struct {
		ID         string         `json:"id"`
		Properties map[string]any `json:"properties"`
	} `json:"entities"`
	Relationships []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"relationships"`
	Summary string `json:"summary"`
}

func sampleGraphContext() *interfaces.GraphContext {
	central := interfaces.Entity{ID: "a", Name: "Ada", Type: "Person"}
	return &interfaces.GraphContext{
		CentralEntity: central,
		Entities: []interfaces.Entity{
			central,
			{ID: "p", Name: "Engine", Type: "Project", Properties: map[string]any{"k": "v"}},
		},
		Relationships: []interfaces.Relationship{
			{ID: "r", SourceID: "a", TargetID: "p", Type: "WORKS_ON", Strength: 1},
		},
		Depth: 2,
	}
}

func TestGetContextMetadata(t *testing.T) {
	tool := NewGetContextTool(&fakeStore{})

	assert.Equal(t, "graphrag_get_context", tool.Name())
	assert.NotEmpty(t, tool.Description())

	params := tool.Parameters()
	require.Contains(t, params, "entity_id")
	assert.True(t, params["entity_id"].Required)
	assert.Equal(t, 2, params["depth"].Default)
	assert.Contains(t, params, "relationship_types")
}

func TestGetContextFormatsGraph(t *testing.T) {
	store := &fakeStore{traverseResult: sampleGraphContext()}
	tool := NewGetContextTool(store)

	out, err := tool.Execute(context.Background(), `{"entity_id": "a", "depth": 3, "relationship_types": ["WORKS_ON"]}`)
	require.NoError(t, err)

	assert.Equal(t, "a", store.lastEntityID)
	assert.Equal(t, 3, store.lastDepth)
	assert.Equal(t, []string{"WORKS_ON"}, store.searchOpts.RelationshipTypes)

	var got contextOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, "a", got.CentralEntity.ID)
	assert.Equal(t, 2, got.Depth)
	require.Len(t, got.Entities, 1, "central entity is excluded from the connected list")
	assert.Equal(t, "p", got.Entities[0].ID)
	assert.Equal(t, map[string]any{"k": "v"}, got.Entities[0].Properties)
	require.Len(t, got.Relationships, 1)
	assert.Equal(t, "WORKS_ON", got.Relationships[0].Type)
	assert.Equal(t, "Entity 'Ada' (Person) has 1 connected entities and 1 relationships within depth 2.", got.Summary)
}

func TestGetContextDepthBounds(t *testing.T) {
	tests := []struct {
		name      string
		args      string
		wantDepth int
	}{
		{name: "default", args: `{"entity_id": "a"}`, wantDepth: 2},
		{name: "negative uses default", args: `{"entity_id": "a", "depth": -1}`, wantDepth: 2},
		{name: "capped at five", args: `{"entity_id": "a", "depth": 9}`, wantDepth: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{traverseResult: sampleGraphContext()}
			_, err := NewGetContextTool(store).Execute(context.Background(), tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDepth, store.lastDepth)
			assert.Empty(t, store.searchOpts.RelationshipTypes)
		})
	}
}

func TestGetContextRun(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "plain entity ID", input: "a"},
		{name: "JSON input", input: `{"entity_id": "a"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{traverseResult: sampleGraphContext()}
			_, err := NewGetContextTool(store).Run(context.Background(), tt.input)
			require.NoError(t, err)
			assert.Equal(t, "a", store.lastEntityID)
		})
	}
}

func TestGetContextErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		store   *fakeStore
		wantErr string
	}{
		{name: "invalid JSON", args: `{bad`, store: &fakeStore{}, wantErr: "failed to parse arguments"},
		{name: "missing entity ID", args: `{}`, store: &fakeStore{}, wantErr: "entity_id is required"},
		{
			name:    "traversal failure",
			args:    `{"entity_id": "a"}`,
			store:   &fakeStore{traverseErr: errors.New("not found")},
			wantErr: "failed to get context: not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGetContextTool(tt.store).Execute(context.Background(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
