package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/sashabaranov/go-openai"
)

// Define a custom type for context keys to avoid collisions
type contextKey string

// Define constants for context keys
const organizationKey contextKey = "organization"

// OpenAIClient implements the LLM interface for OpenAI
type OpenAIClient struct {
	Client *openai.Client
	Model  string
	logger logging.Logger
}

// Option represents an option for configuring the OpenAI client
type Option func(*OpenAIClient)

// WithModel sets the model for the OpenAI client
func WithModel(model string) Option {
	return func(c *OpenAIClient) {
		c.Model = model
	}
}

// WithLogger sets the logger for the OpenAI client
func WithLogger(logger logging.Logger) Option {
	return func(c *OpenAIClient) {
		c.logger = logger
	}
}

// NewClient creates a new OpenAI client
func NewClient(apiKey string, options ...Option) *OpenAIClient {
	// Create client with default options
	client := &OpenAIClient{
		Client: openai.NewClient(apiKey),
		Model:  "gpt-4o-mini",
		logger: logging.New(),
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
		Temperature: 0.7,
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
	messages := []openai.ChatCompletionMessage{}

	// Add system message if available
	if params.SystemMessage != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    "system",
			Content: params.SystemMessage,
		})
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": params.SystemMessage})
	}

	// Add user message
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: prompt,
	})

	// Create request
	req := openai.ChatCompletionRequest{
		Model:            c.Model,
		Messages:         messages,
		Temperature:      float32(params.Temperature),
		TopP:             float32(params.TopP),
		FrequencyPenalty: float32(params.FrequencyPenalty),
		PresencePenalty:  float32(params.PresencePenalty),
		Stop:             params.StopSequences,
	}

	// Set response format if provided
	if params.ResponseFormat != nil {
		req.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   params.ResponseFormat.Name,
				Schema: params.ResponseFormat.Schema,
			},
		}
		c.logger.Debug(ctx, "Using response format", map[string]interface{}{"format": *params.ResponseFormat})
	}

	//c.logger.Debug(ctx, "Sending request to OpenAI", map[string]interface{}{"request": req})
	// Set organization ID if available
	if orgID, ok := ctx.Value(organizationKey).(string); ok && orgID != "" {
		req.User = orgID
	}

	// Send request
	resp, err := c.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		c.logger.Error(ctx, "Error from OpenAI API", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("failed to generate text: %w", err)
	}

	// Return response
	if len(resp.Choices) > 0 {
		return resp.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no response from OpenAI API")
}

// Chat uses the ChatCompletion API to have a conversation (messages) with a model
func (c *OpenAIClient) Chat(ctx context.Context, messages []llm.Message, params *llm.GenerateParams) (string, error) {
	if params == nil {
		params = llm.DefaultGenerateParams()
	}

	// Convert messages to the OpenAI Chat format
	chatMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		chatMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Create chat request
	req := openai.ChatCompletionRequest{
		Model:            c.Model,
		Messages:         chatMessages,
		Temperature:      float32(params.Temperature),
		TopP:             float32(params.TopP),
		FrequencyPenalty: float32(params.FrequencyPenalty),
		PresencePenalty:  float32(params.PresencePenalty),
		Stop:             params.StopSequences,
	}

	// Send request
	resp, err := c.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no completions returned")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// GenerateWithTools implements interfaces.LLM.GenerateWithTools
func (c *OpenAIClient) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	// Convert options to params
	params := &interfaces.GenerateOptions{
		Temperature: 0.7,
	}
	for _, opt := range options {
		if opt != nil {
			opt(params)
		}
	}

	// Check for organization ID in context
	orgID := "default"
	if id, err := multitenancy.GetOrgID(ctx); err == nil {
		orgID = id
	}
	ctx = context.WithValue(ctx, organizationKey, orgID)

	// Convert tools to OpenAI format
	openaiTools := make([]openai.Tool, len(tools))
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
		}

		openaiTools[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": properties,
					"required":   required,
				},
			},
		}
	}

	// Create messages array with system message if provided
	messages := []openai.ChatCompletionMessage{}

	// Add system message if available
	if params.SystemMessage != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    "system",
			Content: params.SystemMessage,
		})
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": params.SystemMessage})
	}

	// Add user message
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: prompt,
	})

	req := openai.ChatCompletionRequest{
		Model:            c.Model,
		Messages:         messages,
		Tools:            openaiTools,
		Temperature:      float32(params.Temperature),
		TopP:             float32(params.TopP),
		FrequencyPenalty: float32(params.FrequencyPenalty),
		PresencePenalty:  float32(params.PresencePenalty),
		Stop:             params.StopSequences,
	}

	// Set response format if provided
	if params.ResponseFormat != nil {
		req.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   params.ResponseFormat.Name,
				Schema: params.ResponseFormat.Schema,
			},
		}
		c.logger.Debug(ctx, "Using response format", map[string]interface{}{"format": *params.ResponseFormat})
	}

	// Send request
	resp, err := c.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		c.logger.Error(ctx, "Error from OpenAI API", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no completions returned")
	}

	// Check if the model wants to use tools
	if len(resp.Choices[0].Message.ToolCalls) > 0 {
		// The model wants to use tools
		toolCalls := resp.Choices[0].Message.ToolCalls
		c.logger.Info(ctx, "Processing tool calls", map[string]interface{}{"count": len(toolCalls)})

		// Create a new conversation with the initial messages
		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: prompt},
			resp.Choices[0].Message,
		}

		// Process each tool call
		for _, toolCall := range toolCalls {
			// Find the requested tool
			var selectedTool interfaces.Tool
			for _, tool := range tools {
				if tool.Name() == toolCall.Function.Name {
					selectedTool = tool
					break
				}
			}

			if selectedTool.Name() == "" {
				c.logger.Error(ctx, "Tool not found", map[string]interface{}{"toolName": toolCall.Function.Name})
				return "", fmt.Errorf("tool not found: %s", toolCall.Function.Name)
			}

			// Execute the tool
			c.logger.Info(ctx, "Executing tool", map[string]interface{}{"toolName": selectedTool.Name()})
			toolResult, err := selectedTool.Execute(ctx, toolCall.Function.Arguments)
			if err != nil {
				c.logger.Error(ctx, "Error executing tool", map[string]interface{}{"toolName": selectedTool.Name(), "error": err.Error()})
				// Add error message as tool response
				messages = append(messages, openai.ChatCompletionMessage{
					Role:       "tool",
					Content:    fmt.Sprintf("Error: %v", err),
					Name:       selectedTool.Name(),
					ToolCallID: toolCall.ID,
				})
				continue
			}

			// Add tool result to messages
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       "tool",
				Content:    toolResult,
				Name:       selectedTool.Name(),
				ToolCallID: toolCall.ID,
			})
		}

		// Get the final response
		c.logger.Info(ctx, "Sending final request with tool results", nil)

		req := openai.ChatCompletionRequest{
			Model:            c.Model,
			Messages:         messages,
			Temperature:      float32(params.Temperature),
			TopP:             float32(params.TopP),
			FrequencyPenalty: float32(params.FrequencyPenalty),
			PresencePenalty:  float32(params.PresencePenalty),
			Stop:             params.StopSequences,
		}

		// Set response format for final request if provided
		if params.ResponseFormat != nil {
			req.ResponseFormat = &openai.ChatCompletionResponseFormat{
				Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
				JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
					Name:   params.ResponseFormat.Name,
					Schema: params.ResponseFormat.Schema,
				},
			}
			c.logger.Debug(ctx, "Using response format", map[string]interface{}{"format": *params.ResponseFormat})
		}

		finalCompletion, err := c.Client.CreateChatCompletion(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "Error from final OpenAI API call", map[string]interface{}{"error": err.Error()})
			return "", fmt.Errorf("failed to create final chat completion: %w", err)
		}

		if len(finalCompletion.Choices) == 0 {
			return "", fmt.Errorf("no completions returned")
		}

		content := strings.TrimSpace(finalCompletion.Choices[0].Message.Content)
		return content, nil
	}

	// No tool was used, return the direct response
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	return content, nil
}

// Name implements interfaces.LLM.Name
func (c *OpenAIClient) Name() string {
	return "openai"
}

// WithTemperature creates a GenerateOption to set the temperature
func WithTemperature(temperature float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.Temperature = temperature
	}
}

// WithTopP creates a GenerateOption to set the top_p
func WithTopP(topP float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.TopP = topP
	}
}

// WithFrequencyPenalty creates a GenerateOption to set the frequency penalty
func WithFrequencyPenalty(frequencyPenalty float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.FrequencyPenalty = frequencyPenalty
	}
}

// WithPresencePenalty creates a GenerateOption to set the presence penalty
func WithPresencePenalty(presencePenalty float64) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.PresencePenalty = presencePenalty
	}
}

// WithStopSequences creates a GenerateOption to set the stop sequences
func WithStopSequences(stopSequences []string) interfaces.GenerateOption {
	return func(options *interfaces.GenerateOptions) {
		options.StopSequences = stopSequences
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
