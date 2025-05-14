package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	gopenai "github.com/sashabaranov/go-openai"
)

func TestGenerate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Expected Authorization header with test-key")
		}

		// Parse request body
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Send response
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"text": "test response",
				},
			},
		})
	}))
	defer server.Close()

	// Create a custom HTTP client that directs to our test server
	customHTTPClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}

	// Create the OpenAI client with our custom HTTP client
	config := gopenai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	config.HTTPClient = customHTTPClient
	openaiClient := gopenai.NewClientWithConfig(config)

	// Create our wrapper client
	client := openai.NewClient("test-key", openai.WithModel("gpt-4"))

	// Override the client with our test client
	client.Client = openaiClient

	// Test generation
	resp, err := client.Generate(context.Background(), "test prompt", nil)
	if err != nil {
		t.Fatalf("Failed to generate: %v", err)
	}

	if resp != "test response" {
		t.Errorf("Expected response 'test response', got '%s'", resp)
	}
}

func TestChat(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Parse request body
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Send response
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "test response",
					},
				},
			},
		})
	}))
	defer server.Close()

	// Create a custom HTTP client that directs to our test server
	customHTTPClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}

	// Create the OpenAI client with our custom HTTP client
	config := gopenai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	config.HTTPClient = customHTTPClient
	openaiClient := gopenai.NewClientWithConfig(config)

	// Create our wrapper client
	client := openai.NewClient("test-key", openai.WithModel("gpt-4"))

	// Override the client with our test client
	client.Client = openaiClient

	// Test chat
	messages := []llm.Message{
		{
			Role:    "user",
			Content: "test message",
		},
	}

	resp, err := client.Chat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("Failed to chat: %v", err)
	}

	if resp != "test response" {
		t.Errorf("Expected response 'test response', got '%s'", resp)
	}
}
