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

func TestAddRelationshipMetadata(t *testing.T) {
	tool := NewAddRelationshipTool(&fakeStore{})

	assert.Equal(t, "graphrag_add_relationship", tool.Name())
	assert.NotEmpty(t, tool.Description())

	params := tool.Parameters()
	for _, key := range []string{"source_id", "target_id", "type"} {
		require.Contains(t, params, key)
		assert.True(t, params[key].Required, key)
	}
	assert.Equal(t, 1.0, params["strength"].Default)
}

func TestAddRelationshipStoresRelationship(t *testing.T) {
	store := &fakeStore{}
	tool := NewAddRelationshipTool(store)

	out, err := tool.Run(context.Background(), `{
		"id": "r-1",
		"source_id": "a",
		"target_id": "b",
		"type": "works on",
		"description": "lead",
		"strength": 0.5,
		"properties": {"since": 2020}
	}`)
	require.NoError(t, err)

	require.Len(t, store.storedRelationships, 1)
	r := store.storedRelationships[0]
	assert.Equal(t, "r-1", r.ID)
	assert.Equal(t, "a", r.SourceID)
	assert.Equal(t, "b", r.TargetID)
	assert.Equal(t, "WORKS_ON", r.Type, "type is upper-cased and spaces become underscores")
	assert.Equal(t, "lead", r.Description)
	assert.InDelta(t, 0.5, r.Strength, 1e-6)
	assert.Equal(t, map[string]any{"since": float64(2020)}, r.Properties)
	assert.False(t, r.CreatedAt.IsZero())

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, true, result["success"])
	assert.Equal(t, "r-1", result["relationship_id"])
	assert.Equal(t, "Relationship 'WORKS_ON' from 'a' to 'b' added successfully", result["message"])
}

func TestAddRelationshipDefaults(t *testing.T) {
	store := &fakeStore{}
	_, err := NewAddRelationshipTool(store).Execute(context.Background(), `{"source_id": "a", "target_id": "b", "type": "MANAGES"}`)
	require.NoError(t, err)

	require.Len(t, store.storedRelationships, 1)
	r := store.storedRelationships[0]
	assert.InDelta(t, 1.0, r.Strength, 1e-6)
	_, err = uuid.Parse(r.ID)
	assert.NoError(t, err)
}

func TestAddRelationshipErrors(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		storeErr error
		wantErr  string
	}{
		{name: "invalid JSON", args: `nope`, wantErr: "failed to parse arguments"},
		{name: "missing source", args: `{"target_id": "b", "type": "X"}`, wantErr: "source_id is required"},
		{name: "missing target", args: `{"source_id": "a", "type": "X"}`, wantErr: "target_id is required"},
		{name: "missing type", args: `{"source_id": "a", "target_id": "b"}`, wantErr: "type is required"},
		{name: "strength too high", args: `{"source_id": "a", "target_id": "b", "type": "X", "strength": 1.5}`, wantErr: "strength must be between 0.0 and 1.0"},
		{name: "negative strength", args: `{"source_id": "a", "target_id": "b", "type": "X", "strength": -0.1}`, wantErr: "strength must be between 0.0 and 1.0"},
		{
			name:     "store failure",
			args:     `{"source_id": "a", "target_id": "b", "type": "X"}`,
			storeErr: errors.New("db down"),
			wantErr:  "failed to store relationship: db down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{storeRelsErr: tt.storeErr}
			_, err := NewAddRelationshipTool(store).Execute(context.Background(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, store.storedRelationships)
		})
	}
}
