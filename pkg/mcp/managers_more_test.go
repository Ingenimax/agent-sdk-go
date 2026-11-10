package mcp

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestToolManagerListAndFilter(t *testing.T) {
	weather := interfaces.MCPTool{Name: "get_weather", Description: "Current Weather", OutputSchema: CreateWeatherSchema()}
	files := interfaces.MCPTool{Name: "list_files", Description: "List files"}
	good := &stubServer{tools: []interfaces.MCPTool{weather, files}}
	broken := &stubServer{listErr: errors.New("down")}
	manager := NewToolManager([]interfaces.MCPServer{broken, good})

	all, err := manager.ListAllTools(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[string][]interfaces.MCPTool{"server-1": {weather, files}}, all)

	byCategory, err := manager.GetToolsByCategory(t.Context(), "WEATHER")
	require.NoError(t, err)
	require.Len(t, byCategory, 1)
	assert.Equal(t, "get_weather", byCategory[0].Tool.Name)
	assert.Same(t, good, byCategory[0].Server)

	withSchema, err := manager.GetToolsWithOutputSchema(t.Context())
	require.NoError(t, err)
	require.Len(t, withSchema, 1)
	assert.Equal(t, "get_weather", withSchema[0].Tool.Name)

	none, err := manager.GetToolsByCategory(t.Context(), "a category longer than any tool text")
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestToolManagerCallToolWithValidation(t *testing.T) {
	weather := interfaces.MCPTool{Name: "get_weather", OutputSchema: CreateWeatherSchema()}

	t.Run("valid response", func(t *testing.T) {
		resp := &interfaces.MCPToolResponse{StructuredContent: map[string]interface{}{"temperature": 21.5, "conditions": "sunny"}}
		server := &stubServer{tools: []interfaces.MCPTool{weather}, callResp: resp}
		manager := NewToolManager([]interfaces.MCPServer{&stubServer{listErr: errors.New("skip me")}, server})

		got, err := manager.CallToolWithValidation(t.Context(), "get_weather", map[string]any{"city": "Lisbon"})
		require.NoError(t, err)
		assert.Same(t, resp, got)
		assert.Equal(t, []any{map[string]any{"city": "Lisbon"}}, server.callArgs())
	})

	t.Run("invalid response is still returned", func(t *testing.T) {
		resp := &interfaces.MCPToolResponse{StructuredContent: map[string]interface{}{"temperature": "hot"}}
		server := &stubServer{tools: []interfaces.MCPTool{weather}, callResp: resp}

		got, err := NewToolManager([]interfaces.MCPServer{server}).CallToolWithValidation(t.Context(), "get_weather", nil)
		require.NoError(t, err)
		assert.Same(t, resp, got)
	})

	t.Run("unknown tool", func(t *testing.T) {
		manager := NewToolManager([]interfaces.MCPServer{&stubServer{tools: []interfaces.MCPTool{weather}}})
		_, err := manager.CallToolWithValidation(t.Context(), "missing", nil)
		assert.ErrorContains(t, err, "tool not found: missing")
	})

	t.Run("call fails", func(t *testing.T) {
		server := &stubServer{tools: []interfaces.MCPTool{weather}, callErr: errors.New("timeout")}
		_, err := NewToolManager([]interfaces.MCPServer{server}).CallToolWithValidation(t.Context(), "get_weather", nil)
		assert.ErrorContains(t, err, "tool call failed: timeout")
	})
}

func TestSchemaBuilder(t *testing.T) {
	schema := NewSchemaBuilder().
		Object().
		Property("name", NewSchemaBuilder().String().Build()).
		Property("score", NewSchemaBuilder().Number().Build()).
		Property("ok", NewSchemaBuilder().Boolean().Build()).
		Property("tags", NewSchemaBuilder().Array(map[string]interface{}{"type": "string"}).Build()).
		Required("name", "ok").
		Description("a record").
		Build()

	assert.Equal(t, map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name":  map[string]interface{}{"type": "string"},
			"score": map[string]interface{}{"type": "number"},
			"ok":    map[string]interface{}{"type": "boolean"},
			"tags":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
		},
		"required":    []interface{}{"name", "ok"},
		"description": "a record",
	}, schema)

	// Property on a non-object schema is ignored.
	assert.Equal(t, map[string]interface{}{"type": "string"}, NewSchemaBuilder().String().Property("x", nil).Build())
}

func TestPrebuiltSchemasValidate(t *testing.T) {
	v := NewSchemaValidator()
	tests := []struct {
		name    string
		schema  map[string]interface{}
		data    map[string]interface{}
		wantErr string
	}{
		{"weather ok", CreateWeatherSchema(), map[string]interface{}{"temperature": 20.0, "conditions": "rain", "humidity": 80}, ""},
		{"weather missing field", CreateWeatherSchema(), map[string]interface{}{"temperature": 20.0}, "missing required field 'conditions'"},
		{"file ok", CreateFileInfoSchema(), map[string]interface{}{"name": "a.txt", "size": 3, "modified": "2025-01-01T00:00:00Z"}, ""},
		{"file wrong type", CreateFileInfoSchema(), map[string]interface{}{"name": 1, "size": 3}, "expected string for tool t.name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := interfaces.MCPTool{Name: "t", OutputSchema: tt.schema}
			err := v.ValidateToolResponse(t.Context(), tool, &interfaces.MCPToolResponse{StructuredContent: tt.data})
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSchemaValidatorTypeMismatches(t *testing.T) {
	v := NewSchemaValidator()
	tests := []struct {
		name    string
		schema  map[string]interface{}
		data    interface{}
		wantErr string
	}{
		{"array of wrong items", map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "number"}}, []interface{}{1.0, "x"}, "expected number for tool t[1]"},
		{"array of right items", map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "integer"}}, []int{1, 2}, ""},
		{"not an array", map[string]interface{}{"type": "array"}, "x", "expected array"},
		{"not a string", map[string]interface{}{"type": "string"}, 1, "expected string"},
		{"json number", map[string]interface{}{"type": "number"}, float32(1), ""},
		{"not a boolean", map[string]interface{}{"type": "boolean"}, "true", "expected boolean"},
		{"boolean", map[string]interface{}{"type": "boolean"}, false, ""},
		{"not an object", map[string]interface{}{"type": "object"}, []int{}, "expected object"},
		{"unknown type", map[string]interface{}{"type": "tuple"}, 1, ""},
		{"schema not a map", nil, 1, "invalid schema format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var schema interface{} = tt.schema
			if tt.schema == nil {
				schema = "not a map"
			}
			err := v.validateAgainstSchema(t.Context(), tt.data, schema, "t")
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// samplingServer is a stubServer that answers CreateMessage and records requests.
type samplingServer struct {
	stubServer
	resp     *interfaces.MCPSamplingResponse
	err      error
	requests []*interfaces.MCPSamplingRequest
}

func (s *samplingServer) CreateMessage(_ context.Context, req *interfaces.MCPSamplingRequest) (*interfaces.MCPSamplingResponse, error) {
	s.requests = append(s.requests, req)
	return s.resp, s.err
}

func TestSamplingManagerRequests(t *testing.T) {
	reply := &interfaces.MCPSamplingResponse{Model: "m", Content: interfaces.MCPContent{Type: "text", Text: "ok"}}
	failing := &samplingServer{err: errors.New("no sampling")}
	working := &samplingServer{resp: reply}
	manager := NewSamplingManager([]interfaces.MCPServer{failing, working})
	ctx := t.Context()

	resp, err := manager.CreateConversation(ctx, []interfaces.MCPMessage{{Role: "user", Content: interfaces.MCPContent{Type: "text", Text: "hi"}}},
		WithSystemPrompt("be kind"),
		WithMaxTokens(50),
		WithTemperature(0.3),
		WithModelHint("claude"),
		WithModelPreferences(0.1, 0.2, 0.9),
		WithStopSequences("END", "STOP"),
		WithIncludeContext("allServers"),
	)
	require.NoError(t, err)
	assert.Same(t, reply, resp)

	// The first server failed, so the request also reached the second one.
	require.Len(t, failing.requests, 1)
	req := working.requests[0]
	assert.Same(t, failing.requests[0], req)
	assert.Equal(t, "be kind", req.SystemPrompt)
	assert.Equal(t, 50, *req.MaxTokens)
	assert.InDelta(t, 0.3, *req.Temperature, 1e-9)
	assert.Equal(t, &interfaces.MCPModelPreferences{
		Hints:        []interfaces.MCPModelHint{{Name: "claude"}},
		CostPriority: 0.1, SpeedPriority: 0.2, IntelligencePriority: 0.9,
	}, req.ModelPreferences)
	assert.Equal(t, []string{"END", "STOP"}, req.StopSequences)
	assert.Equal(t, "allServers", req.IncludeContext)

	_, err = manager.CreateCodeGeneration(ctx, "sort a list", "Go")
	require.NoError(t, err)
	req = working.requests[1]
	assert.Contains(t, req.SystemPrompt, "high-quality Go code")
	assert.InDelta(t, 0.9, req.ModelPreferences.IntelligencePriority, 1e-9)

	_, err = manager.CreateCodeGeneration(ctx, "sort a list", "", WithMaxTokens(10))
	require.NoError(t, err)
	assert.Equal(t, "You are an expert programmer. Generate high-quality code based on the user's request.", working.requests[2].SystemPrompt)
	assert.Equal(t, 10, *working.requests[2].MaxTokens)

	_, err = manager.CreateSummary(ctx, "long text", 25, WithTemperature(0))
	require.NoError(t, err)
	req = working.requests[3]
	assert.Equal(t, "Please summarize the following content in no more than 25 words:\n\nlong text", req.Messages[0].Content.Text)
	assert.Equal(t, 25, *req.MaxTokens)

	_, err = manager.CreateImageAnalysisMessage(ctx, "aGVsbG8=", "image/png", "what is this?", WithSystemPrompt("look"))
	require.NoError(t, err)
	req = working.requests[4]
	require.Len(t, req.Messages, 2)
	assert.Equal(t, interfaces.MCPContent{Type: "image", Data: "aGVsbG8=", MimeType: "image/png"}, req.Messages[0].Content)
	assert.Equal(t, "what is this?", req.Messages[1].Content.Text)
	assert.Equal(t, "look", req.SystemPrompt)

	_, err = NewSamplingManager([]interfaces.MCPServer{failing}).CreateTextMessage(ctx, "x", WithModelHint("a"))
	assert.ErrorContains(t, err, "sampling failed on all servers: no sampling")
}

func TestSamplingOptionsOnEmptyRequest(t *testing.T) {
	req := &interfaces.MCPSamplingRequest{}
	WithModelHint("a")(req)
	assert.Equal(t, []interfaces.MCPModelHint{{Name: "a"}}, req.ModelPreferences.Hints)

	req = &interfaces.MCPSamplingRequest{}
	WithModelPreferences(1, 2, 3)(req)
	assert.Equal(t, &interfaces.MCPModelPreferences{CostPriority: 1, SpeedPriority: 2, IntelligencePriority: 3}, req.ModelPreferences)
}

func TestExtractCodeFromResponse(t *testing.T) {
	text := func(s string) *interfaces.MCPSamplingResponse {
		return &interfaces.MCPSamplingResponse{Content: interfaces.MCPContent{Type: "text", Text: s}}
	}
	tests := []struct {
		name string
		resp *interfaces.MCPSamplingResponse
		want string
	}{
		{"nil", nil, ""},
		{"not text", &interfaces.MCPSamplingResponse{Content: interfaces.MCPContent{Type: "image", Text: "x"}}, ""},
		{"no code block", text("just words"), "just words"},
		{"one block", text("Here:\n```go\nfmt.Println(1)\n```\nDone"), "fmt.Println(1)"},
		{"two blocks", text("```\na\n```\nmid\n```\nb\nc\n```"), "a\nb\nc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ExtractCodeFromResponse(tt.resp))
		})
	}
}

func TestFormatConversationHistory(t *testing.T) {
	long := make([]byte, 120)
	for i := range long {
		long[i] = 'a'
	}
	got := FormatConversationHistory([]interfaces.MCPMessage{
		{Role: "user", Content: interfaces.MCPContent{Text: "hello"}},
		{Role: "ASSISTANT", Content: interfaces.MCPContent{Text: string(long)}},
	})
	assert.Equal(t, "User: hello\nAssistant: "+string(long[:97])+"...", got)
	assert.Empty(t, FormatConversationHistory(nil))
}

func TestFormatUserFriendlyErrorAllTypes(t *testing.T) {
	cause := errors.New("cause")
	withTool := NewMCPError("CallTool", "srv", "http", MCPErrorTypeToolNotFound, cause).WithMetadata("tool_name", "grep")
	tests := []struct {
		err  error
		want string
	}{
		{errors.New("plain"), "plain"},
		{NewMCPError("op", "srv", "stdio", MCPErrorTypeConnection, cause), "Could not start MCP server 'srv'"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeConnection, cause), "Could not connect to MCP server 'srv'"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeTimeout, cause), "took too long"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeAuthentication, cause), "Authentication failed"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeServerNotFound, cause), "command not found"},
		{withTool, "Tool 'grep' is not available"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeToolNotFound, cause), "Requested tool is not available"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeToolInvalidArgs, cause), "Invalid arguments"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeConfiguration, cause), "MCP configuration error: cause"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeServerStartup, cause), "failed to start"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeServerCrash, cause), "crashed unexpectedly"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeToolExecution, cause), "Tool execution failed on MCP server 'srv': cause"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeProtocol, cause), "Protocol error"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeSerialization, cause), "Serialization error"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeValidation, cause), "Validation error"},
		{NewMCPError("op", "srv", "http", MCPErrorTypeUnknown, cause), "MCP server 'srv' error: cause"},
		{NewMCPError("op", "srv", "http", "SOMETHING_NEW", cause), "MCP server 'srv' error: cause"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Contains(t, FormatUserFriendlyError(tt.err), tt.want)
		})
	}
}

func TestBuilderAddPreset(t *testing.T) {
	configs, err := NewBuilder().AddPreset("filesystem").AddPreset("time").BuildLazy()
	require.NoError(t, err)
	require.Len(t, configs, 2)
	assert.Equal(t, "filesystem", configs[0].Name)
	assert.Equal(t, "stdio", configs[0].Type)
	assert.Equal(t, "npx", configs[0].Command)

	_, err = NewBuilder().AddPreset("no-such-preset").BuildLazy()
	assert.ErrorContains(t, err, `preset "no-such-preset" not found`)

	t.Setenv("GITHUB_TOKEN", "")
	_, err = NewBuilder().AddPreset("github").BuildLazy()
	assert.ErrorContains(t, err, "requires environment variable GITHUB_TOKEN")
}

func TestBuilderInitializeServer(t *testing.T) {
	// One attempt only: with more, a failure re-enters retryConnection.
	newBuilder := func() *Builder {
		return NewBuilder().WithLogger(logging.New()).WithRetry(1, time.Millisecond).WithTimeout(10 * time.Second)
	}

	t.Run("http", func(t *testing.T) {
		ts, _ := newStreamableTestServer(t)
		server, err := newBuilder().initializeServer(t.Context(), LazyMCPServerConfig{Name: "h", Type: "http", URL: ts.URL, HttpTransportMode: "streamable"})
		require.NoError(t, err)
		t.Cleanup(func() { _ = server.Close() })
		info, err := server.GetServerInfo()
		require.NoError(t, err)
		assert.Equal(t, "test-server", info.Name)
	})

	t.Run("stdio", func(t *testing.T) {
		server, err := newBuilder().initializeServer(t.Context(), LazyMCPServerConfig{
			Name: "s", Type: "stdio", Command: os.Args[0], Env: []string{stdioServerEnv + "=1"},
		})
		require.NoError(t, err)
		// The builder context ends when initializeServer returns, which stops
		// the subprocess, so only the handshake data is checked here.
		info, err := server.GetServerInfo()
		require.NoError(t, err)
		assert.Equal(t, "test-server", info.Name)
		_ = server.Close()
	})

	t.Run("stdio failure", func(t *testing.T) {
		server, err := newBuilder().initializeServer(t.Context(), LazyMCPServerConfig{Name: "s", Type: "stdio", Command: "definitely-not-a-real-command-xyz"})
		assert.Nil(t, server)
		assert.ErrorContains(t, err, "invalid command")
	})

	t.Run("unsupported type", func(t *testing.T) {
		_, err := newBuilder().initializeServer(t.Context(), LazyMCPServerConfig{Type: "ftp"})
		assert.ErrorContains(t, err, "unsupported server type: ftp")
	})
}
