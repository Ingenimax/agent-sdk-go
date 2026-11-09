package graphrag

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

type searchOutput struct {
	Entity struct {
		ID         string         `json:"id"`
		Name       string         `json:"name"`
		Properties map[string]any `json:"properties"`
	} `json:"entity"`
	Score       float32 `json:"score"`
	CommunityID string  `json:"community_id"`
	Context     []struct {
		ID string `json:"id"`
	} `json:"context"`
	Relationships []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"relationships"`
}

func sampleSearchResults() []interfaces.GraphSearchResult {
	return []interfaces.GraphSearchResult{
		{
			Entity:      interfaces.Entity{ID: "a", Name: "Ada", Type: "Person", Properties: map[string]any{"k": "v"}},
			Score:       0.9,
			CommunityID: "c1",
			Context:     []interfaces.Entity{{ID: "p", Name: "Engine", Type: "Project"}},
			Path:        []interfaces.Relationship{{ID: "r", SourceID: "a", TargetID: "p", Type: "WORKS_ON"}},
		},
		{
			Entity: interfaces.Entity{ID: "b", Name: "Bob", Type: "Person"},
			Score:  0.4,
		},
	}
}

func TestSearchMetadata(t *testing.T) {
	tool := NewSearchTool(&fakeStore{})

	assert.Equal(t, "graphrag_search", tool.Name())
	assert.NotEmpty(t, tool.Description())

	params := tool.Parameters()
	require.Contains(t, params, "query")
	assert.True(t, params["query"].Required)
	assert.Equal(t, "hybrid", params["search_type"].Default)
	assert.Equal(t, []any{"local", "global", "hybrid"}, params["search_type"].Enum)
}

func TestSearchModes(t *testing.T) {
	tests := []struct {
		name       string
		args       string
		wantMethod string
		check      func(t *testing.T, s *fakeStore)
	}{
		{
			name:       "hybrid by default",
			args:       `{"query": "ada"}`,
			wantMethod: "Search",
			check: func(t *testing.T, s *fakeStore) {
				assert.Equal(t, 10, s.lastLimit)
				assert.Equal(t, interfaces.SearchModeHybrid, s.searchOpts.SearchMode)
				assert.True(t, s.searchOpts.IncludeRelationships)
				assert.Empty(t, s.searchOpts.EntityTypes)
			},
		},
		{
			name:       "hybrid with limit and entity types",
			args:       `{"query": "ada", "search_type": "hybrid", "limit": 3, "entity_types": ["Person"]}`,
			wantMethod: "Search",
			check: func(t *testing.T, s *fakeStore) {
				assert.Equal(t, 3, s.lastLimit)
				assert.Equal(t, []string{"Person"}, s.searchOpts.EntityTypes)
			},
		},
		{
			name:       "local uses depth",
			args:       `{"query": "ada", "search_type": "local", "depth": 4}`,
			wantMethod: "LocalSearch",
			check: func(t *testing.T, s *fakeStore) {
				assert.Equal(t, 4, s.lastDepth)
				assert.Equal(t, 4, s.searchOpts.MaxDepth)
				assert.Empty(t, s.lastEntityID)
			},
		},
		{
			name:       "local default depth",
			args:       `{"query": "ada", "search_type": "local"}`,
			wantMethod: "LocalSearch",
			check: func(t *testing.T, s *fakeStore) {
				assert.Equal(t, 2, s.lastDepth)
			},
		},
		{
			name:       "global uses community level one",
			args:       `{"query": "ada", "search_type": "global"}`,
			wantMethod: "GlobalSearch",
			check: func(t *testing.T, s *fakeStore) {
				assert.Equal(t, 1, s.lastLevel)
				assert.True(t, s.searchOpts.IncludeRelationships)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{searchResults: sampleSearchResults()}
			_, err := NewSearchTool(store).Execute(context.Background(), tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantMethod, store.lastMethod)
			assert.Equal(t, "ada", store.lastQuery)
			tt.check(t, store)
		})
	}
}

func TestSearchFormatsResults(t *testing.T) {
	store := &fakeStore{searchResults: sampleSearchResults()}
	ctx := multitenancy.WithOrgID(context.Background(), "org-1")

	out, err := NewSearchTool(store).Execute(ctx, `{"query": "ada"}`)
	require.NoError(t, err)

	var got []searchOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Len(t, got, 2)

	assert.Equal(t, "a", got[0].Entity.ID)
	assert.Equal(t, map[string]any{"k": "v"}, got[0].Entity.Properties)
	assert.InDelta(t, 0.9, got[0].Score, 1e-6)
	assert.Equal(t, "c1", got[0].CommunityID)
	require.Len(t, got[0].Context, 1)
	assert.Equal(t, "p", got[0].Context[0].ID)
	require.Len(t, got[0].Relationships, 1)
	assert.Equal(t, "WORKS_ON", got[0].Relationships[0].Type)

	assert.Equal(t, "b", got[1].Entity.ID)
	assert.Nil(t, got[1].Context)
	assert.Nil(t, got[1].Relationships)
}

func TestSearchEmptyResults(t *testing.T) {
	out, err := NewSearchTool(&fakeStore{}).Execute(context.Background(), `{"query": "none"}`)
	require.NoError(t, err)
	assert.Equal(t, "[]", out)
}

func TestSearchRun(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "plain query", input: "ada"},
		{name: "JSON input", input: "\n\t{\"query\": \"ada\"}\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{}
			_, err := NewSearchTool(store).Run(context.Background(), tt.input)
			require.NoError(t, err)
			assert.Equal(t, "ada", store.lastQuery)
		})
	}
}

func TestSearchErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		store   *fakeStore
		wantErr string
	}{
		{name: "invalid JSON", args: `{bad`, store: &fakeStore{}, wantErr: "failed to parse arguments"},
		{name: "missing query", args: `{}`, store: &fakeStore{}, wantErr: "query parameter is required"},
		{
			name:    "store failure",
			args:    `{"query": "ada"}`,
			store:   &fakeStore{searchErr: errors.New("timeout")},
			wantErr: "search failed: timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSearchTool(tt.store).Execute(context.Background(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestIsJSON(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: `{"a":1}`, want: true},
		{in: `[1]`, want: true},
		{in: " \t\r\n{}", want: true},
		{in: "hello", want: false},
		{in: "", want: false},
		{in: " \n ", want: false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, isJSON(tt.in), "isJSON(%q)", tt.in)
	}
}

func TestTrimWhitespace(t *testing.T) {
	assert.Equal(t, "a b", trimWhitespace(" \t\na b\r\n "))
	assert.Equal(t, "", trimWhitespace(" \t "))
	assert.Equal(t, "x", trimWhitespace("x"))
}
