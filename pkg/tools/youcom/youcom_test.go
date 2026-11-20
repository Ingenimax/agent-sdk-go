package youcom_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/youcom"
)

var _ interfaces.Tool = (*youcom.Tool)(nil)

const sampleResponse = `{"results":{"web":[
 {"title":"Result 1","url":"https://example.com/1","description":"d1","page_age":"2026-11-01T00:00:00","snippets":["first","more"]},
 {"title":"Result 2","url":"https://example.com/2","description":"second description","snippets":[]},
 {"title":"Result 3","url":"https://example.com/3","description":"third","snippets":["third"]}
]},"metadata":{"query":"q"}}`

type captured struct {
	method, path, query, apiKey, userAgent, org string
	body                                        map[string]interface{}
}

func newServer(t *testing.T, calls *int32, got *captured) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		raw, _ := io.ReadAll(r.Body)
		got.body = map[string]interface{}{}
		_ = json.Unmarshal(raw, &got.body)
		got.method = r.Method
		got.path = r.URL.Path
		got.query = r.URL.RawQuery
		got.apiKey = r.Header.Get("X-API-Key")
		got.userAgent = r.Header.Get("User-Agent")
		got.org = r.Header.Get("X-Organization-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleResponse))
	}))
}

func TestKeylessSearch(t *testing.T) {
	var calls int32
	var got captured
	server := newServer(t, &calls, &got)
	defer server.Close()

	tool := youcom.New("", youcom.WithBaseURL(server.URL))
	out, err := tool.Run(context.Background(), `{"query":"golang agents","num_results":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.method != http.MethodGet || got.path != "/v1/agents/search" {
		t.Errorf("request = %s %s, want GET /v1/agents/search", got.method, got.path)
	}
	if !strings.Contains(got.query, "query=golang+agents") || !strings.Contains(got.query, "count=2") {
		t.Errorf("query string = %q", got.query)
	}
	if got.apiKey != "" {
		t.Errorf("keyless request sent X-API-Key %q", got.apiKey)
	}
	if got.userAgent != "youdotcom-integration/Ingenimax-agent-sdk-go" {
		t.Errorf("User-Agent = %q", got.userAgent)
	}
	for _, want := range []string{"Search results for 'golang agents'", "1. Result 1", "URL: https://example.com/1", "Published: 2026-11-01T00:00:00", "first more", "2. Result 2", "second description"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Result 3") {
		t.Errorf("output should be limited to 2 results:\n%s", out)
	}
}

func TestKeylessCountIsCapped(t *testing.T) {
	var calls int32
	var got captured
	server := newServer(t, &calls, &got)
	defer server.Close()

	if _, err := youcom.New("", youcom.WithBaseURL(server.URL)).Run(context.Background(), `{"query":"q","num_results":80}`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.query, "count=50") {
		t.Errorf("query string = %q, want count=50", got.query)
	}
}

// TestKeyedSearchDoesNotSendOrgID also guards against forwarding the tenant ID:
// You.com is a third party, so sending it would only tell the vendor which
// tenant made each query.
func TestKeyedSearchDoesNotSendOrgID(t *testing.T) {
	var calls int32
	var got captured
	server := newServer(t, &calls, &got)
	defer server.Close()

	tool := youcom.New(" test-key ", youcom.WithBaseURL(server.URL))
	ctx := multitenancy.WithOrgID(context.Background(), "org-1")
	if _, err := tool.Run(ctx, "plain text query"); err != nil {
		t.Fatal(err)
	}
	if got.method != http.MethodPost || got.path != "/v1/search" || got.apiKey != "test-key" {
		t.Errorf("request = %s %s, X-API-Key = %q", got.method, got.path, got.apiKey)
	}
	if got.org != "" {
		t.Errorf("X-Organization-ID = %q, want it not sent to a third party", got.org)
	}
	if got.body["query"] != "plain text query" || got.body["count"] != float64(5) {
		t.Errorf("body = %v", got.body)
	}
	if got.userAgent != "youdotcom-integration/Ingenimax-agent-sdk-go" {
		t.Errorf("User-Agent = %q", got.userAgent)
	}
}

func TestCacheAndExecute(t *testing.T) {
	var calls int32
	var got captured
	server := newServer(t, &calls, &got)
	defer server.Close()

	tool := youcom.New("", youcom.WithBaseURL(server.URL))
	for i := 0; i < 2; i++ {
		if _, err := tool.Execute(context.Background(), `{"query":"same"}`); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Errorf("expected 1 upstream call with caching, got %d", calls)
	}
}

func TestErrors(t *testing.T) {
	tool := youcom.New("")
	if _, err := tool.Run(context.Background(), `{"query":"  "}`); err == nil {
		t.Error("expected error for empty query")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
	}))
	defer server.Close()
	tool = youcom.New("bad", youcom.WithBaseURL(server.URL))
	_, err := tool.Run(context.Background(), "q")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 error, got %v", err)
	}
}

func TestErrorBodyIsTruncated(t *testing.T) {
	// An HTML error page from a proxy or CDN. Tool errors are passed to the LLM
	// verbatim, so the whole page would land in the model's context.
	page := "<html>" + strings.Repeat("é", 2<<20) + "</html>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(page))
	}))
	defer server.Close()

	_, err := youcom.New("", youcom.WithBaseURL(server.URL)).Run(context.Background(), "q")
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if len(msg) > 1024 {
		t.Errorf("error is %d bytes, want it bounded", len(msg))
	}
	if !strings.Contains(msg, "502") || !strings.Contains(msg, "(truncated)") {
		t.Errorf("error should keep the status and mark the cut: %q", msg)
	}
	if !utf8.ValidString(msg) {
		t.Error("truncated error is not valid UTF-8")
	}
}

func TestEmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":{"web":[]}}`))
	}))
	defer server.Close()

	out, err := youcom.New("", youcom.WithBaseURL(server.URL)).Run(context.Background(), "nothing matches")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No results found") {
		t.Errorf("empty result set should say so, got %q", out)
	}
}

func TestRedirectIsNotFollowed(t *testing.T) {
	var leaked int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "" {
			atomic.AddInt32(&leaked, 1)
		}
		_, _ = w.Write([]byte(sampleResponse))
	}))
	defer other.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	_, err := youcom.New("secret", youcom.WithBaseURL(redirector.URL)).Run(context.Background(), "q")
	if err == nil || atomic.LoadInt32(&leaked) != 0 {
		t.Errorf("redirect must not be followed (err=%v, leaked=%d)", err, leaked)
	}
}
