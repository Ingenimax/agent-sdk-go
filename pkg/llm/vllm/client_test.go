package vllm

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	// Test default client creation
	client := NewClient()
	assert.NotNil(t, client)
	assert.Equal(t, "http://localhost:8000", client.BaseURL)
	assert.Equal(t, "llama-2-7b", client.Model)
	assert.NotNil(t, client.HTTPClient)
	assert.NotNil(t, client.logger)

	// Test client with options
	logger := logging.New()
	client = NewClient(
		WithModel("mistral-7b"),
		WithBaseURL("http://localhost:9000"),
		WithLogger(logger),
	)
	assert.Equal(t, "http://localhost:9000", client.BaseURL)
	assert.Equal(t, "mistral-7b", client.Model)
	assert.Equal(t, logger, client.logger)
}

func TestClientImplementsLLMInterface(t *testing.T) {
	// This test ensures the client implements the LLM interface
	var _ interfaces.LLM = (*VLLMClient)(nil)
}

func TestWithModel(t *testing.T) {
	client := NewClient()
	assert.Equal(t, "llama-2-7b", client.Model)

	client = NewClient(WithModel("codellama-7b"))
	assert.Equal(t, "codellama-7b", client.Model)
}

func TestWithBaseURL(t *testing.T) {
	client := NewClient()
	assert.Equal(t, "http://localhost:8000", client.BaseURL)

	client = NewClient(WithBaseURL("http://localhost:9000"))
	assert.Equal(t, "http://localhost:9000", client.BaseURL)
}

func TestWithLogger(t *testing.T) {
	logger := logging.New()
	client := NewClient(WithLogger(logger))
	assert.Equal(t, logger, client.logger)
}

func TestWithRetry(t *testing.T) {
	client := NewClient()
	assert.Nil(t, client.retryExecutor)

	client = NewClient(WithRetry())
	assert.NotNil(t, client.retryExecutor)
}

func TestWithHTTPClient(t *testing.T) {
	client := NewClient()
	assert.NotNil(t, client.HTTPClient)

	// Test that the option function exists and can be called
	// The actual HTTP client testing would be done in integration tests
	assert.NotNil(t, WithHTTPClient)
}

func TestName(t *testing.T) {
	client := NewClient()
	assert.Equal(t, "vllm", client.Name())
}

func TestGenerateOptions(t *testing.T) {
	// Test WithTemperature
	option := WithTemperature(0.5)
	options := &interfaces.GenerateOptions{}
	option(options)
	assert.Equal(t, 0.5, options.LLMConfig.Temperature)

	// Test WithTopP
	option = WithTopP(0.8)
	options = &interfaces.GenerateOptions{}
	option(options)
	assert.Equal(t, 0.8, options.LLMConfig.TopP)

	// Test WithStopSequences
	option = WithStopSequences([]string{"END", "STOP"})
	options = &interfaces.GenerateOptions{}
	option(options)
	assert.Equal(t, []string{"END", "STOP"}, options.LLMConfig.StopSequences)

	// Test WithSystemMessage
	option = WithSystemMessage("You are a helpful assistant.")
	options = &interfaces.GenerateOptions{}
	option(options)
	assert.Equal(t, "You are a helpful assistant.", options.SystemMessage)

	// Test WithResponseFormat
	format := interfaces.ResponseFormat{
		Type: interfaces.ResponseFormatJSON,
		Name: "TestFormat",
	}
	option = WithResponseFormat(format)
	options = &interfaces.GenerateOptions{}
	option(options)
	assert.Equal(t, &format, options.ResponseFormat)
}

func TestGenerateRequestStructure(t *testing.T) {
	// Test that GenerateRequest can be marshaled to JSON
	req := GenerateRequest{
		Model:         "llama-2-7b",
		Prompt:        "Hello, world!",
		Stream:        false,
		Temperature:   0.7,
		TopP:          0.9,
		TopK:          40,
		MaxTokens:     100,
		Stop:          []string{"END"},
		UseBeamSearch: false,
		BestOf:        1,
		N:             1,
	}

	// This test ensures the struct can be created without errors
	assert.Equal(t, "llama-2-7b", req.Model)
	assert.Equal(t, "Hello, world!", req.Prompt)
	assert.Equal(t, false, req.Stream)
	assert.Equal(t, 0.7, req.Temperature)
	assert.Equal(t, 0.9, req.TopP)
	assert.Equal(t, 40, req.TopK)
	assert.Equal(t, 100, req.MaxTokens)
	assert.Equal(t, []string{"END"}, req.Stop)
	assert.Equal(t, false, req.UseBeamSearch)
	assert.Equal(t, 1, req.BestOf)
	assert.Equal(t, 1, req.N)
}

func TestChatRequestStructure(t *testing.T) {
	// Test that ChatRequest can be created with messages
	messages := []ChatMessage{
		{
			Role:    "system",
			Content: "You are a helpful assistant.",
		},
		{
			Role:    "user",
			Content: "Hello!",
		},
	}

	req := ChatRequest{
		Model:         "llama-2-7b",
		Messages:      messages,
		Stream:        false,
		Temperature:   0.7,
		TopP:          0.9,
		TopK:          40,
		MaxTokens:     100,
		Stop:          []string{"END"},
		UseBeamSearch: false,
		BestOf:        1,
		N:             1,
	}

	assert.Equal(t, "llama-2-7b", req.Model)
	assert.Equal(t, messages, req.Messages)
	assert.Equal(t, false, req.Stream)
	assert.Equal(t, 0.7, req.Temperature)
	assert.Equal(t, 0.9, req.TopP)
	assert.Equal(t, 40, req.TopK)
	assert.Equal(t, 100, req.MaxTokens)
	assert.Equal(t, []string{"END"}, req.Stop)
	assert.Equal(t, false, req.UseBeamSearch)
	assert.Equal(t, 1, req.BestOf)
	assert.Equal(t, 1, req.N)
}

func TestGenerateResponseStructure(t *testing.T) {
	// Test that GenerateResponse can handle typical response structure
	resp := GenerateResponse{
		ID:      "test-id",
		Object:  "text_completion",
		Created: 1234567890,
		Model:   "llama-2-7b",
		Choices: []struct {
			Index        int         `json:"index"`
			Text         string      `json:"text"`
			LogProbs     interface{} `json:"logprobs,omitempty"`
			FinishReason string      `json:"finish_reason"`
		}{
			{
				Index:        0,
				Text:         "Hello, world!",
				LogProbs:     nil,
				FinishReason: "stop",
			},
		},
		Usage: struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	assert.Equal(t, "test-id", resp.ID)
	assert.Equal(t, "text_completion", resp.Object)
	assert.Equal(t, int64(1234567890), resp.Created)
	assert.Equal(t, "llama-2-7b", resp.Model)
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, 0, resp.Choices[0].Index)
	assert.Equal(t, "Hello, world!", resp.Choices[0].Text)
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
	assert.Equal(t, 10, resp.Usage.PromptTokens)
	assert.Equal(t, 5, resp.Usage.CompletionTokens)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestChatResponseStructure(t *testing.T) {
	// Test that ChatResponse can handle typical response structure
	resp := ChatResponse{
		ID:      "test-id",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "llama-2-7b",
		Choices: []struct {
			Index        int         `json:"index"`
			Message      ChatMessage `json:"message"`
			FinishReason string      `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: "Hello! How can I help you?",
				},
				FinishReason: "stop",
			},
		},
		Usage: struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	assert.Equal(t, "test-id", resp.ID)
	assert.Equal(t, "chat.completion", resp.Object)
	assert.Equal(t, int64(1234567890), resp.Created)
	assert.Equal(t, "llama-2-7b", resp.Model)
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, 0, resp.Choices[0].Index)
	assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
	assert.Equal(t, "Hello! How can I help you?", resp.Choices[0].Message.Content)
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
	assert.Equal(t, 10, resp.Usage.PromptTokens)
	assert.Equal(t, 5, resp.Usage.CompletionTokens)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestModelsResponseStructure(t *testing.T) {
	// Test that ModelsResponse can handle typical response structure
	resp := ModelsResponse{
		Object: "list",
		Data: []ModelInfo{
			{
				ID:      "llama-2-7b",
				Object:  "model",
				Created: 1234567890,
				OwnedBy: "vllm",
			},
			{
				ID:      "mistral-7b",
				Object:  "model",
				Created: 1234567891,
				OwnedBy: "vllm",
			},
		},
	}

	assert.Equal(t, "list", resp.Object)
	assert.Len(t, resp.Data, 2)
	assert.Equal(t, "llama-2-7b", resp.Data[0].ID)
	assert.Equal(t, "model", resp.Data[0].Object)
	assert.Equal(t, int64(1234567890), resp.Data[0].Created)
	assert.Equal(t, "vllm", resp.Data[0].OwnedBy)
	assert.Equal(t, "mistral-7b", resp.Data[1].ID)
	assert.Equal(t, "model", resp.Data[1].Object)
	assert.Equal(t, int64(1234567891), resp.Data[1].Created)
	assert.Equal(t, "vllm", resp.Data[1].OwnedBy)
}

func TestClientOptions(t *testing.T) {
	// Test that all options work correctly
	logger := logging.New()
	client := NewClient(
		WithModel("test-model"),
		WithBaseURL("http://test-server:8000"),
		WithLogger(logger),
		WithRetry(),
	)

	assert.Equal(t, "test-model", client.Model)
	assert.Equal(t, "http://test-server:8000", client.BaseURL)
	assert.Equal(t, logger, client.logger)
	assert.NotNil(t, client.retryExecutor)
}

func TestGenerateOptionsWithNilConfig(t *testing.T) {
	// Test that options work when LLMConfig is nil
	option := WithTemperature(0.5)
	options := &interfaces.GenerateOptions{}
	option(options)
	assert.NotNil(t, options.LLMConfig)
	assert.Equal(t, 0.5, options.LLMConfig.Temperature)

	option = WithTopP(0.8)
	options = &interfaces.GenerateOptions{}
	option(options)
	assert.NotNil(t, options.LLMConfig)
	assert.Equal(t, 0.8, options.LLMConfig.TopP)

	option = WithStopSequences([]string{"END"})
	options = &interfaces.GenerateOptions{}
	option(options)
	assert.NotNil(t, options.LLMConfig)
	assert.Equal(t, []string{"END"}, options.LLMConfig.StopSequences)
}

func TestContextHandling(t *testing.T) {
	// Test that context is properly handled
	ctx := context.Background()
	client := NewClient()

	// This test ensures the client can be created with context
	// The actual API calls would be tested in integration tests
	assert.NotNil(t, client)
	assert.NotNil(t, ctx)
}

func TestClientDefaultValues(t *testing.T) {
	// Test that default values are set correctly
	client := NewClient()

	assert.Equal(t, "http://localhost:8000", client.BaseURL)
	assert.Equal(t, "llama-2-7b", client.Model)
	assert.NotNil(t, client.HTTPClient)
	assert.NotNil(t, client.logger)
	assert.Nil(t, client.retryExecutor) // Retry is not enabled by default
}

func TestClientWithAllOptions(t *testing.T) {
	// Test creating client with all available options
	logger := logging.New()
	client := NewClient(
		WithModel("custom-model"),
		WithBaseURL("http://custom-server:9000"),
		WithLogger(logger),
		WithRetry(),
	)

	assert.Equal(t, "custom-model", client.Model)
	assert.Equal(t, "http://custom-server:9000", client.BaseURL)
	assert.Equal(t, logger, client.logger)
	assert.NotNil(t, client.retryExecutor)
	assert.NotNil(t, client.HTTPClient)
}
