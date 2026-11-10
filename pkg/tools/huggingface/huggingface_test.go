package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const modelsJSON = `[
  {
    "_id": "abc",
    "modelId": "org/bert-base",
    "id": "org/bert-base",
    "description": "A BERT model",
    "downloads": 1200,
    "likes": 34,
    "pipeline_tag": "fill-mask",
    "library_name": "transformers",
    "tags": ["bert", "nlp"]
  },
  {
    "_id": "def",
    "modelId": "org/gpt-small",
    "id": "org/gpt-small",
    "downloads": 5,
    "likes": 1,
    "pipeline_tag": "text-generation",
    "library_name": "pytorch",
    "tags": []
  }
]`

// newServer returns a test server that records the last request query.
func newServer(t *testing.T, status int, body string) (*httptest.Server, *url.Values) {
	t.Helper()
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/models", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		got = r.URL.Query()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestNewDefaultsAndOptions(t *testing.T) {
	tool := New()
	assert.Equal(t, "https://huggingface.co", tool.baseURL)
	require.NotNil(t, tool.httpClient)
	assert.Equal(t, 10*time.Second, tool.httpClient.Timeout)

	client := &http.Client{}
	tool = New(WithBaseURL("http://example.test"), WithHTTPClient(client))
	assert.Equal(t, "http://example.test", tool.baseURL)
	assert.Same(t, client, tool.httpClient)
}

func TestMetadata(t *testing.T) {
	tool := New()
	assert.Equal(t, "huggingface_search", tool.Name())
	assert.Equal(t, "Hugging Face Search", tool.DisplayName())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.Internal())

	params := tool.Parameters()
	require.Contains(t, params, "query")
	assert.True(t, params["query"].Required)
	require.Contains(t, params, "limit")
	assert.False(t, params["limit"].Required)
	assert.Equal(t, 5, params["limit"].Default)
}

func TestRunFormatsResults(t *testing.T) {
	srv, query := newServer(t, http.StatusOK, modelsJSON)
	tool := New(WithBaseURL(srv.URL))

	out, err := tool.Run(context.Background(), `{"query": "bert base", "limit": 2}`)
	require.NoError(t, err)

	assert.Equal(t, "bert base", query.Get("search"))
	assert.Equal(t, "downloads", query.Get("sort"))
	assert.Equal(t, "-1", query.Get("direction"))
	assert.Equal(t, "2", query.Get("limit"))

	assert.Contains(t, out, "Found 2 models matching 'bert base':")
	assert.Contains(t, out, "1. org/bert-base\n")
	assert.Contains(t, out, "   Description: A BERT model\n")
	assert.Contains(t, out, "   Downloads: 1200\n")
	assert.Contains(t, out, "   Likes: 34\n")
	assert.Contains(t, out, "   Task: fill-mask\n")
	assert.Contains(t, out, "   Library: transformers\n")
	assert.Contains(t, out, "   Tags: [bert nlp]\n")
	assert.Contains(t, out, "2. org/gpt-small\n")
	// The second model has no description, so only one description line appears.
	assert.Equal(t, 1, strings.Count(out, "Description:"))
}

func TestRunPlainTextQueryUsesDefaultLimit(t *testing.T) {
	srv, query := newServer(t, http.StatusOK, `[]`)
	tool := New(WithBaseURL(srv.URL))

	out, err := tool.Run(context.Background(), "llama")
	require.NoError(t, err)
	assert.Equal(t, "llama", query.Get("search"))
	assert.Equal(t, "5", query.Get("limit"))
	assert.Equal(t, "Found 0 models matching 'llama':\n\n", out)
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		input   string
		wantErr string
	}{
		{name: "missing query", status: http.StatusOK, body: `[]`, input: `{"limit": 3}`, wantErr: "query parameter is required"},
		{name: "empty query", status: http.StatusOK, body: `[]`, input: `{"query": ""}`, wantErr: "query parameter is required"},
		{name: "non-200 status", status: http.StatusInternalServerError, body: `oops`, input: "bert", wantErr: "non-200 status code: 500"},
		{name: "invalid body", status: http.StatusOK, body: `{not json`, input: "bert", wantErr: "failed to decode response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newServer(t, tt.status, tt.body)
			tool := New(WithBaseURL(srv.URL))

			_, err := tool.Run(context.Background(), tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestRunRequestFailures(t *testing.T) {
	t.Run("invalid base URL", func(t *testing.T) {
		tool := New(WithBaseURL("http://bad\x7fhost"))
		_, err := tool.Run(context.Background(), "bert")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create request")
	})

	t.Run("server unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		base := srv.URL
		srv.Close()

		tool := New(WithBaseURL(base))
		_, err := tool.Run(context.Background(), "bert")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to make request")
	})
}

func TestExecute(t *testing.T) {
	t.Run("passes query and limit", func(t *testing.T) {
		srv, query := newServer(t, http.StatusOK, modelsJSON)
		tool := New(WithBaseURL(srv.URL))

		out, err := tool.Execute(context.Background(), `{"query": "bert", "limit": 7}`)
		require.NoError(t, err)
		assert.Equal(t, "bert", query.Get("search"))
		assert.Equal(t, "7", query.Get("limit"))
		assert.Contains(t, out, "Found 2 models matching 'bert':")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := New().Execute(context.Background(), `not json`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse args")
	})
}
