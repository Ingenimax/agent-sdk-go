package structuredoutput

import (
	"reflect"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// NewResponseFormat creates a ResponseFormat from a struct type
func NewResponseFormat(v interface{}) *interfaces.ResponseFormat {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	schema := interfaces.JSONSchema{
		"type":       "object",
		"properties": getJSONSchema(t),
		"required":   getRequiredFields(t),
	}

	return &interfaces.ResponseFormat{
		Type:   interfaces.ResponseFormatJSON,
		Name:   t.Name(),
		Schema: schema,
	}
}

func getJSONSchema(t reflect.Type) map[string]any {
	properties := make(map[string]any)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag == "" {
			jsonTag = field.Name
		}

		fieldType := field.Type
		// Handle pointer types by getting the underlying element type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		// Handle nested structs (including pointer to structs)
		if fieldType.Kind() == reflect.Struct {
			requiredFields := getRequiredFields(fieldType)
			// Ensure required is an empty array instead of null when no required fields
			if requiredFields == nil {
				requiredFields = []string{}
			}

			properties[jsonTag] = map[string]any{
				"type":        "object",
				"description": field.Tag.Get("description"),
				"properties":  getJSONSchema(fieldType),
				"required":    requiredFields,
			}
		} else if fieldType.Kind() == reflect.Slice || fieldType.Kind() == reflect.Array {
			// Handle arrays/slices with items property
			itemType := fieldType.Elem()
			// Handle pointer element types in slices
			if itemType.Kind() == reflect.Ptr {
				itemType = itemType.Elem()
			}

			// If the slice contains structs, we need to handle them specially
			if itemType.Kind() == reflect.Struct {
				properties[jsonTag] = map[string]any{
					"type":        "array",
					"description": field.Tag.Get("description"),
					"items": map[string]any{
						"type":       "object",
						"properties": getJSONSchema(itemType),
						"required":   getRequiredFields(itemType),
					},
				}
			} else {
				properties[jsonTag] = map[string]any{
					"type":        "array",
					"description": field.Tag.Get("description"),
					"items": map[string]string{
						"type": getJSONType(itemType),
					},
				}
			}
		} else if fieldType.Kind() == reflect.Map {
			// For maps, we can specify that it's an object with additional properties
			valueType := fieldType.Elem()
			properties[jsonTag] = map[string]any{
				"type":        "object",
				"description": field.Tag.Get("description"),
				"additionalProperties": map[string]string{
					"type": getJSONType(valueType),
				},
			}
		} else {
			properties[jsonTag] = map[string]interface{}{
				"type":        getJSONType(fieldType),
				"description": field.Tag.Get("description"),
			}
		}
	}
	return properties
}

func getJSONType(t reflect.Type) string {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		return getJSONType(t.Elem())
	}

	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map:
		return "object"
	case reflect.Struct:
		return "object"
	case reflect.Interface:
		return "object"
	default:
		return "string"
	}
}

func getRequiredFields(t reflect.Type) []string {
	var required []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !strings.Contains(field.Tag.Get("json"), "omitempty") {
			jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
			if jsonTag == "" {
				jsonTag = field.Name
			}
			required = append(required, jsonTag)
		}
	}
	// Ensure we return an empty array instead of nil
	if required == nil {
		return []string{}
	}
	return required
}
