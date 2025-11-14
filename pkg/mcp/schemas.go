package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// SchemaValidator handles validation of tool outputs against their schemas
type SchemaValidator struct {
	logger logging.Logger
}

// NewSchemaValidator creates a new schema validator
func NewSchemaValidator() *SchemaValidator {
	return &SchemaValidator{
		logger: logging.New(),
	}
}

// ValidateToolResponse validates a tool response against its expected schema
func (sv *SchemaValidator) ValidateToolResponse(ctx context.Context, tool interfaces.MCPTool, response *interfaces.MCPToolResponse) error {
	if tool.OutputSchema == nil {
		// No schema to validate against - this is fine
		return nil
	}

	if response.StructuredContent == nil {
		// If there's a schema but no structured content, that's potentially an error
		sv.logger.Warn(ctx, "Tool has output schema but response has no structured content", map[string]interface{}{
			"tool_name": tool.Name,
		})
		return nil
	}

	// Validate the structured content against the schema
	return sv.validateAgainstSchema(ctx, response.StructuredContent, tool.OutputSchema, tool.Name)
}

// validateAgainstSchema performs basic schema validation
func (sv *SchemaValidator) validateAgainstSchema(ctx context.Context, data interface{}, schema interface{}, toolName string) error {
	// Convert schema to map for easier processing
	schemaMap, ok := schema.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid schema format for tool %s: expected object", toolName)
	}

	schemaType, hasType := schemaMap["type"]
	if !hasType {
		return fmt.Errorf("schema missing 'type' field for tool %s", toolName)
	}

	// Basic type validation
	switch schemaType {
	case "object":
		return sv.validateObject(ctx, data, schemaMap, toolName)
	case "array":
		return sv.validateArray(ctx, data, schemaMap, toolName)
	case "string":
		return sv.validateString(ctx, data, schemaMap, toolName)
	case "number", "integer":
		return sv.validateNumber(ctx, data, schemaMap, toolName)
	case "boolean":
		return sv.validateBoolean(ctx, data, schemaMap, toolName)
	default:
		sv.logger.Warn(ctx, "Unknown schema type", map[string]interface{}{
			"tool_name": toolName,
			"type":      schemaType,
		})
		return nil // Don't fail on unknown types
	}
}

// validateObject validates an object against its schema
func (sv *SchemaValidator) validateObject(ctx context.Context, data interface{}, schema map[string]interface{}, toolName string) error {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("expected object for tool %s, got %T", toolName, data)
	}

	// Check required fields
	required, hasRequired := schema["required"]
	if hasRequired {
		requiredFields, ok := required.([]interface{})
		if ok {
			for _, field := range requiredFields {
				fieldName, ok := field.(string)
				if ok {
					if _, exists := dataMap[fieldName]; !exists {
						return fmt.Errorf("missing required field '%s' in tool %s response", fieldName, toolName)
					}
				}
			}
		}
	}

	// Validate properties if present
	properties, hasProperties := schema["properties"]
	if hasProperties {
		propertiesMap, ok := properties.(map[string]interface{})
		if ok {
			for fieldName, fieldValue := range dataMap {
				if fieldSchema, exists := propertiesMap[fieldName]; exists {
					if err := sv.validateAgainstSchema(ctx, fieldValue, fieldSchema, fmt.Sprintf("%s.%s", toolName, fieldName)); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// validateArray validates an array against its schema
func (sv *SchemaValidator) validateArray(ctx context.Context, data interface{}, schema map[string]interface{}, toolName string) error {
	dataSlice := reflect.ValueOf(data)
	if dataSlice.Kind() != reflect.Slice && dataSlice.Kind() != reflect.Array {
		return fmt.Errorf("expected array for tool %s, got %T", toolName, data)
	}

	// Validate items if schema is present
	items, hasItems := schema["items"]
	if hasItems {
		for i := 0; i < dataSlice.Len(); i++ {
			item := dataSlice.Index(i).Interface()
			if err := sv.validateAgainstSchema(ctx, item, items, fmt.Sprintf("%s[%d]", toolName, i)); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateString validates a string against its schema
func (sv *SchemaValidator) validateString(ctx context.Context, data interface{}, schema map[string]interface{}, toolName string) error {
	if _, ok := data.(string); !ok {
		return fmt.Errorf("expected string for tool %s, got %T", toolName, data)
	}
	// Additional string validations could be added here (minLength, maxLength, pattern, etc.)
	return nil
}

// validateNumber validates a number against its schema
func (sv *SchemaValidator) validateNumber(ctx context.Context, data interface{}, schema map[string]interface{}, toolName string) error {
	switch data.(type) {
	case int, int64, float64, float32:
		return nil
	case json.Number:
		return nil
	default:
		return fmt.Errorf("expected number for tool %s, got %T", toolName, data)
	}
}

// validateBoolean validates a boolean against its schema
func (sv *SchemaValidator) validateBoolean(ctx context.Context, data interface{}, schema map[string]interface{}, toolName string) error {
	if _, ok := data.(bool); !ok {
		return fmt.Errorf("expected boolean for tool %s, got %T", toolName, data)
	}
	return nil
}

// ToolManager provides enhanced tool operations with schema support
type ToolManager struct {
	servers   []interfaces.MCPServer
	validator *SchemaValidator
	logger    logging.Logger
}

// NewToolManager creates a new tool manager with schema validation
func NewToolManager(servers []interfaces.MCPServer) *ToolManager {
	return &ToolManager{
		servers:   servers,
		validator: NewSchemaValidator(),
		logger:    logging.New(),
	}
}

// ListAllTools lists tools from all servers with their output schemas
func (tm *ToolManager) ListAllTools(ctx context.Context) (map[string][]interfaces.MCPTool, error) {
	result := make(map[string][]interfaces.MCPTool)

	for i, server := range tm.servers {
		serverName := fmt.Sprintf("server-%d", i)

		tools, err := server.ListTools(ctx)
		if err != nil {
			tm.logger.Warn(ctx, "Failed to list tools from server", map[string]interface{}{
				"server": serverName,
				"error":  err.Error(),
			})
			continue
		}

		result[serverName] = tools
		tm.logger.Debug(ctx, "Listed tools from server", map[string]interface{}{
			"server":     serverName,
			"tool_count": len(tools),
		})
	}

	return result, nil
}

// CallToolWithValidation calls a tool and validates the response against its schema
func (tm *ToolManager) CallToolWithValidation(ctx context.Context, toolName string, args interface{}) (*interfaces.MCPToolResponse, error) {
	// Find the tool first to get its schema
	var tool *interfaces.MCPTool
	var server interfaces.MCPServer

	for _, srv := range tm.servers {
		tools, err := srv.ListTools(ctx)
		if err != nil {
			continue
		}

		for _, t := range tools {
			if t.Name == toolName {
				tool = &t
				server = srv
				break
			}
		}

		if tool != nil {
			break
		}
	}

	if tool == nil {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	// Call the tool
	response, err := server.CallTool(ctx, toolName, args)
	if err != nil {
		return nil, fmt.Errorf("tool call failed: %w", err)
	}

	// Validate the response if it has structured content and the tool has a schema
	if err := tm.validator.ValidateToolResponse(ctx, *tool, response); err != nil {
		tm.logger.Error(ctx, "Tool response validation failed", map[string]interface{}{
			"tool_name": toolName,
			"error":     err.Error(),
		})
		// Return the response anyway but log the validation error
		// In production, you might want to return the error or set a validation flag
	}

	tm.logger.Debug(ctx, "Tool called successfully", map[string]interface{}{
		"tool_name":          toolName,
		"has_structured":     response.StructuredContent != nil,
		"has_output_schema":  tool.OutputSchema != nil,
	})

	return response, nil
}

// GetToolsByCategory returns tools filtered by category or functionality
func (tm *ToolManager) GetToolsByCategory(ctx context.Context, category string) ([]ToolMatch, error) {
	var matches []ToolMatch

	for _, server := range tm.servers {
		tools, err := server.ListTools(ctx)
		if err != nil {
			continue
		}

		for _, tool := range tools {
			if tm.matchesCategory(tool, category) {
				matches = append(matches, ToolMatch{
					Server: server,
					Tool:   tool,
				})
			}
		}
	}

	return matches, nil
}

// GetToolsWithOutputSchema returns only tools that have output schemas defined
func (tm *ToolManager) GetToolsWithOutputSchema(ctx context.Context) ([]ToolMatch, error) {
	var matches []ToolMatch

	for i, server := range tm.servers {
		serverName := fmt.Sprintf("server-%d", i)

		tools, err := server.ListTools(ctx)
		if err != nil {
			tm.logger.Warn(ctx, "Failed to list tools from server", map[string]interface{}{
				"server": serverName,
				"error":  err.Error(),
			})
			continue
		}

		for _, tool := range tools {
			if tool.OutputSchema != nil {
				matches = append(matches, ToolMatch{
					Server: server,
					Tool:   tool,
				})
			}
		}
	}

	tm.logger.Debug(ctx, "Found tools with output schemas", map[string]interface{}{
		"count": len(matches),
	})

	return matches, nil
}

// Helper types

// ToolMatch represents a tool found on a specific server
type ToolMatch struct {
	Server interfaces.MCPServer
	Tool   interfaces.MCPTool
}

// matchesCategory checks if a tool matches a given category
func (tm *ToolManager) matchesCategory(tool interfaces.MCPTool, category string) bool {
	// Check tool name and description for category keywords
	name := fmt.Sprintf("%s %s", tool.Name, tool.Description)
	return containsIgnoreCaseFunc(name, category)
}

// containsIgnoreCaseFunc is a helper function for case-insensitive string matching
func containsIgnoreCaseFunc(str, substr string) bool {
	return len(str) >= len(substr) &&
		   containsIgnoreCase(str, substr)
}

// Schema builder utilities

// SchemaBuilder helps create JSON schemas for tool outputs
type SchemaBuilder struct {
	schema map[string]interface{}
}

// NewSchemaBuilder creates a new schema builder
func NewSchemaBuilder() *SchemaBuilder {
	return &SchemaBuilder{
		schema: make(map[string]interface{}),
	}
}

// Object creates an object schema
func (sb *SchemaBuilder) Object() *SchemaBuilder {
	sb.schema["type"] = "object"
	sb.schema["properties"] = make(map[string]interface{})
	return sb
}

// Array creates an array schema
func (sb *SchemaBuilder) Array(itemSchema map[string]interface{}) *SchemaBuilder {
	sb.schema["type"] = "array"
	sb.schema["items"] = itemSchema
	return sb
}

// String creates a string schema
func (sb *SchemaBuilder) String() *SchemaBuilder {
	sb.schema["type"] = "string"
	return sb
}

// Number creates a number schema
func (sb *SchemaBuilder) Number() *SchemaBuilder {
	sb.schema["type"] = "number"
	return sb
}

// Boolean creates a boolean schema
func (sb *SchemaBuilder) Boolean() *SchemaBuilder {
	sb.schema["type"] = "boolean"
	return sb
}

// Property adds a property to an object schema
func (sb *SchemaBuilder) Property(name string, propertySchema map[string]interface{}) *SchemaBuilder {
	if properties, exists := sb.schema["properties"]; exists {
		if propsMap, ok := properties.(map[string]interface{}); ok {
			propsMap[name] = propertySchema
		}
	}
	return sb
}

// Required sets required fields for an object schema
func (sb *SchemaBuilder) Required(fields ...string) *SchemaBuilder {
	required := make([]interface{}, len(fields))
	for i, field := range fields {
		required[i] = field
	}
	sb.schema["required"] = required
	return sb
}

// Description adds a description to the schema
func (sb *SchemaBuilder) Description(desc string) *SchemaBuilder {
	sb.schema["description"] = desc
	return sb
}

// Build returns the completed schema
func (sb *SchemaBuilder) Build() map[string]interface{} {
	return sb.schema
}

// Common schema patterns

// CreateWeatherSchema creates a schema for weather data
func CreateWeatherSchema() map[string]interface{} {
	return NewSchemaBuilder().
		Object().
		Property("temperature", map[string]interface{}{
			"type":        "number",
			"description": "Temperature in celsius",
		}).
		Property("conditions", map[string]interface{}{
			"type":        "string",
			"description": "Weather conditions description",
		}).
		Property("humidity", map[string]interface{}{
			"type":        "number",
			"description": "Humidity percentage",
			"minimum":     0,
			"maximum":     100,
		}).
		Required("temperature", "conditions").
		Description("Weather information").
		Build()
}

// CreateFileInfoSchema creates a schema for file information
func CreateFileInfoSchema() map[string]interface{} {
	return NewSchemaBuilder().
		Object().
		Property("name", map[string]interface{}{
			"type":        "string",
			"description": "File name",
		}).
		Property("size", map[string]interface{}{
			"type":        "integer",
			"description": "File size in bytes",
			"minimum":     0,
		}).
		Property("modified", map[string]interface{}{
			"type":        "string",
			"description": "Last modified timestamp",
			"format":      "date-time",
		}).
		Property("type", map[string]interface{}{
			"type":        "string",
			"description": "File type or extension",
		}).
		Required("name", "size").
		Description("File information").
		Build()
}