package weaviate

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/embedding"
)

func ok(body string) testutil.WeaviateReply {
	return testutil.WeaviateReply{Status: http.StatusOK, Body: body}
}

var gqlError = testutil.WeaviateReply{Status: http.StatusInternalServerError, Body: `{"error":[{"message":"boom"}]}`}

// newTestStore creates a store backed by the fake server and clears the
// requests made while ensuring the schema.
func newTestStore(t *testing.T, fw *testutil.FakeWeaviate, opts ...Option) *Store {
	t.Helper()
	store, err := New(&Config{Host: fw.Host(), Scheme: "http"}, append([]Option{WithEmbedder(&MockEmbedder{})}, opts...)...)
	require.NoError(t, err)
	fw.Reset()
	return store
}

// batchObjects decodes the objects of a POST /v1/batch/objects request.
func batchObjects(t *testing.T, req testutil.WeaviateRequest) []map[string]interface{} {
	t.Helper()
	var body struct {
		Objects []map[string]interface{} `json:"objects"`
	}
	require.NoError(t, json.Unmarshal(req.Body, &body))
	return body.Objects
}

// getResponse builds a GraphQL Get response for one class.
func getResponse(className string, items ...map[string]interface{}) testutil.WeaviateReply {
	data := map[string]interface{}{"data": map[string]interface{}{"Get": map[string]interface{}{className: items}}}
	b, _ := json.Marshal(data)
	return ok(string(b))
}

// aggregateResponse builds a GraphQL Aggregate response for one class.
func aggregateResponse(className string, items ...map[string]interface{}) testutil.WeaviateReply {
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

// lastPrompt returns the most recent prompt llm received, or "" if none.
func lastPrompt(llm *testutil.FakeLLM) string {
	prompts := llm.Prompts()
	if len(prompts) == 0 {
		return ""
	}
	return prompts[len(prompts)-1]
}

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

func (g *graph) answer(query string) testutil.WeaviateReply {
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
