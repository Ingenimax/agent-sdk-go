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

	// Prepare content similar to non-streaming Generate method
	parts := []*genai.Part{
		{Text: prompt},
	}

	contents := []*genai.Content{
		{
			Role:  "user",
			Parts: parts,
		},
	}

	// Add system instruction if provided or if reasoning is specified
	var systemInstruction *genai.Content
	systemMessage := params.SystemMessage
	
	// Log reasoning mode usage - only affects native thinking models (2.5 series)
	if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
		if SupportsThinking(c.model) {
			c.logger.Debug(ctx, "Using reasoning mode with thinking-capable model", map[string]interface{}{
				"reasoning": params.LLMConfig.Reasoning,
				"model": c.model,
			})
		} else {
			c.logger.Debug(ctx, "Reasoning mode specified for non-thinking model - native thinking tokens not available", map[string]interface{}{
				"reasoning": params.LLMConfig.Reasoning, 
				"model": c.model,
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
			"model": c.model,
			"thinkingEnabled": SupportsThinking(c.model) && c.thinkingConfig != nil && c.thinkingConfig.IncludeThoughts,
		})

		// Start streaming
		streamIter := c.client.Models.GenerateContentStream(ctx, c.model, contents, config)
		
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
						// Send content delta event
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
	// Build tool map for quick lookup
	toolMap := make(map[string]interfaces.Tool)
	for _, tool := range tools {
		toolMap[tool.Name()] = tool
	}

	// Prepare initial content
	parts := []*genai.Part{
		{Text: prompt},
	}

	contents := []*genai.Content{
		{
			Role:  "user",
			Parts: parts,
		},
	}

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

	// Convert tools to Gemini format
	geminiTools := make([]*genai.Tool, 0, len(tools))
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

		geminiTools = append(geminiTools, &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{functionDeclaration},
		})
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

		// Generate content with tools
		result, err := c.client.Models.GenerateContent(ctx, c.model, contents, config)
		if err != nil {
			return "", fmt.Errorf("failed to generate content with tools (iteration %d): %w", iteration+1, err)
		}

		if len(result.Candidates) == 0 {
			return "", fmt.Errorf("no candidates returned (iteration %d)", iteration+1)
		}

		candidate := result.Candidates[0]
		if candidate.Content == nil {
			return "", fmt.Errorf("no content in candidate (iteration %d)", iteration+1)
		}

		// Check if there are function calls to process
		hasFunctionCalls := false
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				hasFunctionCalls = true
				break
			}
		}

		if !hasFunctionCalls {
			// No more function calls, return the final response
			var finalResponse strings.Builder
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					finalResponse.WriteString(part.Text)
				}
			}
			return finalResponse.String(), nil
		}

		// Process function calls and emit streaming events
		c.logger.Info(ctx, "Processing function calls for streaming", map[string]interface{}{
			"iteration": iteration + 1,
		})

		// Add the assistant's message with function calls to the conversation
		assistantContent := &genai.Content{
			Role:  "model",
			Parts: candidate.Content.Parts,
		}
		contents = append(contents, assistantContent)

		// Process each function call
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall == nil {
				continue
			}

			functionCall := part.FunctionCall

			// Send tool use event
			argsBytes, _ := json.Marshal(functionCall.Args)
			select {
			case eventCh <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventToolUse,
				Timestamp: time.Now(),
				ToolCall: &interfaces.ToolCall{
					ID:        fmt.Sprintf("tool_%d_%s", iteration, functionCall.Name),
					Name:      functionCall.Name,
					Arguments: string(argsBytes),
				},
			}:
			case <-ctx.Done():
				return "", ctx.Err()
			}

			// Find and execute the tool
			selectedTool, exists := toolMap[functionCall.Name]
			if !exists {
				errorMsg := fmt.Sprintf("Tool '%s' not found", functionCall.Name)
				
				// Send tool result error event
				select {
				case eventCh <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventToolResult,
					Content:   errorMsg,
					Timestamp: time.Now(),
					ToolCall: &interfaces.ToolCall{
						ID:        fmt.Sprintf("tool_%d_%s", iteration, functionCall.Name),
						Name:      functionCall.Name,
						Arguments: string(argsBytes),
					},
				}:
				case <-ctx.Done():
					return "", ctx.Err()
				}

				// Add error message as function response
				contents = append(contents, &genai.Content{
					Role: "user",
					Parts: []*genai.Part{
						{
							FunctionResponse: &genai.FunctionResponse{
								Name: functionCall.Name,
								Response: map[string]any{
									"error": errorMsg,
								},
							},
						},
					},
				})
				continue
			}

			c.logger.Info(ctx, "Executing tool for streaming", map[string]interface{}{
				"toolName": selectedTool.Name(),
			})

			// Convert function args to JSON string for tool execution (reuse already marshaled args)
			toolStartTime := time.Now()
			toolResult, err := selectedTool.Execute(ctx, string(argsBytes))
			toolEndTime := time.Now()

			// Send tool result event
			toolResultContent := toolResult
			if err != nil {
				// Log tool failure with details
				c.logger.Error(ctx, "Tool execution failed during streaming", map[string]interface{}{
					"toolName": selectedTool.Name(),
					"toolArgs": string(argsBytes),
					"error":    err.Error(),
					"duration": toolEndTime.Sub(toolStartTime).String(),
				})
				toolResultContent = fmt.Sprintf("Error: %v", err)
			}

			select {
			case eventCh <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventToolResult,
				Content:   toolResultContent,
				Timestamp: time.Now(),
				ToolCall: &interfaces.ToolCall{
					ID:        fmt.Sprintf("tool_%d_%s", iteration, functionCall.Name),
					Name:      functionCall.Name,
					Arguments: string(argsBytes),
				},
			}:
			case <-ctx.Done():
				return "", ctx.Err()
			}

			// Add tool result to conversation
			var resultContent *genai.Content
			if err != nil {
				c.logger.Error(ctx, "Error executing tool for streaming", map[string]interface{}{
					"toolName": selectedTool.Name(),
					"error":    err.Error(),
				})

				resultContent = &genai.Content{
					Role: "user",
					Parts: []*genai.Part{
						{
							FunctionResponse: &genai.FunctionResponse{
								Name: functionCall.Name,
								Response: map[string]any{
									"error": err.Error(),
								},
							},
						},
					},
				}
			} else {
				resultContent = &genai.Content{
					Role: "user",
					Parts: []*genai.Part{
						{
							FunctionResponse: &genai.FunctionResponse{
								Name: functionCall.Name,
								Response: map[string]any{
									"result": toolResult,
								},
							},
						},
					},
				}
			}

			contents = append(contents, resultContent)
		}
	}

	// If we've reached max iterations, make final call without tools
	c.logger.Info(ctx, "Maximum iterations reached for streaming, making final call without tools", map[string]interface{}{
		"maxIterations": maxIterations,
	})

	// Add conclusion instruction
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
	
	finalResult, err := c.client.Models.GenerateContent(ctx, c.model, contents, config)
	if err != nil {
		return "", fmt.Errorf("failed to create final content: %w", err)
	}

	if len(finalResult.Candidates) == 0 {
		return "", fmt.Errorf("no candidates returned in final call")
	}

	candidate := finalResult.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("no content in final response")
	}

	var finalResponse strings.Builder
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			finalResponse.WriteString(part.Text)
		}
	}

	return finalResponse.String(), nil
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
		
		// Add small delay to simulate streaming
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
}