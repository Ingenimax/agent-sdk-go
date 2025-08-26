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
	Claude37Sonnet = "claude-3-7-sonnet-20250219"  // Supports thinking tokens
	ClaudeSonnet4  = "claude-sonnet-4-20250514"    // Latest model with thinking
	ClaudeOpus4    = "claude-opus-4-20250514"      // Latest Opus with thinking
	ClaudeOpus41   = "claude-opus-4-1-20250805"    // Latest Opus 4.1
)

// SupportsThinking returns true if the model supports thinking tokens
func SupportsThinking(model string) bool {
	supportedModels := []string{
		"claude-3-7-sonnet-20250219",
		"claude-sonnet-4-20250514",
		"claude-opus-4-20250514",
		"claude-opus-4-1-20250805",
	}
	
	for _, supportedModel := range supportedModels {
		if model == supportedModel {
			return true
		}
	}
	return false
}

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
	Model         string        `json:"model"`
	Messages      []Message     `json:"messages"`
	MaxTokens     int           `json:"max_tokens,omitempty"`
	Temperature   float64       `json:"temperature,omitempty"`
	TopP          float64       `json:"top_p,omitempty"`
	TopK          int           `json:"top_k,omitempty"`
	StopSequences []string      `json:"stop_sequences,omitempty"`
	System        string        `json:"system,omitempty"`
	Tools         []Tool        `json:"tools,omitempty"`
	ToolChoice    interface{}   `json:"tool_choice,omitempty"`
	Stream        bool          `json:"stream,omitempty"`
	MetadataKey   string        `json:"metadata,omitempty"`
	Thinking      *ReasoningSpec `json:"thinking,omitempty"` // Keep "thinking" for API compatibility
}

// ReasoningSpec represents the reasoning configuration for Anthropic API
// Note: API still uses "thinking" parameter name for compatibility
type ReasoningSpec struct {
	Type         string `json:"type"`           // "enabled" to enable reasoning
	BudgetTokens int    `json:"budget_tokens,omitempty"` // Optional token budget for reasoning
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

	// Check for organization ID in context, and add a default one if missing
	defaultOrgID := "default"
	if id, err := multitenancy.GetOrgID(ctx); err == nil {
		// Organization ID found in context, use it
		ctx = multitenancy.WithOrgID(ctx, id) // Ensure consistency in context
	} else {
		// Add default organization ID to context to prevent errors in tool execution
		ctx = multitenancy.WithOrgID(ctx, defaultOrgID)
	}

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
		defer func() {
			if closeErr := httpResp.Body.Close(); closeErr != nil {
				c.logger.Warn(ctx, "Failed to close response body", map[string]interface{}{
					"error": closeErr.Error(),
				})
			}
		}()

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
		switch role {
		case "assistant", "user":
			// These roles are the same in Anthropic
		case "tool":
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
		defer func() {
			if closeErr := httpResp.Body.Close(); closeErr != nil {
				c.logger.Warn(ctx, "Failed to close response body", map[string]interface{}{
					"error": closeErr.Error(),
				})
			}
		}()

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

	// Set default max iterations if not provided
	maxIterations := params.MaxIterations
	if maxIterations == 0 {
		maxIterations = 2 // Default to current behavior
	}

	// Check for organization ID in context, and add a default one if missing
	defaultOrgID := "default"
	if id, err := multitenancy.GetOrgID(ctx); err == nil {
		// Organization ID found in context, use it
		ctx = multitenancy.WithOrgID(ctx, id) // Ensure consistency in context
	} else {
		// Add default organization ID to context to prevent errors in tool execution
		ctx = multitenancy.WithOrgID(ctx, defaultOrgID)
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

	// Track tool call repetitions for loop detection
	toolCallHistory := make(map[string]int)

	// Create messages array with user message
	messages := []Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	// Iterative tool calling loop
	for iteration := 0; iteration < maxIterations; iteration++ {
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
			"model":         c.Model,
			"temperature":   req.Temperature,
			"top_p":         req.TopP,
			"messages":      len(req.Messages),
			"tools":         len(req.Tools),
			"system":        req.System != "",
			"iteration":     iteration + 1,
			"maxIterations": maxIterations,
		})

		// Convert request to JSON
		reqBody, err := json.Marshal(req)
		if err != nil {
			return "", fmt.Errorf("failed to marshal request (iteration %d): %w", iteration+1, err)
		}

		// Create HTTP request
		httpReq, err := http.NewRequestWithContext(
			ctx,
			"POST",
			c.BaseURL+"/v1/messages",
			bytes.NewBuffer(reqBody),
		)
		if err != nil {
			return "", fmt.Errorf("failed to create request (iteration %d): %w", iteration+1, err)
		}

		// Set headers
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-API-Key", c.APIKey)
		httpReq.Header.Set("Anthropic-Version", "2023-06-01")

		// Send request
		httpResp, err := c.HTTPClient.Do(httpReq)
		if err != nil {
			c.logger.Error(ctx, "Error from Anthropic API", map[string]interface{}{
				"error":     err.Error(),
				"model":     c.Model,
				"iteration": iteration + 1,
			})
			return "", fmt.Errorf("failed to send request (iteration %d): %w", iteration+1, err)
		}
		defer func() {
			if closeErr := httpResp.Body.Close(); closeErr != nil {
				c.logger.Warn(ctx, "Failed to close response body", map[string]interface{}{
					"error": closeErr.Error(),
				})
			}
		}()

		// Read response body
		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response body (iteration %d): %w", iteration+1, err)
		}

		// Check for error response
		if httpResp.StatusCode != http.StatusOK {
			c.logger.Error(ctx, "Error from Anthropic API", map[string]interface{}{
				"status_code": httpResp.StatusCode,
				"response":    string(respBody),
				"model":       c.Model,
				"iteration":   iteration + 1,
			})
			return "", fmt.Errorf("error from Anthropic API (iteration %d): %s", iteration+1, string(respBody))
		}

		// Unmarshal response
		var resp CompletionResponse
		err = json.Unmarshal(respBody, &resp)
		if err != nil {
			return "", fmt.Errorf("failed to unmarshal response (iteration %d): %w", iteration+1, err)
		}

		// Log the raw response for debugging
		c.logger.Debug(ctx, "Raw response from Anthropic", map[string]interface{}{
			"response":  string(respBody),
			"iteration": iteration + 1,
		})

		// Make sure content is not nil
		if resp.Content == nil {
			c.logger.Error(ctx, "No content in response", map[string]interface{}{"iteration": iteration + 1})
			return "", fmt.Errorf("no content in response (iteration %d)", iteration+1)
		}

		// Check if the model wants to use tools
		var hasToolUse bool
		var toolCalls []ToolUse
		var textContent []string

		c.logger.Debug(ctx, "Response content blocks", map[string]interface{}{
			"numBlocks": len(resp.Content),
			"iteration": iteration + 1,
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
							"toolName":  toolName,
							"toolID":    block.ToolUse.ID,
							"iteration": iteration + 1,
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
			} else if contentBlock.Type == "text" {
				textContent = append(textContent, contentBlock.Text)
			}
		}

		c.logger.Debug(ctx, "Tool use detection results", map[string]interface{}{
			"hasToolUse": hasToolUse,
			"toolCalls":  len(toolCalls),
			"iteration":  iteration + 1,
		})

		// If no tool use, return the text content
		if !hasToolUse {
			if len(textContent) == 0 {
				return "", fmt.Errorf("no text content in response (iteration %d)", iteration+1)
			}
			return strings.Join(textContent, "\n"), nil
		}

		// The model wants to use tools
		c.logger.Info(ctx, "Processing tool calls", map[string]interface{}{
			"count":     len(toolCalls),
			"iteration": iteration + 1,
		})

		// Add the assistant response to messages
		messages = append(messages, Message{
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
				c.logger.Error(ctx, "Tool call missing both Name and RecipientName", map[string]interface{}{"iteration": iteration + 1})
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
					"toolName":  toolName,
					"iteration": iteration + 1,
					"availableTools": func() []string {
						names := make([]string, len(tools))
						for i, t := range tools {
							names[i] = t.Name()
						}
						return names
					}(),
				})
				
				// Add tool not found error as tool result instead of returning
				errorMessage := fmt.Sprintf("Error: tool not found: %s", toolName)
				
				// Store failed tool call in memory if provided
				if params.Memory != nil {
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:       "assistant",
						Content:    "",
						ToolCalls: []interfaces.ToolCall{{
							ID:        toolCall.ID,
							Name:      toolName,
							Arguments: "{}",
						}},
					})
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:       "tool",
						Content:    errorMessage,
						ToolCallID: toolCall.ID,
						Metadata: map[string]interface{}{
							"tool_name": toolName,
						},
					})
				}
				
				// Add error as tool result
				toolResults = append(toolResults, ToolResult{
					Type:     "tool_result",
					Content:  errorMessage,
					ToolName: toolName,
				})
				
				continue // Continue processing other tool calls
			}

			// Get parameters - could be in either Input or Parameters field
			var parameters map[string]interface{}
			if len(toolCall.Input) > 0 {
				parameters = toolCall.Input
			} else if len(toolCall.Parameters) > 0 {
				parameters = toolCall.Parameters
			}

			// Convert parameters to JSON string
			toolCallJSON, err := json.Marshal(parameters)
			if err != nil {
				c.logger.Error(ctx, "Error marshalling parameters", map[string]interface{}{
					"error":      err.Error(),
					"parameters": parameters,
					"iteration":  iteration + 1,
				})
				return "", fmt.Errorf("error marshalling parameters (iteration %d): %w", iteration+1, err)
			}

			// Log parameters for debugging
			c.logger.Debug(ctx, "Tool parameters", map[string]interface{}{
				"toolName":   toolName,
				"parameters": string(toolCallJSON),
				"iteration":  iteration + 1,
			})

			// Execute the tool
			c.logger.Info(ctx, "Executing tool", map[string]interface{}{
				"toolName":  selectedTool.Name(),
				"iteration": iteration + 1,
			})
			toolResult, err := selectedTool.Execute(ctx, string(toolCallJSON))

			// Check for repetitive calls and add warning if needed
			cacheKey := toolName + ":" + string(toolCallJSON)
			toolCallHistory[cacheKey]++

			if toolCallHistory[cacheKey] > 2 {
				warning := fmt.Sprintf("\n\n[WARNING: This is call #%d to %s with identical parameters. You may be in a loop. Consider using the available information to provide a final answer.]",
					toolCallHistory[cacheKey],
					toolName)
				if err == nil {
					toolResult += warning
				}
				c.logger.Warn(ctx, "Repetitive tool call detected", map[string]interface{}{
					"toolName":  toolName,
					"callCount": toolCallHistory[cacheKey],
					"iteration": iteration + 1,
				})
			}
			
			// Store tool call and result in memory if provided
			if params.Memory != nil {
				if err != nil {
					// Store failed tool call result
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:       "assistant",
						Content:    "",
						ToolCalls: []interfaces.ToolCall{{
							ID:        toolCall.ID,
							Name:      toolName,
							Arguments: string(toolCallJSON),
						}},
					})
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:       "tool",
						Content:    fmt.Sprintf("Error: %v", err),
						ToolCallID: toolCall.ID,
						Metadata: map[string]interface{}{
							"tool_name": toolName,
						},
					})
				} else {
					// Store successful tool call and result
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:       "assistant",
						Content:    "",
						ToolCalls: []interfaces.ToolCall{{
							ID:        toolCall.ID,
							Name:      toolName,
							Arguments: string(toolCallJSON),
						}},
					})
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:       "tool",
						Content:    toolResult,
						ToolCallID: toolCall.ID,
						Metadata: map[string]interface{}{
							"tool_name": toolName,
						},
					})
				}
			}
			
			if err != nil {
				c.logger.Error(ctx, "Error executing tool", map[string]interface{}{
					"toolName":  selectedTool.Name(),
					"error":     err.Error(),
					"iteration": iteration + 1,
				})
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
			return "", fmt.Errorf("failed to marshal tool results (iteration %d): %w", iteration+1, err)
		}

		// Add a user message with the tool results
		messages = append(messages, Message{
			Role:    "user",
			Content: fmt.Sprintf("Here are the tool results: %s", string(toolResultsJSON)),
		})

		// Continue to the next iteration with updated messages
	}

	// If we've reached the maximum iterations and the model is still requesting tools,
	// make one final call without tools to get a conclusion
	c.logger.Info(ctx, "Maximum iterations reached, making final call without tools", map[string]interface{}{
		"maxIterations": maxIterations,
	})

	// Create a final request without tools to force the LLM to provide a conclusion
	finalReq := CompletionRequest{
		Model:       c.Model,
		Messages:    messages,
		MaxTokens:   2048,
		Temperature: params.LLMConfig.Temperature,
		TopP:        params.LLMConfig.TopP,
		Tools:       nil, // No tools for final call
	}

	// Add system message if available
	if params.SystemMessage != "" {
		finalReq.System = params.SystemMessage
	}

	// Add a user message to encourage conclusion
	messages = append(messages, Message{
		Role:    "user",
		Content: "Please provide your final response based on the information available. Do not request any additional tools.",
	})
	finalReq.Messages = messages

	c.logger.Debug(ctx, "Making final request without tools", map[string]interface{}{
		"messages": len(finalReq.Messages),
	})

	// Convert request to JSON
	finalReqBody, err := json.Marshal(finalReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal final request: %w", err)
	}

	// Create HTTP request
	finalHTTPReq, err := http.NewRequestWithContext(
		ctx,
		"POST",
		c.BaseURL+"/v1/messages",
		bytes.NewBuffer(finalReqBody),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create final request: %w", err)
	}

	// Set headers
	finalHTTPReq.Header.Set("Content-Type", "application/json")
	finalHTTPReq.Header.Set("X-API-Key", c.APIKey)
	finalHTTPReq.Header.Set("Anthropic-Version", "2023-06-01")

	// Send final request
	finalHTTPResp, err := c.HTTPClient.Do(finalHTTPReq)
	if err != nil {
		c.logger.Error(ctx, "Error in final call without tools", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("failed to send final request: %w", err)
	}
	defer func() {
		if closeErr := finalHTTPResp.Body.Close(); closeErr != nil {
			c.logger.Warn(ctx, "Failed to close final response body", map[string]interface{}{
				"error": closeErr.Error(),
			})
		}
	}()

	// Read final response body
	finalRespBody, err := io.ReadAll(finalHTTPResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read final response body: %w", err)
	}

	// Check for error response
	if finalHTTPResp.StatusCode != http.StatusOK {
		c.logger.Error(ctx, "Error from Anthropic API in final call", map[string]interface{}{
			"status_code": finalHTTPResp.StatusCode,
			"response":    string(finalRespBody),
		})
		return "", fmt.Errorf("error from Anthropic API in final call: %s", string(finalRespBody))
	}

	// Unmarshal final response
	var finalResp CompletionResponse
	err = json.Unmarshal(finalRespBody, &finalResp)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal final response: %w", err)
	}

	// Extract text content from final response
	if finalResp.Content == nil {
		return "", fmt.Errorf("no content in final response")
	}

	var finalTextContent []string
	for _, contentBlock := range finalResp.Content {
		if contentBlock.Type == "text" {
			finalTextContent = append(finalTextContent, contentBlock.Text)
		}
	}

	if len(finalTextContent) == 0 {
		return "", fmt.Errorf("no text content in final response")
	}

	c.logger.Info(ctx, "Successfully received final response without tools", nil)
	return strings.Join(finalTextContent, "\n"), nil
}

// Name implements interfaces.LLM.Name
func (c *AnthropicClient) Name() string {
	return "anthropic"
}

// SupportsStreaming implements interfaces.LLM.SupportsStreaming
func (c *AnthropicClient) SupportsStreaming() bool {
	return true
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
