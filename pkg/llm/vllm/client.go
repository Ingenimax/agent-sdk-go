package vllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
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
	Tools         []Tool        `json:"tools,omitempty"`
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

	// ToolCalls is set on an assistant message when the model requests tools.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID pairs a tool-result message with the call that produced it.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// Tool is a function declaration in the OpenAI-compatible schema that vLLM
// serves at /v1/chat/completions.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function and its JSON Schema parameters.
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall is a function invocation requested by the model.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction carries the requested function name and its arguments,
// which arrive as a JSON-encoded string.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
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

	// Build prompt with memory context
	finalPrompt := c.buildPromptWithMemory(ctx, prompt, params)

	// Create request
	req := GenerateRequest{
		Model:       c.Model,
		Prompt:      finalPrompt,
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

// GenerateWithTools generates text using vLLM's native tool calling.
//
// vLLM serves an OpenAI-compatible /v1/chat/completions endpoint, so tools are
// declared in the standard schema and the model replies with tool_calls.
//
// This previously stuffed tool names and descriptions into the prompt and
// returned whatever prose came back:
//
//	// For now, vLLM doesn't support tool calling in the same way as
//	// OpenAI/Anthropic. We'll implement a basic version that includes tool
//	// descriptions in the prompt
//
// Nothing parsed a call out of the reply and no tool was ever executed, so an
// agent configured with vLLM silently had no tools at all -- the same defect
// fixed for Ollama in #325.
func (c *VLLMClient) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	if len(tools) == 0 {
		return c.Generate(ctx, prompt, options...)
	}

	params := &interfaces.GenerateOptions{
		LLMConfig: &interfaces.LLMConfig{Temperature: 0.7},
	}
	for _, option := range options {
		option(params)
	}

	maxIterations := 2
	if params.MaxIterations > 0 {
		maxIterations = params.MaxIterations
	}

	vllmTools := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		vllmTools = append(vllmTools, Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  toolParametersSchema(tool),
			},
		})
	}

	var messages []ChatMessage
	if params.SystemMessage != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: params.SystemMessage})
	}
	messages = append(messages, ChatMessage{Role: "user", Content: prompt})

	for iter := 0; iter < maxIterations; iter++ {
		// Stop between iterations when the caller has gone.
		if err := ctx.Err(); err != nil {
			return "", err
		}

		req := ChatRequest{
			Model:       c.Model,
			Messages:    messages,
			Tools:       vllmTools,
			Stream:      false,
			Temperature: params.LLMConfig.Temperature,
			TopP:        params.LLMConfig.TopP,
			Stop:        params.LLMConfig.StopSequences,
		}

		resp, err := c.makeRequest(ctx, "/v1/chat/completions", req)
		if err != nil {
			return "", fmt.Errorf("failed to chat with tools: %w", err)
		}

		var chatResp ChatResponse
		if err := json.Unmarshal(resp, &chatResp); err != nil {
			return "", fmt.Errorf("failed to unmarshal tool-chat response: %w", err)
		}
		if len(chatResp.Choices) == 0 {
			return "", fmt.Errorf("no choices in chat response")
		}

		assistant := chatResp.Choices[0].Message

		// No tool calls means the model produced its final answer.
		if len(assistant.ToolCalls) == 0 {
			return assistant.Content, nil
		}

		// The assistant turn requesting the calls has to stay in the transcript,
		// or the tool results that follow reference a call the model cannot see.
		messages = append(messages, assistant)

		if params.Memory != nil {
			summaries := make([]interfaces.ToolCall, 0, len(assistant.ToolCalls))
			for _, call := range assistant.ToolCalls {
				summaries = append(summaries, interfaces.ToolCall{
					ID:        call.ID,
					Name:      call.Function.Name,
					Arguments: call.Function.Arguments,
				})
			}
			_ = params.Memory.AddMessage(ctx, interfaces.Message{
				Role:      interfaces.MessageRoleAssistant,
				Content:   assistant.Content,
				ToolCalls: summaries,
			})
		}

		byName := make(map[string]interfaces.Tool, len(tools))
		for _, tool := range tools {
			byName[tool.Name()] = tool
		}

		for _, call := range assistant.ToolCalls {
			callID := call.ID
			if callID == "" {
				// Some servers omit the id. Synthesize a stable one so the same
				// tool invoked twice in a turn does not collide.
				callID = fmt.Sprintf("vllm:%s:%d", call.Function.Name, iter)
			}

			var result string
			tool, ok := byName[call.Function.Name]
			if !ok {
				result = fmt.Sprintf("Error: tool %q is not available", call.Function.Name)
			} else if out, execErr := tool.Execute(ctx, call.Function.Arguments); execErr != nil {
				// Report the failure to the model rather than aborting the run,
				// so it can recover or explain.
				result = fmt.Sprintf("Error: %v", execErr)
			} else {
				result = out
			}

			messages = append(messages, ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: callID,
			})

			if params.Memory != nil {
				_ = params.Memory.AddMessage(ctx, interfaces.Message{
					Role:       interfaces.MessageRoleTool,
					Content:    result,
					ToolCallID: callID,
				})
			}
		}
	}

	// Out of iterations with the model still asking for tools. Return the last
	// assistant content rather than an error, matching the other providers.
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			return messages[i].Content, nil
		}
	}
	return "", fmt.Errorf("tool loop exhausted %d iterations without a final answer", maxIterations)
}

// toolParametersSchema renders a tool's parameters as a JSON Schema object, the
// shape the OpenAI-compatible tools API expects.
func toolParametersSchema(tool interfaces.Tool) map[string]interface{} {
	properties := map[string]interface{}{}
	var required []string

	for name, spec := range tool.Parameters() {
		property := map[string]interface{}{
			"type":        spec.Type,
			"description": spec.Description,
		}
		if spec.Default != nil {
			property["default"] = spec.Default
		}
		if len(spec.Enum) > 0 {
			property["enum"] = spec.Enum
		}
		if spec.Items != nil {
			items := map[string]interface{}{"type": spec.Items.Type}
			if len(spec.Items.Enum) > 0 {
				items["enum"] = spec.Items.Enum
			}
			property["items"] = items
		}
		properties[name] = property

		if spec.Required {
			required = append(required, name)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
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

// SupportsStreaming returns false as streaming is not yet implemented for VLLM
func (c *VLLMClient) SupportsStreaming() bool {
	return false
}

// GetModel returns the model name being used
func (c *VLLMClient) GetModel() string {
	return c.Model
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

// buildPromptWithMemory builds a prompt with memory context for prompt-based models
func (c *VLLMClient) buildPromptWithMemory(ctx context.Context, prompt string, params *interfaces.GenerateOptions) string {
	return memory.BuildInlineHistoryPrompt(ctx, prompt, params.Memory, c.logger)
}

// GenerateDetailed generates text and returns detailed response information including token usage
func (c *VLLMClient) GenerateDetailed(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	// Call the existing method and construct a detailed response
	content, err := c.Generate(ctx, prompt, options...)
	if err != nil {
		return nil, err
	}

	// Return a detailed response without usage information (vLLM doesn't provide token usage)
	return &interfaces.LLMResponse{
		Content:    content,
		Model:      c.Model,
		StopReason: "",
		Usage:      nil, // vLLM doesn't provide token usage information
		Metadata: map[string]interface{}{
			"provider": "vllm",
		},
	}, nil
}

// GenerateWithToolsDetailed generates text with tools and returns detailed response information including token usage
func (c *VLLMClient) GenerateWithToolsDetailed(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	// Call the existing method and construct a detailed response
	content, err := c.GenerateWithTools(ctx, prompt, tools, options...)
	if err != nil {
		return nil, err
	}

	// Return a detailed response without usage information
	return &interfaces.LLMResponse{
		Content:    content,
		Model:      c.Model,
		StopReason: "",
		Usage:      nil, // vLLM doesn't provide token usage information
		Metadata: map[string]interface{}{
			"provider":   "vllm",
			"tools_used": true,
		},
	}, nil
}
