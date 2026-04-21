package mcp

import (
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/google/jsonschema-go/jsonschema"
)

// TestMCPToolParameters_ArrayItems_MapSchema verifies that an array
// parameter sourced from a raw map schema carries its `items` through to
// the ParameterSpec. Gemini (and strict OpenAI) reject tools where an
// `array` parameter is missing `items`.
func TestMCPToolParameters_ArrayItems_MapSchema(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tags": map[string]interface{}{
				"type":        "array",
				"description": "list of tags",
				"items": map[string]interface{}{
					"type": "string",
					"enum": []interface{}{"a", "b"},
				},
			},
			"mode": map[string]interface{}{
				"type": "string",
				"enum": []interface{}{"fast", "slow"},
			},
		},
		"required": []interface{}{"tags"},
	}

	tool := NewMCPTool("t", "d", schema, nil)
	params := tool.Parameters()

	tags, ok := params["tags"]
	if !ok {
		t.Fatalf("missing tags param")
	}
	if !tags.Required {
		t.Fatalf("tags should be required")
	}
	if tags.Items == nil {
		t.Fatalf("array param dropped Items")
	}
	if tags.Items.Type != "string" {
		t.Fatalf("unexpected items type: %v", tags.Items.Type)
	}
	if len(tags.Items.Enum) != 2 {
		t.Fatalf("items enum lost: %v", tags.Items.Enum)
	}

	mode := params["mode"]
	if len(mode.Enum) != 2 {
		t.Fatalf("top-level enum lost: %v", mode.Enum)
	}
}

// TestMCPToolParameters_ArrayMissingItems_DefaultsToString mirrors the
// real-world case (e.g. the GitHub MCP server's "files" field) where the
// server advertises `type:array` without an `items` entry. Emitting such a
// tool to Gemini used to fail with INVALID_ARGUMENT
// "properties[...].items: missing field".
func TestMCPToolParameters_ArrayMissingItems_DefaultsToString(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"files": map[string]interface{}{
				"type":        "array",
				"description": "files to commit",
			},
		},
	}
	tool := NewMCPTool("t", "d", schema, nil)
	params := tool.Parameters()
	if params["files"].Items == nil || params["files"].Items.Type != "string" {
		t.Fatalf("expected default string items, got %+v", params["files"].Items)
	}
}

// TestMCPToolParameters_ArrayItems_JSONSchema covers the *jsonschema.Schema
// branch of Parameters; previously it dropped both Items and Enum.
func TestMCPToolParameters_ArrayItems_JSONSchema(t *testing.T) {
	schema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"labels": {
				Type: "array",
				Items: &jsonschema.Schema{
					Type: "string",
					Enum: []any{"bug", "feature"},
				},
			},
		},
		Required: []string{"labels"},
	}

	tool := NewMCPTool("t", "d", schema, nil)
	params := tool.Parameters()

	labels, ok := params["labels"]
	if !ok {
		t.Fatalf("missing labels param")
	}
	if !labels.Required {
		t.Fatalf("labels should be required")
	}
	if labels.Items == nil || labels.Items.Type != "string" {
		t.Fatalf("items dropped or wrong type: %+v", labels.Items)
	}
	if len(labels.Items.Enum) != 2 {
		t.Fatalf("items enum lost: %v", labels.Items.Enum)
	}
}

// Ensure ParameterSpec.Items was actually exposed by the interface we
// depend on; this guards against silent renames upstream.
var _ = interfaces.ParameterSpec{Items: &interfaces.ParameterSpec{}}
