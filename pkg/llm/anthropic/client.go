package anthropic

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
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
)

// AnthropicClient implements the LLM interface for Anthropic
type AnthropicClient struct {
	APIKey        string
	Model         string
	BaseURL       string
	HTTPClient    *http.Client
	logger        logging.Logger
	retryExecutor *retry.Executor
}

// Option represents an option for configuring the Anthropic client
type Option func(*AnthropicClient)

// WithModel sets the model for the Anthropic client
func WithModel(model string) Option {
	return func(c *AnthropicClient) {
		c.Model = model
	}
}

// WithLogger sets the logger for the Anthropic client
func WithLogger(logger logging.Logger) Option {
	return func(c *AnthropicClient) {
		c.logger = logger
	}
}

// WithRetry configures retry policy for the client
func WithRetry(opts ...retry.Option) Option {
	return func(c *AnthropicClient) {
		c.retryExecutor = retry.NewExecutor(retry.NewPolicy(opts...))
	}
}

// WithBaseURL sets the base URL for the Anthropic API
func WithBaseURL(baseURL string) Option {
	return func(c *AnthropicClient) {
		c.BaseURL = baseURL
	}
}

// WithHTTPClient sets the HTTP client for the Anthropic client
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *AnthropicClient) {
		c.HTTPClient = httpClient
	}
}

// NewClient creates a new Anthropic client
func NewClient(apiKey string, options ...Option) *AnthropicClient {
	// Create client with default options
	client := &AnthropicClient{
		APIKey:     apiKey,
		Model:      Claude37Sonnet,
		BaseURL:    "https://api.anthropic.com",
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		logger:     logging.New(),
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	// Log warning if model is not specified
	if client.Model == "" {
		client.logger.Warn(context.TODO(), "No model specified, model must be explicitly set with WithModel", nil)
	}

	return client
}

// ModelName constants for supported Anthropic models
const (
	// Claude 3 family
	Claude35Haiku  = "claude-3-5-haiku-latest"
	Claude35Sonnet = "claude-3-5-sonnet-latest"
	Claude3Opus    = "claude-3-opus-latest"
	Claude37Sonnet = "claude-3-7-sonnet-latest"
)

// Message represents a message for Anthropic API
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolUse represents a tool call for Anthropic API
type ToolUse struct {
	RecipientName string                 `json:"recipient_name"`
	Name          string                 `json:"name"`
	ID            string                 `json:"id"`
	Input         map[string]interface{} `json:"input"`
	Parameters    map[string]interface{} `json:"parameters"`
}

// ToolResult represents a tool result for Anthropic API
type ToolResult struct {
	Type     string `json:"type"`
	Content  string `json:"content"`
	ToolName string `json:"tool_name"`
}

// CompletionRequest represents a request for Anthropic API
type CompletionRequest struct {
	Model         string      `json:"model"`
	Messages      []Message   `json:"messages"`
	MaxTokens     int         `json:"max_tokens,omitempty"`
	Temperature   float64     `json:"temperature,omitempty"`
	TopP          float64     `json:"top_p,omitempty"`
	TopK          int         `json:"top_k,omitempty"`
	StopSequences []string    `json:"stop_sequences,omitempty"`
	System        string      `json:"system,omitempty"`
	Tools         []Tool      `json:"tools,omitempty"`
	ToolChoice    interface{} `json:"tool_choice,omitempty"`
	Stream        bool        `json:"stream,omitempty"`
	MetadataKey   string      `json:"metadata,omitempty"`
}

// Tool represents a tool definition for Anthropic API
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ContentBlock represents a content block in Anthropic API response
type ContentBlock struct {
	Type    string   `json:"type"`
	Text    string   `json:"text,omitempty"`
	ToolUse *ToolUse `json:"tool_use,omitempty"`
}

// CompletionResponse represents a response from Anthropic API
type CompletionResponse struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
	Usage      Usage          `json:"usage"`
}

// Usage represents token usage information
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// WithReasoning creates a GenerateOption to set the reasoning mode
// Note: Reasoning parameter is not supported in the current Anthropic API version.
// This option is kept for compatibility but will have no effect.
func WithReasoning(reasoning string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.Reasoning = reasoning
		// No actual functionality since reasoning is not supported
	}
}

// Generate generates text from a prompt
func (c *AnthropicClient) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// Check if model is specified
	if c.Model == "" {
		return "", fmt.Errorf("model not specified: use WithModel option when creating the client")
	}

	// Apply options
	params := &interfaces.GenerateOptions{
		LLMConfig: &interfaces.LLMConfig{
			Temperature: 0.7, // Default temperature
		},
	}

	// Apply user-provided options
	for _, option := range options {
		option(params)
	}

	// Get organization ID from context if available
	orgID, _ := multitenancy.GetOrgID(ctx)

	// Create request with messages
	messages := []Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	// Create request
	req := CompletionRequest{
		Model:       c.Model,
		Messages:    messages,
		MaxTokens:   2048,
		Temperature: params.LLMConfig.Temperature,
		TopP:        params.LLMConfig.TopP,
	}

	// Add system message if available
	if params.SystemMessage != "" {
		req.System = params.SystemMessage
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": params.SystemMessage})
	}

	// Add reasoning parameter if available
	if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
		c.logger.Debug(ctx, "Reasoning mode not supported in current API version", map[string]interface{}{"reasoning": params.LLMConfig.Reasoning})
	}

	if params.LLMConfig != nil {
		if len(params.LLMConfig.StopSequences) > 0 {
			req.StopSequences = params.LLMConfig.StopSequences
		}
	}

	// Set organization ID if available
	if orgID != "" {
		c.logger.Debug(ctx, "Organization ID ignored in current API version", map[string]interface{}{"organization": orgID})
	}

	var resp CompletionResponse
	var err error

	operation := func() error {
		c.logger.Debug(ctx, "Executing Anthropic API request", map[string]interface{}{
			"model":          c.Model,
			"temperature":    req.Temperature,
			"top_p":          req.TopP,
			"stop_sequences": req.StopSequences,
			"system":         req.System != "",
		})

		// Convert request to JSON
		reqBody, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		// Create HTTP request
		httpReq, err := http.NewRequestWithContext(
			ctx,
			"POST",
			c.BaseURL+"/v1/messages",
			bytes.NewBuffer(reqBody),
		)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-API-Key", c.APIKey)
		httpReq.Header.Set("Anthropic-Version", "2023-06-01")

		// Send request
		httpResp, err := c.HTTPClient.Do(httpReq)
		if err != nil {
			c.logger.Error(ctx, "Error from Anthropic API", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			return fmt.Errorf("failed to send request: %w", err)
		}
		defer httpResp.Body.Close()

		// Read response body
		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		// Check for error response
		if httpResp.StatusCode != http.StatusOK {
			c.logger.Error(ctx, "Error from Anthropic API", map[string]interface{}{
				"status_code": httpResp.StatusCode,
				"response":    string(respBody),
				"model":       c.Model,
			})
			return fmt.Errorf("error from Anthropic API: %s", string(respBody))
		}

		// Unmarshal response
		err = json.Unmarshal(respBody, &resp)
		if err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		return nil
	}

	if c.retryExecutor != nil {
		c.logger.Debug(ctx, "Using retry mechanism for Anthropic request", map[string]interface{}{
			"model": c.Model,
		})
		err = c.retryExecutor.Execute(ctx, operation)
	} else {
		err = operation()
	}

	if err != nil {
		return "", err
	}

	// Extract text from content blocks
	var contentText []string
	for _, block := range resp.Content {
		if block.Type == "text" {
			contentText = append(contentText, block.Text)
		}
	}

	if len(contentText) == 0 {
		return "", fmt.Errorf("no text content in response")
	}

	c.logger.Debug(ctx, "Successfully received response from Anthropic", map[string]interface{}{
		"model": c.Model,
	})

	return strings.Join(contentText, "\n"), nil
}

// Chat uses the messages API to have a conversation with a model
func (c *AnthropicClient) Chat(ctx context.Context, messages []llm.Message, params *llm.GenerateParams) (string, error) {
	// Check if model is specified
	if c.Model == "" {
		return "", fmt.Errorf("model not specified: use WithModel option when creating the client")
	}

	if params == nil {
		params = llm.DefaultGenerateParams()
	}

	// Convert messages to the Anthropic Chat format
	anthropicMessages := make([]Message, len(messages))
	var systemMessage string

	for i, msg := range messages {
		// Check if it's a system message
		if msg.Role == "system" {
			systemMessage = msg.Content
			// Skip this message in the regular messages array
			continue
		}

		// Map role names (Anthropic uses "assistant" and "user")
		role := msg.Role
		if role == "assistant" || role == "user" {
			// These roles are the same in Anthropic
		} else if role == "tool" {
			// Tool messages need special handling
			// For simplicity, we'll convert them to assistant messages
			role = "assistant"
		}

		anthropicMessages[i] = Message{
			Role:    role,
			Content: msg.Content,
		}
	}

	// Filter out any nil messages (from system messages being skipped)
	var filteredMessages []Message
	for _, msg := range anthropicMessages {
		if msg.Role != "" {
			filteredMessages = append(filteredMessages, msg)
		}
	}

	// Create chat request
	req := CompletionRequest{
		Model:         c.Model,
		Messages:      filteredMessages,
		MaxTokens:     2048,
		Temperature:   params.Temperature,
		TopP:          params.TopP,
		StopSequences: params.StopSequences,
	}

	// Add system message if available
	if systemMessage != "" {
		req.System = systemMessage
	}

	// Add reasoning parameter if available
	if params.Reasoning != "" {
		c.logger.Debug(ctx, "Reasoning mode not supported in current API version", map[string]interface{}{"reasoning": params.Reasoning})
	}

	var resp CompletionResponse
	var err error

	operation := func() error {
		c.logger.Debug(ctx, "Executing Anthropic Chat API request", map[string]interface{}{
			"model":          c.Model,
			"temperature":    req.Temperature,
			"top_p":          req.TopP,
			"stop_sequences": req.StopSequences,
			"messages":       len(req.Messages),
		})

		// Convert request to JSON
		reqBody, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		// Create HTTP request
		httpReq, err := http.NewRequestWithContext(
			ctx,
			"POST",
			c.BaseURL+"/v1/messages",
			bytes.NewBuffer(reqBody),
		)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-API-Key", c.APIKey)
		httpReq.Header.Set("Anthropic-Version", "2023-06-01")

		// Send request
		httpResp, err := c.HTTPClient.Do(httpReq)
		if err != nil {
			c.logger.Error(ctx, "Error from Anthropic Chat API", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			return fmt.Errorf("failed to send request: %w", err)
		}
		defer httpResp.Body.Close()

		// Read response body
		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		// Check for error response
		if httpResp.StatusCode != http.StatusOK {
			c.logger.Error(ctx, "Error from Anthropic Chat API", map[string]interface{}{
				"status_code": httpResp.StatusCode,
				"response":    string(respBody),
				"model":       c.Model,
			})
			return fmt.Errorf("error from Anthropic API: %s", string(respBody))
		}

		// Unmarshal response
		err = json.Unmarshal(respBody, &resp)
		if err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		return nil
	}

	if c.retryExecutor != nil {
		c.logger.Debug(ctx, "Using retry mechanism for Anthropic Chat request", map[string]interface{}{
			"model": c.Model,
		})
		err = c.retryExecutor.Execute(ctx, operation)
	} else {
		err = operation()
	}

	if err != nil {
		return "", err
	}

	// Extract text from content blocks
	var contentText []string
	for _, block := range resp.Content {
		if block.Type == "text" {
			contentText = append(contentText, block.Text)
		}
	}

	if len(contentText) == 0 {
		return "", fmt.Errorf("no text content in response")
	}

	c.logger.Debug(ctx, "Successfully received chat response from Anthropic", map[string]interface{}{
		"model": c.Model,
	})

	return strings.Join(contentText, "\n"), nil
}

// GenerateWithTools implements interfaces.LLM.GenerateWithTools
func (c *AnthropicClient) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	// Check if model is specified
	if c.Model == "" {
		return "", fmt.Errorf("model not specified: use WithModel option when creating the client")
	}

	// Apply options
	params := &interfaces.GenerateOptions{
		LLMConfig: &interfaces.LLMConfig{
			Temperature: 0.7, // Default temperature
		},
	}

	// Apply user-provided options
	for _, opt := range options {
		if opt != nil {
			opt(params)
		}
	}

	// Check for organization ID in context, and add a default one if missing
	orgID := "default"
	if id, err := multitenancy.GetOrgID(ctx); err == nil {
		orgID = id
	} else {
		// Add default organization ID to context to prevent errors in tool execution
		ctx = multitenancy.WithOrgID(ctx, orgID)
	}

	// Convert tools to Anthropic format
	anthropicTools := make([]Tool, len(tools))
	for i, tool := range tools {
		// Convert ParameterSpec to JSON Schema
		properties := make(map[string]interface{})
		required := []string{}

		for name, param := range tool.Parameters() {
			properties[name] = map[string]interface{}{
				"type":        param.Type,
				"description": param.Description,
			}
			if param.Default != nil {
				properties[name].(map[string]interface{})["default"] = param.Default
			}
			if param.Required {
				required = append(required, name)
			}
			if param.Items != nil {
				properties[name].(map[string]interface{})["items"] = map[string]interface{}{
					"type": param.Items.Type,
				}
				if param.Items.Enum != nil {
					properties[name].(map[string]interface{})["items"].(map[string]interface{})["enum"] = param.Items.Enum
				}
			}
			if param.Enum != nil {
				properties[name].(map[string]interface{})["enum"] = param.Enum
			}
		}

		// Create the input schema for this tool
		inputSchema := map[string]interface{}{
			"type":       "object",
			"properties": properties,
			"required":   required,
		}

		anthropicTools[i] = Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: inputSchema,
		}
	}

	// Create messages array with user message
	messages := []Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	// Create request
	req := CompletionRequest{
		Model:       c.Model,
		Messages:    messages,
		MaxTokens:   2048,
		Temperature: params.LLMConfig.Temperature,
		TopP:        params.LLMConfig.TopP,
		Tools:       anthropicTools,
		// Auto use tools when needed
		ToolChoice: map[string]string{
			"type": "auto",
		},
	}

	// Add system message if available
	if params.SystemMessage != "" {
		req.System = params.SystemMessage
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": params.SystemMessage})
	}

	// Add reasoning parameter if available
	if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
		c.logger.Debug(ctx, "Reasoning mode not supported in current API version", map[string]interface{}{"reasoning": params.LLMConfig.Reasoning})
	}

	// Send request
	c.logger.Debug(ctx, "Sending request with tools to Anthropic", map[string]interface{}{
		"model":       c.Model,
		"temperature": req.Temperature,
		"top_p":       req.TopP,
		"messages":    len(req.Messages),
		"tools":       len(req.Tools),
		"system":      req.System != "",
	})

	// Convert request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(
		ctx,
		"POST",
		c.BaseURL+"/v1/messages",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.APIKey)
	httpReq.Header.Set("Anthropic-Version", "2023-06-01")

	// Send request
	httpResp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		c.logger.Error(ctx, "Error from Anthropic API", map[string]interface{}{
			"error": err.Error(),
			"model": c.Model,
		})
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer httpResp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for error response
	if httpResp.StatusCode != http.StatusOK {
		c.logger.Error(ctx, "Error from Anthropic API", map[string]interface{}{
			"status_code": httpResp.StatusCode,
			"response":    string(respBody),
			"model":       c.Model,
		})
		return "", fmt.Errorf("error from Anthropic API: %s", string(respBody))
	}

	// Unmarshal response
	var resp CompletionResponse
	err = json.Unmarshal(respBody, &resp)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Log the raw response for debugging
	c.logger.Debug(ctx, "Raw response from Anthropic", map[string]interface{}{
		"response": string(respBody),
	})

	// Make sure content is not nil
	if resp.Content == nil {
		c.logger.Error(ctx, "No content in response", nil)
		return "", fmt.Errorf("no content in response")
	}

	// Check if the model wants to use tools
	var hasToolUse bool
	var toolCalls []ToolUse

	c.logger.Debug(ctx, "Response content blocks", map[string]interface{}{
		"numBlocks": len(resp.Content),
		"blockTypes": func() []string {
			types := make([]string, len(resp.Content))
			for i, block := range resp.Content {
				types[i] = block.Type
				if block.Type == "tool_use" && block.ToolUse != nil {
					toolName := ""
					if block.ToolUse.Name != "" {
						toolName = block.ToolUse.Name
					} else if block.ToolUse.RecipientName != "" {
						toolName = block.ToolUse.RecipientName
					}
					c.logger.Debug(ctx, "Found tool use block", map[string]interface{}{
						"toolName": toolName,
						"toolID":   block.ToolUse.ID,
					})
				}
			}
			return types
		}(),
	})

	for _, contentBlock := range resp.Content {
		if contentBlock.Type == "tool_use" && contentBlock.ToolUse != nil {
			hasToolUse = true
			toolCalls = append(toolCalls, *contentBlock.ToolUse)
		}
	}

	c.logger.Debug(ctx, "Tool use detection results", map[string]interface{}{
		"hasToolUse": hasToolUse,
		"toolCalls":  len(toolCalls),
	})

	if hasToolUse {
		// The model wants to use tools
		c.logger.Info(ctx, "Processing tool calls", map[string]interface{}{"count": len(toolCalls)})

		// Create a new conversation with the initial messages and the assistant response
		messages := append(req.Messages, Message{
			Role: "assistant",
			// We don't need the content here as we'll be adding tool results
		})

		// Process each tool call
		var toolResults []ToolResult
		for _, toolCall := range toolCalls {
			// Get tool name - it could be in either Name or RecipientName field
			toolName := ""
			if toolCall.Name != "" {
				toolName = toolCall.Name
			} else if toolCall.RecipientName != "" {
				toolName = toolCall.RecipientName
			} else {
				c.logger.Error(ctx, "Tool call missing both Name and RecipientName", nil)
				continue
			}

			// Find the requested tool
			var selectedTool interfaces.Tool
			for _, tool := range tools {
				if tool.Name() == toolName {
					selectedTool = tool
					break
				}
			}

			if selectedTool == nil {
				c.logger.Error(ctx, "Tool not found", map[string]interface{}{
					"toolName": toolName,
					"availableTools": func() []string {
						names := make([]string, len(tools))
						for i, t := range tools {
							names[i] = t.Name()
						}
						return names
					}(),
				})
				return "", fmt.Errorf("tool not found: %s", toolName)
			}

			// Get parameters - could be in either Input or Parameters field
			var parameters map[string]interface{}
			if toolCall.Input != nil && len(toolCall.Input) > 0 {
				parameters = toolCall.Input
			} else if toolCall.Parameters != nil && len(toolCall.Parameters) > 0 {
				parameters = toolCall.Parameters
			} else {
				c.logger.Error(ctx, "Tool parameters missing", map[string]interface{}{"toolName": toolName})
				toolResults = append(toolResults, ToolResult{
					Type:     "tool_result",
					Content:  "Error: Tool parameters are missing",
					ToolName: toolName,
				})
				continue
			}

			// Convert parameters to JSON string
			paramsBytes, err := json.Marshal(parameters)
			if err != nil {
				c.logger.Error(ctx, "Error marshalling parameters", map[string]interface{}{
					"error":      err.Error(),
					"parameters": parameters,
				})
				return "", fmt.Errorf("error marshalling parameters: %w", err)
			}

			// Log parameters for debugging
			c.logger.Debug(ctx, "Tool parameters", map[string]interface{}{
				"toolName":   toolName,
				"parameters": string(paramsBytes),
			})

			// Execute the tool
			c.logger.Info(ctx, "Executing tool", map[string]interface{}{"toolName": selectedTool.Name()})
			toolResult, err := selectedTool.Execute(ctx, string(paramsBytes))
			if err != nil {
				c.logger.Error(ctx, "Error executing tool", map[string]interface{}{"toolName": selectedTool.Name(), "error": err.Error()})
				// Return error as tool result
				toolResults = append(toolResults, ToolResult{
					Type:     "tool_result",
					Content:  fmt.Sprintf("Error: %v", err),
					ToolName: toolName,
				})
				continue
			}

			// Add tool result
			toolResults = append(toolResults, ToolResult{
				Type:     "tool_result",
				Content:  toolResult,
				ToolName: toolName,
			})
		}

		// Create a new message from the user with the tool results
		toolResultsJSON, err := json.Marshal(toolResults)
		if err != nil {
			return "", fmt.Errorf("failed to marshal tool results: %w", err)
		}

		// Add a user message with the tool results
		messages = append(messages, Message{
			Role:    "user",
			Content: fmt.Sprintf("Here are the tool results: %s", string(toolResultsJSON)),
		})

		// Get the final response
		c.logger.Info(ctx, "Sending final request with tool results", nil)

		finalReq := CompletionRequest{
			Model:       c.Model,
			Messages:    messages,
			MaxTokens:   2048,
			Temperature: params.LLMConfig.Temperature,
			TopP:        params.LLMConfig.TopP,
		}

		// Add system message if available
		if params.SystemMessage != "" {
			finalReq.System = params.SystemMessage
		}

		// Add reasoning parameter if available
		if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
			c.logger.Debug(ctx, "Reasoning mode not supported in current API version", map[string]interface{}{"reasoning": params.LLMConfig.Reasoning})
		}

		// Convert request to JSON
		reqBody, err := json.Marshal(finalReq)
		if err != nil {
			return "", fmt.Errorf("failed to marshal final request: %w", err)
		}

		// Create HTTP request
		httpReq, err := http.NewRequestWithContext(
			ctx,
			"POST",
			c.BaseURL+"/v1/messages",
			bytes.NewBuffer(reqBody),
		)
		if err != nil {
			return "", fmt.Errorf("failed to create final request: %w", err)
		}

		// Set headers
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-API-Key", c.APIKey)
		httpReq.Header.Set("Anthropic-Version", "2023-06-01")

		// Send request
		httpResp, err := c.HTTPClient.Do(httpReq)
		if err != nil {
			c.logger.Error(ctx, "Error from final Anthropic API call", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			return "", fmt.Errorf("failed to send final request: %w", err)
		}
		defer httpResp.Body.Close()

		// Read response body
		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read final response body: %w", err)
		}

		// Check for error response
		if httpResp.StatusCode != http.StatusOK {
			c.logger.Error(ctx, "Error from final Anthropic API call", map[string]interface{}{
				"status_code": httpResp.StatusCode,
				"response":    string(respBody),
				"model":       c.Model,
			})
			return "", fmt.Errorf("error from final Anthropic API call: %s", string(respBody))
		}

		// Unmarshal response
		var finalResp CompletionResponse
		err = json.Unmarshal(respBody, &finalResp)
		if err != nil {
			return "", fmt.Errorf("failed to unmarshal final response: %w", err)
		}

		// Extract text from content blocks
		var contentText []string
		for _, block := range finalResp.Content {
			if block.Type == "text" {
				contentText = append(contentText, block.Text)
			}
		}

		if len(contentText) == 0 {
			return "", fmt.Errorf("no text content in final response")
		}

		return strings.Join(contentText, "\n"), nil
	}

	// No tool was used, return the direct response
	var contentText []string
	for _, block := range resp.Content {
		if block.Type == "text" {
			contentText = append(contentText, block.Text)
		}
	}

	c.logger.Debug(ctx, "Direct response without tool use", map[string]interface{}{
		"contentBlocks": len(resp.Content),
		"textBlocks":    len(contentText),
	})

	if len(contentText) == 0 {
		return "", fmt.Errorf("no text content in response")
	}

	return strings.Join(contentText, "\n"), nil
}

// Name implements interfaces.LLM.Name
func (c *AnthropicClient) Name() string {
	return "anthropic"
}

// WithTemperature creates a GenerateOption to set the temperature
func WithTemperature(temperature float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.LLMConfig.Temperature = temperature
	}
}

// WithTopP creates a GenerateOption to set the top_p
func WithTopP(topP float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.LLMConfig.TopP = topP
	}
}

// WithFrequencyPenalty creates a GenerateOption to set the frequency penalty
func WithFrequencyPenalty(frequencyPenalty float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.LLMConfig.FrequencyPenalty = frequencyPenalty
	}
}

// WithPresencePenalty creates a GenerateOption to set the presence penalty
func WithPresencePenalty(presencePenalty float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.LLMConfig.PresencePenalty = presencePenalty
	}
}

// WithStopSequences creates a GenerateOption to set the stop sequences
func WithStopSequences(stopSequences []string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.LLMConfig.StopSequences = stopSequences
	}
}

// WithSystemMessage creates a GenerateOption to set the system message
func WithSystemMessage(systemMessage string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.SystemMessage = systemMessage
	}
}

// WithResponseFormat creates a GenerateOption to set the response format
func WithResponseFormat(format interfaces.ResponseFormat) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.ResponseFormat = &format
	}
}
