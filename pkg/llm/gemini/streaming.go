package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// GenerateStream generates text with streaming response using native Gemini streaming
func (c *GeminiClient) GenerateStream(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	// Convert options to params
	params := &interfaces.GenerateOptions{}
	for _, opt := range options {
		if opt != nil {
			opt(params)
		}
	}

	// Get streaming config or use default
	streamConfig := interfaces.DefaultStreamConfig()
	if params.StreamConfig != nil {
		streamConfig = *params.StreamConfig
	}

	// Check for organization ID in context
	orgID := "default"
	if id, err := multitenancy.GetOrgID(ctx); err == nil {
		orgID = id
	}
	_ = orgID

	// Build contents using unified builder
	builder := newMessageHistoryBuilder(c.logger)
	contents := builder.buildContents(ctx, prompt, params)

	// Add system instruction if provided or if reasoning is specified
	var systemInstruction *genai.Content
	systemMessage := params.SystemMessage

	// Log reasoning mode usage - only affects native thinking models (2.5 series)
	if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
		if SupportsThinking(c.model) {
			c.logger.Debug(ctx, "Using reasoning mode with thinking-capable model", map[string]interface{}{
				"reasoning": params.LLMConfig.Reasoning,
				"model":     c.model,
			})
		} else {
			c.logger.Debug(ctx, "Reasoning mode specified for non-thinking model - native thinking tokens not available", map[string]interface{}{
				"reasoning":        params.LLMConfig.Reasoning,
				"model":            c.model,
				"supportsThinking": false,
			})
		}
	}

	if systemMessage != "" {
		systemInstruction = &genai.Content{
			Parts: []*genai.Part{
				{Text: systemMessage},
			},
		}
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": systemMessage})
	}

	// Set generation config
	var genConfig *genai.GenerationConfig
	if params.LLMConfig != nil {
		genConfig = &genai.GenerationConfig{}

		if params.LLMConfig.Temperature > 0 {
			temp := float32(params.LLMConfig.Temperature)
			genConfig.Temperature = &temp
		}
		if params.LLMConfig.TopP > 0 {
			topP := float32(params.LLMConfig.TopP)
			genConfig.TopP = &topP
		}
		if len(params.LLMConfig.StopSequences) > 0 {
			genConfig.StopSequences = params.LLMConfig.StopSequences
		}
	}

	// Set response format if provided
	if params.ResponseFormat != nil {
		if genConfig == nil {
			genConfig = &genai.GenerationConfig{}
		}
		genConfig.ResponseMIMEType = "application/json"

		// Convert schema for genai
		if schemaBytes, err := json.Marshal(params.ResponseFormat.Schema); err == nil {
			var schema *genai.Schema
			if err := json.Unmarshal(schemaBytes, &schema); err != nil {
				c.logger.Warn(ctx, "Failed to convert response schema", map[string]interface{}{"error": err.Error()})
			} else {
				genConfig.ResponseSchema = schema
			}
		}
		c.logger.Debug(ctx, "Using response format", map[string]interface{}{"format": *params.ResponseFormat})
	}

	// Create config
	config := &genai.GenerateContentConfig{
		SystemInstruction: systemInstruction,
	}

	// Apply generation config parameters directly to config
	if genConfig != nil {
		if genConfig.Temperature != nil {
			config.Temperature = genConfig.Temperature
		}
		if genConfig.TopP != nil {
			config.TopP = genConfig.TopP
		}
		if len(genConfig.StopSequences) > 0 {
			config.StopSequences = genConfig.StopSequences
		}
		if genConfig.ResponseMIMEType != "" {
			config.ResponseMIMEType = genConfig.ResponseMIMEType
		}
		if genConfig.ResponseSchema != nil {
			config.ResponseSchema = genConfig.ResponseSchema
		}
	}

	// Add thinking configuration if supported and enabled
	if SupportsThinking(c.model) && c.thinkingConfig != nil {
		if c.thinkingConfig.IncludeThoughts || c.thinkingConfig.ThinkingBudget != nil {
			config.ThinkingConfig = &genai.ThinkingConfig{
				IncludeThoughts: c.thinkingConfig.IncludeThoughts,
				ThinkingBudget:  c.thinkingConfig.ThinkingBudget,
			}

			c.logger.Debug(ctx, "Enabled thinking configuration for streaming", map[string]interface{}{
				"includeThoughts": c.thinkingConfig.IncludeThoughts,
				"thinkingBudget":  c.thinkingConfig.ThinkingBudget,
			})
		}
	}

	// Create event channel
	eventCh := make(chan interfaces.StreamEvent, streamConfig.BufferSize)

	// Start streaming goroutine
	go func() {
		defer close(eventCh)

		// Send message start event
		select {
		case eventCh <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStart,
			Timestamp: time.Now(),
		}:
		case <-ctx.Done():
			return
		}

		c.logger.Debug(ctx, "Starting native Gemini streaming", map[string]interface{}{
			"model":           c.model,
			"thinkingEnabled": SupportsThinking(c.model) && c.thinkingConfig != nil && c.thinkingConfig.IncludeThoughts,
		})

		// Track accumulated content for memory storage
		var accumulatedContent strings.Builder

		// Start streaming
		streamIter := c.genaiClient.Models.GenerateContentStream(ctx, c.model, contents, config)

		for response, err := range streamIter {
			if err != nil {
				// Send error event
				select {
				case eventCh <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventError,
					Error:     err,
					Timestamp: time.Now(),
				}:
				case <-ctx.Done():
				}
				return
			}

			// Process each candidate in the response
			for _, candidate := range response.Candidates {
				if candidate.Content == nil {
					continue
				}

				// Process each part in the content
				for _, part := range candidate.Content.Parts {
					if part.Text == "" {
						continue
					}

					// Check if this is thinking content
					if part.Thought {
						// Send thinking event
						select {
						case eventCh <- interfaces.StreamEvent{
							Type:      interfaces.StreamEventThinking,
							Content:   part.Text,
							Timestamp: time.Now(),
							Metadata: map[string]interface{}{
								"thought_signature": part.ThoughtSignature,
							},
						}:
						case <-ctx.Done():
							return
						}
					} else {
						// Send content delta event and accumulate for memory
						accumulatedContent.WriteString(part.Text)
						select {
						case eventCh <- interfaces.StreamEvent{
							Type:      interfaces.StreamEventContentDelta,
							Content:   part.Text,
							Timestamp: time.Now(),
						}:
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}

		// Send content complete event
		select {
		case eventCh <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventContentComplete,
			Timestamp: time.Now(),
		}:
		case <-ctx.Done():
			return
		}

		// Send message stop event
		select {
		case eventCh <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStop,
			Timestamp: time.Now(),
		}:
		case <-ctx.Done():
			return
		}

		c.logger.Debug(ctx, "Successfully completed native Gemini streaming response", map[string]interface{}{
			"model": c.model,
		})
	}()

	return eventCh, nil
}

// GenerateWithToolsStream generates text with tools and streaming response with real-time tool events
func (c *GeminiClient) GenerateWithToolsStream(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	// Convert options to params
	params := &interfaces.GenerateOptions{}
	for _, opt := range options {
		if opt != nil {
			opt(params)
		}
	}

	// Set default values only if they're not provided
	if params.LLMConfig == nil {
		params.LLMConfig = &interfaces.LLMConfig{
			Temperature:      0.7,
			TopP:             1.0,
			FrequencyPenalty: 0.0,
			PresencePenalty:  0.0,
		}
	}

	// Set default max iterations if not provided
	maxIterations := params.MaxIterations
	if maxIterations == 0 {
		maxIterations = 2 // Default to current behavior
	}

	// Get streaming config or use default
	streamConfig := interfaces.DefaultStreamConfig()
	if params.StreamConfig != nil {
		streamConfig = *params.StreamConfig
	}

	// Create event channel
	eventCh := make(chan interfaces.StreamEvent, streamConfig.BufferSize)

	go func() {
		defer close(eventCh)

		// Send message start event
		select {
		case eventCh <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStart,
			Timestamp: time.Now(),
		}:
		case <-ctx.Done():
			return
		}

		c.logger.Debug(ctx, "Starting streaming with tools with real-time events", map[string]interface{}{
			"model":         c.model,
			"tools":         len(tools),
			"maxIterations": maxIterations,
		})

		// Execute the tool calling process with streaming events
		response, err := c.generateWithToolsAndStream(ctx, prompt, tools, params, maxIterations, eventCh)
		if err != nil {
			// Send error event
			select {
			case eventCh <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventError,
				Error:     err,
				Timestamp: time.Now(),
			}:
			case <-ctx.Done():
			}
			return
		}

		// Stream the final response in chunks
		c.streamResponse(ctx, response, eventCh)

		// Send content complete event
		select {
		case eventCh <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventContentComplete,
			Timestamp: time.Now(),
		}:
		case <-ctx.Done():
			return
		}

		// Send message stop event
		select {
		case eventCh <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStop,
			Timestamp: time.Now(),
		}:
		case <-ctx.Done():
			return
		}

		c.logger.Info(ctx, "Successfully completed streaming response with tools", map[string]interface{}{
			"maxIterations": maxIterations,
		})
	}()

	return eventCh, nil
}

// generateWithToolsAndStream executes tool calling with real-time streaming events
func (c *GeminiClient) generateWithToolsAndStream(ctx context.Context, prompt string, tools []interfaces.Tool, params *interfaces.GenerateOptions, maxIterations int, eventCh chan interfaces.StreamEvent) (string, error) {
	// Determine if we should filter intermediate content (for backward compatibility)
	filterIntermediateContent := params.StreamConfig == nil || !params.StreamConfig.IncludeIntermediateMessages

	// Track captured content for final iteration replay if filtering is enabled
	var capturedContentEvents []interfaces.StreamEvent
	// Build tool map for quick lookup
	toolMap := make(map[string]interfaces.Tool)
	for _, tool := range tools {
		toolMap[tool.Name()] = tool
	}

	// Track tool calls for clean loop continuation

	// Build contents using unified builder
	builder := newMessageHistoryBuilder(c.logger)
	contents := builder.buildContents(ctx, prompt, params)

	// Add system instruction if provided
	var systemInstruction *genai.Content
	if params.SystemMessage != "" {
		systemInstruction = &genai.Content{
			Parts: []*genai.Part{
				{Text: params.SystemMessage},
			},
		}
		c.logger.Debug(ctx, "Using system message for tool streaming", map[string]interface{}{"system_message": params.SystemMessage})
	}

	// Convert tools to Gemini format - all function declarations in a single tool
	var functionDeclarations []*genai.FunctionDeclaration
	for _, tool := range tools {
		functionDeclaration := &genai.FunctionDeclaration{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: make(map[string]*genai.Schema),
				Required:   make([]string, 0),
			},
		}

		// Convert parameters
		for name, param := range tool.Parameters() {
			paramSchema := &genai.Schema{
				Description: param.Description,
			}

			// Set type
			switch param.Type {
			case "string":
				paramSchema.Type = genai.TypeString
			case "number", "integer":
				paramSchema.Type = genai.TypeNumber
			case "boolean":
				paramSchema.Type = genai.TypeBoolean
			case "array":
				paramSchema.Type = genai.TypeArray
			case "object":
				paramSchema.Type = genai.TypeObject
			}

			if param.Enum != nil {
				enumStrings := make([]string, len(param.Enum))
				for i, e := range param.Enum {
					enumStrings[i] = fmt.Sprintf("%v", e)
				}
				paramSchema.Enum = enumStrings
			}

			functionDeclaration.Parameters.Properties[name] = paramSchema
			if param.Required {
				functionDeclaration.Parameters.Required = append(functionDeclaration.Parameters.Required, name)
			}
		}

		functionDeclarations = append(functionDeclarations, functionDeclaration)
	}

	// Create a single tool with all function declarations
	geminiTools := []*genai.Tool{
		{
			FunctionDeclarations: functionDeclarations,
		},
	}

	// Main conversation loop with streaming events
	for iteration := 0; iteration < maxIterations; iteration++ {
		// Set generation config
		var genConfig *genai.GenerationConfig
		if params.LLMConfig != nil {
			genConfig = &genai.GenerationConfig{}

			if params.LLMConfig.Temperature > 0 {
				temp := float32(params.LLMConfig.Temperature)
				genConfig.Temperature = &temp
			}
			if params.LLMConfig.TopP > 0 {
				topP := float32(params.LLMConfig.TopP)
				genConfig.TopP = &topP
			}
			if len(params.LLMConfig.StopSequences) > 0 {
				genConfig.StopSequences = params.LLMConfig.StopSequences
			}
		}

		// Create config
		config := &genai.GenerateContentConfig{
			SystemInstruction: systemInstruction,
			Tools:             geminiTools,
		}

		// Apply generation config parameters
		if genConfig != nil {
			if genConfig.Temperature != nil {
				config.Temperature = genConfig.Temperature
			}
			if genConfig.TopP != nil {
				config.TopP = genConfig.TopP
			}
			if len(genConfig.StopSequences) > 0 {
				config.StopSequences = genConfig.StopSequences
			}
		}

		c.logger.Debug(ctx, "Sending request with tools for streaming", map[string]interface{}{
			"contents":      len(contents),
			"iteration":     iteration + 1,
			"maxIterations": maxIterations,
			"model":         c.model,
			"tools":         len(tools),
		})

		// Execute streaming request and collect tool calls
		shouldFilter := filterIntermediateContent && len(tools) > 0 && iteration < maxIterations-1
		var iterationContentEvents []interfaces.StreamEvent
		toolCalls, hasContent, err := c.executeStreamingRequestWithToolCapture(ctx, contents, config, eventCh, shouldFilter, &iterationContentEvents)
		if err != nil {
			return "", err
		}

		// If we had content during this iteration and tools were called, capture it for final replay
		if shouldFilter && hasContent && len(toolCalls) > 0 {
			capturedContentEvents = append(capturedContentEvents, iterationContentEvents...)
		}

		// If no tool calls, we're done with the iteration loop
		if len(toolCalls) == 0 {
			// No tool calls means we have received the final response content
			// The streaming has already been handled by executeStreamingRequestWithToolCapture
			return "", nil
		}

		// Execute tools and add results to conversation
		c.logger.Info(ctx, "Processing tool calls in streaming", map[string]interface{}{
			"count":     len(toolCalls),
			"iteration": iteration + 1,
		})

		// Add assistant message with tool calls
		assistantMessage := &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{},
		}

		// Convert tool calls to Gemini format and add to message
		for _, toolCall := range toolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(toolCall.Arguments), &args); err != nil {
				args = make(map[string]interface{})
			}
			assistantMessage.Parts = append(assistantMessage.Parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: toolCall.Name,
					Args: args,
				},
			})
		}

		contents = append(contents, assistantMessage)

		// Collect all tool results to add them in a single content message
		var functionResponses []*genai.Part

		// Execute each tool and collect results
		for _, toolCall := range toolCalls {
			// Find the requested tool
			var selectedTool interfaces.Tool
			for _, tool := range tools {
				if tool.Name() == toolCall.Name {
					selectedTool = tool
					break
				}
			}

			if selectedTool == nil {
				c.logger.Error(ctx, "Tool not found in streaming", map[string]interface{}{
					"toolName": toolCall.Name,
				})

				// Add tool not found error as function response
				errorMessage := fmt.Sprintf("Error: tool not found: %s", toolCall.Name)
				functionResponses = append(functionResponses, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						Name: toolCall.Name,
						Response: map[string]any{
							"error": errorMessage,
						},
					},
				})

				// Send tool result event with error
				select {
				case eventCh <- interfaces.StreamEvent{
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
					return "", ctx.Err()
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

			// Add tool result as function response
			functionResponses = append(functionResponses, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: toolCall.Name,
					Response: map[string]any{
						"result": toolResult,
					},
				},
			})

			// Send tool result event
			select {
			case eventCh <- interfaces.StreamEvent{
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
				return "", ctx.Err()
			}
		}

		// Add all function responses in a single content message
		if len(functionResponses) > 0 {
			contents = append(contents, &genai.Content{
				Role:  "user",
				Parts: functionResponses,
			})
		}

		// Continue to next iteration with updated conversation
	}

	// Replay captured content events if we filtered them during iterations
	if filterIntermediateContent && len(capturedContentEvents) > 0 {
		c.logger.Debug(ctx, "Replaying captured content events from tool iterations", map[string]interface{}{
			"eventsCount": len(capturedContentEvents),
		})
		for _, contentEvent := range capturedContentEvents {
			select {
			case eventCh <- contentEvent:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}

	// After all tool iterations, make a final call without tools to get the synthesized answer
	// This ensures the LLM provides a final response after processing all tool results
	c.logger.Info(ctx, "Maximum iterations reached, making final call without tools", map[string]interface{}{
		"maxIterations": maxIterations,
	})

	// Add a message to inform the LLM this is the final call
	contents = append(contents, &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: "Please provide your final response based on the information available. Do not request any additional functions."},
		},
	})

	// Set generation config without tools
	var genConfig *genai.GenerationConfig
	if params.LLMConfig != nil {
		genConfig = &genai.GenerationConfig{}

		if params.LLMConfig.Temperature > 0 {
			temp := float32(params.LLMConfig.Temperature)
			genConfig.Temperature = &temp
		}
		if params.LLMConfig.TopP > 0 {
			topP := float32(params.LLMConfig.TopP)
			genConfig.TopP = &topP
		}
		if len(params.LLMConfig.StopSequences) > 0 {
			genConfig.StopSequences = params.LLMConfig.StopSequences
		}
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: systemInstruction,
		// No tools in final request - we want a final answer
	}

	// Apply generation config parameters
	if genConfig != nil {
		if genConfig.Temperature != nil {
			config.Temperature = genConfig.Temperature
		}
		if genConfig.TopP != nil {
			config.TopP = genConfig.TopP
		}
		if len(genConfig.StopSequences) > 0 {
			config.StopSequences = genConfig.StopSequences
		}
	}

	// Execute final request to get synthesized answer using streaming (no filtering for final call)
	_, _, err := c.executeStreamingRequestWithToolCapture(ctx, contents, config, eventCh, false, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create final content: %w", err)
	}

	return "", nil
}

// streamResponse streams a response string in chunks
func (c *GeminiClient) streamResponse(ctx context.Context, response string, eventCh chan interfaces.StreamEvent) {
	chunkSize := 50 // characters per chunk
	for i := 0; i < len(response); i += chunkSize {
		end := i + chunkSize
		if end > len(response) {
			end = len(response)
		}

		chunk := response[i:end]

		// Send content delta event
		select {
		case eventCh <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventContentDelta,
			Content:   chunk,
			Timestamp: time.Now(),
		}:
		case <-ctx.Done():
			return
		}

		// No artificial delay - stream at real speed
	}
}

// executeStreamingRequestWithToolCapture executes a streaming request and captures tool calls
func (c *GeminiClient) executeStreamingRequestWithToolCapture(
	ctx context.Context,
	contents []*genai.Content,
	config *genai.GenerateContentConfig,
	eventCh chan<- interfaces.StreamEvent,
	filterContent bool,
	capturedEvents *[]interfaces.StreamEvent,
) ([]interfaces.ToolCall, bool, error) {

	var toolCalls []interfaces.ToolCall
	var hasContent bool

	c.logger.Debug(ctx, "Executing Gemini streaming request with tool capture", map[string]interface{}{
		"model":         c.model,
		"filterContent": filterContent,
	})

	// Add thinking configuration if supported and enabled
	if SupportsThinking(c.model) && c.thinkingConfig != nil {
		if c.thinkingConfig.IncludeThoughts || c.thinkingConfig.ThinkingBudget != nil {
			config.ThinkingConfig = &genai.ThinkingConfig{
				IncludeThoughts: c.thinkingConfig.IncludeThoughts,
				ThinkingBudget:  c.thinkingConfig.ThinkingBudget,
			}

			c.logger.Debug(ctx, "Enabled thinking configuration for tool streaming", map[string]interface{}{
				"includeThoughts": c.thinkingConfig.IncludeThoughts,
				"thinkingBudget":  c.thinkingConfig.ThinkingBudget,
			})
		}
	}

	// Generate content with tools using streaming
	streamIter := c.genaiClient.Models.GenerateContentStream(ctx, c.model, contents, config)

	for response, err := range streamIter {
		if err != nil {
			return nil, false, fmt.Errorf("failed to generate content stream: %w", err)
		}

		// Process each candidate in the response
		for _, candidate := range response.Candidates {
			if candidate.Content == nil {
				continue
			}

			// Process each part in the content
			for _, part := range candidate.Content.Parts {
				if part.FunctionCall != nil {
					// This is a tool call - capture it
					argsBytes, _ := json.Marshal(part.FunctionCall.Args)
					toolCall := interfaces.ToolCall{
						ID:        fmt.Sprintf("gemini_tool_%s", part.FunctionCall.Name),
						Name:      part.FunctionCall.Name,
						Arguments: string(argsBytes),
					}
					toolCalls = append(toolCalls, toolCall)

					// Send tool use event to stream
					select {
					case eventCh <- interfaces.StreamEvent{
						Type:      interfaces.StreamEventToolUse,
						Timestamp: time.Now(),
						ToolCall:  &toolCall,
					}:
					case <-ctx.Done():
						return nil, false, ctx.Err()
					}
				} else if part.Text != "" {
					// Check if this is thinking content
					if part.Thought {
						// Send thinking event
						select {
						case eventCh <- interfaces.StreamEvent{
							Type:      interfaces.StreamEventThinking,
							Content:   part.Text,
							Timestamp: time.Now(),
							Metadata: map[string]interface{}{
								"thought_signature": part.ThoughtSignature,
							},
						}:
						case <-ctx.Done():
							return nil, false, ctx.Err()
						}
					} else {
						// This is content
						hasContent = true
						contentEvent := interfaces.StreamEvent{
							Type:      interfaces.StreamEventContentDelta,
							Content:   part.Text,
							Timestamp: time.Now(),
						}

						if filterContent && capturedEvents != nil {
							// Capture content for potential replay later
							*capturedEvents = append(*capturedEvents, contentEvent)
						} else {
							// Stream content immediately
							select {
							case eventCh <- contentEvent:
							case <-ctx.Done():
								return nil, false, ctx.Err()
							}
						}
					}
				}
			}
		}
	}

	return toolCalls, hasContent, nil
}
