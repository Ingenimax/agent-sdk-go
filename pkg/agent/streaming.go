package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tracing"
)

// RunStream executes the agent with streaming response
func (a *Agent) RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error) {
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

		// Inject agent name into context for tracing span naming
		ctx = tracing.WithAgentName(ctx, a.name)

		// If orgID is set on the agent, add it to the context
		if a.orgID != "" {
			ctx = multitenancy.WithOrgID(ctx, a.orgID)
		}

		// Start tracing if available
		var span interfaces.Span
		if a.tracer != nil {
			ctx, span = a.tracer.StartSpan(ctx, "agent.RunStream")
			defer span.End()
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
				// Warning: Failed to collect MCP tools
				fmt.Printf("Warning: Failed to collect MCP tools: %v\n", err)
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
		if err := a.runStreamingGeneration(ctx, processedInput, allTools, streamingLLM, eventChan); err != nil {
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
	tools []interfaces.Tool,
	streamingLLM interfaces.StreamingLLM,
	eventChan chan<- interfaces.AgentStreamEvent,
) error {
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

	// Start LLM streaming
	var llmEventChan <-chan interfaces.StreamEvent
	var err error

	if len(tools) > 0 {
		llmEventChan, err = streamingLLM.GenerateWithToolsStream(ctx, input, tools, options...)
	} else {
		llmEventChan, err = streamingLLM.GenerateStream(ctx, input, options...)
	}

	if err != nil {
		return fmt.Errorf("failed to start LLM streaming: %w", err)
	}

	// Track accumulated content for memory
	var accumulatedContent strings.Builder
	var finalError error

	// Forward LLM events as agent events
	for llmEvent := range llmEventChan {
		agentEvent := a.convertLLMEventToAgentEvent(llmEvent)

		// Handle tool calls specially
		if llmEvent.Type == interfaces.StreamEventToolUse && llmEvent.ToolCall != nil {
			// Execute tool and send progress events
			a.handleToolCallStreaming(ctx, llmEvent.ToolCall, tools, eventChan)
		}

		// Accumulate content for memory
		if llmEvent.Type == interfaces.StreamEventContentDelta {
			accumulatedContent.WriteString(llmEvent.Content)
		}

		// Track errors
		if llmEvent.Error != nil {
			finalError = llmEvent.Error
		}

		// Send agent event
		eventChan <- agentEvent
	}

	// Add accumulated content to memory if available and no error occurred
	if a.memory != nil && finalError == nil && accumulatedContent.Len() > 0 {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    "assistant",
			Content: accumulatedContent.String(),
		}); err != nil {
			// Warning: Failed to add assistant response to memory
			fmt.Printf("Warning: Failed to add assistant response to memory: %v\n", err)
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

	return finalError
}

// convertLLMEventToAgentEvent converts LLM events to agent events
func (a *Agent) convertLLMEventToAgentEvent(llmEvent interfaces.StreamEvent) interfaces.AgentStreamEvent {
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
			agentEvent.ToolCall = &interfaces.ToolCallEvent{
				ID:        llmEvent.ToolCall.ID,
				Name:      llmEvent.ToolCall.Name,
				Arguments: llmEvent.ToolCall.Arguments,
				Status:    "received",
			}
		}

	case interfaces.StreamEventToolResult:
		agentEvent.Type = interfaces.AgentEventToolResult
		if llmEvent.ToolCall != nil {
			agentEvent.ToolCall = &interfaces.ToolCallEvent{
				ID:     llmEvent.ToolCall.ID,
				Name:   llmEvent.ToolCall.Name,
				Result: "", // LLM StreamEvent ToolCall doesn't have Result field
				Status: "completed",
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

// handleToolCallStreaming executes a tool call and sends progress events
func (a *Agent) handleToolCallStreaming(
	ctx context.Context,
	toolCall *interfaces.ToolCall,
	tools []interfaces.Tool,
	eventChan chan<- interfaces.AgentStreamEvent,
) {
	// Send tool execution start event
	eventChan <- interfaces.AgentStreamEvent{
		Type: interfaces.AgentEventToolCall,
		ToolCall: &interfaces.ToolCallEvent{
			ID:        toolCall.ID,
			Name:      toolCall.Name,
			Arguments: toolCall.Arguments,
			Status:    "executing",
		},
		Timestamp: time.Now(),
	}

	// Find the requested tool
	var selectedTool interfaces.Tool
	for _, tool := range tools {
		if tool.Name() == toolCall.Name {
			selectedTool = tool
			break
		}
	}

	if selectedTool == nil {
		// Send tool result event with error instead of error event
		errorMessage := fmt.Sprintf("Error: tool not found: %s", toolCall.Name)
		eventChan <- interfaces.AgentStreamEvent{
			Type: interfaces.AgentEventToolResult,
			ToolCall: &interfaces.ToolCallEvent{
				ID:        toolCall.ID,
				Name:      toolCall.Name,
				Arguments: toolCall.Arguments,
				Result:    errorMessage,
				Status:    "error",
			},
			Error:     fmt.Errorf("tool not found: %s", toolCall.Name),
			Timestamp: time.Now(),
		}
		return
	}

	// Execute the tool
	toolResult, err := selectedTool.Execute(ctx, toolCall.Arguments)

	// Send tool result event
	resultEvent := interfaces.AgentStreamEvent{
		Type: interfaces.AgentEventToolResult,
		ToolCall: &interfaces.ToolCallEvent{
			ID:        toolCall.ID,
			Name:      toolCall.Name,
			Arguments: toolCall.Arguments,
			Result:    toolResult,
		},
		Timestamp: time.Now(),
	}

	if err != nil {
		resultEvent.Error = err
		resultEvent.ToolCall.Status = "error"
		resultEvent.ToolCall.Result = fmt.Sprintf("Error: %v", err)
	} else {
		resultEvent.ToolCall.Status = "completed"
	}

	eventChan <- resultEvent
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
