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

		// Handle nested structs
		if field.Type.Kind() == reflect.Struct {
			requiredFields := getRequiredFields(field.Type)
			// Ensure required is an empty array instead of null when no required fields
			if requiredFields == nil {
				requiredFields = []string{}
			}

			properties[jsonTag] = map[string]any{
				"type":        "object",
				"description": field.Tag.Get("description"),
				"properties":  getJSONSchema(field.Type),
				"required":    requiredFields,
			}
		} else if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Array {
			// Handle arrays/slices with items property
			properties[jsonTag] = map[string]any{
				"type":        "array",
				"description": field.Tag.Get("description"),
				"items": map[string]string{
					"type": getJSONType(field.Type.Elem()),
				},
			}
		} else {
			properties[jsonTag] = map[string]string{
				"type":        getJSONType(field.Type),
				"description": field.Tag.Get("description"),
			}
		}
	}
	return properties
}

func getJSONType(t reflect.Type) string {
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
	case reflect.Struct:
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
