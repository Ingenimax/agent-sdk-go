package weaviate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weaviate/weaviate/entities/models"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func TestStoreEntitiesValidation(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	store := newTestStore(t, fw)
	ctx := context.Background()

	require.NoError(t, store.StoreEntities(ctx, nil))

	tests := []struct {
		name   string
		entity interfaces.Entity
		want   error
	}{
		{"missing id", interfaces.Entity{Name: "n", Type: "t"}, graphrag.ErrInvalidEntityID},
		{"missing name", interfaces.Entity{ID: "1", Type: "t"}, graphrag.ErrMissingEntityName},
		{"missing type", interfaces.Entity{ID: "1", Name: "n"}, graphrag.ErrMissingEntityType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.StoreEntities(ctx, []interfaces.Entity{tt.entity})
			require.ErrorIs(t, err, tt.want)
			assert.True(t, graphrag.IsValidationError(err))
		})
	}
	assert.Empty(t, fw.RequestsTo(http.MethodPost, "/v1/batch/objects"))
}

func TestStoreEntitiesSendsObjects(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	emb := &fakeEmbedder{vectors: map[string][]float32{"A person": {1, 2}, "Acme": {3, 4}}}
	store := newTestStore(t, fw, WithEmbedder(emb))
	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	entities := []interfaces.Entity{
		{ID: "e1", Name: "Ana", Type: "Person", Description: "A person", Properties: map[string]interface{}{"age": 30}, CreatedAt: created},
		{ID: "e2", Name: "Acme", Type: "Org"},
	}
	ctx := multitenancy.WithOrgID(context.Background(), "org-ctx")
	require.NoError(t, store.StoreEntities(ctx, entities))

	// Description is embedded when set, otherwise the name.
	assert.Equal(t, []string{"A person", "Acme"}, emb.texts())

	// Timestamps are filled in place.
	assert.Equal(t, created, entities[0].CreatedAt)
	assert.False(t, entities[1].CreatedAt.IsZero())
	assert.False(t, entities[1].UpdatedAt.IsZero())

	reqs := fw.RequestsTo(http.MethodPost, "/v1/batch/objects")
	require.Len(t, reqs, 1)
	objs := batchObjects(t, reqs[0])
	require.Len(t, objs, 2)

	first := objs[0]
	assert.Equal(t, "GraphEntity", first["class"])
	assert.Equal(t, []interface{}{1.0, 2.0}, first["vector"])
	props := first["properties"].(map[string]interface{})
	assert.Equal(t, "e1", props["entityId"])
	assert.Equal(t, "Ana", props["name"])
	assert.Equal(t, "Person", props["entityType"])
	assert.Equal(t, "A person", props["description"])
	assert.JSONEq(t, `{"age":30}`, props["properties"].(string))
	assert.Equal(t, "org-ctx", props["orgId"])
	assert.Equal(t, "2024-01-02T03:04:05Z", props["createdAt"])

	second := objs[1]["properties"].(map[string]interface{})
	assert.Equal(t, "", second["properties"])
}

func TestStoreEntitiesTenantPrecedence(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		opts []interfaces.GraphStoreOption
		want string
	}{
		{"option wins", multitenancy.WithOrgID(context.Background(), "ctx"), []interfaces.GraphStoreOption{interfaces.WithGraphTenant("opt")}, "opt"},
		{"context next", multitenancy.WithOrgID(context.Background(), "ctx"), nil, "ctx"},
		{"store default last", context.Background(), nil, "store"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fw := testutil.NewFakeWeaviate(t)
			store := newTestStore(t, fw, WithStoreTenant("store"))
			require.NoError(t, store.StoreEntities(tt.ctx, []interfaces.Entity{{ID: "1", Name: "n", Type: "t"}}, tt.opts...))
			objs := batchObjects(t, fw.RequestsTo(http.MethodPost, "/v1/batch/objects")[0])
			assert.Equal(t, tt.want, objs[0]["properties"].(map[string]interface{})["orgId"])
		})
	}
}

func TestStoreEntitiesUsesGivenEmbeddingWithoutGeneration(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	emb := &fakeEmbedder{}
	store := newTestStore(t, fw, WithEmbedder(emb))

	entities := []interfaces.Entity{
		{ID: "1", Name: "n", Type: "t", Embedding: []float32{7, 8}},
		{ID: "2", Name: "m", Type: "t"},
	}
	require.NoError(t, store.StoreEntities(context.Background(), entities, interfaces.WithGenerateEmbeddings(false)))
	assert.Empty(t, emb.texts())

	objs := batchObjects(t, fw.RequestsTo(http.MethodPost, "/v1/batch/objects")[0])
	assert.Equal(t, []interface{}{7.0, 8.0}, objs[0]["vector"])
	assert.NotContains(t, objs[1], "vector")
}

func TestStoreEntitiesBatching(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	store := newTestStore(t, fw)

	entities := make([]interfaces.Entity, 5)
	for i := range entities {
		entities[i] = interfaces.Entity{ID: string(rune('a' + i)), Name: "n", Type: "t"}
	}
	require.NoError(t, store.StoreEntities(context.Background(), entities, interfaces.WithGraphBatchSize(2)))

	reqs := fw.RequestsTo(http.MethodPost, "/v1/batch/objects")
	require.Len(t, reqs, 3)
	assert.Len(t, batchObjects(t, reqs[0]), 2)
	assert.Len(t, batchObjects(t, reqs[1]), 2)
	assert.Len(t, batchObjects(t, reqs[2]), 1)
}

func TestStoreEntitiesNonPositiveBatchSizeUsesDefault(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	store := newTestStore(t, fw)
	entities := []interfaces.Entity{{ID: "1", Name: "n", Type: "t"}, {ID: "2", Name: "n", Type: "t"}}
	require.NoError(t, store.StoreEntities(context.Background(), entities, interfaces.WithGraphBatchSize(0)))
	assert.Len(t, fw.RequestsTo(http.MethodPost, "/v1/batch/objects"), 1)
}

func TestStoreEntitiesErrors(t *testing.T) {
	ctx := context.Background()
	entity := interfaces.Entity{ID: "1", Name: "n", Type: "t"}

	t.Run("embedding fails", func(t *testing.T) {
		store := newTestStore(t, testutil.NewFakeWeaviate(t), WithEmbedder(&fakeEmbedder{err: errors.New("no vec")}))
		require.ErrorContains(t, store.StoreEntities(ctx, []interfaces.Entity{entity}), "failed to generate embedding for entity 1")
	})

	t.Run("properties cannot be serialized", func(t *testing.T) {
		store := newTestStore(t, testutil.NewFakeWeaviate(t))
		bad := entity
		bad.Properties = map[string]interface{}{"ch": make(chan int)}
		require.ErrorContains(t, store.StoreEntities(ctx, []interfaces.Entity{bad}), "failed to serialize properties for entity 1")
	})

	batchFails := func(req testutil.WeaviateRequest) (testutil.WeaviateReply, bool) {
		if req.Path == "/v1/batch/objects" {
			return gqlError, true
		}
		return testutil.WeaviateReply{}, false
	}

	t.Run("final batch fails", func(t *testing.T) {
		fw := testutil.NewFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.SetHandle(batchFails)
		require.ErrorContains(t, store.StoreEntities(ctx, []interfaces.Entity{entity}), "failed to store final entity batch")
	})

	t.Run("full batch fails", func(t *testing.T) {
		fw := testutil.NewFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.SetHandle(batchFails)
		err := store.StoreEntities(ctx, []interfaces.Entity{entity}, interfaces.WithGraphBatchSize(1))
		require.ErrorContains(t, err, "failed to store entity batch")
	})
}

func TestGetEntityFake(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	store := newTestStore(t, fw)

	_, err := store.GetEntity(context.Background(), "")
	require.ErrorIs(t, err, graphrag.ErrInvalidEntityID)

	fw.SetGraphQL(func(string) testutil.WeaviateReply {
		return getResponse("GraphEntity", map[string]interface{}{
			"entityId":    "e1",
			"name":        "Ana",
			"entityType":  "Person",
			"description": "d",
			"properties":  `{"age":30}`,
			"orgId":       "org",
			"createdAt":   "2024-01-02T03:04:05Z",
			"updatedAt":   "2024-02-02T03:04:05Z",
			"_additional": map[string]interface{}{"id": "uuid-1", "vector": []interface{}{0.5, 1.5}},
		})
	})

	got, err := store.GetEntity(context.Background(), "e1", interfaces.WithGraphTenant("org"))
	require.NoError(t, err)
	assert.Equal(t, &interfaces.Entity{
		ID:          "e1",
		Name:        "Ana",
		Type:        "Person",
		Description: "d",
		Properties:  map[string]interface{}{"age": 30.0},
		OrgID:       "org",
		Embedding:   []float32{0.5, 1.5},
		CreatedAt:   time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 2, 2, 3, 4, 5, 0, time.UTC),
	}, got)

	q := fw.GraphQLQueries()[0]
	assert.Contains(t, q, `path: ["entityId"] valueString: "e1"`)
	assert.Contains(t, q, `path: ["orgId"] valueString: "org"`)
	assert.Contains(t, q, "operator: And")
	assert.Contains(t, q, "limit: 1")
}

func TestGetEntityWithoutTenantFiltersOnlyByID(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	store := newTestStore(t, fw)
	fw.SetGraphQL(func(string) testutil.WeaviateReply { return getResponse("GraphEntity") })

	_, err := store.GetEntity(context.Background(), "e1")
	require.ErrorIs(t, err, graphrag.ErrEntityNotFound)
	assert.True(t, graphrag.IsNotFoundError(err))
	q := fw.GraphQLQueries()[0]
	assert.Contains(t, q, `path: ["entityId"] valueString: "e1"`)
	assert.NotContains(t, q, "orgId\"]")
}

func TestGetEntityErrors(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	store := newTestStore(t, fw)

	fw.SetGraphQL(func(string) testutil.WeaviateReply { return gqlError })
	_, err := store.GetEntity(context.Background(), "e1")
	require.ErrorContains(t, err, "failed to query entity")

	fw.SetGraphQL(func(string) testutil.WeaviateReply { return ok(`{"data":{"Get":{}}}`) })
	_, err = store.GetEntity(context.Background(), "e1")
	require.ErrorIs(t, err, graphrag.ErrEntityNotFound)
}

func TestUpdateEntityFake(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	emb := &fakeEmbedder{vectors: map[string][]float32{"Ana": {9, 9}}}
	store := newTestStore(t, fw, WithEmbedder(emb))
	fw.SetGraphQL(func(string) testutil.WeaviateReply {
		return getResponse("GraphEntity", map[string]interface{}{"_additional": map[string]interface{}{"id": "uuid-1"}})
	})

	entity := interfaces.Entity{ID: "e1", Name: "Ana", Type: "Person", Properties: map[string]interface{}{"k": "v"}}
	require.NoError(t, store.UpdateEntity(context.Background(), entity, interfaces.WithGraphTenant("org")))
	assert.Equal(t, []string{"Ana"}, emb.texts(), "the name is embedded when there is no description")

	reqs := fw.RequestsTo(http.MethodPut, "/v1/objects/GraphEntity/uuid-1")
	require.Len(t, reqs, 1)
	var obj models.Object
	require.NoError(t, json.Unmarshal(reqs[0].Body, &obj))
	props := obj.Properties.(map[string]interface{})
	assert.Equal(t, "e1", props["entityId"])
	assert.Equal(t, "Ana", props["name"])
	assert.Equal(t, "org", props["orgId"])
	assert.JSONEq(t, `{"k":"v"}`, props["properties"].(string))
	assert.NotEmpty(t, props["updatedAt"])
	assert.Equal(t, models.C11yVector{9, 9}, obj.Vector)
}

func TestUpdateEntityWithGivenEmbedding(t *testing.T) {
	fw := testutil.NewFakeWeaviate(t)
	emb := &fakeEmbedder{}
	store := newTestStore(t, fw, WithEmbedder(emb))
	fw.SetGraphQL(func(string) testutil.WeaviateReply {
		return getResponse("GraphEntity", map[string]interface{}{"_additional": map[string]interface{}{"id": "uuid-1"}})
	})

	entity := interfaces.Entity{ID: "e1", Name: "n", Embedding: []float32{1, 1}}
	require.NoError(t, store.UpdateEntity(context.Background(), entity, interfaces.WithGenerateEmbeddings(false)))
	assert.Empty(t, emb.texts())

	var obj models.Object
	require.NoError(t, json.Unmarshal(fw.RequestsTo(http.MethodPut, "/v1/objects/GraphEntity/uuid-1")[0].Body, &obj))
	assert.Equal(t, models.C11yVector{1, 1}, obj.Vector)
}

func TestUpdateEntityErrors(t *testing.T) {
	ctx := context.Background()
	found := func(string) testutil.WeaviateReply {
		return getResponse("GraphEntity", map[string]interface{}{"_additional": map[string]interface{}{"id": "uuid-1"}})
	}

	t.Run("missing id", func(t *testing.T) {
		store := newTestStore(t, testutil.NewFakeWeaviate(t))
		require.ErrorIs(t, store.UpdateEntity(ctx, interfaces.Entity{}), graphrag.ErrInvalidEntityID)
	})
	t.Run("lookup fails", func(t *testing.T) {
		fw := testutil.NewFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.SetGraphQL(func(string) testutil.WeaviateReply { return gqlError })
		require.ErrorContains(t, store.UpdateEntity(ctx, interfaces.Entity{ID: "e1"}), "failed to find entity")
	})
	t.Run("not found", func(t *testing.T) {
		store := newTestStore(t, testutil.NewFakeWeaviate(t))
		require.ErrorIs(t, store.UpdateEntity(ctx, interfaces.Entity{ID: "e1"}), graphrag.ErrEntityNotFound)
	})
	t.Run("embedding fails", func(t *testing.T) {
		fw := testutil.NewFakeWeaviate(t)
		store := newTestStore(t, fw, WithEmbedder(&fakeEmbedder{err: errors.New("x")}))
		fw.SetGraphQL(found)
		require.ErrorContains(t, store.UpdateEntity(ctx, interfaces.Entity{ID: "e1", Description: "d"}), "failed to generate embedding")
	})
	t.Run("properties cannot be serialized", func(t *testing.T) {
		fw := testutil.NewFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.SetGraphQL(found)
		err := store.UpdateEntity(ctx, interfaces.Entity{ID: "e1", Properties: map[string]interface{}{"f": func() {}}})
		require.ErrorContains(t, err, "failed to serialize properties")
	})
	t.Run("update request fails", func(t *testing.T) {
		fw := testutil.NewFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.SetGraphQL(found)
		fw.SetHandle(func(req testutil.WeaviateRequest) (testutil.WeaviateReply, bool) {
			if req.Method == http.MethodPut {
				return gqlError, true
			}
			return testutil.WeaviateReply{}, false
		})
		require.ErrorContains(t, store.UpdateEntity(ctx, interfaces.Entity{ID: "e1"}), "failed to update entity")
	})
}

func TestDeleteEntityFake(t *testing.T) {
	ctx := context.Background()
	found := func(string) testutil.WeaviateReply {
		return getResponse("GraphEntity", map[string]interface{}{"_additional": map[string]interface{}{"id": "uuid-1"}})
	}

	fw := testutil.NewFakeWeaviate(t)
	store := newTestStore(t, fw)
	fw.SetGraphQL(found)
	require.NoError(t, store.DeleteEntity(ctx, "e1", interfaces.WithGraphTenant("org")))
	assert.Len(t, fw.RequestsTo(http.MethodDelete, "/v1/objects/GraphEntity/uuid-1"), 1)
	assert.Contains(t, fw.GraphQLQueries()[0], `path: ["orgId"] valueString: "org"`)

	require.ErrorIs(t, store.DeleteEntity(ctx, ""), graphrag.ErrInvalidEntityID)

	fw.SetGraphQL(func(string) testutil.WeaviateReply { return getResponse("GraphEntity") })
	require.ErrorIs(t, store.DeleteEntity(ctx, "e1"), graphrag.ErrEntityNotFound)

	fw.SetGraphQL(func(string) testutil.WeaviateReply { return gqlError })
	require.ErrorContains(t, store.DeleteEntity(ctx, "e1"), "failed to find entity")

	fw.SetGraphQL(found)
	fw.SetHandle(func(req testutil.WeaviateRequest) (testutil.WeaviateReply, bool) {
		if req.Method == http.MethodDelete {
			return gqlError, true
		}
		return testutil.WeaviateReply{}, false
	})
	require.ErrorContains(t, store.DeleteEntity(ctx, "e1"), "failed to delete entity")
}

func TestExtractUUID(t *testing.T) {
	tests := []struct {
		name string
		data map[string]models.JSONObject
		want string
	}{
		{"nil data", nil, ""},
		{"no Get", map[string]models.JSONObject{"Other": 1}, ""},
		{"no class", map[string]models.JSONObject{"Get": map[string]interface{}{}}, ""},
		{"empty class", map[string]models.JSONObject{"Get": map[string]interface{}{"C": []interface{}{}}}, ""},
		{"item not a map", map[string]models.JSONObject{"Get": map[string]interface{}{"C": []interface{}{"x"}}}, ""},
		{"no additional", map[string]models.JSONObject{"Get": map[string]interface{}{"C": []interface{}{map[string]interface{}{}}}}, ""},
		{"id not a string", map[string]models.JSONObject{"Get": map[string]interface{}{"C": []interface{}{
			map[string]interface{}{"_additional": map[string]interface{}{"id": 5}},
		}}}, ""},
		{"found", map[string]models.JSONObject{"Get": map[string]interface{}{"C": []interface{}{
			map[string]interface{}{"_additional": map[string]interface{}{"id": "u"}},
		}}}, "u"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractUUID(&models.GraphQLResponse{Data: tt.data}, "C"))
		})
	}
}

func TestParseEntityResultsEdgeCases(t *testing.T) {
	assert.Empty(t, parseEntityResults(&models.GraphQLResponse{}, "C"))
	assert.Empty(t, parseEntityResults(&models.GraphQLResponse{Data: map[string]models.JSONObject{"Get": "x"}}, "C"))
	assert.Empty(t, parseEntityResults(&models.GraphQLResponse{Data: map[string]models.JSONObject{"Get": map[string]interface{}{}}}, "C"))

	got := parseEntityResults(&models.GraphQLResponse{Data: map[string]models.JSONObject{"Get": map[string]interface{}{"C": []interface{}{
		"not a map",
		map[string]interface{}{
			"entityId":    "e1",
			"properties":  "not json",
			"createdAt":   "yesterday",
			"_additional": map[string]interface{}{"vector": []interface{}{1.0, "x"}},
		},
	}}}}, "C")
	require.Len(t, got, 1)
	assert.Equal(t, "e1", got[0].ID)
	assert.Nil(t, got[0].Properties, "invalid JSON properties are ignored")
	assert.True(t, got[0].CreatedAt.IsZero(), "invalid timestamps are ignored")
	assert.Equal(t, []float32{1, 0}, got[0].Embedding, "non-numeric vector values become zero")
}

func TestBuildEntityIDFilter(t *testing.T) {
	single := buildEntityIDFilter("e1", "").Build()
	assert.Equal(t, "Equal", single.Operator)
	assert.Equal(t, []string{"entityId"}, single.Path)
	assert.Equal(t, "e1", *single.ValueString)

	both := buildEntityIDFilter("e1", "org").Build()
	assert.Equal(t, "And", both.Operator)
	require.Len(t, both.Operands, 2)
	assert.Equal(t, []string{"orgId"}, both.Operands[1].Path)
	assert.Equal(t, "org", *both.Operands[1].ValueString)
}
