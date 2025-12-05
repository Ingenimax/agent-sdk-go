package deepseek

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name       string
		apiKey     string
		options    []Option
		wantModel  string
		wantBase   string
	}{
		{
			name:      "default configuration",
			apiKey:    "test-key",
			options:   nil,
			wantModel: DefaultModel,
			wantBase:  DefaultBaseURL,
		},
		{
			name:      "with custom model",
			apiKey:    "test-key",
			options:   []Option{WithModel("deepseek-reasoner")},
			wantModel: "deepseek-reasoner",
			wantBase:  DefaultBaseURL,
		},
		{
			name:      "with custom base URL",
			apiKey:    "test-key",
			options:   []Option{WithBaseURL("https://custom.api.com")},
			wantModel: DefaultModel,
			wantBase:  "https://custom.api.com",
		},
		{
			name:      "with multiple options",
			apiKey:    "test-key",
			options:   []Option{WithModel("deepseek-reasoner"), WithBaseURL("https://custom.api.com")},
			wantModel: "deepseek-reasoner",
			wantBase:  "https://custom.api.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.apiKey, tt.options...)

			if client == nil {
				t.Fatal("expected non-nil client")
			}

			if client.APIKey != tt.apiKey {
				t.Errorf("APIKey = %v, want %v", client.APIKey, tt.apiKey)
			}

			if client.Model != tt.wantModel {
				t.Errorf("Model = %v, want %v", client.Model, tt.wantModel)
			}

			if client.BaseURL != tt.wantBase {
				t.Errorf("BaseURL = %v, want %v", client.BaseURL, tt.wantBase)
			}

			if client.HTTPClient == nil {
				t.Error("expected non-nil HTTPClient")
			}

			if client.logger == nil {
				t.Error("expected non-nil logger")
			}
		})
	}
}

func TestName(t *testing.T) {
	client := NewClient("test-key")
	if got := client.Name(); got != "deepseek" {
		t.Errorf("Name() = %v, want %v", got, "deepseek")
	}
}

func TestSupportsStreaming(t *testing.T) {
	client := NewClient("test-key")
	if got := client.SupportsStreaming(); !got {
		t.Error("SupportsStreaming() = false, want true")
	}
}

func TestGenerate(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Method = %v, want POST", r.Method)
		}

		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("Path = %v, want /v1/chat/completions", r.URL.Path)
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-key" {
			t.Errorf("Authorization = %v, want Bearer test-key", authHeader)
		}

		// Parse request body
		var req ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		// Verify request fields
		if req.Model != "deepseek-chat" {
			t.Errorf("Model = %v, want deepseek-chat", req.Model)
		}

		if len(req.Messages) == 0 {
			t.Error("expected at least one message")
		}

		// Send response
		resp := ChatCompletionResponse{
			ID:      "test-id",
			Object:  "chat.completion",
			Created: 1234567890,
			Model:   "deepseek-chat",
			Choices: []Choice{
				{
					Index: 0,
					Message: Message{
						Role:    "assistant",
						Content: "This is a test response",
					},
					FinishReason: "stop",
				},
			},
			Usage: Usage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Logf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := NewClient("test-key",
		WithBaseURL(server.URL),
		WithLogger(logging.New()),
	)

	// Test Generate
	ctx := context.Background()
	result, err := client.Generate(ctx, "test prompt")

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if result != "This is a test response" {
		t.Errorf("Generate() = %v, want 'This is a test response'", result)
	}
}

func TestGenerateDetailed(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		var req ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		// Verify temperature is set
		if req.Temperature != 0.9 {
			t.Errorf("Temperature = %v, want 0.9", req.Temperature)
		}

		// Send response
		resp := ChatCompletionResponse{
			ID:      "test-id",
			Object:  "chat.completion",
			Created: 1234567890,
			Model:   "deepseek-chat",
			Choices: []Choice{
				{
					Index: 0,
					Message: Message{
						Role:    "assistant",
						Content: "Detailed response",
					},
					FinishReason: "stop",
				},
			},
			Usage: Usage{
				PromptTokens:     15,
				CompletionTokens: 25,
				TotalTokens:      40,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Logf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := NewClient("test-key", WithBaseURL(server.URL))

	// Test GenerateDetailed with options
	ctx := context.Background()
	result, err := client.GenerateDetailed(ctx, "test prompt",
		interfaces.WithTemperature(0.9),
		interfaces.WithSystemMessage("You are a helpful assistant"),
	)

	if err != nil {
		t.Fatalf("GenerateDetailed() error = %v", err)
	}

	if result.Content != "Detailed response" {
		t.Errorf("Content = %v, want 'Detailed response'", result.Content)
	}

	if result.Model != "deepseek-chat" {
		t.Errorf("Model = %v, want 'deepseek-chat'", result.Model)
	}

	if result.StopReason != "stop" {
		t.Errorf("StopReason = %v, want 'stop'", result.StopReason)
	}

	if result.Usage == nil {
		t.Fatal("expected non-nil Usage")
	}

	if result.Usage.InputTokens != 15 {
		t.Errorf("InputTokens = %v, want 15", result.Usage.InputTokens)
	}

	if result.Usage.OutputTokens != 25 {
		t.Errorf("OutputTokens = %v, want 25", result.Usage.OutputTokens)
	}

	if result.Usage.TotalTokens != 40 {
		t.Errorf("TotalTokens = %v, want 40", result.Usage.TotalTokens)
	}

	if result.Metadata == nil {
		t.Fatal("expected non-nil Metadata")
	}

	if result.Metadata["provider"] != "deepseek" {
		t.Errorf("Metadata[provider] = %v, want 'deepseek'", result.Metadata["provider"])
	}
}

func TestGenerateWithAPIError(t *testing.T) {
	// Create a mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"error": {"message": "Invalid request"}}`)); err != nil {
			t.Logf("Failed to write error response: %v", err)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := NewClient("test-key", WithBaseURL(server.URL))

	// Test Generate with error
	ctx := context.Background()
	_, err := client.Generate(ctx, "test prompt")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGenerateWithSystemMessage(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		var req ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		// Verify system message is first
		if len(req.Messages) < 2 {
			t.Fatal("expected at least 2 messages")
		}

		if req.Messages[0].Role != "system" {
			t.Errorf("First message role = %v, want system", req.Messages[0].Role)
		}

		if req.Messages[0].Content != "You are a helpful assistant" {
			t.Errorf("System message = %v, want 'You are a helpful assistant'", req.Messages[0].Content)
		}

		// Send response
		resp := ChatCompletionResponse{
			ID:      "test-id",
			Object:  "chat.completion",
			Created: 1234567890,
			Model:   "deepseek-chat",
			Choices: []Choice{
				{
					Index: 0,
					Message: Message{
						Role:    "assistant",
						Content: "Response with system message",
					},
					FinishReason: "stop",
				},
			},
			Usage: Usage{
				PromptTokens:     20,
				CompletionTokens: 30,
				TotalTokens:      50,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Logf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := NewClient("test-key", WithBaseURL(server.URL))

	// Test Generate with system message
	ctx := context.Background()
	result, err := client.Generate(ctx, "test prompt",
		interfaces.WithSystemMessage("You are a helpful assistant"),
	)

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if result != "Response with system message" {
		t.Errorf("Generate() = %v, want 'Response with system message'", result)
	}
}

func TestGenerateWithAllOptions(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		var req ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		// Verify all options
		if req.Temperature != 0.8 {
			t.Errorf("Temperature = %v, want 0.8", req.Temperature)
		}

		if req.TopP != 0.95 {
			t.Errorf("TopP = %v, want 0.95", req.TopP)
		}

		if req.FrequencyPenalty != 0.5 {
			t.Errorf("FrequencyPenalty = %v, want 0.5", req.FrequencyPenalty)
		}

		if req.PresencePenalty != 0.3 {
			t.Errorf("PresencePenalty = %v, want 0.3", req.PresencePenalty)
		}

		if len(req.Stop) != 2 {
			t.Errorf("Stop sequences length = %v, want 2", len(req.Stop))
		}

		// Send response
		resp := ChatCompletionResponse{
			ID:      "test-id",
			Object:  "chat.completion",
			Created: 1234567890,
			Model:   "deepseek-chat",
			Choices: []Choice{
				{
					Index: 0,
					Message: Message{
						Role:    "assistant",
						Content: "Response with all options",
					},
					FinishReason: "stop",
				},
			},
			Usage: Usage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Logf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := NewClient("test-key", WithBaseURL(server.URL))

	// Test Generate with all options
	ctx := context.Background()
	result, err := client.Generate(ctx, "test prompt",
		interfaces.WithTemperature(0.8),
		interfaces.WithTopP(0.95),
		interfaces.WithFrequencyPenalty(0.5),
		interfaces.WithPresencePenalty(0.3),
		interfaces.WithStopSequences([]string{"stop1", "stop2"}),
	)

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if result != "Response with all options" {
		t.Errorf("Generate() = %v, want 'Response with all options'", result)
	}
}
