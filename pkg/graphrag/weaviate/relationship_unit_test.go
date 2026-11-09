package weaviate

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weaviate/weaviate/entities/models"

	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func validRel(id string) interfaces.Relationship {
	return interfaces.Relationship{ID: id, SourceID: "a", TargetID: "b", Type: "KNOWS"}
}

func TestStoreRelationshipsValidation(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)
	ctx := context.Background()

	require.NoError(t, store.StoreRelationships(ctx, nil))

	mutate := func(f func(r *interfaces.Relationship)) interfaces.Relationship {
		r := validRel("r1")
		f(&r)
		return r
	}
	tests := []struct {
		name string
		rel  interfaces.Relationship
		want error
	}{
		{"missing id", mutate(func(r *interfaces.Relationship) { r.ID = "" }), graphrag.ErrInvalidRelationshipID},
		{"missing source", mutate(func(r *interfaces.Relationship) { r.SourceID = "" }), graphrag.ErrMissingSourceID},
		{"missing target", mutate(func(r *interfaces.Relationship) { r.TargetID = "" }), graphrag.ErrMissingTargetID},
		{"missing type", mutate(func(r *interfaces.Relationship) { r.Type = "" }), graphrag.ErrMissingRelationshipType},
		{"negative strength", mutate(func(r *interfaces.Relationship) { r.Strength = -0.1 }), graphrag.ErrInvalidStrength},
		{"strength above one", mutate(func(r *interfaces.Relationship) { r.Strength = 1.1 }), graphrag.ErrInvalidStrength},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorIs(t, store.StoreRelationships(ctx, []interfaces.Relationship{tt.rel}), tt.want)
		})
	}
	assert.Empty(t, fw.requestsTo(http.MethodPost, "/v1/batch/objects"))
}

func TestStoreRelationshipsSendsObjects(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	rels := []interfaces.Relationship{
		{ID: "r1", SourceID: "a", TargetID: "b", Type: "KNOWS", Description: "d", Strength: 0.25,
			Properties: map[string]interface{}{"since": 2020}, CreatedAt: created},
		validRel("r2"),
	}
	ctx := multitenancy.WithOrgID(context.Background(), "org")
	require.NoError(t, store.StoreRelationships(ctx, rels))
	assert.False(t, rels[1].CreatedAt.IsZero(), "missing timestamps are filled in place")

	objs := batchObjects(t, fw.requestsTo(http.MethodPost, "/v1/batch/objects")[0])
	require.Len(t, objs, 2)
	assert.Equal(t, "GraphRelationship", objs[0]["class"])
	assert.NotContains(t, objs[0], "vector")

	p := objs[0]["properties"].(map[string]interface{})
	assert.Equal(t, "r1", p["relationshipId"])
	assert.Equal(t, "a", p["sourceId"])
	assert.Equal(t, "b", p["targetId"])
	assert.Equal(t, "KNOWS", p["relationshipType"])
	assert.Equal(t, "d", p["description"])
	assert.InDelta(t, 0.25, p["strength"], 1e-6)
	assert.JSONEq(t, `{"since":2020}`, p["properties"].(string))
	assert.Equal(t, "org", p["orgId"])
	assert.Equal(t, "2024-05-06T07:08:09Z", p["createdAt"])

	// A zero strength defaults to 1.0.
	assert.InDelta(t, 1.0, objs[1]["properties"].(map[string]interface{})["strength"], 1e-6)
}

func TestStoreRelationshipsTenantFallsBackToStore(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw, WithStoreTenant("store"))
	require.NoError(t, store.StoreRelationships(context.Background(), []interfaces.Relationship{validRel("r")}))
	objs := batchObjects(t, fw.requestsTo(http.MethodPost, "/v1/batch/objects")[0])
	assert.Equal(t, "store", objs[0]["properties"].(map[string]interface{})["orgId"])

	fw.reset()
	require.NoError(t, store.StoreRelationships(context.Background(), []interfaces.Relationship{validRel("r")}, interfaces.WithGraphTenant("opt")))
	objs = batchObjects(t, fw.requestsTo(http.MethodPost, "/v1/batch/objects")[0])
	assert.Equal(t, "opt", objs[0]["properties"].(map[string]interface{})["orgId"])
}

func TestStoreRelationshipsBatching(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)
	fw.setHandle(func(req fakeRequest) (fakeReply, bool) {
		if req.Path == "/v1/batch/objects" {
			return ok(`[{"result":{"errors":{"error":[{"message":"bad"}]}}}]`), true
		}
		return fakeReply{}, false
	})

	rels := []interfaces.Relationship{validRel("1"), validRel("2"), validRel("3")}
	require.NoError(t, store.StoreRelationships(context.Background(), rels, interfaces.WithGraphBatchSize(2)))
	reqs := fw.requestsTo(http.MethodPost, "/v1/batch/objects")
	require.Len(t, reqs, 2)
	assert.Len(t, batchObjects(t, reqs[0]), 2)
	assert.Len(t, batchObjects(t, reqs[1]), 1)

	fw.reset()
	require.NoError(t, store.StoreRelationships(context.Background(), rels, interfaces.WithGraphBatchSize(-1)))
	assert.Len(t, fw.requestsTo(http.MethodPost, "/v1/batch/objects"), 1, "a non-positive batch size uses the default")
}

func TestStoreRelationshipsErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("properties cannot be serialized", func(t *testing.T) {
		store := newTestStore(t, newFakeWeaviate(t))
		r := validRel("r1")
		r.Properties = map[string]interface{}{"c": make(chan int)}
		require.ErrorContains(t, store.StoreRelationships(ctx, []interfaces.Relationship{r}), "failed to serialize properties for relationship r1")
	})

	batchFails := func(req fakeRequest) (fakeReply, bool) {
		if req.Path == "/v1/batch/objects" {
			return gqlError, true
		}
		return fakeReply{}, false
	}
	t.Run("final batch fails", func(t *testing.T) {
		fw := newFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.setHandle(batchFails)
		require.ErrorContains(t, store.StoreRelationships(ctx, []interfaces.Relationship{validRel("r")}), "failed to store final relationship batch")
	})
	t.Run("full batch fails", func(t *testing.T) {
		fw := newFakeWeaviate(t)
		store := newTestStore(t, fw)
		fw.setHandle(batchFails)
		err := store.StoreRelationships(ctx, []interfaces.Relationship{validRel("r")}, interfaces.WithGraphBatchSize(1))
		require.ErrorContains(t, err, "failed to store relationship batch")
	})
}

func TestGetRelationshipsFake(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)

	_, err := store.GetRelationships(context.Background(), "", interfaces.DirectionBoth)
	require.ErrorIs(t, err, graphrag.ErrInvalidEntityID)

	fw.setGraphQL(func(string) fakeReply {
		return getResponse("GraphRelationship", map[string]interface{}{
			"relationshipId":   "r1",
			"sourceId":         "a",
			"targetId":         "b",
			"relationshipType": "KNOWS",
			"description":      "d",
			"strength":         0.5,
			"properties":       `{"x":1}`,
			"orgId":            "org",
			"createdAt":        "2024-01-01T00:00:00Z",
		})
	})
	got, err := store.GetRelationships(context.Background(), "a", interfaces.DirectionOutgoing,
		interfaces.WithSearchTenant("org"), interfaces.WithRelationshipTypes("KNOWS"))
	require.NoError(t, err)
	assert.Equal(t, []interfaces.Relationship{{
		ID: "r1", SourceID: "a", TargetID: "b", Type: "KNOWS", Description: "d", Strength: 0.5,
		Properties: map[string]interface{}{"x": 1.0}, OrgID: "org",
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}}, got)

	q := fw.queries()[0]
	assert.Contains(t, q, `path: ["sourceId"] valueString: "a"`)
	assert.Contains(t, q, `path: ["orgId"] valueString: "org"`)
	assert.Contains(t, q, `path: ["relationshipType"] valueString: "KNOWS"`)

	fw.setGraphQL(func(string) fakeReply { return gqlError })
	_, err = store.GetRelationships(context.Background(), "a", interfaces.DirectionIncoming)
	require.ErrorContains(t, err, "failed to query relationships")
}

func TestBuildRelationshipDirectionFilter(t *testing.T) {
	out := buildRelationshipDirectionFilter("e", interfaces.DirectionOutgoing, "", nil).Build()
	assert.Equal(t, []string{"sourceId"}, out.Path)
	assert.Equal(t, "e", *out.ValueString)

	in := buildRelationshipDirectionFilter("e", interfaces.DirectionIncoming, "", nil).Build()
	assert.Equal(t, []string{"targetId"}, in.Path)

	both := buildRelationshipDirectionFilter("e", interfaces.DirectionBoth, "", nil).Build()
	assert.Equal(t, "Or", both.Operator)
	require.Len(t, both.Operands, 2)
	assert.Equal(t, []string{"sourceId"}, both.Operands[0].Path)
	assert.Equal(t, []string{"targetId"}, both.Operands[1].Path)

	full := buildRelationshipDirectionFilter("e", interfaces.DirectionOutgoing, "org", []string{"A", "B"}).Build()
	assert.Equal(t, "And", full.Operator)
	require.Len(t, full.Operands, 3)
	assert.Equal(t, []string{"orgId"}, full.Operands[1].Path)
	types := full.Operands[2]
	assert.Equal(t, "Or", types.Operator)
	require.Len(t, types.Operands, 2)
	assert.Equal(t, "A", *types.Operands[0].ValueString)
	assert.Equal(t, "B", *types.Operands[1].ValueString)

	oneType := buildRelationshipDirectionFilter("e", interfaces.DirectionIncoming, "", []string{"A"}).Build()
	assert.Equal(t, "And", oneType.Operator)
	require.Len(t, oneType.Operands, 2)
	assert.Equal(t, []string{"relationshipType"}, oneType.Operands[1].Path)
}

func TestBuildRelationshipIDFilter(t *testing.T) {
	single := buildRelationshipIDFilter("r1", "").Build()
	assert.Equal(t, []string{"relationshipId"}, single.Path)
	assert.Equal(t, "r1", *single.ValueString)

	both := buildRelationshipIDFilter("r1", "org").Build()
	assert.Equal(t, "And", both.Operator)
	require.Len(t, both.Operands, 2)
	assert.Equal(t, "org", *both.Operands[1].ValueString)
}

func TestDeleteRelationship(t *testing.T) {
	ctx := context.Background()
	found := func(string) fakeReply {
		return getResponse("GraphRelationship", map[string]interface{}{"_additional": map[string]interface{}{"id": "uuid-r"}})
	}

	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)
	fw.setGraphQL(found)
	require.NoError(t, store.DeleteRelationship(ctx, "r1", interfaces.WithGraphTenant("org")))
	assert.Len(t, fw.requestsTo(http.MethodDelete, "/v1/objects/GraphRelationship/uuid-r"), 1)
	assert.Contains(t, fw.queries()[0], `path: ["relationshipId"] valueString: "r1"`)

	require.ErrorIs(t, store.DeleteRelationship(ctx, ""), graphrag.ErrInvalidRelationshipID)

	fw.setGraphQL(func(string) fakeReply { return getResponse("GraphRelationship") })
	require.ErrorIs(t, store.DeleteRelationship(multitenancy.WithOrgID(ctx, "o"), "r1"), graphrag.ErrRelationshipNotFound)

	fw.setGraphQL(func(string) fakeReply { return gqlError })
	require.ErrorContains(t, store.DeleteRelationship(ctx, "r1"), "failed to find relationship")

	fw.setGraphQL(found)
	fw.setHandle(func(req fakeRequest) (fakeReply, bool) {
		if req.Method == http.MethodDelete {
			return gqlError, true
		}
		return fakeReply{}, false
	})
	require.ErrorContains(t, store.DeleteRelationship(ctx, "r1"), "failed to delete relationship")
}

func TestParseRelationshipResultsEdgeCases(t *testing.T) {
	assert.Empty(t, parseRelationshipResults(&models.GraphQLResponse{}, "C"))
	assert.Empty(t, parseRelationshipResults(&models.GraphQLResponse{Data: map[string]models.JSONObject{"Get": 1}}, "C"))
	assert.Empty(t, parseRelationshipResults(&models.GraphQLResponse{Data: map[string]models.JSONObject{"Get": map[string]interface{}{}}}, "C"))

	got := parseRelationshipResults(&models.GraphQLResponse{Data: map[string]models.JSONObject{"Get": map[string]interface{}{"C": []interface{}{
		1,
		map[string]interface{}{"relationshipId": "r", "properties": "{bad", "createdAt": "nope"},
	}}}}, "C")
	require.Len(t, got, 1)
	assert.Equal(t, "r", got[0].ID)
	assert.Nil(t, got[0].Properties)
	assert.True(t, got[0].CreatedAt.IsZero())
}
