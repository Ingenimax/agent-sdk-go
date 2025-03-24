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

		properties[jsonTag] = map[string]string{
			"type":        getJSONType(field.Type),
			"description": field.Tag.Get("description"), // Optional: support for description tags
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
	return required
}
