package openai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tracing"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/responses"
	"github.com/openai/openai-go/v2/shared"
)

// shouldUseResponsesAPI reports whether a tools call must route through the
// /v1/responses endpoint instead of /v1/chat/completions. Chat Completions
// 400s when reasoning_effort and tools are sent together for gpt-5 reasoning
// models; the Responses API supports reasoning + tools in one request.
func shouldUseResponsesAPI(model string, reasoning string, toolCount int) bool {
	return isReasoningModel(model) && reasoning != "" && toolCount > 0
}

// generateWithToolsResponses runs the iterative tool-calling loop over the
// OpenAI Responses API so reasoning and tool use can coexist. It mirrors the
// behavior of GenerateWithTools (loop detection, memory persistence, tracing,
// usage accumulation, final-summary call) using the Responses input-item model.
func (c *OpenAIClient) generateWithToolsResponses(ctx context.Context, prompt string, tools []interfaces.Tool, params *interfaces.GenerateOptions) (string, error) {
	maxIterations := params.MaxIterations
	if maxIterations == 0 {
		maxIterations = 2
	}

	orgID := "default"
	if id, err := multitenancy.GetOrgID(ctx); err == nil {
		orgID = id
	}
	ctx = context.WithValue(ctx, organizationKey, orgID)

	// Convert tools to Responses function-tool params
	responseTools := c.responseToolParams(tools)

	// Build initial input items from memory, or the bare prompt when no memory
	inputItems := c.buildResponseInput(ctx, prompt, params.Memory)

	// Base request shared across loop iterations
	baseReq := responses.ResponseNewParams{
		Model:     shared.ResponsesModel(c.Model),
		Tools:     responseTools,
		Reasoning: shared.ReasoningParam{Effort: shared.ReasoningEffort(params.LLMConfig.Reasoning)},
		Store:     openai.Bool(true),
	}
	if params.SystemMessage != "" {
		baseReq.Instructions = openai.String(params.SystemMessage)
	}
	applyResponseFormat(&baseReq, params.ResponseFormat)

	toolCallHistory := make(map[string]int)
	var lastContent string
	var previousResponseID string

	for iteration := 0; iteration < maxIterations; iteration++ {
		req := baseReq
		if previousResponseID != "" {
			req.PreviousResponseID = openai.String(previousResponseID)
		}
		req.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: inputItems}

		c.logger.Debug(ctx, "Sending request with tools to OpenAI Responses API", map[string]interface{}{
			"model":            c.Model,
			"tools":            len(responseTools),
			"reasoning_effort": params.LLMConfig.Reasoning,
			"iteration":        iteration + 1,
			"maxIterations":    maxIterations,
			"chained":          previousResponseID != "",
		})

		resp, err := c.ResponseService.Responses.New(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "Error from OpenAI Responses API", map[string]interface{}{"error": err.Error()})
			return "", fmt.Errorf("failed to create response: %w", err)
		}

		if acc := getUsageAccumulator(ctx); acc != nil {
			acc.add(
				int(resp.Usage.InputTokens),
				int(resp.Usage.OutputTokens),
				int(resp.Usage.TotalTokens),
				int(resp.Usage.OutputTokensDetails.ReasoningTokens),
				c.Model,
			)
		}

		lastContent = strings.TrimSpace(resp.OutputText())
		previousResponseID = resp.ID

		// Collect function-call output items
		var functionCalls []responses.ResponseFunctionToolCall
		for _, item := range resp.Output {
			if item.Type == "function_call" {
				functionCalls = append(functionCalls, item.AsFunctionCall())
			}
		}

		if len(functionCalls) == 0 {
			return lastContent, nil
		}

		c.logger.Info(ctx, "Processing tool calls", map[string]interface{}{
			"count":     len(functionCalls),
			"iteration": iteration + 1,
		})

		// Execute tools; each result becomes a function_call_output input item
		// chained to this response via previous_response_id.
		nextInput := responses.ResponseInputParam{}
		for _, call := range functionCalls {
			output := c.executeResponseToolCall(ctx, call, tools, toolCallHistory, params)
			nextInput = append(nextInput, responses.ResponseInputItemParamOfFunctionCallOutput(call.CallID, output))
		}
		inputItems = nextInput
	}

	if params.DisableFinalSummary {
		c.logger.Info(ctx, "DisableFinalSummary enabled, skipping final summary call", map[string]interface{}{
			"maxIterations": maxIterations,
		})
		return lastContent, nil
	}

	// Max iterations reached: one final call without tools to force a conclusion
	c.logger.Info(ctx, "Maximum iterations reached, making final call without tools", map[string]interface{}{
		"maxIterations": maxIterations,
	})

	finalReq := responses.ResponseNewParams{
		Model:              shared.ResponsesModel(c.Model),
		Reasoning:          shared.ReasoningParam{Effort: shared.ReasoningEffort(params.LLMConfig.Reasoning)},
		Store:              openai.Bool(true),
		PreviousResponseID: openai.String(previousResponseID),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: responses.ResponseInputParam{
			responses.ResponseInputItemParamOfMessage(
				"Please provide your final response based on the information available. Do not request any additional tools.",
				responses.EasyInputMessageRoleUser,
			),
		}},
	}
	if params.SystemMessage != "" {
		finalReq.Instructions = openai.String(params.SystemMessage)
	}
	applyResponseFormat(&finalReq, params.ResponseFormat)

	finalResp, err := c.ResponseService.Responses.New(ctx, finalReq)
	if err != nil {
		c.logger.Error(ctx, "Error in final Responses call without tools", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("failed to create final response: %w", err)
	}

	if acc := getUsageAccumulator(ctx); acc != nil {
		acc.add(
			int(finalResp.Usage.InputTokens),
			int(finalResp.Usage.OutputTokens),
			int(finalResp.Usage.TotalTokens),
			int(finalResp.Usage.OutputTokensDetails.ReasoningTokens),
			c.Model,
		)
	}

	content := strings.TrimSpace(finalResp.OutputText())
	c.logger.Info(ctx, "Successfully received final response without tools", nil)
	return content, nil
}

// buildResponseInput converts memory and the current prompt into Responses
// input items. Tool-call round-trips from prior memory are omitted to avoid
// dangling call_id references; plain user/assistant/system text is preserved.
func (c *OpenAIClient) buildResponseInput(ctx context.Context, prompt string, memory interfaces.Memory) responses.ResponseInputParam {
	items := responses.ResponseInputParam{}

	if memory == nil {
		items = append(items, responses.ResponseInputItemParamOfMessage(prompt, responses.EasyInputMessageRoleUser))
		return items
	}

	memoryMessages, err := memory.GetMessages(ctx)
	if err != nil {
		c.logger.Error(ctx, "Failed to retrieve memory messages", map[string]interface{}{"error": err.Error()})
		return items
	}

	for _, msg := range memoryMessages {
		switch msg.Role {
		case interfaces.MessageRoleUser:
			items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleUser))
		case interfaces.MessageRoleAssistant:
			if msg.Content != "" {
				items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleAssistant))
			}
		case interfaces.MessageRoleSystem:
			items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleSystem))
		}
	}
	return items
}

// executeResponseToolCall runs a single function tool call, applying loop
// detection, memory persistence, and tracing, and returns the output string
// to send back as a function_call_output item.
func (c *OpenAIClient) executeResponseToolCall(ctx context.Context, call responses.ResponseFunctionToolCall, tools []interfaces.Tool, toolCallHistory map[string]int, params *interfaces.GenerateOptions) string {
	var selectedTool interfaces.Tool
	for _, tool := range tools {
		if tool.Name() == call.Name {
			selectedTool = tool
			break
		}
	}

	if selectedTool == nil || selectedTool.Name() == "" {
		errorMessage := fmt.Sprintf("Error: tool not found: %s", call.Name)
		c.logger.Error(ctx, "Tool not found", map[string]interface{}{"toolName": call.Name})
		if params.Memory != nil {
			persistToolMemory(ctx, params.Memory, call, errorMessage)
		}
		tracing.AddToolCallToContext(ctx, tracing.ToolCall{
			Name:      call.Name,
			Arguments: call.Arguments,
			ID:        call.CallID,
			Timestamp: time.Now().Format(time.RFC3339),
			StartTime: time.Now(),
			Error:     fmt.Sprintf("tool not found: %s", call.Name),
			Result:    errorMessage,
		})
		return errorMessage
	}

	c.logger.Info(ctx, "Executing tool", map[string]interface{}{"toolName": selectedTool.Name()})
	toolStartTime := time.Now()
	toolResult, err := selectedTool.Execute(ctx, call.Arguments)
	toolEndTime := time.Now()

	// Loop detection on identical name+arguments
	cacheKey := call.Name + ":" + call.Arguments
	toolCallHistory[cacheKey]++
	if callCount := toolCallHistory[cacheKey]; callCount > 1 {
		warning := fmt.Sprintf("\n\n[WARNING: This is call #%d to %s with identical parameters. You may be in a loop. Consider using the available information to provide a final answer.]", callCount, call.Name)
		if err == nil {
			toolResult += warning
		}
		c.logger.Warn(ctx, "Repetitive tool call detected", map[string]interface{}{
			"toolName":  call.Name,
			"callCount": callCount,
		})
	}

	executionDuration := toolEndTime.Sub(toolStartTime)
	toolCallTrace := tracing.ToolCall{
		Name:       call.Name,
		Arguments:  call.Arguments,
		ID:         call.CallID,
		Timestamp:  toolStartTime.Format(time.RFC3339),
		StartTime:  toolStartTime,
		Duration:   executionDuration,
		DurationMs: executionDuration.Milliseconds(),
	}

	var output string
	if err != nil {
		c.logger.Error(ctx, "Error executing tool", map[string]interface{}{"toolName": selectedTool.Name(), "error": err.Error()})
		output = fmt.Sprintf("Error: %v", err)
		toolCallTrace.Error = err.Error()
		toolCallTrace.Result = output
	} else {
		output = toolResult
		toolCallTrace.Result = toolResult
	}

	if params.Memory != nil {
		persistToolMemory(ctx, params.Memory, call, output)
	}

	tracing.AddToolCallToContext(ctx, toolCallTrace)
	return output
}

// persistToolMemory stores an assistant tool-call and its result in memory,
// mirroring the Chat Completions path.
func persistToolMemory(ctx context.Context, memory interfaces.Memory, call responses.ResponseFunctionToolCall, result string) {
	_ = memory.AddMessage(ctx, interfaces.Message{
		Role:    "assistant",
		Content: "",
		ToolCalls: []interfaces.ToolCall{{
			ID:        call.CallID,
			Name:      call.Name,
			Arguments: call.Arguments,
		}},
	})
	_ = memory.AddMessage(ctx, interfaces.Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: call.CallID,
		Metadata:   map[string]interface{}{"tool_name": call.Name},
	})
}

// responseToolParams converts SDK tools to Responses function-tool params,
// reusing the same JSON-schema conversion as the Chat Completions path.
func (c *OpenAIClient) responseToolParams(tools []interfaces.Tool) []responses.ToolUnionParam {
	responseTools := make([]responses.ToolUnionParam, len(tools))
	for i, tool := range tools {
		responseTools[i] = responses.ToolParamOfFunction(tool.Name(), c.convertToOpenAISchema(tool.Parameters()), false)
	}
	return responseTools
}

// applyResponseFormat sets a json_schema structured-output format on the request,
// mirroring the Chat Completions ResponseFormat handling.
func applyResponseFormat(req *responses.ResponseNewParams, rf *interfaces.ResponseFormat) {
	if rf == nil {
		return
	}
	req.Text = responses.ResponseTextConfigParam{
		Format: responses.ResponseFormatTextConfigUnionParam{
			OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
				Name:   rf.Name,
				Schema: map[string]interface{}(rf.Schema),
			},
		},
	}
}

// streamErrorEvent builds a terminal stream error event.
func streamErrorEvent(err error) interfaces.StreamEvent {
	return interfaces.StreamEvent{
		Type:      interfaces.StreamEventError,
		Error:     fmt.Errorf("openai responses streaming error: %w", err),
		Timestamp: time.Now(),
	}
}

// generateWithToolsResponsesStream streams the iterative tool-calling loop over
// the OpenAI Responses API so reasoning and tool use coexist while streaming.
// It mirrors GenerateWithToolsStream's event contract (message start, content
// deltas, tool use/result, content complete, message stop) and its
// intermediate-content filtering, executing tools between streamed turns and
// chaining via previous_response_id.
func (c *OpenAIClient) generateWithToolsResponsesStream(ctx context.Context, prompt string, tools []interfaces.Tool, params *interfaces.GenerateOptions) <-chan interfaces.StreamEvent {
	bufferSize := 100
	if params.StreamConfig != nil && params.StreamConfig.BufferSize > 0 {
		bufferSize = params.StreamConfig.BufferSize
	}
	eventChan := make(chan interfaces.StreamEvent, bufferSize)

	go func() {
		defer close(eventChan)

		maxIterations := params.MaxIterations
		if maxIterations == 0 {
			maxIterations = 2
		}

		orgID := "default"
		if id, err := multitenancy.GetOrgID(ctx); err == nil {
			orgID = id
		}
		ctx := context.WithValue(ctx, organizationKey, orgID)

		responseTools := c.responseToolParams(tools)
		inputItems := c.buildResponseInput(ctx, prompt, params.Memory)

		baseReq := responses.ResponseNewParams{
			Model:     shared.ResponsesModel(c.Model),
			Tools:     responseTools,
			Reasoning: shared.ReasoningParam{Effort: shared.ReasoningEffort(params.LLMConfig.Reasoning)},
			Store:     openai.Bool(true),
		}
		if params.SystemMessage != "" {
			baseReq.Instructions = openai.String(params.SystemMessage)
		}

		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStart,
			Timestamp: time.Now(),
			Metadata:  map[string]interface{}{"model": c.Model, "tools": len(responseTools)},
		}

		// Match GenerateWithToolsStream: hold back content emitted during
		// tool-calling iterations and replay it after the loop unless the caller
		// opts into intermediate messages.
		filterIntermediateContent := params.StreamConfig == nil || !params.StreamConfig.IncludeIntermediateMessages
		var capturedContentEvents []interfaces.StreamEvent
		toolCallHistory := make(map[string]int)
		var previousResponseID string
		gotComplete := false

		for iteration := 0; iteration < maxIterations; iteration++ {
			lastIteration := iteration == maxIterations-1

			req := baseReq
			if previousResponseID != "" {
				req.PreviousResponseID = openai.String(previousResponseID)
			}
			req.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: inputItems}

			stream := c.ResponseService.Responses.NewStreaming(ctx, req)
			if stream.Err() != nil {
				c.logger.Error(ctx, "Failed to create OpenAI Responses streaming", map[string]interface{}{"error": stream.Err().Error()})
				eventChan <- streamErrorEvent(stream.Err())
				return
			}

			var finalResp responses.Response
			var iterationContentEvents []interfaces.StreamEvent
			hasContent := false
			completed := false
			for stream.Next() {
				ev := stream.Current()
				switch ev.Type {
				case "response.output_text.delta":
					hasContent = true
					contentEvent := interfaces.StreamEvent{
						Type:      interfaces.StreamEventContentDelta,
						Content:   ev.Delta,
						Timestamp: time.Now(),
						Metadata:  map[string]interface{}{"iteration": iteration + 1},
					}
					if filterIntermediateContent && !lastIteration {
						iterationContentEvents = append(iterationContentEvents, contentEvent)
					} else {
						eventChan <- contentEvent
					}
				case "response.completed", "response.incomplete":
					// "incomplete" carries a full response too (e.g. max tokens).
					finalResp = ev.Response
					completed = true
				case "error", "response.failed":
					c.logger.Error(ctx, "OpenAI Responses streaming event error", map[string]interface{}{"type": ev.Type})
					eventChan <- interfaces.StreamEvent{
						Type:      interfaces.StreamEventError,
						Error:     fmt.Errorf("openai responses streaming failed: %s", ev.Type),
						Timestamp: time.Now(),
					}
					return
				}
			}
			if err := stream.Err(); err != nil {
				c.logger.Error(ctx, "OpenAI Responses streaming error", map[string]interface{}{"error": err.Error()})
				eventChan <- streamErrorEvent(err)
				return
			}
			if !completed {
				c.logger.Error(ctx, "OpenAI Responses stream ended without a completed response", map[string]interface{}{"iteration": iteration + 1})
				eventChan <- streamErrorEvent(fmt.Errorf("stream ended without a completed response"))
				return
			}

			if acc := getUsageAccumulator(ctx); acc != nil {
				acc.add(
					int(finalResp.Usage.InputTokens),
					int(finalResp.Usage.OutputTokens),
					int(finalResp.Usage.TotalTokens),
					int(finalResp.Usage.OutputTokensDetails.ReasoningTokens),
					c.Model,
				)
			}
			previousResponseID = finalResp.ID

			var functionCalls []responses.ResponseFunctionToolCall
			for _, item := range finalResp.Output {
				if item.Type == "function_call" {
					functionCalls = append(functionCalls, item.AsFunctionCall())
				}
			}

			if len(functionCalls) == 0 {
				// Terminal turn: no tools requested. If content was held back
				// (not the last iteration), emit it now as the final answer.
				if filterIntermediateContent && !lastIteration {
					for _, e := range iterationContentEvents {
						eventChan <- e
					}
				}
				if hasContent {
					eventChan <- interfaces.StreamEvent{
						Type:      interfaces.StreamEventContentComplete,
						Timestamp: time.Now(),
						Metadata:  map[string]interface{}{"iteration": iteration + 1},
					}
				}
				gotComplete = true
				break
			}

			if filterIntermediateContent && !lastIteration {
				capturedContentEvents = append(capturedContentEvents, iterationContentEvents...)
			}

			nextInput := responses.ResponseInputParam{}
			for _, call := range functionCalls {
				eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventToolUse,
					ToolCall:  &interfaces.ToolCall{ID: call.CallID, Name: call.Name, Arguments: call.Arguments},
					Timestamp: time.Now(),
					Metadata:  map[string]interface{}{"iteration": iteration + 1},
				}
				output := c.executeResponseToolCall(ctx, call, tools, toolCallHistory, params)
				eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventToolResult,
					Content:   output,
					ToolCall:  &interfaces.ToolCall{ID: call.CallID, Name: call.Name, Arguments: call.Arguments},
					Timestamp: time.Now(),
					Metadata:  map[string]interface{}{"iteration": iteration + 1, "result": output},
				}
				nextInput = append(nextInput, responses.ResponseInputItemParamOfFunctionCallOutput(call.CallID, output))
			}
			inputItems = nextInput
		}

		// Replay content held back during tool iterations.
		if filterIntermediateContent && len(capturedContentEvents) > 0 {
			for _, e := range capturedContentEvents {
				eventChan <- e
			}
		}

		if gotComplete {
			eventChan <- interfaces.StreamEvent{Type: interfaces.StreamEventMessageStop, Timestamp: time.Now()}
			return
		}

		if params.DisableFinalSummary {
			c.logger.Info(ctx, "DisableFinalSummary enabled, skipping final synthesis call", map[string]interface{}{"maxIterations": maxIterations})
			eventChan <- interfaces.StreamEvent{Type: interfaces.StreamEventMessageStop, Timestamp: time.Now()}
			return
		}

		// Final synthesis call without tools, chained to the loop's last response.
		c.logger.Info(ctx, "Maximum iterations reached, making final streaming call without tools", map[string]interface{}{"maxIterations": maxIterations})
		finalReq := responses.ResponseNewParams{
			Model:              shared.ResponsesModel(c.Model),
			Reasoning:          shared.ReasoningParam{Effort: shared.ReasoningEffort(params.LLMConfig.Reasoning)},
			Store:              openai.Bool(true),
			PreviousResponseID: openai.String(previousResponseID),
			Input: responses.ResponseNewParamsInputUnion{OfInputItemList: responses.ResponseInputParam{
				responses.ResponseInputItemParamOfMessage(
					"Please provide your final response based on the information available. Do not request any additional tools.",
					responses.EasyInputMessageRoleUser,
				),
			}},
		}
		if params.SystemMessage != "" {
			finalReq.Instructions = openai.String(params.SystemMessage)
		}
		applyResponseFormat(&finalReq, params.ResponseFormat)

		finalStream := c.ResponseService.Responses.NewStreaming(ctx, finalReq)
		if finalStream.Err() != nil {
			c.logger.Error(ctx, "Error in final Responses streaming call", map[string]interface{}{"error": finalStream.Err().Error()})
			eventChan <- streamErrorEvent(finalStream.Err())
			return
		}

		var finalResp responses.Response
		finalCompleted := false
		for finalStream.Next() {
			ev := finalStream.Current()
			switch ev.Type {
			case "response.output_text.delta":
				eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventContentDelta,
					Content:   ev.Delta,
					Timestamp: time.Now(),
					Metadata:  map[string]interface{}{"final_call": true},
				}
			case "response.completed", "response.incomplete":
				finalResp = ev.Response
				finalCompleted = true
			case "error", "response.failed":
				c.logger.Error(ctx, "OpenAI final Responses streaming event error", map[string]interface{}{"type": ev.Type})
				eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventError,
					Error:     fmt.Errorf("openai responses final streaming failed: %s", ev.Type),
					Timestamp: time.Now(),
				}
				return
			}
		}
		if err := finalStream.Err(); err != nil {
			c.logger.Error(ctx, "OpenAI final Responses streaming error", map[string]interface{}{"error": err.Error()})
			eventChan <- streamErrorEvent(err)
			return
		}
		if !finalCompleted {
			c.logger.Error(ctx, "OpenAI final Responses stream ended without a completed response", nil)
			eventChan <- streamErrorEvent(fmt.Errorf("final stream ended without a completed response"))
			return
		}

		if acc := getUsageAccumulator(ctx); acc != nil {
			acc.add(
				int(finalResp.Usage.InputTokens),
				int(finalResp.Usage.OutputTokens),
				int(finalResp.Usage.TotalTokens),
				int(finalResp.Usage.OutputTokensDetails.ReasoningTokens),
				c.Model,
			)
		}

		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventContentComplete,
			Timestamp: time.Now(),
			Metadata:  map[string]interface{}{"final_call": true},
		}
		eventChan <- interfaces.StreamEvent{Type: interfaces.StreamEventMessageStop, Timestamp: time.Now()}
	}()

	return eventChan
}
