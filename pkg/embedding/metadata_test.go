package embedding

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestNewMetadataFilterGroup(t *testing.T) {
	f1 := NewMetadataFilter("a", "=", 1)
	g := NewMetadataFilterGroup("OR", f1)
	assert.Equal(t, "or", g.Operator)
	assert.Equal(t, []MetadataFilter{{Field: "a", Operator: "=", Value: 1}}, g.Filters)

	f2 := NewMetadataFilter("b", ">", 2)
	g.AddFilter(f2)
	g.AddSubGroup(NewMetadataFilterGroup("and", f1))
	assert.Len(t, g.Filters, 2)
	assert.Len(t, g.SubGroups, 1)
}

func TestEvaluateFilterOperators(t *testing.T) {
	meta := map[string]interface{}{
		"count":   10,
		"score":   0.5,
		"type":    "article",
		"title":   "Go testing guide",
		"created": time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		"tags":    []string{"go"},
	}
	tests := []struct {
		name   string
		filter MetadataFilter
		want   bool
	}{
		{"eq", NewMetadataFilter("type", "=", "article"), true},
		{"eq alias", NewMetadataFilter("type", "eq", "blog"), false},
		{"double eq", NewMetadataFilter("count", "==", 10), true},
		{"ne", NewMetadataFilter("type", "!=", "blog"), true},
		{"ne alias", NewMetadataFilter("type", "<>", "article"), false},
		{"gt int vs float", NewMetadataFilter("count", ">", 9.5), true},
		{"gt false", NewMetadataFilter("count", "gt", 10), false},
		{"gte", NewMetadataFilter("count", ">=", 10), true},
		{"lt", NewMetadataFilter("score", "<", 1), true},
		{"lte", NewMetadataFilter("score", "lte", 0.5), true},
		{"string compare", NewMetadataFilter("type", ">", "aaa"), true},
		{"time compare", NewMetadataFilter("created", "<", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)), true},
		{"time equal", NewMetadataFilter("created", ">=", time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)), true},
		{"time after", NewMetadataFilter("created", ">", time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)), true},
		{"contains", NewMetadataFilter("title", "contains", "testing"), true},
		{"contains false", NewMetadataFilter("title", "contains", "rust"), false},
		{"in slice", NewMetadataFilter("type", "in", []string{"blog", "article"}), true},
		{"in array", NewMetadataFilter("count", "in", [2]int{1, 10}), true},
		{"in scalar", NewMetadataFilter("type", "in", "article"), true},
		{"in miss", NewMetadataFilter("type", "in", []interface{}{"blog"}), false},
		{"not_in", NewMetadataFilter("type", "not_in", []string{"blog"}), true},
		{"unknown operator", NewMetadataFilter("type", "regex", "a.*"), false},
		{"missing field", NewMetadataFilter("missing", "=", "x"), false},
		{"mixed types fall back to string form", NewMetadataFilter("tags", ">", "[a]"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, evaluateFilter(meta, tt.filter))
		})
	}
}

func TestEvaluateNestedFilter(t *testing.T) {
	meta := map[string]interface{}{
		"user": map[string]interface{}{
			"name":    "ana",
			"profile": map[interface{}]interface{}{"age": 30, 1: "ignored"},
		},
		"flat": "value",
	}
	tests := []struct {
		name   string
		filter MetadataFilter
		want   bool
	}{
		{"one level", NewMetadataFilter("user.name", "=", "ana"), true},
		{"interface map", NewMetadataFilter("user.profile.age", ">", 18), true},
		{"missing path", NewMetadataFilter("user.email", "=", "x"), false},
		{"missing parent", NewMetadataFilter("org.name", "=", "x"), false},
		{"parent not a map", NewMetadataFilter("flat.inner", "=", "x"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, evaluateFilter(meta, tt.filter))
		})
	}
}

func TestEvaluateFilterGroup(t *testing.T) {
	meta := map[string]interface{}{"a": 1, "b": 2}
	yes := NewMetadataFilter("a", "=", 1)
	no := NewMetadataFilter("b", "=", 3)

	tests := []struct {
		name  string
		group MetadataFilterGroup
		want  bool
	}{
		{"empty", MetadataFilterGroup{}, true},
		{"and all true", NewMetadataFilterGroup("and", yes, yes), true},
		{"and one false", NewMetadataFilterGroup("and", yes, no), false},
		{"default is and", NewMetadataFilterGroup("", yes, no), false},
		{"or one true", NewMetadataFilterGroup("or", no, yes), true},
		{"or none true", NewMetadataFilterGroup("or", no), false},
		{"unknown operator", NewMetadataFilterGroup("xor", yes), false},
		{"or with true subgroup", MetadataFilterGroup{
			Operator:  "or",
			Filters:   []MetadataFilter{no},
			SubGroups: []MetadataFilterGroup{NewMetadataFilterGroup("and", yes)},
		}, true},
		{"or with false subgroup", MetadataFilterGroup{
			Operator:  "or",
			SubGroups: []MetadataFilterGroup{NewMetadataFilterGroup("and", no)},
		}, false},
		{"and with false subgroup", MetadataFilterGroup{
			Operator:  "and",
			Filters:   []MetadataFilter{yes},
			SubGroups: []MetadataFilterGroup{NewMetadataFilterGroup("and", no)},
		}, false},
		{"and with true subgroup", MetadataFilterGroup{
			Operator:  "and",
			SubGroups: []MetadataFilterGroup{NewMetadataFilterGroup("or", yes)},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, evaluateFilterGroup(meta, tt.group))
		})
	}
}

func TestApplyFilters(t *testing.T) {
	docs := []interfaces.Document{
		{ID: "1", Metadata: map[string]interface{}{"words": 5}},
		{ID: "2", Metadata: map[string]interface{}{"words": 50}},
		{ID: "3", Metadata: map[string]interface{}{}},
	}

	assert.Equal(t, docs, ApplyFilters(docs, MetadataFilterGroup{}))

	got := ApplyFilters(docs, NewMetadataFilterGroup("and", NewMetadataFilter("words", ">", 10)))
	assert.Len(t, got, 1)
	assert.Equal(t, "2", got[0].ID)
}

func TestCompare(t *testing.T) {
	tests := []struct {
		name string
		a, b interface{}
		want int
	}{
		{"both nil", nil, nil, 0},
		{"a nil", nil, 1, -1},
		{"b nil", 1, nil, 1},
		{"ints equal", 3, 3, 0},
		{"uint vs float", uint8(2), 1.5, 1},
		{"strings", "a", "b", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, compare(tt.a, tt.b))
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		in   interface{}
		want float64
	}{
		{int64(-4), -4},
		{uint32(7), 7},
		{float32(1.5), 1.5},
		{"2.25", 2.25},
		{"not a number", 0},
		{true, 0},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, toFloat64(tt.in), "%v", tt.in)
	}
}

func TestToTime(t *testing.T) {
	want := time.Date(2024, 3, 4, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, want, toTime(want))
	assert.Equal(t, want, toTime("2024-03-04"))
	assert.Equal(t, want.Add(time.Hour), toTime("2024-03-04 01:00:00"))
	assert.True(t, toTime("bad").IsZero())
	assert.True(t, toTime(42).IsZero())
}

func TestFilterToMap(t *testing.T) {
	single := FilterToMap(NewMetadataFilterGroup("and", NewMetadataFilter("a", ">", 1)))
	assert.Equal(t, map[string]interface{}{
		"a": map[string]interface{}{"operator": "greaterThan", "value": 1},
	}, single)

	group := NewMetadataFilterGroup("OR",
		NewMetadataFilter("a", "=", 1),
		NewMetadataFilter("b", "not_in", []string{"x"}),
	)
	group.AddSubGroup(NewMetadataFilterGroup("and", NewMetadataFilter("c", "<", 2)))
	assert.Equal(t, map[string]interface{}{
		"or": []map[string]interface{}{
			{"a": map[string]interface{}{"operator": "equals", "value": 1}},
			{"b": map[string]interface{}{"operator": "notIn", "value": []string{"x"}}},
			{"c": map[string]interface{}{"operator": "lessThan", "value": 2}},
		},
	}, FilterToMap(group))

	assert.Empty(t, FilterToMap(MetadataFilterGroup{}))
}

func TestOperatorToMapKey(t *testing.T) {
	tests := map[string]string{
		"==": "equals", "ne": "notEquals", "gt": "greaterThan", ">=": "greaterThanEqual",
		"lt": "lessThan", "<=": "lessThanEqual", "contains": "contains", "in": "in",
		"not_in": "notIn",
	}
	for in, want := range tests {
		assert.Equal(t, want, operatorToMapKey(in), in)
	}
}

func TestWeaviateFilterFromSingle(t *testing.T) {
	tests := []struct {
		name   string
		filter MetadataFilter
		want   map[string]interface{}
	}{
		{"eq", NewMetadataFilter("f", "=", "5"),
			map[string]interface{}{"path": []string{"f"}, "operator": "Equal", "valueString": "5"}},
		{"ne", NewMetadataFilter("f", "!=", "x"),
			map[string]interface{}{"path": []string{"f"}, "operator": "NotEqual", "valueString": "x"}},
		{"gt", NewMetadataFilter("f", ">", 1),
			map[string]interface{}{"path": []string{"f"}, "operator": "GreaterThan", "valueNumber": 1.0}},
		{"gte", NewMetadataFilter("f", "gte", "2"),
			map[string]interface{}{"path": []string{"f"}, "operator": "GreaterThanEqual", "valueNumber": 2.0}},
		{"lt", NewMetadataFilter("f", "<", 3),
			map[string]interface{}{"path": []string{"f"}, "operator": "LessThan", "valueNumber": 3.0}},
		{"lte", NewMetadataFilter("f", "<=", 4),
			map[string]interface{}{"path": []string{"f"}, "operator": "LessThanEqual", "valueNumber": 4.0}},
		{"contains", NewMetadataFilter("f", "contains", "go"),
			map[string]interface{}{"path": []string{"f"}, "operator": "Like", "valueString": "go"}},
		{"in list", NewMetadataFilter("f", "in", []interface{}{"a", 1}),
			map[string]interface{}{"path": []string{"f"}, "operator": "ContainsAny", "valueString": []string{"a", "1"}}},
		{"in scalar", NewMetadataFilter("f", "in", "a"),
			map[string]interface{}{"path": []string{"f"}, "operator": "ContainsAny", "valueString": []string{"a"}}},
		{"not_in list", NewMetadataFilter("f", "not_in", []interface{}{"a"}),
			map[string]interface{}{"path": []string{"f"}, "operator": "NotContainsAny", "valueString": []string{"a"}}},
		{"not_in scalar", NewMetadataFilter("f", "not_in", 2),
			map[string]interface{}{"path": []string{"f"}, "operator": "NotContainsAny", "valueString": []string{"2"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, weaviateFilterFromSingle(tt.filter))
			assert.Equal(t, tt.want, CreateWeaviateFilter(tt.filter.Field, tt.filter.Operator, tt.filter.Value))
		})
	}
}

func TestFilterToWeaviateFormat(t *testing.T) {
	a := NewMetadataFilter("a", "=", "x")
	b := NewMetadataFilter("b", ">", 1)

	assert.Equal(t, weaviateFilterFromSingle(a), FilterToWeaviateFormat(NewMetadataFilterGroup("and", a)))
	assert.Nil(t, FilterToWeaviateFormat(MetadataFilterGroup{}))

	group := NewMetadataFilterGroup("and", a)
	group.AddSubGroup(NewMetadataFilterGroup("or", a, b))
	assert.Equal(t, map[string]interface{}{
		"operator": "And",
		"operands": []map[string]interface{}{
			weaviateFilterFromSingle(a),
			{
				"operator": "Or",
				"operands": []map[string]interface{}{weaviateFilterFromSingle(a), weaviateFilterFromSingle(b)},
			},
		},
	}, FilterToWeaviateFormat(group))
}

func TestCreateWeaviateAndOrFilter(t *testing.T) {
	c1 := CreateWeaviateFilter("a", "=", 1)
	c2 := CreateWeaviateFilter("b", "=", 2)

	assert.Nil(t, CreateWeaviateAndFilter())
	assert.Nil(t, CreateWeaviateOrFilter())
	assert.Equal(t, c1, CreateWeaviateAndFilter(c1))
	assert.Equal(t, c1, CreateWeaviateOrFilter(c1))
	assert.Equal(t, map[string]interface{}{
		"operator": "And", "operands": []map[string]interface{}{c1, c2},
	}, CreateWeaviateAndFilter(c1, c2))
	assert.Equal(t, map[string]interface{}{
		"operator": "Or", "operands": []map[string]interface{}{c1, c2},
	}, CreateWeaviateOrFilter(c1, c2))
}
