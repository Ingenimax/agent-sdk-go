package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// GenerateStream implements interfaces.StreamingLLM.GenerateStream
func (c *AnthropicClient) GenerateStream(
	ctx context.Context,
	prompt string,
	options ...interfaces.GenerateOption,
) (<-chan interfaces.StreamEvent, error) {
	// Check if model is specified
	if c.Model == "" {
		return nil, fmt.Errorf("model not specified: use WithModel option when creating the client")
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

	// Create messages array with user message
	messages := []Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	// Create request with streaming enabled
	// Note: MaxTokens must be greater than reasoning budget_tokens
	maxTokens := 2048 // default
	if params.LLMConfig != nil && params.LLMConfig.EnableReasoning && params.LLMConfig.ReasoningBudget > 0 {
		// Ensure max_tokens > budget_tokens for reasoning
		maxTokens = params.LLMConfig.ReasoningBudget + 4000 // Add buffer for actual response
	}
	
	req := CompletionRequest{
		Model:       c.Model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: params.LLMConfig.Temperature,
		TopP:        params.LLMConfig.TopP,
		Stream:      true, // Enable streaming
	}

	// Add system message if available
	if params.SystemMessage != "" {
		req.System = params.SystemMessage
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": params.SystemMessage})
	}

	// Add stop sequences if provided
	if params.LLMConfig != nil && len(params.LLMConfig.StopSequences) > 0 {
		req.StopSequences = params.LLMConfig.StopSequences
	}

	// Add reasoning (thinking) support if enabled and model supports it
	if params.LLMConfig != nil && params.LLMConfig.EnableReasoning {
		if SupportsThinking(c.Model) {
			req.Thinking = &ReasoningSpec{
				Type: "enabled",
			}
			if params.LLMConfig.ReasoningBudget > 0 {
				req.Thinking.BudgetTokens = params.LLMConfig.ReasoningBudget
			}
			// Anthropic requires temperature = 1.0 when thinking is enabled
			req.Temperature = 1.0
			c.logger.Debug(ctx, "Enabled reasoning (thinking) tokens", map[string]interface{}{
				"model":         c.Model,
				"budget_tokens": params.LLMConfig.ReasoningBudget,
				"max_tokens":    maxTokens,
				"temperature":   req.Temperature, // Show override
			})
		} else {
			c.logger.Warn(ctx, "Thinking tokens not supported by this model", map[string]interface{}{
				"model":           c.Model,
				"supported_models": []string{"claude-3-7-sonnet-20250219", "claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-opus-4-1-20250805"},
			})
		}
	}

	// Get buffer size from stream config
	bufferSize := 100 // default
	if params.StreamConfig != nil {
		bufferSize = params.StreamConfig.BufferSize
	}

	// Create event channel
	eventChan := make(chan interfaces.StreamEvent, bufferSize)

	// Start streaming in a goroutine
	go func() {
		defer func() {
			// Safe close with recovery
			defer func() {
				if r := recover(); r != nil {
					// Channel was already closed, ignore panic
					_ = r
				}
			}()
			close(eventChan)
		}()

		// Execute the streaming request
		if err := c.executeStreamingRequest(ctx, req, eventChan); err != nil {
			select {
			case eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventError,
				Error:     err,
				Timestamp: time.Now(),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventChan, nil
}

// executeStreamingRequest performs the actual streaming HTTP request
func (c *AnthropicClient) executeStreamingRequest(
	ctx context.Context,
	req CompletionRequest,
	eventChan chan<- interfaces.StreamEvent,
) error {
	operation := func() error {
		c.logger.Debug(ctx, "Executing Anthropic streaming API request", map[string]interface{}{
			"model":          c.Model,
			"temperature":    req.Temperature,
			"top_p":          req.TopP,
			"stop_sequences": req.StopSequences,
			"system":         req.System != "",
			"stream":         req.Stream,
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
		httpReq.Header.Set("Accept", "text/event-stream")
		httpReq.Header.Set("Cache-Control", "no-cache")

		// Send request
		httpResp, err := c.HTTPClient.Do(httpReq)
		if err != nil {
			c.logger.Error(ctx, "Error from Anthropic streaming API", map[string]interface{}{
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

		// Check for error response
		if httpResp.StatusCode != http.StatusOK {
			// Read the response body to get the actual error message
			var errorBody []byte
			if httpResp.Body != nil {
				errorBody, _ = io.ReadAll(httpResp.Body)
				_ = httpResp.Body.Close()
			}
			
			c.logger.Error(ctx, "Error from Anthropic streaming API", map[string]interface{}{
				"status_code":    httpResp.StatusCode,
				"model":          c.Model,
				"error_body":     string(errorBody),
				"content_type":   httpResp.Header.Get("Content-Type"),
			})
			
			if len(errorBody) > 0 {
				return fmt.Errorf("error from Anthropic API: HTTP %d - %s", httpResp.StatusCode, string(errorBody))
			}
			return fmt.Errorf("error from Anthropic API: HTTP %d", httpResp.StatusCode)
		}

		// Verify content type
		contentType := httpResp.Header.Get("Content-Type")
		if contentType != "text/event-stream" && contentType != "text/event-stream; charset=utf-8" {
			return fmt.Errorf("unexpected content type: %s", contentType)
		}

		// Create scanner for reading SSE stream
		scanner := bufio.NewScanner(httpResp.Body)

		// Parse SSE stream
		c.parseSSEStream(scanner, eventChan)

		c.logger.Debug(ctx, "Successfully completed Anthropic streaming request", map[string]interface{}{
			"model": c.Model,
		})

		return nil
	}

	// Use retry executor if available
	if c.retryExecutor != nil {
		c.logger.Debug(ctx, "Using retry mechanism for Anthropic streaming request", map[string]interface{}{
			"model": c.Model,
		})
		return c.retryExecutor.Execute(ctx, operation)
	}

	return operation()
}

// GenerateWithToolsStream implements interfaces.StreamingLLM.GenerateWithToolsStream
func (c *AnthropicClient) GenerateWithToolsStream(
	ctx context.Context,
	prompt string,
	tools []interfaces.Tool,
	options ...interfaces.GenerateOption,
) (<-chan interfaces.StreamEvent, error) {
	// Check if model is specified
	if c.Model == "" {
		return nil, fmt.Errorf("model not specified: use WithModel option when creating the client")
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

	// Get buffer size from stream config
	bufferSize := 100 // default
	if params.StreamConfig != nil {
		bufferSize = params.StreamConfig.BufferSize
	}

	// Create event channel
	eventChan := make(chan interfaces.StreamEvent, bufferSize)

	// Start streaming with tools in a goroutine
	go func() {
		defer func() {
			// Safe close with recovery
			defer func() {
				if r := recover(); r != nil {
					// Channel was already closed, ignore panic
					_ = r
				}
			}()
			close(eventChan)
		}()

		// Execute streaming with tools with iterative loop
		if err := c.executeStreamingWithTools(ctx, prompt, anthropicTools, tools, params, eventChan); err != nil {
			select {
			case eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventError,
				Error:     err,
				Timestamp: time.Now(),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventChan, nil
}

// executeStreamingWithTools handles streaming requests with tools using iterative loop
func (c *AnthropicClient) executeStreamingWithTools(
	ctx context.Context,
	prompt string,
	anthropicTools []Tool,
	originalTools []interfaces.Tool,
	params *interfaces.GenerateOptions,
	eventChan chan<- interfaces.StreamEvent,
) error {
	// Create messages array with user message
	messages := []Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	// Get maxIterations from params
	maxIterations := 2 // Default to match non-streaming behavior
	if params.MaxIterations > 0 {
		maxIterations = params.MaxIterations
	}

	// Create base request configuration
	maxTokens := 2048 // default
	if params.LLMConfig != nil && params.LLMConfig.EnableReasoning && params.LLMConfig.ReasoningBudget > 0 {
		// Ensure max_tokens > budget_tokens for reasoning
		maxTokens = params.LLMConfig.ReasoningBudget + 4000 // Add buffer for actual response
	}

	// Iterative tool calling loop
	for iteration := 0; iteration < maxIterations; iteration++ {
		// Create request for this iteration
		req := CompletionRequest{
			Model:       c.Model,
			Messages:    messages,
			MaxTokens:   maxTokens,
			Temperature: params.LLMConfig.Temperature,
			TopP:        params.LLMConfig.TopP,
			Tools:       anthropicTools,
			// Auto use tools when needed
			ToolChoice: map[string]string{
				"type": "auto",
			},
			Stream: true, // Enable streaming
		}

		// Add system message if available
		if params.SystemMessage != "" {
			req.System = params.SystemMessage
		}

		// Add reasoning (thinking) support if enabled and model supports it
		if params.LLMConfig != nil && params.LLMConfig.EnableReasoning {
			if SupportsThinking(c.Model) {
				req.Thinking = &ReasoningSpec{
					Type: "enabled",
				}
				if params.LLMConfig.ReasoningBudget > 0 {
					req.Thinking.BudgetTokens = params.LLMConfig.ReasoningBudget
				}
				// Anthropic requires temperature = 1.0 when thinking is enabled
				req.Temperature = 1.0
				c.logger.Debug(ctx, "Enabled reasoning (thinking) tokens for tools", map[string]interface{}{
					"model":         c.Model,
					"budget_tokens": params.LLMConfig.ReasoningBudget,
					"max_tokens":    maxTokens,
					"temperature":   req.Temperature,
					"iteration":     iteration + 1,
					"maxIterations": maxIterations,
				})
			}
		}

		// Execute streaming request and collect tool calls
		toolCalls, _, err := c.executeStreamingRequestWithToolCapture(ctx, req, eventChan)
		if err != nil {
			return err
		}

		// TODO: Remove debug logs after investigating streaming issues
		c.logger.Debug(ctx, "Tool iteration completed", map[string]interface{}{
			"iteration":      iteration + 1,
			"toolCallsCount": len(toolCalls),
			"maxIterations":  maxIterations,
		})

		// If no tool calls, we're done - return content like OpenAI does
		if len(toolCalls) == 0 {
			// TODO: Remove debug logs after investigating streaming issues
			c.logger.Debug(ctx, "No tool calls detected, ending iteration loop", map[string]interface{}{
				"iteration":     iteration + 1,
				"maxIterations": maxIterations,
			})
			// No tool calls, this means we have the final response
			return nil
		}

		// Execute tools and add results to conversation
		c.logger.Info(ctx, "Processing tool calls in streaming", map[string]interface{}{
			"count":     len(toolCalls),
			"iteration": iteration + 1,
		})

		// Add assistant message with tool calls
		assistantMessage := Message{
			Role:    "assistant",
			Content: "", // Empty content when using tools
		}

		// Convert tool calls to Anthropic format and add to message
		for _, toolCall := range toolCalls {
			assistantMessage.Content += fmt.Sprintf("[Tool: %s called]", toolCall.Name)
		}

		messages = append(messages, assistantMessage)

		// Execute each tool and add results
		for _, toolCall := range toolCalls {
			// Find the requested tool
			var selectedTool interfaces.Tool
			for _, tool := range originalTools {
				if tool.Name() == toolCall.Name {
					selectedTool = tool
					break
				}
			}

			if selectedTool == nil {
				c.logger.Error(ctx, "Tool not found in streaming", map[string]interface{}{
					"toolName": toolCall.Name,
				})
				
				// Add tool not found error as tool result instead of returning
				errorMessage := fmt.Sprintf("Error: tool not found: %s", toolCall.Name)
				
				// Add tool result message
				messages = append(messages, Message{
					Role:    "user", // Tool results come as user messages to Anthropic
					Content: fmt.Sprintf("Tool %s result: %s", toolCall.Name, errorMessage),
				})

				// Send tool result event with error
				select {
				case eventChan <- interfaces.StreamEvent{
					Type: interfaces.StreamEventToolResult,
					ToolCall: &interfaces.ToolCall{
						ID:        toolCall.ID,
						Name:      toolCall.Name,
						Arguments: toolCall.Arguments,
					},
					Content:   errorMessage,
					Timestamp: time.Now(),
				}:
				case <-ctx.Done():
					return ctx.Err()
				}
				
				continue // Continue processing other tool calls
			}

			// Execute tool
			c.logger.Info(ctx, "Executing tool in streaming", map[string]interface{}{
				"toolName":  toolCall.Name,
				"arguments": toolCall.Arguments,
				"iteration": iteration + 1,
			})

			toolResult, err := selectedTool.Execute(ctx, toolCall.Arguments)
			if err != nil {
				toolResult = fmt.Sprintf("Error: %v", err)
			}

			// Add tool result message
			messages = append(messages, Message{
				Role:    "user", // Tool results come as user messages to Anthropic
				Content: fmt.Sprintf("Tool %s result: %s", toolCall.Name, toolResult),
			})

			// Send tool result event
			select {
			case eventChan <- interfaces.StreamEvent{
				Type: interfaces.StreamEventToolResult,
				ToolCall: &interfaces.ToolCall{
					ID:        toolCall.ID,
					Name:      toolCall.Name,
					Arguments: toolCall.Arguments,
				},
				Content:   toolResult, // Tool result goes in Content field
				Timestamp: time.Now(),
			}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Continue to next iteration with updated conversation
	}

	// After all tool iterations, make a final call without tools to get the synthesized answer
	// This ensures the LLM provides a final response after processing all tool results
	// TODO: Remove debug logs after investigating streaming issues
	c.logger.Info(ctx, "Maximum iterations reached, making final call without tools", map[string]interface{}{
		"maxIterations":   maxIterations,
		"messages":        len(messages),
		"finalCallReason": "reached_max_iterations",
	})

	// Add a message to inform the LLM this is the final call
	finalMessages := append(messages, Message{
		Role:    "user",
		Content: "Please provide your final response based on the information available. Do not request any additional tools.",
	})

	finalReq := CompletionRequest{
		Model:       c.Model,
		Messages:    finalMessages,
		MaxTokens:   maxTokens,
		Temperature: params.LLMConfig.Temperature,
		TopP:        params.LLMConfig.TopP,
		// No tools in final request - we want a final answer
		Stream: true, // Enable streaming
	}

	// Add system message if available
	if params.SystemMessage != "" {
		finalReq.System = params.SystemMessage
	}

	// Add reasoning (thinking) support if enabled and model supports it
	if params.LLMConfig != nil && params.LLMConfig.EnableReasoning {
		if SupportsThinking(c.Model) {
			finalReq.Thinking = &ReasoningSpec{
				Type: "enabled",
			}
			if params.LLMConfig.ReasoningBudget > 0 {
				finalReq.Thinking.BudgetTokens = params.LLMConfig.ReasoningBudget
			}
			// Anthropic requires temperature = 1.0 when thinking is enabled
			finalReq.Temperature = 1.0
			c.logger.Debug(ctx, "Getting final answer with reasoning after tools", map[string]interface{}{
				"model":         c.Model,
				"budget_tokens": params.LLMConfig.ReasoningBudget,
				"max_tokens":    maxTokens,
				"temperature":   finalReq.Temperature,
			})
		}
	}

	// TODO: Remove debug logs after investigating streaming issues
	c.logger.Debug(ctx, "Making final request without tools", map[string]interface{}{
		"messages_count":      len(finalMessages),
		"final_message_added": "Please provide your final response based on the information available. Do not request any additional tools.",
		"has_tools":           len(finalReq.Tools) > 0,
	})

	// Execute final request to get synthesized answer
	err := c.executeStreamingRequest(ctx, finalReq, eventChan)
	// TODO: Remove debug logs after investigating streaming issues
	c.logger.Debug(ctx, "Final request completed", map[string]interface{}{
		"error": err,
	})
	return err
}

// executeStreamingRequestWithToolCapture executes a streaming request and captures tool calls
func (c *AnthropicClient) executeStreamingRequestWithToolCapture(
	ctx context.Context,
	req CompletionRequest,
	eventChan chan<- interfaces.StreamEvent,
) ([]interfaces.ToolCall, bool, error) {
	var toolCalls []interfaces.ToolCall
	var hasContent bool

	// Create temporary channel to capture events
	tempEventChan := make(chan interfaces.StreamEvent, 100)
	
	// Execute streaming request in goroutine
	go func() {
		defer func() {
			// Safe close with recovery
			defer func() {
				if r := recover(); r != nil {
					// Channel was already closed, ignore panic
					_ = r
				}
			}()
			close(tempEventChan)
		}()
		
		if err := c.executeStreamingRequest(ctx, req, tempEventChan); err != nil {
			select {
			case tempEventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventError,
				Error:     err,
				Timestamp: time.Now(),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Process events and capture tool calls
	var eventCount int
	for event := range tempEventChan {
		eventCount++
		// TODO: Remove debug logs after investigating streaming issues
		if eventCount <= 3 || event.Type == interfaces.StreamEventToolUse || event.Type == interfaces.StreamEventMessageStop {
			c.logger.Debug(ctx, "Processing stream event", map[string]interface{}{
				"eventNumber": eventCount,
				"eventType":   event.Type,
				"hasToolCall": event.ToolCall != nil,
				"hasContent":  event.Content != "",
			})
		}

		// Forward event to main channel
		select {
		case eventChan <- event:
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}

		// Capture tool calls
		if event.Type == interfaces.StreamEventToolUse && event.ToolCall != nil {
			// TODO: Remove debug logs after investigating streaming issues
			c.logger.Debug(ctx, "Captured tool call", map[string]interface{}{
				"toolName": event.ToolCall.Name,
				"toolID":   event.ToolCall.ID,
			})
			toolCalls = append(toolCalls, *event.ToolCall)
		}

		// Check for content
		if event.Type == interfaces.StreamEventContentDelta && event.Content != "" {
			hasContent = true
		}

		// Check for errors
		if event.Error != nil {
			return nil, false, event.Error
		}
	}

	// TODO: Remove debug logs after investigating streaming issues
	c.logger.Debug(ctx, "Tool capture completed", map[string]interface{}{
		"totalEvents":     eventCount,
		"toolCallsFound":  len(toolCalls),
		"hasContent":      hasContent,
	})

	return toolCalls, hasContent, nil
}

