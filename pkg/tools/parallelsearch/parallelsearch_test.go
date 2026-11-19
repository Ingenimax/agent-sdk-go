package parallelsearch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

type rpcRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type callParams struct {
	Name      string                     `json:"name"`
	Arguments map[string]json.RawMessage `json:"arguments"`
}

func fixtureTool(t *testing.T, call func(http.ResponseWriter, *http.Request, rpcRequest)) *Tool {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" || r.URL.RawQuery != "" {
			t.Error("expected an MCP request without URL query parameters")
		}
		if r.UserAgent() != "agent-sdk-go" {
			t.Errorf("User-Agent = %q, want agent-sdk-go", r.UserAgent())
		}
		for _, name := range []string{"Authorization", "Cookie", "X-API-Key", "X-Organization-ID", "X-Keenable-Title"} {
			if len(r.Header.Values(name)) != 0 {
				t.Errorf("unexpected %s header", name)
			}
		}

		var msg rpcRequest
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
				t.Errorf("decode MCP request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		t.Logf("wire %s /mcp %s User-Agent=%s (no auth or tenant headers)", r.Method, msg.Method, r.UserAgent())
		beforeInit := msg.Method == "server/discover" || msg.Method == "initialize"
		if beforeInit && r.Header.Get("Mcp-Session-Id") != "" {
			t.Error("new query reused a prior protocol session")
		}
		if !beforeInit && r.Header.Get("Mcp-Session-Id") != "fixture-session" {
			t.Error("subsequent request missing the protocol session header")
		}

		switch {
		case msg.Method == "server/discover":
			// This server negotiates with the legacy initialize handshake.
			w.WriteHeader(http.StatusNotFound)
		case msg.Method == "initialize":
			w.Header().Set("Mcp-Session-Id", "fixture-session")
			// The server's cookie must never be sent back on later requests.
			http.SetCookie(w, &http.Cookie{Name: "test-cookie", Value: "fixture"})
			writeRPC(t, w, msg, `{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"fixture","version":"1"}}`)
		case msg.Method == "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case msg.Method == "tools/call", r.Method == http.MethodGet:
			call(w, r, msg)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, msg.Method)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)

	tool := New()
	tool.endpoint = server.URL + "/mcp"
	return tool
}

func writeRPC(t *testing.T, w http.ResponseWriter, msg rpcRequest, result string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":%s}`, msg.ID, result); err != nil {
		t.Errorf("write fixture response: %v", err)
	}
}

func assertSearch(t *testing.T, msg rpcRequest, query string) {
	t.Helper()
	var params callParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Errorf("decode search arguments: %v", err)
		return
	}
	var objective string
	var queries []string
	if params.Name != "web_search" || len(params.Arguments) != 2 ||
		json.Unmarshal(params.Arguments["objective"], &objective) != nil ||
		json.Unmarshal(params.Arguments["search_queries"], &queries) != nil ||
		objective != query || len(queries) != 1 || queries[0] != query {
		t.Error("search should send only the original query as its objective and search query")
	}
}

const sampleResults = `{"content":[],"structuredContent":{"results":[
	{"title":"Foreign URL","url":"javascript:alert(1)","excerpts":[]},
	{"title":"First","url":"https://example.org/a?q=one%20two#section","excerpts":["Snippet","Another excerpt"]},
	{"title":"Second","url":"https://example.org/b","excerpts":["Second snippet"]},
	{"title":"Third","url":"https://example.org/c","excerpts":[]},
	{"title":"Fourth","url":"https://example.org/d","excerpts":[]},
	{"title":"Fifth","url":"https://example.org/e","excerpts":[]},
	{"title":"Sixth","url":"https://example.org/f","excerpts":[]}
]}}`

func TestExecuteWire(t *testing.T) {
	var mu sync.Mutex
	var resumeID json.RawMessage
	var calls, resumes int
	tool := fixtureTool(t, func(w http.ResponseWriter, r *http.Request, msg rpcRequest) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodGet {
			resumes++
			if r.Header.Get("Last-Event-ID") != "fixture-event" {
				t.Error("resume was missing the previous event ID")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			if _, err := fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":%s}\n\n",
				resumeID, strings.ReplaceAll(sampleResults, "\n", "")); err != nil {
				t.Errorf("write resumed stream: %v", err)
			}
			return
		}
		calls++
		assertSearch(t, msg, "public Go documentation")
		if calls == 1 {
			// Finish the first HTTP stream before sending its reply. The SDK
			// must resume it with a GET and the same project User-Agent.
			resumeID = msg.ID
			w.Header().Set("Content-Type", "text/event-stream")
			if _, err := fmt.Fprint(w, "id: fixture-event\nretry: 1\ndata:\n\n"); err != nil {
				t.Errorf("write initial stream: %v", err)
			}
			return
		}
		writeRPC(t, w, msg, sampleResults)
	})

	const firstCitation = "1. First\n   URL: https://example.org/a?q=one%20two#section\n   Snippet\n   Another excerpt"
	var search interfaces.Tool = tool
	ctx := multitenancy.WithOrgID(t.Context(), "fixture-tenant-one")
	out, err := search.Execute(ctx, `{"query":"public Go documentation","num_results":1}`)
	if err != nil || !strings.Contains(out, firstCitation) ||
		strings.Contains(out, "Second") || strings.Contains(out, "javascript:") {
		t.Fatalf("limited results = %q, err = %v", out, err)
	}
	ctx = multitenancy.WithOrgID(t.Context(), "fixture-tenant-two")
	out, err = search.Run(ctx, "public Go documentation")
	if err != nil || !strings.HasPrefix(out, "Search results for 'public Go documentation':\n\n") ||
		!strings.Contains(out, "5. Fifth") || strings.Contains(out, "Sixth") {
		t.Fatalf("default results = %q, err = %v", out, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 || resumes != 1 {
		t.Errorf("got %d calls and %d resumes, want 2 and 1", calls, resumes)
	}
}

func TestExecuteResponses(t *testing.T) {
	tests := []struct {
		name, response, want, wantErr string
		status                        int
	}{
		{name: "empty", response: `{"content":[],"structuredContent":{"results":[]}}`, want: "No results found."},
		{name: "text fallback", response: `{"content":[{"type":"text","text":"{\"results\":[]}"}]}`, want: "No results found."},
		{name: "malformed result", response: `{"content":[],"structuredContent":{"unexpected":true}}`, wantErr: "invalid results"},
		{name: "tool error", response: `{"content":[{"type":"text","text":"untrusted remote diagnostic"}],"isError":true}`, wantErr: "returned a tool error"},
		{name: "rate limited", status: http.StatusTooManyRequests, wantErr: "Too Many Requests"},
		{name: "auth required", status: http.StatusUnauthorized, wantErr: "Unauthorized"},
		{name: "redirect", status: http.StatusTemporaryRedirect, wantErr: "Temporary Redirect"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool := fixtureTool(t, func(w http.ResponseWriter, r *http.Request, msg rpcRequest) {
				if tc.status != 0 {
					w.Header().Set("Location", "/must-not-follow")
					w.WriteHeader(tc.status)
					return
				}
				writeRPC(t, w, msg, tc.response)
			})
			out, err := tool.Execute(t.Context(), `{"query":"public Go documentation"}`)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || strings.Contains(err.Error(), "untrusted remote diagnostic") || out != "" {
					t.Fatalf("output = %q, error = %v, want %q", out, err, tc.wantErr)
				}
			} else if err != nil || !strings.Contains(out, tc.want) {
				t.Fatalf("output = %q, error = %v, want %q", out, err, tc.want)
			}
		})
	}
}

func TestInvalidArguments(t *testing.T) {
	tool := fixtureTool(t, func(http.ResponseWriter, *http.Request, rpcRequest) {
		t.Error("invalid input reached the server")
	})
	for _, input := range []string{`{}`, `{"query":"  "}`, `{"query":7}`, `{"query":"q","num_results":"two"}`, `{"query":`} {
		if _, err := tool.Execute(t.Context(), input); err == nil {
			t.Errorf("expected an error for %q", input)
		}
	}
}

func TestRunPlainNumber(t *testing.T) {
	tool := fixtureTool(t, func(w http.ResponseWriter, _ *http.Request, msg rpcRequest) {
		assertSearch(t, msg, "2026")
		writeRPC(t, w, msg, sampleResults)
	})
	if _, err := tool.Run(t.Context(), "2026"); err != nil {
		t.Fatal(err)
	}
}

func TestLiveAnonymous(t *testing.T) {
	if os.Getenv("PARALLEL_MCP_LIVE_TEST") != "1" {
		t.Skip("set PARALLEL_MCP_LIVE_TEST=1 to call the public Parallel MCP endpoint")
	}
	var search interfaces.Tool = New()
	out, err := search.Execute(t.Context(), `{"query":"Go language documentation site:go.dev","num_results":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "URL: https://go.dev/") {
		t.Fatal("expected a citation from go.dev")
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "   URL: ") {
			t.Log(strings.TrimSpace(line))
		}
	}
}
