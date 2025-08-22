package openai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/shared"
)

// GenerateStream implements interfaces.StreamingLLM.GenerateStream
func (c *OpenAIClient) GenerateStream(
	ctx context.Context,
	prompt string,
	options ...interfaces.GenerateOption,
) (<-chan interfaces.StreamEvent, error) {
	// Apply options
	params := &interfaces.GenerateOptions{
		LLMConfig: &interfaces.LLMConfig{
			Temperature: 0.7,
		},
	}

	for _, option := range options {
		option(params)
	}

	// Check for organization ID in context
	defaultOrgID := "default"
	if id, err := multitenancy.GetOrgID(ctx); err == nil {
		ctx = multitenancy.WithOrgID(ctx, id)
	} else {
		ctx = multitenancy.WithOrgID(ctx, defaultOrgID)
	}

	// Get buffer size from stream config
	bufferSize := 100
	if params.StreamConfig != nil {
		bufferSize = params.StreamConfig.BufferSize
	}

	// Create event channel
	eventChan := make(chan interfaces.StreamEvent, bufferSize)

	// Start streaming in a goroutine
	go func() {
		defer close(eventChan)

		// Build messages
		messages := []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		}

		// Reasoning models don't support system messages
		if params.SystemMessage != "" && !isReasoningModel(c.Model) {
			messages = append([]openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage(params.SystemMessage),
			}, messages...)
		}

		// Create stream request
		streamParams := openai.ChatCompletionNewParams{
			Model:    openai.ChatModel(c.Model),
			Messages: messages,
		}
		
		// Reasoning models only support temperature=1 (default), so don't set it
		if !isReasoningModel(c.Model) {
			streamParams.Temperature = openai.Float(params.LLMConfig.Temperature)
		}

		// Add structured output if specified
		if params.ResponseFormat != nil {
			jsonSchema := shared.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:   params.ResponseFormat.Name,
				Schema: params.ResponseFormat.Schema,
			}

			streamParams.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
					Type:       "json_schema",
					JSONSchema: jsonSchema,
				},
			}
		}

		// Handle reasoning models and reasoning config
		if isReasoningModel(c.Model) || (params.LLMConfig != nil && params.LLMConfig.EnableReasoning) {
			// o1 models or reasoning enabled - ensure we get usage info
			streamParams.StreamOptions = openai.ChatCompletionStreamOptionsParam{
				IncludeUsage: openai.Bool(true),
			}
			
			// Log reasoning support
			if isReasoningModel(c.Model) {
				c.logger.Debug(ctx, "Using reasoning model with built-in reasoning", map[string]interface{}{
					"model": c.Model,
					"note":  "reasoning models have internal reasoning but don't expose raw thinking tokens in streaming",
				})
			} else if params.LLMConfig != nil && params.LLMConfig.EnableReasoning {
				c.logger.Debug(ctx, "Reasoning enabled for non-reasoning model", map[string]interface{}{
					"model": c.Model,
					"note":  "reasoning tokens not supported for this model type",
				})
			}
		}

		// Add other LLM config parameters
		if params.LLMConfig != nil {
			if params.LLMConfig.TopP > 0 {
				streamParams.TopP = openai.Float(params.LLMConfig.TopP)
			}
			if params.LLMConfig.FrequencyPenalty != 0 {
				streamParams.FrequencyPenalty = openai.Float(params.LLMConfig.FrequencyPenalty)
			}
			if params.LLMConfig.PresencePenalty != 0 {
				streamParams.PresencePenalty = openai.Float(params.LLMConfig.PresencePenalty)
			}
			if len(params.LLMConfig.StopSequences) > 0 {
				streamParams.Stop = openai.ChatCompletionNewParamsStopUnion{
					OfStringArray: params.LLMConfig.StopSequences,
				}
			}
		}

		// Log the request
		c.logger.Debug(ctx, "Creating OpenAI streaming request", map[string]interface{}{
			"model":       c.Model,
			"temperature": params.LLMConfig.Temperature,
			"top_p":       params.LLMConfig.TopP,
			"is_reasoning_model": isReasoningModel(c.Model),
		})

		// Create stream
		stream := c.ChatService.Completions.NewStreaming(ctx, streamParams)

		// Send initial message start event
		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStart,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"model": c.Model,
			},
		}

		// Process stream chunks
		for stream.Next() {
			chunk := stream.Current()

			// Process choices
			for _, choice := range chunk.Choices {
				// Handle content delta
				if choice.Delta.Content != "" {
					eventChan <- interfaces.StreamEvent{
						Type:      interfaces.StreamEventContentDelta,
						Content:   choice.Delta.Content,
						Timestamp: time.Now(),
						Metadata: map[string]interface{}{
							"choice_index": choice.Index,
						},
					}
				}

				// Handle tool calls
				if len(choice.Delta.ToolCalls) > 0 {
					for _, toolCall := range choice.Delta.ToolCalls {
						if toolCall.Function.Name != "" || toolCall.Function.Arguments != "" {
							eventChan <- interfaces.StreamEvent{
								Type: interfaces.StreamEventToolUse,
								ToolCall: &interfaces.ToolCall{
									ID:        toolCall.ID,
									Name:      toolCall.Function.Name,
									Arguments: toolCall.Function.Arguments,
								},
								Timestamp: time.Now(),
								Metadata: map[string]interface{}{
									"choice_index": choice.Index,
									"call_type":    "tool_call",
									"tool_index":   toolCall.Index,
								},
							}
						}
					}
				}

				// Check for finish reason
				if choice.FinishReason != "" {
					eventChan <- interfaces.StreamEvent{
						Type: interfaces.StreamEventContentComplete,
						Metadata: map[string]interface{}{
							"finish_reason": choice.FinishReason,
							"choice_index":  choice.Index,
						},
						Timestamp: time.Now(),
					}
				}
			}

			// Handle usage information (especially for o1 models)
			if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 || chunk.Usage.TotalTokens > 0 {
				eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventContentDelta,
					Timestamp: time.Now(),
					Metadata: map[string]interface{}{
						"usage": map[string]interface{}{
							"prompt_tokens":     chunk.Usage.PromptTokens,
							"completion_tokens": chunk.Usage.CompletionTokens,
							"total_tokens":      chunk.Usage.TotalTokens,
						},
					},
				}
			}
		}

		// Check for stream error
		if err := stream.Err(); err != nil {
			c.logger.Error(ctx, "OpenAI streaming error", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventError,
				Error:     fmt.Errorf("openai streaming error: %w", err),
				Timestamp: time.Now(),
			}
			return
		}

		// Send final message stop event
		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStop,
			Timestamp: time.Now(),
		}

		c.logger.Debug(ctx, "Successfully completed OpenAI streaming request", map[string]interface{}{
			"model": c.Model,
		})
	}()

	return eventChan, nil
}

// GenerateWithToolsStream implements interfaces.StreamingLLM.GenerateWithToolsStream with iterative tool calling
func (c *OpenAIClient) GenerateWithToolsStream(
	ctx context.Context,
	prompt string,
	tools []interfaces.Tool,
	options ...interfaces.GenerateOption,
) (<-chan interfaces.StreamEvent, error) {
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
		maxIterations = 2
	}

	// Check for organization ID in context
	defaultOrgID := "default"
	if id, err := multitenancy.GetOrgID(ctx); err == nil {
		ctx = multitenancy.WithOrgID(ctx, id)
	} else {
		ctx = multitenancy.WithOrgID(ctx, defaultOrgID)
	}

	// Get buffer size from stream config
	bufferSize := 100
	if params.StreamConfig != nil {
		bufferSize = params.StreamConfig.BufferSize
	}

	// Create event channel
	eventChan := make(chan interfaces.StreamEvent, bufferSize)

	// Start streaming with iterative tool calling
	go func() {
		defer close(eventChan)

		// Convert tools to OpenAI format
		openaiTools := make([]openai.ChatCompletionToolUnionParam, len(tools))
		for i, tool := range tools {
			schema := c.convertToOpenAISchema(tool.Parameters())
			
			openaiTools[i] = openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        tool.Name(),
				Description: openai.String(tool.Description()),
				Parameters:  schema,
			})
		}

		// Build initial messages
		messages := []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		}

		// Reasoning models don't support system messages
		if params.SystemMessage != "" && !isReasoningModel(c.Model) {
			messages = append([]openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage(params.SystemMessage),
			}, messages...)
		}

		// Send initial message start event
		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStart,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"model": c.Model,
				"tools": len(openaiTools),
			},
		}

		// Iterative tool calling loop
		for iteration := 0; iteration < maxIterations; iteration++ {
			streamParams := openai.ChatCompletionNewParams{
				Model:      openai.ChatModel(c.Model),
				Messages:   messages,
				Tools:      openaiTools,
				ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String("auto")},
			}
			
			// Reasoning models only support temperature=1 (default), so don't set it
			if !isReasoningModel(c.Model) {
				streamParams.Temperature = openai.Float(params.LLMConfig.Temperature)
			}

			// Handle reasoning models
			if isReasoningModel(c.Model) || (params.LLMConfig != nil && params.LLMConfig.EnableReasoning) {
				streamParams.StreamOptions = openai.ChatCompletionStreamOptionsParam{
					IncludeUsage: openai.Bool(true),
				}
				
				if isReasoningModel(c.Model) {
					c.logger.Debug(ctx, "Using reasoning model with built-in reasoning for tools", map[string]interface{}{
						"model": c.Model,
						"note":  "reasoning models have internal reasoning but don't expose raw thinking tokens in streaming",
					})
				} else {
					c.logger.Debug(ctx, "Reasoning enabled for non-reasoning model with tools", map[string]interface{}{
						"model": c.Model,
						"note":  "reasoning tokens not supported for this model type",
					})
				}
			}

			// Add other LLM parameters
			if params.LLMConfig != nil {
				if params.LLMConfig.TopP > 0 {
					streamParams.TopP = openai.Float(params.LLMConfig.TopP)
				}
				if params.LLMConfig.FrequencyPenalty != 0 {
					streamParams.FrequencyPenalty = openai.Float(params.LLMConfig.FrequencyPenalty)
				}
				if params.LLMConfig.PresencePenalty != 0 {
					streamParams.PresencePenalty = openai.Float(params.LLMConfig.PresencePenalty)
				}
			}

			c.logger.Debug(ctx, "Creating OpenAI streaming request with tools", map[string]interface{}{
				"model":         c.Model,
				"tools":         len(openaiTools),
				"temperature":   params.LLMConfig.Temperature,
				"iteration":     iteration + 1,
				"maxIterations": maxIterations,
				"message_count": len(messages),
			})
			
			// Debug log messages array for second iteration
			if iteration > 0 {
				c.logger.Debug(ctx, "Messages array for iteration", map[string]interface{}{
					"iteration":     iteration + 1,
					"message_count": len(messages),
				})
				for i, msg := range messages {
					c.logger.Debug(ctx, "Message details", map[string]interface{}{
						"index": i,
						"type":  fmt.Sprintf("%T", msg),
					})
				}
			}

			// Create stream
			stream := c.ChatService.Completions.NewStreaming(ctx, streamParams)
			if stream.Err() != nil {
				c.logger.Error(ctx, "Failed to create OpenAI streaming", map[string]interface{}{
					"error": stream.Err().Error(),
				})
				eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventError,
					Error:     fmt.Errorf("openai streaming error: %w", stream.Err()),
					Timestamp: time.Now(),
				}
				return
			}

			// Track streaming state
			var currentToolCall *interfaces.ToolCall
			var toolCallBuffer strings.Builder
			var assistantResponse openai.ChatCompletionMessage
			var hasContent bool

			// Process stream chunks
			for stream.Next() {
				chunk := stream.Current()

				for _, choice := range chunk.Choices {
					// Handle content
					if choice.Delta.Content != "" {
						hasContent = true
						assistantResponse.Content += choice.Delta.Content
						eventChan <- interfaces.StreamEvent{
							Type:      interfaces.StreamEventContentDelta,
							Content:   choice.Delta.Content,
							Timestamp: time.Now(),
							Metadata: map[string]interface{}{
								"choice_index": choice.Index,
								"iteration":    iteration + 1,
							},
						}
					}

					// Handle tool calls - OpenAI streams them incrementally
					if len(choice.Delta.ToolCalls) > 0 {
						for _, toolCall := range choice.Delta.ToolCalls {
							if toolCall.Function.Name != "" || toolCall.Function.Arguments != "" {
								// Check if this is a new tool call or continuation
								if toolCall.Function.Name != "" {
									// New tool call started
									if currentToolCall != nil && toolCallBuffer.Len() > 0 {
										// Finish previous tool call
										currentToolCall.Arguments = toolCallBuffer.String()
										eventChan <- interfaces.StreamEvent{
											Type:      interfaces.StreamEventToolUse,
											ToolCall:  currentToolCall,
											Timestamp: time.Now(),
										}
									}

									// Start new tool call
									currentToolCall = &interfaces.ToolCall{
										ID:   toolCall.ID,
										Name: toolCall.Function.Name,
									}
									toolCallBuffer.Reset()
									
									// Add to assistant response
									assistantResponse.ToolCalls = append(assistantResponse.ToolCalls, openai.ChatCompletionMessageToolCallUnion{
										ID:   toolCall.ID,
										Type: "function",
										Function: openai.ChatCompletionMessageFunctionToolCallFunction{
											Name: toolCall.Function.Name,
										},
									})

									c.logger.Debug(ctx, "Started new tool call", map[string]interface{}{
										"tool_id":   toolCall.ID,
										"tool_name": toolCall.Function.Name,
									})
								}

								// Accumulate arguments
								if toolCall.Function.Arguments != "" {
									toolCallBuffer.WriteString(toolCall.Function.Arguments)
									// Update the last tool call arguments
									if len(assistantResponse.ToolCalls) > 0 {
										lastIdx := len(assistantResponse.ToolCalls) - 1
										assistantResponse.ToolCalls[lastIdx].Function.Arguments += toolCall.Function.Arguments
									}
								}
							}
						}
					}

					// Check for finish reason
					if choice.FinishReason == "tool_calls" && currentToolCall != nil {
						// Finish last tool call
						currentToolCall.Arguments = toolCallBuffer.String()
						eventChan <- interfaces.StreamEvent{
							Type:      interfaces.StreamEventToolUse,
							ToolCall:  currentToolCall,
							Timestamp: time.Now(),
							Metadata: map[string]interface{}{
								"finish_reason": "tool_calls",
								"iteration":     iteration + 1,
							},
						}
						currentToolCall = nil
						toolCallBuffer.Reset()

						c.logger.Debug(ctx, "Finished tool calls", map[string]interface{}{
							"finish_reason": choice.FinishReason,
							"iteration":     iteration + 1,
						})
					}
				}
			}

			// Check for stream error
			if err := stream.Err(); err != nil {
				c.logger.Error(ctx, "OpenAI streaming with tools error", map[string]interface{}{
					"error": err.Error(),
					"model": c.Model,
				})
				eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventError,
					Error:     fmt.Errorf("openai streaming error: %w", err),
					Timestamp: time.Now(),
				}
				return
			}

			// Check if the model wants to use tools
			if len(assistantResponse.ToolCalls) == 0 {
				// No tool calls, we're done
				if hasContent {
					eventChan <- interfaces.StreamEvent{
						Type:      interfaces.StreamEventContentComplete,
						Timestamp: time.Now(),
						Metadata: map[string]interface{}{
							"iteration": iteration + 1,
						},
					}
				}
				break // Exit the iteration loop
			}

			// The model wants to use tools
			c.logger.Info(ctx, "Processing tool calls", map[string]interface{}{
				"count":     len(assistantResponse.ToolCalls),
				"iteration": iteration + 1,
			})
			
			// Debug log all tool calls in assistant response
			for i, tc := range assistantResponse.ToolCalls {
				c.logger.Debug(ctx, "Assistant tool call", map[string]interface{}{
					"index":     i,
					"id":        tc.ID,
					"id_length": len(tc.ID),
					"name":      tc.Function.Name,
					"args_len":  len(tc.Function.Arguments),
				})
			}

			// Add the assistant's message with tool calls to the conversation
			assistantResponse.Role = "assistant"
			messages = append(messages, assistantResponse.ToParam())

			// Process each tool call
			for _, toolCall := range assistantResponse.ToolCalls {
				// Find the matching tool
				var foundTool interfaces.Tool
				for _, tool := range tools {
					if tool.Name() == toolCall.Function.Name {
						foundTool = tool
						break
					}
				}

				if foundTool == nil {
					c.logger.Error(ctx, "Tool not found", map[string]interface{}{
						"tool_name": toolCall.Function.Name,
					})
					continue
				}

				// Execute the tool
				result, err := foundTool.Execute(ctx, toolCall.Function.Arguments)
				if err != nil {
					c.logger.Error(ctx, "Tool execution error", map[string]interface{}{
						"tool_name": toolCall.Function.Name,
						"error":     err.Error(),
					})
					result = fmt.Sprintf("Error executing tool: %v", err)
				}

				// Send tool result event
				eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventToolResult,
					Timestamp: time.Now(),
					ToolCall: &interfaces.ToolCall{
						ID:        toolCall.ID,
						Name:      toolCall.Function.Name,
						Arguments: toolCall.Function.Arguments,
					},
					Metadata: map[string]interface{}{
						"iteration": iteration + 1,
						"result":    result,
					},
				}

				// Add the tool result to the conversation
				c.logger.Debug(ctx, "Adding tool result to conversation", map[string]interface{}{
					"tool_call_id":  toolCall.ID,
					"id_length":     len(toolCall.ID),
					"tool_name":     toolCall.Function.Name,
					"result_length": len(result),
				})
				
				// Ensure tool call ID is not swapped with result
				if len(toolCall.ID) > 40 {
					c.logger.Error(ctx, "Tool call ID too long", map[string]interface{}{
						"id":        toolCall.ID,
						"id_length": len(toolCall.ID),
					})
					continue
				}
				
				// Create tool message - correct parameter order: content first, then tool_call_id
				toolMessage := openai.ToolMessage(result, toolCall.ID)
				c.logger.Debug(ctx, "Created tool message", map[string]interface{}{
					"message_type": "tool",
				})
				messages = append(messages, toolMessage)
			}
		}

		// Final call without tools to get synthesis
		c.logger.Info(ctx, "Maximum iterations reached, making final call without tools", map[string]interface{}{
			"maxIterations": maxIterations,
		})

		// Add explicit message to inform LLM this is the final call
		finalMessages := append(messages, openai.UserMessage("Please provide your final response based on the information available. Do not request any additional tools."))

		// Create final request without tools
		finalStreamParams := openai.ChatCompletionNewParams{
			Model:    openai.ChatModel(c.Model),
			Messages: finalMessages,
		}
		
		// Reasoning models only support temperature=1 (default), so don't set it
		if !isReasoningModel(c.Model) {
			finalStreamParams.Temperature = openai.Float(params.LLMConfig.Temperature)
		}

		// Add other parameters
		if params.LLMConfig != nil {
			if params.LLMConfig.TopP > 0 {
				finalStreamParams.TopP = openai.Float(params.LLMConfig.TopP)
			}
			if params.LLMConfig.FrequencyPenalty != 0 {
				finalStreamParams.FrequencyPenalty = openai.Float(params.LLMConfig.FrequencyPenalty)
			}
			if params.LLMConfig.PresencePenalty != 0 {
				finalStreamParams.PresencePenalty = openai.Float(params.LLMConfig.PresencePenalty)
			}
		}

		c.logger.Debug(ctx, "Making final streaming call without tools", map[string]interface{}{
			"model": c.Model,
		})

		// Create final stream
		finalStream := c.ChatService.Completions.NewStreaming(ctx, finalStreamParams)
		if finalStream.Err() != nil {
			c.logger.Error(ctx, "Error in final streaming call without tools", map[string]interface{}{
				"error": finalStream.Err().Error(),
			})
			eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventError,
				Error:     fmt.Errorf("openai final streaming error: %w", finalStream.Err()),
				Timestamp: time.Now(),
			}
			return
		}

		// Process final stream
		for finalStream.Next() {
			chunk := finalStream.Current()

			for _, choice := range chunk.Choices {
				// Handle final content
				if choice.Delta.Content != "" {
					eventChan <- interfaces.StreamEvent{
						Type:      interfaces.StreamEventContentDelta,
						Content:   choice.Delta.Content,
						Timestamp: time.Now(),
						Metadata: map[string]interface{}{
							"choice_index": choice.Index,
							"final_call":   true,
						},
					}
				}

				// Check for finish reason
				if choice.FinishReason != "" {
					eventChan <- interfaces.StreamEvent{
						Type: interfaces.StreamEventContentComplete,
						Metadata: map[string]interface{}{
							"finish_reason": choice.FinishReason,
							"choice_index":  choice.Index,
							"final_call":    true,
						},
						Timestamp: time.Now(),
					}
				}
			}
		}

		// Check for final stream error
		if err := finalStream.Err(); err != nil {
			c.logger.Error(ctx, "OpenAI final streaming error", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventError,
				Error:     fmt.Errorf("openai final streaming error: %w", err),
				Timestamp: time.Now(),
			}
			return
		}

		// Send final message stop event
		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStop,
			Timestamp: time.Now(),
		}

		c.logger.Debug(ctx, "Successfully completed OpenAI streaming request with tools", map[string]interface{}{
			"model": c.Model,
		})
	}()

	return eventChan, nil
}

// convertToOpenAISchema converts tool parameters to OpenAI function schema
func (c *OpenAIClient) convertToOpenAISchema(params map[string]interfaces.ParameterSpec) map[string]interface{} {
	properties := make(map[string]interface{})
	required := []string{}

	for name, param := range params {
		property := map[string]interface{}{
			"type":        param.Type,
			"description": param.Description,
		}

		if param.Default != nil {
			property["default"] = param.Default
		}

		if param.Items != nil {
			property["items"] = map[string]interface{}{
				"type": param.Items.Type,
			}
			if param.Items.Enum != nil {
				property["items"].(map[string]interface{})["enum"] = param.Items.Enum
			}
		}

		if param.Enum != nil {
			property["enum"] = param.Enum
		}

		properties[name] = property

		if param.Required {
			required = append(required, name)
		}
	}

	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}