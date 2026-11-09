package weaviate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weaviate/weaviate/entities/models"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func strPtr(s string) *string   { return &s }
func numPtr(f float64) *float64 { return &f }
func testStore() *Store         { return &Store{logger: logging.New()} }
func build(f map[string]interface{}) *models.WhereFilter {
	w := testStore().buildWhereFilter(f)
	if w == nil {
		return nil
	}
	return w.Build()
}

func TestBuildWhereFilterDirect(t *testing.T) {
	path := []string{"field"}
	tests := []struct {
		name   string
		filter map[string]interface{}
		want   *models.WhereFilter
	}{
		{"equal", map[string]interface{}{"operator": "Equal", "path": path, "valueString": "x"},
			&models.WhereFilter{Operator: "Equal", Path: path, ValueString: strPtr("x")}},
		{"not equal with string path", map[string]interface{}{"operator": "NotEqual", "path": "field", "valueString": 5},
			&models.WhereFilter{Operator: "NotEqual", Path: path, ValueString: strPtr("5")}},
		{"greater than with interface path", map[string]interface{}{"operator": "GreaterThan", "path": []interface{}{"field"}, "valueNumber": 2},
			&models.WhereFilter{Operator: "GreaterThan", Path: path, ValueNumber: numPtr(2)}},
		{"greater than equal", map[string]interface{}{"operator": "GreaterThanEqual", "path": path, "valueNumber": "1.5"},
			&models.WhereFilter{Operator: "GreaterThanEqual", Path: path, ValueNumber: numPtr(1.5)}},
		{"less than", map[string]interface{}{"operator": "LessThan", "path": path, "valueNumber": int64(3)},
			&models.WhereFilter{Operator: "LessThan", Path: path, ValueNumber: numPtr(3)}},
		{"less than equal", map[string]interface{}{"operator": "LessThanEqual", "path": path, "valueNumber": float32(4)},
			&models.WhereFilter{Operator: "LessThanEqual", Path: path, ValueNumber: numPtr(4)}},
		{"like", map[string]interface{}{"operator": "Like", "path": path, "valueString": "go*"},
			&models.WhereFilter{Operator: "Like", Path: path, ValueString: strPtr("go*")}},
		{"contains any strings", map[string]interface{}{"operator": "ContainsAny", "path": path, "valueString": []string{"a", "b"}},
			&models.WhereFilter{Operator: "ContainsAny", Path: path, ValueStringArray: []string{"a", "b"}}},
		{"contains any interfaces", map[string]interface{}{"operator": "ContainsAny", "path": path, "valueString": []interface{}{"a", 1}},
			&models.WhereFilter{Operator: "ContainsAny", Path: path, ValueStringArray: []string{"a", "1"}}},
		{"contains any scalar", map[string]interface{}{"operator": "ContainsAny", "path": path, "valueString": "a"},
			&models.WhereFilter{Operator: "ContainsAny", Path: path, ValueStringArray: []string{"a"}}},
		{"missing value", map[string]interface{}{"operator": "Equal", "path": path}, nil},
		{"unsupported operator", map[string]interface{}{"operator": "WithinGeoRange", "path": path, "valueString": "x"}, nil},
		{"missing operator", map[string]interface{}{"path": path, "valueString": "x"}, nil},
		{"empty", map[string]interface{}{}, nil},
		{"nil", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, build(tt.filter))
		})
	}
}

func TestBuildWhereFilterOperands(t *testing.T) {
	a := map[string]interface{}{"operator": "Equal", "path": []string{"a"}, "valueString": "1"}
	b := map[string]interface{}{"operator": "GreaterThan", "path": []string{"b"}, "valueNumber": 2.0}
	invalid := map[string]interface{}{"path": []string{"c"}}

	and := build(map[string]interface{}{"operator": "And", "operands": []interface{}{a, b}})
	require.NotNil(t, and)
	assert.Equal(t, "And", and.Operator)
	require.Len(t, and.Operands, 2)
	assert.Equal(t, []string{"a"}, and.Operands[0].Path)
	assert.Equal(t, []string{"b"}, and.Operands[1].Path)

	or := build(map[string]interface{}{"operator": "Or", "operands": []interface{}{a}})
	require.NotNil(t, or)
	assert.Equal(t, "Or", or.Operator)

	nested := build(map[string]interface{}{"operator": "And", "operands": []interface{}{
		map[string]interface{}{"operator": "Or", "operands": []interface{}{a, b}},
	}})
	require.NotNil(t, nested)
	require.Len(t, nested.Operands, 1)
	assert.Equal(t, "Or", nested.Operands[0].Operator)
	assert.Len(t, nested.Operands[0].Operands, 2)

	tests := []struct {
		name   string
		filter map[string]interface{}
	}{
		{"missing operator", map[string]interface{}{"operands": []interface{}{a}}},
		{"operands not a slice", map[string]interface{}{"operator": "And", "operands": "nope"}},
		{"unsupported operator", map[string]interface{}{"operator": "Xor", "operands": []interface{}{a}}},
		{"no valid operands", map[string]interface{}{"operator": "And", "operands": []interface{}{invalid}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Nil(t, build(tt.filter))
		})
	}
}
