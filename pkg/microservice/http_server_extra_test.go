package microservice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// sseEvent is one parsed server-sent event.
type sseEvent struct {
	id    string
	event string
	data  string
}

// parseSSE splits an SSE body into events.
func parseSSE(body string) []sseEvent {
	var events []sseEvent
	for _, block := range strings.Split(strings.TrimSpace(body), "\n\n") {
		var e sseEvent
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "id: "):
				e.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				e.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				e.data = strings.TrimPrefix(line, "data: ")
			}
		}
		if e.event != "" {
			events = append(events, e)
		}
	}
	return events
}

// plainWriter is a ResponseWriter that cannot flush.
type plainWriter struct {
	header http.Header
	code   int
	body   strings.Builder
}

func (p *plainWriter) Header() http.Header         { return p.header }
func (p *plainWriter) Write(b []byte) (int, error) { return p.body.Write(b) }
func (p *plainWriter) WriteHeader(code int)        { p.code = code }

func TestHTTPServer_MethodAndInputValidation(t *testing.T) {
	s := NewHTTPServer(newCustomAgent(t, nil, nil), 0)
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		method   string
		body     string
		wantCode int
	}{
		{name: "health rejects POST", handler: s.handleHealth, method: http.MethodPost, wantCode: http.StatusMethodNotAllowed},
		{name: "metadata rejects POST", handler: s.handleMetadata, method: http.MethodPost, wantCode: http.StatusMethodNotAllowed},
		{name: "run rejects GET", handler: s.handleRun, method: http.MethodGet, wantCode: http.StatusMethodNotAllowed},
		{name: "run rejects bad JSON", handler: s.handleRun, method: http.MethodPost, body: "{", wantCode: http.StatusBadRequest},
		{name: "run requires input", handler: s.handleRun, method: http.MethodPost, body: "{}", wantCode: http.StatusBadRequest},
		{name: "stream rejects GET", handler: s.handleStream, method: http.MethodGet, wantCode: http.StatusMethodNotAllowed},
		{name: "stream rejects bad JSON", handler: s.handleStream, method: http.MethodPost, body: "nope", wantCode: http.StatusBadRequest},
		{name: "stream requires input", handler: s.handleStream, method: http.MethodPost, body: `{"input":""}`, wantCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tt.handler(w, httptest.NewRequest(tt.method, "/", strings.NewReader(tt.body)))
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

func TestHTTPServer_RunPassesScopeAndReportsErrors(t *testing.T) {
	var gotOrg, gotConv string
	a := newCustomAgent(t, func(ctx context.Context, input string, _ *agent.Agent) (string, error) {
		gotOrg, _ = multitenancy.GetOrgID(ctx)
		gotConv, _ = memory.GetConversationID(ctx)
		if input == "fail" {
			return "", errors.New("agent exploded")
		}
		return "ok:" + input, nil
	}, nil)
	s := NewHTTPServer(a, 0)

	w := httptest.NewRecorder()
	s.handleRun(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"input":"hi","org_id":"o1","conversation_id":"c1"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var ok map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &ok); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ok["output"] != "ok:hi" || ok["agent"] != "custom-agent" {
		t.Errorf("response = %v", ok)
	}
	if _, has := ok["execution_summary"]; !has {
		t.Error("execution_summary missing")
	}
	if gotOrg != "o1" || gotConv != "c1" {
		t.Errorf("scope = %q/%q", gotOrg, gotConv)
	}

	w = httptest.NewRecorder()
	s.handleRun(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"input":"fail"}`)))
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "agent exploded") {
		t.Errorf("error response = %d %s", w.Code, w.Body.String())
	}
}

func TestHTTPServer_StreamEventTypes(t *testing.T) {
	ts := time.Now()
	a := newCustomAgent(t, nil, replayStream(
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventContent, Content: "c", Timestamp: ts},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventThinking, ThinkingStep: "t", Timestamp: ts},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventToolCall, ToolCall: &interfaces.ToolCallEvent{Name: "x"}, Timestamp: ts},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventToolResult, ToolCall: &interfaces.ToolCallEvent{Name: "x", Result: "r"}, Timestamp: ts},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventError, Error: errors.New("e"), Timestamp: ts},
		interfaces.AgentStreamEvent{Type: "custom", Timestamp: ts},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventComplete, Timestamp: ts},
	))
	s := NewHTTPServer(a, 0)

	w := httptest.NewRecorder()
	s.handleStream(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"input":"go","org_id":"o","conversation_id":"c"}`)))

	events := parseSSE(w.Body.String())
	var names []string
	for _, e := range events {
		names = append(names, e.event)
	}
	want := "connected,content,thinking,tool_call,tool_result,error,content,complete,done"
	if strings.Join(names, ",") != want {
		t.Fatalf("events = %v, want %s", names, want)
	}
	// Agent events carry sequential IDs; connected and done do not.
	if events[0].id != "" || events[1].id != "1" || events[7].id != "7" || events[8].id != "" {
		t.Errorf("ids = %q %q %q %q", events[0].id, events[1].id, events[7].id, events[8].id)
	}
	var complete StreamEventData
	if err := json.Unmarshal([]byte(events[7].data), &complete); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !complete.IsFinal || complete.Timestamp == 0 {
		t.Errorf("complete = %+v", complete)
	}
	var errEvent StreamEventData
	if err := json.Unmarshal([]byte(events[5].data), &errEvent); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errEvent.Error != "e" {
		t.Errorf("error event = %+v", errEvent)
	}
	if w.Header().Get("X-Accel-Buffering") != "no" {
		t.Error("X-Accel-Buffering header missing")
	}
}

func TestHTTPServer_StreamStartError(t *testing.T) {
	a := newCustomAgent(t, nil, func(context.Context, string, *agent.Agent) (<-chan interfaces.AgentStreamEvent, error) {
		return nil, errors.New("no stream")
	})
	w := httptest.NewRecorder()
	NewHTTPServer(a, 0).handleStream(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"input":"go"}`)))

	events := parseSSE(w.Body.String())
	if len(events) != 1 || events[0].event != "error" || !strings.Contains(events[0].data, "no stream") {
		t.Errorf("events = %+v", events)
	}
}

func TestHTTPServer_StreamWithoutFlusher(t *testing.T) {
	w := &plainWriter{header: http.Header{}}
	NewHTTPServer(newCustomAgent(t, nil, nil), 0).handleStream(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"input":"go"}`)))
	if w.code != http.StatusInternalServerError || !strings.Contains(w.body.String(), "SSE not supported") {
		t.Errorf("got %d %q", w.code, w.body.String())
	}
}

func TestHTTPServer_CORSPassesThroughNonPreflight(t *testing.T) {
	s := NewHTTPServer(newCustomAgent(t, nil, nil), 0)
	called := false
	h := s.addCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !called || w.Code != http.StatusTeapot || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("called=%v code=%d", called, w.Code)
	}
}

func TestHTTPServer_StartAndStop(t *testing.T) {
	s := NewHTTPServer(newCustomAgent(t, nil, nil), freePort(t))
	if err := s.Stop(context.Background()); err != nil {
		t.Errorf("Stop before Start = %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- s.Start() }()
	// Stop is safe to call twice; this one covers a failure before the explicit Stop.
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	url := baseURL(s.port)
	waitForHTTP(t, url+"/health")

	resp, err := http.Get(url + "/api/v1/agent/metadata")
	if err != nil {
		t.Fatalf("GET metadata: %v", err)
	}
	var meta map[string]any
	err = json.NewDecoder(resp.Body).Decode(&meta)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if meta["name"] != "custom-agent" || meta["description"] != "an agent for tests" {
		t.Errorf("metadata = %v", meta)
	}

	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := <-done; !errors.Is(err, http.ErrServerClosed) {
		t.Errorf("Start returned %v, want ErrServerClosed", err)
	}
}
