package vertex

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cloud.google.com/go/vertexai/genai"
	"github.com/cenkalti/backoff/v4"
	"google.golang.org/api/option"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
)

// VertexAI model constants
const (
	ModelGemini15Pro     = "gemini-1.5-pro"
	ModelGemini15Flash   = "gemini-1.5-flash"
	ModelGemini20Flash   = "gemini-2.0-flash-exp"
	ModelGeminiProVision = "gemini-pro-vision"
)

// DefaultModel is the default Vertex AI model
const DefaultModel = ModelGemini15Pro

// ReasoningMode defines the reasoning approach for the model
type ReasoningMode string

const (
	ReasoningModeNone          ReasoningMode = "none"
	ReasoningModeMinimal       ReasoningMode = "minimal"
	ReasoningModeComprehensive ReasoningMode = "comprehensive"
)

// Client represents a Vertex AI client
type Client struct {
	client          *genai.Client
	model           string
	projectID       string
	location        string
	maxRetries      int
	retryDelay      time.Duration
	reasoningMode   ReasoningMode
	logger          *slog.Logger
	credentialsFile string
}

// ClientOption is a function that configures the Client
type ClientOption func(*Client)

// WithModel sets the model for the client
func WithModel(model string) ClientOption {
	return func(c *Client) {
		c.model = model
	}
}

// WithLocation sets the location for the client
func WithLocation(location string) ClientOption {
	return func(c *Client) {
		c.location = location
	}
}

// WithMaxRetries sets the maximum number of retries
func WithMaxRetries(maxRetries int) ClientOption {
	return func(c *Client) {
		c.maxRetries = maxRetries
	}
}

// WithRetryDelay sets the retry delay
func WithRetryDelay(delay time.Duration) ClientOption {
	return func(c *Client) {
		c.retryDelay = delay
	}
}

// WithReasoningMode sets the reasoning mode
func WithReasoningMode(mode ReasoningMode) ClientOption {
	return func(c *Client) {
		c.reasoningMode = mode
	}
}

// WithLogger sets the logger for the client
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithCredentialsFile sets the path to the service account credentials file
func WithCredentialsFile(credentialsFile string) ClientOption {
	return func(c *Client) {
		c.credentialsFile = credentialsFile
	}
}

// NewClient creates a new Vertex AI client
func NewClient(ctx context.Context, projectID string, options ...ClientOption) (*Client, error) {
	if projectID == "" {
		return nil, fmt.Errorf("projectID is required")
	}

	client := &Client{
		model:         DefaultModel,
		projectID:     projectID,
		location:      "us-central1",
		maxRetries:    3,
		retryDelay:    time.Second,
		reasoningMode: ReasoningModeNone,
		logger:        slog.Default(),
	}

	// Apply options
	for _, opt := range options {
		opt(client)
	}

	// Initialize Vertex AI client
	var clientOptions []option.ClientOption

	// Add credentials file if specified
	if client.credentialsFile != "" {
		clientOptions = append(clientOptions, option.WithCredentialsFile(client.credentialsFile))
	}

	vertexClient, err := genai.NewClient(ctx, projectID, client.location, clientOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Vertex AI client: %w", err)
	}

	client.client = vertexClient
	return client, nil
}

// Name returns the client name
func (c *Client) Name() string {
	return fmt.Sprintf("vertex:%s", c.model)
}

// SupportsStreaming returns false as streaming is not yet implemented for Vertex
func (c *Client) SupportsStreaming() bool {
	return false
}

// GenerateWithTools implements interfaces.LLM.GenerateWithTools
func (c *Client) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
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
		maxIterations = 2 // Default to current behavior
	}

	// Create parts for the prompt
	parts := []genai.Part{genai.Text(prompt)}

	// Add system message if provided
	if params.SystemMessage != "" {
		systemMessage := params.SystemMessage

		// Apply reasoning if specified
		if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
			switch params.LLMConfig.Reasoning {
			case "minimal":
				systemMessage = fmt.Sprintf("%s\n\nWhen responding, briefly explain your thought process.", systemMessage)
			case "comprehensive":
				systemMessage = fmt.Sprintf("%s\n\nWhen responding, please think step-by-step and explain your complete reasoning process in detail.", systemMessage)
			case "none":
				systemMessage = fmt.Sprintf("%s\n\nProvide direct, concise answers without explaining your reasoning or showing calculations.", systemMessage)
			}
		}

		parts = append([]genai.Part{genai.Text(systemMessage)}, parts...)
	}

	model := c.client.GenerativeModel(c.model)

	// Configure model parameters
	if params.LLMConfig != nil {
		if params.LLMConfig.Temperature > 0 {
			temp := float32(params.LLMConfig.Temperature)
			model.Temperature = &temp
		}
		if params.LLMConfig.TopP > 0 {
			topP := float32(params.LLMConfig.TopP)
			model.TopP = &topP
		}
		if len(params.LLMConfig.StopSequences) > 0 {
			model.StopSequences = params.LLMConfig.StopSequences
		}
	}

	// Convert tools to Vertex AI format
	var vertexTools []*genai.Tool
	if len(tools) > 0 {
		vertexTools = c.convertTools(tools)
		model.Tools = vertexTools
	}

	// Track tool call repetitions for loop detection
	toolCallHistory := make(map[string]int)

	// Create a chat session for iterative conversation
	session := model.StartChat()

	// Add the original user message to start the conversation
	session.History = []*genai.Content{
		{
			Parts: parts,
			Role:  "user",
		},
	}

	// Iterative tool calling loop
	for iteration := 0; iteration < maxIterations; iteration++ {
		// Generate content with retry logic
		var response *genai.GenerateContentResponse
		err := c.withRetry(ctx, func() error {
			var genErr error
			if iteration == 0 {
				// First iteration: use the initial model with tools
				response, genErr = model.GenerateContent(ctx, parts...)
			} else {
				// Subsequent iterations: continue the chat session
				response, genErr = session.SendMessage(ctx)
			}
			return genErr
		})

		if err != nil {
			return "", fmt.Errorf("failed to generate content (iteration %d): %w", iteration+1, err)
		}

		// Extract response
		if len(response.Candidates) == 0 {
			return "", fmt.Errorf("no candidates in response (iteration %d)", iteration+1)
		}

		candidate := response.Candidates[0]
		if candidate.Content == nil {
			return "", fmt.Errorf("no content in response (iteration %d)", iteration+1)
		}

		var text strings.Builder
		var functionCalls []genai.FunctionCall

		for _, part := range candidate.Content.Parts {
			switch p := part.(type) {
			case genai.Text:
				text.WriteString(string(p))
			case genai.FunctionCall:
				functionCalls = append(functionCalls, p)
			}
		}

		// If there are no function calls, return the text response
		if len(functionCalls) == 0 {
			return text.String(), nil
		}

		// Execute all function calls and collect responses
		var functionResponses []genai.Part

		for _, funcCall := range functionCalls {
			// Find the corresponding tool
			var selectedTool interfaces.Tool
			for _, tool := range tools {
				if tool.Name() == funcCall.Name {
					selectedTool = tool
					break
				}
			}

			if selectedTool == nil {
				c.logger.Error("Tool not found", "toolName", funcCall.Name, "iteration", iteration+1)

				// Add tool not found error as function response instead of returning
				errorMessage := fmt.Sprintf("Error: tool not found: %s", funcCall.Name)
				toolCallID := fmt.Sprintf("tool_%d_%s", iteration, funcCall.Name)

				// Store failed tool call in memory if provided
				if params.Memory != nil {
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []interfaces.ToolCall{{
							ID:        toolCallID,
							Name:      funcCall.Name,
							Arguments: "{}",
						}},
					})
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:       "tool",
						Content:    errorMessage,
						ToolCallID: toolCallID,
						Metadata: map[string]interface{}{
							"tool_name": funcCall.Name,
						},
					})
				}

				// Create function response with error
				funcResponse := genai.FunctionResponse{
					Name:     funcCall.Name,
					Response: map[string]any{"result": errorMessage},
				}

				functionResponses = append(functionResponses, funcResponse)
				continue // Continue processing other function calls
			}

			// Convert arguments to JSON string
			argsJSON, err := json.Marshal(funcCall.Args)
			if err != nil {
				return "", fmt.Errorf("failed to marshal function arguments (iteration %d): %w", iteration+1, err)
			}

			// Execute the tool
			toolResult, execErr := selectedTool.Execute(ctx, string(argsJSON))

			// Check for repetitive calls and add warning if needed
			cacheKey := funcCall.Name + ":" + string(argsJSON)
			toolCallHistory[cacheKey]++

			if toolCallHistory[cacheKey] > 2 {
				warning := fmt.Sprintf("\n\n[WARNING: This is call #%d to %s with identical parameters. You may be in a loop. Consider using the available information to provide a final answer.]",
					toolCallHistory[cacheKey],
					funcCall.Name)
				if execErr == nil {
					toolResult += warning
				}
				c.logger.Warn("Repetitive tool call detected", "toolName", funcCall.Name, "callCount", toolCallHistory[cacheKey], "iteration", iteration+1)
			}

			// Store tool call and result in memory if provided
			toolCallID := fmt.Sprintf("tool_%d_%s", iteration, funcCall.Name)
			if params.Memory != nil {
				if execErr != nil {
					// Store failed tool call result
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []interfaces.ToolCall{{
							ID:        toolCallID,
							Name:      funcCall.Name,
							Arguments: string(argsJSON),
						}},
					})
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:       "tool",
						Content:    fmt.Sprintf("Error: %v", execErr),
						ToolCallID: toolCallID,
						Metadata: map[string]interface{}{
							"tool_name": funcCall.Name,
						},
					})
				} else {
					// Store successful tool call and result
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []interfaces.ToolCall{{
							ID:        toolCallID,
							Name:      funcCall.Name,
							Arguments: string(argsJSON),
						}},
					})
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:       "tool",
						Content:    toolResult,
						ToolCallID: toolCallID,
						Metadata: map[string]interface{}{
							"tool_name": funcCall.Name,
						},
					})
				}
			}

			if execErr != nil {
				c.logger.Error("Tool execution failed", "toolName", selectedTool.Name(), "iteration", iteration+1, "error", execErr)
				// Instead of failing, provide error message as tool result
				toolResult = fmt.Sprintf("Error: %v", execErr)
			}

			// Create function response
			funcResponse := genai.FunctionResponse{
				Name:     funcCall.Name,
				Response: map[string]any{"result": toolResult},
			}

			functionResponses = append(functionResponses, funcResponse)
		}

		// Add the assistant's response with function calls to the session history
		if iteration == 0 {
			// For the first iteration, we need to add the assistant response to the session
			session.History = append(session.History, &genai.Content{
				Parts: candidate.Content.Parts,
				Role:  "model",
			})
		}

		// Send function responses for the next iteration
		if iteration < maxIterations-1 {
			// Continue the conversation with function responses
			_, err = session.SendMessage(ctx, functionResponses...)
			if err != nil {
				return "", fmt.Errorf("failed to send function responses (iteration %d): %w", iteration+1, err)
			}
		} else {
			// Last iteration, send function responses and get final response
			finalResponse, err := session.SendMessage(ctx, functionResponses...)
			if err != nil {
				return "", fmt.Errorf("failed to send final function responses: %w", err)
			}

			if len(finalResponse.Candidates) > 0 && finalResponse.Candidates[0].Content != nil {
				var finalText strings.Builder
				for _, part := range finalResponse.Candidates[0].Content.Parts {
					if textPart, ok := part.(genai.Text); ok {
						finalText.WriteString(string(textPart))
					}
				}
				return finalText.String(), nil
			}

			return "", fmt.Errorf("no final response received")
		}
	}

	// If we've reached the maximum iterations and the model is still requesting tools,
	// make one final call without tools to get a conclusion
	c.logger.Info("Maximum iterations reached, making final call without tools", "maxIterations", maxIterations)

	// Create a model without tools for the final call
	finalModel := c.client.GenerativeModel(c.model)

	// Configure model parameters
	if params.LLMConfig != nil {
		if params.LLMConfig.Temperature > 0 {
			temp := float32(params.LLMConfig.Temperature)
			finalModel.Temperature = &temp
		}
		if params.LLMConfig.TopP > 0 {
			topP := float32(params.LLMConfig.TopP)
			finalModel.TopP = &topP
		}
		if len(params.LLMConfig.StopSequences) > 0 {
			finalModel.StopSequences = params.LLMConfig.StopSequences
		}
	}

	// Add conclusion prompt to the session
	conclusionPrompt := genai.Text("Please provide your final response based on the information available. Do not request any additional tools.")

	finalResponse, err := session.SendMessage(ctx, conclusionPrompt)
	if err != nil {
		c.logger.Error("Error in final call without tools", "error", err)
		return "", fmt.Errorf("failed to generate final response without tools: %w", err)
	}

	if len(finalResponse.Candidates) == 0 {
		return "", fmt.Errorf("no candidates in final response")
	}

	candidate := finalResponse.Candidates[0]
	if candidate.Content == nil {
		return "", fmt.Errorf("no content in final response")
	}

	var finalText strings.Builder
	for _, part := range candidate.Content.Parts {
		if textPart, ok := part.(genai.Text); ok {
			finalText.WriteString(string(textPart))
		}
	}

	c.logger.Info("Successfully received final response without tools")
	return finalText.String(), nil
}

// Generate implements interfaces.LLM.Generate
func (c *Client) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// Apply options
	params := &interfaces.GenerateOptions{
		LLMConfig: &interfaces.LLMConfig{
			Temperature: 0.7,
		},
	}

	for _, option := range options {
		option(params)
	}

	// Create parts for the prompt
	parts := []genai.Part{genai.Text(prompt)}

	// Add system message if provided
	if params.SystemMessage != "" {
		systemMessage := params.SystemMessage

		// Apply reasoning if specified
		if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
			switch params.LLMConfig.Reasoning {
			case "minimal":
				systemMessage = fmt.Sprintf("%s\n\nWhen responding, briefly explain your thought process.", systemMessage)
			case "comprehensive":
				systemMessage = fmt.Sprintf("%s\n\nWhen responding, please think step-by-step and explain your complete reasoning process in detail.", systemMessage)
			case "none":
				systemMessage = fmt.Sprintf("%s\n\nProvide direct, concise answers without explaining your reasoning or showing calculations.", systemMessage)
			}
		}

		parts = append([]genai.Part{genai.Text(systemMessage)}, parts...)
	}

	model := c.client.GenerativeModel(c.model)

	// Configure model parameters
	if params.LLMConfig != nil {
		if params.LLMConfig.Temperature > 0 {
			temp := float32(params.LLMConfig.Temperature)
			model.Temperature = &temp
		}
		if params.LLMConfig.TopP > 0 {
			topP := float32(params.LLMConfig.TopP)
			model.TopP = &topP
		}
		if len(params.LLMConfig.StopSequences) > 0 {
			model.StopSequences = params.LLMConfig.StopSequences
		}
	}

	// Generate content with retry logic
	var response *genai.GenerateContentResponse
	err := c.withRetry(ctx, func() error {
		var genErr error
		response, genErr = model.GenerateContent(ctx, parts...)
		return genErr
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	// Extract text from response
	if len(response.Candidates) == 0 {
		return "", fmt.Errorf("no candidates in response")
	}

	candidate := response.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	var result strings.Builder
	for _, part := range candidate.Content.Parts {
		if textPart, ok := part.(genai.Text); ok {
			result.WriteString(string(textPart))
		}
	}

	return result.String(), nil
}

// convertMessages converts llm.Message to Vertex AI parts
func (c *Client) convertMessages(messages []llm.Message) ([]genai.Part, error) {
	var parts []genai.Part

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			// System messages are handled separately in Vertex AI
			continue
		case "user", "assistant":
			parts = append(parts, genai.Text(msg.Content))
		default:
			return nil, fmt.Errorf("unsupported message role: %s", msg.Role)
		}
	}

	return parts, nil
}

// convertTools converts tools to Vertex AI format
func (c *Client) convertTools(tools []interfaces.Tool) []*genai.Tool {
	var vertexTools []*genai.Tool

	for _, tool := range tools {
		schema := &genai.Schema{
			Type: genai.TypeObject,
		}

		// Get tool parameters
		parameters := tool.Parameters()
		if len(parameters) > 0 {
			schema.Properties = make(map[string]*genai.Schema)

			for name, param := range parameters {
				propSchema := &genai.Schema{
					Description: param.Description,
				}

				switch param.Type {
				case "string":
					propSchema.Type = genai.TypeString
				case "number":
					propSchema.Type = genai.TypeNumber
				case "boolean":
					propSchema.Type = genai.TypeBoolean
				case "array":
					propSchema.Type = genai.TypeArray
				case "object":
					propSchema.Type = genai.TypeObject
				default:
					propSchema.Type = genai.TypeString
				}

				schema.Properties[name] = propSchema

				if param.Required {
					schema.Required = append(schema.Required, name)
				}
			}
		}

		vertexTool := &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        tool.Name(),
					Description: tool.Description(),
					Parameters:  schema,
				},
			},
		}

		vertexTools = append(vertexTools, vertexTool)
	}

	return vertexTools
}

// getReasoningInstruction returns the reasoning instruction based on the mode
func (c *Client) getReasoningInstruction() string {
	switch c.reasoningMode {
	case ReasoningModeMinimal:
		return "Provide clear, direct responses with brief explanations when necessary."
	case ReasoningModeComprehensive:
		return "Think through problems step by step, showing your reasoning process and providing detailed explanations."
	default:
		return ""
	}
}

// withRetry executes the function with exponential backoff retry logic
func (c *Client) withRetry(ctx context.Context, fn func() error) error {
	exponentialBackoff := backoff.NewExponentialBackOff()
	exponentialBackoff.InitialInterval = c.retryDelay
	exponentialBackoff.MaxElapsedTime = time.Duration(c.maxRetries) * c.retryDelay * 2

	return backoff.Retry(fn, backoff.WithContext(exponentialBackoff, ctx))
}

// Close closes the Vertex AI client
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}
