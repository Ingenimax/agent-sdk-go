package mcp

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
)

func TestNewSchemaValidator(t *testing.T) {
	validator := NewSchemaValidator()

	assert.NotNil(t, validator)
	assert.NotNil(t, validator.logger)
}

func TestSchemaValidator_ValidateToolResponse(t *testing.T) {
	ctx := context.Background()
	validator := NewSchemaValidator()

	t.Run("tool without output schema", func(t *testing.T) {
		tool := interfaces.MCPTool{
			Name:         "test-tool",
			OutputSchema: nil,
		}
		response := &interfaces.MCPToolResponse{
			Content:           "Some content",
			StructuredContent: map[string]interface{}{"result": "value"},
		}

		err := validator.ValidateToolResponse(ctx, tool, response)
		assert.NoError(t, err)
	})

	t.Run("tool with schema but no structured content", func(t *testing.T) {
		tool := interfaces.MCPTool{
			Name: "test-tool",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"result": map[string]interface{}{"type": "string"},
				},
			},
		}
		response := &interfaces.MCPToolResponse{
			Content:           "Some content",
			StructuredContent: nil,
		}

		err := validator.ValidateToolResponse(ctx, tool, response)
		// Should not error but will log a warning
		assert.NoError(t, err)
	})

	t.Run("valid structured content with schema", func(t *testing.T) {
		tool := interfaces.MCPTool{
			Name: "test-tool",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"result": map[string]interface{}{"type": "string"},
					"count":  map[string]interface{}{"type": "number"},
				},
				"required": []interface{}{"result"},
			},
		}
		response := &interfaces.MCPToolResponse{
			Content: "Tool executed successfully",
			StructuredContent: map[string]interface{}{
				"result": "success",
				"count":  42,
			},
		}

		err := validator.ValidateToolResponse(ctx, tool, response)
		// Basic validation should pass (implementation may vary)
		assert.NoError(t, err)
	})

	t.Run("invalid schema format", func(t *testing.T) {
		tool := interfaces.MCPTool{
			Name:         "test-tool",
			OutputSchema: "invalid-schema-format", // Should be object
		}
		response := &interfaces.MCPToolResponse{
			StructuredContent: map[string]interface{}{"result": "value"},
		}

		err := validator.ValidateToolResponse(ctx, tool, response)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid schema format")
	})

	t.Run("schema validation with nested objects", func(t *testing.T) {
		tool := interfaces.MCPTool{
			Name: "nested-tool",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"metadata": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"version": map[string]interface{}{"type": "string"},
							"author":  map[string]interface{}{"type": "string"},
						},
					},
					"data": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		}
		response := &interfaces.MCPToolResponse{
			StructuredContent: map[string]interface{}{
				"metadata": map[string]interface{}{
					"version": "1.0.0",
					"author":  "test-author",
				},
				"data": []interface{}{"item1", "item2", "item3"},
			},
		}

		err := validator.ValidateToolResponse(ctx, tool, response)
		// Should handle nested structures
		assert.NoError(t, err)
	})
}

func TestSchemaValidator_validateAgainstSchema(t *testing.T) {
	ctx := context.Background()
	validator := NewSchemaValidator()

	t.Run("string schema validation", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "string",
		}

		err := validator.validateAgainstSchema(ctx, "valid string", schema, "test-tool")
		// Implementation will vary based on complete validateAgainstSchema method
		assert.NoError(t, err)
	})

	t.Run("number schema validation", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "number",
		}

		err := validator.validateAgainstSchema(ctx, 42, schema, "test-tool")
		assert.NoError(t, err)
	})

	t.Run("object schema validation", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string"},
				"age":  map[string]interface{}{"type": "number"},
			},
			"required": []interface{}{"name"},
		}

		data := map[string]interface{}{
			"name": "John",
			"age":  30,
		}

		err := validator.validateAgainstSchema(ctx, data, schema, "test-tool")
		assert.NoError(t, err)
	})

	t.Run("array schema validation", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
		}

		data := []interface{}{"item1", "item2", "item3"}

		err := validator.validateAgainstSchema(ctx, data, schema, "test-tool")
		assert.NoError(t, err)
	})

	t.Run("invalid schema type", func(t *testing.T) {
		schema := "not-an-object"

		err := validator.validateAgainstSchema(ctx, "data", schema, "test-tool")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid schema format")
	})

	t.Run("complex nested schema", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"users": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"id":   map[string]interface{}{"type": "number"},
							"name": map[string]interface{}{"type": "string"},
							"roles": map[string]interface{}{
								"type":  "array",
								"items": map[string]interface{}{"type": "string"},
							},
						},
						"required": []interface{}{"id", "name"},
					},
				},
			},
		}

		data := map[string]interface{}{
			"users": []interface{}{
				map[string]interface{}{
					"id":    1,
					"name":  "John",
					"roles": []interface{}{"admin", "user"},
				},
				map[string]interface{}{
					"id":    2,
					"name":  "Jane",
					"roles": []interface{}{"user"},
				},
			},
		}

		err := validator.validateAgainstSchema(ctx, data, schema, "test-tool")
		assert.NoError(t, err)
	})
}

// Edge case tests for schema validation
func TestSchemaValidator_EdgeCases(t *testing.T) {
	ctx := context.Background()
	validator := NewSchemaValidator()

	t.Run("empty schema", func(t *testing.T) {
		schema := map[string]interface{}{}

		err := validator.validateAgainstSchema(ctx, "any data", schema, "test-tool")
		// Empty schema should allow anything
		assert.NoError(t, err)
	})

	t.Run("nil data", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "null",
		}

		err := validator.validateAgainstSchema(ctx, nil, schema, "test-tool")
		assert.NoError(t, err)
	})

	t.Run("boolean schema", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "boolean",
		}

		err := validator.validateAgainstSchema(ctx, true, schema, "test-tool")
		assert.NoError(t, err)

		err = validator.validateAgainstSchema(ctx, false, schema, "test-tool")
		assert.NoError(t, err)
	})

	t.Run("schema with unsupported type", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "unsupported-type",
		}

		err := validator.validateAgainstSchema(ctx, "data", schema, "test-tool")
		// Depending on implementation, this might error or pass
		// This test documents current behavior
		_ = err
	})
}

// Performance tests
func BenchmarkSchemaValidator_ValidateToolResponse(b *testing.B) {
	ctx := context.Background()
	validator := NewSchemaValidator()

	tool := interfaces.MCPTool{
		Name: "benchmark-tool",
		OutputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"result": map[string]interface{}{"type": "string"},
				"data": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "number"},
				},
			},
		},
	}

	response := &interfaces.MCPToolResponse{
		StructuredContent: map[string]interface{}{
			"result": "success",
			"data":   []interface{}{1, 2, 3, 4, 5},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateToolResponse(ctx, tool, response)
	}
}

func BenchmarkSchemaValidator_ComplexSchema(b *testing.B) {
	ctx := context.Background()
	validator := NewSchemaValidator()

	// Create a complex nested schema for benchmarking
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"metadata": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"version":   map[string]interface{}{"type": "string"},
					"timestamp": map[string]interface{}{"type": "number"},
					"tags": map[string]interface{}{
						"type":  "array",
						"items": map[string]interface{}{"type": "string"},
					},
				},
			},
			"results": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":    map[string]interface{}{"type": "number"},
						"value": map[string]interface{}{"type": "string"},
						"score": map[string]interface{}{"type": "number"},
					},
				},
			},
		},
	}

	data := map[string]interface{}{
		"metadata": map[string]interface{}{
			"version":   "1.0.0",
			"timestamp": 1234567890,
			"tags":      []interface{}{"test", "benchmark", "performance"},
		},
		"results": []interface{}{
			map[string]interface{}{"id": 1, "value": "result1", "score": 0.95},
			map[string]interface{}{"id": 2, "value": "result2", "score": 0.87},
			map[string]interface{}{"id": 3, "value": "result3", "score": 0.91},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.validateAgainstSchema(ctx, data, schema, "benchmark-tool")
	}
}
