package weaviate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/embedding"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// fakeRequest is one HTTP request received by the fake Weaviate server.
type fakeRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
	// Query is the GraphQL query text for POST /v1/graphql requests.
	Query string
}

// fakeReply is the status and JSON body the fake server sends back.
type fakeReply struct {
	Status int
	Body   string
}

func ok(body string) fakeReply { return fakeReply{Status: http.StatusOK, Body: body} }

// fakeWeaviate is an httptest server that speaks enough of the Weaviate
// REST and GraphQL API for the store to run against it.
type fakeWeaviate struct {
	srv *httptest.Server

	mu       sync.Mutex
	requests []fakeRequest
	// handle answers requests that the defaults below do not cover.
	handle func(req fakeRequest) (fakeReply, bool)
	// graphql answers GraphQL queries. The default returns no data.
	graphql func(query string) fakeReply
}

func newFakeWeaviate(t *testing.T) *fakeWeaviate {
	t.Helper()
	fw := &fakeWeaviate{}
	fw.srv = httptest.NewServer(http.HandlerFunc(fw.serve))
	t.Cleanup(fw.srv.Close)
	return fw
}

func (fw *fakeWeaviate) host() string {
	u, _ := url.Parse(fw.srv.URL)
	return u.Host
}

func (fw *fakeWeaviate) setHandle(h func(req fakeRequest) (fakeReply, bool)) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.handle = h
}

func (fw *fakeWeaviate) setGraphQL(h func(query string) fakeReply) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.graphql = h
}

func (fw *fakeWeaviate) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	req := fakeRequest{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: body}
	if r.URL.Path == "/v1/graphql" {
		var q struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &q)
		req.Query = q.Query
	}

	fw.mu.Lock()
	fw.requests = append(fw.requests, req)
	handle, gql := fw.handle, fw.graphql
	fw.mu.Unlock()

	reply := fw.defaultReply(req, handle, gql)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(reply.Status)
	_, _ = w.Write([]byte(reply.Body))
}

func (fw *fakeWeaviate) defaultReply(req fakeRequest, handle func(fakeRequest) (fakeReply, bool), gql func(string) fakeReply) fakeReply {
	if handle != nil {
		if reply, handled := handle(req); handled {
			return reply
		}
	}
	switch {
	case req.Path == "/v1/meta":
		return ok(`{"version":"1.25.0"}`)
	case req.Path == "/v1/graphql":
		if gql != nil {
			return gql(req.Query)
		}
		return ok(`{"data":{}}`)
	case req.Method == http.MethodGet && req.Path == "/v1/schema":
		return ok(`{"classes":[]}`)
	case req.Method == http.MethodPost && req.Path == "/v1/schema":
		return ok(`{}`)
	case req.Method == http.MethodPost && req.Path == "/v1/batch/objects":
		return ok(`[]`)
	case req.Method == http.MethodDelete && strings.HasPrefix(req.Path, "/v1/objects/"):
		return fakeReply{Status: http.StatusNoContent}
	case req.Method == http.MethodDelete:
		return ok(`{}`)
	case req.Method == http.MethodPut || req.Method == http.MethodPatch:
		return ok(`{}`)
	}
	return fakeReply{Status: http.StatusNotFound, Body: `{"error":[{"message":"not found"}]}`}
}

// requestsTo returns the recorded requests that match method and path.
func (fw *fakeWeaviate) requestsTo(method, path string) []fakeRequest {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	var out []fakeRequest
	for _, r := range fw.requests {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// queries returns the GraphQL queries received so far.
func (fw *fakeWeaviate) queries() []string {
	var out []string
	for _, r := range fw.requestsTo(http.MethodPost, "/v1/graphql") {
		out = append(out, r.Query)
	}
	return out
}

func (fw *fakeWeaviate) reset() {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.requests = nil
}

// newTestStore creates a store backed by the fake server and clears the
// requests made while ensuring the schema.
func newTestStore(t *testing.T, fw *fakeWeaviate, opts ...Option) *Store {
	t.Helper()
	store, err := New(&Config{Host: fw.host(), Scheme: "http"}, append([]Option{WithEmbedder(&MockEmbedder{})}, opts...)...)
	require.NoError(t, err)
	fw.reset()
	return store
}

// batchObjects decodes the objects of a POST /v1/batch/objects request.
func batchObjects(t *testing.T, req fakeRequest) []map[string]interface{} {
	t.Helper()
	var body struct {
		Objects []map[string]interface{} `json:"objects"`
	}
	require.NoError(t, json.Unmarshal(req.Body, &body))
	return body.Objects
}

// getResponse builds a GraphQL Get response for one class.
func getResponse(className string, items ...map[string]interface{}) fakeReply {
	data := map[string]interface{}{"data": map[string]interface{}{"Get": map[string]interface{}{className: items}}}
	b, _ := json.Marshal(data)
	return ok(string(b))
}

// aggregateResponse builds a GraphQL Aggregate response for one class.
func aggregateResponse(className string, items ...map[string]interface{}) fakeReply {
	data := map[string]interface{}{"data": map[string]interface{}{"Aggregate": map[string]interface{}{className: items}}}
	b, _ := json.Marshal(data)
	return ok(string(b))
}

func groupedBy(value string) map[string]interface{} {
	return map[string]interface{}{"groupedBy": map[string]interface{}{"value": value}}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

var gqlError = fakeReply{Status: http.StatusInternalServerError, Body: `{"error":[{"message":"boom"}]}`}

// valueFor returns the string value bound to a path in a GraphQL where clause.
func valueFor(query, path string) (string, bool) {
	re := regexp.MustCompile(`path: \["` + regexp.QuoteMeta(path) + `"\] valueString: "([^"]*)"`)
	m := re.FindStringSubmatch(query)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// graph is a tiny in-memory knowledge graph that answers the entity and
// relationship queries the store sends.
type graph struct {
	entities map[string]map[string]interface{}
	rels     []map[string]interface{}
}

func newGraph() *graph {
	return &graph{entities: map[string]map[string]interface{}{}}
}

func (g *graph) addEntity(id, name, typ string) {
	g.entities[id] = map[string]interface{}{
		"entityId":    id,
		"name":        name,
		"entityType":  typ,
		"description": name + " description",
		"_additional": map[string]interface{}{"id": "uuid-" + id, "score": 0.8},
	}
}

func (g *graph) addRel(id, src, tgt, typ string) {
	g.rels = append(g.rels, map[string]interface{}{
		"relationshipId":   id,
		"sourceId":         src,
		"targetId":         tgt,
		"relationshipType": typ,
		"strength":         0.9,
	})
}

func (g *graph) answer(query string) fakeReply {
	switch {
	case strings.Contains(query, "Get {GraphEntity"):
		id, hasID := valueFor(query, "entityId")
		if !hasID {
			items := make([]map[string]interface{}, 0, len(g.entities))
			for _, e := range g.entities {
				items = append(items, e)
			}
			return getResponse("GraphEntity", items...)
		}
		if e, found := g.entities[id]; found {
			return getResponse("GraphEntity", e)
		}
		return getResponse("GraphEntity")
	case strings.Contains(query, "Get {GraphRelationship"):
		src, hasSrc := valueFor(query, "sourceId")
		tgt, hasTgt := valueFor(query, "targetId")
		var items []map[string]interface{}
		for _, r := range g.rels {
			switch {
			case hasSrc && hasTgt && (r["sourceId"] == src || r["targetId"] == tgt):
				items = append(items, r)
			case hasSrc && !hasTgt && r["sourceId"] == src:
				items = append(items, r)
			case hasTgt && !hasSrc && r["targetId"] == tgt:
				items = append(items, r)
			}
		}
		return getResponse("GraphRelationship", items...)
	}
	return ok(`{"data":{}}`)
}

// fakeEmbedder is an embedder whose behaviour each test controls.
type fakeEmbedder struct {
	MockEmbedder
	err        error
	batchErr   error
	similarity func(a, b []float32) (float32, error)
	vectors    map[string][]float32
	mu         sync.Mutex
	embedded   []string
}

func (f *fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	f.mu.Lock()
	f.embedded = append(f.embedded, text)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if v, found := f.vectors[text]; found {
		return v, nil
	}
	return []float32{0.1, 0.2}, nil
}

func (f *fakeEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	if f.batchErr != nil {
		return nil, f.batchErr
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		if v, found := f.vectors[text]; found {
			out[i] = v
		} else {
			out[i] = []float32{0, 0}
		}
	}
	return out, nil
}

func (f *fakeEmbedder) CalculateSimilarity(a, b []float32, metric string) (float32, error) {
	if f.similarity != nil {
		return f.similarity(a, b)
	}
	return embedding.CalculateSimilarity(a, b, metric)
}

func (f *fakeEmbedder) texts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.embedded...)
}

// fakeLLM returns a fixed response from Generate.
type fakeLLM struct {
	response string
	err      error
	prompt   string
}

func (l *fakeLLM) Generate(_ context.Context, prompt string, _ ...interfaces.GenerateOption) (string, error) {
	l.prompt = prompt
	return l.response, l.err
}

func (l *fakeLLM) GenerateWithTools(context.Context, string, []interfaces.Tool, ...interfaces.GenerateOption) (string, error) {
	return "", errors.New("not used")
}

func (l *fakeLLM) GenerateDetailed(context.Context, string, ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return nil, errors.New("not used")
}

func (l *fakeLLM) GenerateWithToolsDetailed(context.Context, string, []interfaces.Tool, ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return nil, errors.New("not used")
}

func (l *fakeLLM) Name() string            { return "fake" }
func (l *fakeLLM) SupportsStreaming() bool { return false }
