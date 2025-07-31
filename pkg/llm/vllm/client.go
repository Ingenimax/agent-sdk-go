package vllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
)

// VLLMClient implements the LLM interface for vLLM
type VLLMClient struct {
	BaseURL       string
	HTTPClient    *http.Client
	Model         string
	logger        logging.Logger
	retryExecutor *retry.Executor
}

// Option represents an option for configuring the vLLM client
type Option func(*VLLMClient)

// WithModel sets the model for the vLLM client
func WithModel(model string) Option {
	return func(c *VLLMClient) {
		c.Model = model
	}
}

// WithLogger sets the logger for the vLLM client
func WithLogger(logger logging.Logger) Option {
	return func(c *VLLMClient) {
		c.logger = logger
	}
}

// WithRetry configures retry policy for the client
func WithRetry(opts ...retry.Option) Option {
	return func(c *VLLMClient) {
		c.retryExecutor = retry.NewExecutor(retry.NewPolicy(opts...))
	}
}

// WithBaseURL sets the base URL for the vLLM API
func WithBaseURL(baseURL string) Option {
	return func(c *VLLMClient) {
		c.BaseURL = baseURL
	}
}

// WithHTTPClient sets the HTTP client for the vLLM client
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *VLLMClient) {
		c.HTTPClient = httpClient
	}
}

// NewClient creates a new vLLM client
func NewClient(options ...Option) *VLLMClient {
	// Create client with default options
	client := &VLLMClient{
		BaseURL:    "http://localhost:8000",
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		Model:      "llama-2-7b",
		logger:     logging.New(),
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	return client
}

// vLLM API request/response structures
type GenerateRequest struct {
	Model         string   `json:"model"`
	Prompt        string   `json:"prompt"`
	Stream        bool     `json:"stream"`
	Temperature   float64  `json:"temperature,omitempty"`
	TopP          float64  `json:"top_p,omitempty"`
	TopK          int      `json:"top_k,omitempty"`
	MaxTokens     int      `json:"max_tokens,omitempty"`
	Stop          []string `json:"stop,omitempty"`
	UseBeamSearch bool     `json:"use_beam_search,omitempty"`
	BestOf        int      `json:"best_of,omitempty"`
	N             int      `json:"n,omitempty"`
}

type GenerateResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Text         string      `json:"text"`
		LogProbs     interface{} `json:"logprobs,omitempty"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type ChatRequest struct {
	Model         string        `json:"model"`
	Messages      []ChatMessage `json:"messages"`
	Stream        bool          `json:"stream"`
	Temperature   float64       `json:"temperature,omitempty"`
	TopP          float64       `json:"top_p,omitempty"`
	TopK          int           `json:"top_k,omitempty"`
	MaxTokens     int           `json:"max_tokens,omitempty"`
	Stop          []string      `json:"stop,omitempty"`
	UseBeamSearch bool          `json:"use_beam_search,omitempty"`
	BestOf        int           `json:"best_of,omitempty"`
	N             int           `json:"n,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Message      ChatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

// Generate generates text from a prompt
func (c *VLLMClient) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// Apply options
	params := &interfaces.GenerateOptions{
		LLMConfig: &interfaces.LLMConfig{
			Temperature: 0.7,
		},
	}

	for _, option := range options {
		option(params)
	}

	// Create request
	req := GenerateRequest{
		Model:       c.Model,
		Prompt:      prompt,
		Stream:      false,
		Temperature: params.LLMConfig.Temperature,
		TopP:        params.LLMConfig.TopP,
		Stop:        params.LLMConfig.StopSequences,
	}

	// Handle structured output if provided
	if params.ResponseFormat != nil && params.ResponseFormat.Type == interfaces.ResponseFormatJSON {
		// Add JSON schema to the prompt for vLLM
		schemaJSON, err := json.Marshal(params.ResponseFormat.Schema)
		if err != nil {
			return "", fmt.Errorf("failed to marshal JSON schema: %w", err)
		}

		schemaPrompt := fmt.Sprintf(`%s

Please respond with a valid JSON object that matches the following schema:

Schema Name: %s
JSON Schema: %s

Ensure your response is a valid JSON object that strictly follows the schema above.`,
			prompt,
			params.ResponseFormat.Name,
			string(schemaJSON))

		req.Prompt = schemaPrompt

	}

	// Make request
	resp, err := c.makeRequest(ctx, "/v1/completions", req)
	if err != nil {
		return "", fmt.Errorf("failed to generate text: %w", err)
	}

	var generateResp GenerateResponse
	if err := json.Unmarshal(resp, &generateResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(generateResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return generateResp.Choices[0].Text, nil
}

// GenerateWithTools generates text and can use tools
func (c *VLLMClient) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	// For now, vLLM doesn't support tool calling in the same way as OpenAI/Anthropic
	// We'll implement a basic version that includes tool descriptions in the prompt
	if len(tools) == 0 {
		return c.Generate(ctx, prompt, options...)
	}

	// Build tool descriptions
	var toolDescriptions []string
	for _, tool := range tools {
		toolDescriptions = append(toolDescriptions, fmt.Sprintf("- %s: %s", tool.Name(), tool.Description()))
	}

	// Create enhanced prompt with tool information
	enhancedPrompt := fmt.Sprintf(`%s

Available tools:
%s

Please respond to the user's request. If you need to use any tools, describe what you would do.`, prompt, strings.Join(toolDescriptions, "\n"))

	return c.Generate(ctx, enhancedPrompt, options...)
}

// Chat performs a chat completion with messages
func (c *VLLMClient) Chat(ctx context.Context, messages []llm.Message, params *llm.GenerateParams) (string, error) {
	if params == nil {
		params = llm.DefaultGenerateParams()
	}

	// Convert messages to vLLM format
	var chatMessages []ChatMessage
	for _, msg := range messages {
		chatMessages = append(chatMessages, ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Create request
	req := ChatRequest{
		Model:       c.Model,
		Messages:    chatMessages,
		Stream:      false,
		Temperature: params.Temperature,
		TopP:        params.TopP,
		Stop:        params.StopSequences,
	}

	// Make request
	resp, err := c.makeRequest(ctx, "/v1/chat/completions", req)
	if err != nil {
		return "", fmt.Errorf("failed to chat: %w", err)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(resp, &chatResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal chat response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in chat response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// Name returns the name of the LLM provider
func (c *VLLMClient) Name() string {
	return "vllm"
}

// makeRequest makes an HTTP request to the vLLM API
func (c *VLLMClient) makeRequest(ctx context.Context, endpoint string, payload interface{}) ([]byte, error) {
	// Marshal payload
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request with retry if configured
	var resp *http.Response
	if c.retryExecutor != nil {
		err = c.retryExecutor.Execute(ctx, func() error {
			var execErr error
			resp, execErr = c.HTTPClient.Do(req)
			return execErr
		})
	} else {
		resp, err = c.HTTPClient.Do(req)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// makeGETRequest makes a GET request to the vLLM API
func (c *VLLMClient) makeGETRequest(ctx context.Context, endpoint string) ([]byte, error) {
	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request with retry if configured
	var resp *http.Response
	if c.retryExecutor != nil {
		err = c.retryExecutor.Execute(ctx, func() error {
			var execErr error
			resp, execErr = c.HTTPClient.Do(req)
			return execErr
		})
	} else {
		resp, err = c.HTTPClient.Do(req)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// ListModels lists available models
func (c *VLLMClient) ListModels(ctx context.Context) ([]string, error) {
	resp, err := c.makeGETRequest(ctx, "/v1/models")
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	var modelsResponse ModelsResponse
	if err := json.Unmarshal(resp, &modelsResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal models response: %w", err)
	}

	var models []string
	for _, model := range modelsResponse.Data {
		models = append(models, model.ID)
	}

	return models, nil
}

// GenerateOption functions for vLLM
func WithTemperature(temperature float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.Temperature = temperature
	}
}

func WithTopP(topP float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.TopP = topP
	}
}

func WithStopSequences(stopSequences []string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.StopSequences = stopSequences
	}
}

func WithSystemMessage(systemMessage string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.SystemMessage = systemMessage
	}
}

func WithResponseFormat(format interfaces.ResponseFormat) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.ResponseFormat = &format
	}
}
