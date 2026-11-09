package structuredoutput

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

type address struct {
	Street string `json:"street" description:"Street name"`
	Zip    string `json:"zip,omitempty"`
}

type optionalOnly struct {
	Note string `json:"note,omitempty"`
}

type item struct {
	SKU string `json:"sku"`
}

type person struct {
	Name     string  `json:"name" description:"Full name"`
	Age      int     `json:"age"`
	Score    float64 `json:"score,omitempty"`
	Active   bool    `json:"active"`
	Nick     *string `json:"nick,omitempty"`
	NoTag    string
	Secret   string            `json:"-"`
	Home     address           `json:"home" description:"Home address"`
	Work     *address          `json:"work,omitempty"`
	Extra    optionalOnly      `json:"extra"`
	Tags     []string          `json:"tags" description:"Labels"`
	Scores   [3]int            `json:"scores"`
	Items    []item            `json:"items"`
	ItemPtrs []*item           `json:"item_ptrs"`
	Attrs    map[string]int    `json:"attrs" description:"Attributes"`
	Meta     map[string]any    `json:"meta"`
	Labels   map[string]string `json:"labels"`
}

func TestNewResponseFormat(t *testing.T) {
	for _, v := range []any{person{}, &person{}} {
		rf := NewResponseFormat(v)

		assert.Equal(t, interfaces.ResponseFormatJSON, rf.Type)
		assert.Equal(t, "person", rf.Name)
		assert.Equal(t, "object", rf.Schema["type"])
		assert.Equal(t,
			[]string{"name", "age", "active", "NoTag", "home", "extra", "tags", "scores", "items", "item_ptrs", "attrs", "meta", "labels"},
			rf.Schema["required"])
	}
}

func TestNewResponseFormatProperties(t *testing.T) {
	props, ok := NewResponseFormat(person{}).Schema["properties"].(map[string]any)
	require.True(t, ok)

	assert.NotContains(t, props, "-")
	assert.NotContains(t, props, "Secret")

	tests := []struct {
		key  string
		want any
	}{
		{"name", map[string]any{"type": "string", "description": "Full name"}},
		{"age", map[string]any{"type": "integer", "description": ""}},
		{"score", map[string]any{"type": "number", "description": ""}},
		{"active", map[string]any{"type": "boolean", "description": ""}},
		{"nick", map[string]any{"type": "string", "description": ""}},
		{"NoTag", map[string]any{"type": "string", "description": ""}},
		{"home", map[string]any{
			"type":        "object",
			"description": "Home address",
			"properties": map[string]any{
				"street": map[string]any{"type": "string", "description": "Street name"},
				"zip":    map[string]any{"type": "string", "description": ""},
			},
			"required": []string{"street"},
		}},
		{"work", map[string]any{
			"type":        "object",
			"description": "",
			"properties": map[string]any{
				"street": map[string]any{"type": "string", "description": "Street name"},
				"zip":    map[string]any{"type": "string", "description": ""},
			},
			"required": []string{"street"},
		}},
		{"extra", map[string]any{
			"type":        "object",
			"description": "",
			"properties": map[string]any{
				"note": map[string]any{"type": "string", "description": ""},
			},
			"required": []string{},
		}},
		{"tags", map[string]any{
			"type":        "array",
			"description": "Labels",
			"items":       map[string]string{"type": "string"},
		}},
		{"scores", map[string]any{
			"type":        "array",
			"description": "",
			"items":       map[string]string{"type": "integer"},
		}},
		{"items", map[string]any{
			"type":        "array",
			"description": "",
			"items": map[string]any{
				"type":       "object",
				"properties": map[string]any{"sku": map[string]any{"type": "string", "description": ""}},
				"required":   []string{"sku"},
			},
		}},
		{"item_ptrs", map[string]any{
			"type":        "array",
			"description": "",
			"items": map[string]any{
				"type":       "object",
				"properties": map[string]any{"sku": map[string]any{"type": "string", "description": ""}},
				"required":   []string{"sku"},
			},
		}},
		{"attrs", map[string]any{
			"type":                 "object",
			"description":          "Attributes",
			"additionalProperties": map[string]string{"type": "integer"},
		}},
		{"meta", map[string]any{
			"type":                 "object",
			"description":          "",
			"additionalProperties": map[string]string{"type": "object"},
		}},
		{"labels", map[string]any{
			"type":                 "object",
			"description":          "",
			"additionalProperties": map[string]string{"type": "string"},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			assert.Equal(t, tt.want, props[tt.key])
		})
	}
}

func TestGetJSONType(t *testing.T) {
	tests := []struct {
		value any
		want  string
	}{
		{"", "string"},
		{int(0), "integer"},
		{int8(0), "integer"},
		{int16(0), "integer"},
		{int32(0), "integer"},
		{int64(0), "integer"},
		{float32(0), "number"},
		{float64(0), "number"},
		{true, "boolean"},
		{[]int{}, "array"},
		{[2]int{}, "array"},
		{map[string]int{}, "object"},
		{struct{}{}, "object"},
		{new(int), "integer"},
		{uint(0), "string"},
		{make(chan int), "string"},
	}

	for _, tt := range tests {
		typ := reflect.TypeOf(tt.value)
		t.Run(typ.String(), func(t *testing.T) {
			assert.Equal(t, tt.want, getJSONType(typ))
		})
	}

	// Interface types need to be built from a pointer.
	assert.Equal(t, "object", getJSONType(reflect.TypeOf((*any)(nil)).Elem()))
}

func TestGetRequiredFieldsEmpty(t *testing.T) {
	got := getRequiredFields(reflect.TypeOf(optionalOnly{}))
	assert.NotNil(t, got)
	assert.Empty(t, got)
}
