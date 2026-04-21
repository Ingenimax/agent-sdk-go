package gemini

import (
	"fmt"

	"google.golang.org/genai"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// convertToolsToFunctionDeclarations turns interfaces.Tool definitions into
// Gemini function declarations. It is shared by the non-streaming
// GenerateWithTools path (client.go) and the streaming GenerateStream /
// GenerateStreamWithTools path (streaming.go) so both agree on schema
// details like array items.
func convertToolsToFunctionDeclarations(tools []interfaces.Tool) []*genai.FunctionDeclaration {
	declarations := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		declaration := &genai.FunctionDeclaration{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: make(map[string]*genai.Schema),
				Required:   make([]string, 0),
			},
		}

		for name, param := range tool.Parameters() {
			paramSchema := &genai.Schema{
				Description: param.Description,
			}
			paramSchema.Type = geminiTypeOf(param.Type)

			// Gemini requires `items` on any `array` parameter; the
			// server-supplied schema may omit it, so default to string
			// items when we don't have better information.
			if paramSchema.Type == genai.TypeArray {
				paramSchema.Items = convertItemsToSchema(param.Items)
			}

			if param.Enum != nil {
				enumStrings := make([]string, len(param.Enum))
				for i, e := range param.Enum {
					enumStrings[i] = fmt.Sprintf("%v", e)
				}
				paramSchema.Enum = enumStrings
			}

			declaration.Parameters.Properties[name] = paramSchema
			if param.Required {
				declaration.Parameters.Required = append(declaration.Parameters.Required, name)
			}
		}

		declarations = append(declarations, declaration)
	}
	return declarations
}

// convertItemsToSchema produces a genai.Schema for an array's `items`.
// Never returns nil when called, because Gemini rejects array schemas
// missing `items`.
func convertItemsToSchema(items *interfaces.ParameterSpec) *genai.Schema {
	if items == nil {
		return &genai.Schema{Type: genai.TypeString}
	}
	itemSchema := &genai.Schema{Type: geminiTypeOf(items.Type)}
	if itemSchema.Type == "" {
		itemSchema.Type = genai.TypeString
	}
	if items.Enum != nil {
		enumStrings := make([]string, len(items.Enum))
		for i, e := range items.Enum {
			enumStrings[i] = fmt.Sprintf("%v", e)
		}
		itemSchema.Enum = enumStrings
	}
	return itemSchema
}

// geminiTypeOf maps a ParameterSpec.Type (which may be a string or a
// []string union) to a genai.Type. Returns empty string when unknown so
// callers can decide on a default.
func geminiTypeOf(t interface{}) genai.Type {
	switch v := t.(type) {
	case string:
		switch v {
		case "string":
			return genai.TypeString
		case "number", "integer":
			return genai.TypeNumber
		case "boolean":
			return genai.TypeBoolean
		case "array":
			return genai.TypeArray
		case "object":
			return genai.TypeObject
		}
	case []string:
		for _, s := range v {
			if s == "null" {
				continue
			}
			return geminiTypeOf(s)
		}
	case []interface{}:
		for _, s := range v {
			if str, ok := s.(string); ok && str != "null" {
				return geminiTypeOf(str)
			}
		}
	}
	return ""
}
