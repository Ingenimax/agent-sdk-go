package weaviate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weaviate/weaviate/entities/models"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func entityItems() testutil.WeaviateReply {
	return getResponse("GraphEntity",
		map[string]interface{}{"entityId": "e1", "name": "Ana", "entityType": "Person",
			"_additional": map[string]interface{}{"id": "u1", "certainty": 0.9}},
		map[string]interface{}{"entityId": "e2", "name": "Acme", "entityType": "Org",
			"_additional": map[string]interface{}{"id": "u2", "score": 3.0}},
	)
}

func TestSearchModes(t *testing.T) {
	tests := []struct {
		name     string
		embedder *fakeEmbedder
		noEmbed  bool
		mode     interfaces.GraphSearchMode
		want     string
		notWant  string
	}{
		{name: "keyword", mode: interfaces.SearchModeKeyword, want: "bm25", notWant: "nearVector"},
		{name: "vector", mode: interfaces.SearchModeVector, want: "nearVector", notWant: "bm25"},
		{name: "hybrid", mode: interfaces.SearchModeHybrid, want: "hybrid", notWant: "bm25"},
		{name: "hybrid falls back to keyword without embedder", noEmbed: true, mode: interfaces.SearchModeHybrid, want: "bm25", notWant: "hybrid"},
		{name: "hybrid falls back to keyword when embedding fails", embedder: &fakeEmbedder{err: errors.New("x")}, mode: interfaces.SearchModeHybrid, want: "bm25", notWant: "hybrid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fw := testutil.NewFakeWeaviate(t)
			store := newTestStore(t, fw)
			switch {
			case tt.noEmbed:
				store.embedder = nil
			case tt.embedder != nil:
				store.embedder = tt.embedder
			}
			fw.SetGraphQL(func(string) testutil.WeaviateReply { return entityItems() })

			got, err := store.Search(context.Background(), "who", 0, interfaces.WithSearchMode(tt.mode))
			require.NoError(t, err)
			require.Len(t, got, 2)
			assert.Equal(t, "e1", got[0].Entity.ID)
			assert.Equal(t, "Acme", got[1].Entity.Name)

			q := fw.GraphQLQueries()[0]
			assert.Contains(t, q, tt.want)
			assert.NotContains(t, q, tt.notWant)
			assert.Contains(t, q, "limit: 10", "a non-positive limit defaults to 10")
		})
	}
}

func TestSearchFiltersAndTenant(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	store := newTestStore(t, fw, WithStoreTenant("store-org"))
	fw.SetGraphQL(func(string) testutil.WeaviateReply { return entityItems() })

	got, err := store.Search(context.Background(), "who", 3,
		interfaces.WithEntityTypes("Person", "Org"),
		interfaces.WithMinGraphScore(0.3),
	)
	require.NoError(t, err)
	assert.Len(t, got, 2)

	q := fw.GraphQLQueries()[0]
	assert.Contains(t, q, `path: ["orgId"] valueString: "store-org"`)
	assert.Contains(t, q, `path: ["entityType"] valueString: "Person"`)
	assert.Contains(t, q, `path: ["entityType"] valueString: "Org"`)
	assert.Contains(t, q, "limit: 3")
}

func TestSearchErrors(t *testing.T) {
	ctx := context.Background()

	fw := testutil.NewFakeWeaviate(t)
	store := newTestStore(t, fw)
	_, err := store.Search(ctx, "", 1)
	require.ErrorIs(t, err, graphrag.ErrEmptyQuery)

	fw.SetGraphQL(func(string) testutil.WeaviateReply { return gqlError })
	for mode, msg := range map[interfaces.GraphSearchMode]string{
		interfaces.SearchModeKeyword: "failed to execute keyword search",
		interfaces.SearchModeVector:  "failed to execute vector search",
		interfaces.SearchModeHybrid:  "failed to execute hybrid search",
	} {
		_, err := store.Search(ctx, "q", 1, interfaces.WithSearchMode(mode))
		require.ErrorContains(t, err, msg, string(mode))
	}

	store.embedder = &fakeEmbedder{err: errors.New("x")}
	_, err = store.Search(ctx, "q", 1, interfaces.WithSearchMode(interfaces.SearchModeVector))
	require.ErrorContains(t, err, "failed to generate query embedding")

	store.embedder = nil
	_, err = store.Search(ctx, "q", 1, interfaces.WithSearchMode(interfaces.SearchModeVector))
	require.ErrorIs(t, err, graphrag.ErrNoEmbedder)
}

func TestHybridSearchHandlesOddResponses(t *testing.T) {
	for _, body := range []string{`{"data":{"Get":{}}}`, `{"data":{"Other":{}}}`, `{}`} {
		fw := testutil.NewFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.SetGraphQL(func(string) testutil.WeaviateReply { return ok(body) })
		got, err := store.Search(context.Background(), "q", 1)
		require.NoError(t, err, body)
		assert.Empty(t, got, body)
	}
}

func TestBuildSearchFilter(t *testing.T) {
	assert.Nil(t, buildSearchFilter("", nil))

	tenantOnly := buildSearchFilter("org", nil).Build()
	assert.Equal(t, []string{"orgId"}, tenantOnly.Path)

	typeOnly := buildSearchFilter("", []string{"Person"}).Build()
	assert.Equal(t, []string{"entityType"}, typeOnly.Path)
	assert.Equal(t, "Person", *typeOnly.ValueString)

	multi := buildSearchFilter("", []string{"A", "B"}).Build()
	assert.Equal(t, "Or", multi.Operator)
	assert.Len(t, multi.Operands, 2)

	both := buildSearchFilter("org", []string{"A"}).Build()
	assert.Equal(t, "And", both.Operator)
	require.Len(t, both.Operands, 2)
	assert.Equal(t, []string{"orgId"}, both.Operands[0].Path)
	assert.Equal(t, []string{"entityType"}, both.Operands[1].Path)
}

func TestParseSearchResults(t *testing.T) {
	assert.Empty(t, parseSearchResults(&models.GraphQLResponse{}, "C"))
	assert.Empty(t, parseSearchResults(&models.GraphQLResponse{Data: map[string]models.JSONObject{"Get": 1}}, "C"))
	assert.Empty(t, parseSearchResults(&models.GraphQLResponse{Data: map[string]models.JSONObject{"Get": map[string]interface{}{}}}, "C"))

	resp := &models.GraphQLResponse{Data: map[string]models.JSONObject{"Get": map[string]interface{}{"C": []interface{}{
		"skip me",
		map[string]interface{}{"entityId": "e1", "name": "Ana"},
	}}}}
	got := parseSearchResults(resp, "C")
	require.Len(t, got, 1)
	assert.Equal(t, "e1", got[0].Entity.ID)
	assert.Equal(t, "Ana", got[0].Entity.Name)
}

func TestParseEntityFromMap(t *testing.T) {
	got := parseEntityFromMap(map[string]interface{}{
		"entityId": "e1", "name": "Ana", "entityType": "Person", "description": "d", "orgId": "o", "extra": 1,
	})
	assert.Equal(t, interfaces.Entity{ID: "e1", Name: "Ana", Type: "Person", Description: "d", OrgID: "o"}, got)
	assert.Equal(t, interfaces.Entity{}, parseEntityFromMap(map[string]interface{}{"entityId": 5}))
}

func TestGlobalSearch(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	store := newTestStore(t, fw)
	fw.SetGraphQL(func(query string) testutil.WeaviateReply {
		if strings.Contains(query, "Aggregate") {
			return aggregateResponse("GraphEntity", groupedBy("Person"), groupedBy("Org"), groupedBy("Place"))
		}
		switch {
		case strings.Contains(query, `valueString: "Person"`):
			return getResponse("GraphEntity", map[string]interface{}{"entityId": "p1", "entityType": "Person"})
		case strings.Contains(query, `valueString: "Org"`):
			return getResponse("GraphEntity", map[string]interface{}{"entityId": "o1", "entityType": "Org"})
		}
		return gqlError
	})

	got, err := store.GlobalSearch(context.Background(), "q", 1, interfaces.WithEntityTypes("Person", "Org", "Missing"))
	require.NoError(t, err)
	require.Len(t, got, 2)
	communities := map[string]string{}
	for _, r := range got {
		communities[r.Entity.ID] = r.CommunityID
	}
	assert.Equal(t, map[string]string{"p1": "Person", "o1": "Org"}, communities)

	// Each community query asks for at most five results.
	for _, q := range fw.GraphQLQueries()[1:] {
		assert.Contains(t, q, "limit: 5")
	}

	// A failing community search is skipped.
	got, err = store.GlobalSearch(context.Background(), "q", 1)
	require.NoError(t, err)
	assert.Len(t, got, 2)

	_, err = store.GlobalSearch(context.Background(), "", 1)
	require.ErrorIs(t, err, graphrag.ErrEmptyQuery)
}

func TestGlobalSearchFallsBackWhenAggregateFails(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	store := newTestStore(t, fw)
	fw.SetGraphQL(func(query string) testutil.WeaviateReply {
		if strings.Contains(query, "Aggregate") {
			return gqlError
		}
		return entityItems()
	})

	got, err := store.GlobalSearch(context.Background(), "q", 1)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Empty(t, got[0].CommunityID)
	assert.Contains(t, fw.GraphQLQueries()[1], "limit: 20")
}

func TestLocalSearch(t *testing.T) {
	g := newGraph()
	g.addEntity("a", "Ana", "Person")
	g.addEntity("b", "Bob", "Person")
	g.addRel("r1", "a", "b", "KNOWS")

	ctx := context.Background()

	t.Run("validation", func(t *testing.T) {
		store := newTestStore(t, testutil.NewFakeWeaviate(t))
		_, err := store.LocalSearch(ctx, "", "a", 1)
		require.ErrorIs(t, err, graphrag.ErrEmptyQuery)
		_, err = store.LocalSearch(ctx, "q", "a", -1)
		require.ErrorIs(t, err, graphrag.ErrInvalidDepth)
	})

	t.Run("from given entity", func(t *testing.T) {
		fw := testutil.NewFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.SetGraphQL(g.answer)

		got, err := store.LocalSearch(ctx, "q", "a", 0, interfaces.WithIncludeRelationships(true))
		require.NoError(t, err)
		require.NotEmpty(t, got)
		for _, r := range got {
			assert.Len(t, r.Context, 2)
			require.NotEmpty(t, r.Path)
			assert.Equal(t, "r1", r.Path[0].ID)
		}
	})

	t.Run("from top result", func(t *testing.T) {
		fw := testutil.NewFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.SetGraphQL(g.answer)

		got, err := store.LocalSearch(ctx, "q", "", 1)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		assert.NotEmpty(t, got[0].Context)
		assert.Empty(t, got[0].Path, "relationships are only attached on request")
	})

	t.Run("missing start entity is tolerated", func(t *testing.T) {
		fw := testutil.NewFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.SetGraphQL(g.answer)

		got, err := store.LocalSearch(ctx, "q", "missing", 1)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		for _, r := range got {
			assert.Empty(t, r.Context)
		}
	})

	t.Run("traversal error is returned", func(t *testing.T) {
		store := newTestStore(t, testutil.NewFakeWeaviate(t))
		_, err := store.LocalSearch(ctx, "q", "a", 9)
		require.ErrorIs(t, err, graphrag.ErrMaxDepthExceeded)
	})

	t.Run("top result traversal error is logged", func(t *testing.T) {
		fw := testutil.NewFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.SetGraphQL(g.answer)
		got, err := store.LocalSearch(ctx, "q", "", 9)
		require.NoError(t, err)
		assert.Empty(t, got[0].Context)
	})

	t.Run("search error is returned", func(t *testing.T) {
		fw := testutil.NewFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.SetGraphQL(func(string) testutil.WeaviateReply { return gqlError })
		_, err := store.LocalSearch(ctx, "q", "a", 1)
		require.Error(t, err)
	})
}
