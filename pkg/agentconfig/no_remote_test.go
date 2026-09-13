package agentconfig

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPackagePerformsNoNetworkIO guards the removal of remote configuration
// loading.
//
// This package used to fetch agent YAML over HTTP from a configuration service
// and unmarshal it directly into an agent.AgentConfig. That config carries an
// MCP section naming a local executable, its argv and its environment, which
// pkg/mcp then passes to exec.CommandContext with cmd.Env inheriting the whole
// process environment. The only validation was that the binary path is
// absolute, exists, and is not a directory -- which /bin/sh satisfies. A
// compromised or hostile config server therefore had arbitrary local code
// execution inside the agent process at NewAgent time, with every API key in
// the environment handed to the child.
//
// The structural guarantee that closes that hole is that this package talks to
// no network at all. Asserting it structurally rather than behaviourally means
// the guard cannot be satisfied by a code path that merely happens not to fire
// during a test run.
func TestPackagePerformsNoNetworkIO(t *testing.T) {
	forbidden := []string{
		"net/http",
		"net/url",
		"github.com/go-resty/resty",
	}

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	var scanned int

	for _, entry := range entries {
		name := entry.Name()
		// Guard production code, not this test.
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		require.NoError(t, err, "parsing %s", name)
		scanned++

		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbidden {
				assert.NotEqual(t, bad, path,
					"%s imports %q: this package must not perform network I/O. "+
						"Remote config loading was removed because it gave the config "+
						"server arbitrary command execution through AgentConfig.MCP.",
					filepath.Base(name), path)
			}
		}
	}

	require.NotZero(t, scanned, "expected to scan at least one non-test source file")
}

// TestLoadAgentConfig_SourceIsAlwaysLocal asserts that a successfully loaded
// config is always attributed to a local file. Nothing can produce a config
// marked "remote" any more, which is what pkg/mcp would need to distinguish
// trusted from untrusted MCP server definitions.
func TestLoadAgentConfig_SourceIsAlwaysLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agents.yaml")

	// An MCP stdio server is exactly the payload that made remote loading
	// dangerous. Loading it from a local file the operator controls is fine.
	require.NoError(t, os.WriteFile(path, []byte(`
sentry-agent:
  role: "Site Reliability Engineer"
  goal: "Triage incidents"
  backstory: "Ten years on call"
  mcp:
    mcpServers:
      local-tools:
        command: "/usr/local/bin/mcp-tools"
        args: ["--stdio"]
`), 0o600))

	cfg, err := LoadAgentConfig(context.Background(), "sentry-agent", "production",
		WithLocalPath(path),
		WithoutCache(),
	)
	require.NoError(t, err)
	require.NotNil(t, cfg.ConfigSource)

	assert.Equal(t, string(ConfigSourceLocal), cfg.ConfigSource.Type)
	assert.Equal(t, "Site Reliability Engineer", cfg.Role)
	assert.Equal(t, "production", cfg.ConfigSource.Environment)

	// The MCP section still loads -- local configuration is unaffected by the
	// removal. It is the transport that was the problem, not the schema.
	require.NotNil(t, cfg.MCP)
	require.Contains(t, cfg.MCP.MCPServers, "local-tools")
	assert.Equal(t, "/usr/local/bin/mcp-tools", cfg.MCP.MCPServers["local-tools"].Command)
}

// TestLoadAgentConfig_MissingFileFailsClosed asserts the loader reports a
// missing local config rather than reaching for a fallback source.
func TestLoadAgentConfig_MissingFileFailsClosed(t *testing.T) {
	_, err := LoadAgentConfig(context.Background(), "does-not-exist", "production",
		WithLocalPath(filepath.Join(t.TempDir(), "absent.yaml")),
		WithoutCache(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load agent config")
}

// TestLoadAgentConfig_NoDeploymentIDRequired asserts the loader no longer
// demands AGENT_DEPLOYMENT_ID. That variable existed solely to address a record
// in the remote configuration service, and requiring it made purely local
// loading impossible without it.
func TestLoadAgentConfig_NoDeploymentIDRequired(t *testing.T) {
	t.Setenv("AGENT_DEPLOYMENT_ID", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "agents.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
offline-agent:
  role: "Analyst"
  goal: "Summarise"
`), 0o600))

	cfg, err := LoadAgentConfig(context.Background(), "offline-agent", "staging",
		WithLocalPath(path),
		WithoutCache(),
	)
	require.NoError(t, err)
	assert.Equal(t, "Analyst", cfg.Role)
}
