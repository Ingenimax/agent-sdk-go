package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// WeaviateRequest is one HTTP request received by FakeWeaviate.
type WeaviateRequest struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
	// GraphQL is the query text of a POST /v1/graphql request.
	GraphQL string
}

// WeaviateReply is the status and JSON body FakeWeaviate sends back.
type WeaviateReply struct {
	Status int
	Body   string
}

// FakeWeaviate is an httptest server that speaks enough of the Weaviate REST
// and GraphQL API for a client to run against it. It records every request.
// Tests override replies with SetHandle and SetGraphQL.
type FakeWeaviate struct {
	srv *httptest.Server

	mu       sync.Mutex
	requests []WeaviateRequest
	handle   func(WeaviateRequest) (WeaviateReply, bool)
	graphQL  func(query string) WeaviateReply
}

// NewFakeWeaviate starts a FakeWeaviate that is closed when the test ends.
func NewFakeWeaviate(t *testing.T) *FakeWeaviate {
	t.Helper()
	fw := &FakeWeaviate{}
	fw.srv = httptest.NewServer(http.HandlerFunc(fw.serve))
	t.Cleanup(fw.srv.Close)
	return fw
}

// Host returns the server's host:port, as the Weaviate client config wants it.
func (fw *FakeWeaviate) Host() string {
	return strings.TrimPrefix(fw.srv.URL, "http://")
}

// SetHandle installs a handler that answers before the defaults. It returns
// false to fall through to the defaults. Nil removes it.
func (fw *FakeWeaviate) SetHandle(h func(WeaviateRequest) (WeaviateReply, bool)) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.handle = h
}

// SetGraphQL answers GraphQL queries that the handle does not take. The
// default returns no data.
func (fw *FakeWeaviate) SetGraphQL(h func(query string) WeaviateReply) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.graphQL = h
}

// RequestsTo returns the recorded requests that match method and path.
func (fw *FakeWeaviate) RequestsTo(method, path string) []WeaviateRequest {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	var out []WeaviateRequest
	for _, r := range fw.requests {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// GraphQLQueries returns the GraphQL query texts received so far, in order.
func (fw *FakeWeaviate) GraphQLQueries() []string {
	var out []string
	for _, r := range fw.RequestsTo(http.MethodPost, "/v1/graphql") {
		out = append(out, r.GraphQL)
	}
	return out
}

// Reset forgets the recorded requests.
func (fw *FakeWeaviate) Reset() {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.requests = nil
}

func (fw *FakeWeaviate) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	req := WeaviateRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: body}
	if r.URL.Path == "/v1/graphql" {
		var q struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &q)
		req.GraphQL = q.Query
	}

	fw.mu.Lock()
	fw.requests = append(fw.requests, req)
	handle, gql := fw.handle, fw.graphQL
	fw.mu.Unlock()

	reply, handled := WeaviateReply{}, false
	if handle != nil {
		reply, handled = handle(req)
	}
	if !handled {
		reply = defaultWeaviateReply(req, gql)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(reply.Status)
	_, _ = w.Write([]byte(reply.Body))
}

func defaultWeaviateReply(req WeaviateRequest, gql func(string) WeaviateReply) WeaviateReply {
	ok := func(body string) WeaviateReply { return WeaviateReply{Status: http.StatusOK, Body: body} }
	switch {
	case req.Path == "/v1/meta":
		return ok(`{"version":"1.25.0"}`)
	case req.Path == "/v1/graphql":
		if gql != nil {
			return gql(req.GraphQL)
		}
		return ok(`{"data":{}}`)
	case req.Method == http.MethodGet && req.Path == "/v1/schema":
		return ok(`{"classes":[]}`)
	case req.Method == http.MethodPost && req.Path == "/v1/schema":
		return ok(`{}`)
	case req.Method == http.MethodPost && req.Path == "/v1/batch/objects":
		return ok(`[]`)
	case req.Method == http.MethodDelete && strings.HasPrefix(req.Path, "/v1/objects/"):
		return WeaviateReply{Status: http.StatusNoContent}
	case req.Method == http.MethodDelete:
		return ok(`{}`)
	case req.Method == http.MethodPut || req.Method == http.MethodPatch:
		return ok(`{}`)
	}
	return WeaviateReply{Status: http.StatusNotFound, Body: `{"error":[{"message":"not found"}]}`}
}
