package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	client := NewClient()
	assert.NotNil(t, client)
	assert.Equal(t, "http://localhost:11434", client.BaseURL)
	assert.Equal(t, "qwen3:0.6b", client.Model)
	assert.NotNil(t, client.HTTPClient)
	assert.NotNil(t, client.logger)
}

func TestNewClientWithOptions(t *testing.T) {
	logger := logging.New()
	client := NewClient(
		WithModel("mistral"),
		WithBaseURL("http://localhost:8080"),
		WithLogger(logger),
	)

	assert.Equal(t, "mistral", client.Model)
	assert.Equal(t, "http://localhost:8080", client.BaseURL)
	assert.Equal(t, logger, client.logger)
}

func TestGenerate(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/generate", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse request
		var req GenerateRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "test-model", req.Model)
		assert.Equal(t, "test prompt", req.Prompt)
		assert.False(t, req.Stream)
		assert.Equal(t, 0.8, req.Options.Temperature)

		// Return response
		response := GenerateResponse{
			Model:    "test-model",
			Response: "This is a test response",
			Done:     true,
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	}))
	defer server.Close()

	// Create client
	client := NewClient(
		WithModel("test-model"),
		WithBaseURL(server.URL),
	)

	// Test generate
	response, err := client.Generate(
		context.Background(),
		"test prompt",
		WithTemperature(0.8),
	)

	require.NoError(t, err)
	assert.Equal(t, "This is a test response", response)
}

func TestGenerateWithSystemMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req GenerateRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "You are a helpful assistant", req.System)

		response := GenerateResponse{
			Model:    "test-model",
			Response: "System message received",
			Done:     true,
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	}))
	defer server.Close()

	client := NewClient(
		WithModel("test-model"),
		WithBaseURL(server.URL),
	)

	response, err := client.Generate(
		context.Background(),
		"test prompt",
		WithSystemMessage("You are a helpful assistant"),
	)

	require.NoError(t, err)
	assert.Equal(t, "System message received", response)
}

func TestChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/chat", r.URL.Path)

		var req ChatRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "test-model", req.Model)
		assert.Len(t, req.Messages, 2)
		assert.Equal(t, "system", req.Messages[0].Role)
		assert.Equal(t, "user", req.Messages[1].Role)

		response := ChatResponse{
			Model: "test-model",
			Message: ChatMessage{
				Role:    "assistant",
				Content: "Hello! How can I help you?",
			},
			Done: true,
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	}))
	defer server.Close()

	client := NewClient(
		WithModel("test-model"),
		WithBaseURL(server.URL),
	)

	messages := []llm.Message{
		{
			Role:    "system",
			Content: "You are a helpful assistant",
		},
		{
			Role:    "user",
			Content: "Hello",
		},
	}

	response, err := client.Chat(context.Background(), messages, &llm.GenerateParams{
		Temperature: 0.7,
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello! How can I help you?", response)
}

func TestGenerateWithTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req GenerateRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Check that the prompt includes tool descriptions
		assert.Contains(t, req.Prompt, "Available tools:")
		assert.Contains(t, req.Prompt, "- test-tool: A test tool")

		response := GenerateResponse{
			Model:    "test-model",
			Response: "I can help you with that using the available tools",
			Done:     true,
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	}))
	defer server.Close()

	client := NewClient(
		WithModel("test-model"),
		WithBaseURL(server.URL),
	)

	// Create a mock tool
	mockTool := &mockTool{
		name:        "test-tool",
		description: "A test tool",
	}

	tools := []interfaces.Tool{mockTool}

	response, err := client.GenerateWithTools(
		context.Background(),
		"Help me with something",
		tools,
	)

	require.NoError(t, err)
	assert.Equal(t, "I can help you with that using the available tools", response)
}

func TestListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/tags", r.URL.Path)

		response := struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}{
			Models: []struct {
				Name string `json:"name"`
			}{
				{Name: "llama2"},
				{Name: "mistral"},
				{Name: "codellama"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	models, err := client.ListModels(context.Background())

	require.NoError(t, err)
	assert.Len(t, models, 3)
	assert.Contains(t, models, "llama2")
	assert.Contains(t, models, "mistral")
	assert.Contains(t, models, "codellama")
}

func TestPullModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/pull", r.URL.Path)

		var req struct {
			Name string `json:"name"`
		}
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "mistral", req.Name)

		response := struct {
			Status string `json:"status"`
		}{
			Status: "success",
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	err := client.PullModel(context.Background(), "mistral")

	require.NoError(t, err)
}

func TestMakeRequestError(t *testing.T) {
	// Create a server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte("Internal Server Error"))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	_, err := client.Generate(context.Background(), "test prompt")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API request failed with status 500")
}

func TestName(t *testing.T) {
	client := NewClient()
	assert.Equal(t, "ollama", client.Name())
}

// Mock tool for testing
type mockTool struct {
	name        string
	description string
}

func (t *mockTool) Name() string {
	return t.name
}

func (t *mockTool) Description() string {
	return t.description
}

func (t *mockTool) Run(ctx context.Context, input string) (string, error) {
	return "mock result", nil
}

func (t *mockTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"input": {
			Type:        "string",
			Description: "Input parameter",
			Required:    true,
		},
	}
}

func (t *mockTool) Execute(ctx context.Context, args string) (string, error) {
	return "mock result", nil
}

// Test GenerateOption functions
func TestWithTemperature(t *testing.T) {
	options := &interfaces.GenerateOptions{}
	WithTemperature(0.5)(options)

	assert.NotNil(t, options.LLMConfig)
	assert.Equal(t, 0.5, options.LLMConfig.Temperature)
}

func TestWithTopP(t *testing.T) {
	options := &interfaces.GenerateOptions{}
	WithTopP(0.9)(options)

	assert.NotNil(t, options.LLMConfig)
	assert.Equal(t, 0.9, options.LLMConfig.TopP)
}

func TestWithStopSequences(t *testing.T) {
	options := &interfaces.GenerateOptions{}
	stopSequences := []string{"stop1", "stop2"}
	WithStopSequences(stopSequences)(options)

	assert.NotNil(t, options.LLMConfig)
	assert.Equal(t, stopSequences, options.LLMConfig.StopSequences)
}

func TestWithSystemMessage(t *testing.T) {
	options := &interfaces.GenerateOptions{}
	WithSystemMessage("You are helpful")(options)

	assert.Equal(t, "You are helpful", options.SystemMessage)
}
