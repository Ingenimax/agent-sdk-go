package mcp

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestMCPToolRun(t *testing.T) {
	tests := []struct {
		name    string
		resp    *interfaces.MCPToolResponse
		callErr error
		input   string
		want    string
		wantErr string
	}{
		{name: "string content", resp: &interfaces.MCPToolResponse{Content: "plain"}, input: `{"a":1}`, want: "plain"},
		{name: "byte content", resp: &interfaces.MCPToolResponse{Content: []byte("bytes")}, input: `{}`, want: "bytes"},
		{name: "structured content", resp: &interfaces.MCPToolResponse{Content: map[string]any{"k": "v"}}, input: `{}`, want: `{"k":"v"}`},
		{name: "content that cannot marshal", resp: &interfaces.MCPToolResponse{Content: func() {}}, input: `{}`, want: "0x"},
		{name: "tool error", resp: &interfaces.MCPToolResponse{IsError: true, Content: "nope"}, input: `{}`, wantErr: "MCP tool error: nope"},
		{name: "call error", callErr: errors.New("down"), input: `{}`, wantErr: "down"},
		{name: "bad input", input: `not json`, wantErr: "failed to parse input as JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubServer{callResp: tt.resp, callErr: tt.callErr}
			tool := NewMCPTool("t", "desc", nil, stub)

			got, err := tool.Execute(t.Context(), tt.input)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, got, tt.want)
		})
	}
}

func TestMCPToolAccessors(t *testing.T) {
	tool := NewMCPTool("name", "desc", nil, &stubServer{}).(*MCPTool)
	assert.Equal(t, "name", tool.Name())
	assert.Equal(t, "name", tool.DisplayName())
	assert.Equal(t, "desc", tool.Description())
	assert.False(t, tool.Internal())
	assert.Empty(t, tool.Parameters())

	stub := &stubServer{callResp: &interfaces.MCPToolResponse{Content: "ran"}}
	got, err := NewMCPTool("n", "", nil, stub).Run(t.Context(), `{"x":"y"}`)
	require.NoError(t, err)
	assert.Equal(t, "ran", got)
	assert.Equal(t, []any{map[string]interface{}{"x": "y"}}, stub.callArgs())
}

func TestMCPToolParametersFromJSONSchema(t *testing.T) {
	schema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"mode":   {Type: "string", Enum: []any{"fast", "slow"}, Description: "how"},
			"maybe":  {Types: []string{"string", "null"}},
			"bare":   {},
			"list":   {Type: "array"},
			"choice": {Type: "array", Items: &jsonschema.Schema{Enum: []any{"x"}}},
		},
		Required: []string{"mode"},
	}
	params := NewMCPTool("t", "", schema, nil).Parameters()

	assert.Equal(t, interfaces.ParameterSpec{Type: "string", Description: "how", Enum: []interface{}{"fast", "slow"}, Required: true}, params["mode"])
	assert.Equal(t, []string{"string", "null"}, params["maybe"].Type)
	assert.Equal(t, "string", params["bare"].Type)
	assert.Equal(t, &interfaces.ParameterSpec{Type: "string"}, params["list"].Items)
	assert.Equal(t, &interfaces.ParameterSpec{Type: "string", Enum: []interface{}{"x"}}, params["choice"].Items)
}

func TestMCPToolParametersFromMap(t *testing.T) {
	schema := map[string]interface{}{
		"properties": map[string]interface{}{
			"color": map[string]interface{}{"type": "string", "enum": []interface{}{"red"}},
			"ids":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"enum": []interface{}{1}}},
			"skip":  "not a map",
		},
		"required": []interface{}{"color"},
	}
	params := NewMCPTool("t", "", schema, nil).Parameters()

	assert.Len(t, params, 2)
	assert.True(t, params["color"].Required)
	assert.Equal(t, []interface{}{"red"}, params["color"].Enum)
	assert.Equal(t, &interfaces.ParameterSpec{Type: "string", Enum: []interface{}{1}}, params["ids"].Items)
}

func TestMCPToolAdapter(t *testing.T) {
	stub := &stubServer{callResp: &interfaces.MCPToolResponse{Content: "done"}}
	adapter := NewMCPToolAdapter(stub, "search", "Search things", map[string]ParameterSpec{
		"q": {Type: "string", Description: "query", Required: true},
	})

	assert.Equal(t, "search", adapter.Name())
	assert.Equal(t, "search", adapter.DisplayName())
	assert.Equal(t, "Search things", adapter.Description())
	assert.False(t, adapter.Internal())
	want := map[string]interfaces.ParameterSpec{"q": {Type: "string", Description: "query", Required: true}}
	assert.Equal(t, want, adapter.Parameters())
	assert.Equal(t, want, adapter.InputSchema())

	got, err := adapter.Run(t.Context(), `{"q":"go"}`)
	require.NoError(t, err)
	// The adapter formats the whole response struct with %v.
	assert.Contains(t, got, "done")
	assert.Equal(t, []any{map[string]interface{}{"q": "go"}}, stub.callArgs())

	_, err = adapter.Execute(t.Context(), "bad")
	assert.ErrorContains(t, err, "error parsing arguments for MCP tool search")

	stub.callErr = errors.New("offline")
	_, err = adapter.Execute(t.Context(), "{}")
	assert.ErrorContains(t, err, "error calling MCP tool search: offline")
}

func TestConvertMCPTools(t *testing.T) {
	stub := &stubServer{}
	tools, err := ConvertMCPTools(stub, []map[string]interface{}{
		{
			"name":        "a",
			"description": "first",
			"parameters": map[string]interface{}{
				"x":    map[string]interface{}{"type": "number", "description": "an x", "required": true},
				"skip": "not a map",
			},
		},
		{"name": "b"},
	})
	require.NoError(t, err)
	require.Len(t, tools, 2)

	assert.Equal(t, "first", tools[0].Description())
	assert.Equal(t, map[string]interfaces.ParameterSpec{"x": {Type: "number", Description: "an x", Required: true}}, tools[0].Parameters())
	assert.Equal(t, "No description provided", tools[1].Description())
	assert.Empty(t, tools[1].Parameters())

	_, err = ConvertMCPTools(stub, []map[string]interface{}{{"description": "no name"}})
	assert.ErrorContains(t, err, "missing name")
}

func TestRunStdioToolCommand(t *testing.T) {
	out, err := RunStdioToolCommand(exec.Command("cat"), []byte(`{"jsonrpc":"2.0"}`))
	require.NoError(t, err)
	assert.Equal(t, `{"jsonrpc":"2.0"}`, string(out))

	_, err = RunStdioToolCommand(exec.Command("false"), nil)
	assert.ErrorContains(t, err, "command execution failed")

	_, err = RunStdioToolCommand(exec.Command("/definitely/not/here"), nil)
	assert.ErrorContains(t, err, "failed to start command")

	// Pipes that are already set cannot be taken again.
	cmd := exec.Command("cat")
	cmd.Stdin = strings.NewReader("")
	_, err = RunStdioToolCommand(cmd, nil)
	assert.ErrorContains(t, err, "failed to get stdin pipe")

	cmd = exec.Command("cat")
	cmd.Stdout = io.Discard
	_, err = RunStdioToolCommand(cmd, nil)
	assert.ErrorContains(t, err, "failed to get stdout pipe")
}

func TestFetchMCPToolsFromServer(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		want        []map[string]interface{}
		wantErr     string
	}{
		{name: "tools", status: http.StatusOK, contentType: "application/json; charset=utf-8", body: `[{"name":"a"}]`, want: []map[string]interface{}{{"name": "a"}}},
		{name: "bad status", status: http.StatusBadGateway, contentType: "application/json", wantErr: "non-OK status: 502"},
		{name: "wrong content type", status: http.StatusOK, contentType: "text/html", body: "<html>", wantErr: "unexpected content type: text/html"},
		{name: "bad JSON", status: http.StatusOK, contentType: "application/json", body: `{`, wantErr: "failed to parse JSON response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/tools", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Accept"))
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer ts.Close()

			got, err := FetchMCPToolsFromServer(t.Context(), ts.URL)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := FetchMCPToolsFromServer(t.Context(), "http://bad host")
	assert.ErrorContains(t, err, "failed to create request")

	ts := httptest.NewServer(http.NotFoundHandler())
	ts.Close()
	_, err = FetchMCPToolsFromServer(t.Context(), ts.URL)
	assert.ErrorContains(t, err, "failed to send request")
}
