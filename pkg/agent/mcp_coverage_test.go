package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

type echoArgs struct {
	Text string `json:"text,omitempty"`
}

// newMCPTestServer starts an in-process streamable HTTP MCP server with the
// tools "echo" and "shout". Each tool answers "<name>:<text>".
func newMCPTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "cov-server", Title: "Coverage", Version: "1.0.0"}, nil)
	for _, name := range []string{"echo", "shout"} {
		mcpsdk.AddTool(server, &mcpsdk.Tool{Name: name, Description: name + " text"},
			func(_ context.Context, _ *mcpsdk.CallToolRequest, in echoArgs) (*mcpsdk.CallToolResult, any, error) {
				return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: name + ":" + in.Text}}}, nil, nil
			})
	}
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
	ts := httptest.NewServer(handler)
	// The MCP server cache keeps streaming connections open, so close them
	// before Close, which otherwise waits for them forever.
	t.Cleanup(func() {
		ts.CloseClientConnections()
		ts.Close()
	})
	return ts
}

// fakeMCPServer is an interfaces.MCPServer whose ListTools is scripted. Other
// methods are not used by the code under test.
type fakeMCPServer struct {
	interfaces.MCPServer
	tools []interfaces.MCPTool
	err   error
}

func (f *fakeMCPServer) ListTools(context.Context) ([]interfaces.MCPTool, error) {
	return f.tools, f.err
}

func TestCollectMCPToolsAndAssemble(t *testing.T) {
	good := &fakeMCPServer{tools: []interfaces.MCPTool{{Name: "remote_a", Description: "a"}, {Name: "remote_b"}}}
	bad := &fakeMCPServer{err: errors.New("unreachable")}

	a, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithTools(&testutil.FakeTool{ToolName: "local"}),
		WithMCPServers([]interfaces.MCPServer{bad, good}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Construction already merged the MCP tools into the agent's tools.
	if got := toolNames(a.GetTools()); !slices.Equal(got, []string{"local", "remote_a", "remote_b"}) {
		t.Errorf("tools after init = %v", got)
	}

	// A run re-collects them; deduplication keeps the set stable.
	if got := toolNames(a.assembleTools(context.Background())); !slices.Equal(got, []string{"local", "remote_a", "remote_b"}) {
		t.Errorf("assembled tools = %v", got)
	}
}

func TestLazyMCPToolsDiscoveredFromServer(t *testing.T) {
	ts := newMCPTestServer(t)
	// The MCP package caches servers by name, not URL, for the life of the
	// process. A name tied to this server's address keeps a later run (for
	// example with -count) from reusing a server whose URL is already closed.
	suffix := "-" + ts.Listener.Addr().String()

	tests := []struct {
		name    string
		config  LazyMCPConfig
		want    []string
		execute bool
	}{
		{
			name:    "discovers every tool",
			config:  LazyMCPConfig{Name: "cov-all" + suffix, Type: "http", URL: ts.URL, HttpTransportMode: "streamable"},
			want:    []string{"echo", "shout"},
			execute: true,
		},
		{
			name:   "filters by allowed tools",
			config: LazyMCPConfig{Name: "cov-allowed" + suffix, Type: "http", URL: ts.URL, HttpTransportMode: "streamable", AllowedTools: []string{"shout"}},
			want:   []string{"shout"},
		},
		{
			name: "uses configured tools",
			config: LazyMCPConfig{
				Name: "cov-configured" + suffix, Type: "http", URL: ts.URL, HttpTransportMode: "streamable",
				Tools: []LazyMCPToolConfig{{Name: "echo", Description: "configured echo"}},
			},
			want: []string{"echo"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{logger: logging.New(), lazyMCPConfigs: []LazyMCPConfig{tc.config}}
			got := a.createLazyMCPTools()
			if names := toolNames(got); !slices.Equal(names, tc.want) {
				t.Fatalf("tools = %v, want %v", names, tc.want)
			}
			if !tc.execute {
				return
			}
			out, err := got[0].Execute(context.Background(), `{"text":"hi"}`)
			if err != nil || !strings.Contains(out, "echo:hi") {
				t.Fatalf("Execute() = %q, %v", out, err)
			}
		})
	}
}

func TestLazyMCPToolsServerFailure(t *testing.T) {
	a := &Agent{logger: logging.New(), lazyMCPConfigs: []LazyMCPConfig{
		{Name: "cov-bad-discover", Type: "carrier-pigeon"},
		{Name: "cov-bad-configured", Type: "carrier-pigeon", Tools: []LazyMCPToolConfig{{Name: "kept"}}},
	}}
	// Discovery failure skips the server; configured tools are still created.
	if got := toolNames(a.createLazyMCPTools()); !slices.Equal(got, []string{"kept"}) {
		t.Fatalf("tools = %v", got)
	}
}

func TestAgentRunsLazyMCPToolFromURL(t *testing.T) {
	ts := newMCPTestServer(t)

	var toolOutputs []string
	llm := &testutil.FakeLLM{GenerateFunc: func(ctx context.Context, _ string, tools []interfaces.Tool) (string, error) {
		for _, tool := range tools {
			out, err := tool.Execute(ctx, `{"text":"x"}`)
			if err != nil {
				return "", err
			}
			toolOutputs = append(toolOutputs, out)
		}
		return "done", nil
	}}

	a, err := NewAgent(
		WithLLM(llm),
		WithMCPURLs(ts.URL+"?transport=streamable"),
		WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := toolNames(a.GetTools()); !slices.Equal(got, []string{"echo", "shout"}) {
		t.Fatalf("tools = %v", got)
	}
	got, err := a.Run(context.Background(), "use the tools")
	if err != nil || got != "done" {
		t.Fatalf("Run() = %q, %v", got, err)
	}
	if !slices.ContainsFunc(toolOutputs, func(s string) bool { return strings.Contains(s, "shout:x") }) {
		t.Errorf("tool outputs = %v", toolOutputs)
	}
}

func TestWithMCPURLsAndPresets(t *testing.T) {
	a := &Agent{logger: logging.New()}
	WithMCPURLs("stdio://files/usr/bin/mcp-files?root=/tmp", "https://mcp.example.com/mcp?token=secret&transport=sse")(a)
	if len(a.lazyMCPConfigs) != 2 {
		t.Fatalf("configs = %+v", a.lazyMCPConfigs)
	}
	stdio, http := a.lazyMCPConfigs[0], a.lazyMCPConfigs[1]
	if stdio.Type != "stdio" || stdio.Name != "files" || stdio.Command != "/usr/bin/mcp-files" || !slices.Equal(stdio.Args, []string{"--root=/tmp"}) {
		t.Errorf("stdio config = %+v", stdio)
	}
	if http.Type != "http" || http.Token != "secret" || http.HttpTransportMode != "sse" || strings.Contains(http.URL, "token") {
		t.Errorf("http config = %+v", http)
	}

	// An invalid URL drops the whole batch.
	b := &Agent{logger: logging.New()}
	WithMCPURLs("ftp://nope", "http://fine")(b)
	if len(b.lazyMCPConfigs) != 0 {
		t.Errorf("configs = %+v", b.lazyMCPConfigs)
	}

	c := &Agent{logger: logging.New()}
	WithMCPPresets("filesystem")(c)
	if len(c.lazyMCPConfigs) != 1 || c.lazyMCPConfigs[0].Name != "filesystem" {
		t.Errorf("preset configs = %+v", c.lazyMCPConfigs)
	}

	d := &Agent{logger: logging.New()}
	WithMCPPresets("no-such-preset")(d)
	if len(d.lazyMCPConfigs) != 0 {
		t.Errorf("unknown preset configs = %+v", d.lazyMCPConfigs)
	}
}

func TestMCPServerConfigGetServerType(t *testing.T) {
	tests := []struct {
		cfg  MCPServerConfig
		want string
	}{
		{MCPServerConfig{URL: "http://x", Command: "c"}, "http"},
		{MCPServerConfig{Command: "c"}, "stdio"},
		{MCPServerConfig{}, "stdio"},
	}
	for _, tc := range tests {
		if got := tc.cfg.GetServerType(); got != tc.want {
			t.Errorf("GetServerType(%+v) = %q, want %q", tc.cfg, got, tc.want)
		}
	}
}

func TestApplyMCPConfig(t *testing.T) {
	t.Setenv("COV_MCP_ENV", "from-env")
	cfg := &MCPConfiguration{
		MCPServers: map[string]MCPServerConfig{
			"local": {Command: "/bin/tool", Args: []string{"-v"}, Env: map[string]string{"A": "${COV_VAR}", "B": "${COV_MCP_ENV}"}, AllowedTools: []string{"t1"}},
			"web":   {URL: "http://mcp.local", HttpTransportMode: "STREAMABLE"},
			"auth":  {URL: "http://mcp.auth", Token: "tok"},
		},
		Global: &MCPGlobalConfig{RetryAttempts: 2},
	}

	a := &Agent{logger: logging.New()}
	applyMCPConfig(a, cfg, map[string]string{"COV_VAR": "from-vars"})

	byName := map[string]LazyMCPConfig{}
	for _, c := range a.lazyMCPConfigs {
		byName[c.Name] = c
	}
	if len(byName) != 3 {
		t.Fatalf("configs = %+v", a.lazyMCPConfigs)
	}
	local := byName["local"]
	slices.Sort(local.Env)
	if local.Type != "stdio" || !slices.Equal(local.Env, []string{"A=from-vars", "B=from-env"}) || !slices.Equal(local.AllowedTools, []string{"t1"}) {
		t.Errorf("local = %+v", local)
	}
	if web := byName["web"]; web.Type != "http" || web.HttpTransportMode != "streamable" {
		t.Errorf("web = %+v", web)
	}
	if auth := byName["auth"]; auth.Token != "tok" || auth.HttpTransportMode != "sse" {
		t.Errorf("auth = %+v", auth)
	}

	// Defaults are filled into a partial global config.
	g := cfg.Global
	if g.Timeout != "30s" || g.RetryAttempts != 2 || !*g.HealthCheck || !*g.EnableResources ||
		!*g.EnablePrompts || !*g.EnableSampling || !*g.EnableSchemas || g.LogLevel != "info" {
		t.Errorf("global defaults = %+v", g)
	}

	// No global config and a nil config are both handled.
	b := &Agent{logger: logging.New()}
	applyMCPConfig(b, &MCPConfiguration{MCPServers: map[string]MCPServerConfig{"x": {Command: "c"}}}, nil)
	if len(b.lazyMCPConfigs) != 1 {
		t.Errorf("configs = %+v", b.lazyMCPConfigs)
	}
	applyMCPConfig(b, nil, nil)
	if len(b.lazyMCPConfigs) != 1 {
		t.Error("nil config must not change the agent")
	}

	c := &Agent{logger: logging.New()}
	WithMCPConfig(&MCPConfiguration{MCPServers: map[string]MCPServerConfig{"y": {URL: "http://y"}}})(c)
	if len(c.lazyMCPConfigs) != 1 || c.lazyMCPConfigs[0].Type != "http" {
		t.Errorf("WithMCPConfig = %+v", c.lazyMCPConfigs)
	}
}

func TestMCPConfigFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := &MCPConfiguration{
		MCPServers: map[string]MCPServerConfig{
			"files": {Command: "/bin/files", Args: []string{"--ro"}},
			"web":   {URL: "http://mcp.local", HttpTransportMode: "sse"},
		},
		Global: &MCPGlobalConfig{Timeout: "10s"},
	}

	jsonPath := filepath.Join(dir, "mcp.json")
	if err := SaveMCPConfigToJSON(cfg, jsonPath); err != nil {
		t.Fatal(err)
	}
	fromJSON, err := LoadMCPConfigFromJSON(jsonPath)
	if err != nil || fromJSON.MCPServers["web"].URL != "http://mcp.local" || fromJSON.Global.Timeout != "10s" {
		t.Fatalf("json round trip = %+v, %v", fromJSON, err)
	}

	yamlPath := filepath.Join(dir, "mcp.yaml")
	if err := SaveMCPConfigToYAML(cfg, yamlPath); err != nil {
		t.Fatal(err)
	}
	fromYAML, err := LoadMCPConfigFromYAML(yamlPath)
	if err != nil || fromYAML.MCPServers["files"].Args[0] != "--ro" {
		t.Fatalf("yaml round trip = %+v, %v", fromYAML, err)
	}

	a := &Agent{logger: logging.New()}
	WithMCPConfigFromJSON(jsonPath)(a)
	b := &Agent{logger: logging.New()}
	WithMCPConfigFromYAML(yamlPath)(b)
	if len(a.lazyMCPConfigs) != 2 || len(b.lazyMCPConfigs) != 2 {
		t.Errorf("from files = %d, %d", len(a.lazyMCPConfigs), len(b.lazyMCPConfigs))
	}

	// Load errors leave the agent untouched.
	bad := writeFile(t, dir, "bad.txt", "{not json: [")
	for name, opt := range map[string]Option{
		"json missing": WithMCPConfigFromJSON(filepath.Join(dir, "none.json")),
		"json invalid": WithMCPConfigFromJSON(bad),
		"yaml missing": WithMCPConfigFromYAML(filepath.Join(dir, "none.yaml")),
		"yaml invalid": WithMCPConfigFromYAML(bad),
	} {
		c := &Agent{logger: logging.New()}
		opt(c)
		if len(c.lazyMCPConfigs) != 0 {
			t.Errorf("%s: configs = %+v", name, c.lazyMCPConfigs)
		}
	}
	if _, err := LoadMCPConfigFromJSON(bad); err == nil || !strings.Contains(err.Error(), "failed to parse JSON") {
		t.Errorf("json parse error = %v", err)
	}
	if _, err := LoadMCPConfigFromYAML(bad); err == nil || !strings.Contains(err.Error(), "failed to parse YAML") {
		t.Errorf("yaml parse error = %v", err)
	}

	// Writing into a missing directory fails.
	missing := filepath.Join(dir, "no", "such", "dir", "x")
	if err := SaveMCPConfigToJSON(cfg, missing); err == nil {
		t.Error("expected JSON write error")
	}
	if err := SaveMCPConfigToYAML(cfg, missing); err == nil {
		t.Error("expected YAML write error")
	}
}

func TestGetMCPConfigFromAgent(t *testing.T) {
	a := &Agent{lazyMCPConfigs: []LazyMCPConfig{
		{Name: "files", Command: "/bin/files", Args: []string{"-v"}, Env: []string{"A=1", "B=x=y", "broken"}},
		{Name: "web", URL: "http://mcp.local"},
	}}
	cfg := GetMCPConfigFromAgent(a)
	files := cfg.MCPServers["files"]
	if files.Command != "/bin/files" || files.Env["A"] != "1" || files.Env["B"] != "x=y" || len(files.Env) != 2 {
		t.Errorf("files = %+v", files)
	}
	if cfg.MCPServers["web"].URL != "http://mcp.local" {
		t.Errorf("web = %+v", cfg.MCPServers["web"])
	}
}

func TestValidateMCPConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *MCPConfiguration
		wantErr string
	}{
		{"nil", nil, "config cannot be nil"},
		{"nil servers", &MCPConfiguration{}, "mcpServers cannot be nil"},
		{"empty name", &MCPConfiguration{MCPServers: map[string]MCPServerConfig{"": {Command: "c"}}}, "server name cannot be empty"},
		{"stdio without command", &MCPConfiguration{MCPServers: map[string]MCPServerConfig{"s": {}}}, "command is required"},
		{"valid", &MCPConfiguration{MCPServers: map[string]MCPServerConfig{
			"s": {Command: "c"},
			"h": {URL: "http://x"},
		}}, ""},
		{"bad transport", &MCPConfiguration{MCPServers: map[string]MCPServerConfig{"h": {URL: "http://x", HttpTransportMode: "carrier"}}}, "invalid httpTransportMode"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMCPConfig(tc.cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
