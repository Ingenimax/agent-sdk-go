package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// SamplingManager provides high-level operations for MCP sampling
type SamplingManager struct {
	servers []interfaces.MCPServer
	logger  logging.Logger
}

// NewSamplingManager creates a new sampling manager
func NewSamplingManager(servers []interfaces.MCPServer) *SamplingManager {
	return &SamplingManager{
		servers: servers,
		logger:  logging.New(),
	}
}

// CreateTextMessage creates a simple text message using the first available server
func (sm *SamplingManager) CreateTextMessage(ctx context.Context, prompt string, opts ...SamplingOption) (*interfaces.MCPSamplingResponse, error) {
	request := &interfaces.MCPSamplingRequest{
		Messages: []interfaces.MCPMessage{
			{
				Role: "user",
				Content: interfaces.MCPContent{
					Type: "text",
					Text: prompt,
				},
			},
		},
		ModelPreferences: &interfaces.MCPModelPreferences{
			IntelligencePriority: 0.7,
			SpeedPriority:        0.5,
			CostPriority:         0.3,
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(request)
	}

	return sm.CreateMessage(ctx, request)
}

// CreateMessage creates a message using the first available server
func (sm *SamplingManager) CreateMessage(ctx context.Context, request *interfaces.MCPSamplingRequest) (*interfaces.MCPSamplingResponse, error) {
	if len(sm.servers) == 0 {
		return nil, fmt.Errorf("no MCP servers available for sampling")
	}

	// Try each server until one succeeds
	var lastErr error
	for i, server := range sm.servers {
		serverName := fmt.Sprintf("server-%d", i)

		sm.logger.Debug(ctx, "Attempting sampling with server", map[string]interface{}{
			"server": serverName,
		})

		response, err := server.CreateMessage(ctx, request)
		if err != nil {
			sm.logger.Warn(ctx, "Sampling failed on server", map[string]interface{}{
				"server": serverName,
				"error":  err.Error(),
			})
			lastErr = err
			continue
		}

		sm.logger.Debug(ctx, "Sampling succeeded on server", map[string]interface{}{
			"server": serverName,
			"model":  response.Model,
		})

		return response, nil
	}

	return nil, fmt.Errorf("sampling failed on all servers: %w", lastErr)
}

// CreateConversation creates a multi-turn conversation
func (sm *SamplingManager) CreateConversation(ctx context.Context, messages []interfaces.MCPMessage, opts ...SamplingOption) (*interfaces.MCPSamplingResponse, error) {
	request := &interfaces.MCPSamplingRequest{
		Messages: messages,
		ModelPreferences: &interfaces.MCPModelPreferences{
			IntelligencePriority: 0.8,
			SpeedPriority:        0.4,
			CostPriority:         0.2,
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(request)
	}

	return sm.CreateMessage(ctx, request)
}

// CreateCodeGeneration creates a message optimized for code generation
func (sm *SamplingManager) CreateCodeGeneration(ctx context.Context, prompt string, language string, opts ...SamplingOption) (*interfaces.MCPSamplingResponse, error) {
	systemPrompt := fmt.Sprintf("You are an expert programmer. Generate high-quality %s code based on the user's request.", language)
	if language == "" {
		systemPrompt = "You are an expert programmer. Generate high-quality code based on the user's request."
	}

	request := &interfaces.MCPSamplingRequest{
		Messages: []interfaces.MCPMessage{
			{
				Role: "user",
				Content: interfaces.MCPContent{
					Type: "text",
					Text: prompt,
				},
			},
		},
		SystemPrompt: systemPrompt,
		ModelPreferences: &interfaces.MCPModelPreferences{
			IntelligencePriority: 0.9, // Prioritize intelligence for code generation
			SpeedPriority:        0.3,
			CostPriority:         0.1,
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(request)
	}

	return sm.CreateMessage(ctx, request)
}

// CreateSummary creates a message optimized for summarization tasks
func (sm *SamplingManager) CreateSummary(ctx context.Context, content string, maxLength int, opts ...SamplingOption) (*interfaces.MCPSamplingResponse, error) {
	prompt := fmt.Sprintf("Please summarize the following content in no more than %d words:\n\n%s", maxLength, content)

	request := &interfaces.MCPSamplingRequest{
		Messages: []interfaces.MCPMessage{
			{
				Role: "user",
				Content: interfaces.MCPContent{
					Type: "text",
					Text: prompt,
				},
			},
		},
		SystemPrompt: "You are a skilled summarizer. Create concise, accurate summaries that capture the key points.",
		ModelPreferences: &interfaces.MCPModelPreferences{
			IntelligencePriority: 0.7,
			SpeedPriority:        0.6, // Prioritize speed for summaries
			CostPriority:         0.4,
		},
		MaxTokens: &maxLength,
	}

	// Apply options
	for _, opt := range opts {
		opt(request)
	}

	return sm.CreateMessage(ctx, request)
}

// SamplingOption is a function that modifies a sampling request
type SamplingOption func(*interfaces.MCPSamplingRequest)

// WithSystemPrompt sets the system prompt
func WithSystemPrompt(prompt string) SamplingOption {
	return func(req *interfaces.MCPSamplingRequest) {
		req.SystemPrompt = prompt
	}
}

// WithMaxTokens sets the maximum number of tokens
func WithMaxTokens(maxTokens int) SamplingOption {
	return func(req *interfaces.MCPSamplingRequest) {
		req.MaxTokens = &maxTokens
	}
}

// WithTemperature sets the sampling temperature
func WithTemperature(temperature float64) SamplingOption {
	return func(req *interfaces.MCPSamplingRequest) {
		req.Temperature = &temperature
	}
}

// WithModelHint suggests a specific model
func WithModelHint(modelName string) SamplingOption {
	return func(req *interfaces.MCPSamplingRequest) {
		if req.ModelPreferences == nil {
			req.ModelPreferences = &interfaces.MCPModelPreferences{}
		}
		req.ModelPreferences.Hints = append(req.ModelPreferences.Hints, interfaces.MCPModelHint{
			Name: modelName,
		})
	}
}

// WithModelPreferences sets custom model preferences
func WithModelPreferences(cost, speed, intelligence float64) SamplingOption {
	return func(req *interfaces.MCPSamplingRequest) {
		if req.ModelPreferences == nil {
			req.ModelPreferences = &interfaces.MCPModelPreferences{}
		}
		req.ModelPreferences.CostPriority = cost
		req.ModelPreferences.SpeedPriority = speed
		req.ModelPreferences.IntelligencePriority = intelligence
	}
}

// WithStopSequences sets stop sequences for generation
func WithStopSequences(sequences ...string) SamplingOption {
	return func(req *interfaces.MCPSamplingRequest) {
		req.StopSequences = sequences
	}
}

// WithIncludeContext sets context inclusion preference
func WithIncludeContext(context string) SamplingOption {
	return func(req *interfaces.MCPSamplingRequest) {
		req.IncludeContext = context
	}
}

// Utility functions for common sampling operations

// ExtractCodeFromResponse extracts code from a sampling response
func ExtractCodeFromResponse(response *interfaces.MCPSamplingResponse) string {
	if response == nil || response.Content.Type != "text" {
		return ""
	}

	text := response.Content.Text

	// Try to extract code blocks
	lines := strings.Split(text, "\n")
	var codeLines []string
	inCodeBlock := false

	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			codeLines = append(codeLines, line)
		}
	}

	if len(codeLines) > 0 {
		return strings.Join(codeLines, "\n")
	}

	// If no code blocks found, return the full text
	return text
}

// FormatConversationHistory formats a conversation for display
func FormatConversationHistory(messages []interfaces.MCPMessage) string {
	var formatted []string

	for _, msg := range messages {
		// Use simple title case instead of deprecated strings.Title
		role := strings.ToUpper(msg.Role[:1]) + strings.ToLower(msg.Role[1:])
		if len(msg.Role) == 0 {
			role = ""
		}
		content := msg.Content.Text
		if len(content) > 100 {
			content = content[:97] + "..."
		}
		formatted = append(formatted, fmt.Sprintf("%s: %s", role, content))
	}

	return strings.Join(formatted, "\n")
}

// CreateImageAnalysisMessage creates a message for image analysis
func (sm *SamplingManager) CreateImageAnalysisMessage(ctx context.Context, imageData, mimeType, prompt string, opts ...SamplingOption) (*interfaces.MCPSamplingResponse, error) {
	request := &interfaces.MCPSamplingRequest{
		Messages: []interfaces.MCPMessage{
			{
				Role: "user",
				Content: interfaces.MCPContent{
					Type:     "image",
					Data:     imageData, // base64 encoded
					MimeType: mimeType,
				},
			},
			{
				Role: "user",
				Content: interfaces.MCPContent{
					Type: "text",
					Text: prompt,
				},
			},
		},
		ModelPreferences: &interfaces.MCPModelPreferences{
			IntelligencePriority: 0.9, // High intelligence for image analysis
			SpeedPriority:        0.4,
			CostPriority:         0.2,
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(request)
	}

	return sm.CreateMessage(ctx, request)
}