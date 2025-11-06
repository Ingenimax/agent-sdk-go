package ollama

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
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
)

// OllamaClient implements the LLM interface for Ollama
type OllamaClient struct {
	BaseURL       string
	HTTPClient    *http.Client
	Model         string
	logger        logging.Logger
	retryExecutor *retry.Executor
}

// Option represents an option for configuring the Ollama client
type Option func(*OllamaClient)

// WithModel sets the model for the Ollama client
func WithModel(model string) Option {
	return func(c *OllamaClient) {
		c.Model = model
	}
}

// WithLogger sets the logger for the Ollama client
func WithLogger(logger logging.Logger) Option {
	return func(c *OllamaClient) {
		c.logger = logger
	}
}

// WithRetry configures retry policy for the client
func WithRetry(opts ...retry.Option) Option {
	return func(c *OllamaClient) {
		c.retryExecutor = retry.NewExecutor(retry.NewPolicy(opts...))
	}
}

// WithBaseURL sets the base URL for the Ollama API
func WithBaseURL(baseURL string) Option {
	return func(c *OllamaClient) {
		c.BaseURL = baseURL
	}
}

// WithHTTPClient sets the HTTP client for the Ollama client
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *OllamaClient) {
		c.HTTPClient = httpClient
	}
}

// NewClient creates a new Ollama client
func NewClient(options ...Option) *OllamaClient {
	// Create client with default options
	client := &OllamaClient{
		BaseURL:    "http://localhost:11434",
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		Model:      "qwen3:0.6b",
		logger:     logging.New(),
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	return client
}

// Ollama API request/response structures
type GenerateRequest struct {
	Model     string   `json:"model"`
	Prompt    string   `json:"prompt"`
	Stream    bool     `json:"stream"`
	Options   *Options `json:"options,omitempty"`
	System    string   `json:"system,omitempty"`
	Template  string   `json:"template,omitempty"`
	Context   []int    `json:"context,omitempty"`
	Format    string   `json:"format,omitempty"`
	Raw       bool     `json:"raw,omitempty"`
	KeepAlive string   `json:"keep_alive,omitempty"`
	Images    []string `json:"images,omitempty"`
}

type Options struct {
	Temperature   float64  `json:"temperature,omitempty"`
	TopP          float64  `json:"top_p,omitempty"`
	TopK          int      `json:"top_k,omitempty"`
	NumPredict    int      `json:"num_predict,omitempty"`
	Stop          []string `json:"stop,omitempty"`
	RepeatPenalty float64  `json:"repeat_penalty,omitempty"`
	Seed          int      `json:"seed,omitempty"`
}

type GenerateResponse struct {
	Model              string `json:"model"`
	CreatedAt          string `json:"created_at"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	Context            []int  `json:"context,omitempty"`
	TotalDuration      int64  `json:"total_duration,omitempty"`
	LoadDuration       int64  `json:"load_duration,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
	EvalDuration       int64  `json:"eval_duration,omitempty"`
}

type ChatRequest struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	Stream    bool          `json:"stream"`
	Options   *Options      `json:"options,omitempty"`
	Format    string        `json:"format,omitempty"`
	KeepAlive string        `json:"keep_alive,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Model              string      `json:"model"`
	CreatedAt          string      `json:"created_at"`
	Message            ChatMessage `json:"message"`
	Done               bool        `json:"done"`
	TotalDuration      int64       `json:"total_duration,omitempty"`
	LoadDuration       int64       `json:"load_duration,omitempty"`
	PromptEvalCount    int         `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64       `json:"prompt_eval_duration,omitempty"`
	EvalCount          int         `json:"eval_count,omitempty"`
	EvalDuration       int64       `json:"eval_duration,omitempty"`
}

// Generate generates text from a prompt
func (c *OllamaClient) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// Apply options
	params := &interfaces.GenerateOptions{
		LLMConfig: &interfaces.LLMConfig{
			Temperature: 0.7,
		},
	}

	for _, option := range options {
		option(params)
	}

	// Build prompt with memory context
	finalPrompt := c.buildPromptWithMemory(ctx, prompt, params)

	// Create request
	req := GenerateRequest{
		Model:  c.Model,
		Prompt: finalPrompt,
		Stream: false,
		Options: &Options{
			Temperature: params.LLMConfig.Temperature,
			TopP:        params.LLMConfig.TopP,
			Stop:        params.LLMConfig.StopSequences,
		},
		System: params.SystemMessage,
	}

	// Handle structured output if provided
	if params.ResponseFormat != nil && params.ResponseFormat.Type == interfaces.ResponseFormatJSON {
		// Add JSON schema to the prompt for Ollama
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
		req.Format = "json"
	}

	// Make request
	resp, err := c.makeRequest(ctx, "/api/generate", req)
	if err != nil {
		return "", fmt.Errorf("failed to generate text: %w", err)
	}

	var generateResp GenerateResponse
	if err := json.Unmarshal(resp, &generateResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return generateResp.Response, nil
}

// GenerateWithTools generates text and can use tools
func (c *OllamaClient) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	// For now, Ollama doesn't support tool calling in the same way as OpenAI/Anthropic
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
func (c *OllamaClient) Chat(ctx context.Context, messages []llm.Message, params *llm.GenerateParams) (string, error) {
	// Convert messages to Ollama format
	var chatMessages []ChatMessage
	for _, msg := range messages {
		chatMessages = append(chatMessages, ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Create request
	req := ChatRequest{
		Model:    c.Model,
		Messages: chatMessages,
		Stream:   false,
		Options: &Options{
			Temperature: params.Temperature,
			TopP:        params.TopP,
			Stop:        params.StopSequences,
		},
	}

	// Make request
	resp, err := c.makeRequest(ctx, "/api/chat", req)
	if err != nil {
		return "", fmt.Errorf("failed to chat: %w", err)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(resp, &chatResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal chat response: %w", err)
	}

	return chatResp.Message.Content, nil
}

// Name returns the name of the LLM provider
func (c *OllamaClient) Name() string {
	return "ollama"
}

// SupportsStreaming returns false as streaming is not yet implemented for Ollama
func (c *OllamaClient) SupportsStreaming() bool {
	return false
}

// GetModel returns the model name being used
func (c *OllamaClient) GetModel() string {
	return c.Model
}

// makeRequest makes an HTTP request to the Ollama API
func (c *OllamaClient) makeRequest(ctx context.Context, endpoint string, payload interface{}) ([]byte, error) {
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
	defer func() {
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()

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
func (c *OllamaClient) ListModels(ctx context.Context) ([]string, error) {
	resp, err := c.makeRequest(ctx, "/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	var tagsResponse struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.Unmarshal(resp, &tagsResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal models response: %w", err)
	}

	var models []string
	for _, model := range tagsResponse.Models {
		models = append(models, model.Name)
	}

	return models, nil
}

// PullModel downloads a model
func (c *OllamaClient) PullModel(ctx context.Context, modelName string) error {
	req := struct {
		Name string `json:"name"`
	}{
		Name: modelName,
	}

	_, err := c.makeRequest(ctx, "/api/pull", req)
	if err != nil {
		return fmt.Errorf("failed to pull model %s: %w", modelName, err)
	}

	return nil
}

// GenerateOption functions for Ollama
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

// buildPromptWithMemory builds a prompt with memory context for prompt-based models
func (c *OllamaClient) buildPromptWithMemory(ctx context.Context, prompt string, params *interfaces.GenerateOptions) string {
	return memory.BuildInlineHistoryPrompt(ctx, prompt, params.Memory, c.logger)
}
