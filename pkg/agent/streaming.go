package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tracing"
)

// RunStream executes the agent with streaming response
func (a *Agent) RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error) {
	// If custom stream function is set, use it instead
	if a.customRunStreamFunc != nil {
		return a.customRunStreamFunc(ctx, input, a)
	}

	// If this is a remote agent, delegate to remote execution
	if a.isRemote {
		return a.runRemoteStream(ctx, input)
	}

	// Local agent execution
	return a.runLocalStream(ctx, input)
}

// runLocalStream executes a local agent with streaming
func (a *Agent) runLocalStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error) {
	// Check if LLM supports streaming
	streamingLLM, ok := a.llm.(interfaces.StreamingLLM)
	if !ok {
		return nil, fmt.Errorf("LLM '%s' does not support streaming", a.llm.Name())
	}

	// Get buffer size from default config
	bufferSize := 100

	// Create agent event channel
	eventChan := make(chan interfaces.AgentStreamEvent, bufferSize)

	// Start streaming in a goroutine
	go func() {
		defer close(eventChan)

		// Track execution start time
		startTime := time.Now()

		// Inject agent name into context for tracing span naming
		ctx = tracing.WithAgentName(ctx, a.name)

		// If orgID is set on the agent, add it to the context
		if a.orgID != "" {
			ctx = multitenancy.WithOrgID(ctx, a.orgID)
		}

		// Create usage tracker for detailed metrics collection
		tracker := newUsageTracker(true)
		ctx = withUsageTracker(ctx, tracker)

		// Track response length for span logging
		var responseLength int64

		// Start tracing if available
		var span interfaces.Span
		if a.tracer != nil {
			ctx, span = a.tracer.StartSpan(ctx, "agent.RunStream")
			defer func() {
				// Add detailed execution information to span before ending
				if span != nil {
					executionTimeMs := time.Since(startTime).Milliseconds()
					tracker.setExecutionTime(executionTimeMs)

					usage, execSummary, model := tracker.getResults()

					// Add comprehensive span attributes
					spanData := map[string]interface{}{
						"agent_name":      a.name,
						"execution_time_ms": executionTimeMs,
						"input_length":    len(input),
						"response_length": responseLength,
					}

					// Add organization and conversation context if available
					if orgID, err := multitenancy.GetOrgID(ctx); err == nil && orgID != "" {
						spanData["org_id"] = orgID
					}
					if conversationID, ok := memory.GetConversationID(ctx); ok && conversationID != "" {
						spanData["conversation_id"] = conversationID
					}

					// Add token usage if available
					if usage != nil {
						spanData["input_tokens"] = usage.InputTokens
						spanData["output_tokens"] = usage.OutputTokens
						spanData["total_tokens"] = usage.TotalTokens
						spanData["reasoning_tokens"] = usage.ReasoningTokens
					}

					// Add execution summary if available
					if execSummary != nil {
						spanData["llm_calls"] = execSummary.LLMCalls
						spanData["tool_calls"] = execSummary.ToolCalls
						spanData["sub_agent_calls"] = execSummary.SubAgentCalls
						spanData["used_tools"] = execSummary.UsedTools
						spanData["used_sub_agents"] = execSummary.UsedSubAgents
					}

					// Add model information
					if model != "" {
						spanData["model_used"] = model
					} else if a.llm != nil {
						spanData["model_used"] = a.llm.Name()
					}

					// Log detailed execution information
					log.Printf("[Agent] RunStream execution completed: %+v", spanData)
				}
				span.End()
			}()
		}

		// Add user message to memory
		if a.memory != nil {
			if err := a.memory.AddMessage(ctx, interfaces.Message{
				Role:    "user",
				Content: input,
			}); err != nil {
				eventChan <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventError,
					Error:     fmt.Errorf("failed to add user message to memory: %w", err),
					Timestamp: time.Now(),
				}
				return
			}
		}

		// Apply guardrails to input if available
		processedInput := input
		if a.guardrails != nil {
			guardedInput, err := a.guardrails.ProcessInput(ctx, input)
			if err != nil {
				eventChan <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventError,
					Error:     fmt.Errorf("guardrails error: %w", err),
					Timestamp: time.Now(),
				}
				return
			}
			processedInput = guardedInput
		}

		// Check if the input is related to an existing plan
		taskID, action, planInput := a.extractPlanAction(processedInput)
		if taskID != "" {
			// For now, plan actions are not streamed - fall back to regular handling
			result, err := a.handlePlanAction(ctx, taskID, action, planInput)
			if err != nil {
				eventChan <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventError,
					Error:     err,
					Timestamp: time.Now(),
				}
			} else {
				eventChan <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventContent,
					Content:   result,
					Timestamp: time.Now(),
				}
			}
			return
		}

		// Check if the user is asking about the agent's role or identity
		if a.systemPrompt != "" && a.isAskingAboutRole(processedInput) {
			response := a.generateRoleResponse()

			// Add the role response to memory if available
			if a.memory != nil {
				if err := a.memory.AddMessage(ctx, interfaces.Message{
					Role:    "assistant",
					Content: response,
				}); err != nil {
					eventChan <- interfaces.AgentStreamEvent{
						Type:      interfaces.AgentEventError,
						Error:     fmt.Errorf("failed to add role response to memory: %w", err),
						Timestamp: time.Now(),
					}
					return
				}
			}

			eventChan <- interfaces.AgentStreamEvent{
				Type:      interfaces.AgentEventContent,
				Content:   response,
				Timestamp: time.Now(),
			}
			eventChan <- interfaces.AgentStreamEvent{
				Type:      interfaces.AgentEventComplete,
				Timestamp: time.Now(),
			}
			return
		}

		// Collect all tools
		allTools := a.tools

		// Add MCP tools if available
		if len(a.mcpServers) > 0 {
			mcpTools, err := a.collectMCPTools(ctx)
			if err != nil {
				// Log the error but continue with the agent tools
				a.logger.Warn(ctx, "Failed to collect MCP tools", map[string]interface{}{
					"error": err.Error(),
				})
			} else if len(mcpTools) > 0 {
				allTools = append(allTools, mcpTools...)
			}
		}

		// If tools are available and plan approval is required, we can't stream execution plans yet
		if (len(allTools) > 0) && a.requirePlanApproval {
			// For now, fall back to non-streaming execution plan generation
			result, err := a.runWithExecutionPlan(ctx, processedInput)
			if err != nil {
				eventChan <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventError,
					Error:     err,
					Timestamp: time.Now(),
				}
			} else {
				eventChan <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventContent,
					Content:   result,
					Timestamp: time.Now(),
				}
				eventChan <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventComplete,
					Timestamp: time.Now(),
				}
			}
			return
		}

		// Run with streaming
		length, err := a.runStreamingGeneration(ctx, processedInput, allTools, streamingLLM, eventChan)
		responseLength = length
		if err != nil {
			eventChan <- interfaces.AgentStreamEvent{
				Type:      interfaces.AgentEventError,
				Error:     err,
				Timestamp: time.Now(),
			}
		}
	}()

	return eventChan, nil
}

// runStreamingGeneration handles the core streaming generation logic
func (a *Agent) runStreamingGeneration(
	ctx context.Context,
	input string,
	allTools []interfaces.Tool,
	streamingLLM interfaces.StreamingLLM,
	eventChan chan<- interfaces.AgentStreamEvent,
) (int64, error) {
	// Prepare generation options
	options := []interfaces.GenerateOption{}

	// Add system prompt if available
	if a.systemPrompt != "" {
		options = append(options, func(opts *interfaces.GenerateOptions) {
			opts.SystemMessage = a.systemPrompt
		})
	}

	// Add LLM config if available
	if a.llmConfig != nil {
		options = append(options, func(opts *interfaces.GenerateOptions) {
			opts.LLMConfig = a.llmConfig
		})
	}

	// Add response format if available
	if a.responseFormat != nil {
		options = append(options, func(opts *interfaces.GenerateOptions) {
			opts.ResponseFormat = a.responseFormat
		})
	}

	// Add max iterations if available
	if a.maxIterations > 0 {
		options = append(options, interfaces.WithMaxIterations(a.maxIterations))
	}

	// Add memory if available
	if a.memory != nil {
		options = append(options, interfaces.WithMemory(a.memory))
	}

	// Add stream config if available
	if a.streamConfig != nil {
		options = append(options, interfaces.WithStreamConfig(*a.streamConfig))
	}

	// Start LLM streaming
	var llmEventChan <-chan interfaces.StreamEvent
	var err error

	if len(allTools) > 0 {
		llmEventChan, err = streamingLLM.GenerateWithToolsStream(ctx, input, allTools, options...)
	} else {
		llmEventChan, err = streamingLLM.GenerateStream(ctx, input, options...)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to start LLM streaming: %w", err)
	}

	// Track accumulated content and tool calls for memory
	var accumulatedContent strings.Builder
	var toolCalls []interfaces.ToolCall
	var toolResults map[string]string // map[toolCallID]result
	var finalError error

	toolResults = make(map[string]string)

	// Forward LLM events as agent events
	for llmEvent := range llmEventChan {
		agentEvent := a.convertLLMEventToAgentEvent(llmEvent, allTools)

		// Accumulate content for memory (not thinking)
		if llmEvent.Type == interfaces.StreamEventContentDelta {
			accumulatedContent.WriteString(llmEvent.Content)
		}

		// Track tool calls for memory
		if llmEvent.Type == interfaces.StreamEventToolUse && llmEvent.ToolCall != nil {
			toolCalls = append(toolCalls, *llmEvent.ToolCall)
		}

		// Track tool results for memory
		if llmEvent.Type == interfaces.StreamEventToolResult && llmEvent.ToolCall != nil {
			toolResults[llmEvent.ToolCall.ID] = llmEvent.Content
		}

		// Track errors
		if llmEvent.Error != nil {
			finalError = llmEvent.Error
		}

		// Send agent event
		eventChan <- agentEvent
	}

	// Add messages to memory if available (save even on error to preserve conversation history)
	if a.memory != nil {
		// If we have tool calls, save them in the correct order
		if len(toolCalls) > 0 {
			// Add assistant message with tool calls
			err := a.memory.AddMessage(ctx, interfaces.Message{
				Role:      "assistant",
				Content:   accumulatedContent.String(), // May be empty or contain text before tools
				ToolCalls: toolCalls,
			})
			if err != nil {
				a.logger.Warn(ctx, "Failed to add assistant message with tool calls to memory", map[string]interface{}{
					"error": err.Error(),
				})
			}

			// Add tool result messages
			for _, toolCall := range toolCalls {
				if result, ok := toolResults[toolCall.ID]; ok {
					err := a.memory.AddMessage(ctx, interfaces.Message{
						Role:       "tool",
						Content:    result,
						ToolCallID: toolCall.ID,
						Metadata: map[string]interface{}{
							"tool_name": toolCall.Name,
						},
					})
					if err != nil {
						a.logger.Warn(ctx, "Failed to add tool result to memory", map[string]interface{}{
							"tool_call_id": toolCall.ID,
							"tool_name":    toolCall.Name,
							"error":        err.Error(),
						})
					}
				}
			}
		} else if accumulatedContent.Len() > 0 {
			// No tool calls, just content - add assistant message
			err := a.memory.AddMessage(ctx, interfaces.Message{
				Role:    "assistant",
				Content: accumulatedContent.String(),
			})
			if err != nil {
				a.logger.Warn(ctx, "Failed to add assistant response to memory", map[string]interface{}{
					"error": err.Error(),
				})
			}
		}
	}

	// Send completion event
	eventChan <- interfaces.AgentStreamEvent{
		Type:      interfaces.AgentEventComplete,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"total_content_length": accumulatedContent.Len(),
			"had_error":            finalError != nil,
		},
	}

	return int64(accumulatedContent.Len()), finalError
}

// getToolMetadata retrieves display name and internal flag for a tool
func getToolMetadata(toolName string, tools []interfaces.Tool) (displayName string, internal bool) {
	displayName = toolName
	internal = false

	for _, tool := range tools {
		if tool.Name() == toolName {
			if toolWithDisplayName, ok := tool.(interfaces.ToolWithDisplayName); ok {
				if dn := toolWithDisplayName.DisplayName(); dn != "" {
					displayName = dn
				}
			}
			if internalTool, ok := tool.(interfaces.InternalTool); ok {
				internal = internalTool.Internal()
			}
			break
		}
	}

	return displayName, internal
}

// convertLLMEventToAgentEvent converts LLM events to agent events
func (a *Agent) convertLLMEventToAgentEvent(llmEvent interfaces.StreamEvent, tools []interfaces.Tool) interfaces.AgentStreamEvent {
	agentEvent := interfaces.AgentStreamEvent{
		Timestamp: llmEvent.Timestamp,
		Metadata:  llmEvent.Metadata,
	}

	// Convert event types
	switch llmEvent.Type {
	case interfaces.StreamEventMessageStart:
		agentEvent.Type = interfaces.AgentEventContent
		agentEvent.Content = llmEvent.Content

	case interfaces.StreamEventContentDelta:
		agentEvent.Type = interfaces.AgentEventContent
		agentEvent.Content = llmEvent.Content

	case interfaces.StreamEventContentComplete:
		agentEvent.Type = interfaces.AgentEventContent
		agentEvent.Content = llmEvent.Content

	case interfaces.StreamEventThinking:
		agentEvent.Type = interfaces.AgentEventThinking
		agentEvent.ThinkingStep = llmEvent.Content

	case interfaces.StreamEventToolUse:
		agentEvent.Type = interfaces.AgentEventToolCall
		if llmEvent.ToolCall != nil {
			displayName, internal := getToolMetadata(llmEvent.ToolCall.Name, tools)
			agentEvent.ToolCall = &interfaces.ToolCallEvent{
				ID:          llmEvent.ToolCall.ID,
				Name:        llmEvent.ToolCall.Name,
				DisplayName: displayName,
				Internal:    internal,
				Arguments:   llmEvent.ToolCall.Arguments,
				Status:      "received",
			}
		}

	case interfaces.StreamEventToolResult:
		agentEvent.Type = interfaces.AgentEventToolResult
		if llmEvent.ToolCall != nil {
			displayName, internal := getToolMetadata(llmEvent.ToolCall.Name, tools)
			agentEvent.ToolCall = &interfaces.ToolCallEvent{
				ID:          llmEvent.ToolCall.ID,
				Name:        llmEvent.ToolCall.Name,
				DisplayName: displayName,
				Internal:    internal,
				Arguments:   llmEvent.ToolCall.Arguments,
				Result:      llmEvent.Content, // Tool result is in Content field of StreamEvent
				Status:      "completed",
			}
		}

	case interfaces.StreamEventError:
		agentEvent.Type = interfaces.AgentEventError
		agentEvent.Error = llmEvent.Error

	case interfaces.StreamEventMessageStop:
		agentEvent.Type = interfaces.AgentEventContent
		agentEvent.Content = llmEvent.Content

	default:
		// Unknown event type, treat as content
		agentEvent.Type = interfaces.AgentEventContent
		agentEvent.Content = llmEvent.Content
	}

	return agentEvent
}

// runRemoteStream handles streaming for remote agents
func (a *Agent) runRemoteStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error) {
	if a.remoteClient == nil {
		return nil, fmt.Errorf("remote client not initialized")
	}

	// If orgID is set on the agent, add it to the context
	if a.orgID != "" {
		ctx = multitenancy.WithOrgID(ctx, a.orgID)
	}

	return a.remoteClient.RunStream(ctx, input)
}
