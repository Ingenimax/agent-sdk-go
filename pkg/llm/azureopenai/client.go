package azureopenai

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

// AzureOpenAIClient implements the LLM interface for Azure OpenAI
type AzureOpenAIClient struct {
	Client          openai.Client
	ChatService     openai.ChatService
	ResponseService openai.Client
	Model           string
	apiKey          string
	baseURL         string
	apiVersion      string
	deployment      string
	region          string
	resourceName    string
	logger          logging.Logger
	retryExecutor   *retry.Executor
}

// Option represents an option for configuring the Azure OpenAI client
type Option func(*AzureOpenAIClient)

// WithModel sets the model for the Azure OpenAI client
func WithModel(model string) Option {
	return func(c *AzureOpenAIClient) {
		c.Model = model
	}
}

// WithDeployment sets the deployment name for the Azure OpenAI client
func WithDeployment(deployment string) Option {
	return func(c *AzureOpenAIClient) {
		c.deployment = deployment
	}
}

// WithAPIVersion sets the API version for the Azure OpenAI client
func WithAPIVersion(apiVersion string) Option {
	return func(c *AzureOpenAIClient) {
		c.apiVersion = apiVersion
	}
}

// WithRegion sets the Azure region for the Azure OpenAI client
func WithRegion(region string) Option {
	return func(c *AzureOpenAIClient) {
		c.region = region
	}
}

// WithResourceName sets the Azure resource name for the Azure OpenAI client
func WithResourceName(resourceName string) Option {
	return func(c *AzureOpenAIClient) {
		c.resourceName = resourceName
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
func (c *AzureOpenAIClient) getTemperatureForModel(requestedTemp float64) float64 {
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

// WithLogger sets the logger for the Azure OpenAI client
func WithLogger(logger logging.Logger) Option {
	return func(c *AzureOpenAIClient) {
		c.logger = logger
	}
}

// WithRetry configures retry policy for the client
func WithRetry(opts ...retry.Option) Option {
	return func(c *AzureOpenAIClient) {
		c.retryExecutor = retry.NewExecutor(retry.NewPolicy(opts...))
	}
}

// WithBaseURL sets the base URL for the Azure OpenAI client
func WithBaseURL(baseURL string) Option {
	return func(c *AzureOpenAIClient) {
		c.baseURL = baseURL
		// Recreate the client and services with the new base URL
		c.recreateClients()
	}
}

// recreateClients recreates the OpenAI clients with current configuration
func (c *AzureOpenAIClient) recreateClients() {
	// Build the Azure OpenAI endpoint URL
	// If baseURL is provided, use it directly, otherwise construct from region and resource name
	var azureURL string
	if c.baseURL != "" {
		// Use provided baseURL (e.g., https://your-resource.openai.azure.com)
		azureURL = fmt.Sprintf("%s/openai/deployments/%s", strings.TrimSuffix(c.baseURL, "/"), c.deployment)
	} else if c.region != "" && c.resourceName != "" {
		// Construct URL from region and resource name
		azureURL = fmt.Sprintf("https://%s.openai.azure.com/openai/deployments/%s", c.resourceName, c.deployment)
		c.baseURL = fmt.Sprintf("https://%s.openai.azure.com", c.resourceName)
	} else {
		c.logger.Error(context.Background(), "Either baseURL or both region and resourceName must be provided", nil)
		return
	}

	options := []option.RequestOption{
		option.WithAPIKey(c.apiKey),
		option.WithBaseURL(azureURL),
	}

	// Add API version as query parameter for Azure OpenAI
	if c.apiVersion != "" {
		options = append(options, option.WithQuery("api-version", c.apiVersion))
	}

	c.Client = openai.NewClient(options...)
	c.ChatService = openai.NewChatService(options...)
	c.ResponseService = openai.NewClient(options...)
}

// NewClient creates a new Azure OpenAI client
// You can provide either:
// 1. baseURL (e.g., "https://your-resource.openai.azure.com") - traditional approach
// 2. region and resourceName via options - newer approach
func NewClient(apiKey, baseURL, deployment string, options ...Option) *AzureOpenAIClient {
	// Create client with default options
	client := &AzureOpenAIClient{
		Model:      deployment, // In Azure OpenAI, deployment name is the model identifier
		apiKey:     apiKey,
		baseURL:    baseURL,
		deployment: deployment,
		apiVersion: "2024-08-01-preview", // Default API version (required for structured output)
		logger:     logging.New(),
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	// Create the OpenAI clients with Azure configuration
	client.recreateClients()

	return client
}

// NewClientFromRegion creates a new Azure OpenAI client using region and resource name
// This is the recommended approach for new implementations
func NewClientFromRegion(apiKey, region, resourceName, deployment string, options ...Option) *AzureOpenAIClient {
	// Create client with region-based configuration
	client := &AzureOpenAIClient{
		Model:        deployment, // In Azure OpenAI, deployment name is the model identifier
		apiKey:       apiKey,
		region:       region,
		resourceName: resourceName,
		deployment:   deployment,
		apiVersion:   "2024-08-01-preview", // Default API version (required for structured output)
		logger:       logging.New(),
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	// Create the OpenAI clients with Azure configuration
	client.recreateClients()

	return client
}

// Generate generates text from a prompt
func (c *AzureOpenAIClient) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	response, err := c.generateInternal(ctx, prompt, options...)
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// generateInternal performs the actual generation and returns the full response
func (c *AzureOpenAIClient) generateInternal(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
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

	// Build messages with memory and current prompt
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Add system message if available
	if params.SystemMessage != "" {
		messages = append(messages, openai.SystemMessage(params.SystemMessage))
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": params.SystemMessage})
	}

	// Add memory messages and current prompt
	builder := newMessageHistoryBuilder(c.logger)
	messages = append(messages, builder.buildMessages(ctx, prompt, params.Memory)...)

	// Create request - use deployment name as model for Azure OpenAI
	req := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(c.deployment),
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
		// Set reasoning effort for reasoning models
		if isReasoningModel(c.Model) && params.LLMConfig.Reasoning != "" {
			req.ReasoningEffort = shared.ReasoningEffort(params.LLMConfig.Reasoning)
			c.logger.Debug(ctx, "Setting reasoning effort", map[string]interface{}{"reasoning_effort": params.LLMConfig.Reasoning})
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
		var reasoningEffort string
		if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
			reasoningEffort = params.LLMConfig.Reasoning
		} else {
			reasoningEffort = "none"
		}

		c.logger.Debug(ctx, "Executing Azure OpenAI API request", map[string]interface{}{
			"model":             c.Model,
			"deployment":        c.deployment,
			"temperature":       req.Temperature,
			"top_p":             req.TopP,
			"frequency_penalty": req.FrequencyPenalty,
			"presence_penalty":  req.PresencePenalty,
			"stop_sequences":    req.Stop,
			"messages":          len(req.Messages),
			"response_format":   params.ResponseFormat != nil,
			"reasoning_effort":  reasoningEffort,
		})

		resp, err = c.ChatService.Completions.New(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "Error from Azure OpenAI API", map[string]interface{}{
				"error":      err.Error(),
				"model":      c.Model,
				"deployment": c.deployment,
			})
			return fmt.Errorf("failed to generate text: %w", err)
		}
		return nil
	}

	if c.retryExecutor != nil {
		c.logger.Debug(ctx, "Using retry mechanism for Azure OpenAI request", map[string]interface{}{
			"model":      c.Model,
			"deployment": c.deployment,
		})
		err = c.retryExecutor.Execute(ctx, operation)
	} else {
		err = operation()
	}

	if err != nil {
		return nil, err
	}

	// Return response
	if len(resp.Choices) > 0 {
		c.logger.Debug(ctx, "Successfully received response from Azure OpenAI", map[string]interface{}{
			"model":      c.Model,
			"deployment": c.deployment,
		})

		// Create detailed response with token usage
		response := &interfaces.LLMResponse{
			Content:    resp.Choices[0].Message.Content,
			Model:      string(resp.Model),
			StopReason: string(resp.Choices[0].FinishReason),
			Metadata: map[string]interface{}{
				"provider":   "azure_openai",
				"deployment": c.deployment,
			},
		}

		// Extract token usage if available
		usage := &interfaces.TokenUsage{
			InputTokens:  int(resp.Usage.PromptTokens),
			OutputTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:  int(resp.Usage.TotalTokens),
		}

		// Add reasoning tokens if available (for o1 models)
		if resp.Usage.CompletionTokensDetails.ReasoningTokens > 0 {
			usage.ReasoningTokens = int(resp.Usage.CompletionTokensDetails.ReasoningTokens)
		}

		response.Usage = usage

		return response, nil
	}

	return nil, fmt.Errorf("no response from Azure OpenAI API")
}

// Chat uses the ChatCompletion API to have a conversation (messages) with a model
func (c *AzureOpenAIClient) Chat(ctx context.Context, messages []llm.Message, params *llm.GenerateParams) (string, error) {
	if params == nil {
		params = llm.DefaultGenerateParams()
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

	// Create chat request - use deployment name as model for Azure OpenAI
	req := openai.ChatCompletionNewParams{
		Model:            openai.ChatModel(c.deployment),
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

	// Set reasoning effort for reasoning models
	if isReasoningModel(c.Model) && params.Reasoning != "" {
		req.ReasoningEffort = shared.ReasoningEffort(params.Reasoning)
		c.logger.Debug(ctx, "Setting reasoning effort", map[string]interface{}{"reasoning_effort": params.Reasoning})
	}

	var resp *openai.ChatCompletion
	var err error

	operation := func() error {
		c.logger.Debug(ctx, "Executing Azure OpenAI Chat API request", map[string]interface{}{
			"model":             c.Model,
			"deployment":        c.deployment,
			"temperature":       req.Temperature,
			"top_p":             req.TopP,
			"frequency_penalty": req.FrequencyPenalty,
			"presence_penalty":  req.PresencePenalty,
			"stop_sequences":    req.Stop,
			"messages":          len(req.Messages),
			"reasoning_effort":  params.Reasoning,
		})

		resp, err = c.ChatService.Completions.New(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "Error from Azure OpenAI Chat API", map[string]interface{}{
				"error":      err.Error(),
				"model":      c.Model,
				"deployment": c.deployment,
			})
			return fmt.Errorf("failed to create chat completion: %w", err)
		}
		return nil
	}

	if c.retryExecutor != nil {
		c.logger.Debug(ctx, "Using retry mechanism for Azure OpenAI Chat request", map[string]interface{}{
			"model":      c.Model,
			"deployment": c.deployment,
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

	c.logger.Debug(ctx, "Successfully received chat response from Azure OpenAI", map[string]interface{}{
		"model":      c.Model,
		"deployment": c.deployment,
	})

	return resp.Choices[0].Message.Content, nil
}

// GenerateWithTools implements interfaces.LLM.GenerateWithTools
func (c *AzureOpenAIClient) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
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

	// Build messages with memory and current prompt
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Track tool call repetitions for loop detection
	toolCallHistory := make(map[string]int)
	var toolCallHistoryMu sync.Mutex

	// Add system message if available
	if params.SystemMessage != "" {
		messages = append(messages, openai.SystemMessage(params.SystemMessage))
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": params.SystemMessage})
	}

	// Add memory messages and current prompt
	builder := newMessageHistoryBuilder(c.logger)
	messages = append(messages, builder.buildMessages(ctx, prompt, params.Memory)...)

	// Create request - use deployment name as model for Azure OpenAI
	req := openai.ChatCompletionNewParams{
		Model:            openai.ChatModel(c.deployment),
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

	// Set reasoning effort for reasoning models
	if isReasoningModel(c.Model) && params.LLMConfig.Reasoning != "" {
		req.ReasoningEffort = shared.ReasoningEffort(params.LLMConfig.Reasoning)
		c.logger.Debug(ctx, "Setting reasoning effort", map[string]interface{}{"reasoning_effort": params.LLMConfig.Reasoning})
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
		var reasoningEffort string
		if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
			reasoningEffort = params.LLMConfig.Reasoning
		} else {
			reasoningEffort = "none"
		}

		c.logger.Debug(ctx, "Sending request with tools to Azure OpenAI", map[string]interface{}{
			"model":             c.Model,
			"deployment":        c.deployment,
			"temperature":       req.Temperature,
			"top_p":             req.TopP,
			"frequency_penalty": req.FrequencyPenalty,
			"presence_penalty":  req.PresencePenalty,
			"stop_sequences":    req.Stop,
			"messages":          len(req.Messages),
			"tools":             len(req.Tools),
			"response_format":   params.ResponseFormat != nil,
			"parallel_tools":    req.ParallelToolCalls,
			"reasoning_effort":  reasoningEffort,
			"iteration":         iteration + 1,
			"maxIterations":     maxIterations,
		})
		resp, err := c.ChatService.Completions.New(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "Error from Azure OpenAI API", map[string]interface{}{
				"error":      err.Error(),
				"deployment": c.deployment,
			})
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
		Model:            openai.ChatModel(c.deployment),
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
func (c *AzureOpenAIClient) Name() string {
	return "azure-openai"
}

// SupportsStreaming implements interfaces.LLM.SupportsStreaming
func (c *AzureOpenAIClient) SupportsStreaming() bool {
	return true
}

// GetModel returns the model name being used
func (c *AzureOpenAIClient) GetModel() string {
	return c.Model
}

// GetDeployment returns the deployment name being used
func (c *AzureOpenAIClient) GetDeployment() string {
	return c.deployment
}

// GetRegion returns the Azure region being used
func (c *AzureOpenAIClient) GetRegion() string {
	return c.region
}

// GetResourceName returns the Azure resource name being used
func (c *AzureOpenAIClient) GetResourceName() string {
	return c.resourceName
}

// GetBaseURL returns the base URL being used
func (c *AzureOpenAIClient) GetBaseURL() string {
	return c.baseURL
}

// WithTemperature creates a GenerateOption to set the temperature
func WithTemperature(temperature float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.Temperature = temperature
	}
}

// WithTopP creates a GenerateOption to set the top_p
func WithTopP(topP float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.TopP = topP
	}
}

// WithFrequencyPenalty creates a GenerateOption to set the frequency penalty
func WithFrequencyPenalty(frequencyPenalty float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.FrequencyPenalty = frequencyPenalty
	}
}

// WithPresencePenalty creates a GenerateOption to set the presence penalty
func WithPresencePenalty(presencePenalty float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.PresencePenalty = presencePenalty
	}
}

// WithStopSequences creates a GenerateOption to set the stop sequences
func WithStopSequences(stopSequences []string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
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

// WithReasoning creates a GenerateOption to set the reasoning effort for reasoning models
// For OpenAI reasoning models (o1, o3, o4, gpt-5 series), valid values are:
// "minimal", "low", "medium", "high"
// This parameter is only used with reasoning models and is ignored for standard models.
func WithReasoning(reasoning string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		if options.LLMConfig == nil {
			options.LLMConfig = &interfaces.LLMConfig{}
		}
		options.LLMConfig.Reasoning = reasoning
	}
}

// GenerateDetailed generates text and returns detailed response information including token usage
func (c *AzureOpenAIClient) GenerateDetailed(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return c.generateInternal(ctx, prompt, options...)
}

// GenerateWithToolsDetailed generates text with tools and returns detailed response information including token usage
func (c *AzureOpenAIClient) GenerateWithToolsDetailed(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	// For now, call the existing method and construct a detailed response
	// TODO: Implement full detailed version that tracks token usage across all tool iterations
	content, err := c.GenerateWithTools(ctx, prompt, tools, options...)
	if err != nil {
		return nil, err
	}

	// Return a basic detailed response without usage information for now
	// This will be enhanced to track usage across all tool iterations
	return &interfaces.LLMResponse{
		Content:    content,
		Model:      c.Model,
		StopReason: "",
		Usage:      nil, // TODO: Implement token usage tracking for tool iterations
		Metadata: map[string]interface{}{
			"provider":   "azure_openai",
			"deployment": c.deployment,
			"tools_used": true,
		},
	}, nil
}
