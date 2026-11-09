package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// isolateCLI points HOME and the working directory at temp dirs and clears
// every environment variable the CLI reads, so tests never touch real config.
func isolateCLI(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	for _, key := range []string{
		"LLM_PROVIDER", "OPENAI_API_KEY", "OPENAI_BASE_URL", "ANTHROPIC_API_KEY",
		"OLLAMA_BASE_URL", "VLLM_BASE_URL", "GOOGLE_API_KEY", "GOOGLE_SEARCH_ENGINE_ID",
		"GITHUB_TOKEN", "LANGFUSE_ENABLED",
	} {
		t.Setenv(key, "")
	}
	config.Reload()
	t.Cleanup(func() { config.Reload() })
	return home
}

// withArgs replaces os.Args for the duration of the test.
func withArgs(t *testing.T, args ...string) {
	t.Helper()
	saved := os.Args
	os.Args = append([]string{"agent-cli"}, args...)
	t.Cleanup(func() { os.Args = saved })
}

// captureStdout runs fn and returns what it printed to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	defer func() {
		os.Stdout = saved
	}()
	fn()
	_ = w.Close()
	<-done
	_ = r.Close()
	return buf.String()
}

// withStdin feeds input to os.Stdin for the duration of the test.
func withStdin(t *testing.T, input string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	f, err := os.Open(path) // #nosec G304 -- temp file created above
	if err != nil {
		t.Fatalf("open stdin: %v", err)
	}
	saved := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = saved
		_ = f.Close()
	})
}

// fakeOllama serves the Ollama generate and chat endpoints with a fixed reply
// and selects it as the CLI provider.
type fakeOllama struct {
	mu    sync.Mutex
	paths []string
}

func startFakeOllama(t *testing.T, reply string) *fakeOllama {
	t.Helper()
	f := &fakeOllama{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/generate":
			_ = json.NewEncoder(w).Encode(map[string]any{"response": reply, "done": true})
		case "/api/chat":
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"role": "assistant", "content": reply}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OLLAMA_BASE_URL", srv.URL)
	return f
}

func (f *fakeOllama) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.paths)
}

func TestDirectExecutionFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    DirectExecutionConfig
		wantErr bool
	}{
		{
			name: "equals form",
			args: []string{"--prompt=hi", "--mcp-config=/x.json", "--allowedTools=a,b", "--dangerously-skip-permissions"},
			want: DirectExecutionConfig{Prompt: "hi", MCPConfigFile: "/x.json", AllowedTools: []string{"a", "b"}, DangerouslySkipPermissions: true},
		},
		{
			name: "separate values",
			args: []string{"--prompt", "hello there", "--mcp-config", "/y.json", "--allowedTools", "c"},
			want: DirectExecutionConfig{Prompt: "hello there", MCPConfigFile: "/y.json", AllowedTools: []string{"c"}},
		},
		{name: "missing prompt value", args: []string{"--prompt"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withArgs(t, tt.args...)
			if !hasDirectExecutionFlags() {
				t.Fatal("hasDirectExecutionFlags = false")
			}
			got, err := parseDirectExecutionFlags()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got.Prompt != tt.want.Prompt || got.MCPConfigFile != tt.want.MCPConfigFile ||
				!slices.Equal(got.AllowedTools, tt.want.AllowedTools) || got.DangerouslySkipPermissions != tt.want.DangerouslySkipPermissions {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}

	withArgs(t, "run", "x")
	if hasDirectExecutionFlags() {
		t.Error("hasDirectExecutionFlags = true without --prompt")
	}
}

func TestLoadMCPServersFromFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	bad := filepath.Join(dir, "bad.json")
	writeFile(t, good, `{"mcpServers":{
		"web":{"url":"http://127.0.0.1:1/mcp"},
		"local":{"command":"python","args":["-m","srv"],"env":{"K":"V"}}
	}}`)
	writeFile(t, bad, "{")

	servers, err := loadMCPServersFromFile(good)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	byName := map[string]MCPServerConfig{}
	for _, s := range servers {
		byName[s.Name] = s
	}
	if web := byName["web"]; web.Type != "http" || web.URL != "http://127.0.0.1:1/mcp" || web.Env == nil {
		t.Errorf("web = %+v", web)
	}
	if local := byName["local"]; local.Type != "stdio" || local.Command != "python" || local.Env["K"] != "V" || len(local.Args) != 2 {
		t.Errorf("local = %+v", local)
	}

	for name, path := range map[string]string{"relative": "rel.json", "missing": filepath.Join(dir, "nope.json"), "bad json": bad} {
		if _, err := loadMCPServersFromFile(path); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestContainsAnyTool(t *testing.T) {
	tests := []struct {
		allowed []string
		want    bool
	}{
		{[]string{"mcp__aws__call"}, false},
		{[]string{"mcp__x", "my_websearch"}, true},
		{nil, false},
	}
	for _, tt := range tests {
		if got := containsAnyTool(tt.allowed, []string{"websearch", "github"}); got != tt.want {
			t.Errorf("containsAnyTool(%v) = %v", tt.allowed, got)
		}
	}
}

func TestCreateLazyMCPConfigs(t *testing.T) {
	servers := []MCPServerConfig{
		{Name: "a", Type: "stdio", Command: "cmd", Args: []string{"x"}, Env: map[string]string{"K": "V"}},
		{Name: "b", Type: "http", URL: "http://127.0.0.1:1"},
	}
	got := createLazyMCPConfigs(servers, []string{"t1", "t2"})
	if len(got) != 2 {
		t.Fatalf("configs = %d", len(got))
	}
	if got[0].Name != "a" || got[0].Command != "cmd" || !slices.Equal(got[0].Env, []string{"K=V"}) || len(got[0].Tools) != 2 {
		t.Errorf("a = %+v", got[0])
	}
	if got[0].Tools[1].Name != "t2" || got[0].Tools[1].Description != "MCP tool: t2" {
		t.Errorf("tool = %+v", got[0].Tools[1])
	}
	if got[1].URL != "http://127.0.0.1:1" || got[1].Env != nil {
		t.Errorf("b = %+v", got[1])
	}
	if tools := createDynamicMCPTools("a", nil); len(tools) != 0 {
		t.Errorf("tools without allow list = %v", tools)
	}
}

func TestGetDefaultModel(t *testing.T) {
	tests := map[string]string{
		"openai":    "gpt-4o-mini",
		"anthropic": "claude-3-5-sonnet-20241022",
		"vertex":    "gemini-1.5-pro",
		"ollama":    "llama3.2:latest",
		"vllm":      "meta-llama/Llama-2-7b-chat-hf",
		"other":     "gpt-4o-mini",
	}
	for provider, want := range tests {
		if got := getDefaultModel(provider); got != want {
			t.Errorf("getDefaultModel(%q) = %q, want %q", provider, got, want)
		}
	}
}

func TestLoadEnvFile(t *testing.T) {
	isolateCLI(t)
	for _, k := range []string{"CLI_TEST_PLAIN", "CLI_TEST_DOUBLE", "CLI_TEST_SINGLE", "CLI_TEST_KEEP"} {
		t.Setenv(k, "")
	}
	t.Setenv("CLI_TEST_KEEP", "original")
	writeFile(t, ".env", "# comment\n\nCLI_TEST_PLAIN = value\nCLI_TEST_DOUBLE=\"quoted\"\nCLI_TEST_SINGLE='single'\nCLI_TEST_KEEP=changed\nnot a pair\n")

	loadEnvFile()

	want := map[string]string{"CLI_TEST_PLAIN": "value", "CLI_TEST_DOUBLE": "quoted", "CLI_TEST_SINGLE": "single", "CLI_TEST_KEEP": "original"}
	for k, v := range want {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestLoadEnvFile_Missing(t *testing.T) {
	isolateCLI(t)
	loadEnvFile() // no .env file: nothing to do and no panic
}

func TestLoadAndSaveConfig(t *testing.T) {
	home := isolateCLI(t)

	def := loadConfig()
	if def.Provider != "openai" || def.Model != "gpt-4o-mini" || def.OrgID != "default-org" || !def.EnableMemory {
		t.Errorf("default config = %+v", def)
	}
	t.Setenv("LLM_PROVIDER", "ollama")
	if cfg := loadConfig(); cfg.Provider != "ollama" || cfg.Model != "llama3.2:latest" {
		t.Errorf("env provider config = %+v", cfg)
	}

	if err := os.MkdirAll(getConfigDir(), 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if getConfigDir() != filepath.Join(home, ".agent-cli") {
		t.Errorf("config dir = %q", getConfigDir())
	}
	t.Setenv("LLM_PROVIDER", "")
	saveConfig(&CLIConfig{Provider: "anthropic", Model: "claude-3-5-sonnet-20241022", OrgID: "o"})
	if cfg := loadConfig(); cfg.Provider != "anthropic" || cfg.OrgID != "o" {
		t.Errorf("saved config = %+v", cfg)
	}

	// LLM_PROVIDER overrides the provider in the saved file.
	t.Setenv("LLM_PROVIDER", "vllm")
	if cfg := loadConfig(); cfg.Provider != "vllm" || cfg.OrgID != "o" {
		t.Errorf("override config = %+v", cfg)
	}

	// An empty saved model takes the overriding provider's default.
	t.Setenv("LLM_PROVIDER", "")
	saveConfig(&CLIConfig{Provider: "openai"})
	t.Setenv("LLM_PROVIDER", "ollama")
	if cfg := loadConfig(); cfg.Model != "llama3.2:latest" {
		t.Errorf("empty model config = %+v", cfg)
	}
}

func TestManageConfig(t *testing.T) {
	isolateCLI(t)
	if err := os.MkdirAll(getConfigDir(), 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	for _, kv := range [][2]string{{"provider", "ollama"}, {"model", "m"}, {"system_prompt", "sp"}, {"org_id", "org"}, {"conversation_id", "conv"}} {
		withArgs(t, "config", "set", kv[0], kv[1])
		out := captureStdout(t, manageConfig)
		if !strings.Contains(out, "Set "+kv[0]+" = "+kv[1]) {
			t.Errorf("set %s output = %q", kv[0], out)
		}
	}
	cfg := loadConfig()
	if cfg.Provider != "ollama" || cfg.Model != "m" || cfg.SystemPrompt != "sp" || cfg.OrgID != "org" || cfg.ConversationID != "conv" {
		t.Errorf("config after set = %+v", cfg)
	}

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"config", "set", "bogus", "v"}, "Unknown config key: bogus"},
		{[]string{"config", "set", "model"}, "Usage: agent-cli config set"},
		{[]string{"config", "show"}, `"model": "m"`},
		{[]string{"config"}, "Current configuration:"},
		{[]string{"config", "what"}, "Unknown config subcommand: what"},
		{[]string{"config", "reset"}, "Configuration reset"},
	}
	for _, c := range cases {
		withArgs(t, c.args...)
		if out := captureStdout(t, manageConfig); !strings.Contains(out, c.want) {
			t.Errorf("%v output = %q, want %q", c.args, out, c.want)
		}
	}
	if _, err := os.Stat(filepath.Join(getConfigDir(), "config.json")); !os.IsNotExist(err) {
		t.Errorf("config file still exists after reset: %v", err)
	}
}

func TestManageMCP(t *testing.T) {
	isolateCLI(t)
	if err := os.MkdirAll(getConfigDir(), 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	run := func(args ...string) string {
		withArgs(t, append([]string{"mcp"}, args...)...)
		return captureStdout(t, manageMCP)
	}

	steps := []struct {
		args []string
		want string
	}{
		{nil, "MCP Server Management"},
		{[]string{"bogus"}, "Unknown MCP subcommand: bogus"},
		{[]string{"list"}, "No MCP servers configured."},
		{[]string{"add", "--name=x"}, "--type and --name are required"},
		{[]string{"add", "--name=x", "--type=ftp"}, "--type must be 'http' or 'stdio'"},
		{[]string{"add", "--name=x", "--type=http"}, "--url is required"},
		{[]string{"add", "--name=x", "--type=stdio"}, "--command is required"},
		{[]string{"add", "--name=web", "--type=http", "--url=http://127.0.0.1:1/mcp"}, "Added MCP server 'web' (http)"},
		{[]string{"add", "--name=local", "--type=stdio", "--command=/nonexistent/mcp-binary", "--args=a,b"}, "Args: [a b]"},
		{[]string{"add", "--name=web", "--type=http", "--url=http://other"}, "already exists"},
		{[]string{"list"}, "1. web (http)"},
		{[]string{"remove"}, "--name is required"},
		{[]string{"rm", "--name=ghost"}, "not found"},
		{[]string{"test"}, "--name is required"},
		{[]string{"test", "--name=ghost"}, "not found"},
		{[]string{"test", "--name=local"}, "Failed to"},
		{[]string{"export"}, "--file is required"},
		{[]string{"import"}, "--file is required"},
		{[]string{"import", "--file=relative.json"}, "must be absolute"},
		{[]string{"import", "--file=/nonexistent/dir/x.json"}, "Failed to read file"},
	}
	for _, s := range steps {
		if out := run(s.args...); !strings.Contains(out, s.want) {
			t.Errorf("mcp %v output = %q, want %q", s.args, out, s.want)
		}
	}

	exportPath := filepath.Join(t.TempDir(), "export.json")
	if out := run("export", "--file="+exportPath); !strings.Contains(out, "Exported 2 MCP servers") {
		t.Fatalf("export output = %q", out)
	}
	var exported MCPServersConfig
	data, err := os.ReadFile(exportPath) // #nosec G304 -- temp file
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if err := json.Unmarshal(data, &exported); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if exported.MCPServers["web"].URL != "http://127.0.0.1:1/mcp" || exported.MCPServers["local"].Command != "/nonexistent/mcp-binary" {
		t.Errorf("exported = %+v", exported)
	}

	if out := run("remove", "--name=web"); !strings.Contains(out, "Removed MCP server 'web'") {
		t.Errorf("remove output = %q", out)
	}

	badPath := filepath.Join(t.TempDir(), "bad.json")
	writeFile(t, badPath, "nope")
	if out := run("import", "--file="+badPath); !strings.Contains(out, "Failed to parse JSON") {
		t.Errorf("bad import output = %q", out)
	}

	// Importing the export restores web and skips local, which still exists.
	out := run("import", "--file="+exportPath)
	if !strings.Contains(out, "Imported: 1 servers") || !strings.Contains(out, "Skipped: 1 servers") {
		t.Errorf("import output = %q", out)
	}
	names := []string{}
	for _, s := range loadConfig().MCPServers {
		names = append(names, s.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"local", "web"}) {
		t.Errorf("servers after import = %v", names)
	}

	// Env values are listed.
	cfg := loadConfig()
	cfg.MCPServers[0].Env = map[string]string{"TOKEN": "abc"}
	saveConfig(cfg)
	if out := run("list"); !strings.Contains(out, "TOKEN=abc") {
		t.Errorf("list output = %q", out)
	}

	// Remove everything, then export has nothing to write.
	saveConfig(&CLIConfig{})
	if out := run("export", "--file="+exportPath); !strings.Contains(out, "No MCP servers configured to export") {
		t.Errorf("empty export output = %q", out)
	}
}

func TestMainDispatch(t *testing.T) {
	isolateCLI(t)
	tests := []struct {
		args []string
		want string
	}{
		{nil, "USAGE:"},
		{[]string{"version"}, "Agent SDK CLI v" + version},
		{[]string{"help"}, "COMMANDS:"},
		{[]string{"bogus"}, "Unknown command: bogus"},
		{[]string{"list"}, "Available resources"},
		{[]string{"list", "providers"}, "anthropic"},
		{[]string{"list", "models"}, "llama3.2:latest"},
		{[]string{"list", "tools"}, "websearch"},
		{[]string{"list", "config"}, "Current configuration:"},
		{[]string{"list", "things"}, "Unknown resource: things"},
		{[]string{"config", "show"}, "Current configuration:"},
		{[]string{"mcp"}, "MCP Server Management"},
		{[]string{"run"}, "Usage: agent-cli run"},
		{[]string{"task", "--task=x"}, "Usage: agent-cli task"},
		{[]string{"generate"}, "Usage: agent-cli generate"},
		{[]string{"--prompt"}, "Direct Execution Mode:"},
	}
	for _, tt := range tests {
		withArgs(t, tt.args...)
		if out := captureStdout(t, main); !strings.Contains(out, tt.want) {
			t.Errorf("args %v: output = %q, want %q", tt.args, out, tt.want)
		}
	}
}

func TestExecuteDirectPrompt_BadMCPConfig(t *testing.T) {
	isolateCLI(t)
	f := startFakeOllama(t, "unused")
	withArgs(t, "--prompt=hi", "--mcp-config=relative.json")
	executeDirectPrompt()
	if f.calls() != 0 {
		t.Errorf("LLM was called %d times despite a bad MCP config", f.calls())
	}
}

func TestExecuteDirectPrompt_RunsAgent(t *testing.T) {
	isolateCLI(t)
	f := startFakeOllama(t, "direct answer")
	mcpPath := filepath.Join(t.TempDir(), "mcp.json")
	writeFile(t, mcpPath, `{"mcpServers":{"srv":{"url":"http://127.0.0.1:1/mcp"}}}`)
	withArgs(t, "--prompt", "hello", "--mcp-config", mcpPath, "--allowedTools=websearch", "--dangerously-skip-permissions")

	out := captureStdout(t, main)
	if !strings.Contains(out, "direct answer") {
		t.Errorf("output = %q", out)
	}
	if f.calls() == 0 {
		t.Error("fake LLM was not called")
	}
}

func TestRunAgent(t *testing.T) {
	isolateCLI(t)
	f := startFakeOllama(t, "the answer")
	withArgs(t, "run", `"what`, `is it"`)

	out := captureStdout(t, main)
	if !strings.Contains(out, "Running agent with prompt: what is it\n") || !strings.Contains(out, "the answer") {
		t.Errorf("output = %q", out)
	}
	if f.calls() == 0 {
		t.Error("fake LLM was not called")
	}
}

func TestInteractiveChat(t *testing.T) {
	isolateCLI(t)
	f := startFakeOllama(t, "chat reply")
	withStdin(t, "\nhelp\nclear\nconfig\nhello\nbye\n")
	withArgs(t, "chat")

	out := captureStdout(t, main)
	for _, want := range []string{"Interactive Chat Mode", "Available commands:", "Conversation cleared", "Current configuration:", "chat reply", "Goodbye!"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if f.calls() == 0 {
		t.Error("fake LLM was not called")
	}
}

func TestExecuteTask(t *testing.T) {
	isolateCLI(t)
	startFakeOllama(t, "task done")
	dir := t.TempDir()
	agents := filepath.Join(dir, "agents.yaml")
	tasks := filepath.Join(dir, "tasks.yaml")
	outFile := filepath.Join(dir, "out-{topic}.md")
	writeFile(t, agents, "researcher:\n  role: Researcher on {topic}\n  goal: Find facts\n  backstory: Curious\n")
	writeFile(t, tasks, "research_task:\n  description: Research {topic} for {audience}\n  expected_output: A summary\n  agent: researcher\n  output_file: "+outFile+"\n")
	withArgs(t, "task", "--agent-config="+agents, "--task-config="+tasks, "--task=research_task", "--topic=AI", "--var=audience=kids", "--var=broken")

	out := captureStdout(t, main)
	wantFile := filepath.Join(dir, "out-AI.md")
	if !strings.Contains(out, "task done") || !strings.Contains(out, "Output saved to: "+wantFile) {
		t.Errorf("output = %q", out)
	}
	data, err := os.ReadFile(wantFile) // #nosec G304 -- temp file
	if err != nil || string(data) != "task done" {
		t.Errorf("output file = %q, %v", data, err)
	}
}

func TestCreateLLM(t *testing.T) {
	isolateCLI(t)
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	for _, provider := range []string{"openai", "anthropic", "ollama", "vllm"} {
		llm := createLLM(&CLIConfig{Provider: provider, Model: getDefaultModel(provider)})
		if llm == nil || llm.Name() == "" {
			t.Errorf("%s: got %v", provider, llm)
		}
	}
}

func TestCreateTools(t *testing.T) {
	isolateCLI(t)
	if tools := createTools(); len(tools) != 0 {
		t.Errorf("tools without keys = %d", len(tools))
	}
	t.Setenv("GOOGLE_API_KEY", "k")
	t.Setenv("GOOGLE_SEARCH_ENGINE_ID", "id")
	t.Setenv("GITHUB_TOKEN", "tok")
	config.Reload()
	if tools := createTools(); len(tools) != 2 {
		t.Errorf("tools with keys = %d, want 2", len(tools))
	}
}

func TestCreateAgentAndContext(t *testing.T) {
	isolateCLI(t)
	startFakeOllama(t, "x")
	cfg := &CLIConfig{
		Provider:      "ollama",
		Model:         "m",
		SystemPrompt:  "sp",
		MaxIterations: 3,
		EnableMemory:  true,
		EnableTools:   true,
		EnableTracing: true,
		MCPServers:    []MCPServerConfig{{Name: "s", Type: "http", URL: "http://127.0.0.1:1/mcp"}},
		OrgID:         "org-1",
	}
	a := createAgent(cfg)
	if a.GetName() != "CLI-Agent" || a.GetSystemPrompt() != "sp" || a.GetMemory() == nil {
		t.Errorf("agent name=%q prompt=%q memory=%v", a.GetName(), a.GetSystemPrompt(), a.GetMemory())
	}
	if createTracer() != nil {
		t.Error("tracer created with Langfuse disabled")
	}

	ctx := createContext(&CLIConfig{OrgID: "org-1", ConversationID: "conv-1"})
	if org, _ := multitenancy.GetOrgID(ctx); org != "org-1" {
		t.Errorf("org = %q", org)
	}
	if conv, _ := memory.GetConversationID(ctx); conv != "conv-1" {
		t.Errorf("conversation = %q", conv)
	}
}

func TestInitConfig(t *testing.T) {
	isolateCLI(t)
	withStdin(t, "ollama\n\nCustom prompt\n")
	out := captureStdout(t, initConfig)
	if !strings.Contains(out, "Configuration saved to:") || !strings.Contains(out, "Make sure Ollama is running") {
		t.Errorf("output = %q", out)
	}
	cfg := loadConfig()
	if cfg.Provider != "ollama" || cfg.Model != "llama3.2:latest" || cfg.SystemPrompt != "Custom prompt" {
		t.Errorf("config = %+v", cfg)
	}

	// A second init asks before overwriting; anything but "y" cancels.
	withStdin(t, "n\n")
	if out := captureStdout(t, initConfig); !strings.Contains(out, "cancelled") {
		t.Errorf("second init output = %q", out)
	}

	tests := map[string]string{
		"openai":    "OPENAI_API_KEY",
		"anthropic": "ANTHROPIC_API_KEY",
		"vertex":    "GOOGLE_APPLICATION_CREDENTIALS",
		"vllm":      "vLLM server",
	}
	configFile := filepath.Join(getConfigDir(), "config.json")
	for provider, hint := range tests {
		if err := os.Remove(configFile); err != nil {
			t.Fatalf("remove config: %v", err)
		}
		withStdin(t, provider+"\nmodel-x\n\n")
		out := captureStdout(t, initConfig)
		if !strings.Contains(out, hint) {
			t.Errorf("%s: output missing %q", provider, hint)
		}
		if cfg := loadConfig(); cfg.Model != "model-x" || cfg.SystemPrompt != "You are a helpful AI assistant." {
			t.Errorf("%s: config = %+v", provider, cfg)
		}
	}
}

func TestEvalLiveRunWithFakeProvider(t *testing.T) {
	isolateCLI(t)
	startFakeOllama(t, "hello")
	dir := t.TempDir()
	datasetPath := filepath.Join(dir, "dataset.json")
	configPath := filepath.Join(dir, "config.json")
	writeFile(t, datasetPath, `{"schema_version":"1","id":"live","cases":[{"id":"c1","input":"say hello","checks":[{"id":"exact","type":"output_exact","config":{"value":"hello"}}]}]}`)
	writeFile(t, configPath, `{"provider":"ollama","model":"m","org_id":"o","conversation_id":"c","enable_memory":true,"enable_tools":true}`)

	var stdout, stderr bytes.Buffer
	code := runEvalCommand([]string{"--dataset", datasetPath, "--config", configPath, "--judge-provider", "ollama", "--judge-model", "judge"}, &stdout, &stderr)
	if code != evalExitPass {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "1 passed, 0 failed") {
		t.Errorf("stderr = %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"dataset_id"`) && !strings.Contains(stdout.String(), "live") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestEvaluationBuildRevision(t *testing.T) {
	if got := evaluationBuildRevision(); got == "" {
		t.Error("empty build revision")
	}
}
