package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// reply is one scripted answer from fakeAPI.
type reply struct {
	status int      // HTTP status; zero means 200
	body   string   // JSON body for unary calls and error replies
	chunks []string // JSON chunks sent as server-sent events for streaming calls
}

// recordedRequest is one request fakeAPI received.
type recordedRequest struct {
	path string
	body map[string]any
}

// fakeAPI is an httptest server that answers Gemini API calls with scripted
// replies, in order, and records every request it receives.
type fakeAPI struct {
	t       *testing.T
	server  *httptest.Server
	mu      sync.Mutex
	replies []reply
	reqs    []recordedRequest
}

func newFakeAPI(t *testing.T, replies ...reply) *fakeAPI {
	t.Helper()
	f := &fakeAPI{t: t, replies: replies}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeAPI) handle(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		f.t.Errorf("decode request body: %v", err)
	}

	f.mu.Lock()
	f.reqs = append(f.reqs, recordedRequest{path: r.URL.Path, body: body})
	if len(f.replies) == 0 {
		f.mu.Unlock()
		f.t.Errorf("unexpected request to %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"code":500,"message":"no scripted reply"}}`)
		return
	}
	rep := f.replies[0]
	f.replies = f.replies[1:]
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if rep.status != 0 && rep.status != http.StatusOK {
		w.WriteHeader(rep.status)
		_, _ = io.WriteString(w, rep.body)
		return
	}
	if rep.chunks != nil {
		for _, chunk := range rep.chunks {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		return
	}
	_, _ = io.WriteString(w, rep.body)
}

// requests returns a copy of the requests received so far.
func (f *fakeAPI) requests() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reqs)
}

// client builds a GeminiClient whose genai client talks to the fake server.
func (f *fakeAPI) client(t *testing.T, opts ...Option) *GeminiClient {
	t.Helper()
	gc, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		Backend:     genai.BackendGeminiAPI,
		APIKey:      "test-key",
		HTTPOptions: genai.HTTPOptions{BaseURL: f.server.URL},
	})
	require.NoError(t, err)
	c, err := NewClient(context.Background(), append([]Option{WithClient(gc)}, opts...)...)
	require.NoError(t, err)
	return c
}

// candidateJSON builds a response body with one candidate holding parts.
func candidateJSON(parts ...map[string]any) string {
	return responseJSON(map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"role": "model", "parts": parts},
		}},
	})
}

func responseJSON(v map[string]any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func okReply(parts ...map[string]any) reply { return reply{body: candidateJSON(parts...)} }

func streamReply(chunks ...string) reply { return reply{chunks: chunks} }

func errReply(status int, message string) reply {
	return reply{
		status: status,
		body:   responseJSON(map[string]any{"error": map[string]any{"code": status, "message": message, "status": "ERROR"}}),
	}
}

func textPart(s string) map[string]any { return map[string]any{"text": s} }

func thoughtPart(s string) map[string]any { return map[string]any{"text": s, "thought": true} }

func callPart(name string, args map[string]any) map[string]any {
	return map[string]any{"functionCall": map[string]any{"name": name, "args": args}}
}

func imagePart(mime string, data []byte) map[string]any {
	// The JSON encoder turns []byte into base64, which is what the API sends.
	return map[string]any{"inlineData": map[string]any{"mimeType": mime, "data": data}}
}

// dig walks nested maps and slices in a decoded JSON body. Keys are map keys
// for objects and ints for arrays.
func dig(v any, path ...any) any {
	for _, p := range path {
		switch key := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}
			v = m[key]
		case int:
			s, ok := v.([]any)
			if !ok || key >= len(s) {
				return nil
			}
			v = s[key]
		}
	}
	return v
}

// scriptedTool is a tool that returns a fixed result or error and records its arguments.
type scriptedTool struct {
	name   string
	result string
	err    error
	mu     sync.Mutex
	args   []string
}

func (s *scriptedTool) Name() string        { return s.name }
func (s *scriptedTool) DisplayName() string { return s.name }
func (s *scriptedTool) Description() string { return "tool " + s.name }
func (s *scriptedTool) Internal() bool      { return false }
func (s *scriptedTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"q": {Type: "string", Description: "query", Required: true},
	}
}

func (s *scriptedTool) Run(ctx context.Context, input string) (string, error) {
	return s.Execute(ctx, input)
}

func (s *scriptedTool) Execute(_ context.Context, args string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.args = append(s.args, args)
	return s.result, s.err
}

func (s *scriptedTool) calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.args)
}
