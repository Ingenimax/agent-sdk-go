package testutil

import (
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// weaviateDo sends one request to fw and returns the status and body.
func weaviateDo(t *testing.T, fw *FakeWeaviate, method, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, "http://"+fw.Host()+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	return resp.StatusCode, string(data)
}

func TestFakeWeaviate_DefaultReplies(t *testing.T) {
	fw := NewFakeWeaviate(t)

	tests := []struct {
		method, path string
		wantStatus   int
		wantBody     string
	}{
		{http.MethodGet, "/v1/meta", http.StatusOK, `{"version":"1.25.0"}`},
		{http.MethodPost, "/v1/graphql", http.StatusOK, `{"data":{}}`},
		{http.MethodGet, "/v1/schema", http.StatusOK, `{"classes":[]}`},
		{http.MethodPost, "/v1/schema", http.StatusOK, `{}`},
		{http.MethodPost, "/v1/batch/objects", http.StatusOK, `[]`},
		{http.MethodDelete, "/v1/objects/Class/id", http.StatusNoContent, ``},
		{http.MethodDelete, "/v1/schema/Class", http.StatusOK, `{}`},
		{http.MethodPut, "/v1/objects/Class/id", http.StatusOK, `{}`},
		{http.MethodPatch, "/v1/objects/Class/id", http.StatusOK, `{}`},
		{http.MethodGet, "/v1/unknown", http.StatusNotFound, `{"error":[{"message":"not found"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			status, body := weaviateDo(t, fw, tt.method, tt.path, "")
			if status != tt.wantStatus || body != tt.wantBody {
				t.Errorf("got %d %q, want %d %q", status, body, tt.wantStatus, tt.wantBody)
			}
		})
	}
}

func TestFakeWeaviate_RecordsRequestsAndGraphQL(t *testing.T) {
	fw := NewFakeWeaviate(t)

	weaviateDo(t, fw, http.MethodPost, "/v1/graphql", `{"query":"{ Get { A } }"}`)
	weaviateDo(t, fw, http.MethodPost, "/v1/graphql", `{"query":"{ Get { B } }"}`)
	weaviateDo(t, fw, http.MethodGet, "/v1/schema?x=1", "")

	if got := fw.GraphQLQueries(); !reflect.DeepEqual(got, []string{"{ Get { A } }", "{ Get { B } }"}) {
		t.Errorf("GraphQLQueries() = %v", got)
	}
	schema := fw.RequestsTo(http.MethodGet, "/v1/schema")
	if len(schema) != 1 || schema[0].Query.Get("x") != "1" {
		t.Errorf("RequestsTo(GET, /v1/schema) = %+v, want one request with x=1", schema)
	}

	fw.Reset()
	if got := fw.RequestsTo(http.MethodGet, "/v1/schema"); len(got) != 0 {
		t.Errorf("RequestsTo() after Reset = %v, want none", got)
	}
}

func TestFakeWeaviate_HandleAndGraphQLOverrides(t *testing.T) {
	fw := NewFakeWeaviate(t)
	fw.SetGraphQL(func(query string) WeaviateReply {
		return WeaviateReply{Status: http.StatusOK, Body: `{"data":{"q":"` + query + `"}}`}
	})
	fw.SetHandle(func(r WeaviateRequest) (WeaviateReply, bool) {
		if r.Path == "/v1/meta" {
			return WeaviateReply{Status: http.StatusTeapot, Body: `{"handled":true}`}, true
		}
		return WeaviateReply{}, false
	})

	if status, body := weaviateDo(t, fw, http.MethodGet, "/v1/meta", ""); status != http.StatusTeapot || body != `{"handled":true}` {
		t.Errorf("handled request got %d %q", status, body)
	}
	if status, body := weaviateDo(t, fw, http.MethodPost, "/v1/graphql", `{"query":"x"}`); status != http.StatusOK || body != `{"data":{"q":"x"}}` {
		t.Errorf("GraphQL request got %d %q", status, body)
	}

	fw.SetHandle(nil)
	if status, _ := weaviateDo(t, fw, http.MethodGet, "/v1/meta", ""); status != http.StatusOK {
		t.Errorf("after SetHandle(nil) status = %d, want 200", status)
	}
}
