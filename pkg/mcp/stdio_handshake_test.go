package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// legacyServerEnv makes the test binary act as an MCP server that predates
// the server/discover probe of protocol 2026-07-28. It speaks raw JSON-RPC
// over stdio, and its value picks how it reacts to the probe:
//
//   - "ignore": the probe goes unanswered, and every later message is served
//     as usual.
//   - "die": like Python servers built on older mcp releases (1.0.0, for
//     one), the session dies on the unknown method. The process answers
//     nothing more and exits as soon as it reads another line.
const legacyServerEnv = "AGENT_SDK_GO_MCP_TEST_LEGACY_SERVER"

// legacyStartsEnv names a file the legacy server appends a line to each time
// it starts, so tests can count how many processes a handshake used.
const legacyStartsEnv = "AGENT_SDK_GO_MCP_TEST_LEGACY_STARTS"

// legacyServerProtocol is what the legacy server answers initialize with,
// whatever the client asked for, as Python mcp 1.0.0 does.
const legacyServerProtocol = "2024-11-05"

// runLegacyStdioServer serves the legacy MCP server on stdin and stdout and
// returns the process exit code.
func runLegacyStdioServer(mode string) int {
	if path := os.Getenv(legacyStartsEnv); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = f.WriteString("start\n")
			_ = f.Close()
		}
	}

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 1<<20)
	out := json.NewEncoder(os.Stdout)
	dead := false
	for in.Scan() {
		if dead {
			return 1
		}
		var msg struct {
			ID     json.RawMessage `json:"id,omitempty"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal(in.Bytes(), &msg); err != nil {
			continue
		}
		reply := func(result any) {
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
		}
		switch msg.Method {
		case "server/discover":
			// Unanswered in both modes.
			dead = mode == "die"
		case "initialize":
			reply(map[string]any{
				"protocolVersion": legacyServerProtocol,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "legacy-server", "version": "0.6.2"},
			})
		case "tools/list":
			reply(map[string]any{"tools": []any{map[string]any{
				"name":        "get_current_time",
				"description": "Get the current time",
				"inputSchema": map[string]any{"type": "object"},
			}}})
		case "ping":
			reply(map[string]any{})
		default:
			if len(msg.ID) > 0 {
				_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
					"error": map[string]any{"code": -32601, "message": "Method not found"}})
			}
		}
	}
	return 0
}

// legacyServerEnvVars returns the environment that turns a child test binary
// into the legacy server, with its starts counted in a fresh file. The second
// result reports how many times the server has started so far.
func legacyServerEnvVars(t *testing.T, mode string) ([]string, func() int) {
	t.Helper()
	starts := filepath.Join(t.TempDir(), "starts")
	count := func() int {
		data, err := os.ReadFile(starts)
		if err != nil {
			return 0
		}
		return strings.Count(string(data), "\n")
	}
	return []string{legacyServerEnv + "=" + mode, legacyStartsEnv + "=" + starts}, count
}

// legacyServerCommand returns a command that runs the legacy server.
func legacyServerCommand(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	env, _ := legacyServerEnvVars(t, mode)
	cmd := exec.CommandContext(t.Context(), os.Args[0])
	cmd.Env = append(os.Environ(), env...)
	return cmd
}

// TestSDKConnectWaitsOutSilentDiscover documents the go-sdk behavior that the
// stdio handshake works around. Client.Connect offers protocol 2026-07-28 and
// so starts with a server/discover probe. It falls back to initialize only
// when the server answers the probe with an error, so a server that never
// answers holds Connect until its context ends. If this test starts failing
// after a go-sdk upgrade, the SDK may now bound the probe itself.
func TestSDKConnectWaitsOutSilentDiscover(t *testing.T) {
	t.Run("default protocol waits out the context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 750*time.Millisecond)
		defer cancel()
		client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
		session, err := client.Connect(ctx, &mcp.CommandTransport{Command: legacyServerCommand(t, "ignore")}, nil)
		if err == nil {
			_ = session.Close()
		}
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("legacy protocol skips the probe", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
		session, err := client.Connect(ctx, &mcp.CommandTransport{Command: legacyServerCommand(t, "die")},
			&mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
		require.NoError(t, err)
		t.Cleanup(func() { _ = session.Close() })
		require.Equal(t, legacyServerProtocol, session.InitializeResult().ProtocolVersion)
	})
}

// negotiatedProtocol returns the protocol version a server connection settled on.
func negotiatedProtocol(t *testing.T, server interfaces.MCPServer) string {
	t.Helper()
	impl, ok := server.(*MCPServerImpl)
	require.True(t, ok, "server is %T, not *MCPServerImpl", server)
	return impl.session.InitializeResult().ProtocolVersion
}

func TestNewStdioServerLegacyServer(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		wantStarts int
	}{
		// The server ignores the probe but keeps serving, so initialize on
		// the same connection works.
		{"ignores discover", "ignore", 1},
		// The connection dies with the probe, so the server is started again.
		{"dies on discover", "die", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, starts := legacyServerEnvVars(t, tt.mode)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			begin := time.Now()
			server, err := NewStdioServer(ctx, StdioServerConfig{
				Command:         os.Args[0],
				Env:             env,
				Logger:          logging.New(),
				DiscoverTimeout: 200 * time.Millisecond,
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = server.Close() })

			// Well short of the context: the handshake did not wait it out.
			assert.Less(t, time.Since(begin), 10*time.Second)
			assert.Equal(t, tt.wantStarts, starts())
			assert.Equal(t, legacyServerProtocol, negotiatedProtocol(t, server))

			info, err := server.GetServerInfo()
			require.NoError(t, err)
			assert.Equal(t, "legacy-server", info.Name)

			tools, err := server.ListTools(ctx)
			require.NoError(t, err)
			require.Len(t, tools, 1)
			assert.Equal(t, "get_current_time", tools[0].Name)
		})
	}
}

func TestNewStdioServerModernServerKeepsNewestProtocol(t *testing.T) {
	server, err := NewStdioServer(t.Context(), StdioServerConfig{
		Command: os.Args[0],
		Env:     []string{stdioServerEnv + "=1"},
		Logger:  logging.New(),
		// Generous, so a slow machine does not push the server onto initialize.
		DiscoverTimeout: 30 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	assert.Equal(t, mcp.SupportedProtocolVersions()[0], negotiatedProtocol(t, server))
	tools, err := server.ListTools(t.Context())
	require.NoError(t, err)
	assert.Len(t, tools, 2)
}

func TestNewStdioServerPinnedLegacyProtocol(t *testing.T) {
	t.Run("legacy server is started once", func(t *testing.T) {
		env, starts := legacyServerEnvVars(t, "die")
		server, err := NewStdioServer(t.Context(), StdioServerConfig{
			Command:         os.Args[0],
			Env:             env,
			Logger:          logging.New(),
			ProtocolVersion: LegacyProtocolVersion,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = server.Close() })

		// No probe was sent, so the server never had to be restarted.
		assert.Equal(t, 1, starts())
		assert.Equal(t, legacyServerProtocol, negotiatedProtocol(t, server))
	})

	t.Run("modern server negotiates the pinned version", func(t *testing.T) {
		server, err := NewStdioServer(t.Context(), StdioServerConfig{
			Command:         os.Args[0],
			Env:             []string{stdioServerEnv + "=1"},
			Logger:          logging.New(),
			ProtocolVersion: LegacyProtocolVersion,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = server.Close() })

		assert.Equal(t, LegacyProtocolVersion, negotiatedProtocol(t, server))
	})
}

func TestNewStdioServerNoRestartAfterCallerDeadline(t *testing.T) {
	env, starts := legacyServerEnvVars(t, "die")
	// The caller's deadline ends the probe before DiscoverTimeout does, so
	// there is no time left for a second process.
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	server, err := NewStdioServer(ctx, StdioServerConfig{
		Command:         os.Args[0],
		Env:             env,
		Logger:          logging.New(),
		DiscoverTimeout: 30 * time.Second,
	})
	assert.Nil(t, server)
	require.Error(t, err)
	assert.Equal(t, 1, starts())
}

func TestNewStdioServerRejectsUnknownProtocolVersion(t *testing.T) {
	server, err := NewStdioServer(t.Context(), StdioServerConfig{
		Command:         os.Args[0],
		Logger:          logging.New(),
		ProtocolVersion: "1999-01-01",
	})
	assert.Nil(t, server)
	assert.ErrorContains(t, err, `unsupported MCP protocol version "1999-01-01"`)
}

func TestBoundDiscover(t *testing.T) {
	// waitForCtx is a handler for a peer that never answers.
	waitForCtx := func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	t.Run("probe runs out of its own time", func(t *testing.T) {
		var timedOut atomic.Bool
		h := boundDiscover(50*time.Millisecond, &timedOut)(waitForCtx)
		_, err := h(t.Context(), "server/discover", &mcp.DiscoverRequest{})
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.True(t, timedOut.Load())
	})

	t.Run("caller's context ends first", func(t *testing.T) {
		var timedOut atomic.Bool
		h := boundDiscover(time.Minute, &timedOut)(waitForCtx)
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		_, err := h(ctx, "server/discover", &mcp.DiscoverRequest{})
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.False(t, timedOut.Load())
	})

	t.Run("answered probe", func(t *testing.T) {
		var timedOut atomic.Bool
		want := &mcp.DiscoverResult{}
		h := boundDiscover(time.Minute, &timedOut)(func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return want, nil
		})
		got, err := h(t.Context(), "server/discover", &mcp.DiscoverRequest{})
		require.NoError(t, err)
		assert.Same(t, want, got)
		assert.False(t, timedOut.Load())
	})

	t.Run("other requests are not bounded", func(t *testing.T) {
		var timedOut atomic.Bool
		h := boundDiscover(time.Nanosecond, &timedOut)(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
			_, hasDeadline := ctx.Deadline()
			assert.False(t, hasDeadline)
			return &mcp.InitializeResult{}, nil
		})
		_, err := h(t.Context(), "initialize", &mcp.InitializeRequest{})
		require.NoError(t, err)
		assert.False(t, timedOut.Load())
	})
}
