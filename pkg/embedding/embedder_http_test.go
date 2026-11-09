package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

// recordingServer is an httptest server that stores the last request body
// and replies with a fixed status and body.
type recordingServer struct {
	*httptest.Server
	mu       sync.Mutex
	lastPath string
	lastBody map[string]interface{}
}

func newRecordingServer(t *testing.T, status int, reply string) *recordingServer {
	t.Helper()
	rs := &recordingServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		rs.mu.Lock()
		rs.lastPath = r.URL.Path
		rs.lastBody = body
		rs.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *recordingServer) body() map[string]interface{} {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.lastBody
}

func (rs *recordingServer) path() string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.lastPath
}

func newTestOpenAIEmbedder(url string, config EmbeddingConfig) *OpenAIEmbedder {
	e := NewOpenAIEmbedderWithConfig("test-key", config)
	e.client = openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(url),
		option.WithMaxRetries(0),
	)
	return e
}

func TestOpenAIEmbed(t *testing.T) {
	srv := newRecordingServer(t, http.StatusOK,
		`{"object":"list","model":"m","data":[{"object":"embedding","index":0,"embedding":[0.5,1.5]}]}`)

	cfg := EmbeddingConfig{Model: "m", Dimensions: 2, EncodingFormat: "float", UserID: "u1"}
	e := newTestOpenAIEmbedder(srv.URL, cfg)

	got, err := e.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.5, 1.5}, got)

	body := srv.body()
	assert.True(t, strings.HasSuffix(srv.path(), "/embeddings"))
	assert.Equal(t, "hello", body["input"])
	assert.Equal(t, "m", body["model"])
	assert.EqualValues(t, 2, body["dimensions"])
	assert.Equal(t, "float", body["encoding_format"])
	assert.Equal(t, "u1", body["user"])
}

func TestOpenAIEmbedOmitsUnsetOptions(t *testing.T) {
	srv := newRecordingServer(t, http.StatusOK,
		`{"data":[{"index":0,"embedding":[1]}]}`)
	e := newTestOpenAIEmbedder(srv.URL, EmbeddingConfig{Model: "m"})

	_, err := e.Embed(context.Background(), "x")
	require.NoError(t, err)
	body := srv.body()
	assert.NotContains(t, body, "dimensions")
	assert.NotContains(t, body, "user")
}

func TestOpenAIEmbedErrors(t *testing.T) {
	ctx := context.Background()

	empty := newRecordingServer(t, http.StatusOK, `{"data":[]}`)
	_, err := newTestOpenAIEmbedder(empty.URL, EmbeddingConfig{}).Embed(ctx, "x")
	require.ErrorContains(t, err, "no embedding data returned")

	_, err = newTestOpenAIEmbedder(empty.URL, EmbeddingConfig{}).EmbedBatch(ctx, []string{"x"})
	require.ErrorContains(t, err, "no embedding data returned")

	failing := newRecordingServer(t, http.StatusBadRequest, `{"error":{"message":"bad input"}}`)
	_, err = newTestOpenAIEmbedder(failing.URL, EmbeddingConfig{}).Embed(ctx, "x")
	require.Error(t, err)

	_, err = newTestOpenAIEmbedder(failing.URL, EmbeddingConfig{}).EmbedBatch(ctx, []string{"x"})
	require.Error(t, err)
}

func TestOpenAIEmbedBatchOrdersByIndex(t *testing.T) {
	srv := newRecordingServer(t, http.StatusOK,
		`{"data":[{"index":1,"embedding":[2]},{"index":0,"embedding":[1]}]}`)
	cfg := EmbeddingConfig{Model: "m", Dimensions: 1, EncodingFormat: "float", UserID: "u"}
	e := newTestOpenAIEmbedder(srv.URL, cfg)

	got, err := e.EmbedBatch(context.Background(), []string{"a", "b"})
	require.NoError(t, err)
	assert.Equal(t, [][]float32{{1}, {2}}, got)

	body := srv.body()
	assert.Equal(t, []interface{}{"a", "b"}, body["input"])
	assert.EqualValues(t, 1, body["dimensions"])
	assert.Equal(t, "u", body["user"])
}

func TestOpenAIEmbedBatchInvalidIndex(t *testing.T) {
	srv := newRecordingServer(t, http.StatusOK, `{"data":[{"index":5,"embedding":[1]}]}`)
	_, err := newTestOpenAIEmbedder(srv.URL, EmbeddingConfig{}).EmbedBatch(context.Background(), []string{"a"})
	require.ErrorContains(t, err, "invalid embedding index: 5")
}

func TestOpenAIEmbedBatchEmptyInput(t *testing.T) {
	// No server: an empty batch must return before any request is made.
	e := NewOpenAIEmbedder("k", "")
	got, err := e.EmbedBatch(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func newTestGeminiEmbedder(t *testing.T, url string, opts ...GeminiEmbedderOption) *GeminiEmbedder {
	t.Helper()
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: url},
	})
	require.NoError(t, err)
	e, err := NewGeminiEmbedder(context.Background(), append([]GeminiEmbedderOption{WithGeminiClient(client)}, opts...)...)
	require.NoError(t, err)
	return e
}

func TestNewGeminiEmbedderWithInjectedClient(t *testing.T) {
	e := newTestGeminiEmbedder(t, "http://unused", WithGeminiModel(ModelTextEmbedding005), WithGeminiLogger(nil))
	assert.Equal(t, ModelTextEmbedding005, e.GetModel())
	assert.Equal(t, ModelTextEmbedding005, e.GetConfig().Model)
	assert.Equal(t, genai.BackendGeminiAPI, e.backend)
	assert.Equal(t, "us-central1", e.location)
}

func TestNewGeminiEmbedderFillsEmptyConfigModel(t *testing.T) {
	e := newTestGeminiEmbedder(t, "http://unused", WithGeminiConfig(EmbeddingConfig{Dimensions: 10}))
	assert.Equal(t, DefaultGeminiEmbeddingModel, e.GetConfig().Model)
	assert.Equal(t, DefaultGeminiEmbeddingModel, e.GetModel())
}

func TestNewGeminiEmbedderCreatesClient(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		opts []GeminiEmbedderOption
	}{
		{"gemini api key", []GeminiEmbedderOption{WithGeminiAPIKey("k")}},
		{"vertex api key", []GeminiEmbedderOption{WithGeminiBackend(genai.BackendVertexAI), WithGeminiAPIKey("k")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewGeminiEmbedder(ctx, tt.opts...)
			require.NoError(t, err)
			assert.NotNil(t, e.client)
		})
	}
}

func TestNewGeminiEmbedderBadCredentials(t *testing.T) {
	ctx := context.Background()

	_, err := NewGeminiEmbedder(ctx,
		WithGeminiBackend(genai.BackendVertexAI),
		WithGeminiCredentialsJSON([]byte("not json")),
	)
	require.ErrorContains(t, err, "failed to load credentials from JSON")

	_, err = NewGeminiEmbedder(ctx,
		WithGeminiBackend(genai.BackendVertexAI),
		WithGeminiCredentialsFile(t.TempDir()+"/missing.json"),
	)
	require.ErrorContains(t, err, "failed to load credentials from file")
}

func TestGeminiEmbed(t *testing.T) {
	srv := newRecordingServer(t, http.StatusOK, `{"embeddings":[{"values":[0.25,0.75]}]}`)
	e := newTestGeminiEmbedder(t, srv.URL, WithGeminiTaskType("RETRIEVAL_QUERY"))

	got, err := e.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.25, 0.75}, got)

	assert.Contains(t, srv.path(), ModelTextEmbedding004+":batchEmbedContents")
	reqs, ok := srv.body()["requests"].([]interface{})
	require.True(t, ok)
	require.Len(t, reqs, 1)
	req := reqs[0].(map[string]interface{})
	assert.Equal(t, "RETRIEVAL_QUERY", req["taskType"])
	assert.EqualValues(t, 768, req["outputDimensionality"])
}

func TestGeminiEmbedWithConfigUsesFallbackModel(t *testing.T) {
	srv := newRecordingServer(t, http.StatusOK, `{"embeddings":[{"values":[1]}]}`)
	e := newTestGeminiEmbedder(t, srv.URL, WithGeminiModel(ModelTextEmbedding005))

	_, err := e.EmbedWithConfig(context.Background(), "x", EmbeddingConfig{Dimensions: 40000})
	require.NoError(t, err)
	assert.Contains(t, srv.path(), ModelTextEmbedding005)
	req := srv.body()["requests"].([]interface{})[0].(map[string]interface{})
	// Dimensions above the int32 guard are not sent.
	assert.NotContains(t, req, "outputDimensionality")
}

func TestGeminiEmbedErrors(t *testing.T) {
	ctx := context.Background()

	empty := newRecordingServer(t, http.StatusOK, `{"embeddings":[]}`)
	_, err := newTestGeminiEmbedder(t, empty.URL).Embed(ctx, "x")
	require.ErrorContains(t, err, "no embedding data returned from Gemini API")
	_, err = newTestGeminiEmbedder(t, empty.URL).EmbedBatch(ctx, []string{"x"})
	require.ErrorContains(t, err, "no embedding data returned from Gemini API")

	noValues := newRecordingServer(t, http.StatusOK, `{"embeddings":[{"values":[1]},{"values":[]}]}`)
	_, err = newTestGeminiEmbedder(t, noValues.URL).EmbedBatch(ctx, []string{"a", "b"})
	require.ErrorContains(t, err, "empty embedding at index 1")

	failing := newRecordingServer(t, http.StatusBadRequest, `{"error":{"code":400,"message":"bad"}}`)
	_, err = newTestGeminiEmbedder(t, failing.URL).Embed(ctx, "x")
	require.ErrorContains(t, err, "failed to generate embedding")
	_, err = newTestGeminiEmbedder(t, failing.URL).EmbedBatch(ctx, []string{"x"})
	require.ErrorContains(t, err, "failed to generate batch embeddings")
}

func TestGeminiEmbedBatch(t *testing.T) {
	srv := newRecordingServer(t, http.StatusOK, `{"embeddings":[{"values":[1]},{"values":[2]}]}`)
	e := newTestGeminiEmbedder(t, srv.URL, WithGeminiTaskType("CLUSTERING"))

	got, err := e.EmbedBatchWithConfig(context.Background(), []string{"a", "b"}, EmbeddingConfig{Dimensions: 8})
	require.NoError(t, err)
	assert.Equal(t, [][]float32{{1}, {2}}, got)

	reqs := srv.body()["requests"].([]interface{})
	require.Len(t, reqs, 2)
	for _, r := range reqs {
		req := r.(map[string]interface{})
		assert.Equal(t, "CLUSTERING", req["taskType"])
		assert.EqualValues(t, 8, req["outputDimensionality"])
	}
}

func TestGeminiEmbedBatchEmptyInput(t *testing.T) {
	e := &GeminiEmbedder{}
	got, err := e.EmbedBatch(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
