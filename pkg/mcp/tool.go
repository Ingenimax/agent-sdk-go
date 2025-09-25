package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/google/jsonschema-go/jsonschema"
)

// MCPTool implements interfaces.Tool for MCP tools
type MCPTool struct {
	name        string
	description string
	schema      interface{}
	server      interfaces.MCPServer
}

// NewMCPTool creates a new MCPTool
func NewMCPTool(name, description string, schema interface{}, server interfaces.MCPServer) interfaces.Tool {
	return &MCPTool{
		name:        name,
		description: description,
		schema:      schema,
		server:      server,
	}
}

// Name returns the name of the tool
func (t *MCPTool) Name() string {
	return t.name
}

// Description returns a description of what the tool does
func (t *MCPTool) Description() string {
	return t.description
}

// Run executes the tool with the given input
func (t *MCPTool) Run(ctx context.Context, input string) (string, error) {
	// Parse the input as JSON to get the arguments
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return "", fmt.Errorf("failed to parse input as JSON: %w", err)
	}

	// Call the tool on the MCP server
	resp, err := t.server.CallTool(ctx, t.name, args)
	if err != nil {
		return "", err
	}

	// Convert the response to a string
	if resp.IsError {
		return "", fmt.Errorf("MCP tool error: %v", resp.Content)
	}

	// Try to convert the content to a string
	switch content := resp.Content.(type) {
	case string:
		return content, nil
	case []byte:
		return string(content), nil
	default:
		// Try to JSON marshal the content
		bytes, err := json.Marshal(content)
		if err != nil {
			return fmt.Sprintf("%v", content), nil
		}
		return string(bytes), nil
	}
}

// Parameters returns the parameters that the tool accepts
func (t *MCPTool) Parameters() map[string]interfaces.ParameterSpec {
	// Convert the schema to a map of ParameterSpec
	// This is a simplified implementation; in a real implementation,
	// we would parse the JSON schema to extract parameter information
	params := make(map[string]interfaces.ParameterSpec)
	switch toolSchema := t.schema.(type) {
	// For backward compatibility
	//TODO remove in future releases
	case map[string]interface{}:
		// If the schema is a string, try to parse it as JSON
		// Try to convert the schema to a map
		if properties, ok := toolSchema["properties"].(map[string]interface{}); ok {
			for name, prop := range properties {
				if propMap, ok := prop.(map[string]interface{}); ok {
					paramSpec := interfaces.ParameterSpec{
						Type:        fmt.Sprintf("%v", propMap["type"]),
						Description: fmt.Sprintf("%v", propMap["description"]),
					}

					// Check if the parameter is required
					if required, ok := toolSchema["required"].([]interface{}); ok {
						for _, req := range required {
							if req == name {
								paramSpec.Required = true
								break
							}
						}
					}

					params[name] = paramSpec
				}
			}
		}
	case *jsonschema.Schema:
		for name, prop := range toolSchema.Properties {
			paramSpec := interfaces.ParameterSpec{
				Type:        prop.Type,
				Description: prop.Description,
			}

			if slices.Contains(toolSchema.Required, name) {
				paramSpec.Required = true
			}
			params[name] = paramSpec
		}
	}
	return params
}

// Execute executes the tool with the given arguments
func (t *MCPTool) Execute(ctx context.Context, args string) (string, error) {
	// This is the same as Run for MCPTool
	return t.Run(ctx, args)
}
