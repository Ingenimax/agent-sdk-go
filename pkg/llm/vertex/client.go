package vertex

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cloud.google.com/go/auth/credentials"
	"github.com/cenkalti/backoff/v4"
	"google.golang.org/genai"

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

// WithProjectID sets the GCP project ID for Vertex AI
func WithProjectID(projectID string) ClientOption {
	return func(c *Client) {
		c.projectID = projectID
	}
}

// WithClient injects an already initialized genai.Client. If set, NewClient won't build a new client
func WithClient(existing *genai.Client) ClientOption {
	return func(c *Client) {
		c.client = existing
		if existing != nil {
			cfg := existing.ClientConfig()
			if cfg.Project != "" {
				c.projectID = cfg.Project
			}
			if cfg.Location != "" {
				c.location = cfg.Location
			}
		}
	}
}

// NewClient creates a Vertex AI client wrapper. Provide project and other settings via options
func NewClient(ctx context.Context, options ...ClientOption) (*Client, error) {
	client := &Client{
		model:         DefaultModel,
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

	// If an existing client was injected, use it
	if client.client != nil {
		return client, nil
	}

	if client.projectID == "" {
		return nil, fmt.Errorf("projectID is required")
	}

	cc := &genai.ClientConfig{
		Backend:  genai.BackendVertexAI,
		Project:  client.projectID,
		Location: client.location,
	}

	if client.credentialsFile != "" {
		cred, err := credentials.DetectDefault(&credentials.DetectOptions{
			CredentialsFile: client.credentialsFile,
			Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to load credentials: %w", err)
		}
		cc.Credentials = cred
	}

	vertexClient, err := genai.NewClient(ctx, cc)
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

	// Create contents for the prompt
	contents := []*genai.Content{}

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

		contents = append([]*genai.Content{{
			Role:  genai.RoleUser,
			Parts: []*genai.Part{genai.NewPartFromText(systemMessage)},
		}}, contents...)
	}

	config := &genai.GenerateContentConfig{}
	if params.LLMConfig != nil {
		if params.LLMConfig.Temperature > 0 {
			temp := float32(params.LLMConfig.Temperature)
			config.Temperature = &temp
		}
		if params.LLMConfig.TopP > 0 {
			topP := float32(params.LLMConfig.TopP)
			config.TopP = &topP
		}
		if len(params.LLMConfig.StopSequences) > 0 {
			config.StopSequences = params.LLMConfig.StopSequences
		}
	}

	// Convert tools to Vertex AI format
	if len(tools) > 0 {
		config.Tools = c.convertTools(tools)
	}

	// Track tool call repetitions for loop detection
	toolCallHistory := make(map[string]int)

	chatSession, err := c.client.Chats.Create(ctx, c.model, config, contents)
	if err != nil {
		return "", fmt.Errorf("failed to create chat: %w", err)
	}

	nextMessage := []*genai.Part{genai.NewPartFromText(prompt)}
	// Iterative tool calling loop
	for iteration := 0; iteration < maxIterations; iteration++ {
		// Generate content with retry logic
		var response *genai.GenerateContentResponse
		err := c.withRetry(ctx, func() error {
			var genErr error
			response, genErr = chatSession.Send(ctx, nextMessage...)
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
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				text.WriteString(part.Text)
			}
		}
		functionCalls := response.FunctionCalls()

		// If there are no function calls, return the text response
		if len(functionCalls) == 0 {
			return text.String(), nil
		}

		// Execute all function calls and collect responses
		var functionResponses []*genai.Part

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
				return "", fmt.Errorf("tool not found: %s (iteration %d)", funcCall.Name, iteration+1)
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

			functionResponses = append(functionResponses, genai.NewPartFromFunctionResponse(funcCall.Name, map[string]any{"result": toolResult}))
		}

		// Continue conversation by sending tool responses
		nextMessage = functionResponses
	}

	// Final call asking for conclusion
	c.logger.Info("Maximum iterations reached, requesting final conclusion", "maxIterations", maxIterations)
	finalResponse, err := chatSession.Send(ctx, genai.NewPartFromText("Please provide your final response based on the information available. Do not request any additional tools."))
	if err != nil {
		return "", fmt.Errorf("failed to generate final response: %w", err)
	}
	if len(finalResponse.Candidates) == 0 || finalResponse.Candidates[0].Content == nil {
		return "", fmt.Errorf("no final response received")
	}
	var finalText strings.Builder
	for _, part := range finalResponse.Candidates[0].Content.Parts {
		if part.Text != "" {
			finalText.WriteString(part.Text)
		}
	}
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

	// Create contents for the prompt
	contents := []*genai.Content{
		{
			Role:  genai.RoleUser,
			Parts: []*genai.Part{genai.NewPartFromText(prompt)},
		},
	}

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

		contents = append([]*genai.Content{{
			Role:  genai.RoleUser,
			Parts: []*genai.Part{genai.NewPartFromText(systemMessage)},
		}}, contents...)
	}

	config := &genai.GenerateContentConfig{}
	if params.LLMConfig != nil {
		if params.LLMConfig.Temperature > 0 {
			temp := float32(params.LLMConfig.Temperature)
			config.Temperature = &temp
		}
		if params.LLMConfig.TopP > 0 {
			topP := float32(params.LLMConfig.TopP)
			config.TopP = &topP
		}
		if len(params.LLMConfig.StopSequences) > 0 {
			config.StopSequences = params.LLMConfig.StopSequences
		}
	}

	var response *genai.GenerateContentResponse
	err := c.withRetry(ctx, func() error {
		var genErr error
		response, genErr = c.client.Models.GenerateContent(ctx, c.model, contents, config)
		return genErr
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if len(response.Candidates) == 0 || response.Candidates[0].Content == nil {
		return "", fmt.Errorf("no candidates/content in response")
	}

	var result strings.Builder
	for _, part := range response.Candidates[0].Content.Parts {
		if part.Text != "" {
			result.WriteString(part.Text)
		}
	}
	return result.String(), nil
}

// convertMessages converts llm.Message to Vertex AI parts
func (c *Client) convertMessages(messages []llm.Message) ([]*genai.Part, error) {
	var parts []*genai.Part

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			// System messages are handled separately in Vertex AI
			continue
		case "user", "assistant":
			parts = append(parts, &genai.Part{Text: msg.Content})
		default:
			return nil, fmt.Errorf("unsupported message role: %s", msg.Role)
		}
	}

	return parts, nil
}

// convertTools converts tools to Vertex AI format
func (c *Client) convertTools(tools []interfaces.Tool) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}

	// Create a single tool with multiple function declarations
	var functionDeclarations []*genai.FunctionDeclaration

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

		functionDeclarations = append(functionDeclarations, &genai.FunctionDeclaration{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  schema,
		})
	}

	// Return a single tool with all function declarations
	return []*genai.Tool{
		{
			FunctionDeclarations: functionDeclarations,
		},
	}
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
	// google.golang.org/genai client does not expose a Close method
	return nil
}
