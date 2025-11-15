package mcp

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// PromptManager provides high-level operations for MCP prompts
type PromptManager struct {
	servers []interfaces.MCPServer
	logger  logging.Logger
}

// NewPromptManager creates a new prompt manager
func NewPromptManager(servers []interfaces.MCPServer) *PromptManager {
	return &PromptManager{
		servers: servers,
		logger:  logging.New(),
	}
}

// ListAllPrompts lists prompts from all MCP servers
func (pm *PromptManager) ListAllPrompts(ctx context.Context) (map[string][]interfaces.MCPPrompt, error) {
	result := make(map[string][]interfaces.MCPPrompt)

	for i, server := range pm.servers {
		serverName := fmt.Sprintf("server-%d", i)

		prompts, err := server.ListPrompts(ctx)
		if err != nil {
			pm.logger.Warn(ctx, "Failed to list prompts from server", map[string]interface{}{
				"server": serverName,
				"error":  err.Error(),
			})
			continue
		}

		result[serverName] = prompts
		pm.logger.Debug(ctx, "Listed prompts from server", map[string]interface{}{
			"server":       serverName,
			"prompt_count": len(prompts),
		})
	}

	return result, nil
}

// FindPrompts searches for prompts by name pattern across all servers
func (pm *PromptManager) FindPrompts(ctx context.Context, pattern string) ([]PromptMatch, error) {
	var matches []PromptMatch

	for i, server := range pm.servers {
		serverName := fmt.Sprintf("server-%d", i)

		prompts, err := server.ListPrompts(ctx)
		if err != nil {
			pm.logger.Warn(ctx, "Failed to list prompts from server", map[string]interface{}{
				"server": serverName,
				"error":  err.Error(),
			})
			continue
		}

		for _, prompt := range prompts {
			if pm.matchesPattern(prompt, pattern) {
				matches = append(matches, PromptMatch{
					Server: server,
					Prompt: prompt,
				})
			}
		}
	}

	pm.logger.Debug(ctx, "Found matching prompts", map[string]interface{}{
		"pattern":     pattern,
		"match_count": len(matches),
	})

	return matches, nil
}

// GetPrompt retrieves a prompt by name from any server that has it
func (pm *PromptManager) GetPrompt(ctx context.Context, name string, variables map[string]interface{}) (*PromptResult, error) {
	for i, server := range pm.servers {
		serverName := fmt.Sprintf("server-%d", i)

		// First check if this server has the prompt
		prompts, err := server.ListPrompts(ctx)
		if err != nil {
			continue
		}

		var foundPrompt *interfaces.MCPPrompt
		for _, p := range prompts {
			if p.Name == name {
				foundPrompt = &p
				break
			}
		}

		if foundPrompt == nil {
			continue
		}

		// Get the prompt with variables
		result, err := server.GetPrompt(ctx, name, variables)
		if err != nil {
			pm.logger.Warn(ctx, "Failed to get prompt from server", map[string]interface{}{
				"server": serverName,
				"prompt": name,
				"error":  err.Error(),
			})
			continue
		}

		pm.logger.Debug(ctx, "Successfully retrieved prompt", map[string]interface{}{
			"server": serverName,
			"prompt": name,
		})

		return &PromptResult{
			Server: server,
			Prompt: *foundPrompt,
			Result: *result,
		}, nil
	}

	return nil, fmt.Errorf("prompt not found on any server: %s", name)
}

// ExecutePromptTemplate executes a prompt template with variables and returns rendered content
func (pm *PromptManager) ExecutePromptTemplate(ctx context.Context, promptName string, variables map[string]interface{}) (string, error) {
	promptResult, err := pm.GetPrompt(ctx, promptName, variables)
	if err != nil {
		return "", err
	}

	// If we have a single prompt string, return it
	if promptResult.Result.Prompt != "" {
		return promptResult.Result.Prompt, nil
	}

	// If we have messages, combine them into a single string
	if len(promptResult.Result.Messages) > 0 {
		var parts []string
		for _, msg := range promptResult.Result.Messages {
			if msg.Role != "" {
				parts = append(parts, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
			} else {
				parts = append(parts, msg.Content)
			}
		}
		return strings.Join(parts, "\n"), nil
	}

	return "", fmt.Errorf("prompt %s returned no content", promptName)
}

// GetPromptsByCategory returns prompts filtered by category (from metadata)
func (pm *PromptManager) GetPromptsByCategory(ctx context.Context, category string) ([]PromptMatch, error) {
	var matches []PromptMatch

	for _, server := range pm.servers {
		prompts, err := server.ListPrompts(ctx)
		if err != nil {
			continue
		}

		for _, prompt := range prompts {
			if pm.matchesCategory(prompt, category) {
				matches = append(matches, PromptMatch{
					Server: server,
					Prompt: prompt,
				})
			}
		}
	}

	return matches, nil
}

// ValidatePromptVariables checks if all required variables are provided
func (pm *PromptManager) ValidatePromptVariables(prompt interfaces.MCPPrompt, variables map[string]interface{}) error {
	var missingRequired []string

	for _, arg := range prompt.Arguments {
		if arg.Required {
			if _, exists := variables[arg.Name]; !exists {
				missingRequired = append(missingRequired, arg.Name)
			}
		}
	}

	if len(missingRequired) > 0 {
		return fmt.Errorf("missing required variables: %s", strings.Join(missingRequired, ", "))
	}

	return nil
}

// BuildVariablesFromTemplate builds variables from a Go template string
func (pm *PromptManager) BuildVariablesFromTemplate(templateStr string, data interface{}) (map[string]interface{}, error) {
	tmpl, err := template.New("variables").Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	// For now, return empty map - this would need more sophisticated parsing
	// to extract variable values from the rendered template
	return make(map[string]interface{}), nil
}

// Helper types

// PromptMatch represents a prompt found on a specific server
type PromptMatch struct {
	Server interfaces.MCPServer
	Prompt interfaces.MCPPrompt
}

// PromptResult represents a prompt result with its source
type PromptResult struct {
	Server interfaces.MCPServer
	Prompt interfaces.MCPPrompt
	Result interfaces.MCPPromptResult
}

// PromptCategory represents a category of prompts
type PromptCategory struct {
	Name        string
	Description string
	Prompts     []PromptMatch
}

// Helper methods

func (pm *PromptManager) matchesPattern(prompt interfaces.MCPPrompt, pattern string) bool {
	pattern = strings.ToLower(pattern)

	// Check name and description
	if strings.Contains(strings.ToLower(prompt.Name), pattern) {
		return true
	}
	if strings.Contains(strings.ToLower(prompt.Description), pattern) {
		return true
	}

	// Check metadata
	for key, value := range prompt.Metadata {
		if strings.Contains(strings.ToLower(key), pattern) ||
			strings.Contains(strings.ToLower(value), pattern) {
			return true
		}
	}

	return false
}

func (pm *PromptManager) matchesCategory(prompt interfaces.MCPPrompt, category string) bool {
	if prompt.Metadata == nil {
		return false
	}

	// Check for category in metadata
	promptCategory, exists := prompt.Metadata["category"]
	if !exists {
		// Try alternative keys
		for key, value := range prompt.Metadata {
			if strings.ToLower(key) == "category" ||
				strings.ToLower(key) == "type" ||
				strings.ToLower(key) == "group" {
				promptCategory = value
				break
			}
		}
	}

	if promptCategory == "" {
		return false
	}

	return strings.EqualFold(promptCategory, category)
}

// Utility functions for common prompt operations

// GetPromptParameterInfo returns human-readable information about prompt parameters
func GetPromptParameterInfo(prompt interfaces.MCPPrompt) string {
	if len(prompt.Arguments) == 0 {
		return "No parameters required"
	}

	var parts []string
	for _, arg := range prompt.Arguments {
		paramInfo := arg.Name
		if arg.Type != "" {
			paramInfo += fmt.Sprintf(" (%s)", arg.Type)
		}
		if arg.Required {
			paramInfo += " *required*"
		}
		if arg.Description != "" {
			paramInfo += fmt.Sprintf(" - %s", arg.Description)
		}
		if arg.Default != nil {
			paramInfo += fmt.Sprintf(" (default: %v)", arg.Default)
		}
		parts = append(parts, paramInfo)
	}

	return "Parameters:\n" + strings.Join(parts, "\n")
}

// SuggestPromptVariables suggests default values for prompt variables
func SuggestPromptVariables(prompt interfaces.MCPPrompt, context map[string]interface{}) map[string]interface{} {
	suggested := make(map[string]interface{})

	for _, arg := range prompt.Arguments {
		// Use default value if available
		if arg.Default != nil {
			suggested[arg.Name] = arg.Default
			continue
		}

		// Try to find matching values in context
		if value, exists := context[arg.Name]; exists {
			suggested[arg.Name] = value
			continue
		}

		// Suggest common values based on parameter name
		switch strings.ToLower(arg.Name) {
		case "name", "username", "user":
			if user, exists := context["user"]; exists {
				suggested[arg.Name] = user
			}
		case "project", "repo", "repository":
			if project, exists := context["project"]; exists {
				suggested[arg.Name] = project
			}
		case "language", "lang":
			suggested[arg.Name] = "go"
		case "format", "output_format":
			suggested[arg.Name] = "markdown"
		}
	}

	return suggested
}
