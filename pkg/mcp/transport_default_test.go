package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// recordPaths records the request paths and Accept headers an MCP client sends,
// which is how the two HTTP transports are told apart from the server side:
// the streamable transport POSTs to the endpoint, the legacy SSE transport
// opens a GET stream.
type transportRecorder struct {
	mu      sync.Mutex
	methods []string
	accepts []string
}

func (r *transportRecorder) record(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.methods = append(r.methods, req.Method)
	r.accepts = append(r.accepts, req.Header.Get("Accept"))
}

func (r *transportRecorder) sawSSEStream() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, m := range r.methods {
		if m == http.MethodGet && strings.Contains(r.accepts[i], "text/event-stream") {
			return true
		}
	}
	return false
}

func (r *transportRecorder) sawPost() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.methods {
		if m == http.MethodPost {
			return true
		}
	}
	return false
}

// TestUnpinnedTransportPrefersStreamable guards the default.
//
// Streamable HTTP has shipped since pkg/mcp gained mcp.StreamableClientTransport,
// but the default branch selected SSE and logged "Server protocol type is not
// set, defaulting to SSE". Every caller who did not explicitly opt in therefore
// got the legacy transport, which is the opposite of the intent -- the code's
// own comment calls SSE legacy.
//
// The server here rejects everything, so no connection succeeds. What is under
// test is which transport is ATTEMPTED FIRST, which is observable from the
// request shape.
func TestUnpinnedTransportPrefersStreamable(t *testing.T) {
	rec := &transportRecorder{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec.record(req)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// ProtocolType deliberately left unset.
	_, _ = NewHTTPServer(ctx, HTTPServerConfig{
		BaseURL: server.URL,
		Logger:  logging.New(),
	})

	if !rec.sawPost() {
		t.Error("no POST reached the server: the unpinned default did not attempt " +
			"streamable HTTP first. It must not silently select the legacy SSE transport.")
	}
}

// TestUnpinnedTransportFallsBackToSSE guards the fallback that builder.go has
// always documented but that never existed:
//
//	"if omitted, default is SSE with fallback to streamable"
//
// There was no fallback anywhere in pkg/mcp. Now that streamable is the default,
// a server that only speaks SSE must still connect, or flipping the default
// would break those deployments.
func TestUnpinnedTransportFallsBackToSSE(t *testing.T) {
	rec := &transportRecorder{}

	// Reject POSTs (no streamable support) but accept the SSE GET stream.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec.record(req)
		if req.Method == http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-req.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = NewHTTPServer(ctx, HTTPServerConfig{
		BaseURL: server.URL,
		Logger:  logging.New(),
	})

	if !rec.sawPost() {
		t.Error("streamable HTTP was never attempted before falling back")
	}
	if !rec.sawSSEStream() {
		t.Error("no SSE stream was opened after streamable HTTP failed: an unpinned " +
			"caller talking to an SSE-only server must still connect, or flipping the " +
			"default breaks those deployments")
	}
}

// TestPinnedTransportIsNotProbed asserts an explicit choice is honoured without
// a fallback attempt. A caller who pinned a transport should get the error for
// the transport they asked for, not a confusing one from a transport they did
// not choose.
func TestPinnedTransportIsNotProbed(t *testing.T) {
	rec := &transportRecorder{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec.record(req)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = NewHTTPServer(ctx, HTTPServerConfig{
		BaseURL:      server.URL,
		ProtocolType: StreamableHTTP,
		Logger:       logging.New(),
	})

	if rec.sawSSEStream() {
		t.Error("an explicitly pinned streamable transport fell back to SSE; " +
			"a pinned choice must be honoured")
	}
}
