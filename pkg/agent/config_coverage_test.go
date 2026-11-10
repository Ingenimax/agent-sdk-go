package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"gopkg.in/yaml.v3"
)

func ptr[T any](v T) *T { return &v }

// writeFile writes content to name inside dir and returns the full path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const twoAgentsYAML = `
writer:
  role: Writer for {topic}
  goal: Write
  backstory: Born to write
reviewer:
  role: Reviewer
  goal: Review
  backstory: Born to review
`

const tasksYAML = `
draft:
  description: Draft about {topic}
  expected_output: A draft
  agent: writer
review:
  description: Review it
  expected_output: Notes
  agent: reviewer
  response_format:
    type: json_object
    schema_name: Review
    schema_definition:
      type: object
`

func TestLoadAgentAndTaskConfigsFromDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agents.yaml", twoAgentsYAML)
	writeFile(t, dir, "more.yml", "editor:\n  role: Editor\n")
	writeFile(t, dir, "notes.txt", "ignored")
	if err := os.Mkdir(filepath.Join(dir, "sub.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}

	agents, err := LoadAgentConfigsFromDir(dir)
	if err != nil {
		t.Fatalf("LoadAgentConfigsFromDir() error = %v", err)
	}
	for _, name := range []string{"writer", "reviewer", "editor"} {
		if _, ok := agents[name]; !ok {
			t.Errorf("agent %q not loaded; got %v", name, agents)
		}
	}

	taskDir := t.TempDir()
	writeFile(t, taskDir, "tasks.yaml", tasksYAML)
	writeFile(t, taskDir, "readme.md", "ignored")
	tasks, err := LoadTaskConfigsFromDir(taskDir)
	if err != nil {
		t.Fatalf("LoadTaskConfigsFromDir() error = %v", err)
	}
	if len(tasks) != 2 || tasks["review"].ResponseFormat == nil {
		t.Fatalf("tasks = %+v", tasks)
	}
}

func TestLoadConfigsFromDirErrors(t *testing.T) {
	file := writeFile(t, t.TempDir(), "a.yaml", "x: 1")
	bad := t.TempDir()
	writeFile(t, bad, "bad.yaml", "role: [unclosed")

	loaders := map[string]func(string) error{
		"agents": func(p string) error { _, err := LoadAgentConfigsFromDir(p); return err },
		"tasks":  func(p string) error { _, err := LoadTaskConfigsFromDir(p); return err },
	}
	for name, load := range loaders {
		t.Run(name, func(t *testing.T) {
			if err := load(filepath.Join(t.TempDir(), "missing")); err == nil || !strings.Contains(err.Error(), "failed to access directory") {
				t.Errorf("missing dir error = %v", err)
			}
			if err := load(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
				t.Errorf("file path error = %v", err)
			}
			if err := load(bad); err == nil || !strings.Contains(err.Error(), "bad.yaml") {
				t.Errorf("bad yaml error = %v", err)
			}
		})
	}
}

func TestLoadConfigFilesErrors(t *testing.T) {
	dir := t.TempDir()
	bad := writeFile(t, dir, "bad.yaml", "- just\n- a list\n")

	if _, err := LoadTaskConfigsFromFile(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Error("expected invalid path error for a missing task file")
	}
	if _, err := LoadTaskConfigsFromFile(bad); err == nil || !strings.Contains(err.Error(), "unmarshal task configs") {
		t.Errorf("task unmarshal error = %v", err)
	}
	if _, err := LoadAgentConfigsFromFile(bad); err == nil || !strings.Contains(err.Error(), "unmarshal agent configs") {
		t.Errorf("agent unmarshal error = %v", err)
	}
}

func TestIsValidFilePath(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "ok.yaml", "a: b")
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"empty", "", false},
		{"traversal", "../secret.yaml", false},
		{"proc", "/proc/self/status", false},
		{"missing", filepath.Join(dir, "nope.yaml"), false},
		{"directory", dir, false},
		{"regular file", file, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidFilePath(tc.path); got != tc.want {
				t.Errorf("isValidFilePath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestSaveConfigsToFileRoundTrip(t *testing.T) {
	dir := t.TempDir()

	agentFile, err := os.Create(filepath.Join(dir, "agents.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	agents := AgentConfigs{"a": {Role: "R", Goal: "G", Backstory: "B"}}
	if err := SaveAgentConfigsToFile(agents, agentFile); err != nil {
		t.Fatalf("SaveAgentConfigsToFile() error = %v", err)
	}
	if err := agentFile.Close(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAgentConfigsFromFile(agentFile.Name())
	if err != nil || loaded["a"].Role != "R" {
		t.Fatalf("round trip = %+v, %v", loaded, err)
	}

	taskFile, err := os.Create(filepath.Join(dir, "tasks.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tasks := TaskConfigs{"t": {Description: "D", Agent: "a"}}
	if err := SaveTaskConfigsToFile(tasks, taskFile); err != nil {
		t.Fatalf("SaveTaskConfigsToFile() error = %v", err)
	}
	if err := taskFile.Close(); err != nil {
		t.Fatal(err)
	}
	loadedTasks, err := LoadTaskConfigsFromFile(taskFile.Name())
	if err != nil || loadedTasks["t"].Agent != "a" {
		t.Fatalf("round trip = %+v, %v", loadedTasks, err)
	}

	// Writing to a closed file fails.
	if err := SaveAgentConfigsToFile(agents, agentFile); err == nil {
		t.Error("expected write error on closed agent file")
	}
	if err := SaveTaskConfigsToFile(tasks, taskFile); err == nil {
		t.Error("expected write error on closed task file")
	}
}

func TestGenerateConfigFromSystemPromptPaths(t *testing.T) {
	tests := []struct {
		name     string
		response string
		err      error
		wantErr  string
		wantRole string
	}{
		{"llm error", "", errors.New("down"), "failed to generate configurations", ""},
		{"no agent block", "- just\n- a list\n", nil, "failed to find agent configuration", ""},
		{
			name:     "prose and fence around yaml",
			response: "Here you go:\n```yaml\nagent:\n  role: Helper\ntasks:\n  t1:\n    description: Do\n```\n",
			wantRole: "Helper",
		},
		{
			name:     "broken yaml after prose",
			response: "Sure: agent: [\n```\n",
			wantErr:  "failed to parse generated configurations",
		},
		{
			name:     "plain yaml",
			response: "agent:\n  role: Helper\ntasks:\n  t1:\n    description: Do\n",
			wantRole: "Helper",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			llm := &testutil.FakeLLM{DefaultResponse: tc.response, Err: tc.err}
			agentCfg, tasks, err := GenerateConfigFromSystemPrompt(context.Background(), llm, "be helpful")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(agentCfg.Role) != tc.wantRole || len(tasks) != 1 || tasks[0].Agent != "t1" {
				t.Fatalf("got %+v, %+v", agentCfg, tasks)
			}
		})
	}
}

func TestExpandWithConfigVarsPriority(t *testing.T) {
	t.Setenv("COV_FROM_ENV", "env")
	envVarCache["COV_FROM_CACHE"] = "cache"
	t.Cleanup(func() { delete(envVarCache, "COV_FROM_CACHE") })

	vars := map[string]string{"COV_FROM_VARS": "vars", "COV_FROM_ENV": "vars-wins"}
	got := expandWithConfigVars("${COV_FROM_VARS}/${COV_FROM_ENV}/${COV_FROM_CACHE}/${COV_MISSING}", vars)
	if got != "vars/vars-wins/cache/" {
		t.Fatalf("with vars = %q", got)
	}

	got = expandWithConfigVars("${COV_FROM_ENV}/${COV_FROM_CACHE}", nil)
	if got != "env/cache" {
		t.Fatalf("without vars = %q", got)
	}
}

func TestExpandEnvironmentVariablesTypes(t *testing.T) {
	t.Setenv("COV_X", "x")
	in := map[string]interface{}{
		"s":    "${COV_X}",
		"list": []interface{}{"${COV_X}", 3},
		"nest": map[string]interface{}{"k": "$COV_X"},
		"n":    7,
	}
	got := expandConfigMap(in, nil)
	if got["s"] != "x" || got["n"] != 7 {
		t.Fatalf("scalars = %+v", got)
	}
	list := got["list"].([]interface{})
	if list[0] != "x" || list[1] != 3 {
		t.Fatalf("list = %+v", list)
	}
	if got["nest"].(map[string]interface{})["k"] != "x" {
		t.Fatalf("nest = %+v", got["nest"])
	}
}

func TestExpandAgentConfig(t *testing.T) {
	vars := map[string]string{"V": "val"}
	cfg := AgentConfig{
		Role:         "role-${V}",
		Goal:         "goal-${V}",
		Backstory:    "back-${V}",
		ConfigSource: &ConfigSourceMetadata{Variables: vars},
		Memory:       &MemoryConfigYAML{Type: "buffer", Config: map[string]interface{}{"k": "${V}"}},
		Tools: []ToolConfigYAML{{
			Type: "agent", Name: "t", Description: "d-${V}", URL: "u-${V}", Timeout: "${V}",
			Config: map[string]interface{}{"c": "${V}"},
		}},
		Runtime: &RuntimeConfigYAML{LogLevel: "${V}", TimeoutDuration: "t-${V}", EnableTracing: ptr(true)},
		SubAgents: map[string]AgentConfig{
			"child": {Role: "child-${V}"},
		},
		LLMProvider: &LLMProviderYAML{Provider: "p-${V}", Model: "m-${V}", Config: map[string]interface{}{"k": "${V}"}},
		ImageGeneration: &ImageGenerationYAML{
			Provider: "p-${V}",
			Model:    "m-${V}",
			Config:   map[string]interface{}{"k": "${V}"},
			Storage: &ImageStorageYAML{
				Type:  "s-${V}",
				Local: &LocalStorageYAML{Path: "p-${V}", BaseURL: "b-${V}"},
				GCS: &GCSStorageYAML{
					Bucket: "b-${V}", Prefix: "p-${V}", CredentialsFile: "f-${V}",
					CredentialsJSON: "j-${V}", SignedURLExpiration: "e-${V}",
				},
			},
			MultiTurnEditing: &MultiTurnEditingYAML{Model: "mt-${V}", SessionTimeout: "st-${V}", MaxSessionsPerOrg: ptr(2)},
		},
		MCP: &MCPConfiguration{
			MCPServers: map[string]MCPServerConfig{
				"srv": {Command: "c-${V}", Args: []string{"a-${V}"}, Env: map[string]string{"E": "${V}"}, URL: "u-${V}", Token: "t-${V}"},
			},
		},
	}

	got := ExpandAgentConfig(cfg)

	checks := map[string][2]string{
		"role":          {got.Role, "role-val"},
		"goal":          {got.Goal, "goal-val"},
		"backstory":     {got.Backstory, "back-val"},
		"memory":        {got.Memory.Config["k"].(string), "val"},
		"tool desc":     {got.Tools[0].Description, "d-val"},
		"tool url":      {got.Tools[0].URL, "u-val"},
		"tool timeout":  {got.Tools[0].Timeout, "val"},
		"tool config":   {got.Tools[0].Config["c"].(string), "val"},
		"runtime level": {got.Runtime.LogLevel, "val"},
		"runtime t/o":   {got.Runtime.TimeoutDuration, "t-val"},
		"sub agent":     {got.SubAgents["child"].Role, "child-val"},
		"llm provider":  {got.LLMProvider.Provider, "p-val"},
		"llm model":     {got.LLMProvider.Model, "m-val"},
		"llm config":    {got.LLMProvider.Config["k"].(string), "val"},
		"img provider":  {got.ImageGeneration.Provider, "p-val"},
		"img storage":   {got.ImageGeneration.Storage.Type, "s-val"},
		"img local":     {got.ImageGeneration.Storage.Local.BaseURL, "b-val"},
		"img gcs":       {got.ImageGeneration.Storage.GCS.SignedURLExpiration, "e-val"},
		"img mt model":  {got.ImageGeneration.MultiTurnEditing.SessionTimeout, "st-val"},
		"mcp command":   {got.MCP.MCPServers["srv"].Command, "c-val"},
		"mcp arg":       {got.MCP.MCPServers["srv"].Args[0], "a-val"},
		"mcp env":       {got.MCP.MCPServers["srv"].Env["E"], "val"},
		"mcp token":     {got.MCP.MCPServers["srv"].Token, "t-val"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
	if !*got.Runtime.EnableTracing || *got.ImageGeneration.MultiTurnEditing.MaxSessionsPerOrg != 2 {
		t.Error("pointer fields were not carried over")
	}
	// The input must not be mutated.
	if cfg.Role != "role-${V}" {
		t.Errorf("input mutated: %q", cfg.Role)
	}
}

func TestConvertYAMLConfigsToInterface(t *testing.T) {
	if convertStreamConfigYAMLToInterface(nil) != nil ||
		convertCacheConfigYAMLToInterface(nil) != nil ||
		convertLLMConfigYAMLToInterface(nil) != nil ||
		convertMemoryConfigYAMLToInterface(nil) != nil {
		t.Fatal("nil input must convert to nil")
	}

	stream := convertStreamConfigYAMLToInterface(&StreamConfigYAML{
		BufferSize: ptr(5), IncludeToolProgress: ptr(true), IncludeIntermediateMessages: ptr(true),
	})
	if *stream != (interfaces.StreamConfig{BufferSize: 5, IncludeToolProgress: true, IncludeIntermediateMessages: true}) {
		t.Errorf("stream = %+v", *stream)
	}

	cache := convertCacheConfigYAMLToInterface(&CacheConfigYAML{
		CacheSystemMessage: ptr(true), CacheTools: ptr(true), CacheConversation: ptr(true),
	})
	if *cache != (interfaces.CacheConfig{CacheSystemMessage: true, CacheTools: true, CacheConversation: true}) {
		t.Errorf("cache = %+v", *cache)
	}

	llm := convertLLMConfigYAMLToInterface(&LLMConfigYAML{
		Temperature: ptr(0.1), TopP: ptr(0.2), FrequencyPenalty: ptr(0.3), PresencePenalty: ptr(0.4),
		StopSequences: []string{"x"}, EnableReasoning: ptr(true), ReasoningBudget: ptr(2048),
		Reasoning: ptr("high"), MaxTokens: ptr(99),
	})
	if llm.Temperature != 0.1 || llm.TopP != 0.2 || llm.FrequencyPenalty != 0.3 || llm.PresencePenalty != 0.4 ||
		len(llm.StopSequences) != 1 || !llm.EnableReasoning || llm.ReasoningBudget != 2048 ||
		llm.Reasoning != "high" || llm.MaxTokens != 99 {
		t.Errorf("llm = %+v", *llm)
	}

	mem := convertMemoryConfigYAMLToInterface(&MemoryConfigYAML{Type: "redis", Config: map[string]interface{}{"address": "x"}})
	if mem["type"] != "redis" || mem["address"] != "x" {
		t.Errorf("memory = %+v", mem)
	}
	if got := convertMemoryConfigYAMLToInterface(&MemoryConfigYAML{Type: "buffer"}); len(got) != 1 {
		t.Errorf("memory without config = %+v", got)
	}
}

func TestEnvFileLoading(t *testing.T) {
	t.Setenv("COV_ALREADY_SET", "real")
	path := writeFile(t, t.TempDir(), ".env", strings.Join([]string{
		"# comment",
		"",
		"COV_PLAIN=plain",
		`COV_QUOTED="quoted value"`,
		"COV_SINGLE='single'",
		"COV_ALREADY_SET=from-file",
		"not a pair",
	}, "\n"))
	t.Cleanup(func() {
		for _, k := range []string{"COV_PLAIN", "COV_QUOTED", "COV_SINGLE", "COV_ALREADY_SET"} {
			delete(envVarCache, k)
		}
	})

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile() error = %v", err)
	}
	tests := map[string]string{
		"COV_PLAIN":       "plain",
		"COV_QUOTED":      "quoted value",
		"COV_SINGLE":      "single",
		"COV_ALREADY_SET": "real",
		"COV_NOT_THERE":   "",
	}
	for key, want := range tests {
		if got := GetEnvValue(key); got != want {
			t.Errorf("GetEnvValue(%q) = %q, want %q", key, got, want)
		}
	}
	if got := ExpandEnv("${COV_PLAIN}-${COV_ALREADY_SET}-${COV_NOT_THERE}"); got != "plain-real-" {
		t.Errorf("ExpandEnv() = %q", got)
	}

	// A missing file is not an error, but a directory is.
	if err := LoadEnvFile(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Errorf("missing file error = %v", err)
	}
	if err := LoadEnvFile(t.TempDir()); err == nil {
		t.Error("expected an error reading a directory")
	}
}

func TestFactoryRedirectErrors(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		fn   func(context.Context, string, string, ...Option) (*Agent, error)
		want string
	}{
		{"dual", NewAgentWithDualConfig, "agentconfig.LoadAgentAuto"},
		{"remote", NewAgentFromRemoteConfig, "agentconfig.LoadAgentFromRemote"},
		{"local", NewAgentFromLocalConfig, "agentconfig.LoadAgentFromLocal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, err := tc.fn(ctx, "bot", "dev")
			if a != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, %v", a, err)
			}
		})
	}
}

func TestNewAgentFromFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	single := writeFile(t, dir, "single.yaml", "solo:\n  role: Solo\n  goal: G\n  backstory: B\n")
	multi := writeFile(t, dir, "multi.yaml", twoAgentsYAML)
	llmOpt := WithLLM(testutil.NewFakeLLM())

	a, err := NewAgentFromFile(ctx, single, llmOpt)
	if err != nil {
		t.Fatalf("NewAgentFromFile(single) error = %v", err)
	}
	if !strings.Contains(a.GetSystemPrompt(), "Solo") || a.GetName() != "agent" {
		t.Errorf("agent = %q / %q", a.GetName(), a.GetSystemPrompt())
	}

	if _, err := NewAgentFromFile(ctx, multi, llmOpt); err == nil || !strings.Contains(err.Error(), "contains 2 agent configs") {
		t.Errorf("multi error = %v", err)
	}
	if _, err := NewAgentFromFile(ctx, filepath.Join(dir, "nope.yaml"), llmOpt); err == nil {
		t.Error("expected error for a missing file")
	}

	r, err := NewAgentFromFileWithName(ctx, multi, "reviewer", llmOpt)
	if err != nil || !strings.Contains(r.GetSystemPrompt(), "Reviewer") {
		t.Fatalf("NewAgentFromFileWithName() = %v, %v", r, err)
	}
	if _, err := NewAgentFromFileWithName(ctx, multi, "ghost", llmOpt); err == nil || !strings.Contains(err.Error(), "ghost not found") {
		t.Errorf("missing agent error = %v", err)
	}
	if _, err := NewAgentFromFileWithName(ctx, filepath.Join(dir, "nope.yaml"), "x", llmOpt); err == nil {
		t.Error("expected error for a missing file")
	}

	// NewAgentFromCreateConfig delegates to the file loader when a path is given.
	c, err := NewAgentFromCreateConfig(ctx, CreateAgentConfig{AgentName: "writer", ConfigPath: multi, CustomOptions: []Option{llmOpt}})
	if err != nil || !strings.Contains(c.GetSystemPrompt(), "Writer") {
		t.Fatalf("NewAgentFromCreateConfig() = %v, %v", c, err)
	}
	if _, err := NewAgentFromCreateConfig(ctx, CreateAgentConfig{}); err == nil || !strings.Contains(err.Error(), "AgentName is required") {
		t.Errorf("empty name error = %v", err)
	}
	if _, err := NewAgentFromCreateConfig(ctx, CreateAgentConfig{AgentName: "x", Environment: "dev"}); err == nil || !strings.Contains(err.Error(), "agentconfig.LoadAgentAuto") {
		t.Errorf("remote guidance error = %v", err)
	}
}

func TestValidateConfigPath(t *testing.T) {
	file := writeFile(t, t.TempDir(), "c.yaml", "a: b")
	if err := ValidateConfigPath(file); err != nil {
		t.Errorf("valid path error = %v", err)
	}
	if err := ValidateConfigPath(""); err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("empty path error = %v", err)
	}
	if err := ValidateConfigPath("../x.yaml"); err == nil || !strings.Contains(err.Error(), "invalid or unsafe") {
		t.Errorf("unsafe path error = %v", err)
	}
}

func TestWithConfigFile(t *testing.T) {
	path := writeFile(t, t.TempDir(), "agents.yaml", twoAgentsYAML)

	a := &Agent{}
	WithConfigFile(path, "writer")(a)
	if !strings.Contains(a.systemPrompt, "Writer") {
		t.Errorf("system prompt = %q", a.systemPrompt)
	}

	for name, apply := range map[string]Option{
		"missing file":  WithConfigFile(path+".missing", "writer"),
		"missing agent": WithConfigFile(path, "ghost"),
	} {
		b := &Agent{}
		apply(b)
		if b.systemPrompt != "" || b.generatedAgentConfig != nil {
			t.Errorf("%s: option should be a no-op, got %q", name, b.systemPrompt)
		}
	}
}

func TestAgentCreationError(t *testing.T) {
	inner := errors.New("boom")
	err := NewAgentCreationError("bot", "file.yaml", inner)
	if err.Error() != "failed to create agent bot from file.yaml: boom" {
		t.Errorf("Error() = %q", err.Error())
	}
	if !errors.Is(err, inner) {
		t.Error("errors.Is should see the wrapped error")
	}
	var ace *AgentCreationError
	if !errors.As(err, &ace) || ace.AgentName != "bot" {
		t.Errorf("errors.As = %+v", ace)
	}
}

func TestNewAgentFromConfigAndCreateAgentForTask(t *testing.T) {
	var agents AgentConfigs
	if err := yaml.Unmarshal([]byte(twoAgentsYAML), &agents); err != nil {
		t.Fatal(err)
	}
	var tasks TaskConfigs
	if err := yaml.Unmarshal([]byte(tasksYAML), &tasks); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"topic": "Go"}
	llmOpt := WithLLM(testutil.NewFakeLLM())

	a, err := NewAgentFromConfig("writer", agents, vars, llmOpt)
	if err != nil {
		t.Fatal(err)
	}
	if a.GetName() != "writer" || !strings.Contains(a.GetSystemPrompt(), "Writer for Go") {
		t.Errorf("agent = %q / %q", a.GetName(), a.GetSystemPrompt())
	}
	if a.GetConfig().Role != agents["writer"].Role {
		t.Errorf("GetConfig().Role = %q", a.GetConfig().Role)
	}

	if _, err := NewAgentFromConfig("ghost", agents, vars, llmOpt); err == nil {
		t.Error("expected error for unknown agent")
	}

	// The review task carries its own response format.
	r, err := CreateAgentForTask("review", agents, tasks, vars, llmOpt)
	if err != nil {
		t.Fatal(err)
	}
	if r.responseFormat == nil || r.responseFormat.Name != "Review" {
		t.Errorf("response format = %+v", r.responseFormat)
	}
	if _, err := CreateAgentForTask("ghost", agents, tasks, vars, llmOpt); err == nil {
		t.Error("expected error for unknown task")
	}
}

func TestNewAgentFromConfigObject(t *testing.T) {
	ctx := context.Background()
	if _, err := NewAgentFromConfigObject(ctx, nil, nil); err == nil {
		t.Fatal("expected error for nil config")
	}
	cfg := &AgentConfig{Role: "R", ConfigSource: &ConfigSourceMetadata{AgentName: "named"}}
	a, err := NewAgentFromConfigObject(ctx, cfg, nil, WithLLM(testutil.NewFakeLLM()))
	if err != nil {
		t.Fatal(err)
	}
	if a.GetName() != "named" {
		t.Errorf("name = %q", a.GetName())
	}
}

func TestWithAgentConfigAppliesSettings(t *testing.T) {
	cfg := AgentConfig{
		Role: "R",
		ResponseFormat: &ResponseFormatConfig{
			Type: "json_object", SchemaName: "Out", SchemaDefinition: map[string]interface{}{"type": "object"},
		},
		MaxIterations:       ptr(7),
		DisableFinalSummary: ptr(true),
		RequirePlanApproval: ptr(false),
		StreamConfig:        &StreamConfigYAML{BufferSize: ptr(3)},
		LLMConfig:           &LLMConfigYAML{Temperature: ptr(0.5)},
		CacheConfig:         &CacheConfigYAML{CacheTools: ptr(true)},
		Tools: []ToolConfigYAML{
			{Type: "builtin", Name: "calculator"},
			{Type: "builtin", Name: "calculator"},
			{Type: "builtin", Name: "disabled", Enabled: ptr(false)},
			{Type: "bogus", Name: "broken"},
		},
		Memory: &MemoryConfigYAML{Type: "buffer"},
		Runtime: &RuntimeConfigYAML{
			TimeoutDuration: "45s", EnableTracing: ptr(true), EnableMetrics: ptr(true),
		},
		ImageGeneration: &ImageGenerationYAML{Enabled: ptr(false)},
	}

	a := &Agent{logger: logging.New(), llm: testutil.NewFakeLLM()}
	WithAgentConfig(cfg, nil)(a)

	if a.responseFormat == nil || a.responseFormat.Name != "Out" {
		t.Errorf("response format = %+v", a.responseFormat)
	}
	if a.maxIterations != 7 || !a.disableFinalSummary || a.requirePlanApproval {
		t.Errorf("behaviour = %d %v %v", a.maxIterations, a.disableFinalSummary, a.requirePlanApproval)
	}
	if a.streamConfig.BufferSize != 3 || a.llmConfig.Temperature != 0.5 || !a.cacheConfig.CacheTools {
		t.Error("complex config objects not applied")
	}
	if len(a.tools) != 1 || a.tools[0].Name() != "calculator" {
		t.Errorf("tools = %v", toolNames(a.tools))
	}
	if a.memoryConfig["type"] != "buffer" {
		t.Errorf("memory config = %+v", a.memoryConfig)
	}
	if a.timeout != 45*time.Second || !a.tracingEnabled || !a.metricsEnabled {
		t.Errorf("runtime = %v %v %v", a.timeout, a.tracingEnabled, a.metricsEnabled)
	}
	if a.GetConfig() == nil || a.GetConfig().Role != "R" {
		t.Error("expanded config not stored")
	}
}

func TestWithAgentConfigLLMProvider(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "")
	t.Setenv("OLLAMA_MODEL", "")

	provided := testutil.NewFakeLLM()
	tests := []struct {
		name     string
		existing interfaces.LLM
		provider *LLMProviderYAML
		check    func(*testing.T, interfaces.LLM)
	}{
		{"creates from yaml", nil, &LLMProviderYAML{Provider: "ollama", Model: "llama3"}, func(t *testing.T, l interfaces.LLM) {
			if l == nil || l.Name() != "ollama" {
				t.Errorf("llm = %v", l)
			}
		}},
		{"keeps programmatic llm", provided, &LLMProviderYAML{Provider: "ollama"}, func(t *testing.T, l interfaces.LLM) {
			if l != provided {
				t.Errorf("llm replaced: %v", l)
			}
		}},
		{"bad provider leaves llm unset", nil, &LLMProviderYAML{Provider: "nope"}, func(t *testing.T, l interfaces.LLM) {
			if l != nil {
				t.Errorf("llm = %v", l)
			}
		}},
		{"no provider", nil, nil, func(t *testing.T, l interfaces.LLM) {
			if l != nil {
				t.Errorf("llm = %v", l)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{logger: logging.New(), llm: tc.existing}
			WithAgentConfig(AgentConfig{Role: "r", LLMProvider: tc.provider}, nil)(a)
			tc.check(t, a.llm)
		})
	}
}

func TestWithAgentConfigSubAgents(t *testing.T) {
	llm := testutil.NewFakeLLM()
	mem := &testutil.FakeMemory{}
	tracer := &testutil.FakeTracer{}
	cfg := AgentConfig{
		Role:         "parent",
		ConfigSource: &ConfigSourceMetadata{Variables: map[string]string{"who": "source"}},
		SubAgents: map[string]AgentConfig{
			"helper": {Role: "helper for {who}", RequirePlanApproval: ptr(false)},
		},
	}
	a := &Agent{logger: logging.New(), llm: llm, memory: mem, tracer: tracer}
	WithAgentConfig(cfg, map[string]string{"who": "env", "extra": "x"})(a)

	if len(a.subAgents) != 1 {
		t.Fatalf("sub-agents = %d", len(a.subAgents))
	}
	sub := a.subAgents[0]
	if sub.GetName() != "helper" || sub.GetLLM() != llm || sub.GetMemory() != mem || sub.GetTracer() != tracer {
		t.Errorf("sub-agent did not inherit infrastructure: %+v", sub)
	}
	if !strings.Contains(sub.GetSystemPrompt(), "helper for source") {
		t.Errorf("config source variables should win: %q", sub.GetSystemPrompt())
	}
	if len(a.tools) != 1 || a.tools[0].Name() != "helper_agent" {
		t.Errorf("tools = %v", toolNames(a.tools))
	}
}

func TestCreateSubAgentsFromConfig(t *testing.T) {
	subs, err := createSubAgentsFromConfig(nil, nil, nil, nil, nil, nil)
	if subs != nil || err != nil {
		t.Fatalf("empty = %v, %v", subs, err)
	}

	// Without an LLM every sub-agent fails validation; the error lists them.
	subs, err = createSubAgentsFromConfig(map[string]AgentConfig{"a": {Role: "r"}}, nil, nil, nil, nil, logging.New())
	if len(subs) != 0 || err == nil || !strings.Contains(err.Error(), "sub-agent 'a'") {
		t.Fatalf("missing llm = %v, %v", subs, err)
	}

	// The parent records the failure but is still built.
	a := &Agent{logger: logging.New()}
	WithAgentConfig(AgentConfig{SubAgents: map[string]AgentConfig{"a": {Role: "r"}}}, nil)(a)
	if len(a.subAgents) != 0 {
		t.Errorf("sub-agents = %d", len(a.subAgents))
	}
}

func TestNewAgentCreatesMemoryFromConfig(t *testing.T) {
	a, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithAgentConfig(AgentConfig{Role: "r", Memory: &MemoryConfigYAML{Type: "buffer"}}, nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.GetMemory().(*memory.ConversationBuffer); !ok {
		t.Errorf("memory = %T", a.GetMemory())
	}
	if a.GetMemoryConfig()["type"] != "buffer" {
		t.Errorf("memory config = %+v", a.GetMemoryConfig())
	}

	// An unsupported memory type is logged and the agent is still created.
	b, err := NewAgent(
		WithLLM(testutil.NewFakeLLM()),
		WithAgentConfig(AgentConfig{Role: "r", Memory: &MemoryConfigYAML{Type: "nope"}}, nil),
	)
	if err != nil || b.GetMemory() != nil {
		t.Fatalf("got %v, %v", b.GetMemory(), err)
	}

	if _, err := CreateMemoryFromConfig(nil, nil); err == nil {
		t.Error("expected error for nil memory config")
	}
}

func TestToolFactory(t *testing.T) {
	tf := NewToolFactory()

	calc, err := tf.CreateTool(ToolConfigYAML{Type: "builtin", Name: "calculator"})
	if err != nil || calc.Name() != "calculator" {
		t.Fatalf("calculator = %v, %v", calc, err)
	}

	custom := &testutil.FakeTool{ToolName: "mine"}
	var gotConfig map[string]interface{}
	tf.RegisterCustomTool("mine", func(cfg map[string]interface{}) (interfaces.Tool, error) {
		gotConfig = cfg
		return custom, nil
	})
	tool, err := tf.CreateTool(ToolConfigYAML{Type: "custom", Name: "mine", Config: map[string]interface{}{"k": "v"}})
	if err != nil || tool != custom || gotConfig["k"] != "v" {
		t.Fatalf("custom = %v, %v, %v", tool, err, gotConfig)
	}

	errs := []struct {
		cfg  ToolConfigYAML
		want string
	}{
		{ToolConfigYAML{Type: "builtin", Name: "nope"}, "unknown builtin tool"},
		{ToolConfigYAML{Type: "custom", Name: "nope"}, "unknown custom tool"},
		{ToolConfigYAML{Type: "agent", Name: "a"}, "requires URL"},
		{ToolConfigYAML{Type: "mcp", Name: "m"}, "not implemented"},
		{ToolConfigYAML{Type: "weird"}, "unknown tool type"},
	}
	for _, e := range errs {
		if _, err := tf.CreateTool(e.cfg); err == nil || !strings.Contains(err.Error(), e.want) {
			t.Errorf("CreateTool(%+v) error = %v, want %q", e.cfg, err, e.want)
		}
	}
}

func TestAgentToolWrapperDelegates(t *testing.T) {
	inner, err := NewAgent(WithLLM(&testutil.FakeLLM{DefaultResponse: "inner says hi"}), WithName("inner"))
	if err != nil {
		t.Fatal(err)
	}
	w := &AgentToolWrapper{agent: inner, name: "inner_tool", description: "wraps inner"}
	if w.Name() != "inner_tool" || w.Description() != "wraps inner" {
		t.Errorf("name/description = %q / %q", w.Name(), w.Description())
	}
	if spec, ok := w.Parameters()["input"]; !ok || !spec.Required {
		t.Errorf("parameters = %+v", w.Parameters())
	}
	for name, call := range map[string]func(context.Context, string) (string, error){"Run": w.Run, "Execute": w.Execute} {
		got, err := call(context.Background(), "hello")
		if err != nil || got != "inner says hi" {
			t.Errorf("%s = %q, %v", name, got, err)
		}
	}
}

func TestLLMFactoryProviders(t *testing.T) {
	for _, key := range []string{
		"OPENAI_API_KEY", "OPENAI_MODEL", "DEEPSEEK_API_KEY", "DEEPSEEK_MODEL",
		"AZURE_OPENAI_API_KEY", "AZURE_OPENAI_ENDPOINT", "AZURE_OPENAI_DEPLOYMENT", "AZURE_OPENAI_API_VERSION",
		"GEMINI_API_KEY", "GEMINI_MODEL", "VERTEX_AI_PROJECT", "VERTEX_AI_REGION",
		"VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT", "OLLAMA_BASE_URL", "OLLAMA_MODEL",
		"VLLM_BASE_URL", "VLLM_MODEL",
	} {
		t.Setenv(key, "")
	}

	tests := []struct {
		name    string
		cfg     *LLMProviderYAML
		wantErr string
	}{
		{"nil", nil, "provider configuration is required"},
		{"unsupported", &LLMProviderYAML{Provider: "acme"}, "unsupported LLM provider"},
		{"openai missing key", &LLMProviderYAML{Provider: "openai"}, "api_key is required for OpenAI"},
		{"openai", &LLMProviderYAML{Provider: "OpenAI", Config: map[string]interface{}{"api_key": "k", "model": "gpt", "base_url": "http://127.0.0.1:1"}}, ""},
		{"deepseek missing key", &LLMProviderYAML{Provider: "deepseek"}, "api_key is required for DeepSeek"},
		{"deepseek", &LLMProviderYAML{Provider: "deepseek", Config: map[string]interface{}{"api_key": "k", "model": "m", "base_url": "http://127.0.0.1:1"}}, ""},
		{"azure missing key", &LLMProviderYAML{Provider: "azureopenai"}, "api_key is required for Azure"},
		{"azure missing endpoint", &LLMProviderYAML{Provider: "azure_openai", Config: map[string]interface{}{"api_key": "k"}}, "endpoint is required"},
		{"azure missing deployment", &LLMProviderYAML{Provider: "azureopenai", Config: map[string]interface{}{"api_key": "k", "endpoint": "http://127.0.0.1:1"}}, "deployment is required"},
		{"azure", &LLMProviderYAML{Provider: "azureopenai", Model: "dep", Config: map[string]interface{}{"api_key": "k", "endpoint": "http://127.0.0.1:1"}}, ""},
		{"gemini missing credentials", &LLMProviderYAML{Provider: "gemini"}, "credentials required for Gemini"},
		{"gemini api key", &LLMProviderYAML{Provider: "gemini", Config: map[string]interface{}{"api_key": "k", "model": "gemini-2.5-flash"}}, ""},
		{"gemini bad vertex credentials", &LLMProviderYAML{Provider: "gemini", Config: map[string]interface{}{"google_application_credentials": "not json", "project": "p"}}, "failed to parse Google credentials"},
		{"ollama", &LLMProviderYAML{Provider: "ollama", Config: map[string]interface{}{"model": "llama"}}, ""},
		{"vllm missing url", &LLMProviderYAML{Provider: "vllm"}, "base_url is required"},
		{"vllm", &LLMProviderYAML{Provider: "vllm", Config: map[string]interface{}{"base_url": "http://127.0.0.1:1", "model": "m"}}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			llm, err := createLLMFromConfig(tc.cfg)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || llm == nil {
				t.Fatalf("got %v, %v", llm, err)
			}
		})
	}
}

func TestGetConfigString(t *testing.T) {
	t.Setenv("COV_CFG", "expanded")
	cfg := map[string]interface{}{"s": "${COV_CFG}", "n": 3}
	if got := getConfigString(cfg, "s"); got != "expanded" {
		t.Errorf("string = %q", got)
	}
	if getConfigString(cfg, "n") != "" || getConfigString(cfg, "missing") != "" || getConfigString(nil, "s") != "" {
		t.Error("non-string, missing and nil lookups must return empty")
	}
}

func TestCreateImageGenerationToolFromConfig(t *testing.T) {
	for _, key := range []string{
		"GEMINI_API_KEY", "GOOGLE_API_KEY", "VERTEX_AI_PROJECT", "VERTEX_AI_REGION",
		"VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT",
	} {
		t.Setenv(key, "")
	}
	logger := logging.New()

	tool, err := createImageGenerationToolFromConfig(nil, logger)
	if tool != nil || err != nil {
		t.Fatalf("nil config = %v, %v", tool, err)
	}

	errCases := []struct {
		name string
		cfg  *ImageGenerationYAML
		want string
	}{
		{"unsupported provider", &ImageGenerationYAML{Provider: "dalle"}, "unsupported image generation provider"},
		{"no credentials", &ImageGenerationYAML{}, "credentials required for image generation"},
		{"bad vertex credentials", &ImageGenerationYAML{Config: map[string]interface{}{
			"google_application_credentials": "not json", "project_id": "p", "location": "eu",
		}}, "failed to parse Google credentials"},
		{"model without image support", &ImageGenerationYAML{Model: "gemini-2.5-flash", Config: map[string]interface{}{"api_key": "k"}}, "does not support image generation"},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := createImageGenerationToolFromConfig(tc.cfg, logger); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}

	cfg := &ImageGenerationYAML{
		Config: map[string]interface{}{
			"api_key":              "k",
			"max_prompt_length":    100,
			"default_aspect_ratio": "16:9",
			"default_format":       "jpeg",
		},
		Storage: &ImageStorageYAML{Type: "unknown"},
		MultiTurnEditing: &MultiTurnEditingYAML{
			Model:             "gemini-3-pro-image-preview",
			SessionTimeout:    "10m",
			MaxSessionsPerOrg: ptr(2),
		},
	}
	tool, err = createImageGenerationToolFromConfig(cfg, logger)
	if err != nil || tool == nil || tool.Name() != "generate_image" {
		t.Fatalf("tool = %v, %v", tool, err)
	}

	// Multi-turn without its own model falls back to the generation model.
	cfg.MultiTurnEditing = &MultiTurnEditingYAML{}
	if tool, err := createImageGenerationToolFromConfig(cfg, logger); err != nil || tool == nil {
		t.Fatalf("tool = %v, %v", tool, err)
	}
}

func TestWithAgentConfigImageGeneration(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("VERTEX_AI_PROJECT", "")

	a := &Agent{logger: logging.New()}
	WithAgentConfig(AgentConfig{ImageGeneration: &ImageGenerationYAML{
		Config:           map[string]interface{}{"api_key": "k"},
		MultiTurnEditing: &MultiTurnEditingYAML{Enabled: ptr(false)},
	}}, nil)(a)
	if len(a.tools) != 1 || a.tools[0].Name() != "generate_image" {
		t.Errorf("tools = %v", toolNames(a.tools))
	}

	// Missing credentials are logged, not fatal.
	b := &Agent{logger: logging.New()}
	WithAgentConfig(AgentConfig{ImageGeneration: &ImageGenerationYAML{}}, nil)(b)
	if len(b.tools) != 0 {
		t.Errorf("tools = %v", toolNames(b.tools))
	}
}

func TestCreateImageStorageFromConfig(t *testing.T) {
	if s, err := createImageStorageFromConfig(nil); s != nil || err != nil {
		t.Fatalf("nil = %v, %v", s, err)
	}

	dir := t.TempDir()
	for name, cfg := range map[string]*ImageStorageYAML{
		"explicit local": {Type: "local", Local: &LocalStorageYAML{Path: dir, BaseURL: "http://x"}},
		"inferred local": {Local: &LocalStorageYAML{Path: dir}},
	} {
		t.Run(name, func(t *testing.T) {
			s, err := createImageStorageFromConfig(cfg)
			if err != nil || s == nil {
				t.Fatalf("got %v, %v", s, err)
			}
		})
	}

	errCases := map[string]struct {
		cfg  *ImageStorageYAML
		want string
	}{
		"gcs without config":  {&ImageStorageYAML{Type: "gcs"}, "GCS storage configuration is required"},
		"gcs bad expiration":  {&ImageStorageYAML{GCS: &GCSStorageYAML{Bucket: "b", SignedURLExpiration: "soon"}}, "invalid signed_url_expiration"},
		"gcs not registered":  {&ImageStorageYAML{GCS: &GCSStorageYAML{Bucket: "b", SignedURLExpiration: "1h"}}, "GCS storage backend not registered"},
		"unsupported backend": {&ImageStorageYAML{Type: "s3"}, "unsupported storage type"},
	}
	for name, tc := range errCases {
		t.Run(name, func(t *testing.T) {
			if _, err := createImageStorageFromConfig(tc.cfg); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func toolNames(tools []interfaces.Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name()
	}
	return names
}
