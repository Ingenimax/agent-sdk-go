package openai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/tracing"
	"github.com/openai/openai-go/v2/packages/param"
	"github.com/openai/openai-go/v2/responses"
	"github.com/openai/openai-go/v2/shared"
)

func (c *OpenAIClient) shouldUseResponsesAPI(params *interfaces.GenerateOptions, _ []interfaces.Tool) bool {
	if params != nil && len(params.FileInputs) > 0 {
		return true
	}
	return c.useResponsesAPI
}

func validateResponsesAPIOptions(params *interfaces.GenerateOptions) error {
	if params == nil {
		return nil
	}

	if params.Memory != nil {
		return fmt.Errorf("openai responses api does not support memory in this SDK path yet; remove WithMemory or disable WithResponsesAPI")
	}

	if params.ResponseFormat != nil {
		if params.ResponseFormat.Name == "" {
			return fmt.Errorf("openai responses api response format requires a non-empty name")
		}
		if len(params.ResponseFormat.Schema) == 0 {
			return fmt.Errorf("openai responses api response format requires a non-empty schema")
		}
	}

	if params.LLMConfig != nil {
		unsupported := []string{}
		if params.LLMConfig.FrequencyPenalty != 0 {
			unsupported = append(unsupported, "frequency_penalty")
		}
		if params.LLMConfig.PresencePenalty != 0 {
			unsupported = append(unsupported, "presence_penalty")
		}
		if len(params.LLMConfig.StopSequences) > 0 {
			unsupported = append(unsupported, "stop_sequences")
		}
		if len(unsupported) > 0 {
			return fmt.Errorf("openai responses api does not support %s in this SDK path", strings.Join(unsupported, ", "))
		}
	}

	for i, file := range params.FileInputs {
		sources := 0
		if file.FileID != "" {
			sources++
		}
		if file.FileURL != "" {
			sources++
		}
		if file.FileData != "" {
			sources++
		}
		if sources != 1 {
			return fmt.Errorf("openai file input %d must set exactly one of FileID, FileURL, or FileData", i)
		}
		if file.FileData != "" && file.Filename == "" {
			return fmt.Errorf("openai file input %d with FileData requires Filename", i)
		}
	}

	return nil
}

func validateOpenAIStreamingOptions(params *interfaces.GenerateOptions, useResponsesAPI bool) error {
	if useResponsesAPI {
		return fmt.Errorf("openai responses api streaming is not supported in this SDK path yet; use Generate or disable WithResponsesAPI")
	}
	if params != nil && len(params.FileInputs) > 0 {
		return fmt.Errorf("openai file inputs are not supported with streaming; use Generate or GenerateWithTools")
	}
	return nil
}

func (c *OpenAIClient) generateWithResponsesAPI(ctx context.Context, prompt string, tools []interfaces.Tool, params *interfaces.GenerateOptions) (*interfaces.LLMResponse, error) {
	if err := validateResponsesAPIOptions(params); err != nil {
		return nil, err
	}

	req := c.newResponseRequest(prompt, params)
	if len(tools) > 0 {
		req.Tools = buildResponseTools(tools)
		req.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto)}
		req.ParallelToolCalls = param.NewOpt(true)
	}

	maxIterations := params.MaxIterations
	if maxIterations == 0 {
		maxIterations = 2
	}

	input := req.Input.OfInputItemList
	var lastResp *responses.Response
	for iteration := 0; iteration < maxIterations; iteration++ {
		req.Input.OfInputItemList = input
		resp, err := c.Client.Responses.New(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to create response: %w", err)
		}
		lastResp = resp
		addResponseUsage(ctx, resp, c.Model)

		toolCalls := responseFunctionCalls(resp)
		if len(toolCalls) == 0 {
			return llmResponseFromOpenAIResponse(resp, c.Model), nil
		}

		for _, toolCall := range toolCalls {
			input = append(input, responses.ResponseInputItemUnionParam{OfFunctionCall: &responses.ResponseFunctionToolCallParam{
				ID:        param.NewOpt(toolCall.ID),
				CallID:    toolCall.CallID,
				Name:      toolCall.Name,
				Arguments: toolCall.Arguments,
				Status:    responses.ResponseFunctionToolCallStatusCompleted,
			}})

			result := c.executeResponseToolCall(ctx, tools, toolCall, params)
			input = append(input, responses.ResponseInputItemUnionParam{OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
				CallID: toolCall.CallID,
				Output: result,
			}})
		}
	}

	if params.DisableFinalSummary && lastResp != nil {
		return llmResponseFromOpenAIResponse(lastResp, c.Model), nil
	}

	input = append(input, responses.ResponseInputItemUnionParam{OfMessage: responseMessage("user", "Please provide your final response based on the information available. Do not request any additional tools.")})
	req.Input.OfInputItemList = input
	req.Tools = nil
	req.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{}
	finalResp, err := c.Client.Responses.New(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create final response: %w", err)
	}
	addResponseUsage(ctx, finalResp, c.Model)
	return llmResponseFromOpenAIResponse(finalResp, c.Model), nil
}

func (c *OpenAIClient) newResponseRequest(prompt string, params *interfaces.GenerateOptions) responses.ResponseNewParams {
	input := responses.ResponseInputParam{}
	if params.SystemMessage != "" {
		input = append(input, responses.ResponseInputItemUnionParam{OfMessage: responseMessage("system", params.SystemMessage)})
	}
	input = append(input, responses.ResponseInputItemUnionParam{OfMessage: responseUserMessage(prompt, params.FileInputs)})

	req := responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: input},
		Model: shared.ResponsesModel(c.Model),
		Store: param.NewOpt(false),
	}

	if params.LLMConfig != nil {
		if !isReasoningModel(c.Model) {
			req.Temperature = param.NewOpt(params.LLMConfig.Temperature)
			if params.LLMConfig.TopP > 0 && params.LLMConfig.TopP <= 1 {
				req.TopP = param.NewOpt(params.LLMConfig.TopP)
			}
		}
		if params.LLMConfig.Reasoning != "" {
			req.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffort(params.LLMConfig.Reasoning)}
		}
	}

	if params.ResponseFormat != nil {
		req.Text.Format = responses.ResponseFormatTextConfigUnionParam{OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
			Name:   params.ResponseFormat.Name,
			Schema: map[string]any(params.ResponseFormat.Schema),
			Strict: param.NewOpt(true),
		}}
	}

	return req
}

func responseMessage(role, content string) *responses.EasyInputMessageParam {
	return &responses.EasyInputMessageParam{
		Role: responses.EasyInputMessageRole(role),
		Content: responses.EasyInputMessageContentUnionParam{
			OfString: param.NewOpt(content),
		},
	}
}

func responseUserMessage(prompt string, files []interfaces.FileInput) *responses.EasyInputMessageParam {
	if len(files) == 0 {
		return responseMessage("user", prompt)
	}

	content := responses.ResponseInputMessageContentListParam{
		responses.ResponseInputContentUnionParam{OfInputText: &responses.ResponseInputTextParam{Text: prompt}},
	}
	for _, file := range files {
		inputFile := responses.ResponseInputFileParam{}
		if file.FileID != "" {
			inputFile.FileID = param.NewOpt(file.FileID)
		}
		if file.FileURL != "" {
			inputFile.FileURL = param.NewOpt(file.FileURL)
		}
		if file.FileData != "" {
			inputFile.FileData = param.NewOpt(file.FileData)
		}
		if file.Filename != "" {
			inputFile.Filename = param.NewOpt(file.Filename)
		}
		content = append(content, responses.ResponseInputContentUnionParam{OfInputFile: &inputFile})
	}

	return &responses.EasyInputMessageParam{
		Role: responses.EasyInputMessageRoleUser,
		Content: responses.EasyInputMessageContentUnionParam{
			OfInputItemContentList: content,
		},
	}
}

func buildResponseTools(tools []interfaces.Tool) []responses.ToolUnionParam {
	openaiTools := make([]responses.ToolUnionParam, len(tools))
	for i, tool := range tools {
		properties := make(map[string]any)
		required := []string{}
		for name, paramSpec := range tool.Parameters() {
			property := map[string]any{
				"type":        paramSpec.Type,
				"description": paramSpec.Description,
			}
			if paramSpec.Default != nil {
				property["default"] = paramSpec.Default
			}
			if paramSpec.Items != nil {
				items := map[string]any{"type": paramSpec.Items.Type}
				if paramSpec.Items.Enum != nil {
					items["enum"] = paramSpec.Items.Enum
				}
				property["items"] = items
			}
			if paramSpec.Enum != nil {
				property["enum"] = paramSpec.Enum
			}
			properties[name] = property
			if paramSpec.Required {
				required = append(required, name)
			}
		}

		openaiTools[i] = responses.ToolParamOfFunction(tool.Name(), map[string]any{
			"type":       "object",
			"properties": properties,
			"required":   required,
		}, true)
		openaiTools[i].OfFunction.Description = param.NewOpt(tool.Description())
	}
	return openaiTools
}

func responseFunctionCalls(resp *responses.Response) []responses.ResponseFunctionToolCall {
	toolCalls := []responses.ResponseFunctionToolCall{}
	for _, item := range resp.Output {
		if item.Type == "function_call" {
			toolCalls = append(toolCalls, item.AsFunctionCall())
		}
	}
	return toolCalls
}

func (c *OpenAIClient) executeResponseToolCall(ctx context.Context, tools []interfaces.Tool, toolCall responses.ResponseFunctionToolCall, params *interfaces.GenerateOptions) string {
	var selectedTool interfaces.Tool
	for _, tool := range tools {
		if tool.Name() == toolCall.Name {
			selectedTool = tool
			break
		}
	}

	start := time.Now()
	trace := tracing.ToolCall{
		Name:      toolCall.Name,
		Arguments: toolCall.Arguments,
		ID:        toolCall.CallID,
		Timestamp: start.Format(time.RFC3339),
		StartTime: start,
	}

	if selectedTool == nil {
		result := fmt.Sprintf("Error: tool not found: %s", toolCall.Name)
		trace.Error = result
		trace.Result = result
		tracing.AddToolCallToContext(ctx, trace)
		return result
	}

	result, err := selectedTool.Execute(ctx, toolCall.Arguments)
	trace.Duration = time.Since(start)
	trace.DurationMs = trace.Duration.Milliseconds()
	if err != nil {
		result = fmt.Sprintf("Error: %v", err)
		trace.Error = err.Error()
	}
	trace.Result = result
	tracing.AddToolCallToContext(ctx, trace)

	if params.Memory != nil {
		_ = params.Memory.AddMessage(ctx, interfaces.Message{Role: "assistant", Content: "", ToolCalls: []interfaces.ToolCall{{ID: toolCall.CallID, Name: toolCall.Name, Arguments: toolCall.Arguments}}})
		_ = params.Memory.AddMessage(ctx, interfaces.Message{Role: "tool", Content: result, ToolCallID: toolCall.CallID, Metadata: map[string]interface{}{"tool_name": toolCall.Name}})
	}

	return result
}

func llmResponseFromOpenAIResponse(resp *responses.Response, fallbackModel string) *interfaces.LLMResponse {
	model := string(resp.Model)
	if model == "" {
		model = fallbackModel
	}
	return &interfaces.LLMResponse{
		Content:    strings.TrimSpace(resp.OutputText()),
		Model:      model,
		StopReason: string(resp.Status),
		Usage: &interfaces.TokenUsage{
			InputTokens:     int(resp.Usage.InputTokens),
			OutputTokens:    int(resp.Usage.OutputTokens),
			TotalTokens:     int(resp.Usage.TotalTokens),
			ReasoningTokens: int(resp.Usage.OutputTokensDetails.ReasoningTokens),
		},
		Metadata: map[string]interface{}{
			"provider": "openai",
			"endpoint": "responses",
		},
	}
}

func addResponseUsage(ctx context.Context, resp *responses.Response, model string) {
	if acc := getUsageAccumulator(ctx); acc != nil {
		acc.add(
			int(resp.Usage.InputTokens),
			int(resp.Usage.OutputTokens),
			int(resp.Usage.TotalTokens),
			int(resp.Usage.OutputTokensDetails.ReasoningTokens),
			model,
		)
	}
}
