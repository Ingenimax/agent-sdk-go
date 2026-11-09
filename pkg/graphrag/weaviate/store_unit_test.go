package weaviate

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestNewCreatesMissingSchema(t *testing.T) {
	fw := newFakeWeaviate(t)
	store, err := New(&Config{Host: fw.host(), Scheme: "http", ClassPrefix: "Kg"})
	require.NoError(t, err)
	assert.Equal(t, "KgEntity", store.getEntityClassName())
	assert.Equal(t, "KgRelationship", store.getRelationshipClassName())

	created := fw.requestsTo(http.MethodPost, "/v1/schema")
	require.Len(t, created, 2)
	var names []string
	for _, req := range created {
		var class struct {
			Class      string `json:"class"`
			Properties []struct {
				Name string `json:"name"`
			} `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(req.Body, &class))
		names = append(names, class.Class)
		assert.NotEmpty(t, class.Properties)
	}
	assert.Equal(t, []string{"KgEntity", "KgRelationship"}, names)
}

func TestNewSkipsExistingSchema(t *testing.T) {
	fw := newFakeWeaviate(t)
	fw.setHandle(func(req fakeRequest) (fakeReply, bool) {
		if req.Method == http.MethodGet && req.Path == "/v1/schema" {
			return ok(`{"classes":[{"class":"GraphEntity"},{"class":"GraphRelationship"}]}`), true
		}
		return fakeReply{}, false
	})
	_, err := New(&Config{Host: fw.host(), Scheme: "http"})
	require.NoError(t, err)
	assert.Empty(t, fw.requestsTo(http.MethodPost, "/v1/schema"))
}

func TestNewToleratesSchemaErrors(t *testing.T) {
	tests := []struct {
		name   string
		handle func(req fakeRequest) (fakeReply, bool)
	}{
		{"schema lookup fails", func(req fakeRequest) (fakeReply, bool) {
			if req.Path == "/v1/schema" {
				return gqlError, true
			}
			return fakeReply{}, false
		}},
		{"class create fails", func(req fakeRequest) (fakeReply, bool) {
			if req.Method == http.MethodPost && req.Path == "/v1/schema" {
				return gqlError, true
			}
			return fakeReply{}, false
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fw := newFakeWeaviate(t)
			fw.setHandle(tt.handle)
			store, err := New(&Config{Host: fw.host(), Scheme: "http"})
			require.NoError(t, err, "schema errors are logged, not returned")
			assert.NotNil(t, store.client)
		})
	}
}

func TestEnsureSchemaErrors(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)

	// The entity class exists, so only the relationship class is created, and that fails.
	fw.setHandle(func(req fakeRequest) (fakeReply, bool) {
		switch {
		case req.Method == http.MethodGet && req.Path == "/v1/schema":
			return ok(`{"classes":[{"class":"GraphEntity"}]}`), true
		case req.Method == http.MethodPost && req.Path == "/v1/schema":
			return gqlError, true
		}
		return fakeReply{}, false
	})
	err := store.ensureSchema(context.Background())
	require.ErrorContains(t, err, "failed to create relationship schema")
	require.ErrorContains(t, err, "failed to create relationship class")

	fw.setHandle(func(req fakeRequest) (fakeReply, bool) {
		if req.Path == "/v1/schema" {
			return gqlError, true
		}
		return fakeReply{}, false
	})
	err = store.ensureSchema(context.Background())
	require.ErrorContains(t, err, "failed to create entity schema")
	require.ErrorContains(t, err, "failed to check if class exists")
	require.ErrorContains(t, store.ensureRelationshipSchema(context.Background()), "failed to check if class exists")

	fw.setHandle(func(req fakeRequest) (fakeReply, bool) {
		if req.Method == http.MethodPost && req.Path == "/v1/schema" {
			return gqlError, true
		}
		return fakeReply{}, false
	})
	require.ErrorContains(t, store.ensureEntitySchema(context.Background()), "failed to create entity class")
}

func TestNewWithAPIKeySendsBearerToken(t *testing.T) {
	fw := newFakeWeaviate(t)
	_, err := New(&Config{Host: fw.host(), Scheme: "http", APIKey: "secret"})
	require.NoError(t, err)

	reqs := fw.requestsTo(http.MethodGet, "/v1/schema")
	require.NotEmpty(t, reqs)
	assert.Equal(t, "Bearer secret", reqs[0].Header.Get("Authorization"))
}

func TestStoreOptions(t *testing.T) {
	fw := newFakeWeaviate(t)
	schema := &interfaces.GraphSchema{EntityTypes: []interfaces.EntityTypeSchema{{Name: "Person"}}}
	store := newTestStore(t, fw,
		WithClassPrefix("Custom"),
		WithStoreTenant("acme"),
		WithSchema(schema),
		WithLogger(logging.New()),
	)
	assert.Equal(t, "CustomEntity", store.getEntityClassName())
	assert.Equal(t, "acme", store.GetTenant())
	assert.Same(t, schema, store.schema)

	store.SetTenant("other")
	assert.Equal(t, "other", store.GetTenant())
	assert.NoError(t, store.Close())
}

func TestApplyAndDiscoverStoredSchema(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)

	schema := interfaces.GraphSchema{
		EntityTypes:       []interfaces.EntityTypeSchema{{Name: "Person"}},
		RelationshipTypes: []interfaces.RelationshipTypeSchema{{Name: "KNOWS"}},
	}
	require.NoError(t, store.ApplySchema(context.Background(), schema))

	got, err := store.DiscoverSchema(context.Background())
	require.NoError(t, err)
	assert.Equal(t, schema, *got)
	assert.Empty(t, fw.queries(), "a stored schema needs no query")
}

func TestDiscoverSchemaFromData(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)
	fw.setGraphQL(func(query string) fakeReply {
		if containsAll(query, "Aggregate", "GraphEntity") {
			return aggregateResponse("GraphEntity", groupedBy("Person"), groupedBy("Org"), map[string]interface{}{"bad": 1})
		}
		return aggregateResponse("GraphRelationship", groupedBy("WORKS_AT"))
	})

	got, err := store.DiscoverSchema(context.Background())
	require.NoError(t, err)
	require.Len(t, got.EntityTypes, 2)
	assert.Equal(t, "Person", got.EntityTypes[0].Name)
	assert.Equal(t, "Discovered entity type: Person", got.EntityTypes[0].Description)
	assert.Equal(t, "Org", got.EntityTypes[1].Name)
	require.Len(t, got.RelationshipTypes, 1)
	assert.Equal(t, "WORKS_AT", got.RelationshipTypes[0].Name)
	assert.Equal(t, "Discovered relationship type: WORKS_AT", got.RelationshipTypes[0].Description)

	for _, q := range fw.queries() {
		assert.Contains(t, q, "groupBy")
	}
}

func TestDiscoverSchemaQueryErrors(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)
	fw.setGraphQL(func(string) fakeReply { return gqlError })

	got, err := store.DiscoverSchema(context.Background())
	require.NoError(t, err, "discovery errors are logged, not returned")
	assert.Empty(t, got.EntityTypes)
	assert.Empty(t, got.RelationshipTypes)
}

func TestApplyOptionDefaults(t *testing.T) {
	assert.Equal(t, &interfaces.GraphStoreOptions{BatchSize: 100, GenerateEmbeddings: true}, applyStoreOptions(nil))
	assert.Equal(t, &interfaces.GraphStoreOptions{BatchSize: 5, GenerateEmbeddings: false, Tenant: "t"},
		applyStoreOptions([]interfaces.GraphStoreOption{
			interfaces.WithGraphBatchSize(5), interfaces.WithGenerateEmbeddings(false), interfaces.WithGraphTenant("t"),
		}))

	assert.Equal(t, &interfaces.GraphSearchOptions{MaxDepth: 2, SearchMode: interfaces.SearchModeHybrid}, applySearchOptions(nil))
	assert.Equal(t, interfaces.SearchModeKeyword,
		applySearchOptions([]interfaces.GraphSearchOption{interfaces.WithSearchMode(interfaces.SearchModeKeyword)}).SearchMode)

	assert.Equal(t, &interfaces.ExtractionOptions{MinConfidence: 0.5, MaxEntities: 50, DedupThreshold: 0.92}, applyExtractionOptions(nil))
	assert.Equal(t, 3, applyExtractionOptions([]interfaces.ExtractionOption{interfaces.WithMaxEntities(3)}).MaxEntities)
}

func TestCountAllEntities(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)

	fw.setGraphQL(func(string) fakeReply {
		return aggregateResponse("GraphEntity", map[string]interface{}{"meta": map[string]interface{}{"count": 42}})
	})
	n, err := store.CountAllEntities(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 42, n)
	assert.Contains(t, fw.queries()[0], "Aggregate")

	fw.setGraphQL(func(string) fakeReply { return ok(`{"data":{}}`) })
	n, err = store.CountAllEntities(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	fw.setGraphQL(func(string) fakeReply { return gqlError })
	_, err = store.CountAllEntities(context.Background())
	require.Error(t, err)
}

func TestListAllEntities(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)

	fw.setGraphQL(func(string) fakeReply {
		return getResponse("GraphEntity",
			map[string]interface{}{"entityId": "e1", "name": "Ana", "entityType": "Person"},
			map[string]interface{}{"entityId": "e2", "name": "Acme", "entityType": "Org"},
		)
	})
	got, err := store.ListAllEntities(context.Background(), 0)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "Ana", got[0].Name)
	assert.Equal(t, "Org", got[1].Type)
	assert.Contains(t, fw.queries()[0], "limit: 100", "a non-positive limit defaults to 100")

	fw.reset()
	fw.setGraphQL(func(string) fakeReply { return ok(`{"data":{"Get":{}}}`) })
	got, err = store.ListAllEntities(context.Background(), 7)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Contains(t, fw.queries()[0], "limit: 7")

	fw.setGraphQL(func(string) fakeReply { return gqlError })
	_, err = store.ListAllEntities(context.Background(), 1)
	require.Error(t, err)
}

func TestDeleteSchema(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)

	schemaReply := ok(`{"classes":[{"class":"GraphEntity"},{"class":"GraphRelationship"}]}`)
	fw.setHandle(func(req fakeRequest) (fakeReply, bool) {
		if req.Method == http.MethodGet && req.Path == "/v1/schema" {
			return schemaReply, true
		}
		return fakeReply{}, false
	})
	require.NoError(t, store.DeleteSchema(context.Background()))
	assert.Len(t, fw.requestsTo(http.MethodDelete, "/v1/schema/GraphEntity"), 1)
	assert.Len(t, fw.requestsTo(http.MethodDelete, "/v1/schema/GraphRelationship"), 1)

	// Nothing to delete when neither class exists.
	fw.reset()
	fw.setHandle(nil)
	require.NoError(t, store.DeleteSchema(context.Background()))
	assert.Empty(t, fw.requestsTo(http.MethodDelete, "/v1/schema/GraphEntity"))
}

func TestDeleteSchemaErrors(t *testing.T) {
	tests := []struct {
		name    string
		handle  func(req fakeRequest) (fakeReply, bool)
		wantErr string
	}{
		{
			name: "entity check fails",
			handle: func(req fakeRequest) (fakeReply, bool) {
				if req.Path == "/v1/schema" {
					return gqlError, true
				}
				return fakeReply{}, false
			},
			wantErr: "failed to check entity class",
		},
		{
			name: "entity delete fails",
			handle: func(req fakeRequest) (fakeReply, bool) {
				switch {
				case req.Path == "/v1/schema":
					return ok(`{"classes":[{"class":"GraphEntity"}]}`), true
				case req.Method == http.MethodDelete:
					return gqlError, true
				}
				return fakeReply{}, false
			},
			wantErr: "failed to delete entity class",
		},
		{
			name: "relationship delete fails",
			handle: func(req fakeRequest) (fakeReply, bool) {
				switch {
				case req.Path == "/v1/schema":
					return ok(`{"classes":[{"class":"GraphRelationship"}]}`), true
				case req.Method == http.MethodDelete:
					return gqlError, true
				}
				return fakeReply{}, false
			},
			wantErr: "failed to delete relationship class",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fw := newFakeWeaviate(t)
			store := newTestStore(t, fw)
			fw.setHandle(tt.handle)
			require.ErrorContains(t, store.DeleteSchema(context.Background()), tt.wantErr)
		})
	}
}

func TestDeleteSchemaRelationshipCheckFails(t *testing.T) {
	fw := newFakeWeaviate(t)
	store := newTestStore(t, fw)

	// The first schema lookup succeeds and the second fails.
	calls := 0
	fw.setHandle(func(req fakeRequest) (fakeReply, bool) {
		if req.Path == "/v1/schema" {
			calls++
			if calls > 1 {
				return gqlError, true
			}
			return ok(`{"classes":[]}`), true
		}
		return fakeReply{}, false
	})
	require.ErrorContains(t, store.DeleteSchema(context.Background()), "failed to check relationship class")
}
