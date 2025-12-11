package deepseek

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// StreamChunk represents a chunk from the DeepSeek streaming API
type StreamChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
	Usage   *Usage         `json:"usage,omitempty"`
}

// StreamChoice represents a choice in a streaming response
type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        StreamDelta `json:"delta"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

// StreamDelta represents the delta content in a streaming chunk
type StreamDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// GenerateStream implements interfaces.StreamingLLM.GenerateStream
func (c *DeepSeekClient) GenerateStream(
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
		messages := []Message{}

		// Add system message if provided
		if params.SystemMessage != "" {
			messages = append(messages, Message{
				Role:    "system",
				Content: params.SystemMessage,
			})
			c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": params.SystemMessage})
		}

		// Build messages using unified builder
		builder := newMessageHistoryBuilder(c.logger)
		messages = append(messages, builder.buildMessages(ctx, prompt, params.Memory)...)

		// Create stream request
		req := ChatCompletionRequest{
			Model:    c.Model,
			Messages: messages,
			Stream:   true,
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

		// Add structured output if specified
		if params.ResponseFormat != nil {
			req.ResponseFormat = &ResponseFormatParam{
				Type:       "json_schema",
				JSONSchema: params.ResponseFormat.Schema,
			}
		}

		// Log the request
		c.logger.Debug(ctx, "Creating DeepSeek streaming request", map[string]interface{}{
			"model":       c.Model,
			"temperature": params.LLMConfig.Temperature,
			"top_p":       params.LLMConfig.TopP,
		})

		// Make streaming HTTP request
		resp, err := c.doStreamRequest(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "DeepSeek streaming error", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventError,
				Error:     fmt.Errorf("deepseek streaming error: %w", err),
				Timestamp: time.Now(),
			}
			return
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				c.logger.Error(ctx, "Failed to close response body", map[string]interface{}{
					"error": err.Error(),
				})
			}
		}()

		// Send initial message start event
		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStart,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"model": c.Model,
			},
		}

		// Process stream chunks
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines
			if line == "" {
				continue
			}

			// Parse SSE format: "data: {json}"
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			// Check for stream end marker
			if data == "[DONE]" {
				break
			}

			// Parse JSON chunk
			var chunk StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				c.logger.Error(ctx, "Failed to parse stream chunk", map[string]interface{}{
					"error": err.Error(),
					"data":  data,
				})
				continue
			}

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

			// Handle usage information
			if chunk.Usage != nil && (chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0) {
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

		// Check for scanner error
		if err := scanner.Err(); err != nil {
			c.logger.Error(ctx, "DeepSeek stream scanner error", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventError,
				Error:     fmt.Errorf("deepseek stream scanner error: %w", err),
				Timestamp: time.Now(),
			}
			return
		}

		// Send final message stop event
		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStop,
			Timestamp: time.Now(),
		}

		c.logger.Debug(ctx, "Successfully completed DeepSeek streaming request", map[string]interface{}{
			"model": c.Model,
		})
	}()

	return eventChan, nil
}

// GenerateWithToolsStream implements interfaces.StreamingLLM.GenerateWithToolsStream with iterative tool calling
func (c *DeepSeekClient) GenerateWithToolsStream(
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

		// Convert tools to DeepSeek format
		deepseekTools := c.convertToolsToDeepSeekFormat(tools)

		// Build messages
		messages := []Message{}

		// Add system message if provided
		if params.SystemMessage != "" {
			messages = append(messages, Message{
				Role:    "system",
				Content: params.SystemMessage,
			})
			c.logger.Debug(ctx, "Using system message for tools", map[string]interface{}{"system_message": params.SystemMessage})
		}

		// Build messages using unified builder
		builder := newMessageHistoryBuilder(c.logger)
		messages = append(messages, builder.buildMessages(ctx, prompt, params.Memory)...)

		// Send initial message start event
		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStart,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"model": c.Model,
				"tools": len(deepseekTools),
			},
		}

		// Determine if we should filter intermediate content
		filterIntermediateContent := params.StreamConfig == nil || !params.StreamConfig.IncludeIntermediateMessages

		// Track captured content for final iteration replay if filtering is enabled
		var capturedContentEvents []interfaces.StreamEvent

		// Track if we got a complete response (no tool calls)
		gotCompleteResponse := false

		// Iterative tool calling loop
		for iteration := 0; iteration < maxIterations; iteration++ {
			iterationHasContent := false
			var iterationContentEvents []interfaces.StreamEvent

			// Create request for this iteration
			req := ChatCompletionRequest{
				Model:      c.Model,
				Messages:   messages,
				Tools:      deepseekTools,
				ToolChoice: "auto",
				Stream:     true,
			}

			if params.LLMConfig != nil {
				req.Temperature = params.LLMConfig.Temperature
				req.TopP = params.LLMConfig.TopP
				req.FrequencyPenalty = params.LLMConfig.FrequencyPenalty
				req.PresencePenalty = params.LLMConfig.PresencePenalty
			}

			c.logger.Debug(ctx, "Creating DeepSeek streaming request with tools", map[string]interface{}{
				"model":         c.Model,
				"tools":         len(deepseekTools),
				"temperature":   params.LLMConfig.Temperature,
				"iteration":     iteration + 1,
				"maxIterations": maxIterations,
				"message_count": len(messages),
			})

			// Make streaming HTTP request
			resp, err := c.doStreamRequest(ctx, req)
			if err != nil {
				c.logger.Error(ctx, "Failed to create DeepSeek streaming", map[string]interface{}{
					"error": err.Error(),
				})
				eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventError,
					Error:     fmt.Errorf("deepseek streaming error: %w", err),
					Timestamp: time.Now(),
				}
				return
			}

			// Track streaming state
			var currentToolCall *interfaces.ToolCall
			var toolCallBuffer strings.Builder
			var assistantResponse Message
			assistantResponse.Role = "assistant"
			var hasContent bool

			// Process stream chunks
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()

				// Skip empty lines
				if line == "" {
					continue
				}

				// Parse SSE format: "data: {json}"
				if !strings.HasPrefix(line, "data: ") {
					continue
				}

				data := strings.TrimPrefix(line, "data: ")

				// Check for stream end marker
				if data == "[DONE]" {
					break
				}

				// Parse JSON chunk
				var chunk StreamChunk
				if err := json.Unmarshal([]byte(data), &chunk); err != nil {
					c.logger.Error(ctx, "Failed to parse stream chunk", map[string]interface{}{
						"error": err.Error(),
						"data":  data,
					})
					continue
				}

				// Process choices
				for _, choice := range chunk.Choices {
					// Handle content
					if choice.Delta.Content != "" {
						hasContent = true
						iterationHasContent = true
						assistantResponse.Content += choice.Delta.Content

						contentEvent := interfaces.StreamEvent{
							Type:      interfaces.StreamEventContentDelta,
							Content:   choice.Delta.Content,
							Timestamp: time.Now(),
							Metadata: map[string]interface{}{
								"choice_index": choice.Index,
								"iteration":    iteration + 1,
							},
						}

						if filterIntermediateContent && len(deepseekTools) > 0 && iteration < maxIterations-1 {
							// Capture content for potential replay later
							iterationContentEvents = append(iterationContentEvents, contentEvent)
						} else {
							// Stream content immediately
							eventChan <- contentEvent
						}
					}

					// Handle tool calls - DeepSeek streams them incrementally
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
									assistantResponse.ToolCalls = append(assistantResponse.ToolCalls, ToolCall{
										ID:   toolCall.ID,
										Type: "function",
										Function: FunctionCall{
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

			if err := resp.Body.Close(); err != nil {
				c.logger.Error(ctx, "Failed to close response body in iteration", map[string]interface{}{
					"error": err.Error(),
				})
			}

			// Check for scanner error
			if err := scanner.Err(); err != nil {
				c.logger.Error(ctx, "DeepSeek streaming with tools error", map[string]interface{}{
					"error": err.Error(),
					"model": c.Model,
				})
				eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventError,
					Error:     fmt.Errorf("deepseek streaming error: %w", err),
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
				gotCompleteResponse = true
				break // Exit the iteration loop
			}

			// The model wants to use tools
			c.logger.Info(ctx, "Processing tool calls", map[string]interface{}{
				"count":     len(assistantResponse.ToolCalls),
				"iteration": iteration + 1,
			})

			// Add the assistant's message with tool calls to the conversation
			messages = append(messages, assistantResponse)

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
					"tool_name":     toolCall.Function.Name,
					"result_length": len(result),
				})

				messages = append(messages, Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: toolCall.ID,
				})
			}

			// If we had content during this iteration and tools were called, capture it for final replay
			if filterIntermediateContent && iterationHasContent && len(assistantResponse.ToolCalls) > 0 {
				capturedContentEvents = append(capturedContentEvents, iterationContentEvents...)
			}
		}

		// Replay captured content events if we filtered them during iterations
		if filterIntermediateContent && len(capturedContentEvents) > 0 {
			c.logger.Debug(ctx, "Replaying captured content events from tool iterations", map[string]interface{}{
				"eventsCount": len(capturedContentEvents),
			})
			for _, contentEvent := range capturedContentEvents {
				eventChan <- contentEvent
			}
		}

		// If we got a complete response (no tool calls), skip the final synthesis call
		if gotCompleteResponse {
			c.logger.Debug(ctx, "Skipping final synthesis call - already got complete response", map[string]interface{}{
				"maxIterations": maxIterations,
			})
			eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventMessageStop,
				Timestamp: time.Now(),
			}
			return
		}

		// Final call without tools to get synthesis
		c.logger.Info(ctx, "Maximum iterations reached, making final call without tools", map[string]interface{}{
			"maxIterations": maxIterations,
		})

		// Add explicit message to inform LLM this is the final call
		finalMessages := append(messages, Message{
			Role:    "user",
			Content: "Please provide your final response based on the information available. Do not request any additional tools.",
		})

		// Create final request without tools
		finalReq := ChatCompletionRequest{
			Model:    c.Model,
			Messages: finalMessages,
			Stream:   true,
		}

		if params.LLMConfig != nil {
			finalReq.Temperature = params.LLMConfig.Temperature
			finalReq.TopP = params.LLMConfig.TopP
			finalReq.FrequencyPenalty = params.LLMConfig.FrequencyPenalty
			finalReq.PresencePenalty = params.LLMConfig.PresencePenalty
		}

		// Add structured output if specified
		if params.ResponseFormat != nil {
			finalReq.ResponseFormat = &ResponseFormatParam{
				Type:       "json_schema",
				JSONSchema: params.ResponseFormat.Schema,
			}
		}

		c.logger.Debug(ctx, "Making final streaming call without tools", map[string]interface{}{
			"model": c.Model,
		})

		// Make final streaming request
		finalResp, err := c.doStreamRequest(ctx, finalReq)
		if err != nil {
			c.logger.Error(ctx, "Error in final streaming call without tools", map[string]interface{}{
				"error": err.Error(),
			})
			eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventError,
				Error:     fmt.Errorf("deepseek final streaming error: %w", err),
				Timestamp: time.Now(),
			}
			return
		}
		defer func() {
			if err := finalResp.Body.Close(); err != nil {
				c.logger.Error(ctx, "Failed to close final response body", map[string]interface{}{
					"error": err.Error(),
				})
			}
		}()

		// Process final stream
		finalScanner := bufio.NewScanner(finalResp.Body)
		for finalScanner.Scan() {
			line := finalScanner.Text()

			// Skip empty lines
			if line == "" {
				continue
			}

			// Parse SSE format: "data: {json}"
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			// Check for stream end marker
			if data == "[DONE]" {
				break
			}

			// Parse JSON chunk
			var chunk StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				c.logger.Error(ctx, "Failed to parse final stream chunk", map[string]interface{}{
					"error": err.Error(),
					"data":  data,
				})
				continue
			}

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
		if err := finalScanner.Err(); err != nil {
			c.logger.Error(ctx, "DeepSeek final streaming error", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventError,
				Error:     fmt.Errorf("deepseek final streaming error: %w", err),
				Timestamp: time.Now(),
			}
			return
		}

		// Send final message stop event
		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStop,
			Timestamp: time.Now(),
		}

		c.logger.Debug(ctx, "Successfully completed DeepSeek streaming request with tools", map[string]interface{}{
			"model": c.Model,
		})
	}()

	return eventChan, nil
}

// doStreamRequest makes a streaming HTTP request to the DeepSeek API
func (c *DeepSeekClient) doStreamRequest(ctx context.Context, req ChatCompletionRequest) (*http.Response, error) {
	// Marshal request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	// Make request
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if err := resp.Body.Close(); err != nil {
			return nil, fmt.Errorf("deepseek API error (status %d): %s (close error: %w)", resp.StatusCode, string(body), err)
		}
		return nil, fmt.Errorf("deepseek API error (status %d): %s", resp.StatusCode, string(body))
	}

	return resp, nil
}
