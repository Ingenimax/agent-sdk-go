package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contentsPrefix = "/repos/acme/demo/contents/"

func file(path string, size int) map[string]any {
	return map[string]any{"type": "file", "name": path[strings.LastIndex(path, "/")+1:], "path": path, "size": size}
}

func dir(path string) map[string]any {
	return map[string]any{"type": "dir", "name": path[strings.LastIndex(path, "/")+1:], "path": path}
}

func blob(path, content string) map[string]any {
	m := file(path, len(content))
	m["encoding"] = "base64"
	m["content"] = base64.StdEncoding.EncodeToString([]byte(content))
	return m
}

// defaultRepo describes a small repository as API responses keyed by path.
// The root ("") lists two files, a directory and a non-matching file.
func defaultRepo() map[string]any {
	return map[string]any{
		"": []any{
			file("main.go", 10),
			dir("pkg"),
			file("big.go", 1000),
			file("README.md", 5),
		},
		"pkg": []any{
			file("pkg/util.go", 20),
			dir("pkg/sub"),
			map[string]any{"name": "untyped"},
		},
		"pkg/sub": []any{
			file("pkg/sub/deep.go", 30),
		},
		"main.go":         blob("main.go", "package main"),
		"big.go":          blob("big.go", "package big"),
		"README.md":       blob("README.md", "# demo"),
		"pkg/util.go":     blob("pkg/util.go", "package pkg"),
		"pkg/sub/deep.go": blob("pkg/sub/deep.go", "package sub"),
	}
}

// newTool returns a tool whose client talks to a fake GitHub contents API.
func newTool(t *testing.T, token string, repo map[string]any) (*GitHubContentExtractorTool, *string) {
	t.Helper()
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		path, ok := strings.CutPrefix(r.URL.Path, contentsPrefix)
		body, found := repo[path]
		if !ok || !found {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message": "Not Found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)

	tool, err := NewGitHubContentExtractorTool(token)
	require.NoError(t, err)
	base, err := url.Parse(srv.URL + "/")
	require.NoError(t, err)
	tool.client.BaseURL = base
	return tool, &authHeader
}

func runTool(t *testing.T, tool *GitHubContentExtractorTool, params SearchParams) ([]FileContent, error) {
	t.Helper()
	params.RepositoryURL = "https://github.com/acme/demo"
	input, err := json.Marshal(params)
	require.NoError(t, err)

	out, err := tool.Run(context.Background(), string(input))
	if err != nil {
		return nil, err
	}
	var files []FileContent
	require.NoError(t, json.Unmarshal([]byte(out), &files))
	return files, nil
}

func names(files []FileContent) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path+f.FileName)
	}
	return out
}

func TestMetadata(t *testing.T) {
	tool, err := NewGitHubContentExtractorTool("")
	require.NoError(t, err)

	assert.Equal(t, "github_content_extractor", tool.Name())
	assert.Equal(t, "GitHub Content Extractor", tool.DisplayName())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.Internal())

	params := tool.Parameters()
	for _, key := range []string{"repository_url", "file_patterns"} {
		require.Contains(t, params, key)
		assert.True(t, params[key].Required, key)
	}
	for _, key := range []string{"max_files", "max_depth", "max_file_size", "specific_files"} {
		require.Contains(t, params, key)
		assert.False(t, params[key].Required, key)
	}
	require.NotNil(t, params["file_patterns"].Items)
	assert.Equal(t, "string", params["file_patterns"].Items.Type)
}

func TestRunMatchesFiles(t *testing.T) {
	tests := []struct {
		name   string
		params SearchParams
		want   []string
	}{
		{
			name:   "all go files in traversal order",
			params: SearchParams{FilePatterns: []string{".go"}},
			want:   []string{"main.go", "pkg/util.go", "pkg/sub/deep.go", "big.go"},
		},
		{
			name:   "multiple patterns",
			params: SearchParams{FilePatterns: []string{".md", "util.go"}},
			want:   []string{"pkg/util.go", "README.md"},
		},
		{
			name:   "max file size skips large files",
			params: SearchParams{FilePatterns: []string{".go"}, MaxFileSize: 100},
			want:   []string{"main.go", "pkg/util.go", "pkg/sub/deep.go"},
		},
		{
			name:   "max depth stops traversal",
			params: SearchParams{FilePatterns: []string{".go"}, MaxDepth: 1},
			want:   []string{"main.go", "pkg/util.go", "big.go"},
		},
		{
			name:   "max files stops early",
			params: SearchParams{FilePatterns: []string{".go"}, MaxFiles: 2},
			want:   []string{"main.go", "pkg/util.go"},
		},
		{
			name:   "max files reached at root level",
			params: SearchParams{FilePatterns: []string{".go"}, MaxFiles: 1},
			want:   []string{"main.go"},
		},
		{
			name:   "specific files",
			params: SearchParams{FilePatterns: []string{".go"}, SpecificFiles: []string{"sub/deep.go", "big.go"}},
			want:   []string{"pkg/sub/deep.go", "big.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, _ := newTool(t, "", defaultRepo())
			files, err := runTool(t, tool, tt.params)
			require.NoError(t, err)
			assert.Equal(t, tt.want, names(files))
		})
	}
}

func TestRunReturnsDecodedContent(t *testing.T) {
	tool, _ := newTool(t, "", defaultRepo())
	files, err := runTool(t, tool, SearchParams{FilePatterns: []string{"deep.go"}})
	require.NoError(t, err)

	require.Len(t, files, 1)
	assert.Equal(t, FileContent{FileName: "deep.go", Path: "pkg/sub/", Content: "package sub"}, files[0])
}

func TestRunSendsToken(t *testing.T) {
	tool, auth := newTool(t, "secret-token", defaultRepo())
	_, err := runTool(t, tool, SearchParams{FilePatterns: []string{"main.go"}})
	require.NoError(t, err)
	assert.Equal(t, "Bearer secret-token", *auth)
}

func TestRunDirectoryReturningSingleFile(t *testing.T) {
	// The directory call returns a file object instead of a listing.
	repo := map[string]any{
		"":              []any{dir("single")},
		"single":        blob("single/one.go", "package one"),
		"single/one.go": blob("single/one.go", "package one"),
	}

	tool, _ := newTool(t, "", repo)
	files, err := runTool(t, tool, SearchParams{FilePatterns: []string{".go"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"single/one.go"}, names(files))
}

func TestRunNoMatches(t *testing.T) {
	tool, _ := newTool(t, "", defaultRepo())
	files, err := runTool(t, tool, SearchParams{FilePatterns: []string{".rs"}})
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestRunRootIsSingleFile(t *testing.T) {
	// When the root call returns a file object, the tool treats the repository as empty.
	tool, _ := newTool(t, "", map[string]any{"": blob("only.go", "x")})
	out, err := tool.Execute(context.Background(), `{"repository_url": "https://github.com/acme/demo", "file_patterns": [".go"]}`)
	require.NoError(t, err)
	var files []FileContent
	require.NoError(t, json.Unmarshal([]byte(out), &files))
	assert.Empty(t, files)
}

func TestRunNestedDirectoryErrorIsIgnored(t *testing.T) {
	repo := defaultRepo()
	repo["pkg"] = []any{file("pkg/util.go", 20), dir("pkg/missing")}

	tool, _ := newTool(t, "", repo)
	files, err := runTool(t, tool, SearchParams{FilePatterns: []string{".go"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"main.go", "pkg/util.go", "big.go"}, names(files))
}

func TestRunErrors(t *testing.T) {
	brokenEncoding := blob("main.go", "x")
	brokenEncoding["encoding"] = "weird"

	tests := []struct {
		name    string
		input   string
		repo    func() map[string]any
		wantErr string
	}{
		{
			name:    "invalid JSON",
			input:   `nope`,
			repo:    defaultRepo,
			wantErr: "failed to parse input",
		},
		{
			name:    "invalid repository URL",
			input:   `{"repository_url": "https://github.com/acme", "file_patterns": [".go"]}`,
			repo:    defaultRepo,
			wantErr: "failed to extract repository info",
		},
		{
			name:    "root listing fails",
			input:   `{"repository_url": "https://github.com/acme/demo", "file_patterns": [".go"]}`,
			repo:    func() map[string]any { return map[string]any{} },
			wantErr: "failed to get repository contents",
		},
		{
			name:  "top level directory fails",
			input: `{"repository_url": "https://github.com/acme/demo", "file_patterns": [".go"]}`,
			repo: func() map[string]any {
				return map[string]any{"": []any{dir("gone")}}
			},
			wantErr: "failed to get contents of directory gone",
		},
		{
			name:  "file content missing",
			input: `{"repository_url": "https://github.com/acme/demo", "file_patterns": [".go"]}`,
			repo: func() map[string]any {
				return map[string]any{"": []any{file("main.go", 1)}}
			},
			wantErr: "failed to get content of file main.go",
		},
		{
			name:  "file content has unsupported encoding",
			input: `{"repository_url": "https://github.com/acme/demo", "file_patterns": [".go"]}`,
			repo: func() map[string]any {
				return map[string]any{"": []any{file("main.go", 1)}, "main.go": brokenEncoding}
			},
			wantErr: "unsupported content encoding: weird",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, _ := newTool(t, "", tt.repo())
			out, err := tool.Run(context.Background(), tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, out)
		})
	}
}

func TestExtractRepoInfo(t *testing.T) {
	tests := []struct {
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{url: "https://github.com/acme/demo", wantOwner: "acme", wantRepo: "demo"},
		{url: "acme/demo", wantOwner: "acme", wantRepo: "demo"},
		{url: "https://github.com/acme", wantErr: true},
		{url: "https://github.com/acme/demo/tree/main", wantErr: true},
		{url: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, repo, err := extractRepoInfo(tt.url)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid GitHub URL format")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}
