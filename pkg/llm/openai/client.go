package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
	"github.com/Ingenimax/agent-sdk-go/pkg/tracing"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/shared"
)

// Define a custom type for context keys to avoid collisions
type contextKey string

// Define constants for context keys
const organizationKey contextKey = "organization"

// OpenAIClient implements the LLM interface for OpenAI
type OpenAIClient struct {
	Client          openai.Client
	ChatService     openai.ChatService
	ResponseService openai.Client
	Model           string
	apiKey          string
	baseURL         string
	logger          logging.Logger
	retryExecutor   *retry.Executor
}

// Option represents an option for configuring the OpenAI client
type Option func(*OpenAIClient)

// WithModel sets the model for the OpenAI client
func WithModel(model string) Option {
	return func(c *OpenAIClient) {
		c.Model = model
	}
}

// isReasoningModel returns true if the model is a reasoning model that requires temperature = 1
func isReasoningModel(model string) bool {
	reasoningModels := []string{
		"o1-", "o1-mini", "o1-preview",
		"o3-", "o3-mini",
		"o4-", "o4-mini",
		"gpt-5", "gpt-5-mini", "gpt-5-nano",
	}

	for _, prefix := range reasoningModels {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

// getTemperatureForModel returns the appropriate temperature for a model
func (c *OpenAIClient) getTemperatureForModel(requestedTemp float64) float64 {
	if isReasoningModel(c.Model) {
		if requestedTemp != 1.0 {
			c.logger.Debug(context.Background(), "Overriding temperature for reasoning model", map[string]interface{}{
				"model":                 c.Model,
				"requested_temperature": requestedTemp,
				"forced_temperature":    1.0,
				"reason":                "reasoning models only support temperature = 1",
			})
		}
		return 1.0
	}
	return requestedTemp
}

// WithLogger sets the logger for the OpenAI client
func WithLogger(logger logging.Logger) Option {
	return func(c *OpenAIClient) {
		c.logger = logger
	}
}

// WithRetry configures retry policy for the client
func WithRetry(opts ...retry.Option) Option {
	return func(c *OpenAIClient) {
		c.retryExecutor = retry.NewExecutor(retry.NewPolicy(opts...))
	}
}

// WithBaseURL sets the base URL for the OpenAI client
func WithBaseURL(baseURL string) Option {
	return func(c *OpenAIClient) {
		c.baseURL = baseURL
		// Recreate the client and services with the new base URL
		c.Client = openai.NewClient(option.WithAPIKey(c.apiKey), option.WithBaseURL(baseURL))
		c.ChatService = openai.NewChatService(option.WithAPIKey(c.apiKey), option.WithBaseURL(baseURL))
		c.ResponseService = openai.NewClient(option.WithAPIKey(c.apiKey), option.WithBaseURL(baseURL))
	}
}

// NewClient creates a new OpenAI client
func NewClient(apiKey string, options ...Option) *OpenAIClient {
	// Create client with default options
	client := &OpenAIClient{
		Client:          openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL("https://api.openai.com/v1")),
		ChatService:     openai.NewChatService(option.WithAPIKey(apiKey), option.WithBaseURL("https://api.openai.com/v1")),
		ResponseService: openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL("https://api.openai.com/v1")),
		Model:           "gpt-4o-mini",
		apiKey:          apiKey,
		baseURL:         "https://api.openai.com/v1",
		logger:          logging.New(),
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	return client
}

// Generate generates text from a prompt
func (c *OpenAIClient) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// Apply options
	params := &interfaces.GenerateOptions{
		LLMConfig: &interfaces.LLMConfig{
			Temperature: 0.7,
		},
	}

	for _, option := range options {
		option(params)
	}

	// Get organization ID from context if available
	orgID, _ := multitenancy.GetOrgID(ctx)
	if orgID != "" {
		ctx = context.WithValue(ctx, organizationKey, orgID)
	}

	// Create request with system message if provided
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Add system message if available
	if params.SystemMessage != "" {
		// If reasoning is enabled, enhance the system message
		if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
			switch params.LLMConfig.Reasoning {
			case "minimal":
				params.SystemMessage = fmt.Sprintf("%s\n\nWhen responding, briefly explain your thought process.", params.SystemMessage)
				c.logger.Debug(ctx, "Using minimal reasoning mode", nil)
			case "comprehensive":
				params.SystemMessage = fmt.Sprintf("%s\n\nWhen responding, please think step-by-step and explain your complete reasoning process in detail.", params.SystemMessage)
				c.logger.Debug(ctx, "Using comprehensive reasoning mode", nil)
			case "none":
				params.SystemMessage = fmt.Sprintf("%s\n\nProvide direct, concise answers without explaining your reasoning or showing calculations.", params.SystemMessage)
				c.logger.Debug(ctx, "Using no reasoning mode with explicit instruction", nil)
			default:
				c.logger.Warn(ctx, "Unknown reasoning mode, using default behavior", map[string]interface{}{"reasoning": params.LLMConfig.Reasoning})
			}
		}

		messages = append(messages, openai.SystemMessage(params.SystemMessage))
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": params.SystemMessage})
	} else if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
		// If no system message but reasoning is enabled, create a system message just for reasoning
		var systemMessage string
		switch params.LLMConfig.Reasoning {
		case "minimal":
			systemMessage = "When responding, briefly explain your thought process."
			c.logger.Debug(ctx, "Using minimal reasoning mode with default system message", nil)
		case "comprehensive":
			systemMessage = "When responding, please think step-by-step and explain your complete reasoning process in detail."
			c.logger.Debug(ctx, "Using comprehensive reasoning mode with default system message", nil)
		case "none":
			systemMessage = "Provide direct, concise answers without explaining your reasoning or showing calculations."
			c.logger.Debug(ctx, "Using no reasoning mode with explicit instruction", nil)
		default:
			c.logger.Warn(ctx, "Unknown reasoning mode, using default behavior", map[string]interface{}{"reasoning": params.LLMConfig.Reasoning})
		}

		if systemMessage != "" {
			messages = append(messages, openai.SystemMessage(systemMessage))
			c.logger.Debug(ctx, "Using system message for reasoning", map[string]interface{}{"system_message": systemMessage})
		}
	}

	// Add user message
	messages = append(messages, openai.UserMessage(prompt))

	// Create request
	req := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(c.Model),
		Messages: messages,
	}

	if params.LLMConfig != nil {
		req.Temperature = openai.Float(c.getTemperatureForModel(params.LLMConfig.Temperature))
		// Reasoning models don't support top_p parameter
		if !isReasoningModel(c.Model) {
			req.TopP = openai.Float(params.LLMConfig.TopP)
		}
		req.FrequencyPenalty = openai.Float(params.LLMConfig.FrequencyPenalty)
		req.PresencePenalty = openai.Float(params.LLMConfig.PresencePenalty)
		if len(params.LLMConfig.StopSequences) > 0 {
			req.Stop = openai.ChatCompletionNewParamsStopUnion{OfStringArray: params.LLMConfig.StopSequences}
		}
	}

	// Set response format if provided
	if params.ResponseFormat != nil {
		// Convert to the new API's response format structure
		jsonSchema := shared.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:   params.ResponseFormat.Name,
			Schema: params.ResponseFormat.Schema,
		}

		req.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				Type:       "json_schema",
				JSONSchema: jsonSchema,
			},
		}
		c.logger.Debug(ctx, "Using response format", map[string]interface{}{"format": *params.ResponseFormat})
	}

	// Set organization ID if available
	if orgID, ok := ctx.Value(organizationKey).(string); ok && orgID != "" {
		req.User = openai.String(orgID)
	}

	var resp *openai.ChatCompletion
	var err error

	operation := func() error {
		var reasoningMode string
		if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
			reasoningMode = params.LLMConfig.Reasoning
		} else {
			reasoningMode = "none"
		}

		c.logger.Debug(ctx, "Executing OpenAI API request", map[string]interface{}{
			"model":             c.Model,
			"temperature":       req.Temperature,
			"top_p":             req.TopP,
			"frequency_penalty": req.FrequencyPenalty,
			"presence_penalty":  req.PresencePenalty,
			"stop_sequences":    req.Stop,
			"messages":          len(req.Messages),
			"response_format":   params.ResponseFormat != nil,
			"reasoning":         reasoningMode,
		})

		resp, err = c.ChatService.Completions.New(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "Error from OpenAI API", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			return fmt.Errorf("failed to generate text: %w", err)
		}
		return nil
	}

	if c.retryExecutor != nil {
		c.logger.Debug(ctx, "Using retry mechanism for OpenAI request", map[string]interface{}{
			"model": c.Model,
		})
		err = c.retryExecutor.Execute(ctx, operation)
	} else {
		err = operation()
	}

	if err != nil {
		return "", err
	}

	// Return response
	if len(resp.Choices) > 0 {
		c.logger.Debug(ctx, "Successfully received response from OpenAI", map[string]interface{}{
			"model": c.Model,
		})
		return resp.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no response from OpenAI API")
}

// Chat uses the ChatCompletion API to have a conversation (messages) with a model
func (c *OpenAIClient) Chat(ctx context.Context, messages []llm.Message, params *llm.GenerateParams) (string, error) {
	if params == nil {
		params = llm.DefaultGenerateParams()
	}

	// Handle reasoning if specified
	var systemMessage string
	var hasSystemMessage bool

	// Check for existing system message and apply reasoning if needed
	for i, msg := range messages {
		if msg.Role == "system" {
			hasSystemMessage = true

			// Apply reasoning to the system message if specified
			if params.Reasoning != "" {
				switch params.Reasoning {
				case "minimal":
					messages[i].Content = fmt.Sprintf("%s\n\nWhen responding, briefly explain your thought process.", msg.Content)
					c.logger.Debug(ctx, "Using minimal reasoning mode", nil)
				case "comprehensive":
					messages[i].Content = fmt.Sprintf("%s\n\nWhen responding, please think step-by-step and explain your complete reasoning process in detail.", msg.Content)
					c.logger.Debug(ctx, "Using comprehensive reasoning mode", nil)
				case "none":
					messages[i].Content = fmt.Sprintf("%s\n\nProvide direct, concise answers without explaining your reasoning or showing calculations.", msg.Content)
					c.logger.Debug(ctx, "Using no reasoning mode with explicit instruction", nil)
				default:
					c.logger.Warn(ctx, "Unknown reasoning mode, using default behavior", map[string]interface{}{
						"reasoning": params.Reasoning,
					})
				}
			}
			break
		}
	}

	// If no system message exists but reasoning is specified, create one
	if !hasSystemMessage && params.Reasoning != "" {
		switch params.Reasoning {
		case "minimal":
			systemMessage = "When responding, briefly explain your thought process."
			c.logger.Debug(ctx, "Using minimal reasoning mode with default system message", nil)
		case "comprehensive":
			systemMessage = "When responding, please think step-by-step and explain your complete reasoning process in detail."
			c.logger.Debug(ctx, "Using comprehensive reasoning mode with default system message", nil)
		case "none":
			systemMessage = "Provide direct, concise answers without explaining your reasoning or showing calculations."
			c.logger.Debug(ctx, "Using no reasoning mode with explicit instruction", nil)
		default:
			c.logger.Warn(ctx, "Unknown reasoning mode, using default behavior", map[string]interface{}{
				"reasoning": params.Reasoning,
			})
		}

		// Add system message if one was created
		if systemMessage != "" {
			messages = append([]llm.Message{{Role: "system", Content: systemMessage}}, messages...)
		}
	}

	// Convert messages to the OpenAI Chat format
	chatMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "system":
			chatMessages[i] = openai.SystemMessage(msg.Content)
		case "user":
			chatMessages[i] = openai.UserMessage(msg.Content)
		case "assistant":
			chatMessages[i] = openai.AssistantMessage(msg.Content)
		case "tool":
			// For tool messages, we need to handle tool call ID
			// Use the ToolCallID from the Message struct
			chatMessages[i] = openai.ToolMessage(msg.Content, msg.ToolCallID)
		default:
			// Default to user message for unknown roles
			chatMessages[i] = openai.UserMessage(msg.Content)
		}
	}

	// Create chat request
	req := openai.ChatCompletionNewParams{
		Model:            openai.ChatModel(c.Model),
		Messages:         chatMessages,
		Temperature:      openai.Float(c.getTemperatureForModel(params.Temperature)),
		FrequencyPenalty: openai.Float(params.FrequencyPenalty),
		PresencePenalty:  openai.Float(params.PresencePenalty),
	}

	// Reasoning models don't support top_p parameter
	if !isReasoningModel(c.Model) {
		req.TopP = openai.Float(params.TopP)
	}

	if len(params.StopSequences) > 0 {
		req.Stop = openai.ChatCompletionNewParamsStopUnion{OfStringArray: params.StopSequences}
	}

	var resp *openai.ChatCompletion
	var err error

	operation := func() error {
		c.logger.Debug(ctx, "Executing OpenAI Chat API request", map[string]interface{}{
			"model":             c.Model,
			"temperature":       req.Temperature,
			"top_p":             req.TopP,
			"frequency_penalty": req.FrequencyPenalty,
			"presence_penalty":  req.PresencePenalty,
			"stop_sequences":    req.Stop,
			"messages":          len(req.Messages),
			"reasoning":         params.Reasoning,
		})

		resp, err = c.ChatService.Completions.New(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "Error from OpenAI Chat API", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			return fmt.Errorf("failed to create chat completion: %w", err)
		}
		return nil
	}

	if c.retryExecutor != nil {
		c.logger.Debug(ctx, "Using retry mechanism for OpenAI Chat request", map[string]interface{}{
			"model": c.Model,
		})
		err = c.retryExecutor.Execute(ctx, operation)
	} else {
		err = operation()
	}

	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no completions returned")
	}

	c.logger.Debug(ctx, "Successfully received chat response from OpenAI", map[string]interface{}{
		"model": c.Model,
	})

	return resp.Choices[0].Message.Content, nil
}

// GenerateWithTools implements interfaces.LLM.GenerateWithTools
func (c *OpenAIClient) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
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

	// Check for organization ID in context
	orgID := "default"
	if id, err := multitenancy.GetOrgID(ctx); err == nil {
		orgID = id
	}
	ctx = context.WithValue(ctx, organizationKey, orgID)

	// Convert tools to OpenAI format
	openaiTools := make([]openai.ChatCompletionToolUnionParam, len(tools))
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

		openaiTools[i] = openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        tool.Name(),
			Description: openai.String(tool.Description()),
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		})
	}

	// Create messages array with system message if provided
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Track tool call repetitions for loop detection
	toolCallHistory := make(map[string]int)
	var toolCallHistoryMu sync.Mutex

	// Add system message if available
	if params.SystemMessage != "" {
		// If reasoning is enabled, enhance the system message
		if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
			switch params.LLMConfig.Reasoning {
			case "minimal":
				params.SystemMessage = fmt.Sprintf("%s\n\nWhen responding, briefly explain your thought process.", params.SystemMessage)
				c.logger.Debug(ctx, "Using minimal reasoning mode", nil)
			case "comprehensive":
				params.SystemMessage = fmt.Sprintf("%s\n\nWhen responding, please think step-by-step and explain your complete reasoning process in detail.", params.SystemMessage)
				c.logger.Debug(ctx, "Using comprehensive reasoning mode", nil)
			case "none":
				params.SystemMessage = fmt.Sprintf("%s\n\nProvide direct, concise answers without explaining your reasoning or showing calculations.", params.SystemMessage)
				c.logger.Debug(ctx, "Using no reasoning mode with explicit instruction", nil)
			default:
				c.logger.Warn(ctx, "Unknown reasoning mode, using default behavior", map[string]interface{}{"reasoning": params.LLMConfig.Reasoning})
			}
		}

		messages = append(messages, openai.SystemMessage(params.SystemMessage))
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": params.SystemMessage})
	} else if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
		// If no system message but reasoning is enabled, create a system message just for reasoning
		var systemMessage string
		switch params.LLMConfig.Reasoning {
		case "minimal":
			systemMessage = "When responding, briefly explain your thought process."
			c.logger.Debug(ctx, "Using minimal reasoning mode with default system message", nil)
		case "comprehensive":
			systemMessage = "When responding, please think step-by-step and explain your complete reasoning process in detail."
			c.logger.Debug(ctx, "Using comprehensive reasoning mode with default system message", nil)
		case "none":
			// No system message needed
			c.logger.Debug(ctx, "Using no reasoning mode", nil)
		default:
			c.logger.Warn(ctx, "Unknown reasoning mode, using default behavior", map[string]interface{}{"reasoning": params.LLMConfig.Reasoning})
		}

		if systemMessage != "" {
			messages = append(messages, openai.SystemMessage(systemMessage))
			c.logger.Debug(ctx, "Using system message for reasoning", map[string]interface{}{"system_message": systemMessage})
		}
	}

	// Add user message
	messages = append(messages, openai.UserMessage(prompt))

	req := openai.ChatCompletionNewParams{
		Model:            openai.ChatModel(c.Model),
		Messages:         messages,
		Tools:            openaiTools,
		Temperature:      openai.Float(c.getTemperatureForModel(params.LLMConfig.Temperature)),
		FrequencyPenalty: openai.Float(params.LLMConfig.FrequencyPenalty),
		PresencePenalty:  openai.Float(params.LLMConfig.PresencePenalty),
	}

	// Reasoning models don't support top_p parameter
	if !isReasoningModel(c.Model) {
		req.TopP = openai.Float(params.LLMConfig.TopP)
	}

	// Only set ParallelToolCalls for non-reasoning models
	if !isReasoningModel(c.Model) {
		req.ParallelToolCalls = openai.Bool(true)
	}

	if len(params.LLMConfig.StopSequences) > 0 {
		req.Stop = openai.ChatCompletionNewParamsStopUnion{OfStringArray: params.LLMConfig.StopSequences}
	}

	// Set response format if provided
	if params.ResponseFormat != nil {
		// Convert to the new API's response format structure
		jsonSchema := shared.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:   params.ResponseFormat.Name,
			Schema: params.ResponseFormat.Schema,
		}

		req.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				Type:       "json_schema",
				JSONSchema: jsonSchema,
			},
		}
		c.logger.Debug(ctx, "Using response format", map[string]interface{}{"format": *params.ResponseFormat})
	}

	// Iterative tool calling loop
	for iteration := 0; iteration < maxIterations; iteration++ {
		// Update request with current messages
		req.Messages = messages

		// Send request
		var reasoningMode string
		if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
			reasoningMode = params.LLMConfig.Reasoning
		} else {
			reasoningMode = "none"
		}

		c.logger.Debug(ctx, "Sending request with tools to OpenAI", map[string]interface{}{
			"model":             c.Model,
			"temperature":       req.Temperature,
			"top_p":             req.TopP,
			"frequency_penalty": req.FrequencyPenalty,
			"presence_penalty":  req.PresencePenalty,
			"stop_sequences":    req.Stop,
			"messages":          len(req.Messages),
			"tools":             len(req.Tools),
			"response_format":   params.ResponseFormat != nil,
			"parallel_tools":    req.ParallelToolCalls,
			"reasoning":         reasoningMode,
			"iteration":         iteration + 1,
			"maxIterations":     maxIterations,
		})
		resp, err := c.ChatService.Completions.New(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "Error from OpenAI API", map[string]interface{}{"error": err.Error()})
			return "", fmt.Errorf("failed to create chat completion: %w", err)
		}

		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("no completions returned")
		}

		// Check if the model wants to use tools
		if len(resp.Choices[0].Message.ToolCalls) == 0 {
			// No tool calls, return the response
			content := strings.TrimSpace(resp.Choices[0].Message.Content)
			return content, nil
		}

		// The model wants to use tools
		toolCalls := resp.Choices[0].Message.ToolCalls
		c.logger.Info(ctx, "Processing tool calls", map[string]interface{}{
			"count":     len(toolCalls),
			"iteration": iteration + 1,
		})

		// Add the assistant's message with tool calls to the conversation
		messages = append(messages, resp.Choices[0].Message.ToParam())

		// Process each tool call
		for _, toolCall := range toolCalls {
			// Replace multi_tool_use.parallel name if present
			if toolCall.Function.Name == "multi_tool_use.parallel" {
				c.logger.Info(ctx, "Replacing multi_tool_use.parallel with parallel_tool_use", nil)
				toolCall.Function.Name = "parallel_tool_use"
			}

			if toolCall.Function.Name == "parallel_tool_use" {
				c.logger.Info(ctx, "Parallel tool call", map[string]interface{}{"toolName": toolCall.Function.Name})

				arguments := toolCall.Function.Arguments
				var toolUsesWrapper struct {
					ToolUses []map[string]interface{} `json:"tool_uses"`
				}
				err := json.Unmarshal([]byte(arguments), &toolUsesWrapper)
				if err != nil {
					c.logger.Error(ctx, "Error unmarshalling tool uses", map[string]interface{}{"error": err.Error()})
					continue
				}

				type toolResult struct {
					index  int
					result string
					err    error
				}

				resultCh := make(chan toolResult, len(toolUsesWrapper.ToolUses))
				var wg sync.WaitGroup

				// Launch goroutines for concurrent tool execution
				for i, toolUse := range toolUsesWrapper.ToolUses {
					wg.Add(1)
					go func(index int, toolUse map[string]interface{}) {
						defer wg.Done()

						toolName := toolUse["recipient_name"].(string)
						parameters := toolUse["parameters"].(map[string]interface{})

						c.logger.Info(ctx, "Parallel tool use", map[string]interface{}{"toolName": toolName, "parameters": parameters})

						// Convert parameters to JSON string
						paramsBytes, err := json.Marshal(parameters)
						if err != nil {
							c.logger.Error(ctx, "Error marshalling parameters", map[string]interface{}{"error": err.Error()})
							resultCh <- toolResult{index: index, result: "", err: err}
							return
						}

						// Find the correct tool for this operation
						var tool interfaces.Tool
						for _, t := range tools {
							if t.Name() == toolName {
								tool = t
								break
							}
						}

						if tool == nil {
							err := fmt.Errorf("tool not found: %s", toolName)
							c.logger.Error(ctx, "Tool not found in parallel execution", map[string]interface{}{"toolName": toolName})
							resultCh <- toolResult{index: index, result: "", err: err}
							return
						}

						c.logger.Info(ctx, "Executing tool", map[string]interface{}{"toolName": toolName, "parameters": string(paramsBytes)})

						result, err := tool.Execute(ctx, string(paramsBytes))

						// Check for repetitive calls and add warning if needed
						cacheKey := toolName + ":" + string(paramsBytes)

						toolCallHistoryMu.Lock()
						toolCallHistory[cacheKey]++
						callCount := toolCallHistory[cacheKey]
						toolCallHistoryMu.Unlock()

						if callCount > 2 {
							warning := fmt.Sprintf("\n\n[WARNING: This is call #%d to %s with identical parameters. You may be in a loop. Consider using the available information to provide a final answer.]",
								callCount,
								toolName)
							if err == nil {
								result += warning
							}
							c.logger.Warn(ctx, "Repetitive tool call detected in parallel execution", map[string]interface{}{
								"toolName":  toolName,
								"callCount": callCount,
							})
						}

						// Store tool call and result in memory if provided
						if params.Memory != nil {
							if err != nil {
								// Store failed parallel tool call result
								_ = params.Memory.AddMessage(ctx, interfaces.Message{
									Role:    "assistant",
									Content: "",
									ToolCalls: []interfaces.ToolCall{{
										ID:        toolCall.ID,
										Name:      toolName,
										Arguments: string(paramsBytes),
									}},
								})
								_ = params.Memory.AddMessage(ctx, interfaces.Message{
									Role:       "tool",
									Content:    fmt.Sprintf("Error: %v", err),
									ToolCallID: toolCall.ID,
									Metadata: map[string]interface{}{
										"tool_name": toolCall.Function.Name,
									},
								})
							} else {
								// Store successful parallel tool call and result
								_ = params.Memory.AddMessage(ctx, interfaces.Message{
									Role:    "assistant",
									Content: "",
									ToolCalls: []interfaces.ToolCall{{
										ID:        toolCall.ID,
										Name:      toolName,
										Arguments: string(paramsBytes),
									}},
								})
								_ = params.Memory.AddMessage(ctx, interfaces.Message{
									Role:       "tool",
									Content:    result,
									ToolCallID: toolCall.ID,
									Metadata: map[string]interface{}{
										"tool_name": toolCall.Function.Name,
									},
								})
							}
						}

						resultCh <- toolResult{index: index, result: result, err: err}
					}(i, toolUse)
				}

				// Close channel when all goroutines complete
				go func() {
					wg.Wait()
					close(resultCh)
				}()

				// Collect results and check for errors
				toolsResults := make([]string, len(toolUsesWrapper.ToolUses))
				for result := range resultCh {
					if result.err != nil {
						c.logger.Error(ctx, "Error executing tool", map[string]interface{}{"error": result.err.Error()})
						return "", fmt.Errorf("error executing tool: %s", result.err.Error())
					}
					toolsResults[result.index] = result.result
				}

				// For parallel tool use, we need to create a tool message
				// The new API uses openai.ToolMessage(content, toolCallID) instead of struct literals
				// Create a structured response that clearly identifies which tool each result came from
				var structuredResults []string
				for i, toolUse := range toolUsesWrapper.ToolUses {
					toolName := toolUse["recipient_name"].(string)
					result := toolsResults[i]
					structuredResults = append(structuredResults, fmt.Sprintf("Tool: %s\nResult: %s", toolName, result))
				}
				messages = append(messages, openai.ToolMessage(strings.Join(structuredResults, "\n\n"), toolCall.ID))
				continue
			}

			// Find the requested tool
			var selectedTool interfaces.Tool
			for _, tool := range tools {
				if tool.Name() == toolCall.Function.Name {
					selectedTool = tool
					break
				}
			}

			if selectedTool == nil || selectedTool.Name() == "" {
				c.logger.Error(ctx, "Tool not found", map[string]interface{}{
					"toolName": toolCall.Function.Name,
					"toolcall": toolCall,
					"resp":     resp,
				})
				
				// Add tool not found error as tool result instead of returning
				errorMessage := fmt.Sprintf("Error: tool not found: %s", toolCall.Function.Name)
				
				// Store failed tool call in memory if provided
				if params.Memory != nil {
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []interfaces.ToolCall{{
							ID:        toolCall.ID,
							Name:      toolCall.Function.Name,
							Arguments: toolCall.Function.Arguments,
						}},
					})
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:       "tool",
						Content:    errorMessage,
						ToolCallID: toolCall.ID,
						Metadata: map[string]interface{}{
							"tool_name": toolCall.Function.Name,
						},
					})
				}
				
				// Add to tracing context
				toolCallTrace := tracing.ToolCall{
					Name:       toolCall.Function.Name,
					Arguments:  toolCall.Function.Arguments,
					ID:         toolCall.ID,
					Timestamp:  time.Now().Format(time.RFC3339),
					StartTime:  time.Now(),
					Duration:   0,
					DurationMs: 0,
					Error:      fmt.Sprintf("tool not found: %s", toolCall.Function.Name),
					Result:     errorMessage,
				}
				
				tracing.AddToolCallToContext(ctx, toolCallTrace)
				
				// Add error message as tool response
				messages = append(messages, openai.ToolMessage(errorMessage, toolCall.ID))
				
				continue // Continue processing other tool calls
			}

			// Execute the tool
			c.logger.Info(ctx, "Executing tool", map[string]interface{}{"toolName": selectedTool.Name()})
			toolStartTime := time.Now()
			toolResult, err := selectedTool.Execute(ctx, toolCall.Function.Arguments)
			toolEndTime := time.Now()

			// Check for repetitive calls and add warning if needed
			cacheKey := toolCall.Function.Name + ":" + toolCall.Function.Arguments

			toolCallHistoryMu.Lock()
			toolCallHistory[cacheKey]++
			callCount := toolCallHistory[cacheKey]
			toolCallHistoryMu.Unlock()

			if callCount > 1 {
				warning := fmt.Sprintf("\n\n[WARNING: This is call #%d to %s with identical parameters. You may be in a loop. Consider using the available information to provide a final answer.]",
					callCount,
					toolCall.Function.Name)
				if err == nil {
					toolResult += warning
				}
				c.logger.Warn(ctx, "Repetitive tool call detected", map[string]interface{}{
					"toolName":  toolCall.Function.Name,
					"callCount": callCount,
				})
			}

			// Add tool call to tracing context
			executionDuration := toolEndTime.Sub(toolStartTime)
			toolCallTrace := tracing.ToolCall{
				Name:       toolCall.Function.Name,
				Arguments:  toolCall.Function.Arguments,
				ID:         toolCall.ID,
				Timestamp:  toolStartTime.Format(time.RFC3339),
				StartTime:  toolStartTime,
				Duration:   executionDuration,
				DurationMs: executionDuration.Milliseconds(),
			}

			// Store tool call and result in memory if provided
			if params.Memory != nil {
				if err != nil {
					// Store failed tool call result
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []interfaces.ToolCall{{
							ID:        toolCall.ID,
							Name:      toolCall.Function.Name,
							Arguments: toolCall.Function.Arguments,
						}},
					})
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:       "tool",
						Content:    fmt.Sprintf("Error: %v", err),
						ToolCallID: toolCall.ID,
						Metadata: map[string]interface{}{
							"tool_name": toolCall.Function.Name,
						},
					})
				} else {
					// Store successful tool call and result
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []interfaces.ToolCall{{
							ID:        toolCall.ID,
							Name:      toolCall.Function.Name,
							Arguments: toolCall.Function.Arguments,
						}},
					})
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:       "tool",
						Content:    toolResult,
						ToolCallID: toolCall.ID,
						Metadata: map[string]interface{}{
							"tool_name": toolCall.Function.Name,
						},
					})
				}
			}

			if err != nil {
				c.logger.Error(ctx, "Error executing tool", map[string]interface{}{"toolName": selectedTool.Name(), "error": err.Error()})
				toolCallTrace.Error = err.Error()
				toolCallTrace.Result = fmt.Sprintf("Error: %v", err)
				// Add error message as tool response
				messages = append(messages, openai.ToolMessage(fmt.Sprintf("Error: %v", err), toolCall.ID))
			} else {
				toolCallTrace.Result = toolResult
				// Add tool result to messages
				messages = append(messages, openai.ToolMessage(toolResult, toolCall.ID))
			}

			// Add the tool call to the tracing context
			tracing.AddToolCallToContext(ctx, toolCallTrace)
		}

		// Continue to the next iteration with updated messages
	}

	// If we've reached the maximum iterations and the model is still requesting tools,
	// make one final call without tools to get a conclusion
	c.logger.Info(ctx, "Maximum iterations reached, making final call without tools", map[string]interface{}{
		"maxIterations": maxIterations,
	})

	// Create a final request without tools to force the LLM to provide a conclusion
	finalReq := openai.ChatCompletionNewParams{
		Model:            openai.ChatModel(c.Model),
		Messages:         messages,
		Tools:            nil, // No tools for final call
		Temperature:      openai.Float(c.getTemperatureForModel(params.LLMConfig.Temperature)),
		FrequencyPenalty: openai.Float(params.LLMConfig.FrequencyPenalty),
		PresencePenalty:  openai.Float(params.LLMConfig.PresencePenalty),
	}

	// Reasoning models don't support top_p parameter
	if !isReasoningModel(c.Model) {
		finalReq.TopP = openai.Float(params.LLMConfig.TopP)
	}

	if len(params.LLMConfig.StopSequences) > 0 {
		finalReq.Stop = openai.ChatCompletionNewParamsStopUnion{OfStringArray: params.LLMConfig.StopSequences}
	}

	// Set response format if provided
	if params.ResponseFormat != nil {
		jsonSchema := shared.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:   params.ResponseFormat.Name,
			Schema: params.ResponseFormat.Schema,
		}

		finalReq.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				Type:       "json_schema",
				JSONSchema: jsonSchema,
			},
		}
	}

	// Add a system message to encourage conclusion
	conclusionMessage := openai.SystemMessage("Please provide your final response based on the information available. Do not request any additional tools.")
	finalReq.Messages = append(finalReq.Messages, conclusionMessage)

	c.logger.Debug(ctx, "Making final request without tools", map[string]interface{}{
		"messages": len(finalReq.Messages),
	})

	finalResp, err := c.ChatService.Completions.New(ctx, finalReq)
	if err != nil {
		c.logger.Error(ctx, "Error in final call without tools", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("failed to create final chat completion: %w", err)
	}

	if len(finalResp.Choices) == 0 {
		return "", fmt.Errorf("no completions returned in final call")
	}

	content := strings.TrimSpace(finalResp.Choices[0].Message.Content)
	c.logger.Info(ctx, "Successfully received final response without tools", nil)
	return content, nil
}

// Name implements interfaces.LLM.Name
func (c *OpenAIClient) Name() string {
	return "openai"
}

// SupportsStreaming implements interfaces.LLM.SupportsStreaming
func (c *OpenAIClient) SupportsStreaming() bool {
	return true
}

// GetModel returns the model name being used
func (c *OpenAIClient) GetModel() string {
	return c.Model
}

// WithTemperature creates a GenerateOption to set the temperature
func WithTemperature(temperature float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.LLMConfig.Temperature = temperature
	}
}

// WithTopP creates a GenerateOption to set the top_p
func WithTopP(topP float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.LLMConfig.TopP = topP
	}
}

// WithFrequencyPenalty creates a GenerateOption to set the frequency penalty
func WithFrequencyPenalty(frequencyPenalty float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.LLMConfig.FrequencyPenalty = frequencyPenalty
	}
}

// WithPresencePenalty creates a GenerateOption to set the presence penalty
func WithPresencePenalty(presencePenalty float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.LLMConfig.PresencePenalty = presencePenalty
	}
}

// WithStopSequences creates a GenerateOption to set the stop sequences
func WithStopSequences(stopSequences []string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.LLMConfig.StopSequences = stopSequences
	}
}

// WithSystemMessage creates a GenerateOption to set the system message
func WithSystemMessage(systemMessage string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.SystemMessage = systemMessage
	}
}

// WithResponseFormat creates a GenerateOption to set the response format
func WithResponseFormat(format interfaces.ResponseFormat) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.ResponseFormat = &format
	}
}

// WithReasoning creates a GenerateOption to set the reasoning mode
// reasoning can be "none" (direct answers), "minimal" (brief explanations),
// or "comprehensive" (detailed step-by-step reasoning)
func WithReasoning(reasoning string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.Reasoning = reasoning
	}
}
