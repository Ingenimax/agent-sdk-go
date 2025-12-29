package anthropic

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// GenerateStream implements interfaces.StreamingLLM.GenerateStream
func (c *AnthropicClient) GenerateStream(
	ctx context.Context,
	prompt string,
	options ...interfaces.GenerateOption,
) (<-chan interfaces.StreamEvent, error) {
	c.logger.Debug(ctx, "[LLM RESPONSE DEBUG] GenerateStream called (NO TOOLS)", map[string]interface{}{
		"model":        c.Model,
		"promptLength": len(prompt),
	})

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

	// Build messages using unified builder
	builder := newMessageHistoryBuilder(c.logger)
	messages := builder.buildMessages(ctx, prompt, params)

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
				"model":            c.Model,
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

		// Execute the streaming request with memory support
		c.logger.Debug(ctx, "[LLM RESPONSE DEBUG] Executing streaming request without tools", map[string]interface{}{
			"model":       c.Model,
			"hasMemory":   params != nil && params.Memory != nil,
			"temperature": req.Temperature,
		})
		if err := c.executeStreamingRequestWithMemory(ctx, req, eventChan, prompt, params); err != nil {
			c.logger.Error(ctx, "[LLM RESPONSE DEBUG] Streaming request failed", map[string]interface{}{
				"error": err.Error(),
			})
			select {
			case eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventError,
				Error:     err,
				Timestamp: time.Now(),
			}:
			case <-ctx.Done():
				return
			}
		} else {
			c.logger.Info(ctx, "[LLM RESPONSE DEBUG] Streaming request completed successfully (no tools)", map[string]interface{}{
				"model": c.Model,
			})
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
	return c.executeStreamingRequestWithMemory(ctx, req, eventChan, "", nil)
}

func (c *AnthropicClient) executeStreamingRequestWithMemory(
	ctx context.Context,
	req CompletionRequest,
	eventChan chan<- interfaces.StreamEvent,
	prompt string,
	params *interfaces.GenerateOptions,
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

		// Handle Bedrock streaming separately using AWS SDK
		if c.BedrockConfig != nil && c.BedrockConfig.Enabled {
			return c.executeBedrockStreaming(ctx, &req, eventChan)
		}

		// Create streaming HTTP request (supports both Vertex AI and standard Anthropic API)
		httpReq, err := c.createStreamingHTTPRequest(ctx, &req, "/v1/messages")
		if err != nil {
			return fmt.Errorf("failed to create streaming request: %w", err)
		}

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
				"status_code":  httpResp.StatusCode,
				"model":        c.Model,
				"error_body":   string(errorBody),
				"content_type": httpResp.Header.Get("Content-Type"),
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

		// Create scanner for reading SSE stream with larger buffer
		// Default scanner has 64KB max token size which can cut off large responses
		scanner := bufio.NewScanner(httpResp.Body)
		// Set buffer to handle large SSE data lines (up to 10MB)
		buf := make([]byte, 0, 64*1024)   // 64KB initial buffer
		scanner.Buffer(buf, 10*1024*1024) // Allow up to 10MB per line

		// Parse SSE stream
		_ = c.parseSSEStreamAndCapture(ctx, scanner, eventChan, req, prompt, params)

		// Check for scanner errors (including buffer overflow)
		if err := scanner.Err(); err != nil {
			c.logger.Error(ctx, "Scanner error while reading SSE stream", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			return fmt.Errorf("scanner error while reading SSE stream: %w", err)
		}

		c.logger.Debug(ctx, "Successfully completed Anthropic streaming request", map[string]interface{}{
			"model": c.Model,
		})

		return nil
	}

	// Use vertex retry executor with multi-region support if available
	if c.vertexRetryExecutor != nil {
		c.logger.Info(ctx, "Using Vertex retry mechanism with region rotation for streaming", map[string]interface{}{
			"model":          c.Model,
			"current_region": c.VertexConfig.GetCurrentRegion(),
		})
		return c.vertexRetryExecutor.Execute(ctx, operation)
	} else if c.retryExecutor != nil {
		c.logger.Info(ctx, "Using standard retry mechanism for Anthropic streaming request", map[string]interface{}{
			"model":                   c.Model,
			"vertex_config_available": c.VertexConfig != nil,
		})
		return c.retryExecutor.Execute(ctx, operation)
	} else {
		c.logger.Debug(ctx, "No retry mechanism configured for streaming", map[string]interface{}{
			"model": c.Model,
		})
		return operation()
	}
}

// GenerateWithToolsStream implements interfaces.StreamingLLM.GenerateWithToolsStream
func (c *AnthropicClient) GenerateWithToolsStream(
	ctx context.Context,
	prompt string,
	tools []interfaces.Tool,
	options ...interfaces.GenerateOption,
) (<-chan interfaces.StreamEvent, error) {
	c.logger.Debug(ctx, "[LLM RESPONSE DEBUG] GenerateWithToolsStream called (WITH TOOLS)", map[string]interface{}{
		"model":        c.Model,
		"promptLength": len(prompt),
		"toolsCount":   len(tools),
	})

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
	// Build messages using unified builder
	builder := newMessageHistoryBuilder(c.logger)
	messages := builder.buildMessages(ctx, prompt, params)

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

	gotCompleteResponse := false
	finalIterationCount := 0 // Track total iterations for logging after loop

	// Iterative tool calling loop
	for iteration := 0; iteration < maxIterations; iteration++ {
		finalIterationCount = iteration + 1 // Update the count
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
		c.logger.Debug(ctx, "[LLM RESPONSE DEBUG] Calling LLM for iteration", map[string]interface{}{
			"iteration":     iteration + 1,
			"maxIterations": maxIterations,
			"hasTools":      len(anthropicTools) > 0,
		})
		// First pass: Filter content deltas for internal iterations - only stream thinking and tool events
		// Check if we should include intermediate messages (default is false for backward compatibility)
		filterContentDeltas := true
		if params.StreamConfig != nil && params.StreamConfig.IncludeIntermediateMessages {
			filterContentDeltas = false
		}
		toolCalls, hasContent, capturedContentEvents, err := c.executeStreamingRequestWithToolCapture(ctx, req, eventChan, filterContentDeltas)
		if err != nil {
			c.logger.Error(ctx, "[LLM RESPONSE DEBUG] LLM call failed", map[string]interface{}{
				"iteration": iteration + 1,
				"error":     err.Error(),
			})
			return err
		}
		c.logger.Info(ctx, "[LLM RESPONSE DEBUG] LLM response received", map[string]interface{}{
			"iteration":      iteration + 1,
			"toolCallsCount": len(toolCalls),
			"hasContent":     hasContent,
			"gotToolCalls":   len(toolCalls) > 0,
		})

		// If no tool calls, check if we have content
		if len(toolCalls) == 0 {
			// If we have content, we're done with iterations - the model provided a final response
			if hasContent {
				c.logger.Info(ctx, "[LLM RESPONSE DEBUG] Got final content response without tool calls", map[string]interface{}{
					"iteration":      iteration + 1,
					"hasContent":     hasContent,
					"responseType":   "final_answer",
					"toolCallsCount": 0,
					"capturedEvents": len(capturedContentEvents),
				})

				// Only replay if content was filtered (not already forwarded)
				if filterContentDeltas {
					// Replay the captured content events to stream the final response
					c.logger.Debug(ctx, "[LLM RESPONSE DEBUG] Replaying captured content events", map[string]interface{}{
						"iteration":   iteration + 1,
						"eventsCount": len(capturedContentEvents),
					})

					for _, contentEvent := range capturedContentEvents {
						select {
						case eventChan <- contentEvent:
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				}

				// Send completion event
				select {
				case eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventContentComplete,
					Timestamp: time.Now(),
					Metadata: map[string]interface{}{
						"iteration": iteration + 1,
					},
				}:
				case <-ctx.Done():
					return ctx.Err()
				}
				// Mark that we got a complete response
				gotCompleteResponse = true
				// Break out of iteration loop (don't return - let final synthesis check happen)
				break
			}
			// If no tool calls and no content, log warning and continue to next iteration
			// This might happen if the model returns an empty response
			c.logger.Warn(ctx, "[LLM RESPONSE DEBUG] No tool calls and no content in iteration", map[string]interface{}{
				"iteration":     iteration + 1,
				"maxIterations": maxIterations,
				"responseType":  "empty_response",
			})
			// Continue to next iteration or break if this was the last one
			if iteration >= maxIterations-1 {
				// We've reached max iterations, break out to make final call
				break
			}
			continue
		}

		// Execute tools and add results to conversation
		c.logger.Info(ctx, "[LLM RESPONSE DEBUG] Processing tool calls from LLM response", map[string]interface{}{
			"count":        len(toolCalls),
			"iteration":    iteration + 1,
			"responseType": "tool_calls",
		})

		// Add assistant message to conversation history (matching non-streaming behavior)
		// Build assistant content from captured events
		var assistantContent strings.Builder
		for _, event := range capturedContentEvents {
			if event.Type == interfaces.StreamEventContentDelta {
				assistantContent.WriteString(event.Content)
			}
		}

		// Add assistant message only if there's text content (matching client.go:1196-1200)
		if strings.TrimSpace(assistantContent.String()) != "" {
			messages = append(messages, Message{
				Role:    "assistant",
				Content: assistantContent.String(),
			})
		}

		// Send a line break before tool execution for clarity
		select {
		case eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventContentDelta,
			Content:   "\n", // Single line break before tools
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"before_tools": true,
				"iteration":    iteration + 1,
			},
		}:
		case <-ctx.Done():
			return ctx.Err()
		}

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
			c.logger.Info(ctx, "[TOOL EXECUTION DEBUG] Executing tool in streaming", map[string]interface{}{
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

		// Send a line break between iterations for better readability
		if iteration < maxIterations-1 { // Don't add break after last iteration
			select {
			case eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventContentDelta,
				Content:   "\n\n", // Add double line break for visual separation
				Timestamp: time.Now(),
				Metadata: map[string]interface{}{
					"iteration_boundary": true,
					"iteration":          iteration + 1,
				},
			}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Continue to next iteration with updated conversation
	}

	if gotCompleteResponse {
		c.logger.Debug(ctx, "[LLM RESPONSE DEBUG] Skipping final synthesis call - already got complete response", map[string]interface{}{
			"maxIterations":        maxIterations,
			"totalLLMCalls":        finalIterationCount,
			"skippedSynthesisCall": true,
		})
		return nil
	}

	// After all tool iterations, make a final call without tools to get the synthesized answer
	// This ensures the LLM provides a final response after processing all tool results

	c.logger.Info(ctx, "[LLM RESPONSE DEBUG] Making final synthesis call after tool iterations", map[string]interface{}{
		"maxIterations":         maxIterations,
		"totalPreviousLLMCalls": finalIterationCount,
		"reason":                "no_complete_response_received",
	})

	// Build the final user message
	finalUserMessage := "Please provide your final response based on the information available. Do not request any additional tools."

	// If structured output is requested, enhance the final message with schema and examples
	if params.ResponseFormat != nil {
		// Convert the schema to a string representation for the prompt
		schemaJSON, err := json.MarshalIndent(params.ResponseFormat.Schema, "", "  ")
		if err == nil {
			// Create an example JSON structure based on the schema
			exampleJSON := createExampleFromSchema(params.ResponseFormat.Schema)
			exampleStr, _ := json.MarshalIndent(exampleJSON, "", "  ")

			// Enhance the final user message with schema information and example
			finalUserMessage = fmt.Sprintf(`%s

You must respond with a valid JSON object that exactly follows this schema:
%s

Here is an example of the expected JSON structure:
%s

CRITICAL INSTRUCTIONS:
- Output ONLY valid JSON, no additional text before or after
- Follow the EXACT structure shown in the schema and example
- Use the field names exactly as specified
- Ensure all required fields are present
- Pay special attention to array fields - they must be arrays of objects, not simple objects
- If a field is defined as an array in the schema, it MUST be an array in your response
- The JSON must be directly parsable and match the schema precisely`, finalUserMessage, string(schemaJSON), string(exampleStr))
		}
	}

	// Add a message to inform the LLM this is the final call
	finalMessages := append(messages, Message{
		Role:    "user",
		Content: finalUserMessage,
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

	// Add system message if available and enhance for structured output
	if params.SystemMessage != "" {
		if params.ResponseFormat != nil {
			finalReq.System = params.SystemMessage + "\n\nIMPORTANT: You must respond with valid JSON that matches the specified schema. Return ONLY the raw JSON object without any markdown formatting, code blocks, or wrapper text. Pay special attention to array fields - if a field is defined as an array in the schema, it MUST be an array in your response, not an object."
		} else {
			finalReq.System = params.SystemMessage
		}
	} else if params.ResponseFormat != nil {
		// If no system message but structured output is requested, add a system message for JSON
		finalReq.System = "You must respond with valid JSON that matches the specified schema. Return ONLY the raw JSON object without any markdown formatting, code blocks, or wrapper text. Pay special attention to array fields - if a field is defined as an array in the schema, it MUST be an array in your response, not an object."
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

	// Execute final request to get synthesized answer with memory support
	c.logger.Debug(ctx, "[LLM RESPONSE DEBUG] Executing final synthesis LLM call", map[string]interface{}{
		"finalCallNumber": finalIterationCount + 1,
		"messageCount":    len(finalMessages),
	})
	err := c.executeStreamingRequestWithMemory(ctx, finalReq, eventChan, "", params)
	if err != nil {
		c.logger.Error(ctx, "[LLM RESPONSE DEBUG] Final synthesis call failed", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		c.logger.Info(ctx, "[LLM RESPONSE DEBUG] Final synthesis call completed successfully", map[string]interface{}{
			"totalLLMCalls": finalIterationCount + 1,
		})
	}
	return err
}

// createFilteredEventForwarder processes events and optionally captures content for later replay
func (c *AnthropicClient) createFilteredEventForwarder(
	ctx context.Context,
	tempEventChan <-chan interfaces.StreamEvent,
	eventChan chan<- interfaces.StreamEvent,
	filterContentDeltas bool,
) ([]interfaces.ToolCall, bool, []interfaces.StreamEvent, error) {
	var toolCalls []interfaces.ToolCall
	var hasContent bool
	var capturedContentEvents []interfaces.StreamEvent

	for event := range tempEventChan {
		// Always capture content events for conversation history (not just when filtering)
		if event.Type == interfaces.StreamEventContentDelta && event.Content != "" {
			hasContent = true
			capturedContentEvents = append(capturedContentEvents, event)

			// If filtering is enabled, don't forward the content yet
			if filterContentDeltas {
				continue // Skip forwarding
			}
		}

		// Forward event to main channel (unless it was filtered above)
		select {
		case eventChan <- event:
		case <-ctx.Done():
			return nil, false, nil, ctx.Err()
		}

		// Capture tool calls
		if event.Type == interfaces.StreamEventToolUse && event.ToolCall != nil {
			toolCalls = append(toolCalls, *event.ToolCall)
		}

		// Check for errors
		if event.Error != nil {
			return nil, false, nil, event.Error
		}
	}

	return toolCalls, hasContent, capturedContentEvents, nil
}

// executeStreamingRequestWithToolCapture executes a streaming request and captures tool calls
func (c *AnthropicClient) executeStreamingRequestWithToolCapture(
	ctx context.Context,
	req CompletionRequest,
	eventChan chan<- interfaces.StreamEvent,
	filterContentDeltas bool,
) ([]interfaces.ToolCall, bool, []interfaces.StreamEvent, error) {

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

	// Process events with optional filtering
	return c.createFilteredEventForwarder(ctx, tempEventChan, eventChan, filterContentDeltas)
}

// executeBedrockStreaming handles streaming for AWS Bedrock using the AWS SDK
func (c *AnthropicClient) executeBedrockStreaming(
	ctx context.Context,
	req *CompletionRequest,
	eventChan chan<- interfaces.StreamEvent,
) error {
	modelID := c.BedrockConfig.GetModelID(c.Model)

	c.logger.Debug(ctx, "Executing Bedrock streaming request", map[string]interface{}{
		"modelID": modelID,
		"region":  c.BedrockConfig.Region,
	})

	// Invoke Bedrock streaming
	output, err := c.BedrockConfig.InvokeModelStream(ctx, modelID, req)
	if err != nil {
		return fmt.Errorf("failed to invoke Bedrock streaming: %w", err)
	}

	// Get the event stream
	stream := output.GetStream()
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			c.logger.Warn(ctx, "Failed to close Bedrock stream", map[string]interface{}{
				"error": closeErr.Error(),
			})
		}
	}()

	// Track thinking blocks and tool blocks for proper event handling (reusing SSE logic)
	thinkingBlocks := make(map[int]bool)
	toolBlocks := make(map[int]struct {
		ID        string
		Name      string
		InputJSON strings.Builder
	})

	// Process streaming events
	for event := range stream.Events() {
		// Check context cancellation early
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		switch e := event.(type) {
		case *types.ResponseStreamMemberChunk:
			// Parse the chunk data - Bedrock returns flat JSON event structure
			// Example: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}
			var rawEvent map[string]interface{}
			if err := json.Unmarshal(e.Value.Bytes, &rawEvent); err != nil {
				c.logger.Error(ctx, "Failed to parse Bedrock streaming chunk", map[string]interface{}{
					"error": err.Error(),
				})
				continue
			}

			// Extract event type
			eventType, ok := rawEvent["type"].(string)
			if !ok {
				c.logger.Error(ctx, "Bedrock event missing type field", map[string]interface{}{
					"raw_event": string(e.Value.Bytes),
				})
				continue
			}

			// Convert flat Bedrock format to SSE format expected by convertAnthropicEventToStreamEvent
			// SSE format wraps the event data in a "data" field
			anthropicEvent := &AnthropicSSEEvent{
				Type: eventType,
				Data: e.Value.Bytes, // The raw event bytes contain all the data fields
			}

			// Reuse existing Anthropic event converter from sse.go
			// This handles thinking blocks, tool use, and all event types properly
			streamEvent, err := c.convertAnthropicEventToStreamEvent(anthropicEvent, thinkingBlocks, toolBlocks)
			if err != nil {
				c.logger.Error(ctx, "Failed to convert Bedrock event", map[string]interface{}{
					"error":      err.Error(),
					"event_type": eventType,
				})
				continue
			}

			if streamEvent != nil {
				select {
				case eventChan <- *streamEvent:
				case <-ctx.Done():
					return ctx.Err()
				}
			}

		default:
			// Unknown event type
			c.logger.Debug(ctx, "Unknown Bedrock streaming event type", map[string]interface{}{
				"type": fmt.Sprintf("%T", e),
			})
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		c.logger.Error(ctx, "Bedrock streaming error", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("bedrock streaming error: %w", err)
	}

	c.logger.Debug(ctx, "Successfully completed Bedrock streaming request", map[string]interface{}{
		"modelID": modelID,
	})

	return nil
}
