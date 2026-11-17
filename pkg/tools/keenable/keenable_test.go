package keenable_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/keenable"
)

var _ interfaces.Tool = (*keenable.Tool)(nil)

const sampleResponse = `{"query":"q","mode":"pro","results":[
 {"title":"Result 1","url":"https://example.com/1","description":"","snippet":"first","published_at":"2026-11-01T00:00:00Z"},
 {"title":"Result 2","url":"https://example.com/2","description":"","snippet":"second"},
 {"title":"Result 3","url":"https://example.com/3","description":"","snippet":"third"}
]}`

type captured struct {
	path, apiKey, title, org string
	body                     map[string]interface{}
}

func newServer(t *testing.T, calls *int32, got *captured) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		got.body = map[string]interface{}{}
		_ = json.Unmarshal(raw, &got.body)
		got.path = r.URL.Path
		got.apiKey = r.Header.Get("X-API-Key")
		got.title = r.Header.Get("X-Keenable-Title")
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

	tool := keenable.New("", keenable.WithBaseURL(server.URL))
	out, err := tool.Run(context.Background(), `{"query":"golang agents","num_results":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.path != "/v1/search/public" {
		t.Errorf("path = %s, want /v1/search/public", got.path)
	}
	if got.apiKey != "" {
		t.Errorf("keyless request sent X-API-Key %q", got.apiKey)
	}
	if got.title != "agent-sdk-go" {
		t.Errorf("X-Keenable-Title = %q", got.title)
	}
	if got.body["query"] != "golang agents" || got.body["max_results"] != float64(2) {
		t.Errorf("body = %v", got.body)
	}
	for _, want := range []string{"Search results for 'golang agents'", "1. Result 1", "URL: https://example.com/1", "Published: 2026-11-01T00:00:00Z", "2. Result 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Result 3") {
		t.Errorf("output should be limited to 2 results:\n%s", out)
	}
}

// TestKeyedSearchDoesNotSendOrgID also guards against forwarding the tenant ID:
// Keenable is a third party and its API has no X-Organization-ID header, so
// sending it would only tell the vendor which tenant made each query.
func TestKeyedSearchDoesNotSendOrgID(t *testing.T) {
	var calls int32
	var got captured
	server := newServer(t, &calls, &got)
	defer server.Close()

	tool := keenable.New(" test-key ", keenable.WithBaseURL(server.URL))
	ctx := multitenancy.WithOrgID(context.Background(), "org-1")
	if _, err := tool.Run(ctx, "plain text query"); err != nil {
		t.Fatal(err)
	}
	if got.path != "/v1/search" || got.apiKey != "test-key" {
		t.Errorf("path = %s, X-API-Key = %q", got.path, got.apiKey)
	}
	if got.org != "" {
		t.Errorf("X-Organization-ID = %q, want it not sent to a third party", got.org)
	}
	if got.body["query"] != "plain text query" {
		t.Errorf("body = %v", got.body)
	}
}

func TestCacheAndExecute(t *testing.T) {
	var calls int32
	var got captured
	server := newServer(t, &calls, &got)
	defer server.Close()

	tool := keenable.New("", keenable.WithBaseURL(server.URL))
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
	tool := keenable.New("")
	if _, err := tool.Run(context.Background(), `{"query":"  "}`); err == nil {
		t.Error("expected error for empty query")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
	}))
	defer server.Close()
	tool = keenable.New("bad", keenable.WithBaseURL(server.URL))
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

	_, err := keenable.New("", keenable.WithBaseURL(server.URL)).Run(context.Background(), "q")
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

func TestRateLimitErrorKeepsRetryHint(t *testing.T) {
	body := `{"code":"public_rate_limit_hourly","limit":{"scope":"ip","window":"1h","max":1000,"resets_at":"2026-11-16T10:00:00Z"},"retry_after_seconds":1200}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1200")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	_, err := keenable.New("", keenable.WithBaseURL(server.URL)).Run(context.Background(), "q")
	if err == nil || !strings.Contains(err.Error(), body) {
		t.Errorf("a short error body should be kept whole, got %v", err)
	}
}

func TestEmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"query":"q","results":[]}`))
	}))
	defer server.Close()

	out, err := keenable.New("", keenable.WithBaseURL(server.URL)).Run(context.Background(), "nothing matches")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No results found") {
		t.Errorf("empty result set should say so, got %q", out)
	}
}

func TestMetadata(t *testing.T) {
	tool := keenable.New("")
	if tool.Name() != "web_search" {
		t.Errorf("Name = %q", tool.Name())
	}
	if keenable.New("", keenable.WithName("keenable_search")).Name() != "keenable_search" {
		t.Error("WithName not applied")
	}
	if _, ok := tool.Parameters()["query"]; !ok {
		t.Error("missing query parameter")
	}
}

// TestLive calls the real keyless endpoint. Opt in with KEENABLE_LIVE_TEST=1.
func TestLive(t *testing.T) {
	if os.Getenv("KEENABLE_LIVE_TEST") == "" {
		t.Skip("set KEENABLE_LIVE_TEST=1 to call api.keenable.ai")
	}
	out, err := keenable.New(os.Getenv("KEENABLE_API_KEY")).Run(context.Background(), `{"query":"Go programming language","num_results":3}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "URL: https://") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	t.Log(out)
}
