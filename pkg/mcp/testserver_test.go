package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// stdioServerEnv makes the test binary act as an MCP server over stdio, so
// stdio tests can start a real subprocess without any external program.
const stdioServerEnv = "AGENT_SDK_GO_MCP_TEST_STDIO_SERVER"

func TestMain(m *testing.M) {
	if os.Getenv(stdioServerEnv) == "1" {
		if err := newTestMCPServer().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// newTestMCPServer builds a server with two tools, three resources and two prompts.
func newTestMCPServer() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "test-server", Title: "Test Server", Version: "1.2.3"}, nil)

	s.AddTool(&mcp.Tool{
		Name:        "echo",
		Description: "Echo text back",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string", "description": "text to echo"},
				"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []any{"text"},
		},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var in struct {
			Text string `json:"text"`
		}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &in); err != nil {
				return nil, err
			}
		}
		res := &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + in.Text}},
			Meta:    mcp.Meta{"structuredContent": map[string]any{"text": in.Text}},
		}
		// Send the trace parent back so tests can see what the client attached.
		if tp, ok := req.Params.Meta[TraceParentAttribute]; ok {
			res.Meta[TraceParentAttribute] = tp
		}
		return res, nil
	})

	s.AddTool(&mcp.Tool{
		Name:        "fail",
		Description: "Always fails",
		InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "bad input"}}}, nil
	})

	textResource := func(uri, mime, text string) mcp.ResourceHandler {
		return func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: mime, Text: text}}}, nil
		}
	}
	s.AddResource(&mcp.Resource{
		URI:         "file:///notes.md",
		Name:        "notes",
		Description: "Project notes",
		MIMEType:    "text/markdown",
		Annotations: &mcp.Annotations{
			Audience:     []mcp.Role{"user", "assistant"},
			LastModified: "2025-01-01T00:00:00Z",
			Priority:     0.5,
		},
	}, textResource("file:///notes.md", "text/markdown", "# notes"))
	s.AddResource(&mcp.Resource{URI: "file:///plain.txt", Name: "plain"}, textResource("file:///plain.txt", "", "plain text"))
	s.AddResource(&mcp.Resource{URI: "file:///logo.bin", Name: "logo"},
		func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "file:///logo.bin", Blob: []byte{1, 2, 3}}}}, nil
		})

	s.AddPrompt(&mcp.Prompt{
		Name:        "greet",
		Description: "Greets someone",
		Arguments: []*mcp.PromptArgument{
			{Name: "name", Description: "who to greet", Required: true},
			{Name: "times"},
		},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		text := fmt.Sprintf("Hello, %s x%s", req.Params.Arguments["name"], req.Params.Arguments["times"])
		return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}}}, nil
	})
	s.AddPrompt(&mcp.Prompt{Name: "pair"}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: "look"}},
			{Role: "assistant", Content: &mcp.ImageContent{Data: []byte("img"), MIMEType: "image/png"}},
		}}, nil
	})

	return s
}

// connectServer starts the test server in process and returns the server end
// and the client transport to hand to the code under test.
func connectServer(t *testing.T) mcp.Transport {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	session, err := newTestMCPServer().Connect(context.Background(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return clientTransport
}

// connectInMemory returns an MCPServerImpl (or a retry wrapper) connected to
// an in-process test server.
func connectInMemory(t *testing.T, retry *RetryConfig) interfaces.MCPServer {
	t.Helper()
	server, err := NewCustomTransportServerWithRetry(context.Background(), CustomTransportServerConfig{
		Transport:     connectServer(t),
		Logger:        logging.New(),
		TransportType: "InMemory",
	}, retry)
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })
	return server
}

// headerRecorder keeps the Authorization headers an HTTP server receives.
type headerRecorder struct {
	mu   sync.Mutex
	auth []string
}

func (h *headerRecorder) values() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.auth...)
}

// newStreamableTestServer serves the test MCP server over streamable HTTP.
func newStreamableTestServer(t *testing.T) (*httptest.Server, *headerRecorder) {
	t.Helper()
	rec := &headerRecorder{}
	server := newTestMCPServer()
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.auth = append(rec.auth, r.Header.Get("Authorization"))
		rec.mu.Unlock()
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts, rec
}

// stubServer is an interfaces.MCPServer whose behavior is set per test.
type stubServer struct {
	tools    []interfaces.MCPTool
	listErr  error
	callResp *interfaces.MCPToolResponse
	callErr  error

	mu    sync.Mutex
	calls []any
}

func (s *stubServer) Initialize(context.Context) error { return nil }
func (s *stubServer) ListTools(context.Context) ([]interfaces.MCPTool, error) {
	return s.tools, s.listErr
}
func (s *stubServer) CallTool(_ context.Context, _ string, args interface{}) (*interfaces.MCPToolResponse, error) {
	s.mu.Lock()
	s.calls = append(s.calls, args)
	s.mu.Unlock()
	return s.callResp, s.callErr
}
func (s *stubServer) ListResources(context.Context) ([]interfaces.MCPResource, error) {
	return nil, nil
}
func (s *stubServer) GetResource(context.Context, string) (*interfaces.MCPResourceContent, error) {
	return nil, nil
}
func (s *stubServer) WatchResource(context.Context, string) (<-chan interfaces.MCPResourceUpdate, error) {
	return nil, nil
}
func (s *stubServer) ListPrompts(context.Context) ([]interfaces.MCPPrompt, error) { return nil, nil }
func (s *stubServer) GetPrompt(context.Context, string, map[string]interface{}) (*interfaces.MCPPromptResult, error) {
	return nil, nil
}
func (s *stubServer) CreateMessage(context.Context, *interfaces.MCPSamplingRequest) (*interfaces.MCPSamplingResponse, error) {
	return nil, nil
}
func (s *stubServer) GetServerInfo() (*interfaces.MCPServerInfo, error) { return nil, nil }
func (s *stubServer) GetCapabilities() (*interfaces.MCPServerCapabilities, error) {
	return nil, nil
}
func (s *stubServer) Close() error { return nil }

func (s *stubServer) callArgs() []any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]any(nil), s.calls...)
}
