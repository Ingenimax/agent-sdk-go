package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRegistry serves the registry API from a fixed list of servers and
// records the query string of every list request.
type fakeRegistry struct {
	servers []RegistryServer
	mu      sync.Mutex
	queries []url.Values
}

func (f *fakeRegistry) lastQuery() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries[len(f.queries)-1]
}

func (f *fakeRegistry) start(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		assert.Equal(t, "agent-sdk-go/1.0", r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/api/v1/servers" {
			f.mu.Lock()
			f.queries = append(f.queries, r.URL.Query())
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(SearchResponse{Servers: f.servers, Total: len(f.servers)})
			return
		}

		id, ok := strings.CutPrefix(r.URL.Path, "/api/v1/servers/")
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch id {
		case "broken":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
			return
		case "garbled":
			_, _ = w.Write([]byte("{"))
			return
		}
		for _, s := range f.servers {
			if s.ID == id {
				_ = json.NewEncoder(w).Encode(s)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func sampleServers() []RegistryServer {
	return []RegistryServer{
		{ID: "fs", Name: "Files", Tools: []RegistryTool{{Name: "read_file", Description: "Reads a file", Category: "filesystem"}}, Installation: InstallationInfo{Type: "npm", Command: "@x/fs", Args: []string{"/tmp"}}},
		{ID: "db", Name: "DB", Resources: []RegistryResource{{Type: "database", Description: "SQL rows"}}, Installation: InstallationInfo{Type: "pip", Command: "dbserver", Args: []string{"--ro"}}},
		{ID: "web", Name: "Web", Prompts: []RegistryPrompt{{Name: "browse", Description: "Browse the web", Category: "web"}}, Installation: InstallationInfo{Type: "docker", Command: "img/web", Args: []string{"--x"}}},
		{ID: "code", Name: "Code", Tags: []string{"code"}, Installation: InstallationInfo{Type: "stdio", Command: "/bin/code", Args: []string{"-v"}, Env: map[string]string{"A": "1", "B": "2"}}},
		{ID: "misc", Name: "Misc", Description: "Nothing relevant", Category: "misc", Installation: InstallationInfo{Type: "http", Command: "http://example.invalid/mcp"}},
		{ID: "odd", Name: "Odd", Installation: InstallationInfo{Type: "carrier-pigeon"}},
	}
}

func TestRegistryClientListServersQuery(t *testing.T) {
	reg := &fakeRegistry{servers: sampleServers()}
	client := NewRegistryClient(reg.start(t).URL + "/")

	resp, err := client.ListServers(t.Context(), &SearchOptions{
		Query: "files", Category: "dev", Author: "me", Verified: true,
		Limit: 5, Offset: 10, Tags: []string{"a", "b"},
	})
	require.NoError(t, err)
	assert.Len(t, resp.Servers, len(sampleServers()))
	assert.Equal(t, len(sampleServers()), resp.Total)
	assert.Equal(t, url.Values{
		"q": {"files"}, "category": {"dev"}, "author": {"me"}, "verified": {"true"},
		"limit": {"5"}, "offset": {"10"}, "tags": {"a,b"},
	}, reg.lastQuery())

	tests := []struct {
		name string
		call func() error
		want url.Values
	}{
		{"search", func() error { _, err := client.SearchServers(t.Context(), "git"); return err }, url.Values{"q": {"git"}, "limit": {"50"}}},
		{"category", func() error { _, err := client.GetServersByCategory(t.Context(), "dev"); return err }, url.Values{"category": {"dev"}, "limit": {"100"}}},
		{"verified", func() error { _, err := client.GetVerifiedServers(t.Context()); return err }, url.Values{"verified": {"true"}, "limit": {"100"}}},
		{"tags", func() error { _, err := client.GetServersByTags(t.Context(), []string{"x"}); return err }, url.Values{"tags": {"x"}, "limit": {"100"}}},
		{"empty options", func() error { _, err := client.ListServers(t.Context(), &SearchOptions{}); return err }, url.Values{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.call())
			assert.Equal(t, tt.want, reg.lastQuery())
		})
	}
}

func TestRegistryClientErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "garbled" {
			_, _ = w.Write([]byte("{"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("maintenance"))
	}))
	t.Cleanup(ts.Close)
	client := NewRegistryClient(ts.URL)

	_, err := client.SearchServers(t.Context(), "x")
	assert.ErrorContains(t, err, "HTTP 503, body: maintenance")
	_, err = client.SearchServers(t.Context(), "garbled")
	assert.ErrorContains(t, err, "failed to decode response")

	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	_, err = NewRegistryClient(closed.URL).SearchServers(t.Context(), "x")
	assert.ErrorContains(t, err, "request failed")
	_, err = NewRegistryClient(closed.URL).GetServer(t.Context(), "x")
	assert.ErrorContains(t, err, "request failed")

	_, err = NewRegistryClient("http://bad host").SearchServers(t.Context(), "x")
	assert.ErrorContains(t, err, "failed to create request")
	_, err = NewRegistryClient("http://bad host").GetServer(t.Context(), "x")
	assert.ErrorContains(t, err, "failed to create request")
}

func TestRegistryClientGetServer(t *testing.T) {
	reg := &fakeRegistry{servers: sampleServers()}
	client := NewRegistryClient(reg.start(t).URL)

	server, err := client.GetServer(t.Context(), "fs")
	require.NoError(t, err)
	assert.Equal(t, "Files", server.Name)

	tests := []struct {
		id      string
		wantErr string
	}{
		{"missing", "server not found: missing"},
		{"broken", "HTTP 500, body: boom"},
		{"garbled", "failed to decode response"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			_, err := client.GetServer(t.Context(), tt.id)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestRegistryManagerDiscoverAndInstall(t *testing.T) {
	reg := &fakeRegistry{servers: sampleServers()}
	manager := NewRegistryManager(reg.start(t).URL)

	tests := []struct {
		id   string
		want LazyMCPServerConfig
	}{
		{"fs", LazyMCPServerConfig{Name: "Files", Type: "stdio", Command: "npx", Args: []string{"@x/fs", "/tmp"}}},
		{"db", LazyMCPServerConfig{Name: "DB", Type: "stdio", Command: "python", Args: []string{"-m", "dbserver", "--ro"}}},
		{"web", LazyMCPServerConfig{Name: "Web", Type: "stdio", Command: "docker", Args: []string{"run", "--rm", "-i", "img/web", "--x"}}},
		{"code", LazyMCPServerConfig{Name: "Code", Type: "stdio", Command: "/bin/code", Args: []string{"-v"}, Env: []string{"A=1", "B=2"}}},
		{"misc", LazyMCPServerConfig{Name: "Misc", Type: "http", URL: "http://example.invalid/mcp"}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, err := manager.DiscoverAndInstallServer(t.Context(), tt.id)
			require.NoError(t, err)
			// Env comes from a map, so its order is not fixed.
			sort.Strings(got.Env)
			assert.Equal(t, tt.want, *got)
		})
	}

	_, err := manager.DiscoverAndInstallServer(t.Context(), "odd")
	assert.ErrorContains(t, err, "unsupported installation type: carrier-pigeon")
	_, err = manager.DiscoverAndInstallServer(t.Context(), "missing")
	assert.ErrorContains(t, err, "failed to discover server")
	_, err = manager.BuildServerFromRegistry(t.Context(), "missing")
	assert.ErrorContains(t, err, "failed to discover server")
}

func TestRegistryManagerDiscoverByCapability(t *testing.T) {
	reg := &fakeRegistry{servers: sampleServers()}
	manager := NewRegistryManager(reg.start(t).URL)

	ids := func(servers []*RegistryServer) []string {
		out := make([]string, 0, len(servers))
		for _, s := range servers {
			out = append(out, s.ID)
		}
		return out
	}

	tests := []struct {
		name string
		find func() ([]*RegistryServer, error)
		want []string
	}{
		{"filesystem", func() ([]*RegistryServer, error) { return manager.DiscoverFileSystemServers(t.Context()) }, []string{"fs"}},
		{"database", func() ([]*RegistryServer, error) { return manager.DiscoverDatabaseServers(t.Context()) }, []string{"db"}},
		{"web", func() ([]*RegistryServer, error) { return manager.DiscoverWebServers(t.Context()) }, []string{"web"}},
		{"code", func() ([]*RegistryServer, error) { return manager.DiscoverCodeServers(t.Context()) }, []string{"code"}},
		{"description", func() ([]*RegistryServer, error) { return manager.DiscoverServersByCapability(t.Context(), "RELEVANT") }, []string{"misc"}},
		{"category", func() ([]*RegistryServer, error) { return manager.DiscoverServersByCapability(t.Context(), "misc") }, []string{"misc"}},
		{"nothing", func() ([]*RegistryServer, error) { return manager.DiscoverServersByCapability(t.Context(), "quantum") }, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.find()
			require.NoError(t, err)
			assert.Equal(t, tt.want, ids(got))
		})
	}

	popular, err := manager.GetPopularServers(t.Context())
	require.NoError(t, err)
	assert.Len(t, popular.Servers, len(sampleServers()))
	assert.Equal(t, url.Values{"verified": {"true"}, "limit": {"20"}}, reg.lastQuery())

	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	_, err = NewRegistryManager(closed.URL).DiscoverCodeServers(t.Context())
	assert.Error(t, err)
}

func TestRegistryManagerBuildServerFromRegistry(t *testing.T) {
	mcpServer, _ := newStreamableTestServer(t)
	reg := &fakeRegistry{servers: []RegistryServer{
		{ID: "live", Name: "Live", Installation: InstallationInfo{Type: "http", Command: mcpServer.URL}},
	}}
	manager := NewRegistryManager(reg.start(t).URL)

	server, err := manager.BuildServerFromRegistry(t.Context(), "live")
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	tools, err := server.ListTools(t.Context())
	require.NoError(t, err)
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	assert.Equal(t, []string{"echo", "fail"}, names)
}
