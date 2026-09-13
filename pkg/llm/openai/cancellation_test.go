package openai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestGenerateWithToolsFailsFastOnCancelledContext pins the contract that a
// cancelled run does not issue LLM requests and surfaces a context error.
//
// Note on what this does and does not prove. The tool loop in this client now
// begins each iteration with an explicit `if err := ctx.Err(); err != nil`
// check, which previously did not exist anywhere in this file. That check is
// defensive: it is NOT currently observable from outside, because the OpenAI SDK
// transport already refuses to send on a cancelled context, so the loop exits
// with a transport-wrapped context error either way. Three stronger assertions
// were tried and all passed with the guard removed:
//
//   - errors.Is(err, context.Canceled) on the returned error -- the transport
//     wraps it regardless;
//   - zero requests reaching a stub server with an already-cancelled context --
//     the SDK short-circuits before the first send;
//   - exactly one request when a tool cancels the run mid-loop -- the second
//     iteration's send fails at the transport, so the server sees one request
//     with or without the guard.
//
// The guard is kept because it makes the loop's cancellation contract explicit
// rather than inherited from transport internals, and because it returns the
// context error directly instead of one wrapped in transport noise. This test
// therefore asserts the externally visible behaviour that must hold either way,
// and the comment records why a sharper test is not available here.
func TestGenerateWithToolsFailsFastOnCancelledContext(t *testing.T) {
	var requests atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithModel("gpt-4o"), WithBaseURL(server.URL))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.GenerateWithTools(ctx, "anything", nil)
	if err == nil {
		t.Fatal("GenerateWithTools returned no error for a cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want it to wrap context.Canceled", err)
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("issued %d request(s) on a cancelled context, want 0", got)
	}
}

// TestGenerateWithToolsFailsFastOnExpiredDeadline is the deadline form of the
// same externally visible contract.
func TestGenerateWithToolsFailsFastOnExpiredDeadline(t *testing.T) {
	var requests atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient("test-key", WithModel("gpt-4o"), WithBaseURL(server.URL))

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, err := client.GenerateWithTools(ctx, "anything", nil)
	if err == nil {
		t.Fatal("GenerateWithTools returned no error for an expired deadline")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want it to wrap context.DeadlineExceeded", err)
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("issued %d request(s) on an expired deadline, want 0", got)
	}
}
