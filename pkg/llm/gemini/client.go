package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/auth/credentials"
	"google.golang.org/genai"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
	"github.com/Ingenimax/agent-sdk-go/pkg/tracing"
)

// Model constants for Gemini API
const (
	// Stable models
	ModelGemini25Pro       = "gemini-2.5-pro"
	ModelGemini25Flash     = "gemini-2.5-flash"
	ModelGemini25FlashLite = "gemini-2.5-flash-lite"
	ModelGemini20Flash     = "gemini-2.0-flash"
	ModelGemini20FlashLite = "gemini-2.0-flash-lite"
	ModelGemini15Pro       = "gemini-1.5-pro"
	ModelGemini15Flash     = "gemini-1.5-flash"
	ModelGemini15Flash8B   = "gemini-1.5-flash-8b"

	// Preview/Experimental models
	ModelGeminiLive25FlashPreview            = "gemini-live-2.5-flash-preview"
	ModelGemini25FlashPreviewNativeAudio     = "gemini-2.5-flash-preview-native-audio-dialog"
	ModelGemini25FlashExpNativeAudioThinking = "gemini-2.5-flash-exp-native-audio-thinking-dialog"
	ModelGemini25FlashPreviewTTS             = "gemini-2.5-flash-preview-tts"
	ModelGemini25ProPreviewTTS               = "gemini-2.5-pro-preview-tts"
	ModelGemini20FlashPreviewImageGen        = "gemini-2.0-flash-preview-image-generation"
	ModelGemini20FlashLive001                = "gemini-2.0-flash-live-001"

	// Default model
	DefaultModel = ModelGemini15Flash
)

// GeminiClient implements the LLM interface for Google Gemini API
type GeminiClient struct {
	genaiClient     *genai.Client
	apiKey          string
	model           string
	backend         genai.Backend
	projectID       string
	location        string
	credentialsFile string
	logger          logging.Logger
	retryExecutor   *retry.Executor
	thinkingConfig  *ThinkingConfig
}

// Option represents an option for configuring the Gemini client
type Option func(*GeminiClient)

// WithModel sets the model for the Gemini client
func WithModel(model string) Option {
	return func(c *GeminiClient) {
		c.model = model
	}
}

// WithLogger sets the logger for the Gemini client
func WithLogger(logger logging.Logger) Option {
	return func(c *GeminiClient) {
		c.logger = logger
	}
}

// WithRetry configures retry policy for the client
func WithRetry(opts ...retry.Option) Option {
	return func(c *GeminiClient) {
		c.retryExecutor = retry.NewExecutor(retry.NewPolicy(opts...))
	}
}

// WithAPIKey sets the API key for Gemini API backend
func WithAPIKey(apiKey string) Option {
	return func(c *GeminiClient) {
		c.apiKey = apiKey
	}
}

// WithBaseURL sets the base URL for the Gemini client (not used with genai package)
func WithBaseURL(baseURL string) Option {
	return func(c *GeminiClient) {
		// Note: baseURL is not used with the genai package as it manages the endpoint internally
		c.logger.Warn(context.Background(), "BaseURL option is not supported with Gemini API client", nil)
	}
}

// WithClient injects an already initialized genai.Client. If set, NewClient won't build a new client
func WithClient(existing *genai.Client) Option {
	return func(c *GeminiClient) {
		c.genaiClient = existing
	}
}

// WithBackend sets the backend for the Gemini client
func WithBackend(backend genai.Backend) Option {
	return func(c *GeminiClient) {
		c.backend = backend
	}
}

// WithProjectID sets the GCP project ID for Vertex AI backend
func WithProjectID(projectID string) Option {
	return func(c *GeminiClient) {
		c.projectID = projectID
	}
}

// WithLocation sets the GCP location for Vertex AI backend
func WithLocation(location string) Option {
	return func(c *GeminiClient) {
		c.location = location
	}
}

func WithCredentialsFile(credentialsFile string) Option {
	return func(c *GeminiClient) {
		c.credentialsFile = credentialsFile
	}
}

// NewClient creates a new Gemini client
func NewClient(ctx context.Context, options ...Option) (*GeminiClient, error) {
	// Create client with default options
	defaultThinking := DefaultThinkingConfig()
	client := &GeminiClient{
		model:          DefaultModel,
		backend:        genai.BackendGeminiAPI,
		location:       "us-central1", // Default Vertex AI location
		logger:         logging.New(),
		thinkingConfig: &defaultThinking,
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	// If an existing client was injected, use it
	if client.genaiClient != nil {
		return client, nil
	}

	// Create the genai client if not already provided
	if client.genaiClient == nil {
		config := &genai.ClientConfig{
			Backend: client.backend,
		}

		// Configure based on backend type
		switch client.backend {
		case genai.BackendGeminiAPI:
			if client.apiKey == "" {
				return nil, fmt.Errorf("API key is required for Gemini API backend")
			}
			config.APIKey = client.apiKey
		case genai.BackendVertexAI:
			if client.projectID == "" && client.credentialsFile == "" && client.apiKey == "" {
				return nil, fmt.Errorf("project ID, credentials file or API key are required for Vertex AI backend")
			}
			if client.credentialsFile != "" {
				creds, err := credentials.DetectDefault(&credentials.DetectOptions{
					CredentialsFile: client.credentialsFile,
				})
				if err != nil {
					return nil, fmt.Errorf("failed to load credentials from file: %w", err)
				}
				config.Credentials = creds
			}
			if client.projectID != "" {
				config.Project = client.projectID
				config.Location = client.location
			}
			if client.apiKey != "" {
				config.APIKey = client.apiKey
			}
		}

		genaiClient, err := genai.NewClient(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini client: %w", err)
		}

		client.genaiClient = genaiClient
	}

	return client, nil
}

// Generate generates text from a prompt
func (c *GeminiClient) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
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

	// Build the request content
	parts := []*genai.Part{
		{Text: prompt},
	}

	contents := []*genai.Content{
		{
			Role:  "user",
			Parts: parts,
		},
	}

	// Add system instruction if provided or if reasoning is specified
	var systemInstruction *genai.Content
	systemMessage := params.SystemMessage

	// Log reasoning mode usage - only affects native thinking models (2.5 series)
	if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
		if SupportsThinking(c.model) {
			c.logger.Debug(ctx, "Using reasoning mode with thinking-capable model", map[string]interface{}{
				"reasoning": params.LLMConfig.Reasoning,
				"model":     c.model,
			})
		} else {
			c.logger.Debug(ctx, "Reasoning mode specified for non-thinking model - native thinking tokens not available", map[string]interface{}{
				"reasoning":        params.LLMConfig.Reasoning,
				"model":            c.model,
				"supportsThinking": false,
			})
		}
	}

	if systemMessage != "" {
		systemInstruction = &genai.Content{
			Parts: []*genai.Part{
				{Text: systemMessage},
			},
		}
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": systemMessage})
	}

	// Set generation config
	var genConfig *genai.GenerationConfig
	if params.LLMConfig != nil {
		genConfig = &genai.GenerationConfig{}

		if params.LLMConfig.Temperature > 0 {
			temp := float32(params.LLMConfig.Temperature)
			genConfig.Temperature = &temp
		}
		if params.LLMConfig.TopP > 0 {
			topP := float32(params.LLMConfig.TopP)
			genConfig.TopP = &topP
		}
		if len(params.LLMConfig.StopSequences) > 0 {
			genConfig.StopSequences = params.LLMConfig.StopSequences
		}
	}

	// Set response format if provided
	if params.ResponseFormat != nil {
		if genConfig == nil {
			genConfig = &genai.GenerationConfig{}
		}

		genConfig.ResponseMIMEType = "application/json"

		// Convert schema for genai
		if schemaBytes, err := json.Marshal(params.ResponseFormat.Schema); err == nil {
			var schema *genai.Schema
			if err := json.Unmarshal(schemaBytes, &schema); err != nil {
				c.logger.Warn(ctx, "Failed to convert response schema", map[string]interface{}{"error": err.Error()})
			} else {
				genConfig.ResponseSchema = schema
			}
		}
		c.logger.Debug(ctx, "Using response format", map[string]interface{}{"format": *params.ResponseFormat})
	}

	var result *genai.GenerateContentResponse
	var err error

	operation := func() error {
		c.logger.Debug(ctx, "Executing Gemini API request", map[string]interface{}{
			"model":           c.model,
			"temperature":     genConfig.Temperature,
			"top_p":           genConfig.TopP,
			"stop_sequences":  genConfig.StopSequences,
			"response_format": params.ResponseFormat != nil,
			"org_id":          orgID,
		})

		config := &genai.GenerateContentConfig{
			SystemInstruction: systemInstruction,
		}

		// Apply generation config parameters directly to config
		if genConfig != nil {
			if genConfig.Temperature != nil {
				config.Temperature = genConfig.Temperature
			}
			if genConfig.TopP != nil {
				config.TopP = genConfig.TopP
			}
			if len(genConfig.StopSequences) > 0 {
				config.StopSequences = genConfig.StopSequences
			}
			if genConfig.ResponseMIMEType != "" {
				config.ResponseMIMEType = genConfig.ResponseMIMEType
			}
			if genConfig.ResponseSchema != nil {
				config.ResponseSchema = genConfig.ResponseSchema
			}
		}

		// Add thinking configuration if supported and enabled
		if SupportsThinking(c.model) && c.thinkingConfig != nil {
			if c.thinkingConfig.IncludeThoughts || c.thinkingConfig.ThinkingBudget != nil {
				config.ThinkingConfig = &genai.ThinkingConfig{
					IncludeThoughts: c.thinkingConfig.IncludeThoughts,
					ThinkingBudget:  c.thinkingConfig.ThinkingBudget,
				}

				c.logger.Debug(ctx, "Enabled thinking configuration", map[string]interface{}{
					"includeThoughts": c.thinkingConfig.IncludeThoughts,
					"thinkingBudget":  c.thinkingConfig.ThinkingBudget,
				})
			}
		}

		result, err = c.genaiClient.Models.GenerateContent(ctx, c.model, contents, config)
		if err != nil {
			c.logger.Error(ctx, "Error from Gemini API", map[string]interface{}{
				"error": err.Error(),
				"model": c.model,
			})
			return fmt.Errorf("failed to generate text: %w", err)
		}
		return nil
	}

	if c.retryExecutor != nil {
		c.logger.Debug(ctx, "Using retry mechanism for Gemini request", map[string]interface{}{
			"model": c.model,
		})
		err = c.retryExecutor.Execute(ctx, operation)
	} else {
		err = operation()
	}

	if err != nil {
		return "", err
	}

	// Extract response and separate thinking from final content
	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		c.logger.Debug(ctx, "Successfully received response from Gemini", map[string]interface{}{
			"model": c.model,
		})

		var textParts []string
		var thinkingParts []string

		for _, part := range result.Candidates[0].Content.Parts {
			if part.Text != "" {
				if part.Thought {
					// This is thinking content
					thinkingParts = append(thinkingParts, part.Text)
					c.logger.Debug(ctx, "Received thinking content", map[string]interface{}{
						"length": len(part.Text),
					})
				} else {
					// This is final response content
					textParts = append(textParts, part.Text)
				}
			}
		}

		// For non-streaming Generate, we return only the final response content
		// The thinking content is available but not returned in this interface
		// (it would be available in streaming through StreamEventThinking)
		if len(thinkingParts) > 0 {
			c.logger.Info(ctx, "Thinking content received but not included in response", map[string]interface{}{
				"thinkingParts": len(thinkingParts),
				"finalParts":    len(textParts),
			})
		}

		return strings.Join(textParts, ""), nil
	}

	return "", fmt.Errorf("no response from Gemini API")
}

// GenerateWithTools implements interfaces.LLM.GenerateWithTools
func (c *GeminiClient) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
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
	_ = orgID // Mark as used to avoid linter warning

	// Convert tools to Gemini format
	geminiTools := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		functionDeclaration := &genai.FunctionDeclaration{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: make(map[string]*genai.Schema),
				Required:   make([]string, 0),
			},
		}

		// Convert parameters
		for name, param := range tool.Parameters() {
			paramSchema := &genai.Schema{
				Description: param.Description,
			}

			// Set type
			switch param.Type {
			case "string":
				paramSchema.Type = genai.TypeString
			case "number", "integer":
				paramSchema.Type = genai.TypeNumber
			case "boolean":
				paramSchema.Type = genai.TypeBoolean
			case "array":
				paramSchema.Type = genai.TypeArray
			case "object":
				paramSchema.Type = genai.TypeObject
			}

			// Handle array items
			if param.Items != nil {
				itemSchema := &genai.Schema{}

				// Set items type
				switch param.Items.Type {
				case "string":
					itemSchema.Type = genai.TypeString
				case "number", "integer":
					itemSchema.Type = genai.TypeNumber
				case "boolean":
					itemSchema.Type = genai.TypeBoolean
				case "array":
					itemSchema.Type = genai.TypeArray
				case "object":
					itemSchema.Type = genai.TypeObject
				}

				// Handle items enum if present
				if param.Items.Enum != nil {
					enumStrings := make([]string, len(param.Items.Enum))
					for i, e := range param.Items.Enum {
						enumStrings[i] = fmt.Sprintf("%v", e)
					}
					itemSchema.Enum = enumStrings
				}

				paramSchema.Items = itemSchema
			}

			if param.Enum != nil {
				enumStrings := make([]string, len(param.Enum))
				for i, e := range param.Enum {
					enumStrings[i] = fmt.Sprintf("%v", e)
				}
				paramSchema.Enum = enumStrings
			}

			functionDeclaration.Parameters.Properties[name] = paramSchema
			if param.Required {
				functionDeclaration.Parameters.Required = append(functionDeclaration.Parameters.Required, name)
			}
		}

		geminiTools = append(geminiTools, functionDeclaration)
	}

	// Create contents array starting with system message if provided
	contents := []*genai.Content{}
	var systemInstruction *genai.Content

	// Track tool call repetitions for loop detection
	toolCallHistory := make(map[string]int)
	var toolCallHistoryMu sync.Mutex

	// Add system message if available
	if params.SystemMessage != "" {
		systemMessage := params.SystemMessage

		// Log reasoning mode usage - only affects native thinking models (2.5 series)
		if params.LLMConfig != nil && params.LLMConfig.Reasoning != "" {
			if SupportsThinking(c.model) {
				c.logger.Debug(ctx, "Using reasoning mode with thinking-capable model", map[string]interface{}{
					"reasoning": params.LLMConfig.Reasoning,
					"model":     c.model,
				})
			} else {
				c.logger.Debug(ctx, "Reasoning mode specified for non-thinking model - native thinking tokens not available", map[string]interface{}{
					"reasoning":        params.LLMConfig.Reasoning,
					"model":            c.model,
					"supportsThinking": false,
				})
			}
		}

		systemInstruction = &genai.Content{
			Parts: []*genai.Part{
				{Text: systemMessage},
			},
		}
		c.logger.Debug(ctx, "Using system message", map[string]interface{}{"system_message": systemMessage})
	}

	// Add user message
	contents = append(contents, &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: prompt},
		},
	})

	// Iterative tool calling loop
	for iteration := 0; iteration < maxIterations; iteration++ {
		// Set generation config
		var genConfig *genai.GenerationConfig
		if params.LLMConfig != nil {
			genConfig = &genai.GenerationConfig{}

			if params.LLMConfig.Temperature > 0 {
				temp := float32(params.LLMConfig.Temperature)
				genConfig.Temperature = &temp
			}
			if params.LLMConfig.TopP > 0 {
				topP := float32(params.LLMConfig.TopP)
				genConfig.TopP = &topP
			}
			if len(params.LLMConfig.StopSequences) > 0 {
				genConfig.StopSequences = params.LLMConfig.StopSequences
			}
		}

		// Set response format if provided
		if params.ResponseFormat != nil {
			if genConfig == nil {
				genConfig = &genai.GenerationConfig{}
			}
			genConfig.ResponseMIMEType = "application/json"

			// Convert schema for genai
			if schemaBytes, err := json.Marshal(params.ResponseFormat.Schema); err == nil {
				var schema *genai.Schema
				if err := json.Unmarshal(schemaBytes, &schema); err != nil {
					c.logger.Warn(ctx, "Failed to convert response schema", map[string]interface{}{"error": err.Error()})
				} else {
					genConfig.ResponseSchema = schema
				}
			}
			c.logger.Debug(ctx, "Using response format", map[string]interface{}{"format": *params.ResponseFormat})
		}

		logData := map[string]interface{}{
			"model":           c.model,
			"contents":        len(contents),
			"tools":           len(geminiTools),
			"response_format": params.ResponseFormat != nil,
			"iteration":       iteration + 1,
			"maxIterations":   maxIterations,
		}

		if genConfig != nil {
			if genConfig.Temperature != nil {
				logData["temperature"] = *genConfig.Temperature
			}
			if genConfig.TopP != nil {
				logData["top_p"] = *genConfig.TopP
			}
			if len(genConfig.StopSequences) > 0 {
				logData["stop_sequences"] = genConfig.StopSequences
			}
		}

		c.logger.Debug(ctx, "Sending request with tools to Gemini", logData)

		config := &genai.GenerateContentConfig{
			Tools: []*genai.Tool{
				{
					FunctionDeclarations: geminiTools,
				},
			},
			SystemInstruction: systemInstruction,
		}

		// Apply generation config parameters directly to config
		if genConfig != nil {
			if genConfig.Temperature != nil {
				config.Temperature = genConfig.Temperature
			}
			if genConfig.TopP != nil {
				config.TopP = genConfig.TopP
			}
			if len(genConfig.StopSequences) > 0 {
				config.StopSequences = genConfig.StopSequences
			}
			if genConfig.ResponseMIMEType != "" {
				config.ResponseMIMEType = genConfig.ResponseMIMEType
			}
			if genConfig.ResponseSchema != nil {
				config.ResponseSchema = genConfig.ResponseSchema
			}
		}

		result, err := c.genaiClient.Models.GenerateContent(ctx, c.model, contents, config)
		if err != nil {
			c.logger.Error(ctx, "Error from Gemini API", map[string]interface{}{"error": err.Error()})
			return "", fmt.Errorf("failed to create content: %w", err)
		}

		if len(result.Candidates) == 0 {
			return "", fmt.Errorf("no candidates returned")
		}

		candidate := result.Candidates[0]
		if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
			return "", fmt.Errorf("no content in response")
		}

		// Check if any part contains function calls
		hasFunctionCalls := false
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				hasFunctionCalls = true
				break
			}
		}

		// If no function calls, return the text response
		if !hasFunctionCalls {
			var textParts []string
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					textParts = append(textParts, part.Text)
				}
			}
			return strings.Join(textParts, " "), nil
		}

		// Process function calls
		c.logger.Info(ctx, "Processing function calls", map[string]interface{}{
			"iteration": iteration + 1,
		})

		// Add the assistant's message with function calls to the conversation
		// Ensure the role is set to "model"
		assistantContent := &genai.Content{
			Role:  "model",
			Parts: candidate.Content.Parts,
		}
		contents = append(contents, assistantContent)

		// Collect all function responses to add them in a single content message
		var functionResponses []*genai.Part

		// Process each function call
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall == nil {
				continue
			}

			functionCall := part.FunctionCall

			// Find the requested tool
			var selectedTool interfaces.Tool
			for _, tool := range tools {
				if tool.Name() == functionCall.Name {
					selectedTool = tool
					break
				}
			}

			if selectedTool == nil {
				c.logger.Error(ctx, "Tool not found", map[string]interface{}{
					"toolName": functionCall.Name,
				})

				// Add tool not found error as function response
				functionResponses = append(functionResponses, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						Name: functionCall.Name,
						Response: map[string]any{
							"error": fmt.Sprintf("tool not found: %s", functionCall.Name),
						},
					},
				})

				// Store failed tool call in memory if provided
				if params.Memory != nil {
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []interfaces.ToolCall{{
							Name:      functionCall.Name,
							Arguments: "{}",
						}},
					})
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:    "tool",
						Content: fmt.Sprintf("Error: tool not found: %s", functionCall.Name),
						Metadata: map[string]interface{}{
							"tool_name": functionCall.Name,
						},
					})
				}

				// Add to tracing context
				toolCallTrace := tracing.ToolCall{
					Name:       functionCall.Name,
					Arguments:  "{}",
					Timestamp:  time.Now().Format(time.RFC3339),
					StartTime:  time.Now(),
					Duration:   0,
					DurationMs: 0,
					Error:      fmt.Sprintf("tool not found: %s", functionCall.Name),
					Result:     fmt.Sprintf("Error: tool not found: %s", functionCall.Name),
				}

				tracing.AddToolCallToContext(ctx, toolCallTrace)

				continue // Continue processing other function calls
			}

			// Convert function call arguments to JSON string
			argsBytes, err := json.Marshal(functionCall.Args)
			if err != nil {
				c.logger.Error(ctx, "Failed to marshal function call arguments", map[string]interface{}{
					"error": err.Error(),
				})
				return "", fmt.Errorf("failed to marshal function call arguments: %w", err)
			}

			// Execute the tool
			c.logger.Info(ctx, "Executing tool", map[string]interface{}{"toolName": selectedTool.Name()})
			toolStartTime := time.Now()
			toolResult, err := selectedTool.Execute(ctx, string(argsBytes))
			toolEndTime := time.Now()

			// Check for repetitive calls and add warning if needed
			cacheKey := functionCall.Name + ":" + string(argsBytes)

			toolCallHistoryMu.Lock()
			toolCallHistory[cacheKey]++
			callCount := toolCallHistory[cacheKey]
			toolCallHistoryMu.Unlock()

			if callCount > 1 {
				warning := fmt.Sprintf("\n\n[WARNING: This is call #%d to %s with identical parameters. You may be in a loop. Consider using the available information to provide a final answer.]",
					callCount,
					functionCall.Name)
				if err == nil {
					toolResult += warning
				}
				c.logger.Warn(ctx, "Repetitive tool call detected", map[string]interface{}{
					"toolName":  functionCall.Name,
					"callCount": callCount,
				})
			}

			// Add tool call to tracing context
			executionDuration := toolEndTime.Sub(toolStartTime)
			toolCallTrace := tracing.ToolCall{
				Name:       functionCall.Name,
				Arguments:  string(argsBytes),
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
							Name:      functionCall.Name,
							Arguments: string(argsBytes),
						}},
					})
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:    "tool",
						Content: fmt.Sprintf("Error: %v", err),
						Metadata: map[string]interface{}{
							"tool_name": functionCall.Name,
						},
					})
				} else {
					// Store successful tool call and result
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []interfaces.ToolCall{{
							Name:      functionCall.Name,
							Arguments: string(argsBytes),
						}},
					})
					_ = params.Memory.AddMessage(ctx, interfaces.Message{
						Role:    "tool",
						Content: toolResult,
						Metadata: map[string]interface{}{
							"tool_name": functionCall.Name,
						},
					})
				}
			}

			if err != nil {
				c.logger.Error(ctx, "Tool execution failed", map[string]interface{}{
					"toolName": selectedTool.Name(),
					"toolArgs": string(argsBytes),
					"error":    err.Error(),
					"duration": toolEndTime.Sub(toolStartTime).String(),
				})
				toolCallTrace.Error = err.Error()
				toolCallTrace.Result = fmt.Sprintf("Error: %v", err)

				// Add error message as function response
				functionResponses = append(functionResponses, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						Name: functionCall.Name,
						Response: map[string]any{
							"error": err.Error(),
						},
					},
				})
			} else {
				toolCallTrace.Result = toolResult

				// Add tool result as function response
				functionResponses = append(functionResponses, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						Name: functionCall.Name,
						Response: map[string]any{
							"result": toolResult,
						},
					},
				})
			}

			// Add the tool call to the tracing context
			tracing.AddToolCallToContext(ctx, toolCallTrace)
		}

		// Add all function responses in a single content message
		if len(functionResponses) > 0 {

			// Add all function responses in a single content message
			resultContent := &genai.Content{
				Role:  "user",
				Parts: functionResponses,
			}
			contents = append(contents, resultContent)
		}

		// Continue to the next iteration with updated contents
	}

	// If we've reached the maximum iterations and the model is still requesting tools,
	// make one final call without tools to get a conclusion
	c.logger.Info(ctx, "Maximum iterations reached, making final call without tools", map[string]interface{}{
		"maxIterations": maxIterations,
	})

	// Set generation config
	var genConfig *genai.GenerationConfig
	if params.LLMConfig != nil {
		genConfig = &genai.GenerationConfig{}

		if params.LLMConfig.Temperature > 0 {
			temp := float32(params.LLMConfig.Temperature)
			genConfig.Temperature = &temp
		}
		if params.LLMConfig.TopP > 0 {
			topP := float32(params.LLMConfig.TopP)
			genConfig.TopP = &topP
		}
		if len(params.LLMConfig.StopSequences) > 0 {
			genConfig.StopSequences = params.LLMConfig.StopSequences
		}
	}

	// Set response format if provided
	if params.ResponseFormat != nil {
		if genConfig == nil {
			genConfig = &genai.GenerationConfig{}
		}
		genConfig.ResponseMIMEType = "application/json"

		// Convert schema for genai
		if schemaBytes, err := json.Marshal(params.ResponseFormat.Schema); err == nil {
			var schema *genai.Schema
			if err := json.Unmarshal(schemaBytes, &schema); err != nil {
				c.logger.Warn(ctx, "Failed to convert response schema", map[string]interface{}{"error": err.Error()})
			} else {
				genConfig.ResponseSchema = schema
			}
		}
	}

	// Add a conclusion instruction to the contents
	contents = append(contents, &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: "Please provide your final response based on the information available. Do not request any additional functions."},
		},
	})

	c.logger.Debug(ctx, "Making final request without tools", map[string]interface{}{
		"contents": len(contents),
	})

	config := &genai.GenerateContentConfig{
		SystemInstruction: systemInstruction,
	}

	// Apply generation config parameters directly to config
	if genConfig != nil {
		if genConfig.Temperature != nil {
			config.Temperature = genConfig.Temperature
		}
		if genConfig.TopP != nil {
			config.TopP = genConfig.TopP
		}
		if len(genConfig.StopSequences) > 0 {
			config.StopSequences = genConfig.StopSequences
		}
		if genConfig.ResponseMIMEType != "" {
			config.ResponseMIMEType = genConfig.ResponseMIMEType
		}
		if genConfig.ResponseSchema != nil {
			config.ResponseSchema = genConfig.ResponseSchema
		}
	}

	finalResult, err := c.genaiClient.Models.GenerateContent(ctx, c.model, contents, config)
	if err != nil {
		c.logger.Error(ctx, "Error in final call without tools", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("failed to create final content: %w", err)
	}

	if len(finalResult.Candidates) == 0 {
		return "", fmt.Errorf("no candidates returned in final call")
	}

	candidate := finalResult.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("no content in final response")
	}

	// Extract text from all parts
	var textParts []string
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
	}

	content := strings.TrimSpace(strings.Join(textParts, " "))
	c.logger.Info(ctx, "Successfully received final response without tools", nil)
	return content, nil
}

// Name implements interfaces.LLM.Name
func (c *GeminiClient) Name() string {
	return "gemini"
}

// SupportsStreaming implements interfaces.LLM.SupportsStreaming
func (c *GeminiClient) SupportsStreaming() bool {
	return true
}

// GetModel returns the model name being used
func (c *GeminiClient) GetModel() string {
	return c.model
}
