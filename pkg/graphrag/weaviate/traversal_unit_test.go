package weaviate

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// sampleGraph builds: a -r1-> b -r3-> d, c -r2-> a, and an isolated e.
func sampleGraph() *graph {
	g := newGraph()
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		g.addEntity(id, strings.ToUpper(id), "Node")
	}
	g.addRel("r1", "a", "b", "LINK")
	g.addRel("r2", "c", "a", "LINK")
	g.addRel("r3", "b", "d", "LINK")
	return g
}

func entityIDs(entities []interfaces.Entity) []string {
	ids := make([]string, len(entities))
	for i, e := range entities {
		ids[i] = e.ID
	}
	return ids
}

func relIDs(rels []interfaces.Relationship) []string {
	ids := make([]string, len(rels))
	for i, r := range rels {
		ids[i] = r.ID
	}
	return ids
}

func TestTraverseFromValidation(t *testing.T) {
	store := newTestStore(t, newFakeWeaviate(t))
	ctx := context.Background()

	_, err := store.TraverseFrom(ctx, "", 1)
	require.ErrorIs(t, err, graphrag.ErrInvalidEntityID)
	_, err = store.TraverseFrom(ctx, "a", -1)
	require.ErrorIs(t, err, graphrag.ErrInvalidDepth)
	_, err = store.TraverseFrom(ctx, "a", 6)
	require.ErrorIs(t, err, graphrag.ErrMaxDepthExceeded)
	_, err = store.TraverseFrom(ctx, "missing", 1)
	require.ErrorIs(t, err, graphrag.ErrEntityNotFound)
}

func TestTraverseFromOneHop(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)
	fw.setGraphQL(sampleGraph().answer)

	got, err := store.TraverseFrom(context.Background(), "a", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Depth)
	assert.Equal(t, "a", got.CentralEntity.ID)
	assert.ElementsMatch(t, []string{"a", "b", "c"}, entityIDs(got.Entities))
	ids := relIDs(got.Relationships)
	for _, id := range []string{"r1", "r2", "r3"} {
		assert.Contains(t, ids, id)
	}
}

func TestTraverseFromDefaultDepthReachesTwoHops(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)
	fw.setGraphQL(sampleGraph().answer)

	got, err := store.TraverseFrom(context.Background(), "a", 0,
		interfaces.WithSearchTenant("org"), interfaces.WithRelationshipTypes("LINK"))
	require.NoError(t, err)
	assert.Equal(t, 2, got.Depth)
	assert.ElementsMatch(t, []string{"a", "b", "c", "d"}, entityIDs(got.Entities))

	// The tenant and relationship type reach every relationship query.
	for _, q := range fw.queries() {
		if strings.Contains(q, "GraphRelationship") {
			assert.Contains(t, q, `path: ["orgId"] valueString: "org"`)
			assert.Contains(t, q, `path: ["relationshipType"] valueString: "LINK"`)
		}
	}
}

func TestTraverseFromToleratesRelationshipErrors(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)
	g := sampleGraph()
	fw.setGraphQL(func(q string) fakeReply {
		if strings.Contains(q, "GraphRelationship") {
			return gqlError
		}
		return g.answer(q)
	})

	got, err := store.TraverseFrom(context.Background(), "a", 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, entityIDs(got.Entities))
	assert.Empty(t, got.Relationships)
}

func TestShortestPath(t *testing.T) {
	ctx := context.Background()

	t.Run("validation", func(t *testing.T) {
		store := newTestStore(t, newFakeWeaviate(t))
		_, err := store.ShortestPath(ctx, "", "b")
		require.ErrorIs(t, err, graphrag.ErrInvalidEntityID)
		_, err = store.ShortestPath(ctx, "a", "")
		require.ErrorIs(t, err, graphrag.ErrInvalidEntityID)
	})

	t.Run("same entity", func(t *testing.T) {
		fw := newFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.setGraphQL(sampleGraph().answer)

		got, err := store.ShortestPath(ctx, "a", "a")
		require.NoError(t, err)
		assert.Equal(t, 0, got.Length)
		assert.Equal(t, "a", got.Source.ID)
		assert.Equal(t, "a", got.Target.ID)

		_, err = store.ShortestPath(ctx, "missing", "missing")
		require.ErrorIs(t, err, graphrag.ErrEntityNotFound)
	})

	t.Run("two hops", func(t *testing.T) {
		fw := newFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.setGraphQL(sampleGraph().answer)

		got, err := store.ShortestPath(ctx, "a", "d", interfaces.WithSearchTenant("org"), interfaces.WithRelationshipTypes("LINK"))
		require.NoError(t, err)
		assert.Equal(t, 2, got.Length)
		assert.Equal(t, "a", got.Source.ID)
		assert.Equal(t, "d", got.Target.ID)
		assert.Equal(t, []string{"b"}, entityIDs(got.Entities))
		assert.Equal(t, []string{"r1", "r3"}, relIDs(got.Relationships))
	})

	t.Run("follows incoming edges", func(t *testing.T) {
		fw := newFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.setGraphQL(sampleGraph().answer)

		got, err := store.ShortestPath(ctx, "a", "c")
		require.NoError(t, err)
		assert.Equal(t, 1, got.Length)
		assert.Equal(t, []string{"r2"}, relIDs(got.Relationships))
	})

	t.Run("no path", func(t *testing.T) {
		fw := newFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.setGraphQL(sampleGraph().answer)

		_, err := store.ShortestPath(ctx, "a", "e")
		require.ErrorIs(t, err, graphrag.ErrPathNotFound)
	})

	t.Run("depth limit", func(t *testing.T) {
		fw := newFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.setGraphQL(sampleGraph().answer)

		_, err := store.ShortestPath(ctx, "a", "d", interfaces.WithMaxDepth(1))
		require.ErrorIs(t, err, graphrag.ErrPathNotFound)
	})

	t.Run("relationship errors end the search", func(t *testing.T) {
		fw := newFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.setGraphQL(func(string) fakeReply { return gqlError })

		_, err := store.ShortestPath(ctx, "a", "d")
		require.ErrorIs(t, err, graphrag.ErrPathNotFound)
	})
}

func TestBuildPathResult(t *testing.T) {
	ctx := context.Background()
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)
	fw.setGraphQL(sampleGraph().answer)

	_, err := store.buildPathResult(ctx, []string{"a"}, nil, "")
	require.ErrorIs(t, err, graphrag.ErrPathNotFound)

	_, err = store.buildPathResult(ctx, []string{"missing", "a"}, []string{"r"}, "")
	require.ErrorIs(t, err, graphrag.ErrEntityNotFound)

	_, err = store.buildPathResult(ctx, []string{"a", "missing"}, []string{"r"}, "org")
	require.ErrorIs(t, err, graphrag.ErrEntityNotFound)

	// A missing intermediate entity is skipped.
	got, err := store.buildPathResult(ctx, []string{"a", "missing", "b"}, []string{"r1", "r2"}, "")
	require.NoError(t, err)
	assert.Empty(t, got.Entities)
	assert.Equal(t, 2, got.Length)
}
