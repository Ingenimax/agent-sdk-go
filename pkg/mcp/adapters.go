package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// MCPToolAdapter converts MCP tools to the agent SDK tool interface
type MCPToolAdapter struct {
	mcpServer interfaces.MCPServer
	toolName  string
	toolDesc  string
	params    map[string]ParameterSpec
}

// ParameterSpec represents a parameter specification for an MCP tool
type ParameterSpec struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// NewMCPToolAdapter creates a new adapter for an MCP tool
func NewMCPToolAdapter(server interfaces.MCPServer, name, description string, params map[string]ParameterSpec) *MCPToolAdapter {
	return &MCPToolAdapter{
		mcpServer: server,
		toolName:  name,
		toolDesc:  description,
		params:    params,
	}
}

// Name returns the name of the tool
func (a *MCPToolAdapter) Name() string {
	return a.toolName
}

// DisplayName implements interfaces.ToolWithDisplayName.DisplayName
func (a *MCPToolAdapter) DisplayName() string {
	return a.toolName
}

// Description returns the description of the tool
func (a *MCPToolAdapter) Description() string {
	return a.toolDesc
}

// Internal implements interfaces.InternalTool.Internal
func (a *MCPToolAdapter) Internal() bool {
	return false
}

// Run executes the tool with the given input
func (a *MCPToolAdapter) Run(ctx context.Context, input string) (string, error) {
	return a.Execute(ctx, input)
}

// Parameters returns the parameters that the tool accepts
func (a *MCPToolAdapter) Parameters() map[string]interfaces.ParameterSpec {
	// Convert MCP parameter specs to agent SDK parameter specs
	result := make(map[string]interfaces.ParameterSpec)
	for name, spec := range a.params {
		result[name] = interfaces.ParameterSpec{
			Type:        spec.Type,
			Description: spec.Description,
			Required:    spec.Required,
		}
	}
	return result
}

// InputSchema returns the schema for the tool's input
func (a *MCPToolAdapter) InputSchema() map[string]interfaces.ParameterSpec {
	return a.Parameters()
}

// Execute executes the tool with the given arguments string
func (a *MCPToolAdapter) Execute(ctx context.Context, args string) (string, error) {
	// Parse the arguments from JSON string
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("error parsing arguments for MCP tool %s: %w", a.toolName, err)
	}

	// Call the MCP server's CallTool method
	result, err := a.mcpServer.CallTool(ctx, a.toolName, params)
	if err != nil {
		return "", fmt.Errorf("error calling MCP tool %s: %w", a.toolName, err)
	}

	return fmt.Sprintf("%v", result), nil
}

// ConvertMCPTools converts a list of MCP tools to agent SDK tools
func ConvertMCPTools(server interfaces.MCPServer, mcpTools []map[string]interface{}) ([]interfaces.Tool, error) {
	var tools []interfaces.Tool

	for _, mcpTool := range mcpTools {
		// Extract name and description
		name, ok := mcpTool["name"].(string)
		if !ok {
			return nil, fmt.Errorf("MCP tool missing name field or not a string")
		}

		description, ok := mcpTool["description"].(string)
		if !ok {
			description = "No description provided"
		}

		// Extract parameters
		paramsMap, ok := mcpTool["parameters"].(map[string]interface{})
		if !ok {
			paramsMap = make(map[string]interface{})
		}

		// Convert parameters to ParameterSpec
		params := make(map[string]ParameterSpec)
		for paramName, paramSpec := range paramsMap {
			spec, ok := paramSpec.(map[string]interface{})
			if !ok {
				continue
			}

			paramType, _ := spec["type"].(string)
			paramDesc, _ := spec["description"].(string)
			paramRequired, _ := spec["required"].(bool)

			params[paramName] = ParameterSpec{
				Type:        paramType,
				Description: paramDesc,
				Required:    paramRequired,
			}
		}

		// Create and add the adapter
		adapter := NewMCPToolAdapter(server, name, description, params)
		tools = append(tools, adapter)
	}

	return tools, nil
}
