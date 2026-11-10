package graphrag

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddEntityMetadata(t *testing.T) {
	tool := NewAddEntityTool(&fakeStore{})

	assert.Equal(t, "graphrag_add_entity", tool.Name())
	assert.NotEmpty(t, tool.Description())

	params := tool.Parameters()
	for _, key := range []string{"name", "type", "description"} {
		require.Contains(t, params, key)
		assert.True(t, params[key].Required, key)
	}
	for _, key := range []string{"properties", "id"} {
		require.Contains(t, params, key)
		assert.False(t, params[key].Required, key)
	}
}

func TestAddEntityStoresEntity(t *testing.T) {
	store := &fakeStore{}
	tool := NewAddEntityTool(store)

	out, err := tool.Run(context.Background(), `{
		"id": "e-1",
		"name": "Ada",
		"type": "Person",
		"description": "Mathematician",
		"properties": {"field": "math"}
	}`)
	require.NoError(t, err)

	require.Len(t, store.storedEntities, 1)
	e := store.storedEntities[0]
	assert.Equal(t, "e-1", e.ID)
	assert.Equal(t, "Ada", e.Name)
	assert.Equal(t, "Person", e.Type)
	assert.Equal(t, "Mathematician", e.Description)
	assert.Equal(t, map[string]any{"field": "math"}, e.Properties)
	assert.False(t, e.CreatedAt.IsZero())
	assert.Equal(t, e.CreatedAt, e.UpdatedAt)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, true, result["success"])
	assert.Equal(t, "e-1", result["entity_id"])
	assert.Equal(t, "Entity 'Ada' of type 'Person' added successfully", result["message"])
}

func TestAddEntityGeneratesID(t *testing.T) {
	store := &fakeStore{}
	tool := NewAddEntityTool(store)

	_, err := tool.Execute(context.Background(), `{"name": "Ada", "type": "Person", "description": "d"}`)
	require.NoError(t, err)

	require.Len(t, store.storedEntities, 1)
	_, err = uuid.Parse(store.storedEntities[0].ID)
	assert.NoError(t, err)
}

func TestAddEntityErrors(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		storeErr error
		wantErr  string
	}{
		{name: "invalid JSON", args: `nope`, wantErr: "failed to parse arguments"},
		{name: "missing name", args: `{"type": "Person", "description": "d"}`, wantErr: "name is required"},
		{name: "missing type", args: `{"name": "Ada", "description": "d"}`, wantErr: "type is required"},
		{name: "missing description", args: `{"name": "Ada", "type": "Person"}`, wantErr: "description is required"},
		{
			name:     "store failure",
			args:     `{"name": "Ada", "type": "Person", "description": "d"}`,
			storeErr: errors.New("db down"),
			wantErr:  "failed to store entity: db down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{storeEntitiesErr: tt.storeErr}
			_, err := NewAddEntityTool(store).Execute(context.Background(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, store.storedEntities)
		})
	}
}
