package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
)

const (
	// DefaultBaseURL is the default DeepSeek API base URL
	DefaultBaseURL = "https://api.deepseek.com"

	// DefaultModel is the default DeepSeek model
	DefaultModel = "deepseek-chat"

	// DefaultMaxIterations is the default maximum number of tool calling iterations
	DefaultMaxIterations = 10
)

// DeepSeekClient implements the LLM interface for DeepSeek
type DeepSeekClient struct {
	APIKey        string
	Model         string
	BaseURL       string
	HTTPClient    *http.Client
	logger        logging.Logger
	retryExecutor *retry.Executor
}

// Option represents an option for configuring the DeepSeek client
type Option func(*DeepSeekClient)

// WithModel sets the model for the DeepSeek client
func WithModel(model string) Option {
	return func(c *DeepSeekClient) {
		c.Model = model
	}
}

// WithLogger sets the logger for the DeepSeek client
func WithLogger(logger logging.Logger) Option {
	return func(c *DeepSeekClient) {
		c.logger = logger
	}
}

// WithRetry configures retry policy for the client
func WithRetry(opts ...retry.Option) Option {
	return func(c *DeepSeekClient) {
		c.retryExecutor = retry.NewExecutor(retry.NewPolicy(opts...))
	}
}

// WithBaseURL sets the base URL for the DeepSeek client
func WithBaseURL(baseURL string) Option {
	return func(c *DeepSeekClient) {
		c.BaseURL = strings.TrimSuffix(baseURL, "/")
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) Option {
	return func(c *DeepSeekClient) {
		c.HTTPClient = client
	}
}

// NewClient creates a new DeepSeek client
func NewClient(apiKey string, options ...Option) *DeepSeekClient {
	client := &DeepSeekClient{
		APIKey:  apiKey,
		Model:   DefaultModel,
		BaseURL: DefaultBaseURL,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		logger: logging.New(),
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	return client
}

// Name returns the name of the LLM provider
func (c *DeepSeekClient) Name() string {
	return "deepseek"
}

// SupportsStreaming returns true if this LLM supports streaming
func (c *DeepSeekClient) SupportsStreaming() bool {
	return true
}

// ChatCompletionRequest represents a request to the DeepSeek Chat Completion API
type ChatCompletionRequest struct {
	Model            string                   `json:"model"`
	Messages         []Message                `json:"messages"`
	Temperature      float64                  `json:"temperature,omitempty"`
	TopP             float64                  `json:"top_p,omitempty"`
	FrequencyPenalty float64                  `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64                  `json:"presence_penalty,omitempty"`
	Stop             []string                 `json:"stop,omitempty"`
	MaxTokens        int                      `json:"max_tokens,omitempty"`
	Stream           bool                     `json:"stream,omitempty"`
	Tools            []Tool                   `json:"tools,omitempty"`
	ToolChoice       interface{}              `json:"tool_choice,omitempty"`
	ResponseFormat   *ResponseFormatParam     `json:"response_format,omitempty"`
}

// Message represents a message in the chat
type Message struct {
	Role       string      `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Name       string      `json:"name,omitempty"`
}

// ToolCall represents a tool call in the response
type ToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function FunctionCall   `json:"function"`
}

// FunctionCall represents a function call
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool represents a tool/function definition
type Tool struct {
	Type     string        `json:"type"`
	Function FunctionDef   `json:"function"`
}

// FunctionDef represents a function definition
type FunctionDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ResponseFormatParam represents the response format parameter
type ResponseFormatParam struct {
	Type       string      `json:"type"`
	JSONSchema interface{} `json:"json_schema,omitempty"`
}

// ChatCompletionResponse represents a response from the DeepSeek Chat Completion API
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Generate generates text based on the provided prompt
func (c *DeepSeekClient) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	response, err := c.GenerateDetailed(ctx, prompt, options...)
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// GenerateDetailed generates text and returns detailed response information including token usage
func (c *DeepSeekClient) GenerateDetailed(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	// Apply options
	params := &interfaces.GenerateOptions{
		LLMConfig: &interfaces.LLMConfig{
			Temperature: 0.7,
		},
	}

	for _, option := range options {
		option(params)
	}

	// Get organization ID from context if available
	orgID, _ := multitenancy.GetOrgID(ctx)

	// Build messages
	messages := []Message{}

	// Add system message if available
	if params.SystemMessage != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: params.SystemMessage,
		})
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": params.SystemMessage})
	}

	// Build messages using message history builder
	builder := newMessageHistoryBuilder(c.logger)
	messages = append(messages, builder.buildMessages(ctx, prompt, params.Memory)...)

	// Create request
	req := ChatCompletionRequest{
		Model:    c.Model,
		Messages: messages,
	}

	if params.LLMConfig != nil {
		req.Temperature = params.LLMConfig.Temperature
		req.TopP = params.LLMConfig.TopP
		req.FrequencyPenalty = params.LLMConfig.FrequencyPenalty
		req.PresencePenalty = params.LLMConfig.PresencePenalty
		if len(params.LLMConfig.StopSequences) > 0 {
			req.Stop = params.LLMConfig.StopSequences
		}
	}

	// Set response format if provided
	if params.ResponseFormat != nil {
		req.ResponseFormat = &ResponseFormatParam{
			Type:       "json_schema",
			JSONSchema: params.ResponseFormat.Schema,
		}
		c.logger.Debug(ctx, "Using response format", map[string]interface{}{"format": params.ResponseFormat})
	}

	var resp *ChatCompletionResponse
	var err error

	operation := func() error {
		c.logger.Debug(ctx, "Executing DeepSeek API request", map[string]interface{}{
			"model":             c.Model,
			"temperature":       req.Temperature,
			"top_p":             req.TopP,
			"frequency_penalty": req.FrequencyPenalty,
			"presence_penalty":  req.PresencePenalty,
			"stop_sequences":    req.Stop,
			"messages":          len(req.Messages),
			"response_format":   params.ResponseFormat != nil,
			"org_id":            orgID,
		})

		resp, err = c.doRequest(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "Error from DeepSeek API", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			return fmt.Errorf("failed to generate text: %w", err)
		}
		return nil
	}

	if c.retryExecutor != nil {
		c.logger.Debug(ctx, "Using retry mechanism for DeepSeek request", map[string]interface{}{
			"model": c.Model,
		})
		err = c.retryExecutor.Execute(ctx, operation)
	} else {
		err = operation()
	}

	if err != nil {
		return nil, err
	}

	// Return response
	if len(resp.Choices) > 0 {
		c.logger.Debug(ctx, "Successfully received response from DeepSeek", map[string]interface{}{
			"model": c.Model,
		})

		// Create detailed response with token usage
		response := &interfaces.LLMResponse{
			Content:    resp.Choices[0].Message.Content,
			Model:      resp.Model,
			StopReason: resp.Choices[0].FinishReason,
			Metadata: map[string]interface{}{
				"provider": "deepseek",
			},
		}

		// Extract token usage
		response.Usage = &interfaces.TokenUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		}

		return response, nil
	}

	return nil, fmt.Errorf("no response from DeepSeek API")
}

// doRequest performs an HTTP request to the DeepSeek API
func (c *DeepSeekClient) doRequest(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Marshal request to JSON
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/v1/chat/completions", c.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	// Make request
	httpResp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer httpResp.Body.Close()

	// Read response body
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for errors
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DeepSeek API error: status=%d, body=%s", httpResp.StatusCode, string(body))
	}

	// Parse response
	var resp ChatCompletionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// GenerateWithTools implements interfaces.LLM.GenerateWithTools
func (c *DeepSeekClient) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	response, err := c.GenerateWithToolsDetailed(ctx, prompt, tools, options...)
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// GenerateWithToolsDetailed implements interfaces.LLM.GenerateWithToolsDetailed
func (c *DeepSeekClient) GenerateWithToolsDetailed(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	// Apply options
	params := &interfaces.GenerateOptions{
		LLMConfig: &interfaces.LLMConfig{
			Temperature: 0.7,
		},
	}

	for _, option := range options {
		option(params)
	}

	// Set default max iterations if not provided
	maxIterations := params.MaxIterations
	if maxIterations == 0 {
		maxIterations = DefaultMaxIterations
	}

	// Get organization ID from context if available
	orgID, _ := multitenancy.GetOrgID(ctx)

	// Convert tools to DeepSeek format
	deepseekTools := c.convertToolsToDeepSeekFormat(tools)

	// Build initial messages
	messages := []Message{}

	// Add system message if available
	if params.SystemMessage != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: params.SystemMessage,
		})
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": params.SystemMessage})
	}

	// Build messages using message history builder
	builder := newMessageHistoryBuilder(c.logger)
	messages = append(messages, builder.buildMessages(ctx, prompt, params.Memory)...)

	// Track total token usage across all iterations
	var totalInputTokens, totalOutputTokens int

	// Iterative tool calling loop
	for iteration := 0; iteration < maxIterations; iteration++ {
		// Create request
		req := ChatCompletionRequest{
			Model:    c.Model,
			Messages: messages,
			Tools:    deepseekTools,
		}

		if params.LLMConfig != nil {
			req.Temperature = params.LLMConfig.Temperature
			req.TopP = params.LLMConfig.TopP
			req.FrequencyPenalty = params.LLMConfig.FrequencyPenalty
			req.PresencePenalty = params.LLMConfig.PresencePenalty
			if len(params.LLMConfig.StopSequences) > 0 {
				req.Stop = params.LLMConfig.StopSequences
			}
		}

		// Set response format if provided (only on last iteration)
		if params.ResponseFormat != nil && iteration == maxIterations-1 {
			req.ResponseFormat = &ResponseFormatParam{
				Type:       "json_schema",
				JSONSchema: params.ResponseFormat.Schema,
			}
		}

		c.logger.Debug(ctx, "Sending request with tools to DeepSeek", map[string]interface{}{
			"model":           c.Model,
			"temperature":     req.Temperature,
			"messages":        len(req.Messages),
			"tools":           len(req.Tools),
			"iteration":       iteration + 1,
			"maxIterations":   maxIterations,
			"org_id":          orgID,
		})

		// Make request
		resp, err := c.doRequest(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "Error from DeepSeek API", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			return nil, fmt.Errorf("failed to generate text with tools: %w", err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("no response from DeepSeek API")
		}

		// Accumulate token usage
		totalInputTokens += resp.Usage.PromptTokens
		totalOutputTokens += resp.Usage.CompletionTokens

		// Check if the model wants to use tools
		if len(resp.Choices[0].Message.ToolCalls) == 0 {
			// No tool calls, return the final response
			c.logger.Debug(ctx, "No tool calls, returning final response", map[string]interface{}{
				"iteration": iteration + 1,
			})

			return &interfaces.LLMResponse{
				Content:    resp.Choices[0].Message.Content,
				Model:      resp.Model,
				StopReason: resp.Choices[0].FinishReason,
				Usage: &interfaces.TokenUsage{
					InputTokens:  totalInputTokens,
					OutputTokens: totalOutputTokens,
					TotalTokens:  totalInputTokens + totalOutputTokens,
				},
				Metadata: map[string]interface{}{
					"provider":   "deepseek",
					"iterations": iteration + 1,
				},
			}, nil
		}

		// The model wants to use tools
		toolCalls := resp.Choices[0].Message.ToolCalls
		c.logger.Info(ctx, "Processing tool calls", map[string]interface{}{
			"count":     len(toolCalls),
			"iteration": iteration + 1,
		})

		// Add the assistant's message with tool calls to the conversation
		messages = append(messages, resp.Choices[0].Message)

		// Store assistant message with tool calls in memory
		if params.Memory != nil {
			memToolCalls := make([]interfaces.ToolCall, len(toolCalls))
			for i, tc := range toolCalls {
				memToolCalls[i] = interfaces.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				}
			}
			_ = params.Memory.AddMessage(ctx, interfaces.Message{
				Role:      interfaces.MessageRoleAssistant,
				Content:   resp.Choices[0].Message.Content,
				ToolCalls: memToolCalls,
			})
		}

		// Execute tools in parallel
		toolResults := c.executeToolsParallel(ctx, toolCalls, tools)

		// Add tool results to messages and memory
		for _, result := range toolResults {
			messages = append(messages, Message{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: result.ToolCallID,
				Name:       result.ToolName,
			})

			// Store tool result in memory
			if params.Memory != nil {
				_ = params.Memory.AddMessage(ctx, interfaces.Message{
					Role:       interfaces.MessageRoleTool,
					Content:    result.Content,
					ToolCallID: result.ToolCallID,
					Metadata: map[string]interface{}{
						"tool_name": result.ToolName,
					},
				})
			}
		}
	}

	// If we've exhausted max iterations, make one final call without tools
	c.logger.Warn(ctx, "Max iterations reached, making final call without tools", map[string]interface{}{
		"maxIterations": maxIterations,
	})

	req := ChatCompletionRequest{
		Model:    c.Model,
		Messages: messages,
	}

	if params.LLMConfig != nil {
		req.Temperature = params.LLMConfig.Temperature
		req.TopP = params.LLMConfig.TopP
		req.FrequencyPenalty = params.LLMConfig.FrequencyPenalty
		req.PresencePenalty = params.LLMConfig.PresencePenalty
		if len(params.LLMConfig.StopSequences) > 0 {
			req.Stop = params.LLMConfig.StopSequences
		}
	}

	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to make final request: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from DeepSeek API")
	}

	totalInputTokens += resp.Usage.PromptTokens
	totalOutputTokens += resp.Usage.CompletionTokens

	return &interfaces.LLMResponse{
		Content:    resp.Choices[0].Message.Content,
		Model:      resp.Model,
		StopReason: resp.Choices[0].FinishReason,
		Usage: &interfaces.TokenUsage{
			InputTokens:  totalInputTokens,
			OutputTokens: totalOutputTokens,
			TotalTokens:  totalInputTokens + totalOutputTokens,
		},
		Metadata: map[string]interface{}{
			"provider":       "deepseek",
			"iterations":     maxIterations,
			"max_iterations": true,
		},
	}, nil
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	ToolCallID string
	ToolName   string
	Content    string
}

// executeToolsParallel executes multiple tools in parallel
func (c *DeepSeekClient) executeToolsParallel(ctx context.Context, toolCalls []ToolCall, tools []interfaces.Tool) []ToolResult {
	type result struct {
		index      int
		toolCallID string
		toolName   string
		content    string
		err        error
	}

	resultCh := make(chan result, len(toolCalls))
	var wg sync.WaitGroup

	// Execute each tool call in a goroutine
	for i, toolCall := range toolCalls {
		wg.Add(1)
		go func(index int, tc ToolCall) {
			defer wg.Done()

			// Find the tool
			var tool interfaces.Tool
			for _, t := range tools {
				if t.Name() == tc.Function.Name {
					tool = t
					break
				}
			}

			if tool == nil {
				c.logger.Error(ctx, "Tool not found", map[string]interface{}{
					"tool_name": tc.Function.Name,
				})
				resultCh <- result{
					index:      index,
					toolCallID: tc.ID,
					toolName:   tc.Function.Name,
					content:    fmt.Sprintf("Error: tool '%s' not found", tc.Function.Name),
					err:        fmt.Errorf("tool not found: %s", tc.Function.Name),
				}
				return
			}

			// Execute the tool
			c.logger.Info(ctx, "Executing tool", map[string]interface{}{
				"tool_name": tc.Function.Name,
				"arguments": tc.Function.Arguments,
			})

			content, err := tool.Execute(ctx, tc.Function.Arguments)
			if err != nil {
				c.logger.Error(ctx, "Tool execution failed", map[string]interface{}{
					"tool_name": tc.Function.Name,
					"error":     err.Error(),
				})
				content = fmt.Sprintf("Error executing tool: %v", err)
			}

			resultCh <- result{
				index:      index,
				toolCallID: tc.ID,
				toolName:   tc.Function.Name,
				content:    content,
				err:        nil,
			}
		}(i, toolCall)
	}

	// Wait for all goroutines to finish
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results
	results := make([]ToolResult, len(toolCalls))
	for r := range resultCh {
		results[r.index] = ToolResult{
			ToolCallID: r.toolCallID,
			ToolName:   r.toolName,
			Content:    r.content,
		}
	}

	return results
}

// convertToolsToDeepSeekFormat converts SDK tools to DeepSeek API format
func (c *DeepSeekClient) convertToolsToDeepSeekFormat(tools []interfaces.Tool) []Tool {
	deepseekTools := make([]Tool, len(tools))

	for i, tool := range tools {
		// Convert parameters to JSON schema
		properties := make(map[string]interface{})
		required := []string{}

		for name, param := range tool.Parameters() {
			propDef := map[string]interface{}{
				"type":        param.Type,
				"description": param.Description,
			}

			if param.Default != nil {
				propDef["default"] = param.Default
			}

			if param.Items != nil {
				propDef["items"] = map[string]interface{}{
					"type": param.Items.Type,
				}
				if param.Items.Enum != nil {
					propDef["items"].(map[string]interface{})["enum"] = param.Items.Enum
				}
			}

			if param.Enum != nil {
				propDef["enum"] = param.Enum
			}

			properties[name] = propDef

			if param.Required {
				required = append(required, name)
			}
		}

		deepseekTools[i] = Tool{
			Type: "function",
			Function: FunctionDef{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": properties,
					"required":   required,
				},
			},
		}
	}

	return deepseekTools
}
