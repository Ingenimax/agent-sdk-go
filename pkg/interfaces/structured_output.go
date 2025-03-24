package interfaces

import "encoding/json"

// ResponseFormat defines the format of the response from the LLM
type ResponseFormat struct {
	Type   ResponseFormatType
	Name   string     // The name of the struct/object to be returned
	Schema JSONSchema // JSON schema representation of the struct
}

type JSONSchema map[string]interface{}

// MarshalJSON implements the json.Marshaler interface
func (s JSONSchema) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(s))
}

type ResponseFormatType string

const (
	ResponseFormatJSON ResponseFormatType = "json_object"
	ResponseFormatText ResponseFormatType = "text"
)
