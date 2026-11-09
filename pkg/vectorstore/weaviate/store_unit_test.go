package weaviate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weaviate/weaviate/entities/models"

	"github.com/Ingenimax/agent-sdk-go/pkg/embedding"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// recordedRequest is one HTTP request received by the fake Weaviate server.
type recordedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
	// GraphQL is the query text of a POST /v1/graphql request.
	GraphQL string
}

type reply struct {
	status int
	body   string
}

var serverError = reply{http.StatusInternalServerError, `{"error":[{"message":"boom"}]}`}

// fakeServer answers Weaviate REST and GraphQL calls. Tests override
// individual routes with handle.
type fakeServer struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
	handle   func(req recordedRequest) (reply, bool)
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	fs := &fakeServer{}
	fs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req := recordedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: body}
		if r.URL.Path == "/v1/graphql" {
			var q struct {
				Query string `json:"query"`
			}
			_ = json.Unmarshal(body, &q)
			req.GraphQL = q.Query
		}
		fs.mu.Lock()
		fs.requests = append(fs.requests, req)
		handle := fs.handle
		fs.mu.Unlock()

		rep := reply{http.StatusNotFound, `{"error":[{"message":"not found"}]}`}
		handled := false
		if handle != nil {
			var custom reply
			if custom, handled = handle(req); handled {
				rep = custom
			}
		}
		if !handled {
			switch {
			case req.Path == "/v1/meta":
				rep = reply{http.StatusOK, `{"version":"1.25.0"}`}
			case req.Path == "/v1/schema":
				rep = reply{http.StatusOK, `{"classes":[]}`}
			case req.Path == "/v1/batch/objects":
				rep = reply{http.StatusOK, `[]`}
			case req.Path == "/v1/graphql":
				rep = reply{http.StatusOK, `{"data":{}}`}
			case req.Method == http.MethodDelete && len(req.Path) > len("/v1/objects/") && req.Path[:len("/v1/objects/")] == "/v1/objects/":
				rep = reply{status: http.StatusNoContent}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rep.status)
		_, _ = w.Write([]byte(rep.body))
	}))
	t.Cleanup(fs.srv.Close)
	return fs
}

func (fs *fakeServer) setHandle(h func(req recordedRequest) (reply, bool)) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.handle = h
}

func (fs *fakeServer) requestsTo(method, path string) []recordedRequest {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	var out []recordedRequest
	for _, r := range fs.requests {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

func (fs *fakeServer) graphQL() []string {
	var out []string
	for _, r := range fs.requestsTo(http.MethodPost, "/v1/graphql") {
		out = append(out, r.GraphQL)
	}
	return out
}

func (fs *fakeServer) host() string {
	u, _ := url.Parse(fs.srv.URL)
	return u.Host
}

// stubEmbedder returns a fixed vector, or an error when err is set.
type stubEmbedder struct {
	err error
}

func (s *stubEmbedder) Embed(context.Context, string) ([]float32, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []float32{0.25, 0.5}, nil
}

func (s *stubEmbedder) EmbedWithConfig(ctx context.Context, text string, _ embedding.EmbeddingConfig) ([]float32, error) {
	return s.Embed(ctx, text)
}

func (s *stubEmbedder) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("not used")
}

func (s *stubEmbedder) EmbedBatchWithConfig(context.Context, []string, embedding.EmbeddingConfig) ([][]float32, error) {
	return nil, errors.New("not used")
}

func (s *stubEmbedder) CalculateSimilarity([]float32, []float32, string) (float32, error) {
	return 0, errors.New("not used")
}

func newStore(t *testing.T, fs *fakeServer, opts ...Option) *Store {
	t.Helper()
	store := New(&interfaces.VectorStoreConfig{Host: fs.host(), Scheme: "http"},
		append([]Option{WithEmbedder(&stubEmbedder{})}, opts...)...)
	require.NotNil(t, store)
	return store
}

func searchClass(class string) interfaces.SearchOption {
	return func(o *interfaces.SearchOptions) { o.Class = class }
}

func searchReply(className string, items ...map[string]interface{}) reply {
	b, _ := json.Marshal(map[string]interface{}{"data": map[string]interface{}{"Get": map[string]interface{}{className: items}}})
	return reply{http.StatusOK, string(b)}
}

func onGraphQL(r reply) func(req recordedRequest) (reply, bool) {
	return func(req recordedRequest) (reply, bool) {
		if req.Path == "/v1/graphql" {
			return r, true
		}
		return reply{}, false
	}
}

func TestNewDefaultsAndOptions(t *testing.T) {
	fs := newFakeServer(t)
	store := New(&interfaces.VectorStoreConfig{Host: fs.host(), Scheme: "http"})
	require.NotNil(t, store)
	assert.Equal(t, "Document", store.classPrefix)
	assert.Equal(t, "cosine", store.distanceMetric)
	assert.Nil(t, store.embedder)

	emb := &stubEmbedder{}
	logger := logging.New()
	store = New(&interfaces.VectorStoreConfig{Host: fs.host(), Scheme: "http"},
		WithClassPrefix("Notes"), WithEmbedder(emb), WithDistanceMetric("dot"), WithLogger(logger))
	assert.Equal(t, "Notes", store.classPrefix)
	assert.Same(t, emb, store.embedder)
	assert.Equal(t, "dot", store.distanceMetric)
	assert.Equal(t, logger, store.logger)
}

func TestNewWithAPIKeySendsBearerToken(t *testing.T) {
	fs := newFakeServer(t)
	store := New(&interfaces.VectorStoreConfig{Host: fs.host(), Scheme: "http", APIKey: "secret"})
	require.NotNil(t, store)

	_, err := store.ListTenants(context.Background())
	require.Error(t, err)
	reqs := fs.requestsTo(http.MethodGet, "/v1/schema/Document/tenants")
	require.Len(t, reqs, 1)
	assert.Equal(t, "Bearer secret", reqs[0].Header.Get("Authorization"))
}

func TestGetClassName(t *testing.T) {
	store := newStore(t, newFakeServer(t), WithClassPrefix("Base"))
	name, err := store.getClassName(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "Base", name)
	name, err = store.getClassName(context.Background(), "Other")
	require.NoError(t, err)
	assert.Equal(t, "Other", name)
}

func TestStoreDocuments(t *testing.T) {
	fs := newFakeServer(t)
	store := newStore(t, fs)

	docs := []interfaces.Document{
		{ID: "11111111-1111-1111-1111-111111111111", Content: "first", Metadata: map[string]interface{}{"source": "a"}},
		{ID: "22222222-2222-2222-2222-222222222222", Content: "second"},
		{ID: "33333333-3333-3333-3333-333333333333", Content: "third"},
	}
	require.NoError(t, store.Store(context.Background(), docs,
		interfaces.WithBatchSize(2), interfaces.WithClass("Notes"), interfaces.WithTenant("t1")))

	reqs := fs.requestsTo(http.MethodPost, "/v1/batch/objects")
	require.Len(t, reqs, 2, "three documents with batch size two need two requests")

	var body struct {
		Objects []models.Object `json:"objects"`
	}
	require.NoError(t, json.Unmarshal(reqs[0].Body, &body))
	require.Len(t, body.Objects, 2)
	obj := body.Objects[0]
	assert.Equal(t, "Notes", obj.Class)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", obj.ID.String())
	assert.Equal(t, "t1", obj.Tenant)
	assert.Equal(t, models.C11yVector{0.25, 0.5}, obj.Vector)
	assert.Equal(t, map[string]interface{}{"content": "first", "source": "a"}, obj.Properties)

	require.NoError(t, json.Unmarshal(reqs[1].Body, &body))
	assert.Len(t, body.Objects, 1)
	assert.Equal(t, "t1", body.Objects[0].Tenant, "tenant is set on every object")
}

func TestStoreDocumentsDefaults(t *testing.T) {
	fs := newFakeServer(t)
	store := newStore(t, fs)
	require.NoError(t, store.Store(context.Background(), []interfaces.Document{{ID: "x", Content: "c"}}))

	var body struct {
		Objects []models.Object `json:"objects"`
	}
	require.NoError(t, json.Unmarshal(fs.requestsTo(http.MethodPost, "/v1/batch/objects")[0].Body, &body))
	assert.Equal(t, "Document", body.Objects[0].Class)
	assert.Empty(t, body.Objects[0].Tenant)

	// No documents means no request.
	fs2 := newFakeServer(t)
	require.NoError(t, newStore(t, fs2).Store(context.Background(), nil))
	assert.Empty(t, fs2.requestsTo(http.MethodPost, "/v1/batch/objects"))
}

func TestStoreDocumentsErrors(t *testing.T) {
	doc := interfaces.Document{ID: "x", Content: "c"}

	fs := newFakeServer(t)
	store := newStore(t, fs, WithEmbedder(&stubEmbedder{err: errors.New("no vec")}))
	require.ErrorContains(t, store.Store(context.Background(), []interfaces.Document{doc}), "failed to generate embedding")

	fs = newFakeServer(t)
	store = newStore(t, fs)
	fs.setHandle(func(req recordedRequest) (reply, bool) {
		if req.Path == "/v1/batch/objects" {
			return serverError, true
		}
		return reply{}, false
	})
	require.ErrorContains(t, store.Store(context.Background(), []interfaces.Document{doc}), "failed to store final batch")
	require.ErrorContains(t, store.Store(context.Background(), []interfaces.Document{doc}, interfaces.WithBatchSize(1)), "failed to store batch")
}

func TestSearch(t *testing.T) {
	fs := newFakeServer(t)
	store := newStore(t, fs)
	fs.setHandle(onGraphQL(searchReply("Document",
		map[string]interface{}{"content": "high", "source": "a", "_additional": map[string]interface{}{"id": "1", "certainty": 0.9}},
		map[string]interface{}{"content": "low", "_additional": map[string]interface{}{"id": "2", "certainty": 0.2}},
	)))

	got, err := store.Search(context.Background(), "query", 5,
		interfaces.WithMinScore(0.5),
		interfaces.WithFields("content", "source"),
		interfaces.WithFilters(map[string]interface{}{"operator": "Equal", "path": []string{"source"}, "valueString": "a"}),
		interfaces.WithTenantSearch("t1"),
	)
	require.NoError(t, err)
	require.Len(t, got, 1, "results below the minimum score are dropped")
	assert.Equal(t, "1", got[0].Document.ID)
	assert.Equal(t, "high", got[0].Document.Content)
	assert.Equal(t, map[string]interface{}{"source": "a"}, got[0].Document.Metadata)
	assert.InDelta(t, 0.9, got[0].Score, 1e-6)

	q := fs.graphQL()[0]
	assert.Contains(t, q, "Get {Document")
	assert.Contains(t, q, "nearVector")
	assert.Contains(t, q, "limit: 5")
	assert.Contains(t, q, `tenant: "t1"`)
	assert.Contains(t, q, `path: ["source"] valueString: "a"`)
	assert.Contains(t, q, "content source _additional { certainty id }")
	assert.Empty(t, fs.requestsTo(http.MethodGet, "/v1/schema"), "explicit fields skip schema discovery")
}

func TestSearchDiscoversFieldsFromSchema(t *testing.T) {
	fs := newFakeServer(t)
	store := newStore(t, fs)
	fs.setHandle(func(req recordedRequest) (reply, bool) {
		switch req.Path {
		case "/v1/schema":
			return reply{http.StatusOK, `{"classes":[{"class":"Other"},{"class":"Document","properties":[{"name":"content"},{"name":"title"}]}]}`}, true
		case "/v1/graphql":
			return searchReply("Document"), true
		}
		return reply{}, false
	})

	got, err := store.Search(context.Background(), "q", 1, interfaces.WithFilters(map[string]interface{}{"bad": "filter"}))
	require.NoError(t, err)
	assert.Empty(t, got)
	q := fs.graphQL()[0]
	assert.Contains(t, q, "content title _additional { certainty id }")
	assert.NotContains(t, q, "where:", "a filter that cannot be built is dropped")
}

func TestBuildFieldListFallbacks(t *testing.T) {
	fs := newFakeServer(t)
	store := newStore(t, fs)

	// Class missing from the schema.
	fields, err := store.buildFieldList(context.Background(), "Document", nil)
	require.NoError(t, err)
	assert.Equal(t, "content _additional { certainty id }", fields)

	// Schema lookup fails.
	fs.setHandle(func(req recordedRequest) (reply, bool) {
		if req.Path == "/v1/schema" {
			return serverError, true
		}
		return reply{}, false
	})
	fields, err = store.buildFieldList(context.Background(), "Document", nil)
	require.NoError(t, err)
	assert.Equal(t, "content _additional { certainty id }", fields)

	fields, err = store.buildFieldList(context.Background(), "Document", []string{"a"})
	require.NoError(t, err)
	assert.Equal(t, "a _additional { certainty id }", fields)
}

func TestSearchErrors(t *testing.T) {
	fs := newFakeServer(t)
	store := newStore(t, fs, WithEmbedder(&stubEmbedder{err: errors.New("x")}))
	_, err := store.Search(context.Background(), "q", 1)
	require.ErrorContains(t, err, "failed to generate embedding for query")

	store = newStore(t, fs)
	fs.setHandle(onGraphQL(serverError))
	_, err = store.Search(context.Background(), "q", 1)
	require.ErrorContains(t, err, "failed to execute search")
}

func TestSearchByVector(t *testing.T) {
	fs := newFakeServer(t)
	store := newStore(t, fs)
	fs.setHandle(onGraphQL(searchReply("Notes",
		map[string]interface{}{"content": "c", "_additional": map[string]interface{}{"id": "1", "certainty": 0.1}},
	)))

	got, err := store.SearchByVector(context.Background(), []float32{1, 2}, 3,
		searchClass("Notes"), interfaces.WithTenantSearch("t"), interfaces.WithMinScore(0.9))
	require.NoError(t, err)
	// SearchByVector does not apply the minimum score.
	require.Len(t, got, 1)
	assert.InDelta(t, 0.1, got[0].Score, 1e-6)

	q := fs.graphQL()[0]
	assert.Contains(t, q, "Get {Notes")
	assert.Contains(t, q, "vector: [1,2]")
	assert.Contains(t, q, `tenant: "t"`)

	fs.setHandle(onGraphQL(serverError))
	_, err = store.SearchByVector(context.Background(), []float32{1}, 1)
	require.ErrorContains(t, err, "failed to execute vector search")
}

func TestGlobalVariantsDelegate(t *testing.T) {
	fs := newFakeServer(t)
	store := newStore(t, fs)
	ctx := context.Background()

	require.NoError(t, store.GlobalStore(ctx, []interfaces.Document{{ID: "x", Content: "c"}}))
	assert.Len(t, fs.requestsTo(http.MethodPost, "/v1/batch/objects"), 1)

	_, err := store.GlobalSearch(ctx, "q", 1)
	require.NoError(t, err)
	_, err = store.GlobalSearchByVector(ctx, []float32{1}, 1)
	require.NoError(t, err)
	assert.Len(t, fs.graphQL(), 2)

	require.NoError(t, store.GlobalDelete(ctx, []string{"id-1"}))
	assert.Len(t, fs.requestsTo(http.MethodDelete, "/v1/objects/Document/id-1"), 1)
}

func TestDelete(t *testing.T) {
	fs := newFakeServer(t)
	store := newStore(t, fs)

	require.NoError(t, store.Delete(context.Background(), []string{"a", "b"},
		func(o *interfaces.DeleteOptions) { o.Class = "Notes" }, interfaces.WithTenantDelete("t1")))
	a := fs.requestsTo(http.MethodDelete, "/v1/objects/Notes/a")
	require.Len(t, a, 1)
	assert.Equal(t, "t1", a[0].Query.Get("tenant"))
	assert.Len(t, fs.requestsTo(http.MethodDelete, "/v1/objects/Notes/b"), 1)

	fs.setHandle(func(req recordedRequest) (reply, bool) {
		if req.Method == http.MethodDelete {
			return serverError, true
		}
		return reply{}, false
	})
	require.ErrorContains(t, store.Delete(context.Background(), []string{"a"}), "failed to delete document a")
}

func TestGet(t *testing.T) {
	fs := newFakeServer(t)
	store := newStore(t, fs)
	fs.setHandle(func(req recordedRequest) (reply, bool) {
		if req.Method == http.MethodGet && req.Path == "/v1/objects/Notes/doc-1" {
			return reply{http.StatusOK, `{"class":"Notes","id":"doc-1","properties":{"content":"hello","source":"a"}}`}, true
		}
		return reply{}, false
	})

	doc, err := store.Get(context.Background(), "doc-1", interfaces.WithClass("Notes"), interfaces.WithTenant("t1"))
	require.NoError(t, err)
	assert.Equal(t, &interfaces.Document{ID: "doc-1", Content: "hello", Metadata: map[string]interface{}{"source": "a"}}, doc)
	assert.Equal(t, "t1", fs.requestsTo(http.MethodGet, "/v1/objects/Notes/doc-1")[0].Query.Get("tenant"))

	_, err = store.Get(context.Background(), "missing")
	require.ErrorContains(t, err, "failed to get document missing")

	fs.setHandle(func(req recordedRequest) (reply, bool) {
		if req.Method == http.MethodGet && req.Path != "/v1/meta" {
			return serverError, true
		}
		return reply{}, false
	})
	_, err = store.Get(context.Background(), "doc-1")
	require.ErrorContains(t, err, "failed to get document doc-1")
}

func TestTenants(t *testing.T) {
	fs := newFakeServer(t)
	store := newStore(t, fs, WithClassPrefix("Notes"))
	ctx := context.Background()
	fs.setHandle(func(req recordedRequest) (reply, bool) {
		if req.Path == "/v1/schema/Notes/tenants" {
			if req.Method == http.MethodGet {
				return reply{http.StatusOK, `[{"name":"a"},{"name":"b"}]`}, true
			}
			return reply{http.StatusOK, `[]`}, true
		}
		return reply{}, false
	})

	require.NoError(t, store.CreateTenant(ctx, "a"))
	created := fs.requestsTo(http.MethodPost, "/v1/schema/Notes/tenants")
	require.Len(t, created, 1)
	assert.JSONEq(t, `[{"name":"a"}]`, string(created[0].Body))

	require.NoError(t, store.DeleteTenant(ctx, "a"))
	deleted := fs.requestsTo(http.MethodDelete, "/v1/schema/Notes/tenants")
	require.Len(t, deleted, 1)
	assert.JSONEq(t, `["a"]`, string(deleted[0].Body))

	names, err := store.ListTenants(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, names)

	fs.setHandle(func(req recordedRequest) (reply, bool) {
		if req.Path == "/v1/schema/Notes/tenants" {
			return serverError, true
		}
		return reply{}, false
	})
	require.ErrorContains(t, store.CreateTenant(ctx, "a"), "failed to create tenant a")
	require.ErrorContains(t, store.DeleteTenant(ctx, "a"), "failed to delete tenant a")
	_, err = store.ListTenants(ctx)
	require.ErrorContains(t, err, "failed to list tenants")
}

func TestParseSearchResults(t *testing.T) {
	store := newStore(t, newFakeServer(t))

	got, err := store.parseSearchResults(&models.GraphQLResponse{}, "C")
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = store.parseSearchResults(&models.GraphQLResponse{Data: map[string]models.JSONObject{"Get": "bad"}}, "C")
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = store.parseSearchResults(&models.GraphQLResponse{Data: map[string]models.JSONObject{"Get": map[string]interface{}{}}}, "C")
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = store.parseSearchResults(&models.GraphQLResponse{Data: map[string]models.JSONObject{"Get": map[string]interface{}{"C": []interface{}{
		map[string]interface{}{"content": "no additional"},
		map[string]interface{}{"_additional": map[string]interface{}{"id": "1"}},
		map[string]interface{}{"content": "no id", "_additional": map[string]interface{}{}},
		map[string]interface{}{"content": "default certainty", "_additional": map[string]interface{}{"id": "4"}},
	}}}}, "C")
	require.NoError(t, err)
	require.Len(t, got, 1, "items missing _additional, content or id are skipped")
	assert.Equal(t, "4", got[0].Document.ID)
	assert.InDelta(t, 0.5, got[0].Score, 1e-6, "a missing certainty defaults to 0.5")
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		in   interface{}
		want float64
	}{
		{1.5, 1.5},
		{float32(2.5), 2.5},
		{3, 3},
		{int32(4), 4},
		{int64(5), 5},
		{"6.5", 6.5},
		{"bad", 0},
		{true, 0},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, toFloat64(tt.in), "%v", tt.in)
	}
}
