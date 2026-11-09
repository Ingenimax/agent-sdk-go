package mcp

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// cacheKey is the key getOrCreateServer stores a server under.
func cacheKey(c LazyMCPServerConfig) string {
	return fmt.Sprintf("%s:%s:%v:%s", c.Type, c.Name, c.Command, c.CustomTransportType)
}

// forgetServer closes and removes a server from the global cache when the test ends.
func forgetServer(t *testing.T, c LazyMCPServerConfig) {
	t.Helper()
	t.Cleanup(func() {
		key := cacheKey(c)
		globalServerCache.mu.Lock()
		server := globalServerCache.servers[key]
		delete(globalServerCache.servers, key)
		delete(globalServerCache.serverMetadata, key)
		globalServerCache.mu.Unlock()
		if server != nil {
			_ = server.Close()
		}
	})
}

// putServer places a server in the global cache so lazy tools use it.
func putServer(t *testing.T, c LazyMCPServerConfig, server interfaces.MCPServer) {
	t.Helper()
	globalServerCache.mu.Lock()
	globalServerCache.servers[cacheKey(c)] = server
	globalServerCache.mu.Unlock()
	forgetServer(t, c)
}

func customConfig(t *testing.T) LazyMCPServerConfig {
	t.Helper()
	c := LazyMCPServerConfig{
		Name:                t.Name(),
		Type:                "custom",
		CustomMCPTransport:  connectServer(t),
		CustomTransportType: "InMemory",
	}
	forgetServer(t, c)
	return c
}

func TestLazyMCPToolEndToEnd(t *testing.T) {
	config := customConfig(t)
	echo := NewLazyMCPTool("echo", "", nil, config)

	assert.Equal(t, "echo", echo.Name())
	assert.Equal(t, "echo", echo.(*LazyMCPTool).DisplayName())
	assert.False(t, echo.(*LazyMCPTool).Internal())
	assert.Equal(t, "echo (MCP tool)", echo.Description())

	// Parameters come from the schema the server advertises.
	params := echo.Parameters()
	assert.Equal(t, interfaces.ParameterSpec{Type: "string", Description: "text to echo", Required: true}, params["text"])
	assert.Equal(t, "array", params["tags"].Type)
	assert.Equal(t, &interfaces.ParameterSpec{Type: "string"}, params["tags"].Items)

	got, err := echo.Run(t.Context(), `{"text":"lazy"}`)
	require.NoError(t, err)
	assert.Equal(t, "echo: lazy", got)

	got, err = echo.Execute(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, "echo: ", got)

	_, err = echo.Run(t.Context(), "not json")
	assert.ErrorContains(t, err, "failed to parse input as JSON")

	// Tools on the same server share one connection.
	first, err := GetOrCreateServerFromCache(t.Context(), config)
	require.NoError(t, err)
	second, err := GetOrCreateServerFromCache(t.Context(), config)
	require.NoError(t, err)
	assert.Same(t, first, second)

	fail := NewLazyMCPTool("fail", "fails", nil, config)
	_, err = fail.Run(t.Context(), "{}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("MCP tool error from server '%s'", config.Name))
	assert.Contains(t, err.Error(), "bad input")

	// A tool the server does not list has no parameters and fails when run.
	ghost := NewLazyMCPTool("ghost", "", nil, config)
	assert.Empty(t, ghost.Parameters())
	_, err = ghost.Run(t.Context(), "{}")
	assert.ErrorContains(t, err, "MCP server call failed")
}

func TestLazyMCPToolOverHTTP(t *testing.T) {
	ts, rec := newStreamableTestServer(t)
	config := LazyMCPServerConfig{Name: t.Name(), Type: "http", URL: ts.URL, Token: "tok", HttpTransportMode: "streamable"}
	forgetServer(t, config)

	got, err := NewLazyMCPTool("echo", "", nil, config).Run(t.Context(), `{"text":"http"}`)
	require.NoError(t, err)
	assert.Equal(t, "echo: http", got)
	assert.Contains(t, rec.values(), "Bearer tok")
}

func TestLazyMCPToolOverStdio(t *testing.T) {
	config := LazyMCPServerConfig{
		Name:    t.Name(),
		Type:    "stdio",
		Command: os.Args[0],
		Env:     []string{stdioServerEnv + "=1", "EMPTY=", "LONG=" + strings.Repeat("y", 60), "BARE"},
	}
	forgetServer(t, config)

	got, err := NewLazyMCPTool("echo", "", nil, config).Run(t.Context(), `{"text":"stdio"}`)
	require.NoError(t, err)
	assert.Equal(t, "echo: stdio", got)
}

func TestLazyMCPToolServerErrors(t *testing.T) {
	tests := []struct {
		name    string
		config  LazyMCPServerConfig
		wantErr string
	}{
		{"unsupported type", LazyMCPServerConfig{Type: "carrier-pigeon"}, "unsupported MCP server type"},
		{"custom without transport", LazyMCPServerConfig{Type: "custom", CustomTransportType: "x"}, "custom MCP transport is required"},
		{"custom without type", LazyMCPServerConfig{Type: "custom", CustomMCPTransport: failingTransport{}}, "custom transport type is required"},
		{"connect failure", LazyMCPServerConfig{Type: "custom", CustomMCPTransport: failingTransport{}, CustomTransportType: "x"}, "failed to initialize MCP server"},
		{"bad stdio command", LazyMCPServerConfig{Type: "stdio", Command: "definitely-not-a-real-command-xyz"}, "failed to initialize MCP server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.Name = t.Name()
			tool := NewLazyMCPTool("x", "", nil, tt.config)

			_, err := tool.Run(t.Context(), "{}")
			assert.ErrorContains(t, err, tt.wantErr)
			// Schema discovery fails the same way and leaves no parameters.
			assert.Empty(t, tool.Parameters())
		})
	}
}

func TestLazyMCPToolRunErrorContent(t *testing.T) {
	tests := []struct {
		name    string
		content interface{}
		want    string
	}{
		{"string", "plain failure", "plain failure"},
		{"bytes", []byte("byte failure"), "byte failure"},
		{"map with message", map[string]interface{}{"message": "map failure"}, "map failure"},
		{"map without message", map[string]interface{}{"code": 7}, `{"code":7}`},
		{"map that cannot marshal", map[string]interface{}{"ch": make(chan int)}, "map[ch:"},
		{"array", []interface{}{"a", 1}, `["a",1]`},
		{"array that cannot marshal", []interface{}{make(chan int)}, "[0x"},
		{"other", 42, "42"},
		{"other that cannot marshal", make(chan int), "0x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := LazyMCPServerConfig{Name: t.Name(), Type: "stub"}
			putServer(t, config, &stubServer{callResp: &interfaces.MCPToolResponse{IsError: true, Content: tt.content}})

			_, err := NewLazyMCPTool("t", "", nil, config).Run(t.Context(), `{"a":1}`)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "MCP tool error from server")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestLazyMCPToolRunPassesArguments(t *testing.T) {
	config := LazyMCPServerConfig{Name: t.Name(), Type: "stub", Env: []string{"SHORT=1", "LONG=" + strings.Repeat("z", 60)}}
	stub := &stubServer{callResp: &interfaces.MCPToolResponse{Content: map[string]interface{}{"text": "ok"}}}
	putServer(t, config, stub)

	got, err := NewLazyMCPTool("t", "", nil, config).Run(t.Context(), `{"a":1}`)
	require.NoError(t, err)
	assert.Equal(t, "ok", got)
	assert.Equal(t, []any{map[string]interface{}{"a": float64(1)}}, stub.callArgs())

	stub.callErr = errors.New("transport down")
	_, err = NewLazyMCPTool("t", "", nil, config).Run(t.Context(), "{}")
	assert.ErrorContains(t, err, "MCP server call failed: transport down")
}

func TestLazyMCPToolDiscoverSchemaListError(t *testing.T) {
	config := LazyMCPServerConfig{Name: t.Name(), Type: "stub"}
	putServer(t, config, &stubServer{listErr: errors.New("no list")})

	tool := NewLazyMCPTool("t", "", nil, config).(*LazyMCPTool)
	assert.ErrorContains(t, tool.discoverSchema(t.Context()), "failed to list tools")
	assert.Empty(t, tool.Parameters())
}

// lazyWithSchema returns a lazy tool whose schema is already loaded.
func lazyWithSchema(schema interface{}) *LazyMCPTool {
	return &LazyMCPTool{name: "t", schema: schema, schemaLoaded: true, logger: logging.New()}
}

func TestLazyMCPToolParametersFromSchemaShapes(t *testing.T) {
	objectSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"plain":    map[string]interface{}{"type": "string", "description": "a string"},
			"choice":   map[string]interface{}{"type": "string", "enum": []interface{}{"a", "b"}, "default": "a"},
			"nullable": map[string]interface{}{"anyOf": []interface{}{map[string]interface{}{"type": "null"}, map[string]interface{}{"type": "integer"}}},
			"onlyNull": map[string]interface{}{"anyOf": []interface{}{map[string]interface{}{"type": "null"}}},
			"untyped":  map[string]interface{}{"description": "no type"},
			"list":     map[string]interface{}{"type": []interface{}{"array", "null"}, "items": map[string]interface{}{"type": "number", "enum": []interface{}{1.0, 2.0}}},
			"bareList": map[string]interface{}{"type": "array"},
			"skipped":  "not an object",
		},
		"required": []interface{}{"plain", "list"},
	}

	t.Run("map schema", func(t *testing.T) {
		tool := lazyWithSchema(objectSchema)
		params := tool.Parameters()

		assert.Len(t, params, 7)
		assert.Equal(t, interfaces.ParameterSpec{Type: "string", Description: "a string", Required: true}, params["plain"])
		assert.Equal(t, []interface{}{"a", "b"}, params["choice"].Enum)
		assert.Equal(t, "a", params["choice"].Default)
		assert.Equal(t, "integer", params["nullable"].Type)
		assert.Equal(t, "string", params["onlyNull"].Type)
		assert.Equal(t, "string", params["untyped"].Type)
		assert.Equal(t, "no type", params["untyped"].Description)
		assert.True(t, params["list"].Required)
		assert.Equal(t, &interfaces.ParameterSpec{Type: "number", Enum: []interface{}{1.0, 2.0}}, params["list"].Items)
		assert.Equal(t, &interfaces.ParameterSpec{Type: "string"}, params["bareList"].Items)
	})

	t.Run("JSON string schema", func(t *testing.T) {
		tool := lazyWithSchema(`{"properties":{"q":{"type":"string"}},"required":["q"]}`)
		assert.Equal(t, map[string]interfaces.ParameterSpec{
			"q": {Type: "string", Description: "<nil>", Required: true},
		}, tool.Parameters())
	})

	t.Run("jsonschema struct", func(t *testing.T) {
		schema := &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{"n": {Type: "integer", Description: "count"}},
		}
		tool := lazyWithSchema(schema)
		assert.Equal(t, interfaces.ParameterSpec{Type: "integer", Description: "count"}, tool.Parameters()["n"])
	})

	for name, schema := range map[string]interface{}{
		"invalid JSON string":   "{not json",
		"value that is no map":  []int{1, 2},
		"value that won't JSON": make(chan int),
		"nil schema":            nil,
	} {
		t.Run(name, func(t *testing.T) {
			tool := lazyWithSchema(schema)
			assert.Empty(t, tool.Parameters())
		})
	}
}

func TestLazyMCPToolDescription(t *testing.T) {
	tool := NewLazyMCPTool("t", "explicit", nil, LazyMCPServerConfig{}).(*LazyMCPTool)
	assert.Equal(t, "explicit", tool.Description())

	tool = NewLazyMCPTool("t", "", nil, LazyMCPServerConfig{Logger: &mockLogger{}}).(*LazyMCPTool)
	assert.Equal(t, "t (MCP tool)", tool.Description())
	tool.serverInfo = &interfaces.MCPServerInfo{Name: "srv"}
	assert.Equal(t, "t (MCP tool)", tool.Description())
	tool.serverInfo.Title = "Server"
	assert.Equal(t, "t (from Server)", tool.Description())
}

func TestExtractTextFromMCPContent(t *testing.T) {
	tests := []struct {
		name    string
		content interface{}
		want    string
	}{
		{"string", "hi", "hi"},
		{"bytes", []byte("raw"), "raw"},
		{"mcp content", []mcp.Content{
			&mcp.TextContent{Text: "one"},
			&mcp.TextContent{},
			&mcp.ImageContent{Data: []byte("x"), MIMEType: "image/png"},
		}, "one\n&{Text: Meta:map[] Annotations:<nil>}\n" + `{"type":"image","mimeType":"image/png","data":"eA=="}`},
		{"slice", []interface{}{"a", map[string]interface{}{"text": "b"}}, "a\nb"},
		{"map text", map[string]interface{}{"text": "t"}, "t"},
		{"map content", map[string]interface{}{"content": "c"}, "c"},
		{"map message", map[string]interface{}{"message": "m"}, "m"},
		{"map other", map[string]interface{}{"n": 1}, `{"n":1}`},
		{"map that cannot marshal", map[string]interface{}{"ch": nil, "f": func() {}}, "map[ch:<nil> f:"},
		{"number", 3.5, "3.5"},
		{"func", func() {}, "0x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTextFromMCPContent(tt.content)
			if strings.HasSuffix(tt.want, ":") || tt.want == "0x" {
				assert.True(t, strings.HasPrefix(got, tt.want), "got %q", got)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
