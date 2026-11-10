package mcp

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestMCPServerImplMetadata(t *testing.T) {
	server := connectInMemory(t, nil)

	info, err := server.GetServerInfo()
	require.NoError(t, err)
	assert.Equal(t, &interfaces.MCPServerInfo{Name: "test-server", Title: "Test Server", Version: "1.2.3"}, info)

	caps, err := server.GetCapabilities()
	require.NoError(t, err)
	assert.NotNil(t, caps.Tools)
	assert.NotNil(t, caps.Resources)
	assert.NotNil(t, caps.Prompts)

	assert.NoError(t, server.Initialize(t.Context()))
}

func TestNewMCPServer(t *testing.T) {
	server, err := NewMCPServer(t.Context(), connectServer(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	info, err := server.GetServerInfo()
	require.NoError(t, err)
	assert.Equal(t, "test-server", info.Name)

	tools, err := server.ListTools(t.Context())
	require.NoError(t, err)
	assert.Len(t, tools, 2)
}

func TestNewMCPServerConnectError(t *testing.T) {
	server, err := NewMCPServer(t.Context(), failingTransport{})
	assert.Error(t, err)
	assert.Nil(t, server)
}

// failingTransport is an mcp.Transport that never connects.
type failingTransport struct{}

func (failingTransport) Connect(context.Context) (mcp.Connection, error) {
	return nil, errors.New("connection refused")
}

func TestConvertMCPCapabilities(t *testing.T) {
	assert.Nil(t, convertMCPCapabilities(nil))
	assert.Equal(t, &interfaces.MCPServerCapabilities{}, convertMCPCapabilities(&mcp.ServerCapabilities{}))
	got := convertMCPCapabilities(&mcp.ServerCapabilities{
		Tools:     &mcp.ToolCapabilities{ListChanged: true},
		Resources: &mcp.ResourceCapabilities{Subscribe: true},
		Prompts:   &mcp.PromptCapabilities{ListChanged: true},
	})
	assert.Equal(t, &interfaces.MCPServerCapabilities{
		Tools:     &interfaces.MCPToolCapabilities{ListChanged: true},
		Resources: &interfaces.MCPResourceCapabilities{Subscribe: true},
		Prompts:   &interfaces.MCPPromptCapabilities{ListChanged: true},
	}, got)
}

func TestMCPServerImplListTools(t *testing.T) {
	server := connectInMemory(t, nil)

	tools, err := server.ListTools(t.Context())
	require.NoError(t, err)
	require.Len(t, tools, 2)

	byName := map[string]interfaces.MCPTool{}
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	echo := byName["echo"]
	assert.Equal(t, "Echo text back", echo.Description)
	assert.Equal(t, "present", echo.Metadata["annotations"])
	assert.NotNil(t, echo.Schema)
	assert.Empty(t, byName["fail"].Metadata)
}

func TestMCPServerImplCallTool(t *testing.T) {
	server := connectInMemory(t, nil)

	resp, err := server.CallTool(t.Context(), "echo", map[string]any{"text": "hi"})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	content, ok := resp.Content.([]mcp.Content)
	require.True(t, ok, "content is %T", resp.Content)
	require.Len(t, content, 1)
	assert.Equal(t, "echo: hi", content[0].(*mcp.TextContent).Text)
	assert.Equal(t, map[string]any{"text": "hi"}, resp.StructuredContent)
	assert.NotContains(t, resp.Metadata, TraceParentAttribute)

	resp, err = server.CallTool(t.Context(), "fail", nil)
	require.NoError(t, err)
	assert.True(t, resp.IsError)

	_, err = server.CallTool(t.Context(), "no-such-tool", nil)
	var mcpErr *MCPError
	require.ErrorAs(t, err, &mcpErr)
	assert.Equal(t, "CallTool", mcpErr.Operation)
}

func TestMCPServerImplCallToolSendsTraceParent(t *testing.T) {
	server := connectInMemory(t, nil)

	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	ctx := trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))

	resp, err := server.CallTool(ctx, "echo", map[string]any{"text": "traced"})
	require.NoError(t, err)
	assert.Equal(t, "00-"+traceID.String()+"-"+spanID.String()+"-01", resp.Metadata[TraceParentAttribute])
}

func TestMCPServerImplResources(t *testing.T) {
	server := connectInMemory(t, nil)

	resources, err := server.ListResources(t.Context())
	require.NoError(t, err)
	byURI := map[string]interfaces.MCPResource{}
	for _, r := range resources {
		byURI[r.URI] = r
	}
	require.Len(t, byURI, 3)
	notes := byURI["file:///notes.md"]
	assert.Equal(t, "notes", notes.Name)
	assert.Equal(t, "Project notes", notes.Description)
	assert.Equal(t, "text/markdown", notes.MimeType)
	assert.Equal(t, map[string]string{
		"audience":     "user,assistant",
		"lastModified": "2025-01-01T00:00:00Z",
		"priority":     "0.50",
	}, notes.Metadata)
	assert.Empty(t, byURI["file:///plain.txt"].Metadata)

	tests := []struct {
		uri  string
		want *interfaces.MCPResourceContent
	}{
		{"file:///notes.md", &interfaces.MCPResourceContent{URI: "file:///notes.md", MimeType: "text/markdown", Text: "# notes", Metadata: map[string]string{}}},
		{"file:///plain.txt", &interfaces.MCPResourceContent{URI: "file:///plain.txt", MimeType: "text/plain", Text: "plain text", Metadata: map[string]string{}}},
		{"file:///logo.bin", &interfaces.MCPResourceContent{URI: "file:///logo.bin", MimeType: "application/octet-stream", Blob: []byte{1, 2, 3}, Metadata: map[string]string{}}},
	}
	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got, err := server.GetResource(t.Context(), tt.uri)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err = server.GetResource(t.Context(), "file:///missing")
	var mcpErr *MCPError
	require.ErrorAs(t, err, &mcpErr)
	assert.Equal(t, "file:///missing", mcpErr.Metadata["uri"])
}

func TestMCPServerImplPrompts(t *testing.T) {
	server := connectInMemory(t, nil)

	prompts, err := server.ListPrompts(t.Context())
	require.NoError(t, err)
	byName := map[string]interfaces.MCPPrompt{}
	for _, p := range prompts {
		byName[p.Name] = p
	}
	require.Len(t, byName, 2)
	assert.Equal(t, "Greets someone", byName["greet"].Description)
	assert.Equal(t, []interfaces.MCPPromptArgument{
		{Name: "name", Description: "who to greet", Required: true},
		{Name: "times"},
	}, byName["greet"].Arguments)

	// Non-string variables are formatted before they are sent.
	vars := map[string]interface{}{"name": "Ann", "times": 3}
	greet, err := server.GetPrompt(t.Context(), "greet", vars)
	require.NoError(t, err)
	assert.Equal(t, "Hello, Ann x3", greet.Prompt)
	assert.Equal(t, vars, greet.Variables)
	assert.Equal(t, []interfaces.MCPPromptMessage{{Role: "user", Content: "Hello, Ann x3"}}, greet.Messages)

	// With several messages the single Prompt field stays empty.
	pair, err := server.GetPrompt(t.Context(), "pair", nil)
	require.NoError(t, err)
	assert.Empty(t, pair.Prompt)
	require.Len(t, pair.Messages, 2)
	assert.Equal(t, "look", pair.Messages[0].Content)
	assert.Equal(t, "assistant", pair.Messages[1].Role)
	assert.NotEmpty(t, pair.Messages[1].Content)

	_, err = server.GetPrompt(t.Context(), "missing", nil)
	assert.Error(t, err)
}

func TestMCPServerImplCreateMessageIsNotAvailable(t *testing.T) {
	server := connectInMemory(t, nil)

	resp, err := server.CreateMessage(t.Context(), &interfaces.MCPSamplingRequest{
		Messages: []interfaces.MCPMessage{{Role: "user", Content: interfaces.MCPContent{Type: "text", Text: "hi"}}},
	})
	assert.Nil(t, resp)
	assert.ErrorContains(t, err, "MCP Sampling is not yet available")
}

func TestMCPServerImplWatchResourceStopsOnCancel(t *testing.T) {
	server := connectInMemory(t, nil)
	ctx, cancel := context.WithCancel(t.Context())

	updates, err := server.WatchResource(ctx, "file:///notes.md")
	require.NoError(t, err)
	cancel()

	select {
	case _, open := <-updates:
		assert.False(t, open, "no update is expected before the first poll")
	case <-time.After(2 * time.Second):
		t.Fatal("watch channel was not closed after cancel")
	}
}

func TestMCPServerImplErrorsAfterClose(t *testing.T) {
	server := connectInMemory(t, nil)
	require.NoError(t, server.Close())

	ctx := t.Context()
	_, err := server.ListTools(ctx)
	assert.Error(t, err)
	_, err = server.ListResources(ctx)
	assert.Error(t, err)
	_, err = server.GetResource(ctx, "file:///notes.md")
	assert.Error(t, err)
	_, err = server.ListPrompts(ctx)
	assert.Error(t, err)
	_, err = server.GetPrompt(ctx, "greet", nil)
	assert.Error(t, err)
	_, err = server.CallTool(ctx, "echo", nil)
	assert.Error(t, err)
}

func TestNewCustomTransportServerWithRetryWrapsServer(t *testing.T) {
	server := connectInMemory(t, &RetryConfig{MaxAttempts: 1})
	_, ok := server.(*RetryableServer)
	require.True(t, ok, "got %T", server)

	// The wrapper passes every call through to the real server.
	ctx := t.Context()
	resources, err := server.ListResources(ctx)
	require.NoError(t, err)
	assert.Len(t, resources, 3)
	content, err := server.GetResource(ctx, "file:///plain.txt")
	require.NoError(t, err)
	assert.Equal(t, "plain text", content.Text)
	prompts, err := server.ListPrompts(ctx)
	require.NoError(t, err)
	assert.Len(t, prompts, 2)
	prompt, err := server.GetPrompt(ctx, "greet", map[string]interface{}{"name": "Bo", "times": "2"})
	require.NoError(t, err)
	assert.Equal(t, "Hello, Bo x2", prompt.Prompt)
	_, err = server.CreateMessage(ctx, &interfaces.MCPSamplingRequest{})
	assert.Error(t, err)
	info, err := server.GetServerInfo()
	require.NoError(t, err)
	assert.Equal(t, "test-server", info.Name)
	caps, err := server.GetCapabilities()
	require.NoError(t, err)
	assert.NotNil(t, caps.Tools)
}

func TestNewCustomTransportServerConnectError(t *testing.T) {
	server, err := NewCustomTransportServer(t.Context(), CustomTransportServerConfig{
		Transport:     failingTransport{},
		Logger:        logging.New(),
		TransportType: "Broken",
	})
	assert.Nil(t, server)
	var mcpErr *MCPError
	require.ErrorAs(t, err, &mcpErr)
	assert.Equal(t, "broken-server", mcpErr.ServerName)
	assert.Equal(t, "Broken", mcpErr.ServerType)
}

func TestNewHTTPServerStreamableWithToken(t *testing.T) {
	ts, rec := newStreamableTestServer(t)

	server, err := NewHTTPServer(t.Context(), HTTPServerConfig{
		BaseURL:      ts.URL,
		Token:        "secret-token",
		ProtocolType: StreamableHTTP,
		Logger:       logging.New(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	resp, err := server.CallTool(t.Context(), "echo", map[string]any{"text": "over http"})
	require.NoError(t, err)
	assert.Equal(t, "echo: over http", resp.Content.([]mcp.Content)[0].(*mcp.TextContent).Text)

	auth := rec.values()
	require.NotEmpty(t, auth)
	for _, a := range auth {
		assert.Equal(t, "Bearer secret-token", a)
	}
}

func TestNewHTTPServerPinnedTransportError(t *testing.T) {
	ts, _ := newStreamableTestServer(t)
	ts.Close()

	server, err := NewHTTPServer(t.Context(), HTTPServerConfig{
		BaseURL:      ts.URL,
		ProtocolType: StreamableHTTP,
		Logger:       logging.New(),
	})
	assert.Nil(t, server)
	assert.Error(t, err)
}

func TestNewStdioServerValidation(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr string
	}{
		{"empty command", "", "command cannot be empty"},
		{"unknown command", "definitely-not-a-real-command-xyz", "invalid command"},
		{"directory", t.TempDir(), "invalid command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewStdioServer(t.Context(), StdioServerConfig{Command: tt.command, Logger: logging.New()})
			assert.Nil(t, server)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestNewStdioServerRunsSubprocess(t *testing.T) {
	server, err := NewStdioServer(t.Context(), StdioServerConfig{
		Command: os.Args[0],
		Env: []string{
			stdioServerEnv + "=1",
			"API_KEY=0123456789abcdef",
			"SHORT_TOKEN=abc",
			"NO_EQUALS_SIGN",
			"LONG_VALUE=" + strings.Repeat("x", 70),
		},
		Logger: logging.New(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	resp, err := server.CallTool(t.Context(), "echo", map[string]any{"text": "from subprocess"})
	require.NoError(t, err)
	assert.Equal(t, "echo: from subprocess", resp.Content.([]mcp.Content)[0].(*mcp.TextContent).Text)
}

func TestNewStdioServerSubprocessExits(t *testing.T) {
	// "true" exits at once without speaking MCP, so the handshake fails.
	server, err := NewStdioServer(t.Context(), StdioServerConfig{Command: "true", Logger: logging.New()})
	assert.Nil(t, server)
	assert.Error(t, err)
}

func TestSyncBuffer(t *testing.T) {
	var sb syncBuffer
	n, err := sb.Write([]byte("abc"))
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	_, _ = sb.Write([]byte("def"))
	assert.Equal(t, "abcdef", sb.String())
}
